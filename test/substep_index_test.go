package test

import (
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/fahad/dashboard/internal/config"
)

// An out-of-range sub-step index is a bad request on every sub-step action,
// never a panic.
func TestSubStepIndexOutOfRange(t *testing.T) {
	const id = "b7k2m9xq"
	cases := map[string]struct {
		action string
		index  string
	}{
		"promote -1":  {"promote", "-1"},
		"promote 5":   {"promote", "5"},
		"toggle -1":   {"toggle", "-1"},
		"remove -1":   {"remove", "-1"},
		"promote abc": {"promote", "abc"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := newSeededAppEnv(t, func(cfg *config.Config) {
				seedFile(t, filepath.Join(cfg.UserDataDir, "1", "personal.md"), "# Personal\n\n- [ ] Pay rego [id: "+id+"]\n  - [ ] Find the papers\n")
			})
			path := "/todos/" + id + "/substep/" + tc.action
			if rr := env.post(t, path, url.Values{"index": {tc.index}}); rr.Code != http.StatusBadRequest {
				t.Errorf("POST %s index %s: status %d, want 400", path, tc.index, rr.Code)
			}
		})
	}
}
