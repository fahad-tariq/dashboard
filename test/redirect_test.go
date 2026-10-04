package test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/auth"
)

// Browsers treat "//host" and "/\host" as protocol-relative URLs, so either
// form reaching a Location header is an open redirect.

func TestLoginSubmitRejectsOffsiteNext(t *testing.T) {
	tests := map[string]string{
		"double slash":       "//evil.example",
		"slash backslash":    `/\evil.example`,
		"backslash only":     `\\evil.example`,
		"tab after slash":    "/\t/evil.example",
		"absolute url":       "https://evil.example/",
		"encoded in path ok": "/ideas",
	}
	for name, next := range tests {
		t.Run(name, func(t *testing.T) {
			sm, database := newTestSessionManager(t)
			createTestUser(t, database, "alice@test.com", "secretpw")
			h := auth.NewHandler(sm, database, auth.NewRateLimiter(), loginTmpl)
			handler := sm.LoadAndSave(http.HandlerFunc(h.LoginSubmit))

			form := url.Values{"email": {"alice@test.com"}, "password": {"secretpw"}, "next": {next}}
			req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			want := "/"
			if next == "/ideas" {
				want = "/ideas"
			}
			if loc := rr.Header().Get("Location"); loc != want {
				t.Errorf("next=%q: Location = %q, want %q", next, loc, want)
			}
		})
	}
}

func TestTrackerRedirectBackRejectsOffsiteReferer(t *testing.T) {
	tests := map[string]struct {
		referer string
		want    string
	}{
		"same-site referer":         {referer: "http://dash.local/todos?msg=x", want: "/todos"},
		"double slash path":         {referer: "https://evil.example//evil.example/x", want: "/todos"},
		"slash backslash path":      {referer: `https://evil.example/\evil.example/x`, want: "/todos"},
		"no referer falls back":     {referer: "", want: "/todos"},
		"unparseable falls back":    {referer: "http://[::1", want: "/todos"},
		"other page keeps its path": {referer: "http://dash.local/goals", want: "/goals"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env := setupTrackerEnv(t)
			req := httptest.NewRequest("POST", "/todos/existing-task/priority", strings.NewReader("priority=high"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			rr := httptest.NewRecorder()
			env.router.ServeHTTP(rr, req)

			loc := rr.Header().Get("Location")
			path, _, _ := strings.Cut(loc, "#")
			path, _, _ = strings.Cut(path, "?")
			if path != tc.want {
				t.Errorf("Referer %q: Location = %q, want path %q", tc.referer, loc, tc.want)
			}
		})
	}
}
