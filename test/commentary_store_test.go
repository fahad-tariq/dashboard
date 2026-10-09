package test

import (
	"path/filepath"
	"testing"

	"github.com/fahad/dashboard/internal/commentary"
	dbpkg "github.com/fahad/dashboard/internal/db"
)

func setupCommentaryStore(t *testing.T) *commentary.Store {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := dbpkg.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return commentary.NewStore(database)
}

// commentaryEntry is one Set call in a store test's seed.
type commentaryEntry struct {
	id      string
	user    int
	content string
}

// commentaryWant is one Get the test expects after its operation.
type commentaryWant struct {
	id   string
	user int
	want string
}

func TestCommentaryStore(t *testing.T) {
	const first, second, third = "b7k2m9xq", "t9d3fgh2", "w3th3rs7"
	cases := map[string]struct {
		seed  []commentaryEntry
		op    func(s *commentary.Store) error
		wants []commentaryWant
	}{
		"set and get": {
			seed:  []commentaryEntry{{first, 1, "Open for a week."}},
			wants: []commentaryWant{{first, 1, "Open for a week."}},
		},
		"missing item is empty": {
			wants: []commentaryWant{{first, 1, ""}},
		},
		"set overwrites": {
			seed:  []commentaryEntry{{first, 1, "first version"}, {first, 1, "updated version"}},
			wants: []commentaryWant{{first, 1, "updated version"}},
		},
		"scoped by user": {
			seed:  []commentaryEntry{{first, 1, "user one"}, {first, 2, "user two"}},
			wants: []commentaryWant{{first, 1, "user one"}, {first, 2, "user two"}, {second, 1, ""}},
		},
		"delete removes one user's note": {
			seed:  []commentaryEntry{{first, 1, "user one"}, {first, 2, "user two"}},
			op:    func(s *commentary.Store) error { return s.Delete(first, 1) },
			wants: []commentaryWant{{first, 1, ""}, {first, 2, "user two"}},
		},
		"delete of a missing item is fine": {
			op: func(s *commentary.Store) error { return s.Delete(first, 1) },
		},
		"delete items removes every user's notes": {
			seed: []commentaryEntry{{first, 1, "a"}, {first, 2, "b"}, {second, 1, "c"}, {third, 1, "d"}},
			op:   func(s *commentary.Store) error { return s.DeleteItems(first, second) },
			wants: []commentaryWant{
				{first, 1, ""}, {first, 2, ""}, {second, 1, ""}, {third, 1, "d"},
			},
		},
		"delete items with none is fine": {
			seed:  []commentaryEntry{{first, 1, "a"}},
			op:    func(s *commentary.Store) error { return s.DeleteItems() },
			wants: []commentaryWant{{first, 1, "a"}},
		},
		"forget items deletes": {
			seed: []commentaryEntry{{first, 1, "a"}, {second, 1, "b"}},
			op: func(s *commentary.Store) error {
				s.ForgetItems(first)
				return nil
			},
			wants: []commentaryWant{{first, 1, ""}, {second, 1, "b"}},
		},
		"copy keeps the source and every user": {
			seed: []commentaryEntry{{first, 1, "user one"}, {first, 2, "user two"}},
			op:   func(s *commentary.Store) error { return s.Copy(first, second) },
			wants: []commentaryWant{
				{first, 1, "user one"}, {second, 1, "user one"}, {second, 2, "user two"},
			},
		},
		"copy does not overwrite the target": {
			seed:  []commentaryEntry{{first, 1, "old"}, {second, 1, "kept"}},
			op:    func(s *commentary.Store) error { return s.Copy(first, second) },
			wants: []commentaryWant{{second, 1, "kept"}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := setupCommentaryStore(t)
			for _, e := range tc.seed {
				if err := store.Set(e.id, e.user, e.content); err != nil {
					t.Fatalf("Set: %v", err)
				}
			}
			if tc.op != nil {
				if err := tc.op(store); err != nil {
					t.Fatalf("op: %v", err)
				}
			}
			for _, w := range tc.wants {
				got, err := store.Get(w.id, w.user)
				if err != nil {
					t.Fatalf("Get: %v", err)
				}
				if got != w.want {
					t.Errorf("Get(%s, %d) = %q, want %q", w.id, w.user, got, w.want)
				}
			}
		})
	}
}

// A nil store (no commentary configured) ignores ForgetItems.
func TestCommentaryStoreForgetItemsNil(t *testing.T) {
	var store *commentary.Store
	store.ForgetItems("b7k2m9xq")
}
