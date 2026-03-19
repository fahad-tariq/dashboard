<ARCHITECTURE>
Go + chi + htmx server-rendered dashboard. Markdown files are the source of truth; SQLite caches tracker data for fast queries.

**Two flat-file patterns, separate parsers:**
- Tracker (`internal/tracker/`): tasks and goals in `personal.md`/`family.md`. Drops blank lines in bodies. DB-backed via `Store` for summary counts. In-memory cache serves reads; invalidated by mutations and file watcher.
- Ideas (`internal/ideas/`): ideas in `ideas.md`. Preserves blank lines in bodies (rich markdown with paragraphs). In-memory cache serves reads; no DB cache. Ideas have a `converted` status with `ConvertedTo` field linking to the resulting task slug.

Both follow read-modify-write with a `mutate(slug, fn)` helper: lock, parse file, find by slug, apply callback, write back, update cache, release lock. Title edits re-slugify the item; the old slug becomes invalid after mutation. `mutateBatch(slugs, fn)` applies to multiple items atomically -- if any slug is missing, the entire batch fails with no changes written.

**Soft delete:** Deleting an item sets `[deleted: YYYY-MM-DD]` inline metadata instead of removing it. `List()` and `Search()` filter deleted items at read time. `s.cache` holds ALL items (including soft-deleted); `store.ReplaceAll()` receives only active items so `Summary()` counts stay correct. `Get(slug)` returns items regardless of deleted status (needed by restore/purge handlers). Hourly auto-purge goroutine removes items trashed more than `trashRetentionDays` (7) ago.

**Image captions:** Stored inline as `filename|caption` in `[images:]` tags. `httputil.SplitImageCaption`/`JoinImageCaption`/`SanitiseCaption` are shared helpers (in `httputil` to avoid import cycles between tracker and ideas). Captions are sent via separate `caption-N` form fields, not through the comma-separated `images` hidden input. `httputil.ReconstructImages(r)` zips both server-side. `splitImageCaption` template function returns plain strings, never `template.HTML`.

**Service registry** (`internal/services/`): Caches per-user service instances. `sync.RWMutex` with RLock fast path for cache hits; filesystem I/O (`EnsureUserDirs`) runs outside the lock. `EnsureUserDirs` is deliberately separate from `ForUser` -- directory creation is an explicit side effect, not hidden in a getter.

**Package layout:** Handlers are split across `internal/tracker/`, `internal/ideas/`, `internal/admin/`, `internal/account/`, `internal/home/`, and `internal/search/`. Shared utilities: `internal/httputil/` (JSON response, `ServerError` with correlation IDs, image caption helpers, `ParseCSV`, `CutoffDate`), `internal/auth/` (middleware, context helpers, `TemplateData`), `internal/insights/` (pure computation for age badges, velocity, streaks, goal pace, tag aggregation, digest -- no state, no DB dependency). Routes are registered once via `mountAppRoutes`, conditionally wrapped with auth middleware.

**Digest** (`internal/home/digest.go`): `/digest` page with period-specific activity summaries. `insights.Digest()` is a pure function; the handler merges personal + family items before calling it. `weekStart()` is a shared helper used by both `WeeklyVelocity` and `periodBounds`.

**Search** (`internal/search/`): Queries in-memory caches across personal tracker, family tracker, and ideas. Locks services sequentially (never simultaneously) to prevent deadlock. Returns HTML fragments for htmx consumption. Standalone template parsed separately from layout.

**Auth evolution paths:** Session infrastructure is auth-method-agnostic. OIDC or passkeys can be added by writing a new callback handler that sets the same `user_id` session key. `RequireAuth` middleware does not change.

**Daily planner** (`internal/home/handler.go`): The homepage doubles as a daily planning hub. `[planned: YYYY-MM-DD]` inline metadata marks tasks for a specific day. The planner is a view over existing tasks, not a separate store. `ListPlanned(date)` returns today's planned items; `ListOverdue(beforeDate)` surfaces carried-over tasks. Plan handlers live in the home package (`SetPlanned`, `ClearPlanned`, `CompletePlanned`, `BulkSetPlanned`, `ClearCarriedOver`, `ReorderPlanned`). Auth-enabled mode uses `Handler` methods; single-user mode uses `SingleUserPlanHandlers` closures. The homepage template shows Today's Plan as the primary section with a task picker below. Tasks can also be planned from `/todos` via per-item and bulk actions.

**Plan drag-and-drop** (`web/static/planner.js`): HTML5 native DnD, no external libraries, ES5 compatible. Homepage: reorder within `.plan-today-tasks` by dragging `.plan-item` elements; `POST /plan/reorder` persists order via `[plan-order: N]` inline metadata. Calendar week view: drag `.calendar-task` between `.calendar-cell` elements to reschedule; `POST /plan/set` with new date. `sortPlanItems` sorts explicit `PlanOrder` first (ascending), then unordered by priority. `SetPlanned`/`ClearPlanned`/`BulkSetPlanned` all reset `PlanOrder = 0`. Mobile fallback: up/down arrow buttons visible on `@media (pointer: coarse)`. `window.planDragInProgress` suppresses SSE swaps during drag.

**Auto-promote carried-over:** `renderHomePage` merges overdue items into the planned lists so they appear inline rather than in a separate section. Carried-over items are detected in the template by `Planned < Today` and styled with a dotted peach left-border plus a `relativeDate` label. A "drop all carried" banner (POST `/plan/bulk/clear-carried`) lets users dismiss all overdue items at once. Summary cards (`topTasksExcluding`) exclude planned/carried-over slugs so they don't duplicate the plan section. `PlanPrompt` rotates by weekday for the empty-plan state.

**Calendar** (`internal/home/calendar.go`): `/plan/calendar` shows planned tasks across days in week or month view. `BuildCalendarDays` is a pure function that groups items by date. Week view: 7-column CSS grid (stacked on mobile), capped at 3 tasks per cell with "+N more" linking to `/?date=`. Month view: day cells with count badges linking to the homepage. Navigation via prev/next links and a "today" button. Week view supports drag-and-drop rescheduling via `planner.js`. Keyboard shortcut `g c`.

**API scoping:** Bearer token API uses the service registry for user 1's data. Per-user API tokens are not implemented -- can be added by mapping tokens to user IDs.
</ARCHITECTURE>

<CONVENTIONS>
- `tracker.NewUserStore` filters by `user_id`; `NewSharedStore` never does. Two constructors make intent explicit -- no conditional SQL.
- `user_id DEFAULT 1` in `tracker_items` means existing rows auto-belong to the first user with no data migration.
- Inline metadata tags (`[status: ...]`, `[tags: ...]`, `[deadline: ...]`, `[planned: ...]`, `[plan-order: N]`, `[from-idea: ...]`, `[converted-to: ...]`, `[deleted: YYYY-MM-DD]`) are parsed from checkbox lines only. Titles containing bracket patterns are a known limitation.
- `auth.TemplateData(r)` returns a base `map[string]any` with `UserName` and `IsAdmin`. All handlers merge page-specific data into this map. Use comma-ok type assertions when reading `UserName`: `if name, ok := data["UserName"].(string); ok { ... }`.
- Uploads are shared (not per-user). Random hex filenames with no ownership tracking.
- POST for destructive actions (delete, triage) -- HTML forms only support GET/POST. Destructive actions use themed confirmation modals (`dialog.js` + `#confirm-modal` in layout), not browser `confirm()`.
- Flash messages use query params (`?msg=key`) mapped to display text per handler package. Both success and error messages use this pattern. Only keys in `flashErrorKeys` render with error styling; all others render as success. `redirectBack` accepts an optional variadic `msg` parameter.
- `httputil.ServerError` generates 8-char hex correlation IDs for 500 errors. Use for all internal server errors. 400-level errors use `http.Error` directly with specific messages ("Item not found", "Failed to parse form data").
- Markdown rendering uses goldmark + bluemonday (UGC policy) for XSS sanitisation. Output is cast to `template.HTML` after sanitisation.
- `SecureCookies` defaults to `true`. Set `DASHBOARD_SECURE_COOKIES=false` for local HTTP development.
- All CSS animations respect `prefers-reduced-motion: reduce`. JS is ES5 compatible (no const/let/arrow functions).
- Modal system uses `.modal-overlay`/`.modal-content` as shared base CSS classes with `.visible` to toggle display. All modals (confirm, search, shortcut help) reuse this infrastructure.
- Bulk actions use `mutateBatch` for single-file-write atomicity. Select mode toggle on list pages; sticky bulk bar with `role="toolbar"` and `aria-live="polite"`. Escape key priority: confirm modal > search overlay > shortcut help > select mode. SSE swaps are suppressed while select mode is active.
- Permanent delete flash messages (`item-purged`, `idea-purged`) use error styling via `flashErrorKeys`. "Move to trash" is the user-facing label for soft delete.
</CONVENTIONS>

<GOTCHAS>
**Ideas parser vs tracker parser:** The ideas parser in `internal/ideas/parser.go` MUST preserve blank lines in indented body blocks. The tracker parser drops them. If you modify body parsing logic, ensure round-trip tests with blank lines still pass.

**Indented headings are body content:** Lines starting with spaces followed by `#` (e.g. `  ## Research`) are body lines, not section headings. Only non-indented `#` lines terminate an idea. This was a bug that was caught and fixed -- don't regress it.

**"personal" string in tracker store:** `tracker.NewStore(database, "personal")` uses "personal" as a DB category identifier. The `"Personal"` heading in `config.go` is the markdown file heading. Changing either requires a data migration.

**Legacy password migration:** `DASHBOARD_PASSWORD_HASH` auto-creates `admin@localhost` on startup if no users exist. This collapses auth to a single code path instead of maintaining two.

**ToTaskFunc signature:** `func(ctx, title, body string, tags []string, fromIdeaSlug string) (string, error)`. Returns the created task slug for provenance linking. All three closure implementations in `main.go` (auth-enabled, single-user, API) must match. Idea-to-task conversion marks the idea as "converted" (not deleted) and records bidirectional linkage.

**Search locking order:** `search.Handler.SearchAPI` locks services sequentially: personal tracker, then family tracker, then ideas. Never hold multiple service locks simultaneously -- `ToTask` writes to both ideas and tracker services, so concurrent locking would deadlock.

**insights package avoids import cycles:** `TagAggregation` accepts `[]TagInfo` (a simple struct) instead of concrete `tracker.Item`/`ideas.Idea` types. The homepage handler converts items to `TagInfo` before passing them. `WeeklyVelocity` returns a `VelocityInsight` struct (not a string) so templates can style parts independently.

**Shared tracker template:** `tracker.html` renders both `/todos` and `/family`. Empty state copy and "Recently Deleted" section both use `.ListName` context to scope routes correctly.

**MoveToList calls PermanentDelete, not Delete:** After soft-delete was added, `MoveToList` must use `PermanentDelete` on the source service. Using `Delete` would leave a ghost `[deleted:]` item in the source list's "Recently Deleted" section.

**Auto-purge multi-user iteration:** The service registry is lazily populated and has no iteration method. The purge goroutine queries `auth.AllUsers(database)` in auth-enabled mode to iterate all users. `shutdownCtx` is extracted before the auth/single-user branch so both modes can use it.

**Converted idea linkage survives soft-delete:** Trashing a converted idea does NOT affect the linked task (and vice versa). Permanent delete of either side leaves a dangling `[from-idea:]`/`[converted-to:]` reference -- accepted limitation.

**Caption XSS prevention:** `splitImageCaption` template function returns plain strings, never `template.HTML`. Following the `linkify` pattern (which returns `template.HTML`) would bypass all escaping. `SanitiseCaption` strips `|,]<>"` and truncates to 200 runes.

**Planner dual-mode handlers:** Plan routes need to work in both auth-enabled and single-user modes. Auth mode uses `home.Handler` methods (which resolve services per-request via the registry). Single-user mode uses `home.SingleUserPlanHandlers` closures over fixed service instances. Both are wired as `http.HandlerFunc` variables in `main.go` and passed to `mountAppRoutes`. The API plan handlers are similarly set in both branches.

**Carried-over items merged per-list:** Overdue items are appended to `PersonalPlanned`/`FamilyPlanned` in the handler before sorting, keeping list-source information intact for form actions. The template detects carried-over items by comparing `.Planned` against `.Today`. `ClearCarriedOver` iterates both services' `ListOverdue` results and calls `ClearPlanned` on each.
</GOTCHAS>

<TESTING>
`go test ./...` from repo root. All tests are in `test/` directory. Integration tests use temp dirs and in-memory SQLite -- no external services needed.
</TESTING>
