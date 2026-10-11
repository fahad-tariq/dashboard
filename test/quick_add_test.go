package test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/httputil"
	"github.com/fahad/dashboard/internal/tracker"
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
		// Requirement 6: what is left of the title still cannot set a tag.
		"API, tags in the remainder": {json: `{"title":"Pay rent [tags: x] #a] [planned: 2026-01-01 [deleted: 2026-01-01]","list":"personal"}`, wantLine: "- [ ] Pay rent [added:", wantTags: "[tags: a]"},
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
			for _, tag := range []string{"[planned:", "[deleted:", "[tags: x"} {
				if strings.Contains(string(data), tag) {
					t.Errorf("the title set %q:\n%s", tag, data)
				}
			}
		})
	}
}

func TestQuickAdd(t *testing.T) {
	tests := map[string]struct {
		title, priority string
		tags            []string
		wantTitle       string
		wantTags        []string
		wantPriority    string
	}{
		"plain title":                    {title: "Buy milk", wantTitle: "Buy milk"},
		"tag":                            {title: "Buy milk #groceries", wantTitle: "Buy milk", wantTags: []string{"groceries"}},
		"priority":                       {title: "Call mum !high", wantTitle: "Call mum", wantPriority: "high"},
		"dictated priority":              {title: "Call mum ! High", wantTitle: "Call mum", wantPriority: "high"},
		"priority in capitals":           {title: "Call mum !LOW", wantTitle: "Call mum", wantPriority: "low"},
		"lone bang":                      {title: "Wow ! that", wantTitle: "Wow ! that"},
		"trailing bang":                  {title: "Done !", wantTitle: "Done !"},
		"bang before another word":       {title: "Go ! now", wantTitle: "Go ! now"},
		"unknown priority word":          {title: "Fix it !urgent", wantTitle: "Fix it !urgent"},
		"both markers":                   {title: "Buy milk #groceries ! High", wantTitle: "Buy milk", wantTags: []string{"groceries"}, wantPriority: "high"},
		"two tags":                       {title: "Plan trip #travel #family", wantTitle: "Plan trip", wantTags: []string{"travel", "family"}},
		"first priority wins":            {title: "Do it !low !high", wantTitle: "Do it", wantPriority: "low"},
		"trailing hash":                  {title: "Issue #", wantTitle: "Issue #"},
		"number tag":                     {title: "Fix issue #42", wantTitle: "Fix issue", wantTags: []string{"42"}},
		"hash inside a word":             {title: "Learn C#", wantTitle: "Learn C#"},
		"dictation punctuation":          {title: "Buy milk #groceries. ! High.", wantTitle: "Buy milk", wantTags: []string{"groceries"}, wantPriority: "high"},
		"tags supplied":                  {title: "Buy #x !high", tags: []string{"shop"}, wantTitle: "Buy #x", wantTags: []string{"shop"}, wantPriority: "high"},
		"priority supplied":              {title: "Buy #x ! high", priority: "low", wantTitle: "Buy ! high", wantTags: []string{"x"}, wantPriority: "low"},
		"tag value cleaned":              {title: "Pay #a]b #[c] #,", wantTitle: "Pay", wantTags: []string{"ab", "c"}},
		"tag holding a priority cleaned": {title: "Pay #!high", wantTitle: "Pay"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			title, tags, priority := tracker.QuickAdd(tc.title, tc.tags, tc.priority)
			if title != tc.wantTitle {
				t.Errorf("title = %q, want %q", title, tc.wantTitle)
			}
			if !slices.Equal(tags, tc.wantTags) {
				t.Errorf("tags = %q, want %q", tags, tc.wantTags)
			}
			if priority != tc.wantPriority {
				t.Errorf("priority = %q, want %q", priority, tc.wantPriority)
			}
			if clean := httputil.CleanTitle(title); strings.ContainsAny(clean, "[]") {
				t.Errorf("cleaned title %q still holds a bracket", clean)
			}
		})
	}
}
