package tracker

import (
	"database/sql"
	"encoding/json"
	"strings"
)

// Store provides SQLite caching for tracker items.
type Store struct {
	db       *sql.DB
	listName string
}

// NewStore creates a tracker store backed by the given database.
// listName scopes all operations to a specific list (e.g. "personal", "family").
func NewStore(db *sql.DB, listName string) *Store {
	return &Store{db: db, listName: listName}
}

// ReplaceAll deletes all cached items for this list and inserts the given set in a transaction.
func (s *Store) ReplaceAll(items []Item) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM tracker_items WHERE list = ?", s.listName); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO tracker_items (slug, title, type, category, priority, current_val, target_val, unit, done, graduated, added, completed, tags, images, list)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, it := range items {
		tagsJSON, _ := json.Marshal(it.Tags)
		_, err := stmt.Exec(it.Slug, it.Title, string(it.Type), strings.Join(it.Tags, ","), it.Priority,
			it.Current, it.Target, it.Unit, it.Done, it.Graduated, it.Added, it.Completed, string(tagsJSON),
			strings.Join(it.Images, ","), s.listName)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Summary returns aggregate counts for this list's tracker stats row.
func (s *Store) Summary() (Summary, error) {
	var sum Summary
	err := s.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN type = 'task' AND done = 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN type = 'goal' AND done = 0 THEN 1 ELSE 0 END), 0)
		FROM tracker_items
		WHERE list = ?
	`, s.listName).Scan(&sum.OpenTasks, &sum.ActiveGoals)
	return sum, err
}
