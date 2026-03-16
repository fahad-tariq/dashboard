# Multi-User Support

## Overview

Add multi-user support so each person gets their own personal tasks, goals, ideas, and explorations, while the family task list remains shared. Users are managed via CLI (no self-registration). The existing session infrastructure carries forward -- only the credential verification and service resolution change.

## Current State

**Architecture:**
- Go 1.26 / chi v5 / SQLite (`modernc.org/sqlite`) / HTML templates / HTMX + SSE
- Auth in place: bcrypt password from env var, server-side sessions via `alexedwards/scs`, session stores boolean `authenticated` flag
- Services are singletons created at startup in `main.go`, shared by all requests
- Handlers hold direct references to service instances (`svc *Service`)
- `tracker.Handler` holds both `svc` (own list) and `otherSvc` (opposite list) for cross-list moves via `MoveToList`
- Ideas `ToTask` callback captures `personalSvc` in a closure at startup
- API routes (`/api/v1/*`) use bearer token auth, operate on singleton idea service

**Data storage:**
- Personal/family tasks: markdown file (source of truth) + SQLite `tracker_items` table (cache). `Store` scoped by `list` column.
- Ideas: file-per-idea in `/data/ideas/{status}/{slug}.md`, no DB cache
- Explorations: file-per-exploration in `/data/explorations/{slug}.md`, no DB cache
- Uploads: shared `/data/uploads/`, referenced by filename

**File watcher:** watches idea/exploration directories recursively + parent dirs of `personal.md`/`family.md`. Broadcasts SSE events and calls `Resync()` on change. Callbacks are `func()` with no user context.

## Requirements

**Functional Requirements:**
1. The system MUST support 2-5 users, each with isolated personal tasks, goals, ideas, and explorations
2. The family task list MUST remain shared -- all users see and edit the same list
3. Users MUST be created via a CLI subcommand (`./dashboard useradd --email x --password y`)
4. The login form MUST accept email + password (replacing password-only)
5. The nav bar MUST display the logged-in user's name (local part of email)
6. Personal pages MUST show a subtle indicator of whose data is being viewed
7. Existing single-user data MUST be migrated to the first user created
8. The system MUST continue to work when only one user exists
9. Uploads MUST remain shared (no per-user scoping)
10. API routes (`/api/v1/*`) MUST continue using bearer token auth; they operate on user 1's data (the admin/first user) -- per-user API scoping is out of scope for now

**Technical Constraints:**
1. Per-user file layout: `/data/users/{user_id}/personal.md`, `/data/users/{user_id}/ideas/...`, `/data/users/{user_id}/explorations/...`
2. Family file stays at `/data/family.md` (shared)
3. `tracker_items` table MUST add a `user_id` column (INTEGER, DEFAULT 1 for existing rows)
4. Session MUST store `user_id` (int) instead of boolean `authenticated`
5. Services MUST be resolved per-request based on the authenticated user, not created as singletons
6. File watcher MUST watch per-user directories for all users
7. `tracker.Store` MUST use two constructors: `NewUserStore(db, listName, userID)` for per-user lists and `NewSharedStore(db, listName)` for the family list -- no magic sentinel values
8. Family store inserts MUST set `user_id` to the creating user's ID (for future "added by" attribution), but queries MUST NOT filter by `user_id`
9. Registry MUST separate directory provisioning (`EnsureUserDirs`) from service resolution (`ForUser`) -- no side effects in getters

**CRITICAL: DO NOT create a second user until Phase 3b is complete.** Between Phase 2 (auth changes) and Phase 3b (handler migration), all users see the same personal data. With a single user this is harmless; with two users it is a data integrity risk.

## Assumptions

- User IDs are auto-incrementing integers (SQLite AUTOINCREMENT)
- The first user created gets ID 1, which matches the `DEFAULT 1` on existing `tracker_items` rows
- No concurrent user creation (CLI only, admin runs it once per user)
- Email is the unique identifier for login (no separate username)
- Legacy `DASHBOARD_PASSWORD_HASH` env var: if set and no users exist in the DB, auto-create a default user with email `admin@localhost` and that password hash on startup. This avoids a separate legacy auth path.

## Success Criteria

1. `./dashboard useradd --email alice@example.com --password secret` creates a user
2. Login with email + password authenticates and creates a session with the user's ID
3. Each user sees only their own personal tasks, ideas, and explorations
4. All users see and can edit the shared family task list
5. Moving tasks between personal and family works correctly (personal is per-user, family is shared)
6. Converting an idea to a task adds it to the logged-in user's personal list
7. Existing data (personal tasks, ideas, explorations) is accessible to the first user created
8. File changes trigger SSE updates and pages refresh correctly
9. The homepage shows per-user personal tasks + shared family tasks + per-user ideas/explorations
10. Personal pages show whose data is being viewed (e.g. "alice's list")
11. Logging out and logging in as a different user shows different personal data
12. Login form remembers the last email via localStorage
13. All existing tests pass (with updates for the new auth model)
14. New tests cover: user creation, email+password login, per-user data isolation, shared family access, cross-list moves, ToTask conversion
15. Linting passes, build succeeds (`CGO_ENABLED=0 go build`)

---

## Development Plan

### Phase 1: Database Schema and User Management

- [ ] Add migrations to `internal/db/migrations.go`:
  - Users table: `id INTEGER PRIMARY KEY AUTOINCREMENT, email TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, created_at TEXT NOT NULL`
  - Add `user_id INTEGER NOT NULL DEFAULT 1` column to `tracker_items`
- [ ] Create `internal/auth/users.go` with user CRUD operations:
  - `CreateUser(db, email, password) (int64, error)` -- hash password with bcrypt, insert row
  - `FindByEmail(db, email) (*User, error)` -- return user or nil
  - `UserCount(db) (int, error)` -- count users (used to check if users table is populated)
  - `User` struct: `ID int64`, `Email string`, `PasswordHash string`, `CreatedAt string`
- [ ] Add CLI subcommand handling in `cmd/dashboard/main.go`: when `os.Args[1] == "useradd"`, parse `--email` and `--password` flags, call `CreateUser`, print confirmation, and exit (do not start the HTTP server)
- [ ] Add legacy password migration: on startup, if `DASHBOARD_PASSWORD_HASH` is set and `UserCount == 0`, auto-create a user with email `admin@localhost` and the provided hash, then log a message telling the admin to update their email
- [ ] Write tests: user creation, duplicate email rejection, FindByEmail returns correct user, FindByEmail returns nil for unknown email, UserCount accuracy, legacy hash auto-creates user
- [ ] Verify build and all tests pass
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 2: Auth Changes (Email + Password Login, User ID Sessions)

- [ ] Update `internal/auth/handler.go`:
  - `Handler` struct: replace `passwordHash string` with `db *sql.DB` (to query users table)
  - `NewHandler`: update signature to accept `db *sql.DB` instead of password hash
  - `LoginSubmit`: read `email` + `password` from form; call `FindByEmail`; if no user found or bcrypt mismatch, show error; on success, store `user_id` (int64) and `user_email` (string) in session instead of boolean `authenticated`
  - `LoginPage`: pass email value back to template for re-rendering on error
- [ ] Update `internal/auth/middleware.go`:
  - `RequireAuth`: check `sm.GetInt64(ctx, "user_id") > 0` instead of `sm.GetBool(ctx, "authenticated")`; inject `user_id` and `user_email` into request context via `context.WithValue`
  - `RequireAuthAPI`: same user_id check, return 401 if missing
  - Export helpers: `UserID(ctx) int64` and `UserEmail(ctx) string` to extract from context
- [ ] Update `web/templates/login.html`:
  - Add email input field (`type="email"`) above the password field, with `autofocus` on email
  - Pass `next` and `email` values through hidden input and value attributes
  - Add localStorage pre-fill: on page load, read `lastEmail` from localStorage and pre-fill the email field; on form submit, save email to localStorage
  - Add a small "Not {name}?" link that clears localStorage and resets the email field (only shown when pre-filled)
- [ ] Update `web/templates/layout.html`:
  - Display the local part of the user's email (e.g. "alice") in the nav bar, positioned between theme toggle and logout button
  - Style with `color: var(--fg-dim)`, hide on mobile (under 768px) to avoid nav crowding
- [ ] Update `cmd/dashboard/main.go`: pass `db` to `auth.NewHandler` instead of `passwordHash`; remove `DASHBOARD_PASSWORD_HASH` from auth handler wiring (legacy migration in Phase 1 handles it)
- [ ] Update existing auth tests (`test/auth_test.go`): tests currently use a bcrypt hash string directly; update to create a test user in a temp database, then login with email+password. Update middleware tests to check for `user_id` in context instead of `authenticated` boolean.
- [ ] Write new tests: login with email+password succeeds, wrong email fails, wrong password fails, session contains user_id, middleware injects user_id into context, middleware rejects requests without user_id, UserEmail helper returns correct value
- [ ] Verify build and all tests pass
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 3a: Service Registry and Store Changes

This phase builds the per-user infrastructure without changing handlers yet.

- [ ] Add `UserDataDir string` to `Config` (env var `USER_DATA_DIR`, default `/data/users`)
- [ ] Update `tracker.Store` with two constructors (no magic sentinel values):
  - `NewUserStore(db, listName, userID int64)` -- all queries filter by `list AND user_id`
  - `NewSharedStore(db, listName)` -- all queries filter by `list` only, never by `user_id`
  - Both share the same `Store` struct internally (e.g. a `shared bool` field controls query behaviour)
  - `ReplaceAll` for shared store: delete/insert with `list` filter only; set `user_id` column to the provided value on insert (for attribution, passed as a parameter)
  - `Summary` for shared store: filter by `list` only
- [ ] Create `internal/services/registry.go`:
  - `Registry` struct holding: `*sql.DB`, `Config`, shared family `*tracker.Service` (with shared store)
  - `EnsureUserDirs(userID int64) error` -- creates per-user directories and skeleton files: `/data/users/{id}/personal.md`, `/data/users/{id}/ideas/{untriaged,parked,dropped,research}/`, `/data/users/{id}/explorations/`. Called during user creation and on startup for existing users. Idempotent.
  - `ForUser(userID int64) *UserServices` -- pure lookup, returns cached `UserServices` struct containing: `Personal *tracker.Service`, `Ideas *ideas.Service`, `Explorations *exploration.Service`. Panics if user dirs don't exist (caller must ensure dirs exist first).
  - Caches per-user service instances in a `sync.Mutex`-guarded `map[int64]*UserServices`
  - `Family() *tracker.Service` -- returns the shared family service
  - `InitAll(db) error` -- on startup, iterates all users in the DB, calls `EnsureUserDirs` and `ForUser` to warm the cache
- [ ] Write tests: `EnsureUserDirs` creates correct directory structure, `EnsureUserDirs` is idempotent, `ForUser` returns cached instances (same pointer on second call), `NewUserStore` filters by user_id, `NewSharedStore` does not filter by user_id, two user stores with different IDs don't interfere, shared store inserts preserve the provided user_id for attribution
- [ ] Verify build and all tests pass
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 3b: Handler Migration to Registry

- [ ] Update `tracker.Handler`:
  - Replace `svc *Service` and `otherSvc *Service` fields with `registry *services.Registry` and `listName string`
  - Add a private helper to resolve services per-request: for personal handler, `svc = registry.ForUser(auth.UserID(ctx)).Personal` and `otherSvc = registry.Family()`; for family handler, `svc = registry.Family()` and `otherSvc = registry.ForUser(auth.UserID(ctx)).Personal`
  - This ensures `MoveToList` correctly moves between per-user personal and shared family
- [ ] Update `ideas.Handler`:
  - Replace `svc *Service` with `registry *services.Registry`
  - Resolve via `registry.ForUser(auth.UserID(r.Context())).Ideas` per request
  - Update `ToTaskFunc` signature to accept `context.Context` as first parameter: `type ToTaskFunc func(ctx context.Context, title, body string, tags []string) error`
  - The `ToTask` closure in `main.go` uses `registry.ForUser(auth.UserID(ctx)).Personal.AddItem(item)`
- [ ] Update `exploration.Handler`:
  - Replace `svc *Service` with `registry *services.Registry`
  - Resolve via `registry.ForUser(auth.UserID(r.Context())).Explorations` per request
- [ ] Update `homePage` in `main.go`:
  - Resolve per-user services from registry using `auth.UserID(r.Context())`
  - Family data comes from `registry.Family()`
- [ ] Update handler constructors in `main.go` to pass registry instead of service instances
- [ ] Add per-user indicator to personal page templates: show `{name}'s list` (local part of email) as a subtitle on personal tasks, ideas, and explorations pages. Use `auth.UserEmail(ctx)` to get the email, extract the local part. Do NOT add this to the family page.
- [ ] Update homepage empty state to include user name: "Nothing here yet, {name}."
- [ ] API routes: update `main.go` to resolve API idea service from `registry.ForUser(1)` (first user / admin) so API continues to work on user 1's data. Add a comment noting this is a deliberate scoping decision.
- [ ] Write tests: personal handler for user 1 doesn't see user 2's tasks, family handler shows same data for both users, MoveToList from personal to family works (item leaves user's personal, appears in shared family), ToTask from ideas creates task in the correct user's personal list
- [ ] Verify build and all tests pass
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 4: File Watcher and Data Migration

- [ ] Update `internal/watcher/watcher.go`:
  - Extend `classifyEvent` to extract user_id from paths under `UserDataDir` (e.g. `/data/users/3/personal.md` -> user_id=3, category="personal")
  - Change callback type to support user-scoped callbacks: `func(userID int64, category string)` where `userID=0` means shared (family)
  - Family watcher stays unchanged (file-level category for `/data/family.md`, calls callback with `userID=0`)
- [ ] Update watcher wiring in `main.go`:
  - Register the `UserDataDir` as a watched directory
  - On per-user file changes, call `registry.ForUser(userID).Personal.Resync()` (or the appropriate service depending on category)
  - On family changes, call `registry.Family().Resync()` as before
- [ ] SSE: broadcast `file-changed` to all connected clients (with 2-5 users the overhead of refetching is negligible; the server returns user-scoped data anyway so each user sees only their own content)
- [ ] Create data migration as a CLI subcommand `./dashboard migrate-data --user-id N`:
  - Move `/data/personal.md` -> `/data/users/N/personal.md` (if source exists and destination doesn't)
  - Move `/data/ideas/` contents -> `/data/users/N/ideas/` (preserve subdirectory structure)
  - Move `/data/explorations/` contents -> `/data/users/N/explorations/`
  - Leave `/data/family.md` in place
  - Idempotent -- skip files that already exist at destination
  - Log each file moved
- [ ] Add `USER_DATA_DIR` to `docker-compose.yml` environment section
- [ ] Write tests: watcher classifies per-user events correctly (extracts user_id from path), watcher classifies family events with userID=0, data migration moves files to correct locations, data migration is idempotent (second run moves nothing)
- [ ] Verify build and all tests pass
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 5: Final Verification

- [ ] Run full test suite and verify all tests pass
- [ ] Run linter (`go vet ./...`) and fix any issues
- [ ] Build application: `CGO_ENABLED=0 go build -ldflags="-s -w" ./cmd/dashboard`
- [ ] Manual end-to-end test:
  - Start fresh with `DASHBOARD_PASSWORD_HASH` set, no users -> verify auto-created `admin@localhost` user, can log in with email `admin@localhost`
  - Create user: `./dashboard useradd --email alice@test.com --password alice123`
  - Migrate data: `./dashboard migrate-data --user-id 1`
  - Log in as alice, verify existing data is visible, nav shows "alice", personal page shows "alice's list"
  - Create user 2: `./dashboard useradd --email bob@test.com --password bob456`
  - Log in as bob, verify personal tasks/ideas/explorations are empty with personalised empty state
  - Add personal tasks and ideas for bob, verify alice doesn't see them
  - Verify both users see the same family task list
  - Verify both users can add/complete family tasks
  - Move a task from bob's personal to family, verify alice sees it in family
  - Convert an idea to a task for bob, verify it appears in bob's personal list
  - Verify file watcher triggers SSE updates (edit a markdown file, see page refresh)
  - Verify the homepage shows per-user personal data + shared family data
  - Verify login form remembers last email via localStorage
  - Test logout and login as different user
- [ ] Perform critical self-review of all changes
- [ ] Verify all 15 success criteria are met
- [ ] STOP and wait for human review

---

## Notes

- **Why a service registry, not per-request service creation?** Services hold a `sync.RWMutex` that protects concurrent file access. Creating new services per-request would lose mutex coordination between concurrent requests for the same user. The registry caches services per-user so the mutex works correctly.
- **Why DEFAULT 1 on user_id?** The first user created gets ID 1 (SQLite AUTOINCREMENT). Setting DEFAULT 1 on the existing `tracker_items` rows means all existing data is automatically owned by the first user, with no explicit data migration needed for the DB.
- **Why two store constructors instead of a sentinel value?** `NewUserStore` always filters by `user_id`; `NewSharedStore` never does. This makes the intent explicit at construction time and eliminates conditional SQL logic. The shared store still accepts a `user_id` parameter on insert for attribution (who added this family task) but never filters by it on read.
- **Why separate EnsureUserDirs from ForUser?** Directory creation is a side effect that should happen explicitly (during user creation or startup), not hidden inside a getter. `ForUser` is a pure cache lookup.
- **API scoping.** API routes use bearer token auth and operate on user 1's data. Per-user API tokens are out of scope -- can be added later by mapping tokens to user IDs.
- **Uploads stay shared.** Image filenames are random hex strings with no ownership. Not worth the complexity for 2-5 family members.
- **Legacy password migration.** Instead of maintaining a separate auth path for `DASHBOARD_PASSWORD_HASH`, the system auto-creates a default user on startup. This collapses the auth flow to a single path.

---

## Working Notes (for executing agent)

