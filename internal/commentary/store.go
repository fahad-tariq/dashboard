package commentary

import (
	"database/sql"
	"log/slog"
	"strings"
	"time"
)

// Store manages commentary records in SQLite, keyed by item ID and user.
// The ID is not scoped to a list, so a moved item keeps its commentary.
type Store struct {
	db *sql.DB
}

// NewStore creates a commentary store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Get retrieves commentary for a single item. Returns empty string if none exists.
func (s *Store) Get(itemID string, userID int) (string, error) {
	var content string
	err := s.db.QueryRow(
		"SELECT content FROM item_commentary WHERE item_id = ? AND user_id = ?",
		itemID, userID,
	).Scan(&content)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return content, nil
}

// Set creates or replaces commentary for an item.
func (s *Store) Set(itemID string, userID int, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO item_commentary (item_id, user_id, content, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (item_id, user_id)
		 DO UPDATE SET content = excluded.content, updated_at = excluded.updated_at`,
		itemID, userID, content, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// Delete removes one user's commentary for an item.
func (s *Store) Delete(itemID string, userID int) error {
	_, err := s.db.Exec(
		"DELETE FROM item_commentary WHERE item_id = ? AND user_id = ?",
		itemID, userID,
	)
	return err
}

// DeleteItems removes every user's commentary for the given items, once
// they are permanently deleted.
func (s *Store) DeleteItems(itemIDs ...string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	args := make([]any, len(itemIDs))
	for i, id := range itemIDs {
		args[i] = id
	}
	placeholders := strings.Repeat(",?", len(itemIDs))[1:]
	_, err := s.db.Exec("DELETE FROM item_commentary WHERE item_id IN ("+placeholders+")", args...) //nolint:gosec // G202: only placeholders are concatenated
	return err
}

// Copy duplicates every user's commentary on item from onto item to, for a
// moved item that had to take a new ID.
func (s *Store) Copy(from, to string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO item_commentary (item_id, user_id, content, updated_at)
		 SELECT ?, user_id, content, updated_at FROM item_commentary WHERE item_id = ?`,
		to, from,
	)
	return err
}

// ForgetItems deletes the commentary of permanently deleted items. A failure
// is logged, not returned: the items are already gone, so the request that
// removed them has succeeded. A nil store does nothing.
func (s *Store) ForgetItems(itemIDs ...string) {
	if s == nil {
		return
	}
	if err := s.DeleteItems(itemIDs...); err != nil {
		slog.Error("deleting commentary of removed items", "count", len(itemIDs), "error", err)
	}
}
