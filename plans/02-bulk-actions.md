# Bulk Actions

## Overview

Add a "select mode" to tracker (todos, family, goals) and ideas pages. Users select items via checkboxes and apply batch operations in a single action. Spans both tracker and ideas packages, touching services, handlers, templates, JavaScript, CSS, and routes.

## Design Decisions

Each service gets a `mutateBatch(slugs []string, fn func(*Item) error)` that acquires the lock once, parses the file once, applies `fn` to all matched slugs, writes once, and updates cache once. `BulkDelete` uses a separate filter implementation since it changes slice length.

Batch operations are atomic per file -- if any slug is not found, the entire batch fails with an error and no changes are written. This keeps the read-modify-write pattern simple and avoids partial application.

Slugs are sent as a comma-separated string in a single `slugs` form field, parsed with `ideas.ParseCSV` (already used throughout handlers for tags).

## Constraints

- ES5 JavaScript compatibility required (no const/let/arrow functions) per project conventions
- All CSS animations must respect `prefers-reduced-motion: reduce`
- Flash messages use static keys (e.g. `"bulk-completed"`, `"bulk-deleted"`) not parameterised counts -- the redirect pattern maps keys to display text and does not support dynamic messages
- Use a "select all" button in the bulk bar rather than Ctrl+A (conflicts with browser "select all text"). Escape exits select mode but must yield to modal/overlay close handlers (priority: confirm modal > search overlay > shortcut help > select mode)
- Bulk bar requires `role="toolbar"` and `aria-label="Bulk actions"`. First selection triggers `aria-live="polite"` announcement. Exiting select mode returns focus to the select toggle button
- Goals support bulk delete and reprioritise only (not bulk complete). Bulk triage on ideas needs a target status picker (park/drop)

## Success Criteria

1. Select mode toggle on all four list pages
2. "Select all" button in bulk bar selects all visible non-completed items; Escape exits select mode (respecting modal priority)
3. Sticky bulk bar shows count and actions
4. Bulk complete/delete/retag/reprioritise work on tracker pages
5. Bulk delete/triage work on ideas page
6. All operations happen in one file write per batch
7. New service and handler tests

---

## Development Plan

### Phase 1: Service Layer -- Batch Mutation Helpers

- [ ] Add `mutateBatch` to `tracker.Service` (lock once, parse once, mutate all, write once, fail entirely if any slug missing)
- [ ] Add `BulkComplete`, `BulkDelete`, `BulkUpdatePriority`, `BulkAddTag` to tracker
- [ ] Add `mutateBatch` to `ideas.Service`
- [ ] Add `BulkDelete`, `BulkTriage` to ideas
- [ ] Unit tests for all bulk methods (including error case: one invalid slug rolls back entire batch)
- [ ] STOP and wait for human review

### Phase 2: HTTP Handlers and Routes

- [ ] Add flash message keys for bulk actions to tracker (`bulk-completed`, `bulk-deleted`, `bulk-priority`, `bulk-tagged`) and ideas (`bulk-deleted`, `bulk-triaged`)
- [ ] Add handler methods: `BulkComplete`, `BulkDelete`, `BulkPriority`, `BulkAddTag` (tracker); `BulkDelete`, `BulkTriage` (ideas)
- [ ] Register POST routes in `cmd/dashboard/main.go`:
  - Tracker bulk routes in `mountTrackerRoutes()`: `{prefix}/bulk/complete`, `{prefix}/bulk/delete`, `{prefix}/bulk/priority`, `{prefix}/bulk/tag`
  - Ideas bulk routes in `mountAppRoutes()`: `/ideas/bulk/delete`, `/ideas/bulk/triage`
- [ ] API layer (bearer token) does NOT get bulk endpoints in this iteration
- [ ] Handler tests
- [ ] STOP and wait for human review

### Phase 3: Frontend -- Select Mode UI

- [ ] Add `data-slug` attribute to each `.tracker-item` div for reliable slug extraction (avoid parsing `id` prefixes)
- [ ] CSS: `.bulk-checkbox` (hidden by default, shown in `.select-mode`), `.bulk-bar` (sticky bottom, z-index: 150 -- between nav at 100 and modals at 200), responsive rules
- [ ] Add "select" toggle button to filter bars in `tracker.html`, `goals.html`, `ideas.html`. Goals filter bar is conditional (`{{if or .Categories .Priorities}}`); if absent, the select toggle needs a fallback location (e.g. above the item list)
- [ ] Add checkbox markup to each item
- [ ] Add bulk action bar markup (hidden form with `slugs` input populated by JS)
- [ ] STOP and wait for human review

### Phase 4: Frontend -- JavaScript Logic

- [ ] `toggleSelectMode()`, `updateBulkBar()`, `selectAllVisible()`, `deselectAll()`, `getSelectedSlugs()`, `submitBulkAction(formId)`
- [ ] Wire "select all" button in bulk bar
- [ ] For bulk delete, populate hidden `slugs` input before calling `confirmAction(bulkDeleteForm, msg)`. Message should show count: "Delete N items? This cannot be undone."
- [ ] Escape exits select mode (respecting modal priority -- confirm modal > search overlay > shortcut help > select mode)
- [ ] `isInputFocused()` is defined in `shortcuts.js` which loads after `tracker.js` -- either extract to a shared utility loaded first, or place bulk action JS in `shortcuts.js`
- [ ] Suppress SSE swaps while select mode is active (same pattern as existing `tracker-item-completing` guard in `htmx:beforeSwap`)
- [ ] Update shortcut help modal to document select mode shortcuts
- [ ] `htmx:afterSwap` resets select mode
- [ ] STOP and wait for human review

### Phase 5: Integration and Self-Review

- [ ] Verify ES5 compliance in all new JavaScript (no const/let/arrow functions)
- [ ] Verify locking order safety (single service per request, no multi-service locking)
- [ ] Verify `prefers-reduced-motion: reduce` on any new CSS animations
- [ ] Check flash messages render correctly for both success and error states
- [ ] Run full test suite, linter, build
- [ ] Verify all success criteria met
- [ ] STOP and wait for human review

## Critical Files

- `cmd/dashboard/main.go` -- Route registration in `mountTrackerRoutes()` and `mountAppRoutes()`
- `internal/tracker/service.go` -- `mutateBatch` and four bulk methods
- `internal/ideas/service.go` -- `mutateBatch` and two bulk methods
- `internal/tracker/handler.go` -- Four bulk handler methods
- `internal/ideas/handler.go` -- Two bulk handler methods
- `web/static/tracker.js` -- Select mode, checkbox, bulk bar JS
- `web/templates/tracker.html` -- Checkbox markup, select toggle, bulk action bar
