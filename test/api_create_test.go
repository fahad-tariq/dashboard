package test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/config"
)

// A create that brings an ID already in the list returns the stored item
// and writes nothing, so a retried request from a phone yields one item.
func TestAPICreateIsIdempotent(t *testing.T) {
	const personal = "# Personal\n\n" +
		"- [ ] Buy milk [added: 2026-10-01] [id: mlk00001]\n" +
		"- [ ] Old errand [added: 2026-10-01] [deleted: 2026-10-05] [id: dlt00001]\n"
	const ideasFile = "# Ideas\n\n" +
		"- [ ] Weather station [status: untriaged] [added: 2026-10-01] [id: wth00001]\n" +
		"- [ ] Old idea [status: parked] [added: 2026-10-01] [deleted: 2026-10-05] [id: dlt00002]\n"

	tests := map[string]struct {
		path, body  string
		file        string // personal.md or ideas.md
		wantStatus  int
		wantID      string // empty: any new ID
		wantTitle   string
		wantDeleted string
		wantWrite   bool
	}{
		"todo without an ID":          {path: "/api/v1/todos", body: `{"title":"Call mum","list":"personal"}`, file: "personal.md", wantStatus: http.StatusCreated, wantTitle: "Call mum", wantWrite: true},
		"todo with a new ID":          {path: "/api/v1/todos", body: `{"id":"nw000001","title":"Call mum","list":"personal"}`, file: "personal.md", wantStatus: http.StatusCreated, wantID: "nw000001", wantTitle: "Call mum", wantWrite: true},
		"todo retried with its ID":    {path: "/api/v1/todos", body: `{"id":"mlk00001","title":"Something else","list":"personal"}`, file: "personal.md", wantStatus: http.StatusOK, wantID: "mlk00001", wantTitle: "Buy milk"},
		"todo whose ID is trashed":    {path: "/api/v1/todos", body: `{"id":"dlt00001","title":"Old errand","list":"personal"}`, file: "personal.md", wantStatus: http.StatusOK, wantID: "dlt00001", wantTitle: "Old errand", wantDeleted: "2026-10-05"},
		"todo with an invalid ID":     {path: "/api/v1/todos", body: `{"id":"aeiou123","title":"Call mum","list":"personal"}`, file: "personal.md", wantStatus: http.StatusBadRequest},
		"idea without an ID":          {path: "/api/v1/ideas", body: `{"title":"Sail"}`, file: "ideas.md", wantStatus: http.StatusCreated, wantTitle: "Sail", wantWrite: true},
		"idea with a new ID":          {path: "/api/v1/ideas", body: `{"id":"nw000002","title":"Sail"}`, file: "ideas.md", wantStatus: http.StatusCreated, wantID: "nw000002", wantTitle: "Sail", wantWrite: true},
		"idea retried with its ID":    {path: "/api/v1/ideas", body: `{"id":"wth00001","title":"Something else"}`, file: "ideas.md", wantStatus: http.StatusOK, wantID: "wth00001", wantTitle: "Weather station"},
		"idea whose ID is trashed":    {path: "/api/v1/ideas", body: `{"id":"dlt00002","title":"Old idea"}`, file: "ideas.md", wantStatus: http.StatusOK, wantID: "dlt00002", wantTitle: "Old idea", wantDeleted: "2026-10-05"},
		"idea with an invalid ID":     {path: "/api/v1/ideas", body: `{"id":"short","title":"Sail"}`, file: "ideas.md", wantStatus: http.StatusBadRequest},
		"todo with an empty ID field": {path: "/api/v1/todos", body: `{"id":"","title":"Call mum","list":"personal"}`, file: "personal.md", wantStatus: http.StatusCreated, wantTitle: "Call mum", wantWrite: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env := newSeededAppEnv(t, func(cfg *config.Config) {
				writeSeed(t, filepath.Join(cfg.UserDataDir, "1", "personal.md"), personal)
				writeSeed(t, filepath.Join(cfg.UserDataDir, "1", "ideas.md"), ideasFile)
			})
			file := filepath.Join(env.cfg.UserDataDir, "1", tc.file)
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			rr := env.api(t, "POST", tc.path, tc.body)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status %d, want %d: %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			after, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if wrote := string(after) != string(before); wrote != tc.wantWrite {
				t.Errorf("file written = %v, want %v:\n%s", wrote, tc.wantWrite, after)
			}
			if tc.wantStatus == http.StatusBadRequest {
				return
			}

			it := decodeObject(t, rr.Body.Bytes())
			id, _ := it["id"].(string)
			if tc.wantID != "" && id != tc.wantID {
				t.Errorf("id = %q, want %q", id, tc.wantID)
			}
			if tc.wantWrite && !strings.Contains(string(after), "[id: "+id+"]") {
				t.Errorf("answered id %q is not in the file:\n%s", id, after)
			}
			if it["title"] != tc.wantTitle {
				t.Errorf("title = %v, want %q", it["title"], tc.wantTitle)
			}
			deleted, _ := it["deleted"].(string)
			if deleted != tc.wantDeleted {
				t.Errorf("deleted = %q, want %q", deleted, tc.wantDeleted)
			}
		})
	}
}

func decodeObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return out
}
