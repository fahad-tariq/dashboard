# Status, Errors & Session UX

## Overview

Three UX improvements: (1) replace generic "Bad request" error responses with specific, actionable messages and add correlation IDs for 500 errors, (2) show idea triage status badges on idea cards in the list view, and (3) display a "Session expired" toast before redirecting to login when the SSE error handler detects a 401.

## Current State

**Problems Identified:**
- 39 call sites across 5 handler files use `http.Error(w, "Bad request", ...)` with no distinction between "item not found", "invalid input", or "form parse failure". The upload handler already returns specific messages -- this is the pattern to follow.
- The idea detail page (`idea.html`) renders a `badge-{{.Status}}` but the list page (`ideas.html`) has no such badge, so users must expand each card to see status.
- When a session expires, the SSE error handler in `layout.html` silently redirects to `/login?next=...` with no visual feedback.

**Technical Context:**
- Service layer returns plain `error` values with "not found" in the message
- No structured error types exist
- Flash messages use `?msg=key` query param approach
- Existing badge CSS: `badge-untriaged`, `badge-parked`, `badge-dropped` (defined in `theme.css`)
- SSE error script checks `/events` with HEAD, redirects on 401

## Requirements

**Functional Requirements:**
1. All `http.Error(w, "Bad request", 400)` responses MUST include a specific message
2. All 500 error responses MUST include a correlation ID in the response body and structured log
3. The idea card template MUST display a status badge
4. SSE session expiry MUST show a toast before redirecting
5. Login page MUST accept `?expired=1` and show "Your session has expired."

**Technical Constraints:**
1. Error messages MUST NOT expose internal details
2. Correlation IDs: 8-char hex from `crypto/rand`
3. Toast MUST use existing `.flash-msg` CSS class
4. Changes MUST NOT break existing handler tests
5. Badge rendering MUST use existing CSS classes

## Success Criteria

1. Every handler `http.Error` call returns a contextual message
2. 500-level responses include a correlation ID in both response and log
3. Idea cards on `/ideas` show a coloured status badge
4. SSE session expiry shows a toast before redirecting
5. Login page shows expired message when redirected from SSE timeout
6. `go test ./...` passes, `go build` succeeds

---

## Development Plan

### Phase 1: Contextual Error Messages & Correlation IDs

- [ ] Create `internal/httputil/errors.go` with a `ServerError` helper function:
  - `ServerError(w http.ResponseWriter, msg string, err error, slogArgs ...any)` -- generates 8-char hex correlation ID via `crypto/rand`, writes `"Internal error [ref: <id>]"` to response with 500, logs the full error with `slog.Error` including `"correlation_id"` field
  - **Do NOT create a `ClientError` wrapper** -- it adds no value over `http.Error(w, msg, status)` since the signatures are identical. Just use `http.Error` directly with better message strings. If a centralised client error pattern is needed later (e.g. structured JSON for API routes), it can be added then.
- [ ] Replace all generic 400 errors in `internal/tracker/handler.go` (~15 call sites):
  - `r.ParseForm()` failures: "Failed to parse form data"
  - Service errors containing "not found": "Item not found"
  - Other service mutation errors: "Failed to update item"
- [ ] Replace all generic 400 errors in `internal/ideas/handler.go` (~5 call sites):
  - `r.ParseForm()` failures: "Failed to parse form data"
  - Service errors containing "not found": "Idea not found"
  - Other: "Failed to update idea"
- [ ] Replace generic errors in `internal/admin/handler.go` and `internal/account/handler.go`
- [ ] Replace all `http.Error(w, "Internal server error", 500)` calls with `ServerError` helper:
  - `tracker/handler.go`: ~6 sites
  - `ideas/handler.go`: ~3 sites
  - `admin/handler.go`: ~1 site
  - `account/handler.go`: ~1 site
- [ ] Add unit test for `ServerError` helper verifying correlation ID in response and log
- [ ] Update existing handler tests that assert on "Bad request" response body text
- [ ] Run `go test ./...` and `go build`
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 2: Idea Status Badges on List Cards

- [ ] In the `idea-card` template in `web/templates/ideas.html`, add `<span class="badge badge-{{.Status}}">{{.Status}}</span>` inside `.tracker-item-meta`, after tags and before date. Mirrors the pattern on `idea.html` detail page.
- [ ] Verify triage button behaviour: existing `{{if ne .Status "..."}}` conditionals hide buttons for current status -- confirm this is sufficient
- [ ] Run `go test ./...`
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 3: Session Timeout Notification

- [ ] Modify SSE error handler in `web/templates/layout.html`:
  - On 401 detection, create a `div` with class `flash-msg`, text "Session expired. Redirecting to login...", prepend to `.container`
  - Use `setTimeout` with 2-second delay before redirect to `/login?next=...&expired=1`
  - Keep existing `navigating` guard
- [ ] Update `internal/auth/handler.go` `LoginPage` to read `r.URL.Query().Get("expired")` and set `"SessionExpired": true` when param equals `"1"`
- [ ] Update `web/templates/login.html`: `{{if .SessionExpired}}<div class="form-info">Your session has expired.</div>{{end}}`
- [ ] Add CSS for `.form-info` in `theme.css` -- same layout as `.form-error` but neutral colour
- [ ] Add test case for login page with `?expired=1`
- [ ] Run `go test ./...` and `go build`
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 4: Final Review

- [ ] Run full test suite and `go vet ./...`
- [ ] Build application
- [ ] Verify all success criteria met
- [ ] Review all modified files for debug statements or regressions

---

## Notes

- Using `strings.Contains(err.Error(), "not found")` for error classification is pragmatic but fragile. Consider defining `var ErrNotFound = errors.New("not found")` in the service packages and using `errors.Is` for classification -- this is a ~10-line change per service, not a major refactor, and prevents future misclassification if an error message coincidentally contains "not found". Add a `// TODO: replace with sentinel errors` comment at classification sites if deferring this.
- Hiding triage buttons (vs greying out) reduces visual noise. If disabled-but-visible buttons are preferred, the template change is straightforward.
- Correlation IDs use 8 hex chars (4 bytes of randomness) -- sufficient for a small-team dashboard.

## Critical Files

- `internal/tracker/handler.go` - Largest concentration of generic errors (15+ sites)
- `internal/ideas/handler.go` - Generic errors plus idea card rendering
- `web/templates/ideas.html` - Status badge addition
- `web/templates/layout.html` - SSE error handler toast
- `internal/httputil/` - New `errors.go` with helper functions
