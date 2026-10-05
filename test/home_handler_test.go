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
	"time"

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
	if err := os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] Buy milk !high\n- [ ] Read book\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(familyPath, []byte("# Family\n\n- [ ] Plan holiday\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ideasPath, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	personalSvc := tracker.NewService(personalPath, "Personal", time.UTC)
	familySvc := tracker.NewService(familyPath, "Family", time.UTC)
	houseProjectsSvc := tracker.NewService(filepath.Join(dir, "house-projects.md"), "House", time.UTC)
	ideasSvc := ideas.NewService(ideasPath, time.UTC)

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
		`{{define "content"}}homepage|Title={{.Title}}|Unplanned={{len .UnplannedPersonal}}/{{len .UnplannedFamily}}|Empty={{.Empty}}{{end}}`,
	)
	templates["homepage.html"] = tmpl

	handler := http.HandlerFunc(home.NewHandler(func(*http.Request) home.Lists {
		return home.Lists{Personal: personalSvc, Family: familySvc, HouseProjects: houseProjectsSvc, Ideas: ideasSvc}
	}, templates, time.UTC).HomePage)
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

// The task picker lists open tasks from each list; the summary cards are
// module widgets (see TestHomepageWidgetsComeFromModules).
func TestHomePageListsOpenTasks(t *testing.T) {
	handler, _, _, _ := setupHomeEnv(t)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Unplanned=2/1|Empty=false") {
		t.Errorf("expected two personal and one family task to pick, got: %s", body)
	}
}

func TestHomePageEmpty(t *testing.T) {
	dir := t.TempDir()
	personalPath := filepath.Join(dir, "personal.md")
	familyPath := filepath.Join(dir, "family.md")
	ideasPath := filepath.Join(dir, "ideas.md")
	if err := os.WriteFile(personalPath, []byte("# Personal\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ideasPath, []byte("# Ideas\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	personalSvc := tracker.NewService(personalPath, "Personal", time.UTC)
	familySvc := tracker.NewService(familyPath, "Family", time.UTC)
	houseProjectsSvc := tracker.NewService(filepath.Join(dir, "house-projects.md"), "House", time.UTC)
	ideasSvc := ideas.NewService(ideasPath, time.UTC)

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
		`{{define "content"}}homepage|Title={{.Title}}|Unplanned={{len .UnplannedPersonal}}/{{len .UnplannedFamily}}|Empty={{.Empty}}{{end}}`,
	)
	templates["homepage.html"] = tmpl

	handler := http.HandlerFunc(home.NewHandler(func(*http.Request) home.Lists {
		return home.Lists{Personal: personalSvc, Family: familySvc, HouseProjects: houseProjectsSvc, Ideas: ideasSvc}
	}, templates, time.UTC).HomePage)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Unplanned=0/0|Empty=true") {
		t.Errorf("expected an empty homepage, got: %s", body)
	}
}

func TestGreetingTimePeriods(t *testing.T) {
	tests := []struct {
		hour       int
		wantPrefix string
	}{
		{0, "evening"},
		{4, "evening"},
		{5, "morning"},
		{8, "morning"},
		{11, "morning"},
		{12, "afternoon"},
		{15, "afternoon"},
		{17, "afternoon"},
		{18, "evening"},
		{23, "evening"},
	}
	for _, tt := range tests {
		now := time.Date(2026, 3, 17, tt.hour, 0, 0, 0, time.UTC)
		got := home.Greeting(now, "", 0, false)
		got = strings.ToLower(got)
		if !strings.Contains(got, tt.wantPrefix) {
			t.Errorf("Greeting at hour %d: got %q, want it to contain %q", tt.hour, got, tt.wantPrefix)
		}
	}
}

func TestGreetingWithName(t *testing.T) {
	now := time.Date(2026, 3, 17, 9, 0, 0, 0, time.UTC)
	got := home.Greeting(now, "Fahad", 0, false)
	if !strings.Contains(got, "Fahad") {
		t.Errorf("expected greeting to contain name, got %q", got)
	}
}

func TestGreetingEmptyName(t *testing.T) {
	now := time.Date(2026, 3, 17, 9, 0, 0, 0, time.UTC)
	got := home.Greeting(now, "", 0, false)
	if strings.Contains(got, ",") {
		t.Errorf("expected no comma for empty name, got %q", got)
	}
}

func TestGreetingStreakMilestone(t *testing.T) {
	// Morning + 30-day streak should mention the streak.
	now := time.Date(2026, 3, 17, 8, 0, 0, 0, time.UTC)
	got := home.Greeting(now, "", 30, false)
	if !strings.Contains(got, "30-day streak") {
		t.Errorf("expected streak mention, got %q", got)
	}

	// Non-milestone streak (e.g. 5 days) should NOT mention streak.
	got = home.Greeting(now, "", 5, false)
	if strings.Contains(got, "streak") {
		t.Errorf("expected no streak for 5 days, got %q", got)
	}
}

func TestGreetingPlanAllDone(t *testing.T) {
	// Afternoon + plan done should mention "All clear".
	now := time.Date(2026, 3, 17, 14, 0, 0, 0, time.UTC)
	got := home.Greeting(now, "", 0, true)
	if !strings.Contains(got, "All clear") {
		t.Errorf("expected 'All clear' suffix, got %q", got)
	}

	// Morning + plan done should NOT mention "All clear".
	now = time.Date(2026, 3, 17, 8, 0, 0, 0, time.UTC)
	got = home.Greeting(now, "", 0, true)
	if strings.Contains(got, "All clear") {
		t.Errorf("expected no 'All clear' in morning, got %q", got)
	}
}

func TestGreetingRotatesByDay(t *testing.T) {
	// Two different days should produce at least one different greeting.
	day1 := time.Date(2026, 3, 17, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
	g1 := home.Greeting(day1, "", 0, false)
	g2 := home.Greeting(day2, "", 0, false)
	if g1 == g2 {
		// With a pool of 2, consecutive days should differ.
		t.Errorf("expected different greetings on consecutive days, both got %q", g1)
	}
}

func TestPlanPromptFridayFewTasks(t *testing.T) {
	// Friday with 2 open tasks.
	fri := time.Date(2026, 3, 20, 9, 0, 0, 0, time.UTC) // Friday
	got := home.PlanPrompt(fri, 2, 0)
	if !strings.Contains(got, "Nearly there") {
		t.Errorf("expected Friday few-tasks prompt, got %q", got)
	}
}

func TestPlanPromptStreakMilestone(t *testing.T) {
	mon := time.Date(2026, 3, 16, 9, 0, 0, 0, time.UTC) // Monday
	got := home.PlanPrompt(mon, 10, 30)
	if !strings.Contains(got, "Day 30") {
		t.Errorf("expected streak milestone prompt, got %q", got)
	}
}

func TestPlanPromptAllCaughtUp(t *testing.T) {
	tue := time.Date(2026, 3, 17, 9, 0, 0, 0, time.UTC) // Tuesday
	got := home.PlanPrompt(tue, 0, 0)
	if !strings.Contains(got, "All caught up") {
		t.Errorf("expected all-caught-up prompt, got %q", got)
	}
}

func TestPlanPromptDefaultWeekday(t *testing.T) {
	wed := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC) // Wednesday
	got := home.PlanPrompt(wed, 10, 0)
	if got != "Three things?" {
		t.Errorf("expected Wednesday default prompt, got %q", got)
	}
}
