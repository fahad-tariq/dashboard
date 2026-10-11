package test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/auth"
)

var shownToken = regexp.MustCompile(`id="new-token"[^>]*value="([0-9a-f]{64})"`)

// apiStatus calls GET /api/v1/plan with bearer and returns the status.
func (e *contractEnv) apiStatus(bearer string) int {
	req := httptest.NewRequest("GET", "/api/v1/plan", nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)
	return rr.Code
}

func TestAccountMintsAndRevokesTokens(t *testing.T) {
	env := newTokenAppEnv(t, "", nil)

	rr := env.post(t, "/account/tokens", url.Values{"name": {"iPhone"}})
	if rr.Code != http.StatusOK {
		t.Fatalf("create: status %d", rr.Code)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("create: Cache-Control %q, want no-store", cc)
	}
	m := shownToken.FindStringSubmatch(rr.Body.String())
	if m == nil {
		t.Fatalf("the new token is not shown:\n%s", rr.Body.String())
	}
	plain := m[1]
	if got := env.apiStatus(plain); got != http.StatusOK {
		t.Errorf("the shown token: API status %d, want 200", got)
	}

	page := env.get(t, "/account")
	if strings.Contains(page, plain) {
		t.Error("the account page shows the token again")
	}
	if !strings.Contains(page, "iPhone") || !strings.Contains(page, "last used") {
		t.Errorf("the account page does not list the token:\n%s", page)
	}

	list, _ := auth.ListTokens(env.db, 1)
	if len(list) != 1 {
		t.Fatalf("tokens = %d, want 1", len(list))
	}
	rr = env.post(t, "/account/tokens/"+strconv.FormatInt(list[0].ID, 10)+"/revoke", nil)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/account?msg=token-revoked" {
		t.Fatalf("revoke: %d to %q", rr.Code, rr.Header().Get("Location"))
	}
	if got := env.apiStatus(plain); got != http.StatusUnauthorized {
		t.Errorf("after revoking: API status %d, want 401", got)
	}
	if strings.Contains(env.get(t, "/account"), ">iPhone<") {
		t.Error("a revoked token is still listed")
	}
}

func TestAccountTokenRequests(t *testing.T) {
	tests := map[string]struct {
		path       string
		form       url.Values
		cookie     bool
		bearer     bool
		crossSite  bool
		wantStatus int
		wantTokens int // the owner's unrevoked tokens afterwards; one exists before
	}{
		"empty name":                 {path: "/account/tokens", form: url.Values{"name": {"  "}}, cookie: true, wantStatus: http.StatusBadRequest, wantTokens: 1},
		"no session":                 {path: "/account/tokens", form: url.Values{"name": {"x"}}, wantStatus: http.StatusSeeOther, wantTokens: 1},
		"bearer instead of session":  {path: "/account/tokens", form: url.Values{"name": {"x"}}, bearer: true, wantStatus: http.StatusSeeOther, wantTokens: 1},
		"cross-site create":          {path: "/account/tokens", form: url.Values{"name": {"x"}}, cookie: true, crossSite: true, wantStatus: http.StatusForbidden, wantTokens: 1},
		"cross-site revoke":          {path: "/account/tokens/1/revoke", cookie: true, crossSite: true, wantStatus: http.StatusForbidden, wantTokens: 1},
		"revoke another user's":      {path: "/account/tokens/2/revoke", cookie: true, wantStatus: http.StatusNotFound, wantTokens: 1},
		"revoke an unknown token":    {path: "/account/tokens/99/revoke", cookie: true, wantStatus: http.StatusNotFound, wantTokens: 1},
		"revoke with the bearer":     {path: "/account/tokens/1/revoke", bearer: true, wantStatus: http.StatusSeeOther, wantTokens: 1},
		"revoke own token":           {path: "/account/tokens/1/revoke", cookie: true, wantStatus: http.StatusSeeOther, wantTokens: 0},
		"create with a session":      {path: "/account/tokens", form: url.Values{"name": {"iPad"}}, cookie: true, wantStatus: http.StatusOK, wantTokens: 2},
		"create, name only brackets": {path: "/account/tokens", form: url.Values{"name": {"[]"}}, cookie: true, wantStatus: http.StatusBadRequest, wantTokens: 1},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env := newTokenAppEnv(t, "", nil)
			owner, err := auth.CreateToken(env.db, 1, "phone") // id 1
			if err != nil {
				t.Fatal(err)
			}
			if _, err := auth.CreateToken(env.db, 2, "not yours"); err != nil { // id 2
				t.Fatal(err)
			}
			req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.cookie {
				req.AddCookie(env.cookie)
			}
			if tc.bearer {
				req.Header.Set("Authorization", "Bearer "+owner)
			}
			if tc.crossSite {
				req.Header.Set("Sec-Fetch-Site", "cross-site")
				req.Header.Set("Origin", "https://evil.example")
			}
			rr := httptest.NewRecorder()
			env.h.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Errorf("status %d, want %d", rr.Code, tc.wantStatus)
			}
			if n, _ := auth.CountTokens(env.db, 1); n != tc.wantTokens {
				t.Errorf("owner's tokens = %d, want %d", n, tc.wantTokens)
			}
			if n, _ := auth.CountTokens(env.db, 2); n != 1 {
				t.Errorf("the other user's tokens = %d, want 1", n)
			}
		})
	}
}

// Changing the password keeps device tokens working; the page says so.
func TestPasswordChangeKeepsTokens(t *testing.T) {
	env := newTokenAppEnv(t, "", nil)
	plain, err := auth.CreateToken(env.db, 1, "phone")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env.get(t, "/account"), "Changing your password does not revoke tokens") {
		t.Error("the account page does not say a password change keeps tokens")
	}
	rr := env.post(t, "/account/password", url.Values{"current_password": {"owner-password"}, "password": {"new-password-1"}, "confirm": {"new-password-1"}})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("password change: status %d", rr.Code)
	}
	if got := env.apiStatus(plain); got != http.StatusOK {
		t.Errorf("after a password change: API status %d, want 200", got)
	}
}

// Only the owner's tokens open the API, so only the owner may mint them;
// another user sees no token section.
func TestAccountTokensOwnerOnly(t *testing.T) {
	env := newTokenAppEnv(t, "", nil)
	if _, err := auth.CreateUser(env.db, "second@test.com", "", "second-password"); err != nil {
		t.Fatal(err)
	}
	cookie := login(t, env.h, "second@test.com", "second-password")

	page := getAs(t, env.h, cookie, "/account").Body.String()
	if strings.Contains(page, `action="/account/tokens"`) {
		t.Error("a second user is offered the token form")
	}
	req := httptest.NewRequest("POST", "/account/tokens", strings.NewReader(url.Values{"name": {"x"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	env.h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("second user minting: status %d, want 403", rr.Code)
	}
	if n, _ := auth.CountTokens(env.db, 2); n != 0 {
		t.Errorf("second user's tokens = %d, want 0", n)
	}
}
