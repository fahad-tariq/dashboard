package test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
