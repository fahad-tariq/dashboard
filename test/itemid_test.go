package test

import (
	"regexp"
	"slices"
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

// The route pattern accepts exactly the alphabet's characters, so every ID
// the generator makes is routable and nothing else is.
func TestItemIDRouteMatchesAlphabet(t *testing.T) {
	class := regexp.MustCompile(`\{id:(\[[^\]]+\])\{8\}\}`).FindStringSubmatch(itemid.Route)
	if class == nil {
		t.Fatalf("Route %q is not {id:[...]{8}}", itemid.Route)
	}
	re := regexp.MustCompile("^" + class[1] + "$")
	for c := 0; c < 128; c++ {
		ch := string(rune(c))
		if got, want := re.MatchString(ch), strings.Contains(itemid.Alphabet, ch); got != want {
			t.Errorf("%q: route accepts %v, alphabet holds %v", ch, got, want)
		}
	}
}

func TestItemIDParseList(t *testing.T) {
	cases := map[string]struct {
		in   string
		want []string
	}{
		"one":              {"b7k2m9xq", []string{"b7k2m9xq"}},
		"two with spaces":  {" b7k2m9xq , t9d3fgh2 ", []string{"b7k2m9xq", "t9d3fgh2"}},
		"empty":            {"", nil},
		"one invalid":      {"b7k2m9xq,pay-rego", nil},
		"tag injection":    {"b7k2m9xq] [tags: x", nil},
		"blank entries ok": {"b7k2m9xq,,", []string{"b7k2m9xq"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := itemid.ParseList(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("ParseList(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
