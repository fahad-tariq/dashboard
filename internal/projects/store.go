package projects

import (
	"database/sql"
	"encoding/json"
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

func (s *Store) UpdateTags(slug string, tags []string) error {
	data, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`
		UPDATE projects
		SET tags = ?, updated = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime')
		WHERE slug = ?
	`, string(data), slug)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project %q not found", slug)
	}
	return nil
}

func (s *Store) UpdateLastCommit(slug, isoDate string) error {
	_, err := s.db.Exec(`
		UPDATE projects
		SET last_commit = ?, updated = strftime('%Y-%m-%dT%H:%M:%S', 'now', 'localtime')
		WHERE slug = ?
	`, isoDate, slug)
	return err
}
