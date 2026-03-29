package commentary

import (
	"database/sql"
	"strings"
	"time"
)

// Entry represents a single commentary record for an item.
type Entry struct {
	ItemSlug  string
	ItemList  string
	UserID    int
	Content   string
	UpdatedAt string
}

// Store manages commentary records in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a commentary store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Get retrieves commentary for a single item. Returns empty string if none exists.
func (s *Store) Get(slug, list string, userID int) (string, error) {
	var content string
	err := s.db.QueryRow(
		"SELECT content FROM commentary WHERE item_slug = ? AND item_list = ? AND user_id = ?",
		slug, list, userID,
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
func (s *Store) Set(slug, list string, userID int, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO commentary (item_slug, item_list, user_id, content, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (item_slug, item_list, user_id)
		 DO UPDATE SET content = excluded.content, updated_at = excluded.updated_at`,
		slug, list, userID, content, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// Delete removes commentary for an item.
func (s *Store) Delete(slug, list string, userID int) error {
	_, err := s.db.Exec(
		"DELETE FROM commentary WHERE item_slug = ? AND item_list = ? AND user_id = ?",
		slug, list, userID,
	)
	return err
}

// ListForSlugs retrieves commentary for multiple items in a single list.
// Returns a map of slug -> content. Slugs without commentary are omitted.
func (s *Store) ListForSlugs(slugs []string, list string, userID int) (map[string]string, error) {
	if len(slugs) == 0 {
		return nil, nil
	}

	query, args := inClause(
		"SELECT item_slug, content FROM commentary WHERE item_list = ? AND user_id = ? AND item_slug IN (",
		list, userID, slugs,
	)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var slug, content string
		if err := rows.Scan(&slug, &content); err != nil {
			return nil, err
		}
		result[slug] = content
	}
	return result, rows.Err()
}

// HasCommentary returns a set of slugs (from the provided list) that have
// commentary. Useful for showing indicators without fetching full content.
func (s *Store) HasCommentary(slugs []string, list string, userID int) (map[string]bool, error) {
	if len(slugs) == 0 {
		return nil, nil
	}

	query, args := inClause(
		"SELECT item_slug FROM commentary WHERE item_list = ? AND user_id = ? AND item_slug IN (",
		list, userID, slugs,
	)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		result[slug] = true
	}
	return result, rows.Err()
}

// inClause builds a parameterised IN query with the given prefix, list/userID
// fixed args, and slug values.
func inClause(prefix, list string, userID int, slugs []string) (string, []any) {
	args := make([]any, 0, 2+len(slugs))
	args = append(args, list, userID)
	placeholders := strings.Repeat(",?", len(slugs))[1:] // "?,?,?"
	for _, slug := range slugs {
		args = append(args, slug)
	}
	return prefix + placeholders + ")", args
}
