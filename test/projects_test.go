package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/projects"
)

func TestScanFindsProjectsWithReadme(t *testing.T) {
	dir := t.TempDir()

	// Create two project dirs: one with README.md, one without.
	withReadme := filepath.Join(dir, "proj-a")
	os.MkdirAll(withReadme, 0o755)
	os.WriteFile(filepath.Join(withReadme, "README.md"), []byte("# Project A"), 0o644)

	withoutReadme := filepath.Join(dir, "proj-b")
	os.MkdirAll(withoutReadme, 0o755)

	// Hidden dirs should be skipped.
	hidden := filepath.Join(dir, ".hidden")
	os.MkdirAll(hidden, 0o755)
	os.WriteFile(filepath.Join(hidden, "README.md"), []byte("# Hidden"), 0o644)

	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	svc := projects.NewService(database, dir)
	if err := svc.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 project, got %d", len(list))
	}
	if list[0].Slug != "proj-a" {
		t.Errorf("expected slug 'proj-a', got %q", list[0].Slug)
	}
}

func TestScanUpsertsOnRescan(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "myproj")
	os.MkdirAll(projDir, 0o755)
	os.WriteFile(filepath.Join(projDir, "README.md"), []byte("# My Project"), 0o644)

	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	svc := projects.NewService(database, dir)

	// Scan twice -- should not error or duplicate.
	for range 2 {
		if err := svc.Scan(); err != nil {
			t.Fatalf("scan: %v", err)
		}
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 project after rescan, got %d", len(list))
	}
}

func TestCountBacklogItems(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "backlog-proj")
	os.MkdirAll(projDir, 0o755)
	os.WriteFile(filepath.Join(projDir, "README.md"), []byte("# Project"), 0o644)

	backlog := `# Backlog

## Active

### Task one
- priority: high

### Task two
- priority: low

## Done

### Task three
- done: 2026-01-01
`
	os.WriteFile(filepath.Join(projDir, "backlog.md"), []byte(backlog), 0o644)

	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	svc := projects.NewService(database, dir)
	if err := svc.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 project, got %d", len(list))
	}
	// All ### headings count (active + done).
	if list[0].BacklogLen != 3 {
		t.Errorf("expected 3 backlog items, got %d", list[0].BacklogLen)
	}
}
