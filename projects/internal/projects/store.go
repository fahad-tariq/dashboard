package projects

import (
	"database/sql"
	"fmt"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) UpdateStatus(slug, status string) error {
	res, err := s.db.Exec(`
		UPDATE projects
		SET status = ?, updated = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime')
		WHERE slug = ?
	`, status, slug)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %q not found", slug)
	}
	return nil
}

