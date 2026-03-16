package test

import (
	"database/sql"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/db"
)

// --- Rate Limiter ---

func TestRateLimiterAllow(t *testing.T) {
	rl := auth.NewRateLimiter()
	ip := "1.2.3.4"

	for i := range 5 {
		if !rl.Allow(ip) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if rl.Allow(ip) {
		t.Error("6th attempt should be rejected")
	}
}

func TestRateLimiterDifferentIPs(t *testing.T) {
	rl := auth.NewRateLimiter()
	for range 5 {
		rl.Allow("ip-a")
	}
	if rl.Allow("ip-a") {
		t.Error("ip-a should be rate limited")
	}
	if !rl.Allow("ip-b") {
		t.Error("ip-b should be allowed")
	}
}

func TestRateLimiterRetryAfter(t *testing.T) {
	rl := auth.NewRateLimiter()
	ip := "1.2.3.4"

	for range 6 {
		rl.Allow(ip)
	}

	d := rl.RetryAfter(ip)
	if d <= 0 || d > time.Minute {
		t.Errorf("retry after should be between 0 and 1m, got %v", d)
	}
}

func TestRateLimiterRetryAfterUnknown(t *testing.T) {
	rl := auth.NewRateLimiter()
	if d := rl.RetryAfter("unknown"); d != 0 {
		t.Errorf("expected 0 for unknown IP, got %v", d)
	}
}

// --- Helpers ---

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func newTestSessionManager(t *testing.T) (*scs.SessionManager, *sql.DB) {
	t.Helper()
	database := newTestDB(t)

	sm := scs.New()
	sm.Store = auth.NewSQLiteStore(database)
	sm.Lifetime = time.Hour
	return sm, database
}

func createTestUser(t *testing.T, database *sql.DB, email, password string) int64 {
	t.Helper()
	id, err := auth.CreateUser(database, email, password)
	if err != nil {
		t.Fatalf("creating test user: %v", err)
	}
	return id
}

var loginTmpl = template.Must(template.New("login").Parse(
	`{{.Error}}|{{.Next}}|{{.Email}}`,
))

func hashPassword(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hashing password: %v", err)
	}
	return string(h)
}

// --- Middleware ---

func TestRequireAuthRedirects(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	handler := sm.LoadAndSave(auth.RequireAuth(sm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("GET", "/ideas/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if loc != "/login?next=/ideas/test" {
		t.Errorf("expected redirect to /login?next=/ideas/test, got %q", loc)
	}
}

func TestRequireAuthRedirectsRootNoNext(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	handler := sm.LoadAndSave(auth.RequireAuth(sm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if loc != "/login" {
		t.Errorf("expected redirect to /login (no next param for root), got %q", loc)
	}
}

func TestRequireAuthPassesAuthenticated(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Set up an authenticated session with user_id.
	var sessionCookie *http.Cookie
	setup := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sm.Put(r.Context(), "user_id", int64(1))
		sm.Put(r.Context(), "user_email", "alice@test.com")
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/setup", nil)
	rr := httptest.NewRecorder()
	setup.ServeHTTP(rr, req)
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie set")
	}

	// Use the session cookie to access a protected route.
	handler := sm.LoadAndSave(auth.RequireAuth(sm)(inner))
	req = httptest.NewRequest("GET", "/protected", nil)
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAuthAPIReturns401(t *testing.T) {
	sm, _ := newTestSessionManager(t)

	handler := sm.LoadAndSave(auth.RequireAuthAPI(sm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("GET", "/events", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// --- Login Handler ---

func TestLoginSubmitCorrectPassword(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	rl := auth.NewRateLimiter()
	h := auth.NewHandler(sm, database, rl, loginTmpl)

	handler := sm.LoadAndSave(http.HandlerFunc(h.LoginSubmit))

	form := url.Values{"email": {"alice@test.com"}, "password": {"secretpw"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("expected redirect to /, got %q", loc)
	}
}

func TestLoginSubmitWithNext(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	rl := auth.NewRateLimiter()
	h := auth.NewHandler(sm, database, rl, loginTmpl)

	handler := sm.LoadAndSave(http.HandlerFunc(h.LoginSubmit))

	form := url.Values{"email": {"alice@test.com"}, "password": {"secretpw"}, "next": {"/ideas"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if loc := rr.Header().Get("Location"); loc != "/ideas" {
		t.Errorf("expected redirect to /ideas, got %q", loc)
	}
}

func TestLoginSubmitRejectsOpenRedirect(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	rl := auth.NewRateLimiter()
	h := auth.NewHandler(sm, database, rl, loginTmpl)

	handler := sm.LoadAndSave(http.HandlerFunc(h.LoginSubmit))

	form := url.Values{"email": {"alice@test.com"}, "password": {"secretpw"}, "next": {"//evil.com"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("expected redirect to / (rejecting open redirect), got %q", loc)
	}
}

func TestLoginSubmitWrongPassword(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	rl := auth.NewRateLimiter()
	h := auth.NewHandler(sm, database, rl, loginTmpl)

	handler := sm.LoadAndSave(http.HandlerFunc(h.LoginSubmit))

	form := url.Values{"email": {"alice@test.com"}, "password": {"wrong"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (re-render login), got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Incorrect email or password") {
		t.Error("expected error message in response")
	}
}

func TestLoginSubmitUnknownEmail(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	rl := auth.NewRateLimiter()
	h := auth.NewHandler(sm, database, rl, loginTmpl)

	handler := sm.LoadAndSave(http.HandlerFunc(h.LoginSubmit))

	form := url.Values{"email": {"unknown@test.com"}, "password": {"secretpw"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Incorrect email or password") {
		t.Error("expected error message in response")
	}
}

func TestLoginSubmitRateLimited(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	rl := auth.NewRateLimiter()
	h := auth.NewHandler(sm, database, rl, loginTmpl)

	handler := sm.LoadAndSave(http.HandlerFunc(h.LoginSubmit))

	for range 5 {
		form := url.Values{"email": {"alice@test.com"}, "password": {"wrong"}}
		req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// 6th attempt should be rate limited.
	form := url.Values{"email": {"alice@test.com"}, "password": {"wrong"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !strings.Contains(rr.Body.String(), "Too many attempts") {
		t.Error("expected rate limit message in response")
	}
}

func TestLogout(t *testing.T) {
	sm, database := newTestSessionManager(t)
	h := auth.NewHandler(sm, database, auth.NewRateLimiter(), loginTmpl)

	handler := sm.LoadAndSave(http.HandlerFunc(h.Logout))

	req := httptest.NewRequest("POST", "/logout", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

// --- Session stores user_id ---

func TestLoginSetsUserIDInSession(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	rl := auth.NewRateLimiter()
	h := auth.NewHandler(sm, database, rl, loginTmpl)

	// Login.
	loginHandler := sm.LoadAndSave(http.HandlerFunc(h.LoginSubmit))
	form := url.Values{"email": {"alice@test.com"}, "password": {"secretpw"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	loginHandler.ServeHTTP(rr, req)

	var sessionCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie set after login")
	}

	// Verify the session has user_id by hitting a protected route.
	var gotUserID int64
	var gotEmail string
	protected := sm.LoadAndSave(auth.RequireAuth(sm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = auth.UserID(r.Context())
		gotEmail = auth.UserEmail(r.Context())
		w.WriteHeader(http.StatusOK)
	})))
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	protected.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotUserID != 1 {
		t.Errorf("expected user_id=1, got %d", gotUserID)
	}
	if gotEmail != "alice@test.com" {
		t.Errorf("expected email alice@test.com, got %q", gotEmail)
	}
}

func TestUserNameExtractsLocalPart(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@example.com", "password")

	var gotName string
	handler := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sm.Put(r.Context(), "user_id", int64(1))
		sm.Put(r.Context(), "user_email", "alice@example.com")
		w.WriteHeader(http.StatusOK)
	}))
	// First set up a session.
	req := httptest.NewRequest("GET", "/setup", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var sessionCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session" {
			sessionCookie = c
		}
	}

	// Now check the context through RequireAuth.
	check := sm.LoadAndSave(auth.RequireAuth(sm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotName = auth.UserName(r.Context())
		w.WriteHeader(http.StatusOK)
	})))
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	check.ServeHTTP(rr, req)

	if gotName != "alice" {
		t.Errorf("expected UserName 'alice', got %q", gotName)
	}
}

// --- User Management ---

func TestCreateUser(t *testing.T) {
	database := newTestDB(t)
	id, err := auth.CreateUser(database, "test@example.com", "password123")
	if err != nil {
		t.Fatalf("creating user: %v", err)
	}
	if id != 1 {
		t.Errorf("expected id=1, got %d", id)
	}
}

func TestCreateUserDuplicateEmail(t *testing.T) {
	database := newTestDB(t)
	_, err := auth.CreateUser(database, "test@example.com", "password1")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err = auth.CreateUser(database, "test@example.com", "password2")
	if err == nil {
		t.Error("expected error for duplicate email")
	}
}

func TestFindByEmail(t *testing.T) {
	database := newTestDB(t)
	auth.CreateUser(database, "alice@test.com", "secretpw")

	user, err := auth.FindByEmail(database, "alice@test.com")
	if err != nil {
		t.Fatalf("finding user: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.Email != "alice@test.com" {
		t.Errorf("email: got %q", user.Email)
	}
	if user.ID != 1 {
		t.Errorf("id: got %d", user.ID)
	}
}

func TestFindByEmailNotFound(t *testing.T) {
	database := newTestDB(t)
	user, err := auth.FindByEmail(database, "nobody@test.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != nil {
		t.Error("expected nil for unknown email")
	}
}

func TestUserCount(t *testing.T) {
	database := newTestDB(t)

	count, err := auth.UserCount(database)
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	auth.CreateUser(database, "a@test.com", "password")
	auth.CreateUser(database, "b@test.com", "password")

	count, err = auth.UserCount(database)
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestLegacyHashAutoCreatesUser(t *testing.T) {
	database := newTestDB(t)
	hash := hashPassword(t, "legacy-pass")

	count, _ := auth.UserCount(database)
	if count != 0 {
		t.Fatal("expected 0 users initially")
	}

	// Simulate the legacy migration logic from main.go.
	_, err := auth.CreateUserWithHash(database, "admin@localhost", hash)
	if err != nil {
		t.Fatalf("creating legacy user: %v", err)
	}

	user, err := auth.FindByEmail(database, "admin@localhost")
	if err != nil {
		t.Fatalf("finding legacy user: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.ID != 1 {
		t.Errorf("expected id=1, got %d", user.ID)
	}

	// Verify password works with bcrypt.
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("legacy-pass")); err != nil {
		t.Error("password hash should match the legacy password")
	}
}
