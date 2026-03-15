package db

import (
	"database/sql"
	"fmt"
)

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS projects (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		slug        TEXT NOT NULL UNIQUE,
		path        TEXT NOT NULL,
		status      TEXT NOT NULL DEFAULT 'active'
		            CHECK (status IN ('active', 'paused', 'archived')),
		tags        TEXT,
		last_commit TEXT,
		created     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime')),
		updated     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime'))
	)`,
	`CREATE TABLE IF NOT EXISTS sessions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id  INTEGER REFERENCES projects(id),
		session_id  TEXT,
		description TEXT,
		created     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime'))
	)`,
	`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS tracker_items (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		slug        TEXT NOT NULL,
		title       TEXT NOT NULL,
		type        TEXT NOT NULL CHECK (type IN ('task', 'goal')),
		category    TEXT NOT NULL DEFAULT '',
		priority    TEXT NOT NULL DEFAULT '',
		current_val REAL NOT NULL DEFAULT 0,
		target_val  REAL NOT NULL DEFAULT 0,
		unit        TEXT NOT NULL DEFAULT '',
		done        INTEGER NOT NULL DEFAULT 0,
		graduated   INTEGER NOT NULL DEFAULT 0
	)`,
	`ALTER TABLE tracker_items ADD COLUMN added TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE tracker_items ADD COLUMN completed TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE tracker_items ADD COLUMN tags TEXT NOT NULL DEFAULT '[]'`,
}

func Migrate(db *sql.DB) error {
	current := currentVersion(db)

	for i := current; i < len(migrations); i++ {
		if _, err := db.Exec(migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i, err)
		}
	}

	if current < len(migrations) {
		if _, err := db.Exec("DELETE FROM schema_version"); err != nil {
			return err
		}
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", len(migrations)); err != nil {
			return err
		}
	}

	return nil
}

func currentVersion(db *sql.DB) int {
	var v int
	err := db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&v)
	if err != nil {
		return 0
	}
	return v
}
