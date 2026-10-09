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
	"github.com/fahad/dashboard/internal/itemid"
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
	r.Put("/api/v1/commentary/{list}/"+itemid.Route, commentary.APISetCommentary(store))
	r.Get("/api/v1/commentary/{list}/"+itemid.Route, commentary.APIGetCommentary(store))
	r.Delete("/api/v1/commentary/{list}/"+itemid.Route, commentary.APIDeleteCommentary(store))

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
	w := commentaryRequest(t, env, "PUT", "/api/v1/commentary/personal/tsk00001",
		`{"content":"This task has been open for a week."}`)
	if w.Code != 200 {
		t.Fatalf("PUT status = %d; body: %s", w.Code, w.Body.String())
	}

	// Get commentary
	w = commentaryRequest(t, env, "GET", "/api/v1/commentary/personal/tsk00001", "")
	if w.Code != 200 {
		t.Fatalf("GET status = %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["content"] != "This task has been open for a week." || resp["id"] != "tsk00001" || resp["list"] != "personal" {
		t.Errorf("response = %v", resp)
	}
}

func TestCommentaryAPI_GetEmpty(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	w := commentaryRequest(t, env, "GET", "/api/v1/commentary/personal/n0n3x1st", "")
	if w.Code != 200 {
		t.Fatalf("GET status = %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["content"] != "" {
		t.Errorf("content should be empty, got %v", resp["content"])
	}
}

func TestCommentaryAPI_Delete(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	commentaryRequest(t, env, "PUT", "/api/v1/commentary/personal/tsk00001",
		`{"content":"some content"}`)

	w := commentaryRequest(t, env, "DELETE", "/api/v1/commentary/personal/tsk00001", "")
	if w.Code != 200 {
		t.Fatalf("DELETE status = %d; body: %s", w.Code, w.Body.String())
	}

	// Verify it's gone
	w = commentaryRequest(t, env, "GET", "/api/v1/commentary/personal/tsk00001", "")
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["content"] != "" {
		t.Errorf("content should be empty after delete, got %v", resp["content"])
	}
}

func TestCommentaryAPI_InvalidList(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	w := commentaryRequest(t, env, "PUT", "/api/v1/commentary/invalid/tsk00001",
		`{"content":"test"}`)
	if w.Code != 400 {
		t.Errorf("PUT with invalid list status = %d, want 400", w.Code)
	}
}

func TestCommentaryAPI_EmptyContent(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	w := commentaryRequest(t, env, "PUT", "/api/v1/commentary/personal/tsk00001",
		`{"content":""}`)
	if w.Code != 400 {
		t.Errorf("PUT with empty content status = %d, want 400", w.Code)
	}
}

func TestCommentaryAPI_ContentTooLong(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	longContent := strings.Repeat("x", 5001)
	w := commentaryRequest(t, env, "PUT", "/api/v1/commentary/personal/tsk00001",
		`{"content":"`+longContent+`"}`)
	if w.Code != 400 {
		t.Errorf("PUT with too-long content status = %d, want 400", w.Code)
	}
}

func TestCommentaryAPI_IdeasList(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	w := commentaryRequest(t, env, "PUT", "/api/v1/commentary/ideas/d3000001",
		`{"content":"Idea commentary"}`)
	if w.Code != 200 {
		t.Fatalf("PUT ideas status = %d; body: %s", w.Code, w.Body.String())
	}

	w = commentaryRequest(t, env, "GET", "/api/v1/commentary/ideas/d3000001", "")
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["content"] != "Idea commentary" {
		t.Errorf("content = %v", resp["content"])
	}
}

func TestCommentaryAPI_TodosNormalisesToPersonal(t *testing.T) {
	env := setupCommentaryAPIEnv(t)

	commentaryRequest(t, env, "PUT", "/api/v1/commentary/todos/tsk00001",
		`{"content":"via todos"}`)

	// Should be retrievable as "personal"
	w := commentaryRequest(t, env, "GET", "/api/v1/commentary/personal/tsk00001", "")
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["content"] != "via todos" {
		t.Errorf("content = %v, want 'via todos'", resp["content"])
	}
}

// A path segment that is not an item ID never reaches the handlers.
func TestCommentaryAPI_NonIDPathNotFound(t *testing.T) {
	env := setupCommentaryAPIEnv(t)
	for name, path := range map[string]string{
		"slug":      "/api/v1/commentary/personal/fix-the-sink",
		"too short": "/api/v1/commentary/personal/tsk0001",
		"vowel":     "/api/v1/commentary/personal/tsk0000a",
	} {
		t.Run(name, func(t *testing.T) {
			if w := commentaryRequest(t, env, "GET", path, ""); w.Code != http.StatusNotFound {
				t.Errorf("GET %s status = %d, want 404", path, w.Code)
			}
		})
	}
}
