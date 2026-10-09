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
	"time"

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

// existingTask is the ID of the task setupTrackerEnv and setupAPIEnv seed.
const existingTask = "xst1ngtk"

func setupTrackerEnv(t *testing.T) *trackerTestEnv {
	t.Helper()

	dir := t.TempDir()
	personalPath := filepath.Join(dir, "personal.md")
	familyPath := filepath.Join(dir, "family.md")
	if err := os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] Existing task [id: xst1ngtk]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	personalSvc := tracker.NewService(personalPath, "Personal", time.UTC)
	familySvc := tracker.NewService(familyPath, "Family", time.UTC)

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

	personalHandler := tracker.NewHandler(personalSvc, familySvc, templates, "todos", time.UTC)
	familyHandler := tracker.NewHandler(familySvc, personalSvc, templates, "family", time.UTC)

	r := chi.NewRouter()
	r.Get("/todos", personalHandler.TrackerPage)
	r.Get("/goals", personalHandler.GoalsPage)
	r.Post("/todos/add", personalHandler.QuickAdd)
	r.Post("/todos/add-goal", personalHandler.AddGoal)
	r.Post("/todos/{id}/complete", personalHandler.Complete)
	r.Post("/todos/{id}/uncomplete", personalHandler.Uncomplete)
	r.Post("/todos/{id}/notes", personalHandler.UpdateNotes)
	r.Post("/todos/{id}/edit", personalHandler.UpdateEdit)
	r.Post("/todos/{id}/delete", personalHandler.Delete)
	r.Post("/todos/{id}/priority", personalHandler.UpdatePriority)
	r.Post("/todos/{id}/tags", personalHandler.UpdateTags)
	r.Post("/todos/{id}/move", personalHandler.MoveToList)
	r.Post("/todos/{id}/progress", personalHandler.UpdateProgress)
	r.Post("/todos/{id}/restore", personalHandler.Restore)
	r.Post("/todos/{id}/purge", personalHandler.Purge)
	r.Post("/todos/bulk/complete", personalHandler.BulkComplete)
	r.Post("/todos/bulk/delete", personalHandler.BulkDelete)
	r.Post("/todos/bulk/priority", personalHandler.BulkPriority)
	r.Post("/todos/bulk/tag", personalHandler.BulkAddTag)

	r.Get("/family", familyHandler.TrackerPage)
	r.Post("/family/add", familyHandler.QuickAdd)
	r.Post("/family/{id}/complete", familyHandler.Complete)
	r.Post("/family/{id}/uncomplete", familyHandler.Uncomplete)
	r.Post("/family/{id}/delete", familyHandler.Delete)
	r.Post("/family/{id}/move", familyHandler.MoveToList)
	r.Post("/family/{id}/restore", familyHandler.Restore)
	r.Post("/family/{id}/purge", familyHandler.Purge)

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

	rr := postForm(env.router, "/todos/"+existingTask+"/complete", nil)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get(existingTask)
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
	postForm(env.router, "/todos/"+existingTask+"/complete", nil)
	// Then uncomplete it.
	rr := postForm(env.router, "/todos/"+existingTask+"/uncomplete", nil)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get(existingTask)
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

	rr := postForm(env.router, "/todos/"+existingTask+"/notes", url.Values{
		"body": {"Some notes here"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get(existingTask)
	if err != nil {
		t.Fatalf("getting item: %v", err)
	}
	if item.Body != "Some notes here" {
		t.Errorf("expected body 'Some notes here', got %q", item.Body)
	}
}

func TestUpdateEdit(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/"+existingTask+"/edit", url.Values{
		"title":  {"Existing task"},
		"body":   {"Updated body"},
		"tags":   {"work, urgent"},
		"images": {"img1.jpg"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get(existingTask)
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

	rr := postForm(env.router, "/todos/"+existingTask+"/edit", url.Values{
		"title": {"Renamed task"},
		"body":  {""},
		"tags":  {""},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// A rename keeps the ID.
	item, err := env.personalSvc.Get(existingTask)
	if err != nil {
		t.Fatalf("getting renamed item: %v", err)
	}
	if item.Title != "Renamed task" {
		t.Errorf("expected title 'Renamed task', got %q", item.Title)
	}
}

func TestDeleteTask(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/"+existingTask+"/delete", nil)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Delete is now soft-delete: item still exists via Get but is excluded from List.
	item, err := env.personalSvc.Get(existingTask)
	if err != nil {
		t.Fatalf("expected soft-deleted item to still be accessible via Get: %v", err)
	}
	if item.DeletedAt == "" {
		t.Error("expected DeletedAt to be set after soft delete")
	}

	items, _ := env.personalSvc.List()
	for _, it := range items {
		if it.ID == existingTask {
			t.Error("soft-deleted item should not appear in List()")
		}
	}
}

func TestUpdatePriority(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/"+existingTask+"/priority", url.Values{
		"priority": {"high"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get(existingTask)
	if err != nil {
		t.Fatalf("getting item: %v", err)
	}
	if item.Priority != "high" {
		t.Errorf("expected priority 'high', got %q", item.Priority)
	}
}

func TestUpdateTags(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/"+existingTask+"/tags", url.Values{
		"tags": {"finance, health"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get(existingTask)
	if err != nil {
		t.Fatalf("getting item: %v", err)
	}
	if len(item.Tags) != 2 || item.Tags[0] != "finance" || item.Tags[1] != "health" {
		t.Errorf("expected tags [finance, health], got %v", item.Tags)
	}
}

func TestMoveToList(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/"+existingTask+"/move", nil)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/todos?msg=item-moved" {
		t.Errorf("expected redirect to /todos?msg=item-moved, got %q", loc)
	}

	// Item should no longer be in personal list.
	_, err := env.personalSvc.Get(existingTask)
	if err == nil {
		t.Error("expected item to be removed from personal list")
	}

	// Item should now be in family list.
	item, err := env.familySvc.Get(existingTask)
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

	goal := idOf(t, env.personalSvc, "Run distance")
	rr := postForm(env.router, "/todos/"+goal+"/progress", url.Values{
		"delta": {"10"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get(goal)
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

	goal := idOf(t, env.personalSvc, "Save money")
	rr := postForm(env.router, "/todos/"+goal+"/progress", url.Values{
		"delta": {"500"},
		"set":   {"1"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get(goal)
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
	postForm(env.router, "/todos/"+existingTask+"/delete", nil)

	// Then restore.
	rr := postForm(env.router, "/todos/"+existingTask+"/restore", nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, err := env.personalSvc.Get(existingTask)
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
	postForm(env.router, "/todos/"+existingTask+"/delete", nil)

	// Then permanently delete.
	rr := postForm(env.router, "/todos/"+existingTask+"/purge", nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	_, err := env.personalSvc.Get(existingTask)
	if err == nil {
		t.Error("expected item to be permanently deleted")
	}
}

func TestBulkCompleteHandler(t *testing.T) {
	env := setupTrackerEnv(t)
	second := addSecondTask(t, env)

	rr := postForm(env.router, "/todos/bulk/complete", url.Values{
		"ids": {existingTask + ", " + second},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "msg=bulk-completed") {
		t.Errorf("expected bulk-completed flash, got %q", loc)
	}

	a, _ := env.personalSvc.Get(existingTask)
	b, _ := env.personalSvc.Get(second)
	if !a.Done || !b.Done {
		t.Error("expected both tasks to be completed")
	}
}

func TestBulkDeleteHandler(t *testing.T) {
	env := setupTrackerEnv(t)
	second := addSecondTask(t, env)

	rr := postForm(env.router, "/todos/bulk/delete", url.Values{
		"ids": {existingTask + ", " + second},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	items, _ := env.personalSvc.List()
	if len(items) != 0 {
		t.Errorf("expected 0 active items, got %d", len(items))
	}
}

func TestBulkPriorityHandler(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/bulk/priority", url.Values{
		"ids":      {existingTask},
		"priority": {"high"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, _ := env.personalSvc.Get(existingTask)
	if item.Priority != "high" {
		t.Errorf("expected priority 'high', got %q", item.Priority)
	}
}

func TestBulkAddTagHandler(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/bulk/tag", url.Values{
		"ids": {existingTask},
		"tag": {"urgent"},
	})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	item, _ := env.personalSvc.Get(existingTask)
	if len(item.Tags) != 1 || item.Tags[0] != "urgent" {
		t.Errorf("expected tags [urgent], got %v", item.Tags)
	}
}

func TestBulkNoSlugsReturns400(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/bulk/complete", url.Values{
		"ids": {""},
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestBulkAddTagMissingTag(t *testing.T) {
	env := setupTrackerEnv(t)

	rr := postForm(env.router, "/todos/bulk/tag", url.Values{
		"ids": {existingTask},
		"tag": {""},
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// addSecondTask adds "Second task" through the router and returns its ID.
func addSecondTask(t *testing.T, env *trackerTestEnv) string {
	t.Helper()
	postForm(env.router, "/todos/add", url.Values{"title": {"Second task"}})
	items, err := env.personalSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Title == "Second task" {
			return it.ID
		}
	}
	t.Fatal("Second task was not added")
	return ""
}
