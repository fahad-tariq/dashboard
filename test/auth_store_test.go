package test

import (
	"bytes"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/db"
)

func newTestStore(t *testing.T) *auth.SQLiteStore {
	t.Helper()
	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return auth.NewSQLiteStore(database)
}

func TestStoreRoundTrip(t *testing.T) {
	s := newTestStore(t)
	data := []byte("session-data")
	expiry := time.Now().Add(time.Hour)

	if err := s.Commit("tok1", data, expiry); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, found, err := s.Find("tok1")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if !bytes.Equal(got, data) {
		t.Errorf("data mismatch: got %q, want %q", got, data)
	}
}

func TestStoreExpired(t *testing.T) {
	s := newTestStore(t)
	expiry := time.Now().Add(-time.Hour)

	if err := s.Commit("expired", []byte("old"), expiry); err != nil {
		t.Fatalf("commit: %v", err)
	}

	_, found, err := s.Find("expired")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found {
		t.Error("expected found=false for expired token")
	}
}

func TestStoreDelete(t *testing.T) {
	s := newTestStore(t)
	expiry := time.Now().Add(time.Hour)

	if err := s.Commit("tok2", []byte("data"), expiry); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := s.Delete("tok2"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, found, err := s.Find("tok2")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found {
		t.Error("expected found=false after delete")
	}
}

func TestStoreDeleteNonExistent(t *testing.T) {
	s := newTestStore(t)
	if err := s.Delete("nonexistent"); err != nil {
		t.Fatalf("delete non-existent should not error: %v", err)
	}
}

func TestStoreCleanupExpired(t *testing.T) {
	s := newTestStore(t)

	if err := s.Commit("valid", []byte("keep"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("commit valid: %v", err)
	}
	if err := s.Commit("stale", []byte("remove"), time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("commit stale: %v", err)
	}

	if err := s.CleanupExpired(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	_, found, _ := s.Find("valid")
	if !found {
		t.Error("valid session should still exist")
	}

	_, found, _ = s.Find("stale")
	if found {
		t.Error("stale session should have been cleaned up")
	}
}

func TestStoreUpsert(t *testing.T) {
	s := newTestStore(t)
	expiry := time.Now().Add(time.Hour)

	if err := s.Commit("tok3", []byte("v1"), expiry); err != nil {
		t.Fatalf("commit v1: %v", err)
	}
	if err := s.Commit("tok3", []byte("v2"), expiry); err != nil {
		t.Fatalf("commit v2: %v", err)
	}

	got, found, err := s.Find("tok3")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if !bytes.Equal(got, []byte("v2")) {
		t.Errorf("expected v2, got %q", got)
	}
}
