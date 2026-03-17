package test

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/home"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/tracker"
)

func setupHomeEnv(t *testing.T) (http.HandlerFunc, *tracker.Service, *tracker.Service, *ideas.Service) {
	t.Helper()
	dir := t.TempDir()
	personalPath := filepath.Join(dir, "personal.md")
	familyPath := filepath.Join(dir, "family.md")
	ideasPath := filepath.Join(dir, "ideas.md")
	os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] Buy milk !high\n- [ ] Read book\n"), 0o644)
	os.WriteFile(familyPath, []byte("# Family\n\n- [ ] Plan holiday\n"), 0o644)
	os.WriteFile(ideasPath, []byte("# Ideas\n\n"), 0o644)

	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	personalStore := tracker.NewStore(database, "personal")
	familyStore := tracker.NewStore(database, "family")
	personalSvc := tracker.NewService(personalPath, "Personal", personalStore)
	familySvc := tracker.NewService(familyPath, "Family", familyStore)
	ideasSvc := ideas.NewService(ideasPath)

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
	tmpl, _ := template.Must(layout.Clone()).Parse(
		`{{define "content"}}homepage|Title={{.Title}}|PersonalTaskCount={{.PersonalTaskCount}}|FamilyTaskCount={{.FamilyTaskCount}}|TotalIdeaCount={{.TotalIdeaCount}}{{end}}`,
	)
	templates["homepage.html"] = tmpl

	handler := home.HomePageSingle(personalSvc, familySvc, ideasSvc, templates)
	return handler, personalSvc, familySvc, ideasSvc
}

func TestHomePageRenders(t *testing.T) {
	handler, _, _, _ := setupHomeEnv(t)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "homepage") {
		t.Errorf("expected body to contain 'homepage', got: %s", body)
	}
}

func TestHomePageShowsTaskCounts(t *testing.T) {
	handler, _, _, _ := setupHomeEnv(t)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "PersonalTaskCount=2") {
		t.Errorf("expected PersonalTaskCount=2, got: %s", body)
	}
	if !strings.Contains(body, "FamilyTaskCount=1") {
		t.Errorf("expected FamilyTaskCount=1, got: %s", body)
	}
}

func TestHomePageShowsIdeaCounts(t *testing.T) {
	dir := t.TempDir()
	personalPath := filepath.Join(dir, "personal.md")
	familyPath := filepath.Join(dir, "family.md")
	ideasPath := filepath.Join(dir, "ideas.md")
	os.WriteFile(personalPath, []byte("# Personal\n\n"), 0o644)
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)
	os.WriteFile(ideasPath, []byte("# Ideas\n\n- [ ] First idea\n- [ ] Second idea\n"), 0o644)

	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	personalStore := tracker.NewStore(database, "personal")
	familyStore := tracker.NewStore(database, "family")
	personalSvc := tracker.NewService(personalPath, "Personal", personalStore)
	familySvc := tracker.NewService(familyPath, "Family", familyStore)
	ideasSvc := ideas.NewService(ideasPath)

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
	tmpl, _ := template.Must(layout.Clone()).Parse(
		`{{define "content"}}homepage|Title={{.Title}}|PersonalTaskCount={{.PersonalTaskCount}}|FamilyTaskCount={{.FamilyTaskCount}}|TotalIdeaCount={{.TotalIdeaCount}}{{end}}`,
	)
	templates["homepage.html"] = tmpl

	handler := home.HomePageSingle(personalSvc, familySvc, ideasSvc, templates)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "TotalIdeaCount=2") {
		t.Errorf("expected TotalIdeaCount=2, got: %s", body)
	}
}

func TestHomePageEmpty(t *testing.T) {
	dir := t.TempDir()
	personalPath := filepath.Join(dir, "personal.md")
	familyPath := filepath.Join(dir, "family.md")
	ideasPath := filepath.Join(dir, "ideas.md")
	os.WriteFile(personalPath, []byte("# Personal\n\n"), 0o644)
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)
	os.WriteFile(ideasPath, []byte("# Ideas\n\n"), 0o644)

	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	personalStore := tracker.NewStore(database, "personal")
	familyStore := tracker.NewStore(database, "family")
	personalSvc := tracker.NewService(personalPath, "Personal", personalStore)
	familySvc := tracker.NewService(familyPath, "Family", familyStore)
	ideasSvc := ideas.NewService(ideasPath)

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
	tmpl, _ := template.Must(layout.Clone()).Parse(
		`{{define "content"}}homepage|Title={{.Title}}|PersonalTaskCount={{.PersonalTaskCount}}|FamilyTaskCount={{.FamilyTaskCount}}|TotalIdeaCount={{.TotalIdeaCount}}{{end}}`,
	)
	templates["homepage.html"] = tmpl

	handler := home.HomePageSingle(personalSvc, familySvc, ideasSvc, templates)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "PersonalTaskCount=0") {
		t.Errorf("expected PersonalTaskCount=0, got: %s", body)
	}
	if !strings.Contains(body, "FamilyTaskCount=0") {
		t.Errorf("expected FamilyTaskCount=0, got: %s", body)
	}
	if !strings.Contains(body, "TotalIdeaCount=0") {
		t.Errorf("expected TotalIdeaCount=0, got: %s", body)
	}
}
