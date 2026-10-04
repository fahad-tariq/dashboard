package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/atomicfile"
	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/house"
	"github.com/fahad/dashboard/internal/ideas"
	"github.com/fahad/dashboard/internal/tracker"
)

// Writing through a symlink must update the real file and keep the link.
func TestAtomicWriteFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.md")
	link := filepath.Join(dir, "personal.md")
	if err := os.WriteFile(real, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.Write(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link replaced by a regular file (%v)", err)
	}
	if got, _ := os.ReadFile(real); string(got) != "new" {
		t.Errorf("real file = %q, want the new content", got)
	}
}

// One unreadable directory must not stop startup.
func TestCleanStaleSkipsUnreadableDirs(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, ".family.md.atomic-1.tmp")
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if err := atomicfile.CleanStale(root); err != nil {
		t.Fatalf("CleanStale failed on an unreadable subdirectory: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale temp file survived")
	}
}

// A file deleted outside the app must still empty the cache and count as a
// change, so open pages refresh.
func TestResyncIfChangedOnDeletedFile(t *testing.T) {
	tests := map[string]struct {
		file, content string
		build         func(string) changeSource
		count         func(changeSource) int
	}{
		"tracker": {"personal.md", "# Personal\n\n- [ ] Task\n",
			func(p string) changeSource { return tracker.NewService(p, "Personal", time.UTC) },
			func(s changeSource) int { items, _ := s.(*tracker.Service).List(); return len(items) }},
		"ideas": {"ideas.md", "# Ideas\n\n- [ ] Idea [status: untriaged]\n",
			func(p string) changeSource { return ideas.NewService(p, time.UTC) },
			func(s changeSource) int { items, _ := s.(*ideas.Service).List(); return len(items) }},
		"maintenance": {"maintenance.md", "# Maintenance\n\n- [ ] Gutters [cadence: 6m]\n",
			func(p string) changeSource { return house.NewService(p, time.UTC) },
			func(s changeSource) int { items, _ := s.(*house.Service).List(); return len(items) }},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.file)
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			svc := tc.build(path)
			if tc.count(svc) != 1 {
				t.Fatalf("setup: want 1 item")
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			changed, err := svc.ResyncIfChanged()
			if err != nil || !changed {
				t.Errorf("ResyncIfChanged after delete = %v (%v), want true, nil", changed, err)
			}
			if n := tc.count(svc); n != 0 {
				t.Errorf("cache still has %d items after the file was deleted", n)
			}
		})
	}
}

// Hand-written files may indent ideas under a heading; with no idea open
// yet, an indented checkbox is still an idea rather than being dropped.
func TestIndentedIdeaBeforeAnyIdeaIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ideas.md")
	content := "# Ideas\n\n## Untriaged\n\n  - [ ] Indented idea [status: untriaged]\n    Body line.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ideas.ParseIdeas(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Indented idea" {
		t.Errorf("parsed %+v, want one idea titled %q", got, "Indented idea")
	}
}

// Relative DATA_DIR and USERS_DIR must resolve from the working directory;
// tar -C options are cumulative, which used to compound them.
func TestBackupScriptRelativeDirs(t *testing.T) {
	for _, bin := range []string{"bash", "tar", "python3"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	root := t.TempDir()
	for p, c := range map[string]string{
		"srv/data/family.md":      "# Family\n",
		"srv/users/1/personal.md": "# Personal\n",
	} {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	live, err := db.Open(filepath.Join(root, "srv/data/dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeDB(t, live) })

	script, _ := filepath.Abs("../scripts/backup.sh")
	cmd := exec.Command("bash", script)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "DATA_DIR=srv/data", "USERS_DIR=srv/users", "BACKUP_DIR=out")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("backup.sh with relative dirs failed: %v\n%s", err, out)
	}
	archives, _ := filepath.Glob(filepath.Join(root, "out", "dashboard-backup-*.tar.gz"))
	if len(archives) != 1 {
		t.Fatalf("want one archive, got %v", archives)
	}
	files := readTarGz(t, archives[0])
	for _, want := range []string{"data/family.md", "users/1/personal.md", "dashboard.db"} {
		if _, ok := files[want]; !ok {
			t.Errorf("archive lacks %s", want)
		}
	}
}
