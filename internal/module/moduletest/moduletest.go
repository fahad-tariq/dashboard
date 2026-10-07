// Package moduletest is a permanent fixture module. It exercises every
// capability of the module contract and is registered only by tests.
//
// It keeps a list of lines in DataDir/moduletest.md, one "- " item per line.
package moduletest

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/atomicfile"
	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/module"
)

// ID is the module ID, its template directory and its SSE event suffix.
const ID = "moduletest"

// Module is the fixture.
type Module struct {
	deps module.Deps
	path string

	mu   sync.Mutex
	last []byte // file content as last read or written
}

// New builds the fixture.
func New(deps module.Deps) module.Module {
	return &Module{deps: deps, path: filepath.Join(deps.DataDir, ID+".md")}
}

// Path is the watched file.
func (m *Module) Path() string { return m.path }

func (m *Module) Manifest() module.Manifest {
	return module.Manifest{
		ID:          ID,
		Title:       "Fixture",
		Nav:         []module.NavItem{{Label: "fixture", Path: "/moduletest", Order: 100, Group: module.More, Shortcut: "x"}},
		Prefixes:    []string{"/moduletest"},
		Flash:       map[string]string{"added": "Added.", "add-failed": "Could not add."},
		FlashErrors: []string{"add-failed"},
	}
}

func (m *Module) Routes(r chi.Router) {
	r.Get("/moduletest", m.page)
	r.Post("/moduletest/add", m.add)
}

func (m *Module) APIRoutes(r chi.Router) {
	r.Get("/moduletest", func(w http.ResponseWriter, _ *http.Request) {
		items, err := m.items()
		if err != nil {
			httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "unreadable"})
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]int{"count": len(items)})
	})
}

func (m *Module) Watches() []module.WatchSpec {
	return []module.WatchSpec{{Path: m.path, Reload: func(int64) (bool, error) { return m.reload() }}}
}

func (m *Module) Search(_ context.Context, _ int64, q string) []module.SearchResult {
	items, _ := m.items()
	var out []module.SearchResult
	for _, it := range items {
		if strings.Contains(strings.ToLower(it), strings.ToLower(q)) {
			out = append(out, module.SearchResult{Title: it, Category: ID, URL: "/moduletest"})
		}
	}
	return out
}

func (m *Module) Widgets(_ context.Context, _ int64, _ time.Time) []module.WidgetData {
	items, err := m.items()
	if err != nil {
		return nil
	}
	data := module.WidgetData{Title: "Fixture items", Count: len(items), Link: "/moduletest", EmptyText: "Nothing here."}
	for i, it := range items {
		data.Items = append(data.Items, module.WidgetItem{
			ID: strconv.Itoa(i), Label: it, URL: "/moduletest",
			Meta:   &module.WidgetMeta{Text: "fixture", Level: "attention"},
			Action: &module.WidgetAction{Path: "/moduletest/add", Fields: map[string]string{"title": it + " again"}, Text: "again", Label: "Add " + it + " again"},
		})
	}
	return []module.WidgetData{data}
}

func (m *Module) page(w http.ResponseWriter, r *http.Request) {
	items, err := m.items()
	data := map[string]any{"Items": items}
	if err != nil {
		data["Error"] = "Could not read the fixture list."
	}
	m.deps.Render.Page(w, r, m.Manifest(), "page.html", data)
}

func (m *Module) add(w http.ResponseWriter, r *http.Request) {
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" || strings.ContainsAny(title, "\r\n") {
		http.Redirect(w, r, "/moduletest?msg=add-failed", http.StatusSeeOther)
		return
	}
	m.mu.Lock()
	content, err := os.ReadFile(m.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		m.mu.Unlock()
		httputil.ServerError(w, "reading fixture list", err)
		return
	}
	content = append(content, []byte("- "+title+"\n")...)
	err = atomicfile.Write(m.path, content, 0o644)
	if err == nil {
		m.last = content
	}
	m.mu.Unlock()
	if err != nil {
		httputil.ServerError(w, "writing fixture list", err)
		return
	}
	m.deps.Publish(ID)
	http.Redirect(w, r, "/moduletest?msg=added", http.StatusSeeOther)
}

// items reads the list; a missing file is an empty list.
func (m *Module) items() ([]string, error) {
	content, err := os.ReadFile(m.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for line := range strings.SplitSeq(string(content), "\n") {
		if it, ok := strings.CutPrefix(line, "- "); ok && it != "" {
			out = append(out, it)
		}
	}
	return out, nil
}

// reload reports whether the file differs from the module's own last write.
func (m *Module) reload() (bool, error) {
	content, err := os.ReadFile(m.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if bytes.Equal(content, m.last) {
		return false, nil
	}
	m.last = content
	return true, nil
}
