package test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/fahad/dashboard/internal/config"
)

// Ticking a house project marks it done everywhere: Done for the digest and
// Status for the house page, from the plan and from the house page alike.
func TestHouseTickSetsStatus(t *testing.T) {
	const id = "fnc3pnt1"
	cases := map[string]struct {
		path string
		form url.Values
	}{
		"plan tick":       {"/plan/" + id + "/complete", url.Values{"list": {"house"}}},
		"house page tick": {"/house/projects/" + id + "/complete", nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var path string
			env := newSeededAppEnv(t, func(cfg *config.Config) {
				path = cfg.HouseProjectsPath
				seedFile(t, path, "# House\n\n- [ ] Paint the fence [planned: 2026-10-11] [status: active] [id: "+id+"]\n")
			})
			if rr := env.post(t, tc.path, tc.form); rr.Code != http.StatusSeeOther {
				t.Fatalf("POST %s: status %d: %s", tc.path, rr.Code, rr.Body.String())
			}
			it := itemsByID(t, path)[id]
			if !it.Done || it.Status != "done" || it.Completed == "" {
				t.Errorf("after %s: done %v, status %q, completed %q; want done, status done and a date", name, it.Done, it.Status, it.Completed)
			}
		})
	}
}
