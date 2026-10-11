package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fahad/dashboard/internal/httputil"
)

// MaxTokenNameLen is the longest device name a token may carry, in runes.
const MaxTokenNameLen = 64

// ErrTokenName is returned for a device name that is empty once cleaned.
var ErrTokenName = errors.New("a device name is required")

// ErrTokenNotFound is returned when revoking a token that does not exist,
// belongs to another user or is already revoked.
var ErrTokenNotFound = errors.New("token not found")

// APIToken is a device token as listed on the account page. The plaintext
// is never stored; only its SHA-256 hash is.
type APIToken struct {
	ID         int64
	UserID     int64
	Name       string
	CreatedAt  time.Time
	LastUsedAt time.Time // zero until first used
}

// CreateToken stores a new token for userID under name and returns the
// plaintext, which cannot be recovered afterwards.
func CreateToken(db *sql.DB, userID int64, name string) (string, error) {
	name = cleanTokenName(name)
	if name == "" {
		return "", ErrTokenName
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	plaintext := hex.EncodeToString(raw)
	if _, err := db.Exec(
		"INSERT INTO api_tokens (user_id, name, token_hash, created_at) VALUES (?, ?, ?, ?)",
		userID, name, hashToken(plaintext), time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return "", fmt.Errorf("inserting token: %w", err)
	}
	return plaintext, nil
}

// LookupToken returns the unrevoked token of userID whose hash matches
// plaintext and records that it was used. It returns nil, nil when none
// matches, including for an empty plaintext.
func LookupToken(db *sql.DB, userID int64, plaintext string) (*APIToken, error) {
	if plaintext == "" {
		return nil, nil
	}
	var t APIToken
	var created string
	err := db.QueryRow(
		"SELECT id, user_id, name, created_at FROM api_tokens WHERE token_hash = ? AND revoked_at = '' AND user_id = ?",
		hashToken(plaintext), userID,
	).Scan(&t.ID, &t.UserID, &t.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("looking up token: %w", err)
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, created)
	t.LastUsedAt = time.Now().UTC()
	if _, err := db.Exec("UPDATE api_tokens SET last_used_at = ? WHERE id = ?", t.LastUsedAt.Format(time.RFC3339), t.ID); err != nil {
		return nil, fmt.Errorf("recording token use: %w", err)
	}
	return &t, nil
}

// ListTokens returns userID's unrevoked tokens, newest first.
func ListTokens(db *sql.DB, userID int64) ([]APIToken, error) {
	rows, err := db.Query(
		"SELECT id, user_id, name, created_at, last_used_at FROM api_tokens WHERE user_id = ? AND revoked_at = '' ORDER BY id DESC",
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing tokens: %w", err)
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		var t APIToken
		var created, used string
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &created, &used); err != nil {
			return nil, fmt.Errorf("scanning token: %w", err)
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, created)
		t.LastUsedAt, _ = time.Parse(time.RFC3339, used)
		out = append(out, t)
	}
	return out, rows.Err()
}

// CountTokens returns how many unrevoked tokens userID holds.
func CountTokens(db *sql.DB, userID int64) (int, error) {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM api_tokens WHERE user_id = ? AND revoked_at = ''", userID).Scan(&n)
	return n, err
}

// RevokeToken revokes token id if it belongs to userID; it stops working at
// once, since every request looks the token up.
func RevokeToken(db *sql.DB, id, userID int64) error {
	res, err := db.Exec(
		"UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at = ''",
		time.Now().UTC().Format(time.RFC3339), id, userID,
	)
	if err != nil {
		return fmt.Errorf("revoking token: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// cleanTokenName makes a device name safe to show and log: one line, no
// tag brackets, at most MaxTokenNameLen runes.
func cleanTokenName(name string) string {
	name = strings.Join(strings.Fields(httputil.CleanMetaValue(name)), " ")
	if utf8.RuneCountInString(name) > MaxTokenNameLen {
		name = strings.TrimSpace(string([]rune(name)[:MaxTokenNameLen]))
	}
	return name
}
