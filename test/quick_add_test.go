package test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A dictated title carries its tags and priority: "#tag" and "!priority"
// (any case, a space after "!" allowed) are parsed out of the title by the
// web add form and the API alike, each only when the request did not set
// that field itself.
func TestQuickAddThroughWebAndAPI(t *testing.T) {
	tests := map[string]struct {
		form     url.Values // a web form post when set
		json     string     // an API request otherwise
		wantLine string     // the item line, up to [added:
		wantTags string
	}{
		"web, dictated":             {form: url.Values{"title": {"Buy milk #groceries ! High"}}, wantLine: "- [ ] Buy milk !high [added:", wantTags: "[tags: groceries]"},
		"API, dictated":             {json: `{"title":"Buy milk #groceries ! High","list":"personal"}`, wantLine: "- [ ] Buy milk !high [added:", wantTags: "[tags: groceries]"},
		"API, empty priority field": {json: `{"title":"Buy milk #groceries ! High","list":"personal","priority":""}`, wantLine: "- [ ] Buy milk !high [added:", wantTags: "[tags: groceries]"},
		"API, priority supplied":    {json: `{"title":"Buy milk #groceries","list":"personal","priority":"low"}`, wantLine: "- [ ] Buy milk !low [added:", wantTags: "[tags: groceries]"},
		"web, tags supplied":        {form: url.Values{"title": {"Buy milk #x !high"}, "tags": {"shop"}}, wantLine: "- [ ] Buy milk #x !high [added:", wantTags: "[tags: shop]"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env := newAppEnv(t)
			var rr interface{ Result() *http.Response }
			if tc.form != nil {
				rr = env.post(t, "/todos/add", tc.form)
			} else {
				rr = env.api(t, "POST", "/api/v1/todos", tc.json)
			}
			if code := rr.Result().StatusCode; code != http.StatusSeeOther && code != http.StatusCreated {
				t.Fatalf("status %d", code)
			}
			data, err := os.ReadFile(filepath.Join(env.cfg.UserDataDir, "1", "personal.md"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.wantLine) {
				t.Errorf("file lacks %q:\n%s", tc.wantLine, data)
			}
			if !strings.Contains(string(data), tc.wantTags) {
				t.Errorf("file lacks %q:\n%s", tc.wantTags, data)
			}
		})
	}
}
