package test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/search"
	"github.com/fahad/dashboard/internal/tracker"
)

func TestTrackerServiceSearch(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Buy groceries !high\n  Need milk and eggs\n- [ ] Fix the roof\n- [ ] Learn Go\n  Read the Go book\n")

	tests := []struct {
		name    string
		query   string
		wantLen int
	}{
		{"title match", "groceries", 1},
		{"body match", "milk", 1},
		{"case insensitive", "ROOF", 1},
		{"multiple matches", "the", 2},
		{"no match", "zzzzz", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := svc.Search(tt.query)
			if len(results) != tt.wantLen {
				t.Errorf("Search(%q): got %d results, want %d", tt.query, len(results), tt.wantLen)
			}
		})
	}
}

func TestIdeasServiceSearch(t *testing.T) {
	dir := t.TempDir()
	ideasPath := filepath.Join(dir, "ideas.md")
	content := "# Ideas\n\n- [ ] Build a rocket [status: untriaged]\n  Research propulsion systems\n- [ ] Write a novel [status: parked]\n"
	os.WriteFile(ideasPath, []byte(content), 0o644)

	svc := ideas.NewService(ideasPath, time.UTC)

	tests := []struct {
		name    string
		query   string
		wantLen int
	}{
		{"title match", "rocket", 1},
		{"body match", "propulsion", 1},
		{"case insensitive", "NOVEL", 1},
		{"no match", "zzzzz", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := svc.Search(tt.query)
			if len(results) != tt.wantLen {
				t.Errorf("Search(%q): got %d results, want %d", tt.query, len(results), tt.wantLen)
			}
		})
	}
}

func TestSearchHandler(t *testing.T) {
	dir := t.TempDir()

	// Set up personal tracker.
	personalPath := filepath.Join(dir, "personal.md")
	os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] Buy groceries\n  Need milk\n"), 0o644)
	personalDB, _ := db.Open(filepath.Join(dir, "personal.db"))
	t.Cleanup(func() { personalDB.Close() })
	personalSvc := tracker.NewService(personalPath, "Personal", tracker.NewStore(personalDB, "personal"), time.UTC)

	// Set up family tracker.
	familyPath := filepath.Join(dir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n\n- [ ] Plan holiday\n"), 0o644)
	familyDB, _ := db.Open(filepath.Join(dir, "family.db"))
	t.Cleanup(func() { familyDB.Close() })
	familySvc := tracker.NewService(familyPath, "Family", tracker.NewStore(familyDB, "family"), time.UTC)

	// Set up ideas.
	ideasPath := filepath.Join(dir, "ideas.md")
	os.WriteFile(ideasPath, []byte("# Ideas\n\n- [ ] Build a rocket [status: untriaged]\n"), 0o644)
	ideaSvc := ideas.NewService(ideasPath, time.UTC)

	handler := search.NewHandler(func(r *http.Request) (*tracker.Service, *tracker.Service, *tracker.Service, *house.Service, *ideas.Service) {
		return personalSvc, familySvc, nil, nil, ideaSvc
	})

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantInBody string
	}{
		{"empty query returns empty", "", http.StatusOK, ""},
		{"matches personal", "groceries", http.StatusOK, "Buy groceries"},
		{"matches family", "holiday", http.StatusOK, "Plan holiday"},
		{"matches ideas", "rocket", http.StatusOK, "Build a rocket"},
		{"no results", "zzzzz", http.StatusOK, "No results"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/search?q="+tt.query, nil)
			rec := httptest.NewRecorder()
			handler.SearchAPI(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantInBody != "" && !strings.Contains(rec.Body.String(), tt.wantInBody) {
				t.Errorf("body should contain %q, got: %s", tt.wantInBody, rec.Body.String())
			}
		})
	}
}

func TestSearchQueryTooLong(t *testing.T) {
	dir := t.TempDir()
	personalPath := filepath.Join(dir, "personal.md")
	os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] Buy groceries\n"), 0o644)
	personalDB, _ := db.Open(filepath.Join(dir, "p.db"))
	t.Cleanup(func() { personalDB.Close() })
	personalSvc := tracker.NewService(personalPath, "Personal", tracker.NewStore(personalDB, "personal"), time.UTC)

	familyPath := filepath.Join(dir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)
	familyDB, _ := db.Open(filepath.Join(dir, "f.db"))
	t.Cleanup(func() { familyDB.Close() })
	familySvc := tracker.NewService(familyPath, "Family", tracker.NewStore(familyDB, "family"), time.UTC)

	ideasPath := filepath.Join(dir, "ideas.md")
	os.WriteFile(ideasPath, []byte("# Ideas\n\n"), 0o644)
	ideaSvc := ideas.NewService(ideasPath, time.UTC)

	handler := search.NewHandler(func(r *http.Request) (*tracker.Service, *tracker.Service, *tracker.Service, *house.Service, *ideas.Service) {
		return personalSvc, familySvc, nil, nil, ideaSvc
	})

	longQuery := strings.Repeat("a", 201)
	req := httptest.NewRequest("GET", "/search?q="+longQuery, nil)
	rec := httptest.NewRecorder()
	handler.SearchAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.Len() > 0 {
		t.Errorf("expected empty body for overlong query, got %d bytes", rec.Body.Len())
	}
}

func TestSearchExcludesDeletedItems(t *testing.T) {
	dir := t.TempDir()

	// Personal tracker with a soft-deleted item.
	personalPath := filepath.Join(dir, "personal.md")
	os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] Active task\n- [ ] Trashed task [deleted: 2026-03-01]\n"), 0o644)
	personalDB, _ := db.Open(filepath.Join(dir, "personal.db"))
	t.Cleanup(func() { personalDB.Close() })
	personalSvc := tracker.NewService(personalPath, "Personal", tracker.NewStore(personalDB, "personal"), time.UTC)

	// Family tracker empty.
	familyPath := filepath.Join(dir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)
	familyDB, _ := db.Open(filepath.Join(dir, "family.db"))
	t.Cleanup(func() { familyDB.Close() })
	familySvc := tracker.NewService(familyPath, "Family", tracker.NewStore(familyDB, "family"), time.UTC)

	// Ideas with a soft-deleted idea.
	ideasPath := filepath.Join(dir, "ideas.md")
	os.WriteFile(ideasPath, []byte("# Ideas\n\n- [ ] Active idea [status: untriaged]\n- [ ] Trashed idea [status: untriaged] [deleted: 2026-03-01]\n"), 0o644)
	ideaSvc := ideas.NewService(ideasPath, time.UTC)

	handler := search.NewHandler(func(r *http.Request) (*tracker.Service, *tracker.Service, *tracker.Service, *house.Service, *ideas.Service) {
		return personalSvc, familySvc, nil, nil, ideaSvc
	})

	// Search for "trashed" should return no results (both are soft-deleted).
	req := httptest.NewRequest("GET", "/search?q=trashed", nil)
	rec := httptest.NewRecorder()
	handler.SearchAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Trashed task") {
		t.Error("soft-deleted tracker item should not appear in search results")
	}
	if strings.Contains(body, "Trashed idea") {
		t.Error("soft-deleted idea should not appear in search results")
	}

	// Search for "active" should find both active items.
	req = httptest.NewRequest("GET", "/search?q=active", nil)
	rec = httptest.NewRecorder()
	handler.SearchAPI(rec, req)

	body = rec.Body.String()
	if !strings.Contains(body, "Active task") {
		t.Error("active task should appear in search results")
	}
	if !strings.Contains(body, "Active idea") {
		t.Error("active idea should appear in search results")
	}
}

func TestSearchSnippetInResults(t *testing.T) {
	dir := t.TempDir()
	personalPath := filepath.Join(dir, "personal.md")
	os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] Research project\n  The quick brown fox jumps over the lazy dog near the riverbank\n"), 0o644)
	personalDB, _ := db.Open(filepath.Join(dir, "p.db"))
	t.Cleanup(func() { personalDB.Close() })
	personalSvc := tracker.NewService(personalPath, "Personal", tracker.NewStore(personalDB, "personal"), time.UTC)

	familyPath := filepath.Join(dir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)
	familyDB, _ := db.Open(filepath.Join(dir, "f.db"))
	t.Cleanup(func() { familyDB.Close() })
	familySvc := tracker.NewService(familyPath, "Family", tracker.NewStore(familyDB, "family"), time.UTC)

	ideasPath := filepath.Join(dir, "ideas.md")
	os.WriteFile(ideasPath, []byte("# Ideas\n\n"), 0o644)
	ideaSvc := ideas.NewService(ideasPath, time.UTC)

	handler := search.NewHandler(func(r *http.Request) (*tracker.Service, *tracker.Service, *tracker.Service, *house.Service, *ideas.Service) {
		return personalSvc, familySvc, nil, nil, ideaSvc
	})

	req := httptest.NewRequest("GET", "/search?q=fox", nil)
	rec := httptest.NewRecorder()
	handler.SearchAPI(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "fox") {
		t.Errorf("expected snippet to contain 'fox', got: %s", body)
	}
	if !strings.Contains(body, "Research project") {
		t.Errorf("expected result to contain title 'Research project', got: %s", body)
	}
}
