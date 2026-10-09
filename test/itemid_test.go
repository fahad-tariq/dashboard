package test

import (
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/itemid"
)

func TestItemIDNew(t *testing.T) {
	seen := map[string]bool{}
	for range 2000 {
		id := itemid.New()
		if !itemid.Valid(id) {
			t.Fatalf("New() = %q, not valid", id)
		}
		if strings.ContainsAny(id, "aeiouy") {
			t.Fatalf("New() = %q holds a vowel", id)
		}
		if seen[id] {
			t.Fatalf("New() repeated %q within 2000 draws", id)
		}
		seen[id] = true
	}
}

// Assign fills missing and malformed IDs, keeps the first of a repeated ID
// and gives later copies new ones, never reusing an ID held further down.
func TestItemIDAssign(t *testing.T) {
	type item struct{ ID string }
	cases := map[string]struct {
		in          []string
		wantChanged bool
		keep        map[int]string // index -> ID that must be kept
	}{
		"all valid and unique":  {[]string{"b7k2m9xq", "t9d3fgh2"}, false, map[int]string{0: "b7k2m9xq", 1: "t9d3fgh2"}},
		"missing":               {[]string{"", "t9d3fgh2"}, true, map[int]string{1: "t9d3fgh2"}},
		"malformed":             {[]string{"abc", "t9d3fgh2"}, true, map[int]string{1: "t9d3fgh2"}},
		"duplicate: first wins": {[]string{"b7k2m9xq", "t9d3fgh2", "b7k2m9xq"}, true, map[int]string{0: "b7k2m9xq", 1: "t9d3fgh2"}},
		"empty list":            {nil, false, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			items := make([]item, len(tc.in))
			for i, id := range tc.in {
				items[i].ID = id
			}
			changed := itemid.Assign(items, func(it *item) *string { return &it.ID })
			if changed != tc.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tc.wantChanged)
			}
			seen := map[string]bool{}
			for i, it := range items {
				if !itemid.Valid(it.ID) || seen[it.ID] {
					t.Errorf("item %d has ID %q: invalid or repeated", i, it.ID)
				}
				seen[it.ID] = true
				if want, ok := tc.keep[i]; ok && it.ID != want {
					t.Errorf("item %d ID %q, want %q kept", i, it.ID, want)
				}
			}
		})
	}
}
