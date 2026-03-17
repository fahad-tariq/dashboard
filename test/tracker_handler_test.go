package test

import (
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
	"github.com/fahad/dashboard/internal/tracker"
)

type trackerTestEnv struct {
	personalHandler *tracker.Handler
	familyHandler   *tracker.Handler
	personalSvc     *tracker.Service
	familySvc       *tracker.Service
	router          *chi.Mux
}

func setupTrackerEnv(t *testing.T) *trackerTestEnv {
	t.Helper()

	dir := t.TempDir()
	personalPath := filepath.Join(dir, "personal.md")
	familyPath := filepath.Join(dir, "family.md")
	os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] Existing task\n"), 0o644)
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)

	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	personalStore := tracker.NewStore(database, "personal")
	familyStore := tracker.NewStore(database, "family")
	personalSvc := tracker.NewService(personalPath, "Personal", personalStore)
	familySvc := tracker.NewService(familyPath, "Family", familyStore)

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
	for _, name := range []string{"tracker.html", "goals.html"} {
		tmpl, _ := template.Must(layout.Clone()).Parse(
			`{{define "content"}}` + name + `|Title={{.Title}}|FlashMsg={{.FlashMsg}}{{end}}`,
		)
		templates[name] = tmpl
	}

	personalHandler := tracker.NewHandler(personalSvc, familySvc, templates, "todos")
	familyHandler := tracker.NewHandler(familySvc, personalSvc, templates, "family")

	r := chi.NewRouter()
	r.Get("/todos", personalHandler.TrackerPage)
	r.Get("/goals", personalHandler.GoalsPage)
	r.Post("/todos/add", personalHandler.QuickAdd)
	r.Post("/todos/add-goal", personalHandler.AddGoal)
	r.Post("/todos/{slug}/complete", personalHandler.Complete)
	r.Post("/todos/{slug}/uncomplete", personalHandler.Uncomplete)
	r.Post("/todos/{slug}/notes", personalHandler.UpdateNotes)
	r.Post("/todos/{slug}/edit", personalHandler.UpdateEdit)
	r.Post("/todos/{slug}/delete", personalHandler.Delete)
	r.Post("/todos/{slug}/priority", personalHandler.UpdatePriority)
	r.Post("/todos/{slug}/tags", personalHandler.UpdateTags)
	r.Post("/todos/{slug}/move", personalHandler.MoveToList)
	r.Post("/todos/{slug}/progress", personalHandler.UpdateProgress)
	r.Post("/todos/{slug}/restore", personalHandler.Restore)
	r.Post("/todos/{slug}/purge", personalHandler.Purge)

	r.Get("/family", familyHandler.TrackerPage)
	r.Post("/family/add", familyHandler.QuickAdd)
	r.Post("/family/{slug}/complete", familyHandler.Complete)
	r.Post("/family/{slug}/uncomplete", familyHandler.Uncomplete)
	r.Post("/family/{slug}/delete", familyHandler.Delete)
	r.Post("/family/{slug}/move", familyHandler.MoveToList)
	r.Post("/family/{slug}/restore", familyHandler.Restore)
	r.Post("/family/{slug}/purge", familyHandler.Purge)

	return &trackerTestEnv{
		personalHandler: personalHandler,
		familyHandler:   familyHandler,
		personalSvc:     personalSvc,
		familySvc:       familySvc,
		router:          r,
	}
}

func postForm(router *chi.Mux, path string, values url.Values) *httptest.ResponseRecorder {
	body := values.Encode()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestTrackerPageRenders(t *testing.T) {
	env := setupTrackerEnv(t)

	req := httptest.NewRequest("GET", "/todos", nil)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "tracker.html") {
		t.Error("expected tracker.html template to render")
	}
	if !strings.Contains(body, "Title=Todos") {
		t.Errorf("expected Title=Todos in body, got: %s", body)
	}
}

func TestGoalsPageRenders(t *testing.T) {
	env := setupTrackerEnv(t)

	req := httptest.NewRequest("GET", "/goals", nil)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "goals.html") {
		t.Error("expected goals.html template to render")
	}
	if !strings.Contains(body, "Title=Goals") {
		t.Errorf("expected Title=Goals in body, got: %s", body)
	}
}

func TestQuickAddTask(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/add", url.Values{
		"title": {"Buy groceries"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/todos?msg=task-added" {
		t.Errorf("expected redirect to /todos?msg=task-added, got %q", loc)
	}

	items, err := env.personalSvc.List()
	if err != nil {
		t.Fatalf("listing items: %v", err)
	}
	found := false
	for _, it := range items {
		if it.Title == "Buy groceries" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Buy groceries' to exist after quick add")
	}
}

func TestQuickAddEmptyTitle(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/add", url.Values{
		"title": {""},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "msg=title-required") {
		t.Errorf("expected redirect with msg=title-required, got %q", loc)
	}
}

func TestAddGoal(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/add-goal", url.Values{
		"title":   {"Read books"},
		"current": {"2"},
		"target":  {"12"},
		"unit":    {"books"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/goals?msg=goal-added" {
		t.Errorf("expected redirect to /goals?msg=goal-added, got %q", loc)
	}

	items, err := env.personalSvc.List()
	if err != nil {
		t.Fatalf("listing items: %v", err)
	}
	found := false
	for _, it := range items {
		if it.Title == "Read books" && it.Type == tracker.GoalType {
			if it.Current != 2 || it.Target != 12 || it.Unit != "books" {
				t.Errorf("goal metadata mismatch: current=%g target=%g unit=%s", it.Current, it.Target, it.Unit)
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("expected goal 'Read books' to exist after add")
	}
}

func TestCompleteTask(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/existing-task/complete", nil)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get("existing-task")
	if err != nil {
		t.Fatalf("getting item: %v", err)
	}
	if !item.Done {
		t.Error("expected item to be marked done")
	}
	if item.Completed == "" {
		t.Error("expected completed date to be set")
	}
}

func TestUncompleteTask(t *testing.T) {
	env := setupTrackerEnv(t)

	// First complete it.
	postForm(env.router, "/todos/existing-task/complete", nil)
	// Then uncomplete it.
	rr := postForm(env.router, "/todos/existing-task/uncomplete", nil)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get("existing-task")
	if err != nil {
		t.Fatalf("getting item: %v", err)
	}
	if item.Done {
		t.Error("expected item to not be done after uncomplete")
	}
	if item.Completed != "" {
		t.Errorf("expected completed date to be cleared, got %q", item.Completed)
	}
}

func TestUpdateNotes(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/existing-task/notes", url.Values{
		"body": {"Some notes here"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get("existing-task")
	if err != nil {
		t.Fatalf("getting item: %v", err)
	}
	if item.Body != "Some notes here" {
		t.Errorf("expected body 'Some notes here', got %q", item.Body)
	}
}

func TestUpdateEdit(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/existing-task/edit", url.Values{
		"title":  {"Existing task"},
		"body":   {"Updated body"},
		"tags":   {"work, urgent"},
		"images": {"img1.jpg"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get("existing-task")
	if err != nil {
		t.Fatalf("getting item: %v", err)
	}
	if item.Body != "Updated body" {
		t.Errorf("expected body 'Updated body', got %q", item.Body)
	}
	if len(item.Tags) != 2 || item.Tags[0] != "work" || item.Tags[1] != "urgent" {
		t.Errorf("expected tags [work, urgent], got %v", item.Tags)
	}
	if len(item.Images) != 1 || item.Images[0] != "img1.jpg" {
		t.Errorf("expected images [img1.jpg], got %v", item.Images)
	}
}

func TestUpdateEditTitle(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/existing-task/edit", url.Values{
		"title": {"Renamed task"},
		"body":  {""},
		"tags":  {""},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Old slug should no longer exist.
	_, err := env.personalSvc.Get("existing-task")
	if err == nil {
		t.Error("expected old slug 'existing-task' to not exist after rename")
	}

	// New slug should exist.
	item, err := env.personalSvc.Get("renamed-task")
	if err != nil {
		t.Fatalf("getting renamed item: %v", err)
	}
	if item.Title != "Renamed task" {
		t.Errorf("expected title 'Renamed task', got %q", item.Title)
	}
}

func TestDeleteTask(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/existing-task/delete", nil)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Delete is now soft-delete: item still exists via Get but is excluded from List.
	item, err := env.personalSvc.Get("existing-task")
	if err != nil {
		t.Fatalf("expected soft-deleted item to still be accessible via Get: %v", err)
	}
	if item.DeletedAt == "" {
		t.Error("expected DeletedAt to be set after soft delete")
	}

	items, _ := env.personalSvc.List()
	for _, it := range items {
		if it.Slug == "existing-task" {
			t.Error("soft-deleted item should not appear in List()")
		}
	}
}

func TestUpdatePriority(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/existing-task/priority", url.Values{
		"priority": {"high"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get("existing-task")
	if err != nil {
		t.Fatalf("getting item: %v", err)
	}
	if item.Priority != "high" {
		t.Errorf("expected priority 'high', got %q", item.Priority)
	}
}

func TestUpdateTags(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/existing-task/tags", url.Values{
		"tags": {"finance, health"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get("existing-task")
	if err != nil {
		t.Fatalf("getting item: %v", err)
	}
	if len(item.Tags) != 2 || item.Tags[0] != "finance" || item.Tags[1] != "health" {
		t.Errorf("expected tags [finance, health], got %v", item.Tags)
	}
}

func TestMoveToList(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/existing-task/move", nil)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/todos?msg=item-moved" {
		t.Errorf("expected redirect to /todos?msg=item-moved, got %q", loc)
	}

	// Item should no longer be in personal list.
	_, err := env.personalSvc.Get("existing-task")
	if err == nil {
		t.Error("expected item to be removed from personal list")
	}

	// Item should now be in family list.
	item, err := env.familySvc.Get("existing-task")
	if err != nil {
		t.Fatalf("expected item in family list: %v", err)
	}
	if item.Title != "Existing task" {
		t.Errorf("expected title 'Existing task', got %q", item.Title)
	}
}

func TestUpdateProgress(t *testing.T) {
	env := setupTrackerEnv(t)

	// Add a goal first.
	postForm(env.router, "/todos/add-goal", url.Values{
		"title":   {"Run distance"},
		"current": {"5"},
		"target":  {"100"},
		"unit":    {"km"},
	})

	rr := postForm(env.router, "/todos/run-distance/progress", url.Values{
		"delta": {"10"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get("run-distance")
	if err != nil {
		t.Fatalf("getting goal: %v", err)
	}
	if item.Current != 15 {
		t.Errorf("expected current=15 after delta +10, got %g", item.Current)
	}
}

func TestSetProgress(t *testing.T) {
	env := setupTrackerEnv(t)

	// Add a goal first.
	postForm(env.router, "/todos/add-goal", url.Values{
		"title":   {"Save money"},
		"current": {"100"},
		"target":  {"1000"},
		"unit":    {"dollars"},
	})

	rr := postForm(env.router, "/todos/save-money/progress", url.Values{
		"delta": {"500"},
		"set":   {"1"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get("save-money")
	if err != nil {
		t.Fatalf("getting goal: %v", err)
	}
	if item.Current != 500 {
		t.Errorf("expected current=500 after set, got %g", item.Current)
	}
}

func TestCompleteNonExistent(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/nonexistent/complete", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestFamilyPageRenders(t *testing.T) {
	env := setupTrackerEnv(t)

	req := httptest.NewRequest("GET", "/family", nil)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "tracker.html") {
		t.Error("expected tracker.html template to render")
	}
	if !strings.Contains(body, "Title=Family Tasks") {
		t.Errorf("expected Title=Family Tasks in body, got: %s", body)
	}
}

func TestRestoreTask(t *testing.T) {
	env := setupTrackerEnv(t)

	// Soft delete first.
	postForm(env.router, "/todos/existing-task/delete", nil)

	// Then restore.
	rr := postForm(env.router, "/todos/existing-task/restore", nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get("existing-task")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.DeletedAt != "" {
		t.Error("expected DeletedAt to be cleared after restore")
	}

	items, _ := env.personalSvc.List()
	if len(items) != 1 {
		t.Errorf("expected 1 item in List after restore, got %d", len(items))
	}
}

func TestPurgeTask(t *testing.T) {
	env := setupTrackerEnv(t)

	// Soft delete first.
	postForm(env.router, "/todos/existing-task/delete", nil)

	// Then permanently delete.
	rr := postForm(env.router, "/todos/existing-task/purge", nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	_, err := env.personalSvc.Get("existing-task")
	if err == nil {
		t.Error("expected item to be permanently deleted")
	}
}
