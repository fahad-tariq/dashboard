package test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/app"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/web"
)

const strongAPIToken = "0123456789abcdef0123456789abcdef"

// buildTestRouter builds the real router in auth mode with env overrides.
func buildTestRouter(t *testing.T, overrides map[string]string) http.Handler {
	t.Helper()
	paths := tempPaths(t)
	paths["DASHBOARD_PASSWORD_HASH"] = testBcryptHash
	paths["DASHBOARD_API_TOKEN"] = strongAPIToken
	for k, v := range overrides {
		paths[k] = v
	}
	setEnvForConfig(t, paths)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { closeDB(t, database) })
	router, err := app.NewRouter(t.Context(), cfg, database, "test")
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return router
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestAPIRequiresStrongToken(t *testing.T) {
	tests := map[string]struct {
		token  string
		bearer string
		want   int
	}{
		"unset token: not mounted":   {token: "", bearer: "", want: http.StatusNotFound},
		"short token: not mounted":   {token: "short-token", bearer: "short-token", want: http.StatusNotFound},
		"strong token, right bearer": {token: strongAPIToken, bearer: strongAPIToken, want: http.StatusOK},
		"strong token, wrong bearer": {token: strongAPIToken, bearer: "nope", want: http.StatusUnauthorized},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			h := buildTestRouter(t, map[string]string{"DASHBOARD_API_TOKEN": tc.token})
			req := httptest.NewRequest("GET", "/api/v1/todos", nil)
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			if got := serve(h, req).Code; got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAPIRateLimitsFailedBearerAttempts(t *testing.T) {
	h := buildTestRouter(t, nil)
	var last int
	for range 11 {
		req := httptest.NewRequest("GET", "/api/v1/todos", nil)
		req.RemoteAddr = "203.0.113.9:1234"
		req.Header.Set("Authorization", "Bearer wrong-token")
		last = serve(h, req).Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("11th failed attempt: status %d, want 429", last)
	}
	// A valid token is never blocked, so a flood of bad guesses cannot lock
	// out the real client sharing the address.
	req := httptest.NewRequest("GET", "/api/v1/todos", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("Authorization", "Bearer "+strongAPIToken)
	if got := serve(h, req).Code; got != http.StatusOK {
		t.Errorf("valid token from a limited address: status %d, want 200", got)
	}
}

func TestCrossOriginRequestsRejected(t *testing.T) {
	tests := map[string]struct {
		method, path string
		headers      map[string]string
		forbidden    bool
	}{
		"cross-site form post":       {method: "POST", path: "/todos/add", headers: map[string]string{"Sec-Fetch-Site": "cross-site"}, forbidden: true},
		"mismatched origin":          {method: "POST", path: "/todos/add", headers: map[string]string{"Origin": "https://evil.example"}, forbidden: true},
		"cross-site login":           {method: "POST", path: "/login", headers: map[string]string{"Sec-Fetch-Site": "cross-site"}, forbidden: true},
		"cross-site logout":          {method: "POST", path: "/logout", headers: map[string]string{"Sec-Fetch-Site": "cross-site"}, forbidden: true},
		"cross-site upload":          {method: "POST", path: "/upload", headers: map[string]string{"Sec-Fetch-Site": "cross-site"}, forbidden: true},
		"same-origin form post":      {method: "POST", path: "/todos/add", headers: map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://dash.local"}},
		"same-origin htmx post":      {method: "POST", path: "/todos/add", headers: map[string]string{"Sec-Fetch-Site": "same-origin", "HX-Request": "true"}},
		"cross-site GET is harmless": {method: "GET", path: "/login", headers: map[string]string{"Sec-Fetch-Site": "cross-site"}},
		"API is outside protection":  {method: "POST", path: "/api/v1/todos", headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Authorization": "Bearer " + strongAPIToken}},
	}
	h := buildTestRouter(t, nil)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://dash.local"+tc.path, strings.NewReader("title=x"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			got := serve(h, req).Code
			if tc.forbidden && got != http.StatusForbidden {
				t.Errorf("status = %d, want 403", got)
			}
			if !tc.forbidden && got == http.StatusForbidden {
				t.Errorf("status = 403, want the request to reach the handler")
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	want := map[string]string{
		"Content-Security-Policy": "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self'",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "same-origin",
	}
	h := buildTestRouter(t, nil)
	for _, path := range []string{"/login", "/static/theme.css", "/todos", "/api/v1/todos"} {
		t.Run(path, func(t *testing.T) {
			rr := serve(h, httptest.NewRequest("GET", path, nil))
			for k, v := range want {
				if got := rr.Header().Get(k); got != v {
					t.Errorf("%s = %q, want %q", k, got, v)
				}
			}
		})
	}
}

func TestHTMXEvalDisabled(t *testing.T) {
	layout, err := fs.ReadFile(web.TemplateFS, "templates/layout.html")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`<meta name="htmx-config" content='\{"allowEval":false[,}]`).Match(layout) {
		t.Error(`layout.html's htmx-config meta must set "allowEval":false`)
	}
}
