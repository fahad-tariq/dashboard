package test

import (
	"io/fs"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/fahad/dashboard/web"
)

// Every static URL goes through the static template function so it carries a
// content hash; a hard-coded path would be served from a stale cache after a
// deploy.
func TestTemplatesUseVersionedStaticURLs(t *testing.T) {
	raw := regexp.MustCompile(`["']/static/`)
	err := fs.WalkDir(web.TemplateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(web.TemplateFS, path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if raw.MatchString(line) {
				t.Errorf("%s:%d hard-codes a /static/ URL; use {{static \"name\"}}", path, i+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStaticAssetCaching(t *testing.T) {
	h := buildTestRouter(t, nil)
	page := serve(h, httptest.NewRequest("GET", "/login", nil)).Body.String()
	m := regexp.MustCompile(`/static/theme\.css\?v=[0-9a-f]+`).FindString(page)
	if m == "" {
		t.Fatalf("login page does not reference a versioned theme.css:\n%s", page)
	}

	tests := map[string]struct {
		url       string
		immutable bool
	}{
		"current version is immutable":  {url: m, immutable: true},
		"unversioned must revalidate":   {url: "/static/theme.css"},
		"stale version must revalidate": {url: "/static/theme.css?v=0000000000"},
		"missing file is not cacheable": {url: "/static/nope.css?v=abc"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rr := serve(h, httptest.NewRequest("GET", tc.url, nil))
			cc := rr.Header().Get("Cache-Control")
			if got := strings.Contains(cc, "immutable"); got != tc.immutable {
				t.Errorf("%s: Cache-Control %q (status %d), immutable = %v, want %v", tc.url, cc, rr.Code, got, tc.immutable)
			}
		})
	}
}
