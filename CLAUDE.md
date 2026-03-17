<ARCHITECTURE>
Go + chi + htmx server-rendered dashboard. Markdown files are the source of truth; SQLite caches tracker data for fast queries.

**Two flat-file patterns, separate parsers:**
- Tracker (`internal/tracker/`): tasks and goals in `personal.md`/`family.md`. Drops blank lines in bodies. DB-backed via `Store` for summary counts. In-memory cache serves reads; invalidated by mutations and file watcher.
- Ideas (`internal/ideas/`): ideas in `ideas.md`. Preserves blank lines in bodies (rich markdown with paragraphs). In-memory cache serves reads; no DB cache.

Both follow read-modify-write with a `mutate(slug, fn)` helper: lock, parse file, find by slug, apply callback, write back, update cache, release lock. Title edits re-slugify the item; the old slug becomes invalid after mutation.

**Service registry** (`internal/services/`): Caches per-user service instances. `sync.RWMutex` with RLock fast path for cache hits; filesystem I/O (`EnsureUserDirs`) runs outside the lock. `EnsureUserDirs` is deliberately separate from `ForUser` -- directory creation is an explicit side effect, not hidden in a getter.

**Package layout:** Handlers are split across `internal/tracker/`, `internal/ideas/`, `internal/admin/`, `internal/account/`, and `internal/home/`. Shared utilities: `internal/httputil/` (JSON response), `internal/auth/` (middleware, context helpers, `TemplateData`). Routes are registered once via `mountAppRoutes`, conditionally wrapped with auth middleware.

**Auth evolution paths:** Session infrastructure is auth-method-agnostic. OIDC or passkeys can be added by writing a new callback handler that sets the same `user_id` session key. `RequireAuth` middleware does not change.

**API scoping:** Bearer token API uses the service registry for user 1's data. Per-user API tokens are not implemented -- can be added by mapping tokens to user IDs.
</ARCHITECTURE>

<CONVENTIONS>
- `tracker.NewUserStore` filters by `user_id`; `NewSharedStore` never does. Two constructors make intent explicit -- no conditional SQL.
- `user_id DEFAULT 1` in `tracker_items` means existing rows auto-belong to the first user with no data migration.
- Inline metadata tags (`[status: ...]`, `[tags: ...]`) are parsed from checkbox lines only. Titles containing bracket patterns are a known limitation.
- `auth.TemplateData(r)` returns a base `map[string]any` with `UserName` and `IsAdmin`. All handlers merge page-specific data into this map.
- Uploads are shared (not per-user). Random hex filenames with no ownership tracking.
- POST for destructive actions (delete, triage) -- HTML forms only support GET/POST.
- Flash messages use query params (`?msg=key`) mapped to display text per handler package. No session-based flash needed.
- Markdown rendering uses goldmark + bluemonday (UGC policy) for XSS sanitisation. Output is cast to `template.HTML` after sanitisation.
- `SecureCookies` defaults to `true`. Set `DASHBOARD_SECURE_COOKIES=false` for local HTTP development.
</CONVENTIONS>

<GOTCHAS>
**Ideas parser vs tracker parser:** The ideas parser in `internal/ideas/parser.go` MUST preserve blank lines in indented body blocks. The tracker parser drops them. If you modify body parsing logic, ensure round-trip tests with blank lines still pass.

**Indented headings are body content:** Lines starting with spaces followed by `#` (e.g. `  ## Research`) are body lines, not section headings. Only non-indented `#` lines terminate an idea. This was a bug that was caught and fixed -- don't regress it.

**"personal" string in tracker store:** `tracker.NewStore(database, "personal")` uses "personal" as a DB category identifier. The `"Personal"` heading in `config.go` is the markdown file heading. Changing either requires a data migration.

**Legacy password migration:** `DASHBOARD_PASSWORD_HASH` auto-creates `admin@localhost` on startup if no users exist. This collapses auth to a single code path instead of maintaining two.
</GOTCHAS>

<TESTING>
`go test ./...` from repo root. All tests are in `test/` directory. Integration tests use temp dirs and in-memory SQLite -- no external services needed.
</TESTING>
