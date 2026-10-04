package test

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/sse"
	"github.com/fahad/dashboard/internal/tracker"
	"github.com/fahad/dashboard/internal/watcher"
)

// changeSource is what every markdown-backed service offers: publish after
// its own writes, and re-read only when the file differs from that write.
type changeSource interface {
	OnChange(func())
	ResyncIfChanged() (bool, error)
}

func TestServicesPublishOwnWritesAndSkipOwnResync(t *testing.T) {
	tests := map[string]struct {
		file    string
		initial string
		build   func(path string) changeSource
		mutate  func(svc changeSource) error
		edited  string
	}{
		"tracker": {
			file: "personal.md", initial: "# Personal\n\n",
			build: func(p string) changeSource { return tracker.NewService(p, "Personal", time.UTC) },
			mutate: func(s changeSource) error {
				return s.(*tracker.Service).AddItem(tracker.Item{Title: "Mow", Type: tracker.TaskType})
			},
			edited: "# Personal\n\n- [ ] Edited by hand\n",
		},
		"ideas": {
			file: "ideas.md", initial: "# Ideas\n\n",
			build:  func(p string) changeSource { return ideas.NewService(p, time.UTC) },
			mutate: func(s changeSource) error { return s.(*ideas.Service).Add(&ideas.Idea{Title: "Solar logger"}) },
			edited: "# Ideas\n\n## Untriaged\n\n- [ ] Edited by hand\n",
		},
		"maintenance": {
			file: "maintenance.md", initial: "# Maintenance\n\n",
			build: func(p string) changeSource { return house.NewService(p, time.UTC) },
			mutate: func(s changeSource) error {
				return s.(*house.Service).Add(&house.MaintenanceItem{Title: "Clean gutters", Cadence: "6m"})
			},
			edited: "# Maintenance\n\n- [ ] Service the heater [cadence: 1y]\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.file)
			if err := os.WriteFile(path, []byte(tc.initial), 0o644); err != nil {
				t.Fatal(err)
			}
			svc := tc.build(path)
			var events atomic.Int32
			svc.OnChange(func() { events.Add(1) })

			if err := tc.mutate(svc); err != nil {
				t.Fatal(err)
			}
			if n := events.Load(); n != 1 {
				t.Errorf("own write published %d events, want 1", n)
			}
			if changed, err := svc.ResyncIfChanged(); err != nil || changed {
				t.Errorf("ResyncIfChanged after own write = %v (%v), want false", changed, err)
			}

			if err := os.WriteFile(path, []byte(tc.edited), 0o644); err != nil {
				t.Fatal(err)
			}
			if changed, err := svc.ResyncIfChanged(); err != nil || !changed {
				t.Errorf("ResyncIfChanged after external edit = %v (%v), want true", changed, err)
			}
			if changed, _ := svc.ResyncIfChanged(); changed {
				t.Error("second ResyncIfChanged for the same external content re-read the file")
			}
		})
	}
}

// End to end through the real watcher and broker: a web mutation and an API
// mutation each produce exactly one event and no re-parse; an external edit
// produces one of each.
func TestOneEventPerChangeAndNoSelfResync(t *testing.T) {
	dir := t.TempDir()
	personalPath := filepath.Join(dir, "personal.md")
	familyPath := filepath.Join(dir, "family.md")
	for _, p := range []string{personalPath, familyPath} {
		if err := os.WriteFile(p, []byte("# List\n\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	personal := tracker.NewService(personalPath, "Personal", time.UTC)
	family := tracker.NewService(familyPath, "Family", time.UTC)

	broker := sse.NewBroker()
	personal.OnChange(func() { broker.Send("file-changed", "personal") })
	family.OnChange(func() { broker.Send("file-changed", "family") })

	var resyncs atomic.Int32
	callback := func(svc *tracker.Service) func() bool {
		return func() bool {
			changed, err := svc.ResyncIfChanged()
			if err != nil {
				t.Errorf("resync: %v", err)
			}
			if changed {
				resyncs.Add(1)
			}
			return changed
		}
	}
	err := watcher.Watch(nil,
		map[string]string{personalPath: "personal", familyPath: "family"},
		broker,
		map[string]func() bool{"personal": callback(personal), "family": callback(family)})
	if err != nil {
		t.Fatal(err)
	}

	events := make(chan string, 16)
	broker.Subscribe(events)
	t.Cleanup(func() { broker.Unsubscribe(events) })
	// Debounce is 500ms; wait well past it before counting.
	settle := func() int {
		time.Sleep(1500 * time.Millisecond)
		n := 0
		for {
			select {
			case <-events:
				n++
			default:
				return n
			}
		}
	}

	h := tracker.NewHandler(personal, family, map[string]*template.Template{}, "todos", time.UTC)
	r := chi.NewRouter()
	r.Post("/todos/add", h.QuickAdd)
	r.Post("/api/v1/todos", tracker.APIAddTodo(personal, family))

	steps := []struct {
		name            string
		do              func()
		events, resyncs int32
	}{
		{"web mutation", func() {
			req := httptest.NewRequest("POST", "/todos/add", strings.NewReader(url.Values{"title": {"From the web"}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.ServeHTTP(httptest.NewRecorder(), req)
		}, 1, 0},
		{"API mutation", func() {
			req := httptest.NewRequest("POST", "/api/v1/todos", strings.NewReader(`{"title":"From the API","list":"personal"}`))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			if rr.Code >= http.StatusBadRequest {
				t.Fatalf("API add: %d %s", rr.Code, rr.Body.String())
			}
		}, 1, 0},
		{"external edit", func() {
			if err := os.WriteFile(familyPath, []byte("# Family\n\n- [ ] Edited by hand\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, 1, 1},
	}
	for _, step := range steps {
		before := resyncs.Load()
		step.do()
		if n := settle(); int32(n) != step.events {
			t.Errorf("%s: %d events, want %d", step.name, n, step.events)
		}
		if n := resyncs.Load() - before; n != step.resyncs {
			t.Errorf("%s: %d resyncs, want %d", step.name, n, step.resyncs)
		}
	}
}
