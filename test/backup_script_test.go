package test

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/db"
)

// readTarGz returns the regular files in a .tar.gz, keyed by path.
func readTarGz(t *testing.T, path string) map[string][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		files[strings.TrimPrefix(h.Name, "./")] = b
	}
	return files
}

// The backup must capture every markdown file, uploads, and a consistent
// copy of the live database, never the raw -wal/-shm files.
func TestBackupScriptCapturesEverything(t *testing.T) {
	for _, bin := range []string{"bash", "tar", "python3"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	root := t.TempDir()
	data := filepath.Join(root, "data")
	users := filepath.Join(root, "users")
	backups := filepath.Join(root, "backups")
	write := func(p, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(data, "family.md"), "# Family\n\n- [ ] Book the dentist\n")
	write(filepath.Join(data, "uploads", "abc.png"), "png-bytes")
	write(filepath.Join(users, "1", "personal.md"), "# Personal\n\n- [ ] Renew passport\n")
	write(filepath.Join(users, "1", "ideas.md"), "# Ideas\n\n## Untriaged\n")

	// Keep the database open so -wal/-shm exist, as on a running server.
	live, err := db.Open(filepath.Join(data, "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeDB(t, live) })
	if _, err := auth.CreateUser(live, "owner@example.com", "", "correct-horse"); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", "scripts/backup.sh")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "DATA_DIR="+data, "USERS_DIR="+users, "BACKUP_DIR="+backups, "RETENTION_DAYS=7")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("backup.sh failed: %v\n%s", err, out)
	}

	archives, err := filepath.Glob(filepath.Join(backups, "dashboard-backup-*.tar.gz"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("want one archive, got %v (%v)\n%s", archives, err, out)
	}
	// The logged size must be the file's byte count: du reports allocated
	// blocks, which delayed allocation leaves near zero right after a write.
	info, err := os.Stat(archives[0])
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("(%d bytes)", info.Size()); !strings.Contains(string(out), want) {
		t.Errorf("log does not report %s:\n%s", want, out)
	}

	files := readTarGz(t, archives[0])
	var names []string
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, want := range []string{"data/family.md", "data/uploads/abc.png", "users/1/personal.md", "users/1/ideas.md", "dashboard.db"} {
		if _, ok := files[want]; !ok {
			t.Errorf("archive lacks %s; has %v", want, names)
		}
	}
	for _, n := range names {
		if strings.HasPrefix(n, "data/dashboard.db") {
			t.Errorf("archive contains live database file %s; want only the snapshot", n)
		}
	}

	snap := filepath.Join(root, "restored.db")
	if err := os.WriteFile(snap, files["dashboard.db"], 0o644); err != nil {
		t.Fatal(err)
	}
	restored, err := sql.Open("sqlite", snap)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var n int
	if err := restored.QueryRow("SELECT COUNT(*) FROM users").Scan(&n); err != nil || n != 1 {
		t.Errorf("snapshot users = %d (%v), want 1", n, err)
	}
}

func TestBackupScriptFailsLoudly(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	root := t.TempDir()
	cmd := exec.Command("bash", "scripts/backup.sh")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "DATA_DIR="+filepath.Join(root, "missing"), "USERS_DIR="+filepath.Join(root, "missing-users"), "BACKUP_DIR="+filepath.Join(root, "backups"))
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("backup.sh succeeded with missing data directories:\n%s", out)
	}
}
