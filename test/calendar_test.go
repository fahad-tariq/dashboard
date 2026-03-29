package test

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/home"
	"github.com/fahad/dashboard/internal/tracker"
)

func TestBuildCalendarDays_Grouping(t *testing.T) {
	personal := []tracker.Item{
		{Slug: "a", Title: "Task A", Planned: "2026-03-16"},
		{Slug: "b", Title: "Task B", Planned: "2026-03-18"},
	}
	family := []tracker.Item{
		{Slug: "c", Title: "Task C", Planned: "2026-03-16"},
	}

	start := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC) // Monday
	end := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)   // Sunday
	days := home.BuildCalendarDays(personal, family, nil, start, end, "2026-03-19")

	if len(days) != 7 {
		t.Fatalf("expected 7 days, got %d", len(days))
	}

	// Monday (16th): 1 personal + 1 family = 2 total.
	mon := days[0]
	if mon.Date != "2026-03-16" {
		t.Errorf("day 0 date: got %q", mon.Date)
	}
	if mon.TaskCount != 2 {
		t.Errorf("day 0 task count: got %d, want 2", mon.TaskCount)
	}
	if len(mon.Personal) != 1 {
		t.Errorf("day 0 personal: got %d, want 1", len(mon.Personal))
	}
	if len(mon.Family) != 1 {
		t.Errorf("day 0 family: got %d, want 1", len(mon.Family))
	}

	// Wednesday (18th): 1 personal.
	wed := days[2]
	if wed.TaskCount != 1 {
		t.Errorf("day 2 task count: got %d, want 1", wed.TaskCount)
	}

	// Thursday (19th) is today.
	thu := days[3]
	if !thu.IsToday {
		t.Error("expected Thursday to be today")
	}
	if thu.TaskCount != 0 {
		t.Errorf("day 3 task count: got %d, want 0", thu.TaskCount)
	}
}

func TestBuildCalendarDays_MonthBoundary(t *testing.T) {
	personal := []tracker.Item{
		{Slug: "a", Title: "Last day of March", Planned: "2026-03-31"},
		{Slug: "b", Title: "First day of April", Planned: "2026-04-01"},
	}

	// Week spanning March/April boundary: Mon 30 Mar - Sun 5 Apr.
	start := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	days := home.BuildCalendarDays(personal, nil, nil, start, end, "2026-04-01")

	if len(days) != 7 {
		t.Fatalf("expected 7 days, got %d", len(days))
	}

	// Tuesday (31st).
	if days[1].TaskCount != 1 {
		t.Errorf("March 31 task count: got %d, want 1", days[1].TaskCount)
	}
	// Wednesday (1st).
	if days[2].TaskCount != 1 {
		t.Errorf("April 1 task count: got %d, want 1", days[2].TaskCount)
	}
	if !days[2].IsToday {
		t.Error("expected April 1 to be today")
	}
}

func TestBuildCalendarDays_Empty(t *testing.T) {
	start := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)
	days := home.BuildCalendarDays(nil, nil, nil, start, end, "2026-03-19")

	if len(days) != 7 {
		t.Fatalf("expected 7 days, got %d", len(days))
	}
	for _, d := range days {
		if d.TaskCount != 0 {
			t.Errorf("expected 0 tasks on %s, got %d", d.Date, d.TaskCount)
		}
	}
}

func setupCalendarEnv(t *testing.T) (http.HandlerFunc, *tracker.Service, *tracker.Service) {
	t.Helper()
	dir := t.TempDir()

	database, err := db.Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	personalSvc := tracker.NewService(dir+"/personal.md", "Personal", tracker.NewStore(database, "personal"), time.UTC)
	familySvc := tracker.NewService(dir+"/family.md", "Family", tracker.NewStore(database, "family"), time.UTC)
	houseProjectsSvc := tracker.NewService(dir+"/house-projects.md", "House", tracker.NewStore(database, "house"), time.UTC)

	funcMap := template.FuncMap{
		"authEnabled":  func() bool { return false },
		"buildVersion": func() string { return "test" },
		"subtract":     func(a, b int) int { return a - b },
		"formatNum":    func(f float64) string { return fmt.Sprintf("%g", f) },
		"planPercent":  func(done, total int) int { return 0 },
	}
	layout := template.Must(template.New("layout.html").Funcs(funcMap).Parse(
		`{{define "layout.html"}}{{template "content" .}}{{end}}`,
	))
	templates := make(map[string]*template.Template)
	tmpl, _ := template.Must(layout.Clone()).Parse(
		`{{define "content"}}calendar|Title={{.Title}}|View={{.View}}|Header={{.Header}}|Days={{len .Days}}{{end}}`,
	)
	templates["calendar.html"] = tmpl

	handler := home.CalendarPageSingle(personalSvc, familySvc, houseProjectsSvc, nil, templates, time.UTC)
	return handler, personalSvc, familySvc
}

func TestCalendarHandler_WeekView(t *testing.T) {
	handler, _, _ := setupCalendarEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/plan/calendar?view=week", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "View=week") {
		t.Errorf("expected week view, got: %s", body)
	}
	if !strings.Contains(body, "Days=7") {
		t.Errorf("expected 7 days, got: %s", body)
	}
}

func TestCalendarHandler_MonthView(t *testing.T) {
	handler, _, _ := setupCalendarEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/plan/calendar?view=month&date=2026-03-01", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "View=month") {
		t.Errorf("expected month view, got: %s", body)
	}
	if !strings.Contains(body, "March 2026") {
		t.Errorf("expected March 2026, got: %s", body)
	}
	if !strings.Contains(body, "Days=31") {
		t.Errorf("expected 31 days for March, got: %s", body)
	}
}
