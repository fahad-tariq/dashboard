package test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fahad/dashboard/internal/app"
	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/commentary"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
)

// authRouter builds the real router with auth on and the given users. files
// maps paths relative to USER_DATA_DIR to content written before start-up.
func authRouter(t *testing.T, users, files map[string]string) (http.Handler, *config.Config, *sql.DB) {
	t.Helper()
	paths := tempPaths(t)
	paths["DASHBOARD_PASSWORD_HASH"] = ""
	paths["DASHBOARD_SECURE_COOKIES"] = "false"
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
	for email, password := range users {
		if _, err := auth.CreateUser(database, email, "", password); err != nil {
			t.Fatal(err)
		}
	}
	for rel, content := range files {
		path := filepath.Join(cfg.UserDataDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	router, err := app.NewRouter(t.Context(), cfg, database, "test")
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return router, cfg, database
}

// login posts the login form and returns the session cookie.
func login(t *testing.T, h http.Handler, email, password string) *http.Cookie {
	t.Helper()
	form := url.Values{"email": {email}, "password": {password}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session" {
			return c
		}
	}
	t.Fatalf("login as %s: no session cookie (status %d)", email, rr.Code)
	return nil
}

func getAs(t *testing.T, h http.Handler, cookie *http.Cookie, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// No-auth mode serves user 1 through the same routes as auth mode, so pages
// that need a user, such as /account, work locally.
func TestNoAuthServesUserOne(t *testing.T) {
	h, _ := renderRouter(t)
	tests := map[string]struct {
		path string
		want string
	}{
		"account page":      {path: "/account", want: "Account Settings"},
		"greeting has name": {path: "/", want: ", local"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rr := getAs(t, h, nil, tc.path)
			if rr.Code != http.StatusOK {
				t.Fatalf("GET %s: status %d, want 200", tc.path, rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tc.want) {
				t.Errorf("GET %s: body lacks %q", tc.path, tc.want)
			}
		})
	}
}

// The trash purge iterates auth.AllUsers, so no-auth mode needs a user-1 row
// or it would purge nothing.
func TestNoAuthEnsuresUserOneRow(t *testing.T) {
	paths := tempPaths(t)
	paths["DASHBOARD_PASSWORD_HASH"] = ""
	paths["DASHBOARD_AUTH"] = "disabled"
	paths["ADDR"] = "127.0.0.1:0"
	setEnvForConfig(t, paths)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeDB(t, database) })
	if _, err := app.NewRouter(t.Context(), cfg, database, "test"); err != nil {
		t.Fatal(err)
	}
	u, err := auth.FindByID(database, 1)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		t.Fatal("no user 1 row after starting in no-auth mode")
	}
}

// No-auth mode keeps reading PERSONAL_PATH and IDEAS_PATH for user 1 rather
// than USER_DATA_DIR/1/.
func TestNoAuthUsesLegacyPaths(t *testing.T) {
	h, cfg := renderRouter(t)
	form := url.Values{"title": {"Water the ferns"}}
	req := httptest.NewRequest("POST", "/todos/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("POST /todos/add: status %d", rr.Code)
	}
	got, err := os.ReadFile(cfg.PersonalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Water the ferns") {
		t.Errorf("PERSONAL_PATH lacks the new task:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(cfg.UserDataDir, "1", "personal.md")); err == nil {
		t.Error("no-auth mode created USER_DATA_DIR/1/personal.md")
	}
}

// Commentary belongs to the logged-in user, not always user 1.
func TestCommentaryScopedToUser(t *testing.T) {
	idea := "# Ideas\n\n- [ ] Shared idea [status: untriaged] [added: 2026-09-10]\n"
	h, _, database := authRouter(t, map[string]string{
		"one@test.com": "password-one",
		"two@test.com": "password-two",
	}, map[string]string{"1/ideas.md": idea, "2/ideas.md": idea})
	store := commentary.NewStore(database)
	for uid, note := range map[int]string{1: "note for user one", 2: "note for user two"} {
		if err := store.Set("shared-idea", "ideas", uid, note); err != nil {
			t.Fatal(err)
		}
		if err := store.Set("a-task", "personal", uid, note); err != nil {
			t.Fatal(err)
		}
	}
	cookie := login(t, h, "two@test.com", "password-two")

	for name, path := range map[string]string{
		"web commentary": "/commentary/todos/a-task",
		"idea detail":    "/ideas/shared-idea",
	} {
		t.Run(name, func(t *testing.T) {
			rr := getAs(t, h, cookie, path)
			if rr.Code != http.StatusOK {
				t.Fatalf("GET %s: status %d", path, rr.Code)
			}
			body := rr.Body.String()
			if strings.Contains(body, "note for user one") {
				t.Errorf("user 2 sees user 1's commentary at %s", path)
			}
			if !strings.Contains(body, "note for user two") {
				t.Errorf("user 2 does not see their own commentary at %s", path)
			}
		})
	}
}

// A database first used in no-auth mode holds only the unusable local user.
// Starting it with auth on must behave as if it had no users: bootstrap from
// DASHBOARD_PASSWORD_HASH, or refuse to start without one.
func TestAuthModeIgnoresLocalPlaceholderUser(t *testing.T) {
	hash := "$2a$10$abcdefghijklmnopqrstuuJ5bW3pH0bQ0b9b7xg2y2Jm8m2pXq6m6"
	tests := map[string]struct {
		passwordHash string
		wantErr      bool
	}{
		"hash set bootstraps admin": {passwordHash: hash},
		"no hash refuses to start":  {wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			paths := tempPaths(t)
			paths["DASHBOARD_PASSWORD_HASH"] = ""
			paths["DASHBOARD_AUTH"] = "disabled"
			paths["ADDR"] = "127.0.0.1:0"
			setEnvForConfig(t, paths)
			cfg, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			database, err := db.Open(cfg.DBPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { closeDB(t, database) })
			if _, err := app.NewRouter(t.Context(), cfg, database, "test"); err != nil {
				t.Fatal(err)
			}

			cfg.AuthDisabled = false
			cfg.PasswordHash = tc.passwordHash
			_, err = app.NewRouter(t.Context(), cfg, database, "test")
			if tc.wantErr {
				if err == nil {
					t.Fatal("auth mode started with only the unusable local user")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewRouter in auth mode: %v", err)
			}
			u, err := auth.FindByEmail(database, "admin@localhost")
			if err != nil {
				t.Fatal(err)
			}
			if u == nil || u.PasswordHash != hash || u.Role != "admin" {
				t.Fatalf("admin@localhost not bootstrapped from DASHBOARD_PASSWORD_HASH: %+v", u)
			}
		})
	}
}
