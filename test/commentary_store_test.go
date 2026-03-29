package test

import (
	"path/filepath"
	"testing"

	"github.com/fahad/dashboard/internal/commentary"
	dbpkg "github.com/fahad/dashboard/internal/db"
)

func setupCommentaryStore(t *testing.T) *commentary.Store {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := dbpkg.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return commentary.NewStore(database)
}

func TestCommentaryStore_SetAndGet(t *testing.T) {
	store := setupCommentaryStore(t)

	content := "This task has been open for a week."
	if err := store.Set("fix-bug", "personal", 1, content); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := store.Get("fix-bug", "personal", 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != content {
		t.Errorf("Get = %q, want %q", got, content)
	}
}

func TestCommentaryStore_GetEmpty(t *testing.T) {
	store := setupCommentaryStore(t)

	got, err := store.Get("nonexistent", "personal", 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "" {
		t.Errorf("Get = %q, want empty", got)
	}
}

func TestCommentaryStore_SetOverwrites(t *testing.T) {
	store := setupCommentaryStore(t)

	store.Set("task-1", "personal", 1, "first version")
	store.Set("task-1", "personal", 1, "updated version")

	got, _ := store.Get("task-1", "personal", 1)
	if got != "updated version" {
		t.Errorf("Get after overwrite = %q, want %q", got, "updated version")
	}
}

func TestCommentaryStore_Delete(t *testing.T) {
	store := setupCommentaryStore(t)

	store.Set("task-1", "personal", 1, "some content")
	if err := store.Delete("task-1", "personal", 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ := store.Get("task-1", "personal", 1)
	if got != "" {
		t.Errorf("Get after delete = %q, want empty", got)
	}
}

func TestCommentaryStore_DeleteNonexistent(t *testing.T) {
	store := setupCommentaryStore(t)

	if err := store.Delete("nonexistent", "personal", 1); err != nil {
		t.Fatalf("Delete nonexistent: %v", err)
	}
}

func TestCommentaryStore_ScopedByListAndUser(t *testing.T) {
	store := setupCommentaryStore(t)

	store.Set("task-1", "personal", 1, "personal user 1")
	store.Set("task-1", "family", 1, "family user 1")
	store.Set("task-1", "personal", 2, "personal user 2")

	tests := []struct {
		list   string
		userID int
		want   string
	}{
		{"personal", 1, "personal user 1"},
		{"family", 1, "family user 1"},
		{"personal", 2, "personal user 2"},
		{"family", 2, ""},
	}
	for _, tt := range tests {
		got, _ := store.Get("task-1", tt.list, tt.userID)
		if got != tt.want {
			t.Errorf("Get(%q, %q, %d) = %q, want %q", "task-1", tt.list, tt.userID, got, tt.want)
		}
	}
}

func TestCommentaryStore_ListForSlugs(t *testing.T) {
	store := setupCommentaryStore(t)

	store.Set("task-1", "personal", 1, "comment 1")
	store.Set("task-2", "personal", 1, "comment 2")
	store.Set("task-3", "family", 1, "family comment")

	got, err := store.ListForSlugs([]string{"task-1", "task-2", "task-3"}, "personal", 1)
	if err != nil {
		t.Fatalf("ListForSlugs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListForSlugs returned %d items, want 2", len(got))
	}
	if got["task-1"] != "comment 1" {
		t.Errorf("task-1 = %q, want %q", got["task-1"], "comment 1")
	}
	if got["task-2"] != "comment 2" {
		t.Errorf("task-2 = %q, want %q", got["task-2"], "comment 2")
	}
}

func TestCommentaryStore_ListForSlugsEmpty(t *testing.T) {
	store := setupCommentaryStore(t)

	got, err := store.ListForSlugs(nil, "personal", 1)
	if err != nil {
		t.Fatalf("ListForSlugs: %v", err)
	}
	if got != nil {
		t.Errorf("ListForSlugs(nil) = %v, want nil", got)
	}
}

func TestCommentaryStore_HasCommentary(t *testing.T) {
	store := setupCommentaryStore(t)

	store.Set("task-1", "personal", 1, "has commentary")
	store.Set("task-3", "personal", 1, "also has")

	got, err := store.HasCommentary([]string{"task-1", "task-2", "task-3"}, "personal", 1)
	if err != nil {
		t.Fatalf("HasCommentary: %v", err)
	}
	if !got["task-1"] {
		t.Error("task-1 should have commentary")
	}
	if got["task-2"] {
		t.Error("task-2 should not have commentary")
	}
	if !got["task-3"] {
		t.Error("task-3 should have commentary")
	}
}
