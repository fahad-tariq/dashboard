package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/services"
	"github.com/fahad/dashboard/internal/tracker"
)

func setupRegistry(t *testing.T) (*services.Registry, string) {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	userDataDir := filepath.Join(tmpDir, "users")
	familyPath := filepath.Join(tmpDir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)
	houseProjectsPath := filepath.Join(tmpDir, "house-projects.md")
	os.WriteFile(houseProjectsPath, []byte("# House\n\n"), 0o644)

	reg := services.NewRegistry(database, userDataDir, familyPath, houseProjectsPath, time.UTC)
	return reg, tmpDir
}

func TestPersonalDataIsolation(t *testing.T) {
	reg, _ := setupRegistry(t)
	reg.EnsureUserDirs(1)
	reg.EnsureUserDirs(2)

	user1 := reg.ForUser(1)
	user2 := reg.ForUser(2)

	// Resync both personal services.
	user1.Personal.Resync()
	user2.Personal.Resync()

	// Add a task for user 1.
	if err := user1.Personal.AddItem(tracker.Item{Title: "User 1 task", Type: tracker.TaskType}); err != nil {
		t.Fatalf("adding task for user 1: %v", err)
	}

	// Add a task for user 2.
	if err := user2.Personal.AddItem(tracker.Item{Title: "User 2 task", Type: tracker.TaskType}); err != nil {
		t.Fatalf("adding task for user 2: %v", err)
	}

	// User 1 should only see their own task.
	items1, _ := user1.Personal.List()
	if len(items1) != 1 || items1[0].Title != "User 1 task" {
		t.Errorf("user 1 should see only their task, got %v", items1)
	}

	// User 2 should only see their own task.
	items2, _ := user2.Personal.List()
	if len(items2) != 1 || items2[0].Title != "User 2 task" {
		t.Errorf("user 2 should see only their task, got %v", items2)
	}
}

func TestFamilyListSharedAcrossUsers(t *testing.T) {
	reg, _ := setupRegistry(t)
	reg.EnsureUserDirs(1)
	reg.EnsureUserDirs(2)

	familySvc := reg.Family()
	familySvc.Resync()

	// Add a task to family.
	if err := familySvc.AddItem(tracker.Item{Title: "Shared task", Type: tracker.TaskType}); err != nil {
		t.Fatalf("adding family task: %v", err)
	}

	// Both users should see the same family data.
	items, _ := familySvc.List()
	if len(items) != 1 || items[0].Title != "Shared task" {
		t.Errorf("family list should contain the shared task, got %v", items)
	}

	// Summary should be the same view.
	sum, _ := familySvc.Summary()
	if sum.OpenTasks != 1 {
		t.Errorf("family summary should show 1 open task, got %d", sum.OpenTasks)
	}
}

func TestMoveFromPersonalToFamily(t *testing.T) {
	reg, _ := setupRegistry(t)
	reg.EnsureUserDirs(1)

	user1 := reg.ForUser(1)
	familySvc := reg.Family()

	user1.Personal.Resync()
	familySvc.Resync()

	// Add a task to personal.
	if err := user1.Personal.AddItem(tracker.Item{Title: "Move me", Type: tracker.TaskType}); err != nil {
		t.Fatalf("adding personal task: %v", err)
	}

	// Get the item and move it to family.
	item, err := user1.Personal.Get("move-me")
	if err != nil {
		t.Fatalf("getting item: %v", err)
	}
	if err := familySvc.AddItem(*item); err != nil {
		t.Fatalf("adding to family: %v", err)
	}
	if err := user1.Personal.Delete("move-me"); err != nil {
		t.Fatalf("deleting from personal: %v", err)
	}

	// Personal should be empty.
	personalItems, _ := user1.Personal.List()
	if len(personalItems) != 0 {
		t.Errorf("personal should be empty after move, got %d items", len(personalItems))
	}

	// Family should have the item.
	familyItems, _ := familySvc.List()
	if len(familyItems) != 1 || familyItems[0].Title != "Move me" {
		t.Errorf("family should have the moved item, got %v", familyItems)
	}
}

func TestToTaskFromIdeasCreatesInUserPersonal(t *testing.T) {
	reg, _ := setupRegistry(t)
	reg.EnsureUserDirs(1)

	user1 := reg.ForUser(1)
	user1.Personal.Resync()

	// Add an idea for user 1.
	idea := &ideas.Idea{
		Slug:  "test-idea",
		Title: "Test Idea",
		Tags:  []string{"feature"},
		Body:  "# Test Idea\n\nSome description.",
	}
	if err := user1.Ideas.Add(idea); err != nil {
		t.Fatalf("adding idea: %v", err)
	}

	// Simulate the ToTaskFunc closure.
	toTask := func(_ context.Context, title, body string, tags []string) error {
		item := tracker.Item{
			Title: title,
			Type:  tracker.TaskType,
			Body:  body,
			Tags:  tags,
		}
		return user1.Personal.AddItem(item)
	}

	if err := toTask(context.Background(), "Test Idea", "Some description.", []string{"feature"}); err != nil {
		t.Fatalf("converting idea to task: %v", err)
	}

	// Personal should now have the task.
	items, _ := user1.Personal.List()
	found := false
	for _, it := range items {
		if it.Title == "Test Idea" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find 'Test Idea' in user 1's personal list")
	}
}
