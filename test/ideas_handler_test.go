package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/ideas"
)

type ideasTestEnv struct {
	handler *ideas.Handler
	svc     *ideas.Service
	router  *chi.Mux
}

func setupIdeasEnv(t *testing.T) *ideasTestEnv {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n- [ ] Existing idea [status: untriaged] [added: 2026-03-16]\n  Some body text.\n"), 0o644)

	svc := ideas.NewService(path)

	toTask := func(ctx context.Context, title, body string, tags []string) error {
		return nil
	}

	h := ideas.NewHandler(svc, toTask, nil)

	r := chi.NewRouter()
	r.Post("/ideas/add", h.QuickAdd)
	r.Post("/ideas/{slug}/triage", h.TriageAction)
	r.Post("/ideas/{slug}/edit", h.Edit)
	r.Post("/ideas/{slug}/to-task", h.ToTask)
	r.Post("/ideas/{slug}/delete", h.DeleteIdea)

	return &ideasTestEnv{handler: h, svc: svc, router: r}
}

func TestIdeasQuickAddFlash(t *testing.T) {
	env := setupIdeasEnv(t)
	form := url.Values{"title": {"New idea"}}
	req := httptest.NewRequest("POST", "/ideas/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=idea-added") {
		t.Errorf("expected idea-added flash, got %q", loc)
	}
}

func TestIdeasQuickAddEmptyTitleFlash(t *testing.T) {
	env := setupIdeasEnv(t)
	form := url.Values{"title": {""}}
	req := httptest.NewRequest("POST", "/ideas/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=title-required") {
		t.Errorf("expected title-required flash, got %q", loc)
	}
}

func TestIdeasTriageFlash(t *testing.T) {
	env := setupIdeasEnv(t)
	form := url.Values{"action": {"park"}}
	req := httptest.NewRequest("POST", "/ideas/existing-idea/triage", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=idea-triaged") {
		t.Errorf("expected idea-triaged flash, got %q", loc)
	}
}

func TestIdeasEditFlash(t *testing.T) {
	env := setupIdeasEnv(t)
	form := url.Values{"body": {"Updated body"}, "tags": {"tag1"}}
	req := httptest.NewRequest("POST", "/ideas/existing-idea/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=idea-edited") {
		t.Errorf("expected idea-edited flash, got %q", loc)
	}
}

func TestIdeasToTaskFlash(t *testing.T) {
	env := setupIdeasEnv(t)
	req := httptest.NewRequest("POST", "/ideas/existing-idea/to-task", nil)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=idea-converted") {
		t.Errorf("expected idea-converted flash, got %q", loc)
	}
}

func TestIdeasDeleteFlash(t *testing.T) {
	env := setupIdeasEnv(t)
	req := httptest.NewRequest("POST", "/ideas/existing-idea/delete", nil)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=idea-deleted") {
		t.Errorf("expected idea-deleted flash, got %q", loc)
	}
}
