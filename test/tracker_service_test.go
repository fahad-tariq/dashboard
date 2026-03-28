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

func newTestService(t *testing.T, content string) *tracker.Service {
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
	return tracker.NewService(mdPath, "Tracker", store, time.UTC)
}

func TestTrackerServiceAddItem(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n")

	err := svc.AddItem(tracker.Item{Title: "Buy groceries", Type: tracker.TaskType})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
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
	if items[0].Slug != "buy-groceries" {
		t.Errorf("slug: got %q, want %q", items[0].Slug, "buy-groceries")
	}
	if items[0].Added == "" {
		t.Error("expected Added to be set automatically")
	}
}

func TestTrackerServiceAddItemEmptyTitle(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n")

	err := svc.AddItem(tracker.Item{Title: "", Type: tracker.TaskType})
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestTrackerServiceCompleteUncomplete(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Fix the sink\n")

	if err := svc.Complete("fix-the-sink"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	item, err := svc.Get("fix-the-sink")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !item.Done {
		t.Error("expected Done=true after Complete")
	}
	if item.Completed == "" {
		t.Error("expected Completed date to be set")
	}

	if err := svc.Uncomplete("fix-the-sink"); err != nil {
		t.Fatalf("Uncomplete: %v", err)
	}

	item, err = svc.Get("fix-the-sink")
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

	if err := svc.Delete("first-task"); err != nil {
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
	deleted, err := svc.Get("first-task")
	if err != nil {
		t.Fatalf("Get soft-deleted item: %v", err)
	}
	if deleted.DeletedAt == "" {
		t.Error("expected DeletedAt to be set")
	}
}

func TestTrackerServiceUpdateNotes(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Write report\n")

	if err := svc.UpdateNotes("write-report", "Draft in Google Docs\nDue Friday"); err != nil {
		t.Fatalf("UpdateNotes: %v", err)
	}

	item, _ := svc.Get("write-report")
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

			if err := svc.UpdatePriority("some-task", tt.priority); err != nil {
				t.Fatalf("UpdatePriority: %v", err)
			}

			item, _ := svc.Get("some-task")
			if item.Priority != tt.priority {
				t.Errorf("priority: got %q, want %q", item.Priority, tt.priority)
			}
		})
	}
}

func TestTrackerServiceUpdateTags(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Learn Go\n")

	if err := svc.UpdateTags("learn-go", []string{"tech", "study"}); err != nil {
		t.Fatalf("UpdateTags: %v", err)
	}

	item, _ := svc.Get("learn-go")
	if !slices.Equal(item.Tags, []string{"tech", "study"}) {
		t.Errorf("tags: got %v, want [tech study]", item.Tags)
	}

	if err := svc.UpdateTags("learn-go", nil); err != nil {
		t.Fatalf("UpdateTags clear: %v", err)
	}

	item, _ = svc.Get("learn-go")
	if len(item.Tags) != 0 {
		t.Errorf("expected empty tags after clear, got %v", item.Tags)
	}
}

func TestTrackerServiceUpdateEdit(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Old title\n  Old body\n")

	err := svc.UpdateEdit("old-title", "", "New body content", []string{"updated"}, []string{"img1.png"})
	if err != nil {
		t.Fatalf("UpdateEdit: %v", err)
	}

	item, _ := svc.Get("old-title")
	if item.Body != "New body content" {
		t.Errorf("body: got %q", item.Body)
	}
	if !slices.Equal(item.Tags, []string{"updated"}) {
		t.Errorf("tags: got %v", item.Tags)
	}
	if !slices.Equal(item.Images, []string{"img1.png"}) {
		t.Errorf("images: got %v", item.Images)
	}
}

func TestTrackerServiceUpdateEditTitle(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Old title\n  Some notes\n")

	err := svc.UpdateEdit("old-title", "New title", "Some notes", nil, nil)
	if err != nil {
		t.Fatalf("UpdateEdit: %v", err)
	}

	// Old slug should be gone.
	_, err = svc.Get("old-title")
	if err == nil {
		t.Error("old slug should no longer resolve")
	}

	// New slug should exist with updated title.
	item, err := svc.Get("new-title")
	if err != nil {
		t.Fatalf("Get by new slug: %v", err)
	}
	if item.Title != "New title" {
		t.Errorf("title: got %q, want %q", item.Title, "New title")
	}
	if item.Body != "Some notes" {
		t.Errorf("body: got %q", item.Body)
	}
}

func TestTrackerServiceUpdateEditEmptyTitleKeepsOriginal(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Keep me\n")

	err := svc.UpdateEdit("keep-me", "", "New body", nil, nil)
	if err != nil {
		t.Fatalf("UpdateEdit: %v", err)
	}

	item, err := svc.Get("keep-me")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.Title != "Keep me" {
		t.Errorf("title should remain %q, got %q", "Keep me", item.Title)
	}
}

func TestTrackerServiceSetProgress(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Read 40 books [goal: 5/40 books]\n")

	if err := svc.SetProgress("read-40-books", 20); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}

	item, _ := svc.Get("read-40-books")
	if item.Current != 20 {
		t.Errorf("current: got %v, want 20", item.Current)
	}
}

func TestTrackerServiceSetProgressClampNegative(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Run 100km [goal: 10/100 km]\n")

	if err := svc.SetProgress("run-100km", -5); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}

	item, _ := svc.Get("run-100km")
	if item.Current != 0 {
		t.Errorf("current: got %v, want 0 (clamped)", item.Current)
	}
}

func TestTrackerServiceSetProgressOnTask(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Not a goal\n")

	err := svc.SetProgress("not-a-goal", 10)
	if err == nil {
		t.Fatal("expected error when SetProgress on a task")
	}
}

func TestTrackerServiceUpdateProgress(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Save 10000 [goal: 2000/10000 dollars]\n")

	if err := svc.UpdateProgress("save-10000", 500); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}

	item, _ := svc.Get("save-10000")
	if item.Current != 2500 {
		t.Errorf("current: got %v, want 2500", item.Current)
	}
}

func TestTrackerServiceUpdateProgressNegativeDelta(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Save 10000 [goal: 100/10000 dollars]\n")

	if err := svc.UpdateProgress("save-10000", -200); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}

	item, _ := svc.Get("save-10000")
	if item.Current != 0 {
		t.Errorf("current: got %v, want 0 (clamped)", item.Current)
	}
}

func TestTrackerServiceUpdateProgressOnTask(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Not a goal\n")

	err := svc.UpdateProgress("not-a-goal", 10)
	if err == nil {
		t.Fatal("expected error when UpdateProgress on a task")
	}
}

func TestTrackerServiceGet(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha !high\n- [ ] Beta\n")

	item, err := svc.Get("alpha")
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
	if err := svc.Delete("alpha"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Alpha excluded from List.
	items, _ := svc.List()
	if len(items) != 1 {
		t.Fatalf("expected 1 in List, got %d", len(items))
	}

	// Alpha in ListDeleted.
	deleted := svc.ListDeleted()
	if len(deleted) != 1 || deleted[0].Slug != "alpha" {
		t.Fatalf("expected alpha in ListDeleted, got %v", deleted)
	}

	// Alpha excluded from Search.
	results := svc.Search("alpha")
	if len(results) != 0 {
		t.Errorf("soft-deleted item should not appear in Search, got %d results", len(results))
	}

	// Restore Alpha.
	if err := svc.Restore("alpha"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	items, _ = svc.List()
	if len(items) != 2 {
		t.Fatalf("expected 2 in List after restore, got %d", len(items))
	}

	restored, _ := svc.Get("alpha")
	if restored.DeletedAt != "" {
		t.Error("expected DeletedAt to be cleared after restore")
	}
}

func TestTrackerServicePermanentDelete(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha\n- [ ] Beta\n")

	if err := svc.PermanentDelete("alpha"); err != nil {
		t.Fatalf("PermanentDelete: %v", err)
	}

	// Alpha is completely gone.
	_, err := svc.Get("alpha")
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
	content := "# Tracker\n\n- [ ] Active task\n- [ ] Old trash [deleted: 2020-01-01]\n- [ ] Recent trash [deleted: 2099-12-31]\n"
	svc := newTestService(t, content)

	if err := svc.PurgeExpired(7); err != nil {
		t.Fatalf("PurgeExpired: %v", err)
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

	if err := svc.PurgeExpired(7); err != nil {
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

	if err := svc.BulkComplete([]string{"alpha", "gamma"}); err != nil {
		t.Fatalf("BulkComplete: %v", err)
	}

	alpha, _ := svc.Get("alpha")
	if !alpha.Done {
		t.Error("expected Alpha to be done")
	}
	gamma, _ := svc.Get("gamma")
	if !gamma.Done {
		t.Error("expected Gamma to be done")
	}
	beta, _ := svc.Get("beta")
	if beta.Done {
		t.Error("Beta should not be done")
	}
}

func TestTrackerServiceBulkDelete(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha\n- [ ] Beta\n- [ ] Gamma\n")

	if err := svc.BulkDelete([]string{"alpha", "beta"}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}

	items, _ := svc.List()
	if len(items) != 1 {
		t.Fatalf("expected 1 active item, got %d", len(items))
	}
	if items[0].Slug != "gamma" {
		t.Errorf("expected Gamma, got %q", items[0].Title)
	}

	deleted := svc.ListDeleted()
	if len(deleted) != 2 {
		t.Fatalf("expected 2 deleted items, got %d", len(deleted))
	}
}

func TestTrackerServiceBulkUpdatePriority(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha\n- [ ] Beta\n")

	if err := svc.BulkUpdatePriority([]string{"alpha", "beta"}, "high"); err != nil {
		t.Fatalf("BulkUpdatePriority: %v", err)
	}

	alpha, _ := svc.Get("alpha")
	if alpha.Priority != "high" {
		t.Errorf("Alpha priority: got %q, want %q", alpha.Priority, "high")
	}
	beta, _ := svc.Get("beta")
	if beta.Priority != "high" {
		t.Errorf("Beta priority: got %q, want %q", beta.Priority, "high")
	}
}

func TestTrackerServiceBulkAddTag(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha [tags: existing]\n- [ ] Beta\n")

	if err := svc.BulkAddTag([]string{"alpha", "beta"}, "urgent"); err != nil {
		t.Fatalf("BulkAddTag: %v", err)
	}

	alpha, _ := svc.Get("alpha")
	if len(alpha.Tags) != 2 {
		t.Errorf("Alpha should have 2 tags, got %v", alpha.Tags)
	}
	beta, _ := svc.Get("beta")
	if len(beta.Tags) != 1 || beta.Tags[0] != "urgent" {
		t.Errorf("Beta tags: got %v", beta.Tags)
	}
}

func TestTrackerServiceBulkAddTagSkipsDuplicates(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha [tags: urgent]\n")

	if err := svc.BulkAddTag([]string{"alpha"}, "urgent"); err != nil {
		t.Fatalf("BulkAddTag: %v", err)
	}

	alpha, _ := svc.Get("alpha")
	if len(alpha.Tags) != 1 {
		t.Errorf("expected 1 tag (no duplicate), got %v", alpha.Tags)
	}
}

func TestTrackerServiceBulkInvalidSlugRollsBack(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Alpha\n- [ ] Beta\n")

	err := svc.BulkComplete([]string{"alpha", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for invalid slug")
	}

	// Alpha should NOT be completed because the batch failed atomically.
	alpha, _ := svc.Get("alpha")
	if alpha.Done {
		t.Error("Alpha should not be done -- batch should have failed atomically")
	}
}

func TestTrackerServiceAddSubStep(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] Plan party\n  Book a venue\n")

	if err := svc.AddSubStep("plan-party", "Send invitations"); err != nil {
		t.Fatalf("AddSubStep: %v", err)
	}

	item, _ := svc.Get("plan-party")
	if item.SubStepsTotal != 1 {
		t.Errorf("SubStepsTotal: got %d, want 1", item.SubStepsTotal)
	}
	if item.SubStepsDone != 0 {
		t.Errorf("SubStepsDone: got %d, want 0", item.SubStepsDone)
	}

	// Add a second step.
	if err := svc.AddSubStep("plan-party", "Order cake"); err != nil {
		t.Fatalf("AddSubStep: %v", err)
	}
	item, _ = svc.Get("plan-party")
	if item.SubStepsTotal != 2 {
		t.Errorf("SubStepsTotal: got %d, want 2", item.SubStepsTotal)
	}
}

func TestTrackerServiceToggleSubStep(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] My task\n  - [ ] Step one\n  - [ ] Step two\n")

	// Toggle first step to done.
	if err := svc.ToggleSubStep("my-task", 0); err != nil {
		t.Fatalf("ToggleSubStep: %v", err)
	}
	item, _ := svc.Get("my-task")
	if item.SubStepsDone != 1 || item.SubStepsTotal != 2 {
		t.Errorf("after toggle: got %d/%d, want 1/2", item.SubStepsDone, item.SubStepsTotal)
	}

	// Toggle first step back to undone.
	if err := svc.ToggleSubStep("my-task", 0); err != nil {
		t.Fatalf("ToggleSubStep back: %v", err)
	}
	item, _ = svc.Get("my-task")
	if item.SubStepsDone != 0 {
		t.Errorf("after untoggle: got %d done, want 0", item.SubStepsDone)
	}
}

func TestTrackerServiceRemoveSubStep(t *testing.T) {
	svc := newTestService(t, "# Tracker\n\n- [ ] My task\n  - [ ] Step one\n  - [x] Step two\n  - [ ] Step three\n")

	// Remove middle step (index 1).
	if err := svc.RemoveSubStep("my-task", 1); err != nil {
		t.Fatalf("RemoveSubStep: %v", err)
	}
	item, _ := svc.Get("my-task")
	if item.SubStepsTotal != 2 {
		t.Errorf("SubStepsTotal: got %d, want 2", item.SubStepsTotal)
	}
	if item.SubStepsDone != 0 {
		t.Errorf("SubStepsDone: got %d, want 0 (removed the done step)", item.SubStepsDone)
	}

	// Out-of-range index should error.
	if err := svc.RemoveSubStep("my-task", 99); err == nil {
		t.Error("expected error for out-of-range index")
	}
}

func TestTrackerServicePurgeExpiredMalformedDate(t *testing.T) {
	// Use a date that matches the regex pattern but is invalid for time.Parse.
	content := "# Tracker\n\n- [ ] Bad date [deleted: 2026-13-45]\n"
	svc := newTestService(t, content)

	// Should not panic; malformed date items are kept.
	if err := svc.PurgeExpired(7); err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}

	deleted := svc.ListDeleted()
	if len(deleted) != 1 {
		t.Errorf("expected malformed-date item to be kept, got %d", len(deleted))
	}
}
