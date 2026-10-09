package test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/itemid"
	"github.com/fahad/dashboard/internal/tracker"
)

func newTestService(t *testing.T, content string) *tracker.Service {
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
	return tracker.NewService(mdPath, "Tracker", time.UTC)
}

func TestTrackerServiceAddItem(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n")

	id, err := svc.AddItem(tracker.Item{Title: "Buy groceries", Type: tracker.TaskType})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if item, err := svc.Get(id); err != nil || item.Title != "Buy groceries" || !itemid.Valid(id) {
		t.Errorf("AddItem ID = %q; stored item %+v (%v)", id, item, err)
	}

	items, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Buy groceries" {
		t.Errorf("title: got %q, want %q", items[0].Title, "Buy groceries")
	}
	if items[0].ID != id {
		t.Errorf("ID: got %q, want %q", items[0].ID, id)
	}
	if items[0].Added == "" {
		t.Error("expected Added to be set automatically")
	}
}

func TestTrackerServiceAddItemEmptyTitle(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n")

	_, err := svc.AddItem(tracker.Item{Title: "", Type: tracker.TaskType})
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestTrackerServiceCompleteUncomplete(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Fix the sink\n")

	if err := svc.Complete(idOf(t, svc, "Fix the sink")); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	item, err := svc.Get(idOf(t, svc, "Fix the sink"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !item.Done {
		t.Error("expected Done=true after Complete")
	}
	if item.Completed == "" {
		t.Error("expected Completed date to be set")
	}

	if err := svc.Uncomplete(idOf(t, svc, "Fix the sink")); err != nil {
		t.Fatalf("Uncomplete: %v", err)
	}

	item, err = svc.Get(idOf(t, svc, "Fix the sink"))
	if err != nil {
		t.Fatalf("Get after Uncomplete: %v", err)
	}
	if item.Done {
		t.Error("expected Done=false after Uncomplete")
	}
	if item.Completed != "" {
		t.Errorf("expected Completed cleared, got %q", item.Completed)
	}
}

func TestTrackerServiceDelete(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] First task\n- [ ] Second task\n")

	if err := svc.Delete(idOf(t, svc, "First task")); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Delete is now soft-delete: item excluded from List but accessible via Get.
	items, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item in List after soft delete, got %d", len(items))
	}
	if items[0].Title != "Second task" {
		t.Errorf("remaining item: got %q, want %q", items[0].Title, "Second task")
	}

	// Soft-deleted item should still be accessible via Get.
	deleted, err := svc.Get(idOf(t, svc, "First task"))
	if err != nil {
		t.Fatalf("Get soft-deleted item: %v", err)
	}
	if deleted.DeletedAt == "" {
		t.Error("expected DeletedAt to be set")
	}
}

func TestTrackerServiceUpdateNotes(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Write report\n")

	if err := svc.UpdateNotes(idOf(t, svc, "Write report"), "Draft in Google Docs\nDue Friday"); err != nil {
		t.Fatalf("UpdateNotes: %v", err)
	}

	item, _ := svc.Get(idOf(t, svc, "Write report"))
	if item.Body != "Draft in Google Docs\nDue Friday" {
		t.Errorf("body: got %q", item.Body)
	}
}

func TestTrackerServiceUpdatePriority(t *testing.T) {
	tests := []struct {
		name     string
		priority string
	}{
		{"set high", "high"},
		{"set medium", "medium"},
		{"set low", "low"},
		{"clear priority", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t, "# Tracker\n\n- [ ] Some task\n")

			if err := svc.UpdatePriority(idOf(t, svc, "Some task"), tt.priority); err != nil {
				t.Fatalf("UpdatePriority: %v", err)
			}

			item, _ := svc.Get(idOf(t, svc, "Some task"))
			if item.Priority != tt.priority {
				t.Errorf("priority: got %q, want %q", item.Priority, tt.priority)
			}
		})
	}
}

func TestTrackerServiceUpdateTags(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Learn Go\n")

	if err := svc.UpdateTags(idOf(t, svc, "Learn Go"), []string{"tech", "study"}); err != nil {
		t.Fatalf("UpdateTags: %v", err)
	}

	item, _ := svc.Get(idOf(t, svc, "Learn Go"))
	if !slices.Equal(item.Tags, []string{"tech", "study"}) {
		t.Errorf("tags: got %v, want [tech study]", item.Tags)
	}

	if err := svc.UpdateTags(idOf(t, svc, "Learn Go"), nil); err != nil {
		t.Fatalf("UpdateTags clear: %v", err)
	}

	item, _ = svc.Get(idOf(t, svc, "Learn Go"))
	if len(item.Tags) != 0 {
		t.Errorf("expected empty tags after clear, got %v", item.Tags)
	}
}

// ApplyEdit changes only the fields an edit sets: an empty title and nil
// fields keep their values, an empty deadline clears it, and a new title
// keeps the ID.
func TestTrackerServiceApplyEdit(t *testing.T) {
	const seed = "# Tracker\n\n- [ ] Old title [deadline: 2026-11-20] [tags: home] [id: b7k2m9xq]\n  Old body\n"
	cases := map[string]struct {
		edit         tracker.Edit
		wantTitle    string
		wantBody     string
		wantTags     []string
		wantImages   []string
		wantDeadline string
	}{
		"every field the web form holds": {
			edit:      tracker.Edit{Body: new("New body content"), Tags: &[]string{"updated"}, Images: &[]string{"img1.png"}},
			wantTitle: "Old title", wantBody: "New body content",
			wantTags: []string{"updated"}, wantImages: []string{"img1.png"}, wantDeadline: "2026-11-20",
		},
		"a new title keeps the ID": {
			edit:      tracker.Edit{Title: "New title"},
			wantTitle: "New title", wantBody: "Old body",
			wantTags: []string{"home"}, wantDeadline: "2026-11-20",
		},
		"an empty title keeps the original": {
			edit:      tracker.Edit{Body: new("New body")},
			wantTitle: "Old title", wantBody: "New body",
			wantTags: []string{"home"}, wantDeadline: "2026-11-20",
		},
		"a deadline is set": {
			edit:      tracker.Edit{Deadline: new("2026-10-30")},
			wantTitle: "Old title", wantBody: "Old body",
			wantTags: []string{"home"}, wantDeadline: "2026-10-30",
		},
		"an empty deadline clears it": {
			edit:      tracker.Edit{Deadline: new("")},
			wantTitle: "Old title", wantBody: "Old body",
			wantTags: []string{"home"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := newTestService(t, seed)
			if err := svc.ApplyEdit("b7k2m9xq", tc.edit); err != nil {
				t.Fatalf("ApplyEdit: %v", err)
			}
			item, err := svc.Get("b7k2m9xq")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if item.Title != tc.wantTitle || item.Body != tc.wantBody || item.Deadline != tc.wantDeadline {
				t.Errorf("title, body, deadline = %q, %q, %q; want %q, %q, %q", item.Title, item.Body, item.Deadline, tc.wantTitle, tc.wantBody, tc.wantDeadline)
			}
			if !slices.Equal(item.Tags, tc.wantTags) || !slices.Equal(item.Images, tc.wantImages) {
				t.Errorf("tags, images = %v, %v; want %v, %v", item.Tags, item.Images, tc.wantTags, tc.wantImages)
			}
		})
	}
}

func TestTrackerServiceSetProgress(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Read 40 books [goal: 5/40 books]\n")

	if err := svc.SetProgress(idOf(t, svc, "Read 40 books"), 20); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}

	item, _ := svc.Get(idOf(t, svc, "Read 40 books"))
	if item.Current != 20 {
		t.Errorf("current: got %v, want 20", item.Current)
	}
}

func TestTrackerServiceSetProgressClampNegative(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Run 100km [goal: 10/100 km]\n")

	if err := svc.SetProgress(idOf(t, svc, "Run 100km"), -5); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}

	item, _ := svc.Get(idOf(t, svc, "Run 100km"))
	if item.Current != 0 {
		t.Errorf("current: got %v, want 0 (clamped)", item.Current)
	}
}

func TestTrackerServiceSetProgressOnTask(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Not a goal\n")

	err := svc.SetProgress(idOf(t, svc, "Not a goal"), 10)
	if err == nil {
		t.Fatal("expected error when SetProgress on a task")
	}
}

func TestTrackerServiceUpdateProgress(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Save 10000 [goal: 2000/10000 dollars]\n")

	if err := svc.UpdateProgress(idOf(t, svc, "Save 10000"), 500); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}

	item, _ := svc.Get(idOf(t, svc, "Save 10000"))
	if item.Current != 2500 {
		t.Errorf("current: got %v, want 2500", item.Current)
	}
}

func TestTrackerServiceUpdateProgressNegativeDelta(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Save 10000 [goal: 100/10000 dollars]\n")

	if err := svc.UpdateProgress(idOf(t, svc, "Save 10000"), -200); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}

	item, _ := svc.Get(idOf(t, svc, "Save 10000"))
	if item.Current != 0 {
		t.Errorf("current: got %v, want 0 (clamped)", item.Current)
	}
}

func TestTrackerServiceUpdateProgressOnTask(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Not a goal\n")

	err := svc.UpdateProgress(idOf(t, svc, "Not a goal"), 10)
	if err == nil {
		t.Fatal("expected error when UpdateProgress on a task")
	}
}

func TestTrackerServiceGet(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha !high\n- [ ] Beta\n")

	item, err := svc.Get(idOf(t, svc, "Alpha"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.Title != "Alpha" {
		t.Errorf("title: got %q", item.Title)
	}
	if item.Priority != "high" {
		t.Errorf("priority: got %q", item.Priority)
	}
}

func TestTrackerServiceErrorCases(t *testing.T) {
	tests := []struct {
		name string
		fn   func(svc *tracker.Service) error
	}{
		{"Get non-existent", func(svc *tracker.Service) error {
			_, err := svc.Get("nope")
			return err
		}},
		{"Complete non-existent", func(svc *tracker.Service) error {
			return svc.Complete("nope")
		}},
		{"Uncomplete non-existent", func(svc *tracker.Service) error {
			return svc.Uncomplete("nope")
		}},
		{"Delete non-existent", func(svc *tracker.Service) error {
			return svc.Delete("nope")
		}},
		{"UpdateNotes non-existent", func(svc *tracker.Service) error {
			return svc.UpdateNotes("nope", "body")
		}},
		{"UpdatePriority non-existent", func(svc *tracker.Service) error {
			return svc.UpdatePriority("nope", "high")
		}},
		{"UpdateTags non-existent", func(svc *tracker.Service) error {
			return svc.UpdateTags("nope", []string{"tag"})
		}},
		{"SetProgress non-existent", func(svc *tracker.Service) error {
			return svc.SetProgress("nope", 10)
		}},
		{"UpdateProgress non-existent", func(svc *tracker.Service) error {
			return svc.UpdateProgress("nope", 10)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t, "# Tracker\n\n- [ ] Existing task\n")
			if err := tt.fn(svc); err == nil {
				t.Error("expected error for non-existent slug")
			}
		})
	}
}

func TestTrackerServiceSoftDeleteAndRestore(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha\n- [ ] Beta\n")

	// Soft delete Alpha.
	if err := svc.Delete(idOf(t, svc, "Alpha")); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Alpha excluded from List.
	items, _ := svc.List()
	if len(items) != 1 {
		t.Fatalf("expected 1 in List, got %d", len(items))
	}

	// Alpha in ListDeleted.
	deleted := svc.ListDeleted()
	if len(deleted) != 1 || deleted[0].Title != "Alpha" {
		t.Fatalf("expected alpha in ListDeleted, got %v", deleted)
	}

	// Alpha excluded from Search.
	results := svc.Search(idOf(t, svc, "Alpha"))
	if len(results) != 0 {
		t.Errorf("soft-deleted item should not appear in Search, got %d results", len(results))
	}

	// Restore Alpha.
	if err := svc.Restore(idOf(t, svc, "Alpha")); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	items, _ = svc.List()
	if len(items) != 2 {
		t.Fatalf("expected 2 in List after restore, got %d", len(items))
	}

	restored, _ := svc.Get(idOf(t, svc, "Alpha"))
	if restored.DeletedAt != "" {
		t.Error("expected DeletedAt to be cleared after restore")
	}
}

func TestTrackerServicePermanentDelete(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha\n- [ ] Beta\n")

	alpha := idOf(t, svc, "Alpha")
	if err := svc.PermanentDelete(alpha); err != nil {
		t.Fatalf("PermanentDelete: %v", err)
	}

	// Alpha is completely gone.
	_, err := svc.Get(alpha)
	if err == nil {
		t.Error("expected error after permanent delete")
	}

	items, _ := svc.List()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestTrackerServicePurgeExpired(t *testing.T) {
	// Create file with a recently deleted item and an old deleted item.
	content := "# Tracker\n\n- [ ] Active task\n- [ ] Old trash [deleted: 2020-01-01] [id: 1dtr4sh5]\n- [ ] Recent trash [deleted: 2099-12-31] [id: r3c3ntt5]\n"
	svc := newTestService(t, content)

	purged, err := svc.PurgeExpired(7)
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if !slices.Equal(purged, []string{"1dtr4sh5"}) {
		t.Errorf("PurgeExpired returned %v, want [1dtr4sh5]", purged)
	}

	// Active task and recent trash should remain.
	items, _ := svc.List()
	if len(items) != 1 {
		t.Fatalf("expected 1 active item, got %d", len(items))
	}
	if items[0].Title != "Active task" {
		t.Errorf("expected Active task, got %q", items[0].Title)
	}

	deleted := svc.ListDeleted()
	if len(deleted) != 1 || deleted[0].Title != "Recent trash" {
		t.Errorf("expected only Recent trash in deleted, got %v", deleted)
	}
}

func TestTrackerServicePurgeExpiredBoundary(t *testing.T) {
	// Item deleted exactly 7 days ago should be purged (cutoff is strictly before).
	// Use UTC to match the service's timezone (time.UTC passed to NewService).
	now := time.Now().UTC()
	sevenDaysAgo := now.AddDate(0, 0, -7).Format("2006-01-02")
	sixDaysAgo := now.AddDate(0, 0, -6).Format("2006-01-02")
	content := "# Tracker\n\n- [ ] At boundary [deleted: " + sevenDaysAgo + "]\n- [ ] Within window [deleted: " + sixDaysAgo + "]\n"
	svc := newTestService(t, content)

	if _, err := svc.PurgeExpired(7); err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}

	deleted := svc.ListDeleted()
	if len(deleted) != 1 {
		t.Fatalf("expected 1 remaining deleted item, got %d", len(deleted))
	}
	if deleted[0].Title != "Within window" {
		t.Errorf("expected 'Within window' to remain, got %q", deleted[0].Title)
	}
}

func TestTrackerServiceBulkComplete(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha\n- [ ] Beta\n- [ ] Gamma\n")

	if err := svc.BulkComplete([]string{idOf(t, svc, "Alpha"), idOf(t, svc, "Gamma")}); err != nil {
		t.Fatalf("BulkComplete: %v", err)
	}

	alpha, _ := svc.Get(idOf(t, svc, "Alpha"))
	if !alpha.Done {
		t.Error("expected Alpha to be done")
	}
	gamma, _ := svc.Get(idOf(t, svc, "Gamma"))
	if !gamma.Done {
		t.Error("expected Gamma to be done")
	}
	beta, _ := svc.Get(idOf(t, svc, "Beta"))
	if beta.Done {
		t.Error("Beta should not be done")
	}
}

func TestTrackerServiceBulkDelete(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha\n- [ ] Beta\n- [ ] Gamma\n")

	if err := svc.BulkDelete([]string{idOf(t, svc, "Alpha"), idOf(t, svc, "Beta")}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}

	items, _ := svc.List()
	if len(items) != 1 {
		t.Fatalf("expected 1 active item, got %d", len(items))
	}
	if items[0].Title != "Gamma" {
		t.Errorf("expected Gamma, got %q", items[0].Title)
	}

	deleted := svc.ListDeleted()
	if len(deleted) != 2 {
		t.Fatalf("expected 2 deleted items, got %d", len(deleted))
	}
}

func TestTrackerServiceBulkUpdatePriority(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha\n- [ ] Beta\n")

	if err := svc.BulkUpdatePriority([]string{idOf(t, svc, "Alpha"), idOf(t, svc, "Beta")}, "high"); err != nil {
		t.Fatalf("BulkUpdatePriority: %v", err)
	}

	alpha, _ := svc.Get(idOf(t, svc, "Alpha"))
	if alpha.Priority != "high" {
		t.Errorf("Alpha priority: got %q, want %q", alpha.Priority, "high")
	}
	beta, _ := svc.Get(idOf(t, svc, "Beta"))
	if beta.Priority != "high" {
		t.Errorf("Beta priority: got %q, want %q", beta.Priority, "high")
	}
}

func TestTrackerServiceBulkAddTag(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha [tags: existing]\n- [ ] Beta\n")

	if err := svc.BulkAddTag([]string{idOf(t, svc, "Alpha"), idOf(t, svc, "Beta")}, "urgent"); err != nil {
		t.Fatalf("BulkAddTag: %v", err)
	}

	alpha, _ := svc.Get(idOf(t, svc, "Alpha"))
	if len(alpha.Tags) != 2 {
		t.Errorf("Alpha should have 2 tags, got %v", alpha.Tags)
	}
	beta, _ := svc.Get(idOf(t, svc, "Beta"))
	if len(beta.Tags) != 1 || beta.Tags[0] != "urgent" {
		t.Errorf("Beta tags: got %v", beta.Tags)
	}
}

func TestTrackerServiceBulkAddTagSkipsDuplicates(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha [tags: urgent]\n")

	if err := svc.BulkAddTag([]string{idOf(t, svc, "Alpha")}, "urgent"); err != nil {
		t.Fatalf("BulkAddTag: %v", err)
	}

	alpha, _ := svc.Get(idOf(t, svc, "Alpha"))
	if len(alpha.Tags) != 1 {
		t.Errorf("expected 1 tag (no duplicate), got %v", alpha.Tags)
	}
}

func TestTrackerServiceBulkInvalidSlugRollsBack(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha\n- [ ] Beta\n")

	err := svc.BulkComplete([]string{idOf(t, svc, "Alpha"), "nonexistent"})
	if err == nil {
		t.Fatal("expected error for invalid slug")
	}

	// Alpha should NOT be completed because the batch failed atomically.
	alpha, _ := svc.Get(idOf(t, svc, "Alpha"))
	if alpha.Done {
		t.Error("Alpha should not be done -- batch should have failed atomically")
	}
}

func TestTrackerServiceAddSubStep(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Plan party\n  Book a venue\n")

	if err := svc.AddSubStep(idOf(t, svc, "Plan party"), "Send invitations"); err != nil {
		t.Fatalf("AddSubStep: %v", err)
	}

	item, _ := svc.Get(idOf(t, svc, "Plan party"))
	if item.SubStepsTotal != 1 {
		t.Errorf("SubStepsTotal: got %d, want 1", item.SubStepsTotal)
	}
	if item.SubStepsDone != 0 {
		t.Errorf("SubStepsDone: got %d, want 0", item.SubStepsDone)
	}

	// Add a second step.
	if err := svc.AddSubStep(idOf(t, svc, "Plan party"), "Order cake"); err != nil {
		t.Fatalf("AddSubStep: %v", err)
	}
	item, _ = svc.Get(idOf(t, svc, "Plan party"))
	if item.SubStepsTotal != 2 {
		t.Errorf("SubStepsTotal: got %d, want 2", item.SubStepsTotal)
	}
}

func TestTrackerServiceToggleSubStep(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] My task\n  - [ ] Step one\n  - [ ] Step two\n")

	// Toggle first step to done.
	if err := svc.ToggleSubStep(idOf(t, svc, "My task"), 0); err != nil {
		t.Fatalf("ToggleSubStep: %v", err)
	}
	item, _ := svc.Get(idOf(t, svc, "My task"))
	if item.SubStepsDone != 1 || item.SubStepsTotal != 2 {
		t.Errorf("after toggle: got %d/%d, want 1/2", item.SubStepsDone, item.SubStepsTotal)
	}

	// Toggle first step back to undone.
	if err := svc.ToggleSubStep(idOf(t, svc, "My task"), 0); err != nil {
		t.Fatalf("ToggleSubStep back: %v", err)
	}
	item, _ = svc.Get(idOf(t, svc, "My task"))
	if item.SubStepsDone != 0 {
		t.Errorf("after untoggle: got %d done, want 0", item.SubStepsDone)
	}
}

func TestTrackerServiceRemoveSubStep(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] My task\n  - [ ] Step one\n  - [x] Step two\n  - [ ] Step three\n")

	// Remove middle step (index 1).
	if err := svc.RemoveSubStep(idOf(t, svc, "My task"), 1); err != nil {
		t.Fatalf("RemoveSubStep: %v", err)
	}
	item, _ := svc.Get(idOf(t, svc, "My task"))
	if item.SubStepsTotal != 2 {
		t.Errorf("SubStepsTotal: got %d, want 2", item.SubStepsTotal)
	}
	if item.SubStepsDone != 0 {
		t.Errorf("SubStepsDone: got %d, want 0 (removed the done step)", item.SubStepsDone)
	}

	// Out-of-range index should error.
	if err := svc.RemoveSubStep(idOf(t, svc, "My task"), 99); err == nil {
		t.Error("expected error for out-of-range index")
	}
}

func TestTrackerServicePurgeExpiredMalformedDate(t *testing.T) {
	// Use a date that matches the regex pattern but is invalid for time.Parse.
	content := "# Tracker\n\n- [ ] Bad date [deleted: 2026-13-45]\n"
	svc := newTestService(t, content)

	// Should not panic; malformed date items are kept.
	if _, err := svc.PurgeExpired(7); err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}

	deleted := svc.ListDeleted()
	if len(deleted) != 1 {
		t.Errorf("expected malformed-date item to be kept, got %d", len(deleted))
	}
}

// idOf returns the ID of the item titled title, deleted or not.
func idOf(t *testing.T, svc *tracker.Service, title string) string {
	t.Helper()
	for _, it := range svc.All() {
		if it.Title == title {
			return it.ID
		}
	}
	t.Fatalf("no item titled %q", title)
	return ""
}
