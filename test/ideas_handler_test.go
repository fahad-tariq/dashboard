package test

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/tracker"
)

type ideasTestEnv struct {
	handler    *ideas.Handler
	ideasSvc   *ideas.Service
	personalSvc *tracker.Service
	router     *chi.Mux
}

func setupIdeasEnv(t *testing.T) *ideasTestEnv {
	t.Helper()
	dir := t.TempDir()
	ideasPath := filepath.Join(dir, "ideas.md")
	personalPath := filepath.Join(dir, "personal.md")
	os.WriteFile(ideasPath, []byte("# Ideas\n\n"), 0o644)
	os.WriteFile(personalPath, []byte("# Personal\n\n"), 0o644)

	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	ideasSvc := ideas.NewService(ideasPath)
	personalStore := tracker.NewStore(database, "personal")
	personalSvc := tracker.NewService(personalPath, "Personal", personalStore)

	toTask := func(_ context.Context, title, body string, tags []string) error {
		return personalSvc.AddItem(tracker.Item{
			Title: title,
			Type:  tracker.TaskType,
			Body:  body,
			Tags:  tags,
		})
	}

	funcMap := template.FuncMap{
		"authEnabled":  func() bool { return false },
		"buildVersion": func() string { return "test" },
		"percentage":   func(c, t float64) int { return 0 },
		"formatNum":    func(f float64) string { return fmt.Sprintf("%g", f) },
		"subtract":     func(a, b int) int { return a - b },
		"linkify":      func(text string) template.HTML { return template.HTML(text) },
	}
	layout := template.Must(template.New("layout.html").Funcs(funcMap).Parse(
		`{{define "layout.html"}}{{template "content" .}}{{end}}`,
	))
	templates := make(map[string]*template.Template)
	for _, name := range []string{"ideas.html", "idea.html"} {
		tmpl, _ := template.Must(layout.Clone()).Parse(
			`{{define "content"}}` + name + `|Title={{.Title}}{{end}}`,
		)
		templates[name] = tmpl
	}

	handler := ideas.NewHandler(ideasSvc, toTask, templates)

	r := chi.NewRouter()
	r.Get("/ideas", handler.IdeasPage)
	r.Get("/ideas/{slug}", handler.IdeaDetail)
	r.Post("/ideas/add", handler.QuickAdd)
	r.Post("/ideas/{slug}/triage", handler.TriageAction)
	r.Post("/ideas/{slug}/to-task", handler.ToTask)
	r.Post("/ideas/{slug}/edit", handler.Edit)
	r.Post("/ideas/{slug}/delete", handler.DeleteIdea)

	return &ideasTestEnv{
		handler:     handler,
		ideasSvc:    ideasSvc,
		personalSvc: personalSvc,
		router:      r,
	}
}

// addTestIdea is a helper that adds an idea via the service and returns its slug.
func addTestIdea(t *testing.T, svc *ideas.Service, title string) string {
	t.Helper()
	slug := ideas.Slugify(title)
	err := svc.Add(&ideas.Idea{
		Slug:  slug,
		Title: title,
		Tags:  []string{"test"},
	})
	if err != nil {
		t.Fatalf("adding test idea: %v", err)
	}
	return slug
}

func TestIdeasPageRenders(t *testing.T) {
	env := setupIdeasEnv(t)

	req := httptest.NewRequest("GET", "/ideas", nil)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "ideas.html") {
		t.Error("expected ideas.html template to render")
	}
	if !strings.Contains(body, "Title=Ideas") {
		t.Errorf("expected Title=Ideas, got: %s", body)
	}
}

func TestIdeaDetailRenders(t *testing.T) {
	env := setupIdeasEnv(t)
	slug := addTestIdea(t, env.ideasSvc, "Build a widget")

	req := httptest.NewRequest("GET", "/ideas/"+slug, nil)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "idea.html") {
		t.Error("expected idea.html template to render")
	}
	if !strings.Contains(body, "Title=Build a widget") {
		t.Errorf("expected idea title in output, got: %s", body)
	}
}

func TestIdeaDetailNotFound(t *testing.T) {
	env := setupIdeasEnv(t)

	req := httptest.NewRequest("GET", "/ideas/nonexistent", nil)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestIdeasQuickAdd(t *testing.T) {
	env := setupIdeasEnv(t)

	form := url.Values{
		"title": {"My new idea"},
		"tags":  {"golang, testing"},
	}
	req := httptest.NewRequest("POST", "/ideas/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/ideas" {
		t.Errorf("expected redirect to /ideas, got %q", loc)
	}

	// Verify idea was created.
	slug := ideas.Slugify("My new idea")
	idea, err := env.ideasSvc.Get(slug)
	if err != nil {
		t.Fatalf("idea not found after quick add: %v", err)
	}
	if idea.Title != "My new idea" {
		t.Errorf("expected title 'My new idea', got %q", idea.Title)
	}
	if len(idea.Tags) != 2 || idea.Tags[0] != "golang" || idea.Tags[1] != "testing" {
		t.Errorf("expected tags [golang, testing], got %v", idea.Tags)
	}
}

func TestIdeasQuickAddEmptyTitle(t *testing.T) {
	env := setupIdeasEnv(t)

	form := url.Values{
		"title": {""},
		"tags":  {"test"},
	}
	req := httptest.NewRequest("POST", "/ideas/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "msg=title-required") {
		t.Errorf("expected redirect with msg=title-required, got %q", loc)
	}
}

func TestIdeasTriageAction(t *testing.T) {
	env := setupIdeasEnv(t)
	slug := addTestIdea(t, env.ideasSvc, "Triage me")

	form := url.Values{"action": {"park"}}
	req := httptest.NewRequest("POST", "/ideas/"+slug+"/triage", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify status changed.
	idea, err := env.ideasSvc.Get(slug)
	if err != nil {
		t.Fatalf("idea not found: %v", err)
	}
	if idea.Status != "parked" {
		t.Errorf("expected status 'parked', got %q", idea.Status)
	}
}

func TestIdeasTriageInvalidAction(t *testing.T) {
	env := setupIdeasEnv(t)
	slug := addTestIdea(t, env.ideasSvc, "Bad triage")

	form := url.Values{"action": {"invalid"}}
	req := httptest.NewRequest("POST", "/ideas/"+slug+"/triage", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestIdeasToTask(t *testing.T) {
	env := setupIdeasEnv(t)
	slug := addTestIdea(t, env.ideasSvc, "Convert me")

	req := httptest.NewRequest("POST", "/ideas/"+slug+"/to-task", nil)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify idea was deleted.
	_, err := env.ideasSvc.Get(slug)
	if err == nil {
		t.Error("expected idea to be deleted after to-task conversion")
	}

	// Verify task was created in personal tracker.
	items, err := env.personalSvc.List()
	if err != nil {
		t.Fatalf("listing personal items: %v", err)
	}
	found := false
	for _, item := range items {
		if item.Title == "Convert me" && item.Type == tracker.TaskType {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected task 'Convert me' to exist in personal tracker")
	}
}

func TestIdeasEdit(t *testing.T) {
	env := setupIdeasEnv(t)
	slug := addTestIdea(t, env.ideasSvc, "Edit me")

	form := url.Values{
		"title":  {"Edit me"},
		"body":   {"Updated body content"},
		"tags":   {"updated, tags"},
		"images": {""},
	}
	req := httptest.NewRequest("POST", "/ideas/"+slug+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify changes.
	idea, err := env.ideasSvc.Get(slug)
	if err != nil {
		t.Fatalf("idea not found: %v", err)
	}
	if idea.Body != "Updated body content" {
		t.Errorf("expected updated body, got %q", idea.Body)
	}
	if len(idea.Tags) != 2 || idea.Tags[0] != "updated" || idea.Tags[1] != "tags" {
		t.Errorf("expected tags [updated, tags], got %v", idea.Tags)
	}
}

func TestIdeasEditTitle(t *testing.T) {
	env := setupIdeasEnv(t)
	slug := addTestIdea(t, env.ideasSvc, "Old title")

	form := url.Values{
		"title":  {"New title"},
		"body":   {""},
		"tags":   {""},
		"images": {""},
	}
	req := httptest.NewRequest("POST", "/ideas/"+slug+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Old slug should no longer work.
	_, err := env.ideasSvc.Get(slug)
	if err == nil {
		t.Error("expected old slug to no longer resolve")
	}

	// New slug should work.
	newSlug := ideas.Slugify("New title")
	idea, err := env.ideasSvc.Get(newSlug)
	if err != nil {
		t.Fatalf("idea not found under new slug: %v", err)
	}
	if idea.Title != "New title" {
		t.Errorf("expected title 'New title', got %q", idea.Title)
	}
}

func TestIdeasEditNonExistent(t *testing.T) {
	env := setupIdeasEnv(t)

	form := url.Values{
		"title":  {"Anything"},
		"body":   {"Anything"},
		"tags":   {""},
		"images": {""},
	}
	req := httptest.NewRequest("POST", "/ideas/nonexistent/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestIdeasDelete(t *testing.T) {
	env := setupIdeasEnv(t)
	slug := addTestIdea(t, env.ideasSvc, "Delete me")

	req := httptest.NewRequest("POST", "/ideas/"+slug+"/delete", nil)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify idea is gone.
	_, err := env.ideasSvc.Get(slug)
	if err == nil {
		t.Error("expected idea to be deleted")
	}
}

func TestIdeasDeleteNonExistent(t *testing.T) {
	env := setupIdeasEnv(t)

	req := httptest.NewRequest("POST", "/ideas/nonexistent/delete", nil)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}
