package db

import (
	"database/sql"
	"fmt"
)

var migrations = []string{
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
		graduated   INTEGER NOT NULL DEFAULT 0,
		added       TEXT NOT NULL DEFAULT '',
		completed   TEXT NOT NULL DEFAULT '',
		tags        TEXT NOT NULL DEFAULT '[]'
	)`,
	`ALTER TABLE tracker_items ADD COLUMN images TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE tracker_items ADD COLUMN list TEXT NOT NULL DEFAULT 'personal'`,
	`CREATE TABLE IF NOT EXISTS sessions (
		token  TEXT PRIMARY KEY,
		data   BLOB NOT NULL,
		expiry REAL NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expiry)`,
	`CREATE TABLE IF NOT EXISTS users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		email         TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		created_at    TEXT NOT NULL
	)`,
	`ALTER TABLE tracker_items ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user'`,
	`UPDATE users SET role = 'admin' WHERE id = (SELECT MIN(id) FROM users)`,
	`ALTER TABLE sessions ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE users ADD COLUMN first_name TEXT NOT NULL DEFAULT ''`,
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
