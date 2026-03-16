package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fahad/projects/internal/projects"
)

func TestParseBacklog(t *testing.T) {
	dir := t.TempDir()

	backlog := `# Backlog

## Active

### Migrate to JWT auth
- priority: high
- added: 2026-03-10
- plan: plans/jwt-auth.md

Replace session-based auth with JWT tokens.

### Add dark mode
- priority: low
- added: 2026-03-08

## Done

### Set up CI pipeline
- added: 2026-02-15
- done: 2026-03-01
`
	os.WriteFile(filepath.Join(dir, "backlog.md"), []byte(backlog), 0o644)

	items, err := projects.ParseBacklog(dir)
	if err != nil {
		t.Fatalf("parse backlog: %v", err)
	}

	tests := []struct {
		idx      int
		title    string
		section  string
		priority string
		plan     string
		done     string
		body     string
	}{
		{0, "Migrate to JWT auth", "Active", "high", "plans/jwt-auth.md", "", "Replace session-based auth with JWT tokens."},
		{1, "Add dark mode", "Active", "low", "", "", ""},
		{2, "Set up CI pipeline", "Done", "", "", "2026-03-01", ""},
	}

	if len(items) != len(tests) {
		t.Fatalf("expected %d items, got %d", len(tests), len(items))
	}

	for _, tt := range tests {
		item := items[tt.idx]
		if item.Title != tt.title {
			t.Errorf("[%d] title: got %q, want %q", tt.idx, item.Title, tt.title)
		}
		if item.Section != tt.section {
			t.Errorf("[%d] section: got %q, want %q", tt.idx, item.Section, tt.section)
		}
		if item.Priority != tt.priority {
			t.Errorf("[%d] priority: got %q, want %q", tt.idx, item.Priority, tt.priority)
		}
		if item.Plan != tt.plan {
			t.Errorf("[%d] plan: got %q, want %q", tt.idx, item.Plan, tt.plan)
		}
		if item.Done != tt.done {
			t.Errorf("[%d] done: got %q, want %q", tt.idx, item.Done, tt.done)
		}
		if item.Body != tt.body {
			t.Errorf("[%d] body: got %q, want %q", tt.idx, item.Body, tt.body)
		}
	}
}

func TestParseBacklogMissing(t *testing.T) {
	items, err := projects.ParseBacklog(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil for missing backlog, got %v", items)
	}
}

func TestListPlans(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")
	os.MkdirAll(plansDir, 0o755)
	os.WriteFile(filepath.Join(plansDir, "auth.md"), []byte("# Auth"), 0o644)
	os.WriteFile(filepath.Join(plansDir, "api.md"), []byte("# API"), 0o644)
	os.WriteFile(filepath.Join(plansDir, "notes.txt"), []byte("not a plan"), 0o644)

	plans := projects.ListPlans(dir)
	if len(plans) != 2 {
		t.Fatalf("expected 2 plans, got %d: %v", len(plans), plans)
	}
}
