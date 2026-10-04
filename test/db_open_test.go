package test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/db"
)

// A database the process cannot write (wrong owner after a deploy) must stop
// startup. Otherwise pages render and every save fails at runtime.
func TestOpenRefusesReadOnlyDatabase(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	// Copy a live database, -wal and -shm included, the way scp of a running
	// deployment does. With those files present SQLite opens read-only without
	// complaint, which is how the server used to start and then fail on save.
	live := t.TempDir()
	liveDB, err := db.Open(filepath.Join(live, "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	entries, err := os.ReadDir(live)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(live, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o444); err != nil {
			t.Fatal(err)
		}
	}
	closeDB(t, liveDB)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
		for _, e := range entries {
			_ = os.Chmod(filepath.Join(dir, e.Name()), 0o644)
		}
	})
	path := filepath.Join(dir, "dashboard.db")

	var database *sql.DB
	database, err = db.Open(path)
	if err == nil {
		closeDB(t, database)
		t.Fatal("db.Open succeeded on a read-only database; want a startup error")
	}
	if !strings.Contains(err.Error(), "not writable") {
		t.Errorf("error %q should say the database is not writable", err)
	}
}
