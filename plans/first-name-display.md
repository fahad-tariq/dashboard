# Display First Name Instead of Username in Top Nav

## Overview

Add a `first_name` column to the users table and display it in the nav, homepage greeting, and page subtitles instead of the email local part. Redefine the existing `UserName(ctx)` function to check first name first and fall back to email local part, avoiding changes to 13+ handler data maps. Add self-service name editing so users can set their own display name.

## Current State

**Data flow:** DB (users table) → User struct → session (user_id, user_email, is_admin) → context middleware → handler data maps ("UserName": auth.UserName()) → templates ({{.UserName}}).

`auth.UserName()` extracts the local part of email (before @) as the display name. There is no first name field anywhere. The nav, homepage greeting, and page subtitles all use `auth.UserName()`.

**Technical Context:**
- Go + chi/v5, html/template, SQLite3, SCS session manager
- Migration system: sequential SQL strings in `internal/db/migrations.go`
- Auth: `internal/auth/` (users.go, handler.go, middleware.go)
- Admin UI: `internal/admin/handler.go`, templates `web/templates/admin-*.html`
- Self-service: `accountPasswordForm`/`accountPasswordSubmit` in `cmd/dashboard/main.go`, renders `admin-password.html` with `SelfService` flag
- CLI: `runUserAdd` subcommand in `cmd/dashboard/main.go`
- Legacy: auto-create from `DASHBOARD_PASSWORD_HASH` env var in `main.go`
- All handler data maps pass `"UserName": auth.UserName(r.Context())` to templates
- Subtitles built from `auth.UserName()` in tracker, ideas, and exploration handlers

## Requirements

1. Users table MUST have a `first_name` column (TEXT, NOT NULL, DEFAULT '')
2. User struct MUST include `FirstName` field
3. All user queries (FindByEmail, FindByID, AllUsers) MUST select and scan first_name
4. `CreateUser`/`CreateUserWithHash` MUST accept and insert first_name
5. `UserName(ctx)` MUST be redefined: return first name from context if non-empty, else email local part — this avoids changing any handler data maps or template references
6. First name MUST be stored in the session and extracted by both `RequireAuth` and `RequireAuthAPI` middleware
7. Admin create/edit user forms MUST include an optional first name field
8. Users MUST be able to edit their own first name via a self-service account page
9. `.nav-user` MUST have overflow protection (max-width, ellipsis) for long names
10. ALL callers of `CreateUser`/`CreateUserWithHash` MUST be updated (admin handler, CLI `useradd`, legacy auto-create)
11. Page subtitles ("X's list", "X's goals", "X's ideas", "X's explorations") MUST use the display name, not the raw email local part — these already use `auth.UserName()` so the redefinition handles this automatically
12. Existing users with no first name MUST see their email local part (current behaviour preserved by the fallback)
13. Build MUST succeed; all tests MUST pass

## Success Criteria

1. `go build ./cmd/dashboard` succeeds
2. `go test ./...` passes
3. New user created with first name → first name shown in nav, subtitles, and greeting
4. Existing user with empty first name → email local part shown everywhere (no change)
5. Admin user list shows first name as primary label when set, email as secondary
6. User can edit their own first name via self-service account page
7. Nav truncates long names with ellipsis rather than overflowing
8. CLI `useradd --first-name "Fahad"` stores the first name

---

## Development Plan

### Phase 1: Database & Model

- [ ] Add migration to `internal/db/migrations.go`: `ALTER TABLE users ADD COLUMN first_name TEXT NOT NULL DEFAULT ''`
- [ ] Add `FirstName string` field to User struct in `internal/auth/users.go`
- [ ] Update `FindByEmail()` to SELECT and scan `first_name`
- [ ] Update `FindByID()` to SELECT and scan `first_name`
- [ ] Update `AllUsers()` to SELECT and scan `first_name`
- [ ] Update `CreateUserWithHash()` to accept a `firstName` parameter and INSERT it
- [ ] Update `CreateUser()` to accept and pass through `firstName`
- [ ] Add `UpdateUserFirstName(db, id, firstName)` function
- [ ] Update ALL callers of `CreateUser`/`CreateUserWithHash` to pass firstName:
  - `cmd/dashboard/main.go` legacy auto-create (~line 106): pass `""`
  - `cmd/dashboard/main.go` `runUserAdd` (~line 446): add `--first-name` flag (optional, default `""`), pass it through
  - `internal/admin/handler.go` CreateUser (~line 147): pass `""` for now (Phase 4 adds form parsing)
- [ ] Update all test helpers and direct test calls to pass `""` as firstName (~20 call sites across `test/auth_test.go`, `test/admin_test.go`, `test/admin_handler_test.go`)
- [ ] Verify: `go build ./cmd/dashboard` succeeds
- [ ] Verify: `go test ./...` passes
- [ ] STOP and wait for human review

### Phase 2: Session, Context & UserName Redefinition

- [ ] In `internal/auth/middleware.go`: add `ctxFirstName` context key and `FirstName(ctx)` extraction function
- [ ] Redefine `UserName(ctx)` to: return `FirstName(ctx)` if non-empty, else extract email local part as before. Add a new `EmailLocalPart(ctx)` function for the original behaviour if needed internally.
- [ ] In `RequireAuth()`: extract `first_name` from session and inject into context as `ctxFirstName`
- [ ] In `RequireAuthAPI()`: same extraction
- [ ] In `internal/auth/handler.go` LoginSubmit: store `first_name` in session alongside user_id, user_email, is_admin
- [ ] Verify: `go build ./cmd/dashboard` succeeds
- [ ] Verify: `go test ./...` passes
- [ ] STOP and wait for human review

### Phase 3: Admin UI for First Name

- [ ] In `web/templates/admin-user-form.html`: add first name input field to both create and edit forms, before the email field. Field details: `name="first_name"`, `label="First Name"`, `placeholder="Optional"`, NOT required, keep autofocus on email. On edit form, pre-populate from `{{.EditUser.FirstName}}`
- [ ] In `web/templates/admin-users.html`: when first name is set, show it as the primary label (bold, `--fg` colour) and move email to the meta line. When empty, keep email as primary. Delete confirmation MUST still use email (unique identifier)
- [ ] In `internal/admin/handler.go` CreateUser: parse `first_name` from form, pass to `auth.CreateUser()`
- [ ] In `internal/admin/handler.go` UpdateUser: parse `first_name` from form, call `auth.UpdateUserFirstName()` if changed
- [ ] In `internal/admin/handler.go` renderUserForm: add `FormFirstName` to data map for re-populating on validation errors. Ensure all `renderUserForm` call sites in CreateUser (lines ~136, 140, 150) pass the parsed firstName
- [ ] Verify: `go build ./cmd/dashboard` succeeds
- [ ] STOP and wait for human review

### Phase 4: Self-Service Account Page

- [ ] Expand the self-service account page from password-only to account settings (name + password). Either:
  - Rename route from `/account/password` to `/account` and update the existing template, OR
  - Add a new `/account` route with a combined form, keeping `/account/password` as a redirect
- [ ] Add first name field to the self-service form (pre-populated from current user)
- [ ] Add handler logic to save first name via `auth.UpdateUserFirstName()`
- [ ] After saving first name, update the session's `first_name` value so the nav reflects the change immediately without requiring re-login
- [ ] Update the nav link: non-admin users currently link to `/account/password` — update to the new account route
- [ ] In `web/templates/layout.html` line 29: update the non-admin user link href
- [ ] Verify: `go build ./cmd/dashboard` succeeds
- [ ] STOP and wait for human review

### Phase 5: CSS & Polish

- [ ] Add overflow protection to `.nav-user` in `web/static/theme.css`: `max-width: 10ch`, `overflow: hidden`, `text-overflow: ellipsis`, `white-space: nowrap`
- [ ] Verify the nav doesn't break with long names on desktop
- [ ] Verify: `go build ./cmd/dashboard` succeeds
- [ ] Run `go test ./...` — all tests pass
- [ ] Self-review all changes across all phases
- [ ] STOP and wait for human review

---

## Notes

- **Key simplification**: By redefining `UserName(ctx)` to check first name first, we avoid touching 13+ handler data maps and all template references. The existing `"UserName"` key and `{{.UserName}}` in templates automatically carry the display name.
- Subtitles ("X's list", "X's goals", etc.) also use `auth.UserName()`, so they get the first name for free.
- Existing sessions won't have `first_name` — `UserName()` falls back to email local part via `FirstName(ctx)` returning `""`.
- The `admin-password.html` template is shared between admin password reset and self-service. Phase 4 will need to handle this carefully — either expand the existing template or create a new account template.
- `runUserAdd` usage line should update to: `usage: dashboard useradd --email <email> --password <password> [--first-name <name>]`

---

## Working Notes (for executing agent)

