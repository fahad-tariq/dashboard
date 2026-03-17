package test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/db"
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

	svc := ideas.NewService(ideasPath)

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
	personalSvc := tracker.NewService(personalPath, "Personal", tracker.NewStore(personalDB, "personal"))

	// Set up family tracker.
	familyPath := filepath.Join(dir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n\n- [ ] Plan holiday\n"), 0o644)
	familyDB, _ := db.Open(filepath.Join(dir, "family.db"))
	t.Cleanup(func() { familyDB.Close() })
	familySvc := tracker.NewService(familyPath, "Family", tracker.NewStore(familyDB, "family"))

	// Set up ideas.
	ideasPath := filepath.Join(dir, "ideas.md")
	os.WriteFile(ideasPath, []byte("# Ideas\n\n- [ ] Build a rocket [status: untriaged]\n"), 0o644)
	ideaSvc := ideas.NewService(ideasPath)

	handler := search.NewHandler(func(r *http.Request) (*tracker.Service, *tracker.Service, *ideas.Service) {
		return personalSvc, familySvc, ideaSvc
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
