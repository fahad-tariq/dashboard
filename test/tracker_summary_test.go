package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/tracker"
)

// Summary comes from the in-memory cache; the service needs no database.
func TestTrackerSummaryFromCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "personal.md")
	content := "# Personal\n\n" +
		"- [ ] Open task one\n" +
		"- [ ] Open task two\n" +
		"- [x] Finished task [completed: 2026-10-01]\n" +
		"- [ ] Trashed task [deleted: 2026-10-02]\n" +
		"- [ ] Run a marathon [goal: 10/42 km]\n" +
		"- [x] Done goal [goal: 5/5 kg]\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := tracker.NewService(path, "Personal", time.UTC)
	sum, err := svc.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if sum.OpenTasks != 2 || sum.ActiveGoals != 1 {
		t.Errorf("Summary = %+v, want 2 open tasks and 1 active goal", sum)
	}

	if err := svc.AddItem(tracker.Item{Title: "Another", Type: tracker.TaskType}); err != nil {
		t.Fatal(err)
	}
	if sum, _ := svc.Summary(); sum.OpenTasks != 3 {
		t.Errorf("after AddItem OpenTasks = %d, want 3", sum.OpenTasks)
	}
}
