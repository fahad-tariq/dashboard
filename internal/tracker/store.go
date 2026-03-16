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
	userID   int64
	shared   bool
}

// NewUserStore creates a tracker store scoped to a specific user and list.
// All queries filter by both list AND user_id.
func NewUserStore(db *sql.DB, listName string, userID int64) *Store {
	return &Store{db: db, listName: listName, userID: userID, shared: false}
}

// NewSharedStore creates a tracker store for the shared family list.
// Queries filter by list only, never by user_id.
func NewSharedStore(db *sql.DB, listName string) *Store {
	return &Store{db: db, listName: listName, shared: true}
}

// NewStore creates a tracker store scoped by list name (legacy constructor).
// Equivalent to NewUserStore with userID=1 for backwards compatibility.
func NewStore(db *sql.DB, listName string) *Store {
	return NewUserStore(db, listName, 1)
}

// ReplaceAll deletes all cached items for this store's scope and inserts the given set.
// For shared stores, attributionUserID is used for the user_id column on insert.
func (s *Store) ReplaceAll(items []Item) error {
	return s.ReplaceAllWithAttribution(items, s.userID)
}

// ReplaceAllWithAttribution deletes all cached items for this store's scope
// and inserts the given set with the specified user_id for attribution.
func (s *Store) ReplaceAllWithAttribution(items []Item, attributionUserID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if s.shared {
		if _, err := tx.Exec("DELETE FROM tracker_items WHERE list = ?", s.listName); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec("DELETE FROM tracker_items WHERE list = ? AND user_id = ?", s.listName, s.userID); err != nil {
			return err
		}
	}

	stmt, err := tx.Prepare(`
		INSERT INTO tracker_items (slug, title, type, category, priority, current_val, target_val, unit, done, graduated, added, completed, tags, images, list, user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, it := range items {
		tagsJSON, _ := json.Marshal(it.Tags)
		_, err := stmt.Exec(it.Slug, it.Title, string(it.Type), strings.Join(it.Tags, ","), it.Priority,
			it.Current, it.Target, it.Unit, it.Done, it.Graduated, it.Added, it.Completed, string(tagsJSON),
			strings.Join(it.Images, ","), s.listName, attributionUserID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Summary returns aggregate counts for this store's scope.
func (s *Store) Summary() (Summary, error) {
	var sum Summary
	var err error

	if s.shared {
		err = s.db.QueryRow(`
			SELECT
				COALESCE(SUM(CASE WHEN type = 'task' AND done = 0 THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN type = 'goal' AND done = 0 THEN 1 ELSE 0 END), 0)
			FROM tracker_items
			WHERE list = ?
		`, s.listName).Scan(&sum.OpenTasks, &sum.ActiveGoals)
	} else {
		err = s.db.QueryRow(`
			SELECT
				COALESCE(SUM(CASE WHEN type = 'task' AND done = 0 THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN type = 'goal' AND done = 0 THEN 1 ELSE 0 END), 0)
			FROM tracker_items
			WHERE list = ? AND user_id = ?
		`, s.listName, s.userID).Scan(&sum.OpenTasks, &sum.ActiveGoals)
	}

	return sum, err
}
