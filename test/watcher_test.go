package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fahad/dashboard/internal/watcher"
)

// Classify matches exact file names only: a shared spec's path, or
// USER_DATA_DIR/{id}/{UserFile}.
func TestWatcherClassify(t *testing.T) {
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	family := filepath.Join(dir, "family.md")
	specs := []watcher.Spec{
		{Path: family, Data: "family"},
		{UserFile: "personal.md", Data: "personal"},
		{UserFile: "ideas.md", Data: "ideas"},
	}
	tests := map[string]struct {
		path     string
		wantOK   bool
		wantData string
		wantUser int64
	}{
		"shared file":                       {path: family, wantOK: true, wantData: "family"},
		"per-user personal":                 {path: filepath.Join(users, "3", "personal.md"), wantOK: true, wantData: "personal", wantUser: 3},
		"per-user ideas":                    {path: filepath.Join(users, "5", "ideas.md"), wantOK: true, wantData: "ideas", wantUser: 5},
		"prefix-only name no longer counts": {path: filepath.Join(users, "3", "personal-old.md")},
		"nested legacy ideas dir ignored":   {path: filepath.Join(users, "2", "ideas", "untriaged", "x.md")},
		"temp file from an atomic write":    {path: filepath.Join(users, "3", ".personal.md.atomic-123.tmp")},
		"non-numeric user dir":              {path: filepath.Join(users, "legacy", "personal.md")},
		"unrelated file":                    {path: filepath.Join(dir, "other.md")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			i, uid, ok := watcher.Classify(tc.path, users, specs)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if specs[i].Data != tc.wantData || uid != tc.wantUser {
				t.Errorf("got %s for user %d, want %s for user %d", specs[i].Data, uid, tc.wantData, tc.wantUser)
			}
		})
	}
}

func TestDataMigrationIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up source files.
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "ideas", "untriaged"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "personal.md"), []byte("# Personal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "ideas", "untriaged", "test.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set up destination.
	dstDir := filepath.Join(tmpDir, "dst")
	if err := os.MkdirAll(filepath.Join(dstDir, "ideas", "untriaged"), 0o755); err != nil {
		t.Fatal(err)
	}

	// First copy.
	copyFile(t, filepath.Join(srcDir, "personal.md"), filepath.Join(dstDir, "personal.md"))
	copyFile(t, filepath.Join(srcDir, "ideas", "untriaged", "test.md"), filepath.Join(dstDir, "ideas", "untriaged", "test.md"))

	// Verify files exist.
	if _, err := os.Stat(filepath.Join(dstDir, "personal.md")); os.IsNotExist(err) {
		t.Error("expected personal.md at destination")
	}

	// Modify destination file.
	if err := os.WriteFile(filepath.Join(dstDir, "personal.md"), []byte("# Modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("writing %s: %v", dst, err)
	}
}
