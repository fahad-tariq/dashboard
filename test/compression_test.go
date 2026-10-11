package test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// Pages are gzipped when the client accepts it. chi's compressor decides
// from Content-Type when the header is written, so a handler that renders a
// template without setting the type would be sent uncompressed.
func TestHTMLPagesCompressed(t *testing.T) {
	h, _, _ := authRouter(t, []testUser{{"one@test.com", "password-one"}}, nil)
	cookie := login(t, h, "one@test.com", "password-one")

	tests := map[string]struct {
		method, path string
		form         url.Values
		cookie       bool
		htmx         bool
		wantType     string
	}{
		"home":     {method: "GET", path: "/", cookie: true, wantType: "text/html"},
		"todos":    {method: "GET", path: "/todos", cookie: true, wantType: "text/html"},
		"login":    {method: "GET", path: "/login", wantType: "text/html"},
		"account":  {method: "GET", path: "/account", cookie: true, wantType: "text/html"},
		"ideas":    {method: "GET", path: "/ideas", cookie: true, wantType: "text/html"},
		"fragment": {method: "POST", path: "/todos/add", form: url.Values{"title": {"Compressed"}}, cookie: true, htmx: true, wantType: "text/html"},
		"static":   {method: "GET", path: "/static/theme.css", wantType: "text/css"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.form.Encode()))
			if tc.form != nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			if tc.cookie {
				req.AddCookie(cookie)
			}
			if tc.htmx {
				req.Header.Set("HX-Request", "true")
				req.Header.Set("Referer", "/todos")
			}
			req.Header.Set("Accept-Encoding", "gzip")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			assertGzipped(t, rr, tc.wantType)
		})
	}
}

// The rate-limited login page writes its 429 status before rendering, so
// the type must be filled in at WriteHeader as well as at Write.
func TestRateLimitedLoginCompressed(t *testing.T) {
	h, _, _ := authRouter(t, []testUser{{"one@test.com", "password-one"}}, nil)
	for i := range 50 {
		rr := loginAttempt(h, "192.0.2.7:1234", map[string]string{"Accept-Encoding": "gzip"}, "nobody@test.com", "wrong")
		if rr.Code == http.StatusTooManyRequests {
			assertGzipped(t, rr, "text/html")
			return
		}
		if i == 49 {
			t.Fatalf("no 429 after 50 failed logins; last status %d", rr.Code)
		}
	}
}

func assertGzipped(t *testing.T, rr *httptest.ResponseRecorder, wantType string) {
	t.Helper()
	if rr.Code >= 300 && rr.Code < 400 {
		t.Fatalf("status %d, Location %q", rr.Code, rr.Header().Get("Location"))
	}
	if got := rr.Header().Get("Content-Type"); !strings.HasPrefix(got, wantType) {
		t.Errorf("Content-Type = %q, want %s", got, wantType)
	}
	if got := rr.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
	if !slices.Contains(rr.Header().Values("Vary"), "Accept-Encoding") {
		t.Errorf("Vary = %q, want Accept-Encoding", rr.Header().Values("Vary"))
	}
}
