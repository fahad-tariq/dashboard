# Power User Features: Keyboard Shortcuts and Search

## Overview

Add keyboard shortcuts for common actions and full-text search across all item types (tasks, goals, ideas). Both features target power users who want faster navigation and the ability to find items by body content, which is currently stored but not queryable from the UI.

## Current State

**Problems Identified:**
- No keyboard shortcuts exist. Every interaction requires clicking through the UI.
- Body content is stored in markdown files and cached in memory, but there is no way to search across items from the UI.
- No modal/overlay pattern exists in the CSS or JS -- both features need one.

**Technical Context:**
- Go 1.26.1, chi router, htmx 2.x, vanilla JS (no bundler)
- Tracker items: in-memory cache in `tracker.Service` + SQLite `tracker_items` table (body NOT in DB)
- Ideas: in-memory cache in `ideas.Service`, no DB backing
- Static JS: `tracker.js`, `upload.js` -- both plain scripts
- `mountAppRoutes` registers all shared routes

## Requirements

**Functional Requirements:**
1. `Ctrl+K` / `Cmd+K` opens a search overlay
2. `?` opens a keyboard shortcut help modal (suppressed when input focused)
3. `Ctrl+Shift+N` / `Cmd+Shift+N` focuses the quick-add form on current page
4. Search queries titles and body content across todos, family tasks, goals, and ideas
5. Results link to the item on its respective page
6. Results indicate item type and include a matching text snippet
7. Search endpoint scoped to current user in auth-enabled mode
8. Shortcuts MUST NOT fire when typing in inputs (except `Escape` to close)

**Technical Constraints:**
1. Search MUST use server-side in-memory caches (not SQLite FTS) to include ideas and body content
2. Search endpoint MUST return HTML fragments for htmx consumption
3. Vanilla JS, no modules, no build step
4. CSS MUST use existing Catppuccin custom properties
5. No new Go dependencies

## Success Criteria

1. `Ctrl+K` / `Cmd+K` opens search overlay; `Escape` closes it
2. Typing returns matching items within 200ms perceived latency
3. Each result links to correct page and anchor
4. `?` opens help modal listing all shortcuts
5. `Ctrl+Shift+N` opens and focuses quick-add form
6. All shortcuts suppressed when text input focused
7. `go test ./...` passes with new search endpoint tests
8. Build succeeds

---

## Development Plan

### Phase 1: Keyboard Shortcut Framework (Reuse Existing Modal Infrastructure)

**Prerequisite: Plan 02 must be complete.** Plan 02 Phase 4 establishes the shared `.modal-overlay`/`.modal-content` CSS classes and the confirm modal HTML in `layout.html`. This plan reuses that infrastructure -- do NOT create a parallel modal system.

- [ ] Verify that `.modal-overlay` and `.modal-content` CSS classes from Plan 02 exist in `theme.css`. If Plan 02 hasn't run yet, STOP and flag the dependency.
- [ ] Create `web/static/shortcuts.js` with keyboard event listener on `document`:
  - Ignores keystrokes when `event.target` is `input`, `textarea`, or `select` (except `Escape`)
  - Maps `Ctrl+K` / `Cmd+K` to opening search overlay (placeholder for Phase 3). **Note:** `Ctrl+K` conflicts with Firefox's search bar focus on Linux. Consider also binding `/` (when not in an input) as an alternative trigger, which has fewer browser conflicts and is used by many dashboards (GitHub, YouTube).
  - Maps `?` to opening shortcut help modal
  - Maps `Ctrl+Shift+N` / `Cmd+Shift+N` to opening `<details class="quick-add">` and focusing title input
  - Maps `Escape` to closing any open modal
- [ ] Add shortcut help modal to `web/templates/layout.html`: hidden `div.modal-overlay#shortcut-help` with shortcut table
- [ ] Add `<script defer src="/static/shortcuts.js"></script>` to `layout.html`
- [ ] Verify: `?` opens help modal, `Escape` closes it, `Ctrl+Shift+N` focuses quick-add
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 2: Search Endpoint (Server-Side)

- [ ] Add `Search(query string) []SearchResult` method to `internal/tracker/service.go`: take `s.mu.RLock()`, case-insensitive substring match on `Title` and `Body` of cached items, **release the lock before returning**. Returns `Slug`, `Title`, `Type`, `ListName`, `Snippet` (first ~120 chars of matching line). **Locking safety: lock one service at a time. Never hold locks on both tracker and ideas simultaneously** -- this prevents deadlock if a write operation (e.g. ToTask) needs both locks.
- [ ] Add `Search(query string) []SearchResult` method to `internal/ideas/service.go`: same pattern (RLock, search, RUnlock before returning)
- [ ] Create `internal/search/handler.go` with a `Handler` struct accepting resolver functions. The `Search` handler:
  - Reads `q` query parameter
  - Returns empty for blank queries
  - Calls `Search(q)` on personal tracker, then family tracker, then ideas -- **sequentially**, never holding multiple service locks at once
  - Merges results, caps at 20
  - Renders `search-results.html` fragment template
- [ ] Create `web/templates/search-results.html`: standalone template (not layout-wrapped) rendering `<a>` elements with type badge, title, snippet. Empty state: "no results".
- [ ] **Template parsing:** There is no existing precedent for standalone fragment templates. All current templates are parsed by cloning from `layout.html`. For `search-results.html`, create a separate `template.New("search-results").ParseFS(web.TemplateFS, "templates/search-results.html")` call in `parseTemplates()` that does NOT clone from layout. Store the result in a dedicated field (e.g. `searchTmpl`) on the search handler, or add it to the templates map with a distinct key like `"fragment:search-results"`.
- [ ] Register `GET /search` in `mountAppRoutes`
- [ ] Wire search handler with service resolvers for both auth-enabled and single-user modes
- [ ] Add tests in `test/search_handler_test.go`: matching across all lists, empty query, cap at 20, body content matching
- [ ] Verify: `curl '/search?q=test'` returns HTML fragment
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 3: Search Overlay (Client-Side)

- [ ] Add search overlay markup to `layout.html`: hidden `div.modal-overlay#search-overlay` with input field using `hx-get="/search"`, `hx-trigger="keyup changed delay:200ms"`, `hx-target="#search-results"`, and results container
- [ ] Style in `theme.css`: prominent input, scrollable results (`max-height: 60vh`), type badges with colours (todo=`--blue`, goal=`--green`, idea=`--mauve`), snippet in `--fg-dim`
- [ ] Update `shortcuts.js` to wire `Ctrl+K` / `Cmd+K`: show overlay, focus input, clear previous state. `Escape` closes. Clicking backdrop closes.
- [ ] Ensure clicking a result navigates to item (standard `<a>`)
- [ ] Verify end-to-end: `Ctrl+K`, type, results appear, click, arrive at correct item
- [ ] Perform a critical self-review
- [ ] STOP and wait for human review

### Phase 4: Final Review

- [ ] Run `go test ./...` and verify all tests pass
- [ ] Run `go vet ./...`
- [ ] Build application
- [ ] Test shortcuts on both macOS (`Cmd`) and non-macOS (`Ctrl`)
- [ ] Verify search in both auth-enabled and single-user modes
- [ ] Verify modals respect theme toggling
- [ ] Verify no regressions in filter behaviour
- [ ] Perform critical self-review
- [ ] Verify all success criteria met

---

## Cross-Plan Dependencies

- **Plan 02 MUST be complete before this plan starts.** Plan 02 Phase 4 establishes the shared `.modal-overlay`/`.modal-content` CSS and the confirm modal HTML in `layout.html`. This plan adds `#search-overlay` and `#shortcut-help` using the same base classes. Do NOT create a parallel modal system.

## Notes

- `search-results.html` is the first standalone fragment template in the codebase. It needs a separate parsing path that does NOT clone from layout. Add explicit parsing code in `parseTemplates()`.
- `Ctrl+K` conflicts with browser address bar focus in some browsers. `preventDefault()` is standard practice (VS Code, GitHub, Notion all do this). Also bind `/` (when not in an input) as an alternative trigger for fewer conflicts.
- Body text is not in SQLite. If search performance becomes an issue, a future iteration could add FTS5.
- **Locking safety:** Search locks services sequentially (personal tracker → family tracker → ideas), never holding multiple locks simultaneously. This prevents deadlock with write operations like `ToTask` that touch both services.
- Tracker items link to `/{listName}#{slug}` (anchors), ideas to `/ideas/{slug}` (detail page).

## Critical Files

- `web/static/theme.css` - Modal/overlay CSS
- `web/templates/layout.html` - Search overlay, shortcut help modal, script tag
- `cmd/dashboard/main.go` - Route registration, template parsing, service wiring
- `internal/tracker/service.go` - Search method on in-memory cache
- `internal/ideas/service.go` - Search method on in-memory cache
