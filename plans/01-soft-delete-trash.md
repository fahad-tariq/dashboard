# Soft Delete / Trash

## Overview

Items (tracker tasks, goals, and ideas) MUST move to a "trash" state instead of being permanently deleted. A "Recently Deleted" section appears on each list page with restore and permanent-delete actions. Items auto-purge after 7 days.

## Current State

- Delete is permanent and immediate -- no undo path exists
- Confirmation modals are the only guard
- Both tracker and ideas use a `mutate(slug, fn)` pattern for read-modify-write
- The "Done" section in tracker and "Converted" section in ideas provide a UI pattern for collapsible secondary item lists

## Design Decision: Inline `[deleted: YYYY-MM-DD]` metadata tag

A separate trash file would require a new parser, file watcher, and skeleton file creation. Instead, adding `[deleted: YYYY-MM-DD]` to existing items keeps everything within the current parse/write/mutate flow. `List()` filters them out; `ListDeleted()` returns only trashed items. Auto-purge runs as an hourly background goroutine (same pattern as session cleanup).

## Success Criteria

1. Deleting a tracker item or idea sets `[deleted: YYYY-MM-DD]` in the markdown file
2. "Recently Deleted" collapsible section on tracker, goals, and ideas pages
3. Restore button clears the deleted tag; "permanently delete" removes from file
4. Trashed items excluded from search, homepage, and summary counts
5. Auto-purge removes items trashed more than 7 days ago
6. All existing tests pass; new tests cover soft delete, restore, purge, list filtering

## Known Limitations / Edge Cases

- Converted idea linkage: trashing a converted idea does NOT affect the linked task (and vice versa). The `[converted-to:]`/`[from-idea:]` linkage survives soft-delete and restore. Permanent delete of either side leaves a dangling reference -- acceptable for v1.
- Auto-purge of items with bidirectional links will leave dangling references on the other side.
- No CSRF protection exists on any POST endpoint (pre-existing). The new `purge` endpoint is irreversible -- consider adding CSRF middleware in a future iteration.

---

## Development Plan

### Phase 1: Data Model and Parser Changes

- [ ] Add `DeletedAt string` field to `tracker.Item` in `internal/tracker/tracker.go`
- [ ] Add `deletedRe` regex and parse/write support for `[deleted: YYYY-MM-DD]`
- [ ] Add `DeletedAt string` field to `ideas.Idea` in `internal/ideas/parser.go`
- [ ] Add `deletedRe` regex and parse/write support for `[deleted: YYYY-MM-DD]`
- [ ] Add parser round-trip tests for both (including blank-line body preservation for ideas)
- [ ] Verify all existing parser tests still pass
- [ ] STOP and wait for human review

### Phase 2: Service Layer -- Soft Delete, Restore, Purge

- [ ] Modify `tracker.Service.Delete()` to set `DeletedAt` to today's date instead of removing the item
- [ ] Add `Restore(slug)`, `PermanentDelete(slug)`, `PurgeExpired(days int)`, `ListDeleted()` methods
- [ ] Update `MoveToList` to call `PermanentDelete` (not `Delete`) on the source service, so moved items do not appear in "Recently Deleted" on the source list
- [ ] Modify `List()` to exclude items where `DeletedAt != ""`
- [ ] Modify `Search()` to skip deleted items
- [ ] Cache strategy: `s.cache` holds ALL items including soft-deleted. `List()` and `Search()` filter at read time. `Get(slug)` returns items regardless of deleted status (needed by restore/purge handlers). `store.ReplaceAll()` receives only non-deleted items so `Summary()` counts remain correct.
- [ ] Apply same pattern to `ideas.Service`: `Delete()` becomes soft-delete, add `Restore`, `PermanentDelete`, `PurgeExpired`, `ListDeleted`
- [ ] Filter deleted items out before calling `store.ReplaceAll()` so they never enter the DB cache (avoids schema migration)
- [ ] Add service tests for all new methods
- [ ] Handle malformed `DeletedAt` dates gracefully in `PurgeExpired` (log warning and skip, do not panic)
- [ ] STOP and wait for human review

### Phase 3: Handler and Route Changes

- [ ] Add `Restore` and `PermanentDelete` handler methods to `tracker.Handler`
- [ ] Update `TrackerPage()` and `GoalsPage()` to pass `DeletedTasks`/`DeletedGoals` to template data
- [ ] Add flash messages: `"item-restored"`, `"item-purged"`. Add `item-purged` to `flashErrorKeys` so permanent delete uses error styling
- [ ] Add `RestoreIdea` and `PermanentDeleteIdea` to `ideas.Handler`
- [ ] Add flash messages: `"idea-restored"`, `"idea-purged"`. Add `idea-purged` to `flashErrorKeys` so permanent delete uses error styling
- [ ] Register routes: `POST {prefix}/{slug}/restore`, `POST {prefix}/{slug}/purge`
- [ ] Add handler tests
- [ ] Add test confirming `GET /api/v1/ideas` excludes trashed ideas
- [ ] STOP and wait for human review

### Phase 4: Template and UI Changes

- [ ] Add "Recently Deleted" collapsible section to `tracker.html`, `goals.html`, `ideas.html` (same `<details>` pattern as Done section)
- [ ] Note: `tracker.html` is shared between personal (`/todos`) and family (`/family`) views. The "Recently Deleted" section must use `.ListName` context to scope routes correctly (same pattern as empty state copy). `TrackerPage()` and `FamilyPage()` already resolve the correct service, so `DeletedItems` template data will be scoped per-list automatically.
- [ ] Each trashed item shows title, deletion date, restore button, permanent-delete button (with confirm modal)
- [ ] Update `idea.html` detail page: if deleted, show trash banner with restore/purge instead of normal actions
- [ ] Change delete button labels to "move to trash" and update confirmation text
- [ ] STOP and wait for human review

### Phase 5: Auto-Purge and Integration

- [ ] Extract `shutdownCtx` creation to before the auth/single-user branch so the purge goroutine can use it in both modes
- [ ] Add background goroutine in `main.go` that runs hourly, calling `PurgeExpired(7)` on all services. In auth-enabled mode, iterate all users via the database (the service registry is lazily populated and has no iteration method). In single-user mode, purge the singleton services directly
- [ ] Log purge errors via `slog.Error`; a failed purge must not crash the goroutine
- [ ] Verify homepage correctly excludes deleted items (via `List()` filtering)
- [ ] Verify search excludes deleted items
- [ ] Add integration test for purge timing
- [ ] STOP and wait for human review

### Phase 6: Final Review

- [ ] Run full test suite, linter, build
- [ ] Verify all success criteria met
- [ ] Manual walkthrough of delete/restore/purge for tasks, goals, and ideas
- [ ] Verify converted ideas with bidirectional linkage (`[converted-to:]`/`[from-idea:]`) behave correctly when trashed and restored
- [ ] Check auto-purge goroutine shuts down cleanly on context cancellation
- [ ] Confirm flash messages render with correct styling (success vs error keys in `flashErrorKeys`)
- [ ] STOP and wait for human review

## Critical Files

- `internal/tracker/tracker.go` -- Parser/writer: `DeletedAt` field and `[deleted:]` tag
- `internal/tracker/service.go` -- `Delete()` becomes soft-delete, new `Restore`/`PermanentDelete`/`PurgeExpired`/`ListDeleted`
- `internal/ideas/parser.go` -- Same parser changes as tracker
- `internal/ideas/service.go` -- Same service changes as tracker
- `cmd/dashboard/main.go` -- Route registration, auto-purge goroutine
- `web/templates/tracker.html` -- Shared template for personal and family views
- `web/templates/goals.html` -- Goals list with "Recently Deleted" section
- `web/templates/ideas.html` -- Ideas list with "Recently Deleted" section
- `web/templates/idea.html` -- Idea detail page trash banner
- `test/tracker_service_test.go` -- Service tests for soft delete, restore, purge
- `test/ideas_service_test.go` -- Service tests for ideas soft delete, restore, purge
- `test/search_test.go` -- Verify deleted items excluded from search
