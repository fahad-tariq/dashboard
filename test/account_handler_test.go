package test

import (
	"database/sql"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	accountpkg "github.com/fahad/dashboard/internal/account"
	"github.com/fahad/dashboard/internal/auth"
	dbpkg "github.com/fahad/dashboard/internal/db"
)

type accountTestEnv struct {
	database *sql.DB
	sm       *scs.SessionManager
	router   *chi.Mux
	cookie   *http.Cookie
}

func setupAccountEnv(t *testing.T) *accountTestEnv {
	t.Helper()
	tmpDir := t.TempDir()

	database, err := dbpkg.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	sm := scs.New()
	store := auth.NewSQLiteStore(database)
	sm.Store = store
	sm.Lifetime = time.Hour
	sm.Cookie.Name = "session"

	funcMap := template.FuncMap{
		"authEnabled": func() bool { return true },
	}
	layout := template.Must(template.New("layout.html").Funcs(funcMap).Parse(
		`{{define "layout.html"}}{{template "content" .}}{{end}}`,
	))
	templates := make(map[string]*template.Template)
	tmpl, _ := template.Must(layout.Clone()).Parse(
		`{{define "content"}}account|FlashMsg={{.FlashMsg}}|FirstName={{.FirstName}}|NameError={{.NameError}}|PasswordError={{.PasswordError}}{{end}}`,
	)
	templates["account.html"] = tmpl

	acctHandler := accountpkg.NewHandler(database, sm, templates)

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	r.Use(auth.RequireAuth(sm))
	r.Get("/account", acctHandler.AccountPage)
	r.Post("/account/name", acctHandler.NameSubmit)
	r.Post("/account/password", acctHandler.PasswordSubmit)

	createTestUser(t, database, "alice@test.com", "password1")
	cookie := setupAdminSession(t, sm, 1, "alice@test.com", false)

	return &accountTestEnv{
		database: database,
		sm:       sm,
		router:   r,
		cookie:   cookie,
	}
}

func TestAccountPageRenders(t *testing.T) {
	env := setupAccountEnv(t)

	req := httptest.NewRequest("GET", "/account", nil)
	req.AddCookie(env.cookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "account") {
		t.Errorf("expected account template to render, got: %s", body)
	}
}

func TestAccountNameUpdate(t *testing.T) {
	env := setupAccountEnv(t)

	form := url.Values{"first_name": {"Alice"}}
	req := httptest.NewRequest("POST", "/account/name", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.cookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=name-updated") {
		t.Errorf("expected redirect with msg=name-updated, got %q", loc)
	}
}

func TestAccountNameUpdateEmpty(t *testing.T) {
	env := setupAccountEnv(t)

	form := url.Values{"first_name": {""}}
	req := httptest.NewRequest("POST", "/account/name", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.cookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=name-updated") {
		t.Errorf("expected redirect with msg=name-updated, got %q", loc)
	}
}

func TestAccountPasswordUpdate(t *testing.T) {
	env := setupAccountEnv(t)

	form := url.Values{
		"current_password": {"password1"},
		"password":         {"newpass123"},
		"confirm":          {"newpass123"},
	}
	req := httptest.NewRequest("POST", "/account/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.cookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=password-updated") {
		t.Errorf("expected redirect with msg=password-updated, got %q", loc)
	}
}

func TestAccountPasswordWrongCurrent(t *testing.T) {
	env := setupAccountEnv(t)

	form := url.Values{
		"current_password": {"wrongpassword"},
		"password":         {"newpass123"},
		"confirm":          {"newpass123"},
	}
	req := httptest.NewRequest("POST", "/account/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.cookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (re-render with error), got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Current password is incorrect") {
		t.Errorf("expected 'Current password is incorrect' error, got: %s", body)
	}
}

func TestAccountPasswordMismatch(t *testing.T) {
	env := setupAccountEnv(t)

	form := url.Values{
		"current_password": {"password1"},
		"password":         {"newpass123"},
		"confirm":          {"different1"},
	}
	req := httptest.NewRequest("POST", "/account/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.cookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (re-render with error), got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Passwords do not match") {
		t.Errorf("expected 'Passwords do not match' error, got: %s", body)
	}
}

func TestAccountPasswordMissing(t *testing.T) {
	env := setupAccountEnv(t)

	form := url.Values{
		"current_password": {""},
		"password":         {"newpass123"},
		"confirm":          {"newpass123"},
	}
	req := httptest.NewRequest("POST", "/account/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.cookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (re-render with error), got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Current password is required") {
		t.Errorf("expected 'Current password is required' error, got: %s", body)
	}
}
