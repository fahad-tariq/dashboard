package test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/config"
)

// Commentary is keyed by item ID, so it follows an item through a rename and
// a move, and goes when the item is purged, on every list's purge route.
func TestCommentaryFollowsTheItemID(t *testing.T) {
	const id, note = "b7k2m9xq", "Waiting on the insurer."
	type step struct {
		path string
		form url.Values
	}
	personal := func(cfg *config.Config) {
		seedFile(t, filepath.Join(cfg.UserDataDir, "1", "personal.md"), "# Personal\n\n- [ ] Pay rego [id: "+id+"]\n")
	}
	// readOwn reads the commentary of the item the case started with.
	readOwn := func(list string) func(*testing.T, *config.Config) string {
		return func(*testing.T, *config.Config) string { return "/commentary/" + list + "/" + id }
	}
	cases := map[string]struct {
		seed     func(*config.Config)
		list     string // the list the commentary is written under
		steps    []step
		read     func(*testing.T, *config.Config) string // commentary URL read afterwards
		wantNote bool
	}{
		"rename keeps it": {
			seed: personal, list: "todos",
			steps:    []step{{"/todos/" + id + "/edit", url.Values{"title": {"Renew the car rego"}, "body": {""}, "tags": {""}}}},
			read:     readOwn("todos"),
			wantNote: true,
		},
		"move to family keeps it": {
			seed: personal, list: "todos",
			steps:    []step{{"/todos/" + id + "/move", nil}},
			read:     readOwn("family"),
			wantNote: true,
		},
		"move that must take a new ID copies it": {
			seed: func(cfg *config.Config) {
				personal(cfg)
				// A leftover of a failed move already holds the ID in family.
				seedFile(t, cfg.FamilyPath, "# Family\n\n- [ ] Pay rego [id: "+id+"]\n")
			},
			list:  "todos",
			steps: []step{{"/todos/" + id + "/move", nil}},
			read: func(t *testing.T, cfg *config.Config) string {
				for newID := range itemsByID(t, cfg.FamilyPath) {
					if newID != id {
						return "/commentary/family/" + newID
					}
				}
				t.Fatal("the moved item did not get a new ID")
				return ""
			},
			wantNote: true,
		},
		"trash keeps it": {
			seed: personal, list: "todos",
			steps:    []step{{"/todos/" + id + "/delete", nil}},
			read:     readOwn("todos"),
			wantNote: true,
		},
		"task purge removes it": {
			seed: personal, list: "todos",
			steps: []step{{"/todos/" + id + "/delete", nil}, {"/todos/" + id + "/purge", nil}},
			read:  readOwn("todos"),
		},
		"idea purge removes it": {
			seed: func(cfg *config.Config) {
				seedFile(t, filepath.Join(cfg.UserDataDir, "1", "ideas.md"), "# Ideas\n\n- [ ] Kayak [status: untriaged] [id: "+id+"]\n")
			},
			list:  "ideas",
			steps: []step{{"/ideas/" + id + "/delete", nil}, {"/ideas/" + id + "/purge", nil}},
			read:  readOwn("ideas"),
		},
		"maintenance purge removes it": {
			seed: func(cfg *config.Config) {
				seedFile(t, cfg.MaintenancePath, "# Maintenance\n\n- [ ] Clean gutters [cadence: 6m] [id: "+id+"]\n")
			},
			list:  "house",
			steps: []step{{"/house/maintenance/" + id + "/delete", nil}, {"/house/maintenance/" + id + "/purge", nil}},
			read:  readOwn("house"),
		},
		"house project purge removes it": {
			seed: func(cfg *config.Config) {
				seedFile(t, cfg.HouseProjectsPath, "# House\n\n- [ ] Paint the fence [status: todo] [id: "+id+"]\n")
			},
			list:  "house",
			steps: []step{{"/house/projects/" + id + "/delete", nil}, {"/house/projects/" + id + "/purge", nil}},
			read:  readOwn("house"),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := newSeededAppEnv(t, tc.seed)
			req := httptest.NewRequest("PUT", "/api/v1/commentary/"+tc.list+"/"+id, strings.NewReader(`{"content":"`+note+`"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+env.token)
			rr := httptest.NewRecorder()
			env.h.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("PUT commentary: status %d: %s", rr.Code, rr.Body.String())
			}

			for _, s := range tc.steps {
				if rr := env.post(t, s.path, s.form); rr.Code != http.StatusSeeOther {
					t.Fatalf("POST %s: status %d: %s", s.path, rr.Code, rr.Body.String())
				}
			}
			if got := env.get(t, tc.read(t, env.cfg)); strings.Contains(got, note) != tc.wantNote {
				t.Errorf("commentary after %s = %q, want note present = %v", name, got, tc.wantNote)
			}
		})
	}
}
