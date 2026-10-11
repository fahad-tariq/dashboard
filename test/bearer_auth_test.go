package test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/auth"
)

// The API answers 401 to anything but a valid bearer token: an empty or
// short DASHBOARD_API_TOKEN never matches, not even an empty bearer, and a
// session cookie is not a credential for it. A bearer token is not one for
// the browser routes.
func TestBearerAuth(t *testing.T) {
	strong := strings.Repeat("s", 32)
	tests := map[string]struct {
		envToken string
		header   string // the Authorization header; empty sends none
		cookie   bool
		path     string
		want     int
	}{
		"env unset, empty bearer":       {envToken: "", header: "Bearer ", path: "/api/v1/todos", want: http.StatusUnauthorized},
		"env unset, bearer x":           {envToken: "", header: "Bearer x", path: "/api/v1/todos", want: http.StatusUnauthorized},
		"env unset, no header":          {envToken: "", path: "/api/v1/todos", want: http.StatusUnauthorized},
		"env short, empty bearer":       {envToken: "short", header: "Bearer ", path: "/api/v1/todos", want: http.StatusUnauthorized},
		"env short, matching bearer":    {envToken: "short", header: "Bearer short", path: "/api/v1/todos", want: http.StatusUnauthorized},
		"env set, matching bearer":      {envToken: strong, header: "Bearer " + strong, path: "/api/v1/todos", want: http.StatusOK},
		"env set, empty bearer":         {envToken: strong, header: "Bearer ", path: "/api/v1/todos", want: http.StatusUnauthorized},
		"env set, lower-case scheme":    {envToken: strong, header: "bearer " + strong, path: "/api/v1/todos", want: http.StatusUnauthorized},
		"env set, token in the query":   {envToken: strong, path: "/api/v1/todos?access_token=" + strong, want: http.StatusUnauthorized},
		"env set, session cookie":       {envToken: strong, cookie: true, path: "/api/v1/todos", want: http.StatusUnauthorized},
		"env unset, session cookie":     {envToken: "", cookie: true, path: "/api/v1/plan", want: http.StatusUnauthorized},
		"bearer on /account":            {envToken: strong, header: "Bearer " + strong, path: "/account", want: http.StatusSeeOther},
		"bearer on the todos page":      {envToken: strong, header: "Bearer " + strong, path: "/todos", want: http.StatusSeeOther},
		"env unset, API module route":   {envToken: "", header: "Bearer ", path: "/api/v1/ideas", want: http.StatusUnauthorized},
		"env unset, unknown API path":   {envToken: "", header: "Bearer ", path: "/api/v1/nothing", want: http.StatusUnauthorized},
		"env set, plan with the bearer": {envToken: strong, header: "Bearer " + strong, path: "/api/v1/plan", want: http.StatusOK},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env := newTokenAppEnv(t, tc.envToken, nil)
			req := httptest.NewRequest("GET", tc.path, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if tc.cookie {
				req.AddCookie(env.cookie)
			}
			rr := httptest.NewRecorder()
			env.h.ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Errorf("status %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

// A device token minted for the owner works whether or not an env token is
// set, and stops working the moment it is revoked.
func TestBearerAuthDeviceTokens(t *testing.T) {
	tests := map[string]struct {
		envToken string
		revoke   bool
		want     int
	}{
		"device token, env unset":         {envToken: "", want: http.StatusOK},
		"device token, env set":           {envToken: strings.Repeat("s", 32), want: http.StatusOK},
		"revoked device token, env unset": {envToken: "", revoke: true, want: http.StatusUnauthorized},
		"revoked device token, env set":   {envToken: strings.Repeat("s", 32), revoke: true, want: http.StatusUnauthorized},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env := newTokenAppEnv(t, tc.envToken, nil)
			plain, err := auth.CreateToken(env.db, 1, "phone")
			if err != nil {
				t.Fatal(err)
			}
			call := func() int {
				req := httptest.NewRequest("GET", "/api/v1/plan", nil)
				req.Header.Set("Authorization", "Bearer "+plain)
				rr := httptest.NewRecorder()
				env.h.ServeHTTP(rr, req)
				return rr.Code
			}
			if got := call(); got != http.StatusOK {
				t.Fatalf("before revoking: status %d, want 200", got)
			}
			if tc.revoke {
				list, _ := auth.ListTokens(env.db, 1)
				if err := auth.RevokeToken(env.db, list[0].ID, 1); err != nil {
					t.Fatal(err)
				}
			}
			if got := call(); got != tc.want {
				t.Errorf("status %d, want %d", got, tc.want)
			}
		})
	}
}

// Each API write logs the token's name and the route, and never the token
// or the item's title; reads log nothing.
func TestAPIWritesLogged(t *testing.T) {
	env := newTokenAppEnv(t, "", nil)
	plain, err := auth.CreateToken(env.db, 1, "phone")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(orig) })

	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/v1/todos", ""},
		{"POST", "/api/v1/todos", `{"title":"Secret errand","list":"personal"}`},
	} {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		req.Header.Set("Authorization", "Bearer "+plain)
		env.h.ServeHTTP(httptest.NewRecorder(), req)
	}

	var lines []string
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if strings.Contains(line, "api write") {
			lines = append(lines, line)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("api write lines = %d, want 1:\n%s", len(lines), buf.String())
	}
	for _, want := range []string{"token=phone", "method=POST", "route=/api/v1/todos", "status=201"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("log line lacks %q: %s", want, lines[0])
		}
	}
	if strings.Contains(buf.String(), plain) || strings.Contains(buf.String(), "Secret errand") {
		t.Errorf("the log holds the token or the title:\n%s", buf.String())
	}
}
