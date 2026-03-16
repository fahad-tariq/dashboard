package test

import (
	"database/sql"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	adminpkg "github.com/fahad/dashboard/internal/admin"
	"github.com/fahad/dashboard/internal/auth"
	dbpkg "github.com/fahad/dashboard/internal/db"
)

// adminTestEnv bundles all dependencies for admin handler tests.
type adminTestEnv struct {
	database    *sql.DB
	sm          *scs.SessionManager
	handler     *adminpkg.Handler
	router      *chi.Mux
	tmpDir      string
	adminCookie *http.Cookie
}

func setupAdminEnv(t *testing.T) *adminTestEnv {
	t.Helper()
	tmpDir := t.TempDir()

	database, err := dbpkg.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	userDataDir := filepath.Join(tmpDir, "users")
	os.MkdirAll(userDataDir, 0o755)

	sm := scs.New()
	store := auth.NewSQLiteStore(database)
	sm.Store = store
	sm.Lifetime = time.Hour
	sm.Cookie.Name = "session"

	// Minimal templates for testing -- just enough to not panic.
	funcMap := template.FuncMap{
		"authEnabled": func() bool { return true },
	}
	layout := template.Must(template.New("layout.html").Funcs(funcMap).Parse(
		`{{define "layout.html"}}{{template "content" .}}{{end}}`,
	))

	templates := make(map[string]*template.Template)
	for _, name := range []string{"admin-users.html", "admin-user-form.html", "admin-password.html"} {
		tmpl, _ := template.Must(layout.Clone()).Parse(
			`{{define "content"}}` + name + `|FlashMsg={{.FlashMsg}}|Error={{.Error}}{{end}}`,
		)
		templates[name] = tmpl
	}

	adminHandler := adminpkg.NewHandler(database, nil, userDataDir, templates)

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	// Use the real RequireAuth middleware to inject context values.
	r.Use(auth.RequireAuth(sm))

	r.Get("/admin/users", adminHandler.ListUsers)
	r.Get("/admin/users/new", adminHandler.NewUserForm)
	r.Post("/admin/users/new", adminHandler.CreateUser)
	r.Get("/admin/users/{id}/edit", adminHandler.EditUserForm)
	r.Post("/admin/users/{id}/edit", adminHandler.UpdateUser)
	r.Get("/admin/users/{id}/password", adminHandler.ResetPasswordForm)
	r.Post("/admin/users/{id}/password", adminHandler.ResetPassword)
	r.Post("/admin/users/{id}/delete", adminHandler.DeleteUser)

	// Create admin user.
	createTestUser(t, database, "admin@test.com", "adminpass")

	// Set up admin session.
	cookie := setupAdminSession(t, sm, 1, "admin@test.com", true)

	return &adminTestEnv{
		database:    database,
		sm:          sm,
		handler:     adminHandler,
		router:      r,
		tmpDir:      tmpDir,
		adminCookie: cookie,
	}
}

func TestAdminListUsers(t *testing.T) {
	env := setupAdminEnv(t)

	req := httptest.NewRequest("GET", "/admin/users", nil)
	req.AddCookie(env.adminCookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "admin-users.html") {
		t.Error("expected admin-users template to render")
	}
}

func TestAdminListUsersWithFlash(t *testing.T) {
	env := setupAdminEnv(t)

	req := httptest.NewRequest("GET", "/admin/users?msg=user-created", nil)
	req.AddCookie(env.adminCookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "FlashMsg=User created.") {
		t.Errorf("expected flash message, got: %s", body)
	}
}

func TestAdminCreateUser(t *testing.T) {
	env := setupAdminEnv(t)

	form := url.Values{
		"email":    {"bob@test.com"},
		"password": {"bobpass1"},
		"role":     {"user"},
	}
	req := httptest.NewRequest("POST", "/admin/users/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=user-created") {
		t.Errorf("expected redirect with flash, got %q", loc)
	}
}

func TestAdminCreateUserMissingFields(t *testing.T) {
	env := setupAdminEnv(t)

	form := url.Values{
		"email":    {""},
		"password": {""},
		"role":     {"user"},
	}
	req := httptest.NewRequest("POST", "/admin/users/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (re-render form), got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Email and password are required") {
		t.Errorf("expected validation error, got: %s", body)
	}
}

func TestAdminCannotDeleteSelf(t *testing.T) {
	env := setupAdminEnv(t)

	// Admin is user ID 1 -- try to delete self.
	req := httptest.NewRequest("POST", "/admin/users/1/delete", nil)
	req.AddCookie(env.adminCookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "cannot-delete-self") {
		t.Errorf("expected cannot-delete-self flash, got %q", loc)
	}
}

func TestAdminCanDeleteOtherUser(t *testing.T) {
	env := setupAdminEnv(t)

	// Create another user to delete.
	auth.CreateUser(env.database, "victim@test.com", "password")

	// Create user data dir for user 2.
	userDir := filepath.Join(env.tmpDir, "users", "2")
	os.MkdirAll(userDir, 0o755)
	os.WriteFile(filepath.Join(userDir, "personal.md"), []byte("# Personal\n"), 0o644)

	req := httptest.NewRequest("POST", "/admin/users/2/delete", nil)
	req.AddCookie(env.adminCookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=user-deleted") {
		t.Errorf("expected user-deleted flash, got %q", loc)
	}

	// Verify user data dir was removed.
	if _, err := os.Stat(userDir); !os.IsNotExist(err) {
		t.Error("user data directory should have been removed")
	}
}

func TestAdminResetPassword(t *testing.T) {
	env := setupAdminEnv(t)

	form := url.Values{
		"password": {"newpass123"},
		"confirm":  {"newpass123"},
	}
	req := httptest.NewRequest("POST", "/admin/users/1/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "msg=password-reset") {
		t.Errorf("expected password-reset flash, got %q", loc)
	}
}

func TestAdminResetPasswordMismatch(t *testing.T) {
	env := setupAdminEnv(t)

	form := url.Values{
		"password": {"newpass123"},
		"confirm":  {"different"},
	}
	req := httptest.NewRequest("POST", "/admin/users/1/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (re-render), got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Passwords do not match") {
		t.Errorf("expected mismatch error, got: %s", body)
	}
}

func TestAdminUpdateUserEmail(t *testing.T) {
	env := setupAdminEnv(t)

	form := url.Values{
		"email": {"newemail@test.com"},
		"role":  {"admin"},
	}
	req := httptest.NewRequest("POST", "/admin/users/1/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminCannotDemoteLastAdmin(t *testing.T) {
	env := setupAdminEnv(t)

	// Only one admin exists. Try to change their role to user.
	form := url.Values{
		"email": {"admin@test.com"},
		"role":  {"user"},
	}
	req := httptest.NewRequest("POST", "/admin/users/1/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (re-render with error), got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Cannot remove admin role from the last admin") {
		t.Errorf("expected last admin error, got: %s", body)
	}
}

func TestAdminRoleChangeInvalidatesSessions(t *testing.T) {
	env := setupAdminEnv(t)

	// Create a second user (regular).
	auth.CreateUser(env.database, "bob@test.com", "password")

	// Insert a session for bob (user_id=2).
	_, err := env.database.Exec(
		"INSERT INTO sessions (token, data, expiry, user_id) VALUES (?, ?, ?, ?)",
		"bob-session", []byte("data"), time.Now().Add(time.Hour).Unix(), 2,
	)
	if err != nil {
		t.Fatalf("inserting session: %v", err)
	}

	// Change bob's role to admin.
	form := url.Values{
		"email": {"bob@test.com"},
		"role":  {"admin"},
	}
	req := httptest.NewRequest("POST", "/admin/users/2/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify bob's sessions were invalidated.
	var count int
	env.database.QueryRow("SELECT COUNT(*) FROM sessions WHERE user_id = 2").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 sessions for bob after role change, got %d", count)
	}
}
