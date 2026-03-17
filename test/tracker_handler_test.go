package test

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	dbpkg "github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/tracker"
)

type trackerTestEnv struct {
	handler *tracker.Handler
	svc     *tracker.Service
	router  *chi.Mux
}

func setupTrackerEnv(t *testing.T) *trackerTestEnv {
	t.Helper()
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "personal.md")
	os.WriteFile(mdPath, []byte("# Personal\n\n- [ ] Existing task\n"), 0o644)

	database, err := dbpkg.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	store := tracker.NewStore(database, "personal")
	svc := tracker.NewService(mdPath, "Personal", store)

	funcMap := template.FuncMap{
		"authEnabled":  func() bool { return false },
		"buildVersion": func() string { return "test" },
		"linkify":      func(s string) template.HTML { return template.HTML(s) },
		"percentage":   func(a, b float64) float64 { return 0 },
		"formatNum":    func(f float64) string { return "" },
		"subtract":     func(a, b int) int { return a - b },
	}
	layout := template.Must(template.New("layout.html").Funcs(funcMap).Parse(
		`{{define "layout.html"}}{{template "content" .}}{{end}}`,
	))
	templates := make(map[string]*template.Template)
	for _, name := range []string{"tracker.html", "goals.html"} {
		tmpl, _ := template.Must(layout.Clone()).Parse(
			`{{define "content"}}rendered{{end}}`,
		)
		templates[name] = tmpl
	}

	h := tracker.NewHandler(svc, svc, templates, "todos")

	r := chi.NewRouter()
	r.Post("/todos/add", h.QuickAdd)
	r.Post("/todos/add-goal", h.AddGoal)
	r.Post("/todos/{slug}/complete", h.Complete)
	r.Post("/todos/{slug}/uncomplete", h.Uncomplete)
	r.Post("/todos/{slug}/notes", h.UpdateNotes)
	r.Post("/todos/{slug}/priority", h.UpdatePriority)
	r.Post("/todos/{slug}/tags", h.UpdateTags)
	r.Post("/todos/{slug}/edit", h.UpdateEdit)
	r.Post("/todos/{slug}/delete", h.Delete)
	r.Post("/todos/{slug}/move", h.MoveToList)

	return &trackerTestEnv{handler: h, svc: svc, router: r}
}

func TestTrackerQuickAddFlash(t *testing.T) {
	env := setupTrackerEnv(t)
	form := url.Values{"title": {"New task"}}
	req := httptest.NewRequest("POST", "/todos/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=task-added") {
		t.Errorf("expected task-added flash, got %q", loc)
	}
}

func TestTrackerQuickAddEmptyTitleFlash(t *testing.T) {
	env := setupTrackerEnv(t)
	form := url.Values{"title": {""}}
	req := httptest.NewRequest("POST", "/todos/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=title-required") {
		t.Errorf("expected title-required flash, got %q", loc)
	}
}

func TestTrackerAddGoalFlash(t *testing.T) {
	env := setupTrackerEnv(t)
	form := url.Values{"title": {"New goal"}, "current": {"0"}, "target": {"10"}, "unit": {"km"}}
	req := httptest.NewRequest("POST", "/todos/add-goal", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=goal-added") {
		t.Errorf("expected goal-added flash, got %q", loc)
	}
}

func TestTrackerCompleteFlash(t *testing.T) {
	env := setupTrackerEnv(t)
	req := httptest.NewRequest("POST", "/todos/existing-task/complete", nil)
	req.Header.Set("Referer", "http://localhost/todos")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=task-completed") {
		t.Errorf("expected task-completed flash, got %q", loc)
	}
}

func TestTrackerDeleteFlash(t *testing.T) {
	env := setupTrackerEnv(t)
	req := httptest.NewRequest("POST", "/todos/existing-task/delete", nil)
	req.Header.Set("Referer", "http://localhost/todos")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=item-deleted") {
		t.Errorf("expected item-deleted flash, got %q", loc)
	}
}

func TestTrackerMoveFlash(t *testing.T) {
	env := setupTrackerEnv(t)

	// Need to set up chi URL param context for the move handler.
	req := httptest.NewRequest("POST", "/todos/existing-task/move", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", "existing-task")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=item-moved") {
		t.Errorf("expected item-moved flash, got %q", loc)
	}
}
