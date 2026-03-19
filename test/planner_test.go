package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/tracker"
)

func TestPlannedMetadataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tracker.md")
	content := "# Test\n\n- [ ] Review docs [added: 2026-03-01] [planned: 2026-03-19]\n- [ ] Fix bug [added: 2026-03-02]\n"
	os.WriteFile(path, []byte(content), 0o644)

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Planned != "2026-03-19" {
		t.Errorf("item[0].Planned: got %q, want %q", items[0].Planned, "2026-03-19")
	}
	if items[1].Planned != "" {
		t.Errorf("item[1].Planned: got %q, want empty", items[1].Planned)
	}

	// Write back and re-parse to verify round-trip.
	outPath := filepath.Join(dir, "out.md")
	if err := tracker.WriteTracker(outPath, "Test", items); err != nil {
		t.Fatalf("write: %v", err)
	}
	items2, err := tracker.ParseTracker(outPath)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if items2[0].Planned != "2026-03-19" {
		t.Errorf("round-trip item[0].Planned: got %q, want %q", items2[0].Planned, "2026-03-19")
	}
	if items2[0].Title != "Review docs" {
		t.Errorf("round-trip title: got %q, want %q", items2[0].Title, "Review docs")
	}
	if items2[1].Planned != "" {
		t.Errorf("round-trip item[1].Planned: got %q, want empty", items2[1].Planned)
	}
}

func newPlannerService(t *testing.T, content string) *tracker.Service {
	t.Helper()
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "tracker.md")
	os.WriteFile(mdPath, []byte(content), 0o644)
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	store := tracker.NewStore(database, "personal")
	return tracker.NewService(mdPath, "Test", store)
}

func TestSetPlannedAndClear(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Task A [added: 2026-03-01]\n- [ ] Task B [added: 2026-03-02]\n")

	if err := svc.SetPlanned("task-a", "2026-03-19"); err != nil {
		t.Fatalf("SetPlanned: %v", err)
	}

	item, err := svc.Get("task-a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.Planned != "2026-03-19" {
		t.Errorf("Planned: got %q, want %q", item.Planned, "2026-03-19")
	}

	if err := svc.ClearPlanned("task-a"); err != nil {
		t.Fatalf("ClearPlanned: %v", err)
	}
	item, err = svc.Get("task-a")
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if item.Planned != "" {
		t.Errorf("Planned after clear: got %q, want empty", item.Planned)
	}
}

func TestListPlanned(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Task A [added: 2026-03-01] [planned: 2026-03-19]\n- [ ] Task B [added: 2026-03-02] [planned: 2026-03-20]\n- [ ] Task C [added: 2026-03-03]\n")

	planned := svc.ListPlanned("2026-03-19")
	if len(planned) != 1 {
		t.Fatalf("ListPlanned: expected 1, got %d", len(planned))
	}
	if planned[0].Slug != "task-a" {
		t.Errorf("ListPlanned[0].Slug: got %q, want %q", planned[0].Slug, "task-a")
	}
}

func TestListOverdue(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Overdue task [added: 2026-03-01] [planned: 2026-03-17]\n- [x] Done task [added: 2026-03-01] [planned: 2026-03-17] [completed: 2026-03-17]\n- [ ] Today task [added: 2026-03-01] [planned: 2026-03-19]\n")

	overdue := svc.ListOverdue("2026-03-19")
	if len(overdue) != 1 {
		t.Fatalf("ListOverdue: expected 1, got %d", len(overdue))
	}
	if overdue[0].Slug != "overdue-task" {
		t.Errorf("ListOverdue[0].Slug: got %q, want %q", overdue[0].Slug, "overdue-task")
	}
}

func TestListPlannedRange(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Mon [added: 2026-03-01] [planned: 2026-03-16]\n- [ ] Tue [added: 2026-03-01] [planned: 2026-03-17]\n- [ ] Fri [added: 2026-03-01] [planned: 2026-03-20]\n- [ ] Sat [added: 2026-03-01] [planned: 2026-03-21]\n")

	rangeItems := svc.ListPlannedRange("2026-03-16", "2026-03-20")
	if len(rangeItems) != 3 {
		t.Fatalf("ListPlannedRange: expected 3, got %d", len(rangeItems))
	}
}

func TestBulkSetPlanned(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Task A [added: 2026-03-01]\n- [ ] Task B [added: 2026-03-02]\n- [ ] Task C [added: 2026-03-03]\n")

	if err := svc.BulkSetPlanned([]string{"task-a", "task-c"}, "2026-03-19"); err != nil {
		t.Fatalf("BulkSetPlanned: %v", err)
	}

	planned := svc.ListPlanned("2026-03-19")
	if len(planned) != 2 {
		t.Fatalf("after bulk set: expected 2 planned, got %d", len(planned))
	}

	// Task B should remain unplanned.
	b, err := svc.Get("task-b")
	if err != nil {
		t.Fatalf("Get task-b: %v", err)
	}
	if b.Planned != "" {
		t.Errorf("task-b should be unplanned, got %q", b.Planned)
	}
}

func TestPlanAndCompleteRetainsPlannedDate(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Task A [added: 2026-03-01] [planned: 2026-03-19]\n")

	if err := svc.Complete("task-a"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	item, err := svc.Get("task-a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !item.Done {
		t.Error("expected item to be done")
	}
	if item.Planned != "2026-03-19" {
		t.Errorf("Planned should be retained after complete, got %q", item.Planned)
	}
}

func TestDeletedItemsExcludedFromListPlanned(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Active [added: 2026-03-01] [planned: 2026-03-19]\n- [ ] Deleted [added: 2026-03-01] [planned: 2026-03-19] [deleted: 2026-03-18]\n")

	planned := svc.ListPlanned("2026-03-19")
	if len(planned) != 1 {
		t.Fatalf("expected 1 (deleted excluded), got %d", len(planned))
	}
	if planned[0].Slug != "active" {
		t.Errorf("expected 'active', got %q", planned[0].Slug)
	}
}
