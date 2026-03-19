package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/services"
	"github.com/fahad/dashboard/internal/tracker"
)

func TestEnsureUserDirsCreatesStructure(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	userDataDir := filepath.Join(tmpDir, "users")
	familyPath := filepath.Join(tmpDir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)

	reg := services.NewRegistry(database, userDataDir, familyPath, time.UTC)

	if err := reg.EnsureUserDirs(1); err != nil {
		t.Fatalf("EnsureUserDirs: %v", err)
	}

	// Verify directory structure.
	expected := []string{
		"1",
		"1/personal.md",
		"1/ideas.md",
	}
	for _, rel := range expected {
		path := filepath.Join(userDataDir, rel)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", rel)
		}
	}
}

func TestEnsureUserDirsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	userDataDir := filepath.Join(tmpDir, "users")
	familyPath := filepath.Join(tmpDir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)

	reg := services.NewRegistry(database, userDataDir, familyPath, time.UTC)

	// Write some content to personal.md.
	if err := reg.EnsureUserDirs(1); err != nil {
		t.Fatalf("first EnsureUserDirs: %v", err)
	}
	personalPath := filepath.Join(userDataDir, "1", "personal.md")
	os.WriteFile(personalPath, []byte("# Personal\n\n- [ ] My task\n"), 0o644)

	// Call again -- should not overwrite existing personal.md.
	if err := reg.EnsureUserDirs(1); err != nil {
		t.Fatalf("second EnsureUserDirs: %v", err)
	}
	data, _ := os.ReadFile(personalPath)
	if string(data) != "# Personal\n\n- [ ] My task\n" {
		t.Error("EnsureUserDirs should not overwrite existing personal.md")
	}
}

func TestForUserReturnsCachedInstances(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	userDataDir := filepath.Join(tmpDir, "users")
	familyPath := filepath.Join(tmpDir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)

	reg := services.NewRegistry(database, userDataDir, familyPath, time.UTC)
	reg.EnsureUserDirs(1)

	svc1 := reg.ForUser(1)
	svc2 := reg.ForUser(1)
	if svc1 != svc2 {
		t.Error("ForUser should return the same cached instance on second call")
	}
}

func TestNewUserStoreFiltersByUserID(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	store1 := tracker.NewUserStore(database, "personal", 1)
	store2 := tracker.NewUserStore(database, "personal", 2)

	items1 := []tracker.Item{{Slug: "task-a", Title: "Task A", Type: tracker.TaskType}}
	items2 := []tracker.Item{{Slug: "task-b", Title: "Task B", Type: tracker.TaskType}}

	if err := store1.ReplaceAll(items1); err != nil {
		t.Fatalf("store1 ReplaceAll: %v", err)
	}
	if err := store2.ReplaceAll(items2); err != nil {
		t.Fatalf("store2 ReplaceAll: %v", err)
	}

	sum1, _ := store1.Summary()
	sum2, _ := store2.Summary()

	if sum1.OpenTasks != 1 {
		t.Errorf("store1 should have 1 open task, got %d", sum1.OpenTasks)
	}
	if sum2.OpenTasks != 1 {
		t.Errorf("store2 should have 1 open task, got %d", sum2.OpenTasks)
	}
}

func TestNewSharedStoreDoesNotFilterByUserID(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	// Insert items for two users into the family list manually.
	store := tracker.NewSharedStore(database, "family")
	items := []tracker.Item{
		{Slug: "task-a", Title: "Task A", Type: tracker.TaskType},
		{Slug: "task-b", Title: "Task B", Type: tracker.TaskType},
	}
	if err := store.ReplaceAll(items); err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	sum, _ := store.Summary()
	if sum.OpenTasks != 2 {
		t.Errorf("shared store should see all 2 tasks, got %d", sum.OpenTasks)
	}
}

func TestSharedStorePreservesAttributionUserID(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	store := tracker.NewSharedStore(database, "family")
	items := []tracker.Item{
		{Slug: "task-a", Title: "Task A", Type: tracker.TaskType},
	}
	// Insert with user_id=42 for attribution.
	if err := store.ReplaceAllWithAttribution(items, 42); err != nil {
		t.Fatalf("ReplaceAllWithAttribution: %v", err)
	}

	// Verify the user_id column value.
	var userID int64
	err = database.QueryRow("SELECT user_id FROM tracker_items WHERE slug = 'task-a'").Scan(&userID)
	if err != nil {
		t.Fatalf("querying user_id: %v", err)
	}
	if userID != 42 {
		t.Errorf("expected user_id=42, got %d", userID)
	}
}

func TestTwoUserStoresDontInterfere(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	store1 := tracker.NewUserStore(database, "personal", 1)
	store2 := tracker.NewUserStore(database, "personal", 2)

	items1 := []tracker.Item{
		{Slug: "task-1", Title: "Task 1", Type: tracker.TaskType},
		{Slug: "task-2", Title: "Task 2", Type: tracker.TaskType},
	}
	items2 := []tracker.Item{
		{Slug: "task-3", Title: "Task 3", Type: tracker.TaskType},
	}

	store1.ReplaceAll(items1)
	store2.ReplaceAll(items2)

	// Now replace store1 with a single item -- should not affect store2.
	store1.ReplaceAll([]tracker.Item{{Slug: "task-4", Title: "Task 4", Type: tracker.TaskType}})

	sum1, _ := store1.Summary()
	sum2, _ := store2.Summary()

	if sum1.OpenTasks != 1 {
		t.Errorf("store1 should have 1 task after replace, got %d", sum1.OpenTasks)
	}
	if sum2.OpenTasks != 1 {
		t.Errorf("store2 should still have 1 task, got %d", sum2.OpenTasks)
	}
}
