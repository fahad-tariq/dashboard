package test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

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
	return tracker.NewService(mdPath, "Tracker", store)
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

	items, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item after delete, got %d", len(items))
	}
	if items[0].Title != "Second task" {
		t.Errorf("remaining item: got %q, want %q", items[0].Title, "Second task")
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

	err := svc.UpdateEdit("old-title", "New body content", []string{"updated"}, []string{"img1.png"})
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
