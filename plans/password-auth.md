# Password Authentication

## Overview

Add lightweight password authentication to the dashboard using a bcrypt-hashed password stored in an environment variable, with server-side sessions via `alexedwards/scs` backed by SQLite. Auth is optional -- if no password hash is configured, the dashboard runs unauthenticated (backward compatible).

## Current State

**Technical Context:**
- Go 1.26 / chi v5 / SQLite (`modernc.org/sqlite`, pure Go) / HTML templates / HTMX + SSE
- Config loaded from env vars in `internal/config/config.go`
- All routes defined in `cmd/dashboard/main.go` with chi router
- Existing middleware: `RealIP`, `Recoverer`, `Compress(5)`
- API routes (`/api/v1/*`) have separate bearer token auth via `DASHBOARD_API_TOKEN`
- Templates use `layout.html` base with `{{template "content" .}}` blocks; layout includes `sse-connect="/events"` on `<body>` and a full nav bar
- Catppuccin dark/light theme with responsive mobile styles
- `/upload` (POST) and `/uploads/*` (GET) are registered at the top level in `main.go`, outside any route group
- Existing tests in `test/` cover tracker, ideas, exploration, and upload -- none test middleware or routing

**Key constraint:** The `scs` library's built-in `sqlite3store` depends on `mattn/go-sqlite3` (CGO). This project uses `modernc.org/sqlite` (pure Go, no CGO). A small adapter implementing the `scs.Store` interface (three methods: `Find`, `Commit`, `Delete`) is needed.

## Requirements

**Functional Requirements:**
1. The system MUST authenticate users via a single password checked against a bcrypt hash in `DASHBOARD_PASSWORD_HASH`
2. The system MUST display a login page when unauthenticated users access any protected route, preserving the original URL so the user returns there after login
3. The system MUST create a server-side session on successful login, stored in SQLite
4. The system MUST regenerate the session token on login to prevent session fixation
5. The system MUST provide a logout action in the nav bar that destroys the session
6. The system MUST skip auth entirely when `DASHBOARD_PASSWORD_HASH` is empty (backward compatible)
7. API routes (`/api/v1/*`) MUST continue using bearer token auth, unaffected by session auth
8. The login page MUST work well on both mobile and desktop, matching the existing Catppuccin theme
9. The system SHOULD rate-limit login attempts (5 per minute per IP) to prevent brute force
10. The system SHOULD log failed login attempts at WARN level

**Technical Constraints:**
1. Session store MUST use `modernc.org/sqlite` via a custom `scs.Store` adapter -- no CGO
2. The session cookie MUST use `HttpOnly`, `SameSite=Lax`, and `Secure: true` by default (with `DASHBOARD_INSECURE_COOKIES=true` opt-out for local dev)
3. The session table MUST be created via the existing migration system in `internal/db/migrations.go` -- use two separate migration entries (CREATE TABLE and CREATE INDEX) since `db.Exec` may not support multiple statements
4. Static assets (`/static/*`) and the login page (`/login`) MUST be accessible without authentication
5. All data-bearing routes MUST be behind auth: `/events` (SSE), `/upload`, `/uploads/*`, and all page/form routes
6. The SSE endpoint (`/events`) MUST return 401 (not redirect) for unauthenticated requests, so HTMX's SSE extension can detect auth failure without a redirect loop
7. Expired sessions MUST be cleaned up periodically to prevent unbounded table growth
8. The login page MUST NOT use `layout.html` (it includes SSE connection and nav bar) -- use a standalone minimal template that loads `theme.css` and includes the `localStorage` theme-restore script

**Prerequisites:**
1. `github.com/alexedwards/scs/v2` added as dependency

## Unknowns & Assumptions

**Unknowns:**
- Whether Go 1.26 includes `http.CrossOriginProtection` for CSRF -- if not, POST forms are still protected by `SameSite=Lax` cookies which prevent cross-origin form submissions in modern browsers

**Assumptions:**
- Single-user for now; the session stores a boolean `authenticated` flag, not a user ID (trivial to change when multi-user lands)
- The password hash is generated externally (e.g. `htpasswd -nbBC 10 "" 'password' | cut -d: -f2`) and set in the env var
- Session lifetime defaults to 30 days, configurable via `SESSION_LIFETIME` env var

## Success Criteria

1. Accessing any page while unauthenticated redirects to `/login?next=/original/path`
2. Entering the correct password creates a session and redirects to the `next` URL (or `/` if none)
3. Entering a wrong password shows an error message on the login page
4. After 5 failed login attempts from the same IP within a minute, the login page shows "Too many attempts. Try again in 1 minute."
5. The logout button in the nav destroys the session and redirects to `/login`
6. Static assets load on the login page (CSS, JS) without authentication
7. API routes work with bearer token, unaffected by session auth
8. With `DASHBOARD_PASSWORD_HASH` unset, the dashboard works exactly as before -- no login page, no session middleware
9. SSE live updates work for authenticated sessions; unauthenticated SSE requests get 401
10. `/upload` and `/uploads/*` are protected by auth
11. Expired sessions are cleaned up automatically
12. Failed login attempts are logged at WARN level
13. Linting passes without warnings or errors
14. All tests pass (new and existing)
15. Build succeeds (`CGO_ENABLED=0 go build`)

---

## Development Plan

### Phase 1: Dependencies, Config, and Session Infrastructure

These tasks can be done in parallel since they touch independent files:

**Config (internal/config):**
- [ ] Add auth-related fields to `Config` struct: `PasswordHash string`, `SessionLifetime time.Duration`, `InsecureCookies bool`
- [ ] Load from env vars: `DASHBOARD_PASSWORD_HASH`, `SESSION_LIFETIME` (default `720h`), `DASHBOARD_INSECURE_COOKIES` (default `false`)
- [ ] `AuthEnabled() bool` helper method on Config (returns `PasswordHash != ""`)

**Database migration (internal/db):**
- [ ] Add two migration entries to the `migrations` slice in `internal/db/migrations.go`:
  - Entry 4: `CREATE TABLE IF NOT EXISTS sessions (token TEXT PRIMARY KEY, data BLOB NOT NULL, expiry REAL NOT NULL)`
  - Entry 5: `CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expiry)`

**Session store (internal/auth):**
- [ ] Add `github.com/alexedwards/scs/v2` dependency (`go get`)
- [ ] Create `internal/auth/store.go` -- custom `scs.Store` adapter wrapping `*sql.DB`:
  - `Find(token)` -- returns data if token exists and is not expired
  - `Commit(token, data, expiry)` -- upsert session row
  - `Delete(token)` -- remove session row
  - `CleanupExpired()` -- delete rows where expiry < now (called periodically)

**Tests:**
- [ ] Write tests for the session store in `test/auth_store_test.go`:
  - Round-trip: commit a session, find it, verify data matches
  - Expiry: commit a session with past expiry, verify `Find` returns `(nil, false, nil)`
  - Delete: commit, delete, verify `Find` returns not found
  - CleanupExpired: commit two sessions (one expired, one valid), run cleanup, verify only the valid one remains
  - Upsert: commit same token twice with different data, verify latest data returned
- [ ] Verify build succeeds: `CGO_ENABLED=0 go build ./...`
- [ ] Run `go vet ./...` and fix any issues
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 2: Auth Middleware, Handlers, and Rate Limiter

These tasks can be done in parallel since they are independent modules within `internal/auth`:

**Rate limiter (internal/auth/ratelimit.go):**
- [ ] Create in-memory rate limiter keyed by IP address (5 attempts per minute window)
- [ ] Background cleanup goroutine that removes stale entries every 5 minutes
- [ ] Methods: `Allow(ip string) bool`, `RetryAfter(ip string) time.Duration`

**Auth middleware (internal/auth/middleware.go):**
- [ ] `RequireAuth` middleware: checks `scs` session for `authenticated` key; if missing, redirects to `/login?next=<original-path>` (only accept relative paths starting with `/` to prevent open redirects)
- [ ] `RequireAuthAPI` middleware variant: returns 401 JSON instead of redirect (for SSE and any future non-browser endpoints)

**Auth handlers (internal/auth/handler.go):**
- [ ] `Handler` struct holding: session manager, password hash, rate limiter, login template
- [ ] `LoginPage(w, r)` -- render login template with optional error message and `next` param (GET)
- [ ] `LoginSubmit(w, r)` -- check rate limiter first (render "Too many attempts. Try again in N minute(s)." on 429); validate password via `bcrypt.CompareHashAndPassword`; on success call `sessionManager.RenewToken(r.Context())` then set `authenticated=true` in session, log at INFO, redirect to `next` param (validated, default `/`); on failure log at WARN with IP, re-render login with "Incorrect password" error
- [ ] `Logout(w, r)` -- destroy session via `sessionManager.Destroy(r.Context())`, redirect to `/login` (POST)

**Tests:**
- [ ] Write tests in `test/auth_test.go`:
  - **Rate limiter:** 5 calls to `Allow` succeed, 6th fails; after 1 minute window passes, calls succeed again; `RetryAfter` returns remaining duration; stale cleanup removes old entries
  - **Middleware RequireAuth:** unauthenticated request redirects to `/login?next=/original`; authenticated request passes through; `next` param rejects absolute URLs (open redirect prevention)
  - **Middleware RequireAuthAPI:** unauthenticated request returns 401 JSON; authenticated request passes through
  - **LoginSubmit:** correct password sets session and redirects to `/`; correct password with `?next=/ideas` redirects to `/ideas`; wrong password re-renders login with error; rate-limited request shows "Too many attempts" message
  - **Logout:** session destroyed, redirects to `/login`
- [ ] Run `go vet ./...` and all tests pass
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 3: Templates, Routing, and Integration

**Login template (web/templates):**
- [ ] Create `web/templates/login.html` -- standalone HTML page (NOT using `layout.html`):
  - Full `<html>` document with `<head>` loading `/static/theme.css`
  - Include the `localStorage` theme-restore script from `layout.html`'s `<head>` (the `(function(){ var t = localStorage... })()` block)
  - Centred card layout with: app brand (`dash>_`), password input (`autofocus`, using `.form-input` class), submit button (`.form-btn`), error message area (`.login-error` styled with `color: var(--red)`)
  - Hidden input or query param for `next` URL to preserve through form submission
  - Disable submit button on click to prevent double-submission during bcrypt verification (small inline JS)
  - Mobile-friendly: input uses existing `.form-input` class which has `font-size: 16px` override on mobile (prevents iOS zoom)

**Layout update (web/templates/layout.html):**
- [ ] Add logout button after the theme toggle in the nav bar -- styled like `.theme-toggle` (small bordered button), not inside `.nav-links` to avoid crowding mobile nav scroll. Use a `<form method="POST" action="/logout">` with a submit button

**Routing (cmd/dashboard/main.go):**
- [ ] Parse `login.html` as a standalone template (not cloned from layout -- it's self-contained)
- [ ] Initialise `scs.SessionManager` with:
  - Custom SQLite store adapter
  - `Lifetime` from config
  - `Cookie.HttpOnly = true`, `Cookie.SameSite = http.SameSiteLaxMode`
  - `Cookie.Secure = !cfg.InsecureCookies`
  - `Cookie.Name = "session"`
- [ ] Start session cleanup background goroutine (call store's `CleanupExpired` every hour)
- [ ] Structure routes with auth (when `cfg.AuthEnabled()`):
  - **Public group** (no auth): `GET /login`, `POST /login`, `/static/*`
  - **Protected group** (wrapped with `sessionManager.LoadAndSave` + `RequireAuth`): all page routes (`/`, `/personal`, `/family`, `/goals`, `/ideas/*`, `/exploration/*`), all POST form routes, `/upload`, `/uploads/*`, `POST /logout`
  - **Protected API-style** (wrapped with `sessionManager.LoadAndSave` + `RequireAuthAPI`): `GET /events` (SSE) -- returns 401 not redirect when unauthenticated
  - **Unchanged**: `/api/v1/*` routes keep existing bearer token auth only
- [ ] When `!cfg.AuthEnabled()`, register all routes without any auth middleware (current behaviour)
- [ ] Add `DASHBOARD_PASSWORD_HASH`, `SESSION_LIFETIME`, and `DASHBOARD_INSECURE_COOKIES` to `docker-compose.yml` environment section

**SSE auth error handling (web/static or inline in layout.html):**
- [ ] Add a small JS handler for SSE connection errors: if the SSE connection to `/events` fails with a 401, redirect the browser to `/login`. This handles the case where a session expires while a page is open

**Tests:**
- [ ] Write integration tests in `test/auth_integration_test.go` that start the full app (or a test server with the real router):
  - **Auth enabled flow:** unauthenticated GET `/` redirects to `/login?next=/`; GET `/login` returns 200 with login form; POST `/login` with correct password redirects to `/`; subsequent GET `/` returns 200; POST `/logout` redirects to `/login`; GET `/` after logout redirects to `/login`
  - **Redirect preservation:** unauthenticated GET `/ideas` redirects to `/login?next=/ideas`; login redirects to `/ideas`
  - **Protected routes:** unauthenticated requests to `/upload`, `/uploads/test.png`, `/events` all fail (redirect or 401)
  - **SSE auth:** unauthenticated GET `/events` returns 401 (not redirect)
  - **API unaffected:** `/api/v1/ideas` with bearer token works regardless of session auth state
  - **Auth disabled flow:** with empty `PasswordHash`, GET `/` returns 200 directly (no redirect, no session middleware)
- [ ] Run full test suite (existing + new tests), verify all pass
- [ ] Run `go vet ./...` and fix any issues
- [ ] Build: `CGO_ENABLED=0 go build -ldflags="-s -w" ./cmd/dashboard`
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 4: Final Verification

- [ ] Run full test suite and verify all tests pass
- [ ] Run linter (`go vet ./...`) and fix any issues
- [ ] Build application: `CGO_ENABLED=0 go build -ldflags="-s -w" ./cmd/dashboard`
- [ ] Manual smoke test with auth enabled:
  - Start app with `DASHBOARD_PASSWORD_HASH` set
  - Verify login page loads with correct theme
  - Verify wrong password shows error
  - Verify correct password logs in and redirects to home
  - Verify all pages accessible after login
  - Verify SSE live updates work (edit a markdown file, see update)
  - Verify logout works
  - Verify deep link preservation (`/ideas/some-slug` -> login -> redirected back)
  - Trigger rate limiting (5 wrong passwords), verify "Too many attempts" message
- [ ] Manual smoke test with auth disabled:
  - Start app without `DASHBOARD_PASSWORD_HASH`
  - Verify dashboard works exactly as before (no login page, no session overhead)
- [ ] Perform critical self-review of all changes across all files
- [ ] Verify all 15 success criteria are met
- [ ] STOP and wait for human review

---

## Notes

- **Generating a password hash:** `go run golang.org/x/crypto/bcrypt/...` or `htpasswd -nbBC 10 "" 'mypassword' | cut -d: -f2`
- **Multi-user migration path:** Swap the `authenticated` boolean session key for a `user_id`, add a users table, check credentials against it instead of the env var. Session infrastructure, middleware, and cookie config all carry forward unchanged.
- **OIDC migration path:** An OIDC callback handler writes the same `user_id` session key after validating the ID token. `RequireAuth` middleware does not change.
- **Passkeys migration path:** Can be added as an additional login method alongside password. Same session mechanism.

---

## Working Notes (for executing agent)

