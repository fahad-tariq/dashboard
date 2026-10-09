package test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/tracker"
)

func TestPlannedMetadataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tracker.md")
	content := "# Test\n\n- [ ] Review docs [added: 2026-03-01] [planned: 2026-03-19]\n- [ ] Fix bug [added: 2026-03-02]\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

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

// IDs of the seeded Task A, B and C.
const taskA, taskB, taskC = "tskbbbb1", "tskbbbb2", "tskbbbb3"

func newPlannerService(t *testing.T, content string) *tracker.Service {
	t.Helper()
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "tracker.md")
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return tracker.NewService(mdPath, "Test", time.UTC)
}

func TestSetPlannedAndClear(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Task A [added: 2026-03-01] [id: tskbbbb1]\n- [ ] Task B [added: 2026-03-02] [id: tskbbbb2]\n")

	if err := svc.SetPlanned(taskA, "2026-03-19"); err != nil {
		t.Fatalf("SetPlanned: %v", err)
	}

	item, err := svc.Get(taskA)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.Planned != "2026-03-19" {
		t.Errorf("Planned: got %q, want %q", item.Planned, "2026-03-19")
	}

	if err := svc.ClearPlanned(taskA); err != nil {
		t.Fatalf("ClearPlanned: %v", err)
	}
	item, err = svc.Get(taskA)
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if item.Planned != "" {
		t.Errorf("Planned after clear: got %q, want empty", item.Planned)
	}
}

func TestListPlanned(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Task A [added: 2026-03-01] [planned: 2026-03-19] [id: tskbbbb1]\n- [ ] Task B [added: 2026-03-02] [planned: 2026-03-20] [id: tskbbbb2]\n- [ ] Task C [added: 2026-03-03] [id: tskbbbb3]\n")

	planned := svc.ListPlanned("2026-03-19")
	if len(planned) != 1 {
		t.Fatalf("ListPlanned: expected 1, got %d", len(planned))
	}
	if planned[0].Title != "Task A" {
		t.Errorf("ListPlanned[0].Title: got %q, want %q", planned[0].Title, "Task A")
	}
}

func TestListOverdue(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Overdue task [added: 2026-03-01] [planned: 2026-03-17]\n- [x] Done task [added: 2026-03-01] [planned: 2026-03-17] [completed: 2026-03-17]\n- [ ] Today task [added: 2026-03-01] [planned: 2026-03-19]\n")

	overdue := svc.ListOverdue("2026-03-19")
	if len(overdue) != 1 {
		t.Fatalf("ListOverdue: expected 1, got %d", len(overdue))
	}
	if overdue[0].Title != "Overdue task" {
		t.Errorf("ListOverdue[0].Title: got %q, want %q", overdue[0].Title, "Overdue task")
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
	svc := newPlannerService(t, "# Test\n\n- [ ] Task A [added: 2026-03-01] [id: tskbbbb1]\n- [ ] Task B [added: 2026-03-02] [id: tskbbbb2]\n- [ ] Task C [added: 2026-03-03] [id: tskbbbb3]\n")

	if err := svc.BulkSetPlanned([]string{taskA, taskC}, "2026-03-19"); err != nil {
		t.Fatalf("BulkSetPlanned: %v", err)
	}

	planned := svc.ListPlanned("2026-03-19")
	if len(planned) != 2 {
		t.Fatalf("after bulk set: expected 2 planned, got %d", len(planned))
	}

	// Task B should remain unplanned.
	b, err := svc.Get(taskB)
	if err != nil {
		t.Fatalf("Get task-b: %v", err)
	}
	if b.Planned != "" {
		t.Errorf("task-b should be unplanned, got %q", b.Planned)
	}
}

func TestPlanAndCompleteRetainsPlannedDate(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Task A [added: 2026-03-01] [planned: 2026-03-19] [id: tskbbbb1]\n")

	if err := svc.Complete(taskA); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	item, err := svc.Get(taskA)
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
	if planned[0].Title != "Active" {
		t.Errorf("expected 'Active', got %q", planned[0].Title)
	}
}

func TestPlanOrderMetadataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tracker.md")
	content := "# Test\n\n- [ ] Task A [added: 2026-03-01] [planned: 2026-03-19] [plan-order: 2] [id: tskbbbb1]\n- [ ] Task B [added: 2026-03-02] [planned: 2026-03-19] [plan-order: 1] [id: tskbbbb2]\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].PlanOrder != 2 {
		t.Errorf("item[0].PlanOrder: got %d, want 2", items[0].PlanOrder)
	}
	if items[1].PlanOrder != 1 {
		t.Errorf("item[1].PlanOrder: got %d, want 1", items[1].PlanOrder)
	}

	// Write back and re-parse.
	outPath := filepath.Join(dir, "out.md")
	if err := tracker.WriteTracker(outPath, "Test", items); err != nil {
		t.Fatalf("write: %v", err)
	}
	items2, err := tracker.ParseTracker(outPath)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if items2[0].PlanOrder != 2 {
		t.Errorf("round-trip item[0].PlanOrder: got %d, want 2", items2[0].PlanOrder)
	}
	if items2[1].PlanOrder != 1 {
		t.Errorf("round-trip item[1].PlanOrder: got %d, want 1", items2[1].PlanOrder)
	}
	if items2[0].Title != "Task A" {
		t.Errorf("round-trip title: got %q, want %q", items2[0].Title, "Task A")
	}
}

func TestReorderPlanned(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Task A [added: 2026-03-01] [planned: 2026-03-19] [id: tskbbbb1]\n- [ ] Task B [added: 2026-03-02] [planned: 2026-03-19] [id: tskbbbb2]\n- [ ] Task C [added: 2026-03-03] [planned: 2026-03-19] [id: tskbbbb3]\n")

	// Reorder: C, A, B
	if err := svc.ReorderPlanned([]string{taskC, taskA, taskB}); err != nil {
		t.Fatalf("ReorderPlanned: %v", err)
	}

	a, _ := svc.Get(taskA)
	b, _ := svc.Get(taskB)
	c, _ := svc.Get(taskC)

	if c.PlanOrder != 1 {
		t.Errorf("task-c PlanOrder: got %d, want 1", c.PlanOrder)
	}
	if a.PlanOrder != 2 {
		t.Errorf("task-a PlanOrder: got %d, want 2", a.PlanOrder)
	}
	if b.PlanOrder != 3 {
		t.Errorf("task-b PlanOrder: got %d, want 3", b.PlanOrder)
	}
}

func TestSortPlanItems(t *testing.T) {
	items := []tracker.Item{
		{Title: "unordered-high", Priority: "high", PlanOrder: 0},
		{Title: "ordered-3", PlanOrder: 3},
		{Title: "ordered-1", PlanOrder: 1},
		{Title: "unordered-low", Priority: "low", PlanOrder: 0},
		{Title: "ordered-2", PlanOrder: 2},
	}

	// Use the same sort logic as sortPlanItems.
	slices.SortStableFunc(items, func(a, b tracker.Item) int {
		aHas := a.PlanOrder > 0
		bHas := b.PlanOrder > 0
		switch {
		case aHas && bHas:
			return a.PlanOrder - b.PlanOrder
		case aHas:
			return -1
		case bHas:
			return 1
		default:
			return tracker.PriorityWeight[a.Priority] - tracker.PriorityWeight[b.Priority]
		}
	})

	want := []string{"ordered-1", "ordered-2", "ordered-3", "unordered-high", "unordered-low"}
	for i, title := range want {
		if items[i].Title != title {
			t.Errorf("position %d: got %q, want %q", i, items[i].Title, title)
		}
	}
}

func TestClearPlannedResetsPlanOrder(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Task A [added: 2026-03-01] [planned: 2026-03-19] [plan-order: 2] [id: tskbbbb1]\n")

	if err := svc.ClearPlanned(taskA); err != nil {
		t.Fatalf("ClearPlanned: %v", err)
	}
	item, err := svc.Get(taskA)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.Planned != "" {
		t.Errorf("Planned should be empty, got %q", item.Planned)
	}
	if item.PlanOrder != 0 {
		t.Errorf("PlanOrder should be 0 after clear, got %d", item.PlanOrder)
	}
}

func TestSetPlannedResetsPlanOrder(t *testing.T) {
	svc := newPlannerService(t, "# Test\n\n- [ ] Task A [added: 2026-03-01] [planned: 2026-03-19] [plan-order: 3] [id: tskbbbb1]\n")

	// Re-plan to a different date -- order should reset.
	if err := svc.SetPlanned(taskA, "2026-03-20"); err != nil {
		t.Fatalf("SetPlanned: %v", err)
	}
	item, err := svc.Get(taskA)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.Planned != "2026-03-20" {
		t.Errorf("Planned: got %q, want %q", item.Planned, "2026-03-20")
	}
	if item.PlanOrder != 0 {
		t.Errorf("PlanOrder should be 0 after replanning, got %d", item.PlanOrder)
	}
}
