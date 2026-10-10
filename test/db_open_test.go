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

// Pragmas set with db.Exec only reach the connection that ran them; every
// pooled connection must carry them.
func TestPragmasOnEveryPooledConnection(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeDB(t, database) })

	want := map[string]string{"busy_timeout": "5000", "foreign_keys": "1", "journal_mode": "wal", "synchronous": "1"}
	var conns []*sql.Conn
	for range 3 {
		c, err := database.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	for i, c := range conns {
		for pragma, v := range want {
			var got string
			if err := c.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&got); err != nil {
				t.Fatalf("conn %d PRAGMA %s: %v", i, pragma, err)
			}
			if got != v {
				t.Errorf("conn %d: %s = %q, want %q", i, pragma, got, v)
			}
		}
	}
	for _, c := range conns {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	}
}

// The slug-keyed commentary table and the old tracker_items mirror are gone
// once migrations run; item_commentary replaces the first.
func TestMigrationsDropDeadTables(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeDB(t, database) })

	tests := map[string]struct {
		name string
		want bool
	}{
		"commentary dropped":    {name: "commentary", want: false},
		"tracker_items dropped": {name: "tracker_items", want: false},
		"list index dropped":    {name: "idx_tracker_items_list_user", want: false},
		"unique index dropped":  {name: "idx_tracker_items_unique", want: false},
		"item_commentary kept":  {name: "item_commentary", want: true},
		"users kept":            {name: "users", want: true},
		"sessions kept":         {name: "sessions", want: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var n int
			if err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name = ?", tc.name).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if got := n == 1; got != tc.want {
				t.Errorf("%s present = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
