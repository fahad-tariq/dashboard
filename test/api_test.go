package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/fahad/dashboard/internal/ideas"
)

func withChiURLParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func newTestHandler(t *testing.T) (*ideas.Handler, *ideas.Service) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n\n"), 0o644)
	svc := ideas.NewService(path)
	h := ideas.NewHandler(svc, nil, nil)
	return h, svc
}

func TestAPIAddIdea_Valid(t *testing.T) {
	h, svc := newTestHandler(t)

	body := `{"title":"Test Idea","tags":["go","api"],"body":"Some details."}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ideas", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.APIAddIdea(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp ideas.Idea
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Title != "Test Idea" {
		t.Errorf("response title: got %q", resp.Title)
	}
	if resp.Status != "untriaged" {
		t.Errorf("response status: got %q, want untriaged", resp.Status)
	}

	list, _ := svc.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 idea in store, got %d", len(list))
	}
	if list[0].Title != "Test Idea" {
		t.Errorf("stored title: got %q", list[0].Title)
	}
}

func TestAPIAddIdea_EmptyTitle(t *testing.T) {
	h, _ := newTestHandler(t)

	body := `{"title":"","body":"No title provided."}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ideas", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.APIAddIdea(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "title required" {
		t.Errorf("error message: got %q", resp["error"])
	}
}

func TestAPIAddIdea_InvalidJSON(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ideas", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.APIAddIdea(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAPITriageIdea_Valid(t *testing.T) {
	h, svc := newTestHandler(t)
	svc.Add(&ideas.Idea{Slug: "triage-me", Title: "Triage Me", Body: "Content."})

	body := `{"action":"park"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/ideas/triage-me/triage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	req = withChiURLParam(req, "slug", "triage-me")

	rec := httptest.NewRecorder()
	h.APITriageIdea(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d\nbody: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	idea, _ := svc.Get("triage-me")
	if idea.Status != "parked" {
		t.Errorf("status: got %q, want parked", idea.Status)
	}
}

func TestAPIAddResearch_Valid(t *testing.T) {
	h, svc := newTestHandler(t)
	svc.Add(&ideas.Idea{Slug: "research-target", Title: "Research Target", Body: "Initial body."})

	body := `{"content":"New research findings."}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ideas/research-target/research", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	req = withChiURLParam(req, "slug", "research-target")

	rec := httptest.NewRecorder()
	h.APIAddResearch(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d\nbody: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	idea, _ := svc.Get("research-target")
	if !strings.Contains(idea.Body, "## Research") {
		t.Errorf("body should contain ## Research heading, got %q", idea.Body)
	}
	if !strings.Contains(idea.Body, "New research findings.") {
		t.Errorf("body should contain research content, got %q", idea.Body)
	}
	if !strings.Contains(idea.Body, "Initial body.") {
		t.Errorf("body should preserve initial content, got %q", idea.Body)
	}
}
