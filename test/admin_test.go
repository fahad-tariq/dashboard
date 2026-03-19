package test

import (
	"bytes"
	"context"
	"encoding/gob"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/db"
	"github.com/fahad/dashboard/internal/services"
)

// --- Role Assignment ---

func TestFirstUserGetsAdminRole(t *testing.T) {
	database := newTestDB(t)
	id, err := auth.CreateUser(database, "first@test.com", "", "password")
	if err != nil {
		t.Fatalf("creating first user: %v", err)
	}

	user, err := auth.FindByID(database, id)
	if err != nil {
		t.Fatalf("finding user: %v", err)
	}
	if user.Role != "admin" {
		t.Errorf("first user should be admin, got %q", user.Role)
	}
}

func TestSecondUserGetsUserRole(t *testing.T) {
	database := newTestDB(t)
	auth.CreateUser(database, "first@test.com", "", "password")

	id, err := auth.CreateUser(database, "second@test.com", "", "password")
	if err != nil {
		t.Fatalf("creating second user: %v", err)
	}

	user, err := auth.FindByID(database, id)
	if err != nil {
		t.Fatalf("finding user: %v", err)
	}
	if user.Role != "user" {
		t.Errorf("second user should be 'user', got %q", user.Role)
	}
}

func TestFirstUserWithHashGetsAdminRole(t *testing.T) {
	database := newTestDB(t)
	hash := hashPassword(t, "pass")

	id, err := auth.CreateUserWithHash(database, "admin@localhost", "", hash)
	if err != nil {
		t.Fatalf("creating user with hash: %v", err)
	}

	user, err := auth.FindByID(database, id)
	if err != nil {
		t.Fatalf("finding user: %v", err)
	}
	if user.Role != "admin" {
		t.Errorf("first user (via hash) should be admin, got %q", user.Role)
	}
}

// --- FindByID ---

func TestFindByID(t *testing.T) {
	database := newTestDB(t)
	id := createTestUser(t, database, "alice@test.com", "password")

	user, err := auth.FindByID(database, id)
	if err != nil {
		t.Fatalf("finding user: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.Email != "alice@test.com" {
		t.Errorf("email: got %q", user.Email)
	}
}

func TestFindByIDNotFound(t *testing.T) {
	database := newTestDB(t)

	user, err := auth.FindByID(database, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != nil {
		t.Error("expected nil for unknown ID")
	}
}

// --- UpdateUserEmail ---

func TestUpdateUserEmail(t *testing.T) {
	database := newTestDB(t)
	id := createTestUser(t, database, "old@test.com", "password")

	if err := auth.UpdateUserEmail(database, id, "new@test.com"); err != nil {
		t.Fatalf("updating email: %v", err)
	}

	user, _ := auth.FindByID(database, id)
	if user.Email != "new@test.com" {
		t.Errorf("expected new@test.com, got %q", user.Email)
	}
}

// --- UpdateUserPassword ---

func TestUpdateUserPassword(t *testing.T) {
	database := newTestDB(t)
	id := createTestUser(t, database, "alice@test.com", "oldpassword")

	if err := auth.UpdateUserPassword(database, id, "newpassword"); err != nil {
		t.Fatalf("updating password: %v", err)
	}

	user, _ := auth.FindByID(database, id)
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("newpassword")); err != nil {
		t.Error("new password hash should verify against 'newpassword'")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("oldpassword")); err == nil {
		t.Error("old password should no longer verify")
	}
}

// --- UpdateUserRole ---

func TestUpdateUserRole(t *testing.T) {
	database := newTestDB(t)
	id := createTestUser(t, database, "alice@test.com", "password")

	if err := auth.UpdateUserRole(database, id, "admin"); err != nil {
		t.Fatalf("updating role: %v", err)
	}

	user, _ := auth.FindByID(database, id)
	if user.Role != "admin" {
		t.Errorf("expected admin, got %q", user.Role)
	}
}

func TestUpdateUserRoleInvalid(t *testing.T) {
	database := newTestDB(t)
	id := createTestUser(t, database, "alice@test.com", "password")

	if err := auth.UpdateUserRole(database, id, "superadmin"); err == nil {
		t.Error("expected error for invalid role")
	}
}

// --- AdminCount ---

func TestAdminCount(t *testing.T) {
	database := newTestDB(t)

	count, err := auth.AdminCount(database)
	if err != nil {
		t.Fatalf("counting admins: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	// First user becomes admin.
	auth.CreateUser(database, "admin@test.com", "", "password")
	count, _ = auth.AdminCount(database)
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	// Second user is regular user.
	auth.CreateUser(database, "user@test.com", "", "password")
	count, _ = auth.AdminCount(database)
	if count != 1 {
		t.Errorf("expected still 1, got %d", count)
	}
}

// --- DeleteUser ---

func TestDeleteUserCascade(t *testing.T) {
	database := newTestDB(t)
	id := createTestUser(t, database, "alice@test.com", "password")

	// Insert a tracker item for this user.
	_, err := database.Exec(
		"INSERT INTO tracker_items (slug, title, type, user_id) VALUES (?, ?, ?, ?)",
		"task-1", "Task 1", "task", id,
	)
	if err != nil {
		t.Fatalf("inserting tracker item: %v", err)
	}

	// Insert a session for this user.
	err = database.QueryRow("SELECT 1").Err() // ensure DB works
	if err != nil {
		t.Fatalf("db check: %v", err)
	}
	_, err = database.Exec(
		"INSERT INTO sessions (token, data, expiry, user_id) VALUES (?, ?, ?, ?)",
		"tok-alice", []byte("data"), time.Now().Add(time.Hour).Unix(), id,
	)
	if err != nil {
		t.Fatalf("inserting session: %v", err)
	}

	if err := auth.DeleteUser(database, id); err != nil {
		t.Fatalf("deleting user: %v", err)
	}

	// User should be gone.
	user, _ := auth.FindByID(database, id)
	if user != nil {
		t.Error("user should be deleted")
	}

	// Tracker items should be gone.
	var itemCount int
	database.QueryRow("SELECT COUNT(*) FROM tracker_items WHERE user_id = ?", id).Scan(&itemCount)
	if itemCount != 0 {
		t.Errorf("tracker items should be deleted, got %d", itemCount)
	}

	// Sessions should be gone.
	var sessCount int
	database.QueryRow("SELECT COUNT(*) FROM sessions WHERE user_id = ?", id).Scan(&sessCount)
	if sessCount != 0 {
		t.Errorf("sessions should be deleted, got %d", sessCount)
	}
}

// --- InvalidateSessions ---

func TestInvalidateSessionsRemovesOnlyTargetUser(t *testing.T) {
	database := newTestDB(t)

	// Insert sessions for two users.
	_, err := database.Exec(
		"INSERT INTO sessions (token, data, expiry, user_id) VALUES (?, ?, ?, ?)",
		"tok-1", []byte("data"), time.Now().Add(time.Hour).Unix(), 1,
	)
	if err != nil {
		t.Fatalf("inserting session: %v", err)
	}
	_, err = database.Exec(
		"INSERT INTO sessions (token, data, expiry, user_id) VALUES (?, ?, ?, ?)",
		"tok-2", []byte("data"), time.Now().Add(time.Hour).Unix(), 2,
	)
	if err != nil {
		t.Fatalf("inserting session: %v", err)
	}

	if err := auth.InvalidateSessions(database, 1); err != nil {
		t.Fatalf("invalidating sessions: %v", err)
	}

	var count1, count2 int
	database.QueryRow("SELECT COUNT(*) FROM sessions WHERE user_id = 1").Scan(&count1)
	database.QueryRow("SELECT COUNT(*) FROM sessions WHERE user_id = 2").Scan(&count2)

	if count1 != 0 {
		t.Errorf("user 1 sessions should be gone, got %d", count1)
	}
	if count2 != 1 {
		t.Errorf("user 2 sessions should remain, got %d", count2)
	}
}

// --- Session store user_id ---

// gobEncodeSession encodes session data in the same format SCS uses.
func gobEncodeSession(t *testing.T, values map[string]any) []byte {
	t.Helper()
	aux := &struct {
		Deadline time.Time
		Values   map[string]any
	}{
		Deadline: time.Now().Add(time.Hour),
		Values:   values,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(aux); err != nil {
		t.Fatalf("encoding session data: %v", err)
	}
	return buf.Bytes()
}

func TestStoreCommitWritesUserID(t *testing.T) {
	database := newTestDB(t)
	store := auth.NewSQLiteStore(database)

	data := gobEncodeSession(t, map[string]any{"user_id": int64(42)})
	if err := store.Commit("tok-test", data, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var uid int64
	if err := database.QueryRow("SELECT user_id FROM sessions WHERE token = ?", "tok-test").Scan(&uid); err != nil {
		t.Fatalf("querying user_id: %v", err)
	}
	if uid != 42 {
		t.Errorf("expected user_id=42, got %d", uid)
	}
}

func TestStoreCommitDefaultsUserIDToZero(t *testing.T) {
	database := newTestDB(t)
	store := auth.NewSQLiteStore(database)

	if err := store.Commit("tok-noid", []byte("data"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var uid int64
	if err := database.QueryRow("SELECT user_id FROM sessions WHERE token = ?", "tok-noid").Scan(&uid); err != nil {
		t.Fatalf("querying user_id: %v", err)
	}
	if uid != 0 {
		t.Errorf("expected user_id=0, got %d", uid)
	}
}

// --- Lazy ForUser ---

func TestForUserCreatesDirectoriesLazily(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	userDataDir := filepath.Join(tmpDir, "users")
	familyPath := filepath.Join(tmpDir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)

	reg := services.NewRegistry(database, userDataDir, familyPath, time.UTC)

	// ForUser should NOT panic -- it should lazily create directories.
	svc := reg.ForUser(1)
	if svc == nil {
		t.Fatal("ForUser should return non-nil services")
	}

	// Verify directories were created.
	base := filepath.Join(userDataDir, "1")
	if _, err := os.Stat(base); os.IsNotExist(err) {
		t.Error("user directory should have been created lazily")
	}
	if _, err := os.Stat(filepath.Join(base, "personal.md")); os.IsNotExist(err) {
		t.Error("personal.md should have been created lazily")
	}
}

// --- EvictUser ---

func TestEvictUserRemovesFromCache(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer database.Close()

	userDataDir := filepath.Join(tmpDir, "users")
	familyPath := filepath.Join(tmpDir, "family.md")
	os.WriteFile(familyPath, []byte("# Family\n\n"), 0o644)

	reg := services.NewRegistry(database, userDataDir, familyPath, time.UTC)

	svc1 := reg.ForUser(1)
	reg.EvictUser(1)
	svc2 := reg.ForUser(1)

	// After eviction, a new instance should be returned.
	if svc1 == svc2 {
		t.Error("after eviction, ForUser should return a fresh instance")
	}
}

// --- AllUsers includes role ---

func TestAllUsersIncludesRole(t *testing.T) {
	database := newTestDB(t)
	auth.CreateUser(database, "admin@test.com", "", "password")
	auth.CreateUser(database, "user@test.com", "", "password")

	users, err := auth.AllUsers(database)
	if err != nil {
		t.Fatalf("listing users: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Role != "admin" {
		t.Errorf("first user role: expected admin, got %q", users[0].Role)
	}
	if users[1].Role != "user" {
		t.Errorf("second user role: expected user, got %q", users[1].Role)
	}
}

// --- FindByEmail includes role ---

func TestFindByEmailIncludesRole(t *testing.T) {
	database := newTestDB(t)
	auth.CreateUser(database, "admin@test.com", "", "password")

	user, err := auth.FindByEmail(database, "admin@test.com")
	if err != nil {
		t.Fatalf("finding user: %v", err)
	}
	if user.Role != "admin" {
		t.Errorf("expected admin, got %q", user.Role)
	}
}

// --- RequireAdmin middleware ---

func setupAdminSession(t *testing.T, sm *scs.SessionManager, userID int64, email string, isAdmin bool) *http.Cookie {
	t.Helper()
	setup := sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sm.Put(r.Context(), "user_id", userID)
		sm.Put(r.Context(), "user_email", email)
		sm.Put(r.Context(), "is_admin", isAdmin)
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/setup", nil)
	rr := httptest.NewRecorder()
	setup.ServeHTTP(rr, req)

	for _, c := range rr.Result().Cookies() {
		if c.Name == "session" {
			return c
		}
	}
	t.Fatal("no session cookie set")
	return nil
}

func TestRequireAdminPassesForAdmin(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "admin@test.com", "password")
	cookie := setupAdminSession(t, sm, 1, "admin@test.com", true)

	var gotAdmin bool
	handler := sm.LoadAndSave(auth.RequireAuth(sm)(auth.RequireAdmin(sm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAdmin = auth.IsAdmin(r.Context())
		w.WriteHeader(http.StatusOK)
	}))))

	req := httptest.NewRequest("GET", "/admin/users", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", rr.Code)
	}
	if !gotAdmin {
		t.Error("IsAdmin should return true for admin user")
	}
}

func TestRequireAdminBlocksRegularUser(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "admin@test.com", "password")
	createTestUser(t, database, "user@test.com", "password")
	cookie := setupAdminSession(t, sm, 2, "user@test.com", false)

	handler := sm.LoadAndSave(auth.RequireAuth(sm)(auth.RequireAdmin(sm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))))

	req := httptest.NewRequest("GET", "/admin/users", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for regular user, got %d", rr.Code)
	}
}

func TestIsAdminHelperReturnsFalseWithoutContext(t *testing.T) {
	if auth.IsAdmin(context.Background()) {
		t.Error("IsAdmin should return false for empty context")
	}
}

func TestRequireAuthSetsIsAdmin(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "admin@test.com", "password")
	cookie := setupAdminSession(t, sm, 1, "admin@test.com", true)

	var gotAdmin bool
	handler := sm.LoadAndSave(auth.RequireAuth(sm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAdmin = auth.IsAdmin(r.Context())
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !gotAdmin {
		t.Error("RequireAuth should inject is_admin into context")
	}
}
