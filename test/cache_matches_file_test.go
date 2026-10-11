package test

import (
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/house"
)

// A body is cached as the parser would read it back (lines trimmed, blank
// lines dropped), so the sub-steps an API response shows are the ones a
// restart would load.
func TestAPIBodyMatchesFile(t *testing.T) {
	tests := map[string]struct {
		method, path, body string
		wantStatus         int
	}{
		"edit": {method: "PUT", path: "/api/v1/todos/pck00001", body: `{"list":"personal","body":"Note\n\n  - [ ] Step"}`, wantStatus: http.StatusOK},
		"add":  {method: "POST", path: "/api/v1/todos", body: `{"list":"personal","title":"New","body":"Note\n\n  - [ ] Step"}`, wantStatus: http.StatusCreated},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env := itemEnv(t)
			it := env.apiJSON(t, tc.method, tc.path, tc.body, tc.wantStatus)
			if it["body"] != "Note\n- [ ] Step" {
				t.Errorf("body = %q, want %q", it["body"], "Note\n- [ ] Step")
			}
			if steps, _ := it["sub_steps"].([]any); len(steps) != 1 {
				t.Errorf("sub_steps = %v, want one step", it["sub_steps"])
			}
		})
	}
}

// Every API path that sets tags holds them to the same length limit.
func TestAPISetTagsLimitsLength(t *testing.T) {
	env := itemEnv(t)
	body := `{"list":"personal","tags":["` + strings.Repeat("x", 51) + `"]}`
	if rr := env.api(t, "PUT", "/api/v1/todos/pck00001/tags", body); rr.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

// Maintenance tags are cleaned before they reach the cache, as the writer
// cleans them for the file.
func TestMaintenanceTagsMatchFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "maintenance.md")
	svc := house.NewService(path, time.UTC)
	item := &house.MaintenanceItem{Title: "Clean gutters", Cadence: "6m", Tags: []string{"roof]", " out "}}
	if err := svc.Add(item); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"roof", "out"}; !slices.Equal(got.Tags, want) {
		t.Errorf("after add, tags = %q, want %q", got.Tags, want)
	}
	if err := svc.UpdateEdit(item.ID, "Clean gutters", "", []string{"[a]"}); err != nil {
		t.Fatal(err)
	}
	if got, _ = svc.Get(item.ID); !slices.Equal(got.Tags, []string{"a"}) {
		t.Errorf("after edit, tags = %q, want [a]", got.Tags)
	}
}
