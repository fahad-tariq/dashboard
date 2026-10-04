package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA cache_size=-8000", // 8MB
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
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
