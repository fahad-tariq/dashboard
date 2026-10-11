package test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/db"
)

func tokenDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeDB(t, database) })
	return database
}

func TestCreateToken(t *testing.T) {
	tests := map[string]struct {
		name     string
		wantName string
		wantErr  error
	}{
		"plain name":             {name: "iPhone", wantName: "iPhone"},
		"spaces collapsed":       {name: "  Fahad's   iPhone \n", wantName: "Fahad's iPhone"},
		"brackets and markers":   {name: "phone] [planned: x !high", wantName: "phone planned: x"},
		"capped at 64 runes":     {name: strings.Repeat("é", 70), wantName: strings.Repeat("é", 64)},
		"empty":                  {name: "", wantErr: auth.ErrTokenName},
		"empty once cleaned":     {name: " [] !high ", wantErr: auth.ErrTokenName},
		"line breaks are spaces": {name: "a\r\nb", wantName: "a b"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			database := tokenDB(t)
			plain, err := auth.CreateToken(database, 1, tc.name)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			if len(plain) != 64 || strings.Trim(plain, "0123456789abcdef") != "" {
				t.Errorf("plaintext %q is not 64 hex characters", plain)
			}
			var stored string
			if err := database.QueryRow("SELECT token_hash FROM api_tokens").Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored == plain || strings.Contains(stored, plain) {
				t.Error("the plaintext is stored")
			}
			tokens, err := auth.ListTokens(database, 1)
			if err != nil || len(tokens) != 1 || tokens[0].Name != tc.wantName {
				t.Fatalf("ListTokens = %+v, %v; want one named %q", tokens, err, tc.wantName)
			}
		})
	}
}

func TestLookupToken(t *testing.T) {
	database := tokenDB(t)
	phone, err := auth.CreateToken(database, 1, "phone")
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := auth.CreateToken(database, 1, "old phone")
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.CreateToken(database, 2, "someone else")
	if err != nil {
		t.Fatal(err)
	}
	list, _ := auth.ListTokens(database, 1)
	for _, tok := range list {
		if tok.Name == "old phone" {
			if err := auth.RevokeToken(database, tok.ID, 1); err != nil {
				t.Fatal(err)
			}
		}
	}

	tests := map[string]struct {
		plaintext string
		wantName  string // empty: no match
	}{
		"device token":            {plaintext: phone, wantName: "phone"},
		"revoked token":           {plaintext: revoked},
		"unknown token":           {plaintext: strings.Repeat("a", 64)},
		"empty":                   {plaintext: ""},
		"another user's token":    {plaintext: other},
		"a prefix of a token":     {plaintext: phone[:32]},
		"token with extra spaces": {plaintext: phone + " "},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tok, err := auth.LookupToken(database, 1, tc.plaintext)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case tc.wantName == "" && tok != nil:
				t.Errorf("matched %q, want no match", tok.Name)
			case tc.wantName != "" && (tok == nil || tok.Name != tc.wantName):
				t.Errorf("got %+v, want %q", tok, tc.wantName)
			}
		})
	}

	t.Run("a lookup records the last use", func(t *testing.T) {
		list, _ := auth.ListTokens(database, 1)
		if len(list) != 1 || list[0].LastUsedAt.IsZero() {
			t.Errorf("ListTokens = %+v, want phone with a last-used time", list)
		}
	})
}

func TestRevokeToken(t *testing.T) {
	database := tokenDB(t)
	if _, err := auth.CreateToken(database, 1, "phone"); err != nil {
		t.Fatal(err)
	}
	list, _ := auth.ListTokens(database, 1)
	id := list[0].ID

	if err := auth.RevokeToken(database, id, 2); !errors.Is(err, auth.ErrTokenNotFound) {
		t.Errorf("revoking another user's token: err = %v, want ErrTokenNotFound", err)
	}
	if n, _ := auth.CountTokens(database, 1); n != 1 {
		t.Errorf("after a refused revoke, count = %d, want 1", n)
	}
	if err := auth.RevokeToken(database, id, 1); err != nil {
		t.Fatal(err)
	}
	if err := auth.RevokeToken(database, id, 1); !errors.Is(err, auth.ErrTokenNotFound) {
		t.Errorf("revoking twice: err = %v, want ErrTokenNotFound", err)
	}
	if n, _ := auth.CountTokens(database, 1); n != 0 {
		t.Errorf("after revoke, count = %d, want 0", n)
	}
}
