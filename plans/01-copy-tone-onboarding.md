# Copy, Tone & Onboarding Plan

## Overview

Improve the dashboard's written communication across flash messages, empty states, button labels, and help text. Currently, successful mutations produce no user feedback, empty states are clinical, flash copy is robotic, button labels use inconsistent capitalisation, and new users get no contextual guidance. This plan addresses all six backlog items in three phases.

## Current State

**Problems Identified:**
- Successful mutations (add task, complete, edit, triage, etc.) redirect without any feedback. Only validation errors like `title-required` produce flash messages.
- Empty state copy is clinical and unhelpful: "No tasks yet. Add one above.", "No goals yet. Add one above.", "No ideas yet. Add one above or POST to /api/v1/ideas." (exposes API endpoint to end users).
- Flash message copy is robotic: "Title is required.", "Name updated.", "User deleted."
- Button/label capitalisation is inconsistent: `add task` (lowercase submit), `Add idea` (summary text title case), `add goal` (lowercase submit), `Add goal` (summary), `add` (ideas submit -- verb only, no noun), `Save Name` (title case), `Update Password` (title case).
- No distinction between "no items exist at all" and "items exist but are filtered to zero results".
- No onboarding guidance for new users -- concepts like todos vs goals vs ideas are unexplained, form field purposes are unclear, and idea statuses (untriaged/parked/dropped) have no legend.
- Homepage greeting is static with no time-of-day context.

**Technical Context:**
- Go 1.22+, chi router, server-rendered HTML templates with htmx
- Flash messages use `?msg=key` query params mapped to display text in per-package `flashMessages` maps
- Flash rendering in `layout.html` uses `FlashMsg` and `FlashError` template variables (line 31)
- `redirectBack` in tracker handler strips query params from Referer -- success flashes on mutation endpoints MUST use direct redirect paths (e.g. `/todos?msg=task-added`) rather than `redirectBack`, or `redirectBack` must be extended to support a msg param
- Ideas handler redirects directly to `/ideas` or `/ideas?msg=...` (no `redirectBack` helper)
- Account handler has its own flash map with `name-updated` and `password-updated` (these already work as success messages but use robotic copy)
- Template filtering is client-side JavaScript (localStorage-based) -- "filtered to zero" detection MUST happen in JS, not server-side
- Homepage uses `renderHomePage` with `auth.TemplateData(r)` which includes `UserName`

## Requirements

**Functional Requirements:**
1. Every successful mutation MUST show a flash message confirming the action
2. Empty states MUST use contextual copy explaining what each section is for
3. Ideas empty state MUST NOT mention the API endpoint
4. When filters produce zero results, the UI MUST show a distinct "no matches" message rather than the "no items" empty state
5. All flash messages MUST use warm but concise tone
6. All button and label text MUST use consistent sentence case (lowercase verbs, e.g. "add task")
7. New users MUST see contextual help text explaining todos vs goals vs ideas
8. Form fields MUST have visible placeholder help text where purpose is non-obvious
9. Ideas page MUST include a status legend explaining untriaged/parked/dropped
10. Homepage MUST display a time-of-day greeting ("Good morning", "Good afternoon", "Good evening")

**Technical Constraints:**
1. Flash messages MUST continue using the `?msg=key` query param pattern
2. Success flashes MUST NOT set `FlashError` to true (they render without the error class)
3. Time-of-day greeting MUST be server-rendered (using Go's `time.Now()`) -- not client-side JS
4. Template changes MUST NOT break existing handler tests
5. Help text and legends MUST be HTML only -- no new JavaScript required
6. All copy MUST use Australian English spelling

## Success Criteria

1. All 15 mutation endpoints redirect with a `?msg=` success flash on success
2. Every empty state uses distinct copy that describes the section's purpose
3. Ideas empty state contains no API endpoint reference
4. Filtered-to-zero state shows "No matches" message distinct from true empty state on tracker, goals, and ideas pages
5. All button/submit labels use consistent sentence case
6. Homepage shows time-appropriate greeting with user's name
7. Help text appears on todos, goals, and ideas pages for new users
8. Idea status legend is visible on the ideas page
9. `go test ./...` passes
10. `go vet ./...` passes
11. Application builds without errors

---

## Development Plan

### Phase 1: Success Flash Messages and Tone Upgrade

This phase adds success flash messages to all mutation endpoints and rewrites all existing flash copy (both new and old) to use warmer tone.

**Mutation endpoints requiring success flashes (15 total):**

Tracker handler (`internal/tracker/handler.go`):
1. `QuickAdd` -- redirect to `/todos` or `/family` after add
2. `AddGoal` -- redirect to `/goals` after add
3. `Complete` -- `redirectBack` after complete
4. `Uncomplete` -- `redirectBack` after uncomplete
5. `UpdateNotes` -- `redirectBack` after notes update
6. `UpdatePriority` -- `redirectBack` after priority change
7. `UpdateTags` -- `redirectBack` after tags change
8. `UpdateEdit` -- `redirectBack` after edit
9. `Delete` -- `redirectBack` after delete
10. `MoveToList` -- redirect after move

Ideas handler (`internal/ideas/handler.go`):
11. `QuickAdd` -- redirect to `/ideas` after add
12. `TriageAction` -- redirect to `/ideas` after triage
13. `Edit` -- redirect to `/ideas` after edit
14. `ToTask` -- redirect to `/ideas` after conversion
15. `DeleteIdea` -- redirect to `/ideas` after delete

- [ ] Extend `redirectBack` in `internal/tracker/handler.go` to accept an optional flash message key. Use a **variadic** signature: `redirectBack(w, r, anchor string, msg ...string)` -- when `msg` is provided and non-empty, append `?msg=<msg[0]>` to the destination path before the anchor fragment. This avoids touching existing callers that don't need flash messages (all 8 current call sites remain unchanged)
- [ ] Add success flash message keys to the tracker `flashMessages` map. Use warm tone:
  - `"task-added": "Task added."`
  - `"goal-added": "Goal added."`
  - `"task-completed": "Nice one -- task completed."`
  - `"task-uncompleted": "Task reopened."`
  - `"notes-updated": "Notes saved."`
  - `"priority-updated": "Priority updated."`
  - `"tags-updated": "Tags updated."`
  - `"item-updated": "Changes saved."`
  - `"item-deleted": "Item removed."`
  - `"item-moved": "Moved to the other list."`
  - Rewrite existing: `"title-required": "A title is required."` (keep error messages direct -- save warmth for success messages)
- [ ] Update only the tracker mutation handlers that need flash messages -- pass the message key as the optional 4th arg to `redirectBack`. Existing callers without flash messages do NOT need updating thanks to the variadic signature
- [ ] Update tracker `TrackerPage` and `GoalsPage` flash rendering to support both error and success flashes -- success keys MUST NOT set `FlashError`. Add logic: only set `FlashError = true` for keys in an explicit error set (currently just `title-required`), leave it false for success keys
- [ ] Add success flash message keys to the ideas `flashMessages` map:
  - `"idea-added": "Idea captured."`
  - `"idea-triaged": "Status updated."`
  - `"idea-edited": "Changes saved."`
  - `"idea-converted": "Idea converted to a task -- check your todos."`
  - `"idea-deleted": "Idea removed."`
  - Rewrite existing: `"title-required": "A title is required."` (direct, not cutesy)
- [ ] Update all 5 ideas mutation handlers to redirect with `?msg=` success keys
- [ ] Update ideas `IdeasPage` flash rendering to support both error and success flashes (same pattern as tracker)
- [ ] Rewrite account `flashMessages` map in `internal/account/handler.go` with warmer tone:
  - `"name-updated": "All good -- name saved."`
  - `"password-updated": "Password updated successfully."`
- [ ] Read `internal/admin/handler.go` and rewrite its `flashMessages` map with warmer tone, preserving the existing error vs success distinction pattern
- [ ] Update handler tests in `test/tracker_handler_test.go` and `test/ideas_handler_test.go` to verify success flash redirects (check `Location` header contains `?msg=` for mutation endpoints)
- [ ] Verify: `go test ./...` passes, `go vet ./...` passes
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 2: Empty States, Filtered-to-Zero, and Button/Label Consistency

- [ ] Rewrite empty state copy in `web/templates/tracker.html`. Replace `"No tasks yet. Add one above."` with contextual copy: `"No tasks yet. Todos are for things you want to get done -- open the form above to add your first."` **Note:** The tracker template is shared between `/todos` and `/family`. Use a conditional: `{{if eq .ListName "family"}}No family tasks yet. Use this list to coordinate shared household tasks.{{else}}No tasks yet. ...{{end}}`
- [ ] Rewrite empty state copy in `web/templates/goals.html`. Replace `"No goals yet. Add one above."` with: `"No goals yet. Goals track measurable progress over time -- things like fitness targets, reading goals, or savings milestones."`
- [ ] Rewrite empty state copy in `web/templates/ideas.html`. Replace `"No ideas yet. Add one above or POST to /api/v1/ideas."` with: `"No ideas yet. Ideas are for things you might want to explore someday -- projects, topics, half-formed thoughts worth capturing."`
- [ ] Rewrite homepage empty state in `web/templates/homepage.html`. Replace `"Nothing here yet, {{.UserName}}. Use the nav above to get started."` with: `"Welcome{{if .UserName}}, {{.UserName}}{{end}}. Your todos, goals, and ideas will appear here once you start adding them."`
- [ ] Rewrite admin users empty state in `web/templates/admin-users.html`. Replace `"No users yet."` with: `"No users yet. Create one to get started."`
- [ ] Add filtered-to-zero messaging to `web/templates/tracker.html`: Add a hidden paragraph `<p class="empty filter-empty" style="display:none">No tasks match the current filter.</p>` below the task list, alongside the existing empty state
- [ ] Add filtered-to-zero messaging to `web/templates/goals.html`: Same pattern
- [ ] Add filtered-to-zero messaging to `web/templates/ideas.html`: Same pattern
- [ ] Read `web/static/tracker.js` and update the `applyFilter` function to count visible items after filtering. When zero items are visible and items exist in the DOM, show the `.filter-empty` message. When items are visible or no items exist at all, hide it. When filter is cleared (all), hide the filter-empty message. **Cross-plan note:** Plan 02 also modifies `applyFilter` to add a filter-active badge. This plan (01) MUST run first. Keep changes minimal -- only add the show/hide logic for `.filter-empty`
- [ ] Standardise all button/label text to consistent sentence case. Changes needed:
  - `web/templates/ideas.html`: `add` button -- change to `add idea`
  - `web/templates/account.html`: `Save Name` -- change to `Save name`
  - `web/templates/account.html`: `Update Password` -- change to `Update password`
  - `web/templates/admin-users.html`: `+ New User` -- change to `+ New user`
- [ ] Verify: `go test ./...` passes, `go vet ./...` passes
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 3: Onboarding Help Text, Status Legend, and Time-of-Day Greeting

- [ ] Add brief help text below each quick-add `<details>` section explaining the concept. Use a `<p class="section-help">` element (subtle, small text):
  - `web/templates/tracker.html`: `"Todos are actionable items you want to complete. Use tags and priorities to organise them."`
  - `web/templates/goals.html`: `"Goals track measurable progress. Set a target and unit, then update your progress over time."`
  - `web/templates/ideas.html`: `"Ideas are things worth capturing but not ready to act on. Triage them as you go -- park what you might revisit, drop what you won't."`
- [ ] Improve form field placeholders where purpose is non-obvious:
  - `web/templates/goals.html` current field: change placeholder from `"Current"` to `"Starting value"`
  - `web/templates/goals.html` target field: change placeholder from `"Target (optional)"` to `"Target to reach (optional)"`
  - `web/templates/tracker.html` body field: change placeholder from `"Notes (optional)"` to `"Notes, links, or context (optional)"`
- [ ] Add an idea status legend to `web/templates/ideas.html`, between the summary stats and the quick-add form:
  - **Untriaged** -- New ideas waiting for a decision
  - **Parked** -- Worth keeping, not acting on now
  - **Dropped** -- Decided against, kept for reference
- [ ] Add CSS for `.section-help` and `.status-legend` in `web/static/theme.css`: small font size (`0.85rem`), muted colour (`var(--fg-dim)`), minimal top margin. The legend items should display inline with a separator
- [ ] Implement time-of-day greeting on the homepage. In `internal/home/handler.go`, add a `greeting(now time.Time) string` function that accepts a `time.Time` parameter (for testability -- do NOT call `time.Now()` directly inside the function). Call it as `greeting(time.Now())` from `renderHomePage`. Returns "Good morning" (5-11), "Good afternoon" (12-17), or "Good evening" (18-4). Pass as `data["Greeting"]`
- [ ] Update `web/templates/homepage.html` to display the greeting: `{{if .Greeting}}<h1 class="homepage-greeting">{{.Greeting}}{{if .UserName}}, {{.UserName}}{{end}}</h1>{{end}}`
- [ ] Update `test/home_handler_test.go` to verify the greeting appears in homepage output
- [ ] Verify: `go test ./...` passes, `go vet ./...` passes
- [ ] Perform a critical self-review of all changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 4: Final Review

- [ ] Run full test suite (`go test ./...`) and verify all tests pass
- [ ] Run `go vet ./...` and fix any issues
- [ ] Build application and verify no errors or warnings
- [ ] Perform critical self-review of all changes across all phases
- [ ] Verify all 11 success criteria are met

---

## Cross-Plan Dependencies

- **Plan 02 depends on this plan's Phase 2.** Both plans modify `applyFilter()` in `tracker.js`. This plan adds `.filter-empty` show/hide logic; Plan 02 adds `.filter-active-badge` insertion. This plan MUST run first.
- **Plan 05 also modifies `internal/home/handler.go`.** This plan adds `Greeting`; Plan 05 adds insight data. No conflict if this plan runs first, but both plans touch the same function.
- **`ideas.html` is touched by Plans 01, 02, 04, and 05.** This plan modifies: empty state copy, button label, help text, status legend, filtered-to-zero message. Other plans modify: idea-card template (04), triage animation handlers (02), age badges (05). Different sections of the file -- low conflict risk if plans run in recommended order.

## Notes

- The variadic `redirectBack` signature (`anchor string, msg ...string`) means existing callers don't change. Only handlers that need flash messages pass the extra argument.
- The account handler's flash rendering does not use `FlashError` -- it renders all flashes with the same style. Since account flashes are all success messages, this is fine.
- Client-side filtered-to-zero detection requires updating `applyFilter()` in `tracker.js` to count visible `.tracker-item` elements after filtering.
- The `greeting()` function accepts `time.Time` for testability. Tests can pass a fixed time to verify deterministic output.
- The tracker template is shared between `/todos` and `/family`. Empty state copy needs a conditional on `.ListName` to show family-specific text.

## Critical Files

- `internal/tracker/handler.go` - Core mutation handlers, flash message map, `redirectBack` helper
- `internal/ideas/handler.go` - Ideas mutation handlers and flash message map
- `web/templates/ideas.html` - Empty state, API endpoint removal, status legend, button label, help text
- `web/templates/homepage.html` - Time-of-day greeting, empty state rewrite
- `internal/home/handler.go` - Greeting logic
- `web/static/tracker.js` - Filtered-to-zero show/hide logic in `applyFilter()`
