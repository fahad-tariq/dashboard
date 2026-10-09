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

// Commentary is keyed by item ID, so it follows a task through a rename and a
// move, and goes when the task is purged.
func TestCommentaryFollowsTheItemID(t *testing.T) {
	const id, note = "b7k2m9xq", "Waiting on the insurer."
	type step struct {
		path string
		form url.Values
	}
	cases := map[string]struct {
		steps    []step
		readFrom string // the list whose commentary URL is read afterwards
		wantNote bool
	}{
		"rename keeps it": {
			steps:    []step{{"/todos/" + id + "/edit", url.Values{"title": {"Renew the car rego"}, "body": {""}, "tags": {""}}}},
			readFrom: "todos", wantNote: true,
		},
		"move to family keeps it": {
			steps:    []step{{"/todos/" + id + "/move", nil}},
			readFrom: "family", wantNote: true,
		},
		"trash keeps it": {
			steps:    []step{{"/todos/" + id + "/delete", nil}},
			readFrom: "todos", wantNote: true,
		},
		"purge removes it": {
			steps:    []step{{"/todos/" + id + "/delete", nil}, {"/todos/" + id + "/purge", nil}},
			readFrom: "todos", wantNote: false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := newSeededAppEnv(t, func(cfg *config.Config) {
				seedFile(t, filepath.Join(cfg.UserDataDir, "1", "personal.md"), "# Personal\n\n- [ ] Pay rego [id: "+id+"]\n")
			})
			req := httptest.NewRequest("PUT", "/api/v1/commentary/todos/"+id, strings.NewReader(`{"content":"`+note+`"}`))
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
			if got := env.get(t, "/commentary/"+tc.readFrom+"/"+id); strings.Contains(got, note) != tc.wantNote {
				t.Errorf("commentary after %s = %q, want note present = %v", name, got, tc.wantNote)
			}
		})
	}
}
