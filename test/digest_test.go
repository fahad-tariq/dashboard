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
	"github.com/fahad/dashboard/internal/insights"
	"github.com/fahad/dashboard/internal/tracker"
)

// --- Insights: Digest function tests ---

func TestDigest_PeriodBoundaries(t *testing.T) {
	// Wednesday 2026-03-18, week starts Monday 2026-03-16.
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)

	items := []insights.DigestItem{
		{Added: "2026-03-18", Completed: "2026-03-18", Done: true, Type: "task"},  // this week
		{Added: "2026-03-16", Completed: "2026-03-16", Done: true, Type: "task"},  // this week (Monday)
		{Added: "2026-03-15", Completed: "2026-03-15", Done: true, Type: "task"},  // last week (Sunday)
		{Added: "2026-03-10", Completed: "2026-03-10", Done: true, Type: "task"},  // last week (Tuesday)
		{Added: "2026-03-01", Completed: "2026-03-01", Done: true, Type: "task"},  // this month but earlier
		{Added: "2026-02-28", Completed: "2026-02-28", Done: true, Type: "task"},  // last month
		{Added: "2026-03-17", Type: "idea"},                                       // idea this week
	}

	tests := []struct {
		name            string
		period          insights.DigestPeriod
		wantCompleted   int
		wantAddedTasks  int
		wantAddedIdeas  int
	}{
		{"this-week", insights.PeriodThisWeek, 2, 2, 1},
		{"last-week", insights.PeriodLastWeek, 2, 2, 0},
		{"this-month", insights.PeriodThisMonth, 5, 5, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := insights.Digest(items, tt.period, now)
			if result.CompletedTasks != tt.wantCompleted {
				t.Errorf("completed: got %d, want %d", result.CompletedTasks, tt.wantCompleted)
			}
			if result.AddedTasks != tt.wantAddedTasks {
				t.Errorf("added tasks: got %d, want %d", result.AddedTasks, tt.wantAddedTasks)
			}
			if result.AddedIdeas != tt.wantAddedIdeas {
				t.Errorf("added ideas: got %d, want %d", result.AddedIdeas, tt.wantAddedIdeas)
			}
		})
	}
}

func TestDigest_EmptyData(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	result := insights.Digest(nil, insights.PeriodThisWeek, now)

	if result.CompletedTasks != 0 || result.AddedTasks != 0 || result.AddedIdeas != 0 {
		t.Errorf("expected all zeros, got completed=%d added=%d ideas=%d",
			result.CompletedTasks, result.AddedTasks, result.AddedIdeas)
	}
	if result.MaxValue != 0 {
		t.Errorf("max value: got %d, want 0", result.MaxValue)
	}
	if len(result.TagCounts) != 0 {
		t.Errorf("tag counts: got %d, want 0", len(result.TagCounts))
	}
}

func TestDigest_TagAggregation(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	items := []insights.DigestItem{
		{Added: "2026-03-16", Completed: "2026-03-16", Done: true, Type: "task", Tags: []string{"health", "fitness"}},
		{Added: "2026-03-17", Completed: "2026-03-17", Done: true, Type: "task", Tags: []string{"health"}},
		{Added: "2026-03-18", Completed: "2026-03-18", Done: true, Type: "task", Tags: []string{"tech"}},
	}

	result := insights.Digest(items, insights.PeriodThisWeek, now)
	if len(result.TagCounts) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(result.TagCounts))
	}

	// Tags should be sorted by count descending, then name.
	byTag := map[string]int{}
	for _, tc := range result.TagCounts {
		byTag[tc.Tag] = tc.Count
	}
	if byTag["health"] != 2 {
		t.Errorf("health: got %d, want 2", byTag["health"])
	}
	if byTag["fitness"] != 1 {
		t.Errorf("fitness: got %d, want 1", byTag["fitness"])
	}
	if byTag["tech"] != 1 {
		t.Errorf("tech: got %d, want 1", byTag["tech"])
	}
}

func TestDigest_SundayEdgeCase(t *testing.T) {
	// Sunday 2026-03-22 -- should be part of the week starting Monday 2026-03-16.
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	items := []insights.DigestItem{
		{Added: "2026-03-22", Completed: "2026-03-22", Done: true, Type: "task"},
		{Added: "2026-03-16", Completed: "2026-03-16", Done: true, Type: "task"},
	}

	result := insights.Digest(items, insights.PeriodThisWeek, now)
	if result.CompletedTasks != 2 {
		t.Errorf("completed: got %d, want 2 (Sunday should be in same week as Monday)", result.CompletedTasks)
	}
}

func TestDigest_MaxValueAndPercentages(t *testing.T) {
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	items := []insights.DigestItem{
		{Added: "2026-03-16", Completed: "2026-03-16", Done: true, Type: "task"},
		{Added: "2026-03-17", Completed: "2026-03-17", Done: true, Type: "task"},
		{Added: "2026-03-18", Completed: "2026-03-18", Done: true, Type: "task"},
		{Added: "2026-03-17", Type: "idea"},
	}

	result := insights.Digest(items, insights.PeriodThisWeek, now)
	if result.MaxValue != 3 {
		t.Errorf("max value: got %d, want 3", result.MaxValue)
	}
	if result.CompletedPct != 100 {
		t.Errorf("completed pct: got %d, want 100", result.CompletedPct)
	}
	// AddedTasks also 3, so 100%.
	if result.AddedPct != 100 {
		t.Errorf("added pct: got %d, want 100", result.AddedPct)
	}
	// 1 idea / 3 max = 33%.
	if result.IdeasPct != 33 {
		t.Errorf("ideas pct: got %d, want 33", result.IdeasPct)
	}
}

func TestParseDigestPeriod(t *testing.T) {
	tests := []struct {
		input string
		want  insights.DigestPeriod
	}{
		{"this-week", insights.PeriodThisWeek},
		{"last-week", insights.PeriodLastWeek},
		{"this-month", insights.PeriodThisMonth},
		{"invalid", insights.PeriodThisWeek},
		{"", insights.PeriodThisWeek},
	}
	for _, tt := range tests {
		got := insights.ParseDigestPeriod(tt.input)
		if got != tt.want {
			t.Errorf("ParseDigestPeriod(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- Digest handler tests ---

func setupDigestEnv(t *testing.T) (http.HandlerFunc, *tracker.Service, *tracker.Service, *ideas.Service) {
	t.Helper()
	dir := t.TempDir()
	personalPath := filepath.Join(dir, "personal.md")
	familyPath := filepath.Join(dir, "family.md")
	ideasPath := filepath.Join(dir, "ideas.md")
	os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] Buy milk\n  [added: 2026-03-17]\n- [x] Done task\n  [added: 2026-03-16]\n  [completed: 2026-03-17]\n"), 0o644)
	os.WriteFile(familyPath, []byte("# Family\n\n- [ ] Plan holiday\n"), 0o644)
	os.WriteFile(ideasPath, []byte("# Ideas\n\n- [ ] Cool idea\n  [added: 2026-03-17]\n"), 0o644)

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
		`{{define "content"}}digest|Title={{.Title}}|Period={{.Period}}|Completed={{.Digest.CompletedTasks}}|Added={{.Digest.AddedTasks}}|Ideas={{.Digest.AddedIdeas}}{{end}}`,
	)
	templates["digest.html"] = tmpl

	handler := home.DigestPageSingle(personalSvc, familySvc, ideasSvc, templates)
	return handler, personalSvc, familySvc, ideasSvc
}

func TestDigestPageRenders(t *testing.T) {
	handler, _, _, _ := setupDigestEnv(t)

	req := httptest.NewRequest("GET", "/digest", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "digest") {
		t.Errorf("expected body to contain 'digest', got: %s", body)
	}
	if !strings.Contains(body, "Title=Digest") {
		t.Errorf("expected Title=Digest, got: %s", body)
	}
}

func TestDigestPagePeriodSwitching(t *testing.T) {
	handler, _, _, _ := setupDigestEnv(t)

	tests := []struct {
		query      string
		wantPeriod string
	}{
		{"?period=this-week", "this-week"},
		{"?period=last-week", "last-week"},
		{"?period=this-month", "this-month"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/digest"+tt.query, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("period %s: expected 200, got %d", tt.wantPeriod, rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "Period="+tt.wantPeriod) {
			t.Errorf("expected Period=%s, got: %s", tt.wantPeriod, body)
		}
	}
}

func TestDigestPageInvalidPeriodFallback(t *testing.T) {
	handler, _, _, _ := setupDigestEnv(t)

	req := httptest.NewRequest("GET", "/digest?period=garbage", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Period=this-week") {
		t.Errorf("expected fallback to this-week, got: %s", body)
	}
}

func TestDigestPageEmptyState(t *testing.T) {
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
		`{{define "content"}}digest|Completed={{.Digest.CompletedTasks}}|Added={{.Digest.AddedTasks}}|Ideas={{.Digest.AddedIdeas}}{{end}}`,
	)
	templates["digest.html"] = tmpl

	handler := home.DigestPageSingle(personalSvc, familySvc, ideasSvc, templates)

	req := httptest.NewRequest("GET", "/digest", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Completed=0") {
		t.Errorf("expected Completed=0, got: %s", body)
	}
}
