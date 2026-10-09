package test

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/tracker"
)

// itemsByID parses a tracker file and maps each item's ID to the item.
func itemsByID(t *testing.T, path string) map[string]tracker.Item {
	t.Helper()
	items, err := tracker.ParseTracker(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]tracker.Item{}
	for _, it := range items {
		out[it.ID] = it
	}
	return out
}

// Two tasks with the same title are completed, edited and trashed
// independently: every action reaches the item its ID names.
func TestDuplicateTitlesAreAddressedIndependently(t *testing.T) {
	const first, second = "b7k2m9xq", "w3th3rs7"
	cases := map[string]struct {
		path  string
		form  url.Values
		check func(t *testing.T, items map[string]tracker.Item)
	}{
		"complete the second": {
			path: "/todos/" + second + "/complete",
			check: func(t *testing.T, items map[string]tracker.Item) {
				if items[first].Done || !items[second].Done {
					t.Errorf("done: first %v, second %v; want only the second", items[first].Done, items[second].Done)
				}
			},
		},
		"edit the second": {
			path: "/todos/" + second + "/edit",
			form: url.Values{"title": {"Pay rego for the trailer"}, "body": {""}, "tags": {""}},
			check: func(t *testing.T, items map[string]tracker.Item) {
				if items[first].Title != "Pay rego" || items[second].Title != "Pay rego for the trailer" {
					t.Errorf("titles: first %q, second %q", items[first].Title, items[second].Title)
				}
			},
		},
		"trash the second": {
			path: "/todos/" + second + "/delete",
			check: func(t *testing.T, items map[string]tracker.Item) {
				if items[first].DeletedAt != "" || items[second].DeletedAt == "" {
					t.Errorf("deleted: first %q, second %q; want only the second", items[first].DeletedAt, items[second].DeletedAt)
				}
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var personal string
			env := newSeededAppEnv(t, func(cfg *config.Config) {
				personal = filepath.Join(cfg.UserDataDir, "1", "personal.md")
				seedFile(t, personal, "# Personal\n\n- [ ] Pay rego [id: "+first+"]\n- [ ] Pay rego [id: "+second+"]\n")
			})
			if rr := env.post(t, tc.path, tc.form); rr.Code != 303 {
				t.Fatalf("POST %s: status %d: %s", tc.path, rr.Code, rr.Body.String())
			}
			tc.check(t, itemsByID(t, personal))
		})
	}
}

// Renaming a task keeps its address: the row id and every action URL stay
// the same, so the next action on the old URL still reaches it.
func TestRenameKeepsTheItemURL(t *testing.T) {
	const id = "b7k2m9xq"
	var personal string
	env := newSeededAppEnv(t, func(cfg *config.Config) {
		personal = filepath.Join(cfg.UserDataDir, "1", "personal.md")
		seedFile(t, personal, "# Personal\n\n- [ ] Pay rego [id: "+id+"]\n")
	})

	form := url.Values{"title": {"Renew the car rego"}, "body": {""}, "tags": {""}}
	if rr := env.post(t, "/todos/"+id+"/edit", form); rr.Code != 303 {
		t.Fatalf("edit: status %d", rr.Code)
	}
	page := env.get(t, "/todos")
	for _, want := range []string{`id="item-` + id + `"`, `action="/todos/` + id + `/complete"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page after rename lacks %s", want)
		}
	}
	if rr := env.post(t, "/todos/"+id+"/complete", nil); rr.Code != 303 {
		t.Fatalf("complete after rename: status %d", rr.Code)
	}
	if it := itemsByID(t, personal)[id]; !it.Done || it.Title != "Renew the car rego" {
		t.Errorf("item after rename and complete: %+v", it)
	}
}
