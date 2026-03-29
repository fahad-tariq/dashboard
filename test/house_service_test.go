package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/house"
)

func setupMaintenanceSvc(t *testing.T) (*house.Service, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "maintenance.md")
	os.WriteFile(path, []byte("# Maintenance\n\n"), 0o644)
	svc := house.NewService(path, time.UTC)
	return svc, path
}

func TestMaintenanceServiceAddAndList(t *testing.T) {
	svc, _ := setupMaintenanceSvc(t)

	err := svc.Add(&house.MaintenanceItem{
		Title:   "Clean gutters",
		Cadence: "3m",
		Tags:    []string{"exterior"},
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	items, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Clean gutters" {
		t.Errorf("title: got %q", items[0].Title)
	}
	if items[0].Cadence != "3m" {
		t.Errorf("cadence: got %q", items[0].Cadence)
	}
	if items[0].Added == "" {
		t.Error("added should be auto-set")
	}
}

func TestMaintenanceServiceLogCompletion(t *testing.T) {
	svc, _ := setupMaintenanceSvc(t)

	svc.Add(&house.MaintenanceItem{Title: "Mow lawn", Cadence: "2w"})

	err := svc.LogCompletion("mow-lawn", "used new mower")
	if err != nil {
		t.Fatalf("log: %v", err)
	}

	item, err := svc.Get("mow-lawn")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(item.Log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(item.Log))
	}
	if item.Log[0].Note != "used new mower" {
		t.Errorf("note: got %q", item.Log[0].Note)
	}

	// Log again -- should prepend.
	err = svc.LogCompletion("mow-lawn", "")
	if err != nil {
		t.Fatalf("log2: %v", err)
	}
	item, _ = svc.Get("mow-lawn")
	if len(item.Log) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(item.Log))
	}
	// First entry should be the newest.
	if item.Log[0].Date > item.Log[1].Date {
		// Both are today, so dates are equal -- that's fine.
	}
}

func TestMaintenanceServiceLogCompletionStripsNewlines(t *testing.T) {
	svc, _ := setupMaintenanceSvc(t)
	svc.Add(&house.MaintenanceItem{Title: "Test item", Cadence: "1m"})

	err := svc.LogCompletion("test-item", "line1\n- [ ] injected\r\nline2")
	if err != nil {
		t.Fatalf("log: %v", err)
	}

	item, _ := svc.Get("test-item")
	if len(item.Log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(item.Log))
	}
	note := item.Log[0].Note
	if note != "line1 - [ ] injected  line2" {
		t.Errorf("note should have newlines stripped: got %q", note)
	}
}

func TestMaintenanceServiceListOverdue(t *testing.T) {
	svc, path := setupMaintenanceSvc(t)

	// Write items with known log dates.
	content := `# Maintenance

- [ ] Clean gutters [cadence: 3m] [added: 2025-01-01]
  - [x] 2025-11-01

- [ ] Mow lawn [cadence: 2w] [added: 2025-01-01]
  - [x] 2026-03-20

- [ ] Check smoke alarms [cadence: 6m] [added: 2025-01-01]
`
	os.WriteFile(path, []byte(content), 0o644)
	svc.Resync()

	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	overdue := svc.ListOverdue(now)

	// Gutters: last done Nov 1, 3m cadence -> due Feb 1 -> overdue
	// Lawn: last done Mar 20, 2w cadence -> due Apr 3 -> NOT overdue
	// Smoke alarms: never done -> overdue
	if len(overdue) != 2 {
		t.Fatalf("expected 2 overdue, got %d", len(overdue))
	}

	slugs := map[string]bool{}
	for _, it := range overdue {
		slugs[it.Slug] = true
	}
	if !slugs["clean-gutters"] || !slugs["check-smoke-alarms"] {
		t.Errorf("unexpected overdue items: %v", slugs)
	}
}

func TestMaintenanceServiceDeleteRestore(t *testing.T) {
	svc, _ := setupMaintenanceSvc(t)
	svc.Add(&house.MaintenanceItem{Title: "Test item", Cadence: "1m"})

	// Delete.
	if err := svc.Delete("test-item"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	items, _ := svc.List()
	if len(items) != 0 {
		t.Errorf("expected 0 active items, got %d", len(items))
	}

	// Restore.
	if err := svc.Restore("test-item"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	items, _ = svc.List()
	if len(items) != 1 {
		t.Errorf("expected 1 active item, got %d", len(items))
	}
}

func TestMaintenanceServiceSearch(t *testing.T) {
	svc, _ := setupMaintenanceSvc(t)
	svc.Add(&house.MaintenanceItem{Title: "Clean gutters", Cadence: "3m"})
	svc.Add(&house.MaintenanceItem{Title: "Mow lawn", Cadence: "2w"})

	results := svc.Search("gutter")
	if len(results) != 1 || results[0].Title != "Clean gutters" {
		t.Errorf("search: got %v", results)
	}

	results = svc.Search("xyz")
	if len(results) != 0 {
		t.Errorf("expected no results, got %d", len(results))
	}
}

func TestMaintenanceServiceResync(t *testing.T) {
	svc, path := setupMaintenanceSvc(t)
	svc.Add(&house.MaintenanceItem{Title: "Item one", Cadence: "1m"})

	// Externally modify the file.
	os.WriteFile(path, []byte("# Maintenance\n\n- [ ] External item [cadence: 1w]\n"), 0o644)

	if err := svc.Resync(); err != nil {
		t.Fatalf("resync: %v", err)
	}

	items, _ := svc.List()
	if len(items) != 1 || items[0].Title != "External item" {
		t.Errorf("resync: got %v", items)
	}
}
