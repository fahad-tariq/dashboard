# Admin UI and User Roles

## Overview

Add an in-app admin interface for user management so the dashboard can be administered entirely through the browser. This replaces the CLI as the primary user management method, which is necessary because the app runs exclusively as a Docker container on a remote server.

## Current State

- Multi-user support already implemented: users table, email+password login, per-user data isolation, service registry
- User CRUD functions exist in `internal/auth/users.go`: `CreateUser`, `CreateUserWithHash`, `FindByEmail`, `UserCount`, `AllUsers`
- Missing: `DeleteUser`, `UpdateUserEmail`, `UpdateUserPassword`, `FindByID`
- No `role` or `is_admin` column in the users table
- CLI subcommands `useradd` and `migrate-data` exist but are impractical for Docker deployments
- The first user is auto-created from `DASHBOARD_PASSWORD_HASH` on startup
- Templates use `.form-input`, `.form-btn`, `.action-btn` classes consistently
- Nav bar already conditionally shows auth elements via `{{if authEnabled}}`
- `.login-error` class is defined inline in `login.html`, not in `theme.css`
- `registry.ForUser` panics if user dirs don't exist; `EnsureUserDirs` must be called first
- Sessions table has no `user_id` column -- no way to invalidate sessions for a specific user

## Requirements

**Functional Requirements:**
1. The users table MUST have a role column with two values: `admin` and `user`
2. The first user created MUST automatically be an `admin`
3. Admins MUST be able to: list all users, create new users, edit user email, reset user password, delete users, and change user roles
4. Regular users MUST NOT see or access admin pages
5. An admin MUST NOT be able to delete themselves
6. An admin MUST NOT be able to demote themselves if they are the last admin
7. The nav bar MUST show an "admin" link for admin users only
8. The admin UI MUST be styled consistently with the existing Catppuccin theme
9. The `useradd` CLI MUST continue working as a bootstrap mechanism (first user on fresh install)
10. Deleting a user MUST cascade: remove `tracker_items` rows, invalidate active sessions, remove files on disk, evict registry cache
11. Changing a user's role MUST invalidate their active sessions (forces re-login with new role)
12. Admin actions MUST show success/error feedback via flash messages
13. Delete actions MUST require a browser `confirm()` dialog

**Technical Constraints:**
1. Admin routes at `/admin/users` protected by `RequireAdmin` middleware
2. Role stored as `TEXT NOT NULL DEFAULT 'user'` in users table
3. Admin status available in request context alongside user_id and user_email
4. Admin pages use `layout.html` (with nav, SSE) like all other pages
5. No JavaScript frameworks -- standard HTML forms with POST actions
6. Add `user_id INTEGER` column to sessions table for targeted session invalidation
7. `registry.ForUser` MUST call `EnsureUserDirs` lazily instead of panicking -- eliminates the need for callers to provision dirs
8. Move `.login-error` to `theme.css` as `.form-error` for reuse across admin templates
9. Flash messages via query params (`?msg=user-created`) with a `.flash-msg` CSS class
10. Admin link in nav positioned in utility area (outside `.nav-links`, near theme toggle), styled with `var(--mauve)` to be visually distinct

## Success Criteria

1. First user created is automatically admin
2. Admin sees "admin" link in nav (distinct colour); regular user does not
3. Admin can list, create, edit, reset password, delete users, and toggle roles from `/admin/users`
4. Regular user accessing `/admin/*` gets 403
5. Admin cannot delete themselves
6. Admin cannot remove admin role from themselves if they are the last admin
7. Deleting a user removes their DB rows, sessions, files, and registry cache
8. Changing a user's role invalidates their sessions
9. Success/error messages displayed after admin actions
10. Delete button shows browser confirm dialog
11. New users created via admin UI get their data directories provisioned automatically
12. Existing CLI `useradd` still works and creates admin if first user
13. All existing tests pass
14. New tests cover: admin middleware, user CRUD, role assignment, cascading delete, session invalidation, self-deletion prevention
15. Linting passes, build succeeds

---

## Development Plan

### Phase 1: Schema, Role Model, Session Invalidation, and User CRUD

These tasks are independent and can be done in parallel:

**Schema (internal/db/migrations.go):**
- [ ] Add migration: `ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user'`
- [ ] Add migration: `ALTER TABLE sessions ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0`

**User model (internal/auth/users.go):**
- [ ] Update `User` struct to include `Role string` field
- [ ] Update `AllUsers`, `FindByEmail` to include `role` in SELECT
- [ ] Add new functions:
  - `FindByID(db, id) (*User, error)`
  - `UpdateUserEmail(db, id, newEmail) error`
  - `UpdateUserPassword(db, id, newPassword) error` -- bcrypt hash the new password
  - `UpdateUserRole(db, id, role) error` -- validate role is "admin" or "user"
  - `DeleteUser(db, id) error` -- transaction: delete from `users`, `tracker_items`, and `sessions` where `user_id = id`
  - `AdminCount(db) (int, error)` -- count users with role="admin"
  - `InvalidateSessions(db, userID) error` -- `DELETE FROM sessions WHERE user_id = ?`
- [ ] Update `CreateUser` and `CreateUserWithHash`: if `UserCount == 0`, set role to `admin`

**Session store (internal/auth/store.go):**
- [ ] Update `Commit` to accept and store `user_id` in the sessions table (SCS calls Commit with the session data; the user_id needs to be extracted and stored in the column for targeted invalidation)
- [ ] Alternative simpler approach: `InvalidateSessions` in users.go directly deletes by user_id column; the `Commit` method writes the user_id column alongside the session data

**Auth handler (internal/auth/handler.go):**
- [ ] Update `LoginSubmit`: when committing the session, also store the user's role (`is_admin` bool) in session; ensure the sessions row gets the `user_id` column set

**Registry (internal/services/registry.go):**
- [ ] Change `ForUser` to call `EnsureUserDirs` lazily on cache miss instead of panicking
- [ ] Add `EvictUser(userID int64)` method to remove a user from the cache

**Tests:**
- [ ] Write tests: FindByID, UpdateUserEmail, UpdateUserPassword (verify bcrypt), UpdateUserRole (valid and invalid roles), DeleteUser (cascades tracker_items and sessions), AdminCount, first user gets admin role, second user gets user role, InvalidateSessions removes only that user's sessions, ForUser creates dirs lazily (no panic)
- [ ] Verify build and all tests pass
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 2: Admin Middleware, Context, and CSS

- [ ] Update `internal/auth/middleware.go`:
  - `RequireAuth`: also inject `is_admin` into request context from session
  - Add `IsAdmin(ctx) bool` helper function
  - Add `RequireAdmin(sm)` middleware: checks `IsAdmin(ctx)`, returns 403 with a simple "Forbidden" message if not admin
- [ ] Add `isAdmin` template function (similar to existing `authEnabled`) so templates can conditionally render admin elements
- [ ] Update `web/templates/layout.html`:
  - Add admin link in utility area (outside `.nav-links`, between theme toggle and username): `{{if isAdmin}}<a href="/admin/users" class="nav-admin">admin</a>{{end}}`
  - Add flash message rendering at the top of `<main>`: check for `.FlashMsg` in template data and render a `.flash-msg` banner
- [ ] Update `web/static/theme.css`:
  - Move `.login-error` inline styles to `.form-error` class in theme.css
  - Add `.nav-admin` style: `color: var(--mauve)`, same sizing as other nav utility elements
  - Add `.flash-msg` style: subtle banner with `var(--green)` for success, `var(--red)` for errors, auto-dismiss or closable
  - Add `.admin-form-card` style: `max-width: 480px; margin: 0 auto` with card background/border
- [ ] Update `web/templates/login.html`: replace inline `.login-error` with `.form-error` class from theme.css
- [ ] Write tests: RequireAdmin passes for admin, RequireAdmin blocks non-admin with 403, IsAdmin helper returns correct value
- [ ] Verify build and all tests pass
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 3: Admin Pages and Route Wiring

- [ ] Create `internal/admin/handler.go` with `AdminHandler` struct holding `db *sql.DB`, `registry *services.Registry`, `templates map[string]*template.Template`:
  - `ListUsers(w, r)` -- GET `/admin/users`: render list of all users with flash message support
  - `NewUserForm(w, r)` -- GET `/admin/users/new`: render create form (email, password, role)
  - `CreateUser(w, r)` -- POST `/admin/users/new`: validate, create user (dirs auto-provisioned via lazy ForUser), redirect to `/admin/users?msg=user-created`
  - `EditUserForm(w, r)` -- GET `/admin/users/{id}/edit`: form pre-filled with current email and role
  - `UpdateUser(w, r)` -- POST `/admin/users/{id}/edit`: update email and/or role; prevent last-admin demotion; on role change call `InvalidateSessions`; redirect with flash
  - `ResetPasswordForm(w, r)` -- GET `/admin/users/{id}/password`: new password + confirm password fields only (no current password -- admin doesn't need it)
  - `ResetPassword(w, r)` -- POST `/admin/users/{id}/password`: validate passwords match, hash and update, invalidate user's sessions, redirect with flash
  - `DeleteUser(w, r)` -- POST `/admin/users/{id}/delete`: prevent self-deletion, call `auth.DeleteUser` (cascading), call `registry.EvictUser`, remove user's data directory (`os.RemoveAll`), redirect with flash
- [ ] Create `web/templates/admin-users.html`: list page using `layout.html` base
  - Flat `.backlog-item` card per user (not tracker expand/collapse)
  - Each card: email (bold) + created date (dim) on left; role badge (`.badge`) + action buttons (edit, password, delete) on right
  - Delete button has `onclick="return confirm('Delete user {{.Email}}?')"`
  - "New user" button at top
  - Flash message area at top
  - Breadcrumb: `admin / users`
  - Mobile: flex-wrap so action buttons wrap below email on narrow screens
- [ ] Create `web/templates/admin-user-form.html`: create/edit form using `layout.html` base
  - Wrapped in `.admin-form-card` (constrained width, centred)
  - Email field (`.form-input`, type="email"), role dropdown (`.form-select`), password field (only on create)
  - Error display using `.form-error`
  - Breadcrumb back to user list
- [ ] Create `web/templates/admin-password.html`: password reset form using `layout.html` base
  - New password + confirm password fields only
  - Client-side match validation via `onsubmit`
  - Server-side mismatch check as fallback
  - `.admin-form-card` layout with breadcrumb
- [ ] Add admin templates to `parseTemplates()` in `main.go`
- [ ] Wire admin routes in `main.go` inside the auth-protected group, wrapped with `RequireAdmin`:
  ```
  r.Route("/admin", func(r chi.Router) {
      r.Use(auth.RequireAdmin(sm))
      GET  /users              -> ListUsers
      GET  /users/new          -> NewUserForm
      POST /users/new          -> CreateUser
      GET  /users/{id}/edit    -> EditUserForm
      POST /users/{id}/edit    -> UpdateUser
      GET  /users/{id}/password -> ResetPasswordForm
      POST /users/{id}/password -> ResetPassword
      POST /users/{id}/delete  -> DeleteUser
  })
  ```
- [ ] Write tests: admin can list users, admin can create user, admin can edit email, admin can change role (not last admin), admin can reset password, admin cannot delete self, non-admin gets 403, flash messages appear after actions, cascading delete removes tracker_items and sessions and evicts cache
- [ ] Verify build and all tests pass
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 4: Final Verification

- [ ] Run full test suite and verify all tests pass
- [ ] Run `go vet ./...` and fix any issues
- [ ] Build: `CGO_ENABLED=0 go build -ldflags="-s -w" ./cmd/dashboard`
- [ ] Manual smoke test:
  - Fresh start with `DASHBOARD_PASSWORD_HASH` set -> auto-created admin@localhost is admin role
  - Log in as admin, verify "admin" link in nav (mauve colour, utility area)
  - Navigate to /admin/users, see user list with admin@localhost
  - Create new user bob@test.com with role "user" -> flash "User created" appears
  - Log out, log in as bob -> no "admin" link in nav
  - Bob tries to access /admin/users directly -> 403
  - Log back in as admin, edit bob's email -> flash "User updated"
  - Reset bob's password -> flash "Password reset"
  - Change bob's role to admin -> bob's sessions invalidated
  - Try to demote self (should be prevented -- last admin)
  - Try to delete self (should be prevented)
  - Delete bob -> confirm dialog -> flash "User deleted" -> bob's files removed
  - Verify CLI `./dashboard useradd` still works
- [ ] Perform critical self-review of all changes
- [ ] Verify all 15 success criteria met
- [ ] STOP and wait for human review

---

## Notes

- **Why not a separate admin app?** 2-5 person family dashboard. A few pages behind a role check is proportionate.
- **Why POST for delete?** HTML forms only support GET and POST. `/delete` suffix with POST is the existing codebase pattern.
- **CLI useradd stays.** Bootstrap mechanism for first user on fresh install. Admin UI is the primary method once the first admin exists.
- **Session invalidation on role change/delete.** The `user_id` column on sessions enables targeted `DELETE FROM sessions WHERE user_id = ?`. This forces re-login, picking up the new role (or preventing login for deleted users).
- **Lazy EnsureUserDirs.** `ForUser` calls `EnsureUserDirs` on cache miss. This eliminates panics and removes the need for every caller to provision dirs explicitly.
- **Flash messages.** Query-param approach (`?msg=key`) mapped to display text. No session-based flash needed. Simple and stateless.

---

## Working Notes (for executing agent)

