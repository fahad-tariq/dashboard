package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/commentary"
	dbpkg "github.com/fahad/dashboard/internal/db"
)

type commentaryAPIEnv struct {
	store  *commentary.Store
	router *chi.Mux
}

func setupCommentaryAPIEnv(t *testing.T) *commentaryAPIEnv {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := dbpkg.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	store := commentary.NewStore(database)
	r := chi.NewRouter()
	r.Put("/api/v1/commentary/{list}/{slug}", commentary.APISetCommentary(store))
	r.Get("/api/v1/commentary/{list}/{slug}", commentary.APIGetCommentary(store))
	r.Delete("/api/v1/commentary/{list}/{slug}", commentary.APIDeleteCommentary(store))

	return &commentaryAPIEnv{store: store, router: r}
}

func commentaryRequest(t *testing.T, env *commentaryAPIEnv, method, path, body string) *httptest.ResponseRecorder {
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

func TestCommentaryAPI_SetAndGet(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	// Set commentary
	w := commentaryRequest(t, env, "PUT", "/api/v1/commentary/personal/task-1",
		`{"content":"This task has been open for a week."}`)
	if w.Code != 200 {
		t.Fatalf("PUT status = %d; body: %s", w.Code, w.Body.String())
	}

	// Get commentary
	w = commentaryRequest(t, env, "GET", "/api/v1/commentary/personal/task-1", "")
	if w.Code != 200 {
		t.Fatalf("GET status = %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["content"] != "This task has been open for a week." {
		t.Errorf("content = %v", resp["content"])
	}
}

func TestCommentaryAPI_GetEmpty(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	w := commentaryRequest(t, env, "GET", "/api/v1/commentary/personal/nonexistent", "")
	if w.Code != 200 {
		t.Fatalf("GET status = %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["content"] != "" {
		t.Errorf("content should be empty, got %v", resp["content"])
	}
}

func TestCommentaryAPI_Delete(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	commentaryRequest(t, env, "PUT", "/api/v1/commentary/personal/task-1",
		`{"content":"some content"}`)

	w := commentaryRequest(t, env, "DELETE", "/api/v1/commentary/personal/task-1", "")
	if w.Code != 200 {
		t.Fatalf("DELETE status = %d; body: %s", w.Code, w.Body.String())
	}

	// Verify it's gone
	w = commentaryRequest(t, env, "GET", "/api/v1/commentary/personal/task-1", "")
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["content"] != "" {
		t.Errorf("content should be empty after delete, got %v", resp["content"])
	}
}

func TestCommentaryAPI_InvalidList(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	w := commentaryRequest(t, env, "PUT", "/api/v1/commentary/invalid/task-1",
		`{"content":"test"}`)
	if w.Code != 400 {
		t.Errorf("PUT with invalid list status = %d, want 400", w.Code)
	}
}

func TestCommentaryAPI_EmptyContent(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	w := commentaryRequest(t, env, "PUT", "/api/v1/commentary/personal/task-1",
		`{"content":""}`)
	if w.Code != 400 {
		t.Errorf("PUT with empty content status = %d, want 400", w.Code)
	}
}

func TestCommentaryAPI_ContentTooLong(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	longContent := strings.Repeat("x", 5001)
	w := commentaryRequest(t, env, "PUT", "/api/v1/commentary/personal/task-1",
		`{"content":"`+longContent+`"}`)
	if w.Code != 400 {
		t.Errorf("PUT with too-long content status = %d, want 400", w.Code)
	}
}

func TestCommentaryAPI_IdeasList(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	w := commentaryRequest(t, env, "PUT", "/api/v1/commentary/ideas/idea-1",
		`{"content":"Idea commentary"}`)
	if w.Code != 200 {
		t.Fatalf("PUT ideas status = %d; body: %s", w.Code, w.Body.String())
	}

	w = commentaryRequest(t, env, "GET", "/api/v1/commentary/ideas/idea-1", "")
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["content"] != "Idea commentary" {
		t.Errorf("content = %v", resp["content"])
	}
}

func TestCommentaryAPI_TodosNormalisesToPersonal(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	commentaryRequest(t, env, "PUT", "/api/v1/commentary/todos/task-1",
		`{"content":"via todos"}`)

	// Should be retrievable as "personal"
	w := commentaryRequest(t, env, "GET", "/api/v1/commentary/personal/task-1", "")
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["content"] != "via todos" {
		t.Errorf("content = %v, want 'via todos'", resp["content"])
	}
}
