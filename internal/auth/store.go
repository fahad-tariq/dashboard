package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"log/slog"
	"time"
)

// SQLiteStore implements scs.CtxStore using modernc.org/sqlite.
// It extracts user_id from the gob-encoded session data during CommitCtx
// to associate sessions with users for targeted invalidation.
type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

// extractUserID decodes gob-encoded session data and returns the user_id
// value if present. Returns 0 if not found or on decode error.
func extractUserID(data []byte) int64 {
	aux := &struct {
		Deadline time.Time
		Values   map[string]interface{}
	}{}
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(aux); err != nil {
		return 0
	}
	if v, ok := aux.Values["user_id"]; ok {
		switch id := v.(type) {
		case int64:
			return id
		case int:
			return int64(id)
		}
	}
	return 0
}

func (s *SQLiteStore) Find(token string) ([]byte, bool, error) {
	return s.FindCtx(context.Background(), token)
}

func (s *SQLiteStore) FindCtx(_ context.Context, token string) ([]byte, bool, error) {
	var data []byte
	err := s.db.QueryRow(
		"SELECT data FROM sessions WHERE token = ? AND expiry > ?",
		token, time.Now().Unix(),
	).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (s *SQLiteStore) Commit(token string, data []byte, expiry time.Time) error {
	return s.CommitCtx(context.Background(), token, data, expiry)
}

func (s *SQLiteStore) CommitCtx(_ context.Context, token string, data []byte, expiry time.Time) error {
	uid := extractUserID(data)
	if uid == 0 {
		slog.Debug("session commit without user_id", "token_prefix", token[:min(8, len(token))])
	}

	_, err := s.db.Exec(
		`INSERT INTO sessions (token, data, expiry, user_id) VALUES (?, ?, ?, ?)
		 ON CONFLICT(token) DO UPDATE SET data = excluded.data, expiry = excluded.expiry, user_id = excluded.user_id`,
		token, data, expiry.Unix(), uid,
	)
	return err
}

func (s *SQLiteStore) Delete(token string) error {
	return s.DeleteCtx(context.Background(), token)
}

func (s *SQLiteStore) DeleteCtx(_ context.Context, token string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

func (s *SQLiteStore) CleanupExpired() error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE expiry < ?", time.Now().Unix())
	return err
}
