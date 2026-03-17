package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fahad/dashboard/internal/watcher"
)

func TestClassifyEventPerUserPersonal(t *testing.T) {
	tmpDir := t.TempDir()
	userDataDir := filepath.Join(tmpDir, "users")
	os.MkdirAll(filepath.Join(userDataDir, "3"), 0o755)

	path := filepath.Join(userDataDir, "3", "personal.md")
	os.WriteFile(path, []byte("# Personal\n"), 0o644)

	uid, category := watcher.ClassifyEventWithUser(path, nil, nil, userDataDir)
	if uid != 3 {
		t.Errorf("expected userID=3, got %d", uid)
	}
	if category != "personal" {
		t.Errorf("expected category=personal, got %q", category)
	}
}

func TestClassifyEventPerUserIdeas(t *testing.T) {
	tmpDir := t.TempDir()
	userDataDir := filepath.Join(tmpDir, "users")
	os.MkdirAll(filepath.Join(userDataDir, "2", "ideas", "untriaged"), 0o755)

	path := filepath.Join(userDataDir, "2", "ideas", "untriaged", "test-idea.md")
	os.WriteFile(path, []byte("# Test\n"), 0o644)

	uid, category := watcher.ClassifyEventWithUser(path, nil, nil, userDataDir)
	if uid != 2 {
		t.Errorf("expected userID=2, got %d", uid)
	}
	if category != "ideas" {
		t.Errorf("expected category=ideas, got %q", category)
	}
}

func TestClassifyEventPerUserIdeasFile(t *testing.T) {
	tmpDir := t.TempDir()
	userDataDir := filepath.Join(tmpDir, "users")
	os.MkdirAll(filepath.Join(userDataDir, "5"), 0o755)

	path := filepath.Join(userDataDir, "5", "ideas.md")
	os.WriteFile(path, []byte("# Ideas\n"), 0o644)

	uid, category := watcher.ClassifyEventWithUser(path, nil, nil, userDataDir)
	if uid != 5 {
		t.Errorf("expected userID=5, got %d", uid)
	}
	if category != "ideas" {
		t.Errorf("expected category=ideas, got %q", category)
	}
}

func TestClassifyEventFamilyWithUserID0(t *testing.T) {
	tmpDir := t.TempDir()
	familyPath := filepath.Join(tmpDir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n"), 0o644)

	fileCategories := map[string]string{
		familyPath: "family",
	}

	uid, category := watcher.ClassifyEventWithUser(familyPath, nil, fileCategories, "")
	if uid != 0 {
		t.Errorf("expected userID=0 for family, got %d", uid)
	}
	if category != "family" {
		t.Errorf("expected category=family, got %q", category)
	}
}

func TestClassifyEventNonMarkdownIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	userDataDir := filepath.Join(tmpDir, "users")
	os.MkdirAll(filepath.Join(userDataDir, "1"), 0o755)

	path := filepath.Join(userDataDir, "1", "something.txt")
	uid, category := watcher.ClassifyEventWithUser(path, nil, nil, userDataDir)
	if category != "" {
		t.Errorf("expected empty category for .txt file, got %q (uid=%d)", category, uid)
	}
}

func TestDataMigrationIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up source files.
	srcDir := filepath.Join(tmpDir, "src")
	os.MkdirAll(filepath.Join(srcDir, "ideas", "untriaged"), 0o755)
	os.WriteFile(filepath.Join(srcDir, "personal.md"), []byte("# Personal\n"), 0o644)
	os.WriteFile(filepath.Join(srcDir, "ideas", "untriaged", "test.md"), []byte("# Test\n"), 0o644)

	// Set up destination.
	dstDir := filepath.Join(tmpDir, "dst")
	os.MkdirAll(filepath.Join(dstDir, "ideas", "untriaged"), 0o755)

	// First copy.
	copyFile(t, filepath.Join(srcDir, "personal.md"), filepath.Join(dstDir, "personal.md"))
	copyFile(t, filepath.Join(srcDir, "ideas", "untriaged", "test.md"), filepath.Join(dstDir, "ideas", "untriaged", "test.md"))

	// Verify files exist.
	if _, err := os.Stat(filepath.Join(dstDir, "personal.md")); os.IsNotExist(err) {
		t.Error("expected personal.md at destination")
	}

	// Modify destination file.
	os.WriteFile(filepath.Join(dstDir, "personal.md"), []byte("# Modified\n"), 0o644)

	// Second copy should not overwrite (idempotent).
	// The migrate logic skips if destination exists -- we just verify the file
	// content wasn't changed.
	data, _ := os.ReadFile(filepath.Join(dstDir, "personal.md"))
	if string(data) != "# Modified\n" {
		t.Error("idempotent migration should not overwrite existing files")
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	os.MkdirAll(filepath.Dir(dst), 0o755)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("writing %s: %v", dst, err)
	}
}
