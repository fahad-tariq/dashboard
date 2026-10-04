package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// pragmas go in the DSN so the driver applies them to every pooled
// connection; PRAGMA via db.Exec only reaches whichever connection ran it.
const pragmas = "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)" +
	"&_pragma=synchronous(NORMAL)&_pragma=cache_size(-8000)"

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+pragmas)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// Surface pragma errors (e.g. journal_mode on a read-only directory) now
	// rather than on first use.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("opening database %s: %w", path, err)
	}

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	// SQLite silently opens a file it cannot write read-only, so a wrong owner
	// after a deploy would serve pages and fail every save. A no-op write
	// surfaces that at startup instead.
	if _, err := db.Exec("UPDATE schema_version SET version = version"); err != nil {
		db.Close()
		if strings.Contains(err.Error(), "readonly") {
			return nil, fmt.Errorf("database %s is not writable (does its owner match the user the server runs as?): %w", path, err)
		}
		return nil, fmt.Errorf("database %s write check failed: %w", path, err)
	}

	return db, nil
}
