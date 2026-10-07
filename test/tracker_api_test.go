package test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/tracker"
)

type apiTestEnv struct {
	personalSvc *tracker.Service
	familySvc   *tracker.Service
	router      *chi.Mux
}

func setupAPIEnv(t *testing.T) *apiTestEnv {
	t.Helper()

	dir := t.TempDir()
	personalPath := filepath.Join(dir, "personal.md")
	familyPath := filepath.Join(dir, "family.md")
	if err := os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] Existing task [tags: backend]\n- [ ] Private item [tags: private]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(familyPath, []byte("# Family\n\n- [ ] Family task\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	personalSvc := tracker.NewService(personalPath, "Personal", time.UTC)
	familySvc := tracker.NewService(familyPath, "Family", time.UTC)
	fixed := func(*http.Request) (*tracker.Service, *tracker.Service) { return personalSvc, familySvc }

	r := chi.NewRouter()
	r.Get("/api/v1/todos", tracker.APIListTodos(fixed))
	r.Post("/api/v1/todos", tracker.APIAddTodo(fixed))
	r.Get("/api/v1/todos/{slug}", tracker.APIGetTodo(fixed))
	r.Put("/api/v1/todos/{slug}", tracker.APIUpdateTodo(fixed))
	r.Post("/api/v1/todos/{slug}/complete", tracker.APICompleteTodo(fixed))
	r.Post("/api/v1/todos/{slug}/uncomplete", tracker.APIUncompleteTodo(fixed))
	r.Delete("/api/v1/todos/{slug}", tracker.APIDeleteTodo(fixed))
	r.Put("/api/v1/todos/{slug}/priority", tracker.APIUpdatePriority(fixed))
	r.Put("/api/v1/todos/{slug}/tags", tracker.APIUpdateTags(fixed))
	r.Post("/api/v1/todos/{slug}/substeps", tracker.APIAddSubStep(fixed))
	r.Put("/api/v1/todos/{slug}/substeps/{index}", tracker.APIToggleSubStep(fixed))
	r.Delete("/api/v1/todos/{slug}/substeps/{index}", tracker.APIRemoveSubStep(fixed))

	return &apiTestEnv{personalSvc: personalSvc, familySvc: familySvc, router: r}
}

func apiRequest(t *testing.T, env *apiTestEnv, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	return w
}

func TestAPIListTodos(t *testing.T) {
	env := setupAPIEnv(t)

	w := apiRequest(t, env, "GET", "/api/v1/todos", "")
	if w.Code != 200 {
		t.Fatalf("GET /todos status = %d, want 200", w.Code)
	}

	var resp map[string][]map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	// "Existing task" should be present, "Private item" should be filtered
	personal := resp["personal"]
	for _, item := range personal {
		if item["title"] == "Private item" {
			t.Error("private-tagged item should be filtered from API response")
		}
	}
	found := false
	for _, item := range personal {
		if item["title"] == "Existing task" {
			found = true
			if item["list"] != "personal" {
				t.Errorf("item list = %v, want personal", item["list"])
			}
			if item["type"] != "task" {
				t.Errorf("item type = %v, want task", item["type"])
			}
		}
	}
	if !found {
		t.Error("Existing task not found in response")
	}

	family := resp["family"]
	if len(family) != 1 {
		t.Errorf("family items = %d, want 1", len(family))
	}
}

func TestAPIAddTodo(t *testing.T) {
	env := setupAPIEnv(t)

	w := apiRequest(t, env, "POST", "/api/v1/todos",
		`{"title":"New API task","tags":["test"],"priority":"high","list":"personal"}`)
	if w.Code != 201 {
		t.Fatalf("POST /todos status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["title"] != "New API task" {
		t.Errorf("title = %v, want New API task", resp["title"])
	}
	if resp["priority"] != "high" {
		t.Errorf("priority = %v, want high", resp["priority"])
	}
}

func TestAPIAddTodoStripsMetadata(t *testing.T) {
	env := setupAPIEnv(t)

	w := apiRequest(t, env, "POST", "/api/v1/todos",
		`{"title":"Task [planned: 2026-01-01] [deleted: 2026-01-01]","list":"personal"}`)
	if w.Code != 201 {
		t.Fatalf("POST /todos status = %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	title := resp["title"].(string)
	if strings.Contains(title, "[planned:") || strings.Contains(title, "[deleted:") {
		t.Errorf("title should have metadata stripped, got %q", title)
	}
}

func TestAPIAddTodoMissingList(t *testing.T) {
	env := setupAPIEnv(t)

	w := apiRequest(t, env, "POST", "/api/v1/todos",
		`{"title":"No list task"}`)
	if w.Code != 400 {
		t.Errorf("POST /todos without list status = %d, want 400", w.Code)
	}
}

func TestAPIGetTodo(t *testing.T) {
	env := setupAPIEnv(t)

	w := apiRequest(t, env, "GET", "/api/v1/todos/existing-task?list=personal", "")
	if w.Code != 200 {
		t.Fatalf("GET /todos/existing-task status = %d, want 200", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["title"] != "Existing task" {
		t.Errorf("title = %v, want Existing task", resp["title"])
	}
}

func TestAPIGetTodoNotFound(t *testing.T) {
	env := setupAPIEnv(t)

	w := apiRequest(t, env, "GET", "/api/v1/todos/nonexistent?list=personal", "")
	if w.Code != 404 {
		t.Errorf("GET /todos/nonexistent status = %d, want 404", w.Code)
	}
}

func TestAPICompleteTodo(t *testing.T) {
	env := setupAPIEnv(t)

	w := apiRequest(t, env, "POST", "/api/v1/todos/existing-task/complete",
		`{"list":"personal"}`)
	if w.Code != 200 {
		t.Fatalf("POST complete status = %d; body: %s", w.Code, w.Body.String())
	}

	item, _ := env.personalSvc.Get("existing-task")
	if !item.Done {
		t.Error("item should be done after complete")
	}
}

func TestAPIDeleteTodo(t *testing.T) {
	env := setupAPIEnv(t)

	w := apiRequest(t, env, "DELETE", "/api/v1/todos/existing-task",
		`{"list":"personal"}`)
	if w.Code != 200 {
		t.Fatalf("DELETE status = %d; body: %s", w.Code, w.Body.String())
	}

	item, _ := env.personalSvc.Get("existing-task")
	if item.DeletedAt == "" {
		t.Error("item should be soft-deleted")
	}
}

func TestAPIUpdatePriority(t *testing.T) {
	env := setupAPIEnv(t)

	w := apiRequest(t, env, "PUT", "/api/v1/todos/existing-task/priority",
		`{"priority":"high","list":"personal"}`)
	if w.Code != 200 {
		t.Fatalf("PUT priority status = %d; body: %s", w.Code, w.Body.String())
	}

	item, _ := env.personalSvc.Get("existing-task")
	if item.Priority != "high" {
		t.Errorf("priority = %q, want high", item.Priority)
	}
}

func TestAPISubSteps(t *testing.T) {
	env := setupAPIEnv(t)

	// Add sub-step
	w := apiRequest(t, env, "POST", "/api/v1/todos/existing-task/substeps",
		`{"text":"First step","list":"personal"}`)
	if w.Code != 200 {
		t.Fatalf("POST substeps status = %d; body: %s", w.Code, w.Body.String())
	}

	item, _ := env.personalSvc.Get("existing-task")
	if item.SubStepsTotal != 1 {
		t.Fatalf("SubStepsTotal = %d, want 1", item.SubStepsTotal)
	}

	// Toggle sub-step
	w = apiRequest(t, env, "PUT", "/api/v1/todos/existing-task/substeps/0",
		`{"list":"personal"}`)
	if w.Code != 200 {
		t.Fatalf("PUT substeps/0 status = %d; body: %s", w.Code, w.Body.String())
	}

	item, _ = env.personalSvc.Get("existing-task")
	if item.SubStepsDone != 1 {
		t.Errorf("SubStepsDone = %d, want 1", item.SubStepsDone)
	}

	// Remove sub-step
	w = apiRequest(t, env, "DELETE", "/api/v1/todos/existing-task/substeps/0",
		`{"list":"personal"}`)
	if w.Code != 200 {
		t.Fatalf("DELETE substeps/0 status = %d; body: %s", w.Code, w.Body.String())
	}

	item, _ = env.personalSvc.Get("existing-task")
	if item.SubStepsTotal != 0 {
		t.Errorf("SubStepsTotal = %d after remove, want 0", item.SubStepsTotal)
	}
}

func TestAPISubStepBoundsCheck(t *testing.T) {
	env := setupAPIEnv(t)

	// Try to toggle index 99 on item with no sub-steps
	w := apiRequest(t, env, "PUT", "/api/v1/todos/existing-task/substeps/99",
		`{"list":"personal"}`)
	if w.Code != 400 {
		t.Errorf("PUT substeps/99 status = %d, want 400", w.Code)
	}
}

func TestAPIInvalidListReturns400(t *testing.T) {
	env := setupAPIEnv(t)

	w := apiRequest(t, env, "POST", "/api/v1/todos/existing-task/complete",
		`{"list":"invalid"}`)
	if w.Code != 400 {
		t.Errorf("POST with invalid list status = %d, want 400", w.Code)
	}
}

// The tracker API serves personal and family only. "house" passes the shared
// list validation, so it must still be refused with a 400, never reach a nil
// service.
func TestAPIRejectsHouseList(t *testing.T) {
	env := setupAPIEnv(t)
	handler := middleware.Recoverer(env.router)

	tests := map[string]struct {
		method, path, body string
	}{
		"get":         {"GET", "/api/v1/todos/existing-task?list=house", ""},
		"add":         {"POST", "/api/v1/todos", `{"title":"x","list":"house"}`},
		"update":      {"PUT", "/api/v1/todos/existing-task", `{"title":"x","list":"house"}`},
		"complete":    {"POST", "/api/v1/todos/existing-task/complete", `{"list":"house"}`},
		"uncomplete":  {"POST", "/api/v1/todos/existing-task/uncomplete", `{"list":"house"}`},
		"delete":      {"DELETE", "/api/v1/todos/existing-task", `{"list":"house"}`},
		"priority":    {"PUT", "/api/v1/todos/existing-task/priority", `{"priority":"high","list":"house"}`},
		"tags":        {"PUT", "/api/v1/todos/existing-task/tags", `{"tags":["a"],"list":"house"}`},
		"add substep": {"POST", "/api/v1/todos/existing-task/substeps", `{"text":"x","list":"house"}`},
		"toggle step": {"PUT", "/api/v1/todos/existing-task/substeps/0", `{"list":"house"}`},
		"remove step": {"DELETE", "/api/v1/todos/existing-task/substeps/0", `{"list":"house"}`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body: %s", w.Code, w.Body.String())
			}
		})
	}
}

// PUT keeps whatever the request leaves out, so a client can send only the
// fields it means to change.
func TestAPIUpdateTodoKeepsOmittedFields(t *testing.T) {
	tests := map[string]struct {
		body                 string
		wantTitle, wantBody  string
		wantTags, wantImages []string
	}{
		"title only": {
			body:      `{"title":"Renamed task","list":"personal"}`,
			wantTitle: "Renamed task", wantBody: "Original notes",
			wantTags: []string{"backend"}, wantImages: []string{"a.png"},
		},
		"body only": {
			body:      `{"body":"New notes","list":"personal"}`,
			wantTitle: "Existing task", wantBody: "New notes",
			wantTags: []string{"backend"}, wantImages: []string{"a.png"},
		},
		"null keeps": {
			body:      `{"body":null,"tags":null,"images":null,"list":"personal"}`,
			wantTitle: "Existing task", wantBody: "Original notes",
			wantTags: []string{"backend"}, wantImages: []string{"a.png"},
		},
		"body metadata is stripped": {
			body:      `{"body":"Call them [status: done] back","list":"personal"}`,
			wantTitle: "Existing task", wantBody: "Call them  back",
			wantTags: []string{"backend"}, wantImages: []string{"a.png"},
		},
		"explicit empties clear": {
			body:      `{"body":"","tags":[],"images":[],"list":"personal"}`,
			wantTitle: "Existing task", wantBody: "",
			wantTags: nil, wantImages: nil,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env := setupAPIEnv(t)
			if err := env.personalSvc.UpdateEdit("existing-task", "", "Original notes", []string{"backend"}, []string{"a.png"}); err != nil {
				t.Fatal(err)
			}
			w := apiRequest(t, env, "PUT", "/api/v1/todos/existing-task", tc.body)
			if w.Code != 200 {
				t.Fatalf("PUT status = %d; body: %s", w.Code, w.Body.String())
			}
			slug := tracker.Slugify(tc.wantTitle)
			item, err := env.personalSvc.Get(slug)
			if err != nil {
				t.Fatalf("get %s: %v", slug, err)
			}
			if item.Title != tc.wantTitle || item.Body != tc.wantBody {
				t.Errorf("title, body = %q, %q; want %q, %q", item.Title, item.Body, tc.wantTitle, tc.wantBody)
			}
			if !slices.Equal(item.Tags, tc.wantTags) || !slices.Equal(item.Images, tc.wantImages) {
				t.Errorf("tags, images = %v, %v; want %v, %v", item.Tags, item.Images, tc.wantTags, tc.wantImages)
			}
		})
	}
}

func TestAPIUpdateTodoEnforcesLimits(t *testing.T) {
	env := setupAPIEnv(t)
	long := strings.Repeat("x", 100001)
	w := apiRequest(t, env, "PUT", "/api/v1/todos/existing-task", `{"body":"`+long+`","list":"personal"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("oversized body: status = %d, want 400", w.Code)
	}
}
