# Quick Wins: Backlog Items #3, #4, #5, #7, #8

## Overview

Five quick UI/UX improvements to the dashboard: removing redundant "view all" links, verifying mobile single-column layout, simplifying mobile list items, wrapping mobile nav to two rows, and renaming "Personal" to "Todos" across routes/templates/handlers.

## Current State

**Technical Context:**
- Go + chi/v5, html/template, HTMX, SQLite3
- Single CSS file: `web/static/theme.css` (1160 lines, Catppuccin theme)
- One mobile breakpoint at 768px
- Templates: `web/templates/layout.html`, `web/templates/homepage.html`, `web/templates/tracker.html`, `web/templates/goals.html`, `web/templates/ideas.html`
- Routes in `cmd/dashboard/main.go`, handler logic in `internal/tracker/handler.go`

## Parallelisation Strategy

**File conflict analysis:**
- Items #4, #5, #7 all touch the mobile media query in `theme.css` — these MUST be done by a single CSS agent
- Items #3 and #8 both touch `homepage.html` — #3 should be done first (removes "view all" lines), then #8 renames remaining references
- Item #8 touches many files across templates, routes, and handlers — it should run as a dedicated agent

**Execution plan: 2 parallel agents, then 1 sequential agent**

| Agent | Items | Files Owned |
|-------|-------|-------------|
| **Agent A: CSS** | #4, #5, #7 | `web/static/theme.css` |
| **Agent B: Homepage cleanup** | #3 | `web/templates/homepage.html` |
| **Agent C: Rename Personal→Todos** | #8 | `web/templates/layout.html`, `web/templates/homepage.html`, `web/templates/tracker.html`, `web/templates/goals.html`, `web/templates/ideas.html`, `cmd/dashboard/main.go`, `internal/tracker/handler.go`, `internal/config/config.go` |

Agents A and B run in parallel. Agent C runs after B completes (since both touch `homepage.html`).

## Requirements

**Functional:**
1. Homepage MUST NOT display "view all" links; "+N more" links MUST remain
2. Homepage grid MUST display as single column at ≤768px
3. Homepage card items MUST hide date metadata on mobile
4. Nav links MUST wrap to multiple rows on mobile instead of horizontal scrolling
5. All `/personal` routes MUST be accessible at `/todos`
6. Nav MUST show "todos" instead of "personal"
7. Page title MUST show "Todos" (not "Todos Tasks")
8. "Move to personal" in tracker MUST read "move to todos"
9. Goals template form actions MUST use `/todos/` prefix
10. Ideas "to personal" button MUST read "to todos"

11. A 301 redirect from `/personal` → `/todos` MUST exist for backwards compatibility (bookmarks, browser autocomplete)

**Technical Constraints:**
1. MUST NOT break existing functionality or HTMX live-reload
2. MUST NOT change Go variable names (personalHandler, personalSvc, etc.) — only user-facing strings and routes
3. MUST NOT change the config env var `PERSONAL_PATH` or the underlying file path
4. MUST NOT change watcher category strings (`"personal"` in `fileCategories` map values and callback keys in `main.go`, nor in `watcher.go`) — these are internal plumbing that must stay consistent between auth-enabled and auth-disabled code paths
5. Build MUST succeed with `go build ./cmd/dashboard`

## Success Criteria

1. `go build ./cmd/dashboard` succeeds
2. No "view all" text in `homepage.html`
3. "+N more" links still present in `homepage.html`
4. Mobile nav wraps instead of scrolling (CSS `flex-wrap: wrap` present, `overflow-x: auto` removed)
5. Mobile homepage cards hide `.meta-dim` dates
6. All routes use `/todos` instead of `/personal`
7. All templates reference `/todos` and display "Todos" not "Personal"
8. `otherListName()` returns "todos" when listName is "family"
9. Handler title logic produces "Todos" (not "Todos Tasks")
10. GET `/personal` returns 301 redirect to `/todos`
11. Watcher category strings remain `"personal"` (internal, not user-facing)

---

## Development Plan

### Phase 1: CSS Changes — Items #4, #5, #7 (Agent A)

**Item #4: Verify mobile single-column layout**
- [ ] Confirm `homepage-grid` already has `grid-template-columns: 1fr` inside the 768px media query — this is already done
- [ ] Mark #4 as complete (no changes needed)

**Item #7: Two-row mobile nav**
- [ ] In the `@media (max-width: 768px)` block, replace the `.nav-links` rule:
  - Remove: `overflow-x: auto`, `-webkit-overflow-scrolling: touch`, `scrollbar-width: none`
  - Add: `flex-wrap: wrap`, `justify-content: center`
- [ ] Remove the `.nav-links::-webkit-scrollbar { display: none; }` rule from the media query
- [ ] Remove `white-space: nowrap` from `.nav-links a` in the media query
- [ ] Verify the nav-links already use `display: flex` in the base styles (they do via `.nav-links` base rule)

**Item #5: Simplify mobile list items**
- [ ] In the `@media (max-width: 768px)` block, add:
  ```css
  .homepage-exploration .meta-dim {
      display: none;
  }
  ```
- [ ] This hides the date span next to exploration entries on mobile
- [ ] Verify build: `go build ./cmd/dashboard`
- [ ] Self-review all CSS changes
- [ ] STOP and wait for human review

### Phase 2: Remove "View All" Links — Item #3 (Agent B, parallel with Phase 1)

- [ ] In `web/templates/homepage.html`, remove all standalone "view all" links (5 total, one per card section). These are `<a>` elements with class `homepage-card-link` containing the text "view all"
- [ ] Keep ALL `homepage-card-link` elements that contain "+N more" text — do NOT remove those
- [ ] Keep the `.homepage-card-link` CSS class in theme.css (still used by "+N more" links)
- [ ] Verify build: `go build ./cmd/dashboard`
- [ ] Self-review changes
- [ ] STOP and wait for human review

### Phase 3: Rename Personal → Todos — Item #8 (Agent C, after Phase 2)

**Templates — all user-facing references to "personal" become "todos":**
- [ ] `web/templates/layout.html`: Nav link href and text from "personal" to "todos"
- [ ] `web/templates/homepage.html`: Card title from "Personal Tasks" to "Todos", all `/personal` hrefs to `/todos`
- [ ] `web/templates/goals.html`: All 7 hardcoded `/personal/` form actions to `/todos/` (use bulk replacement to catch all)
- [ ] `web/templates/ideas.html`: Button text "to personal" to "to todos"

**Routes (`cmd/dashboard/main.go`):**
- [ ] Change route registrations: `r.Get("/personal", ...)` → `r.Get("/todos", ...)`
- [ ] Change `mountTrackerRoutes` map key: `"/personal"` → `"/todos"`
- [ ] Change goal route: `/personal/add-goal` → `/todos/add-goal`
- [ ] Add 301 redirect: `r.Get("/personal", http.RedirectHandler("/todos", http.StatusMovedPermanently).ServeHTTP)` — register AFTER the `/todos` route, or use a simple redirect handler
- [ ] DO NOT change watcher `fileCategories` values or callback keys — these MUST remain `"personal"` (internal plumbing, see Technical Constraint #4)

**Handler (`internal/tracker/handler.go`):**
- [ ] Change handler init in main.go: listName parameter from `"personal"` to `"todos"`
- [ ] Update `otherListName()`: condition from `"personal"` to `"todos"`, else branch returns `"todos"`
- [ ] Fix title generation so `listName == "todos"` produces page title "Todos" (not "Todos Tasks")
- [ ] Update subtitle condition from `"personal"` to `"todos"`

**DO NOT change:**
- Go variable names (personalHandler, personalSvc, personalStore, etc.)
- Config struct field `PersonalPath` or env var `PERSONAL_PATH`
- The `"Personal"` markdown heading passed to `tracker.NewService` (this is for the .md file format)
- Internal service registry field names (`.Personal`)
- Log messages referencing "personal" as an internal identifier
- Database store name `tracker.NewStore(database, "personal")` — DB category, not user-facing
- Watcher category strings in `fileCategories`, callbacks, `watcher.go`, or `userCallback` — internal plumbing

**Verification:**
- [ ] `go build ./cmd/dashboard` succeeds
- [ ] Grep for remaining `/personal` in templates — should find NONE
- [ ] Grep for remaining `"personal"` in route definitions — should find NONE (except internal/non-user-facing)
- [ ] Self-review all changes
- [ ] STOP and wait for human review

### Phase 4: Final Verification

- [ ] `go build ./cmd/dashboard` succeeds
- [ ] All success criteria verified
- [ ] No leftover debug statements or console.log

---

## Notes

- Item #4 appears to already be implemented based on the existing CSS. Verification only needed.
- The `"Personal"` heading in config.go (`{c.PersonalPath, "Personal"}`) is used for markdown file parsing and MUST NOT change — it's the heading in the actual `personal.md` data file.
- The `tracker.NewStore(database, "personal")` call uses "personal" as a database table/category identifier — changing this would require a data migration.

---

## Working Notes (for executing agent)

