<ARCHITECTURE>
Go + chi + htmx server-rendered dashboard. Markdown files are the source of truth; SQLite caches tracker data for fast queries.

**Two flat-file patterns, separate parsers:**
- Tracker (`internal/tracker/`): tasks and goals in `personal.md`/`family.md`. Drops blank lines in bodies. DB-backed via `Store` for summary counts.
- Ideas (`internal/ideas/`): ideas in `ideas.md`. Preserves blank lines in bodies (rich markdown with paragraphs). No DB cache -- always reads from disk.

Both follow read-modify-write: parse file, mutate in memory, write back, release lock.

**Service registry** (`internal/services/`): Caches per-user service instances so `sync.RWMutex` coordinates concurrent requests for the same user. Creating services per-request would lose mutex coordination. `EnsureUserDirs` is deliberately separate from `ForUser` -- directory creation is an explicit side effect, not hidden in a getter.

**Auth evolution paths:** Session infrastructure is auth-method-agnostic. OIDC or passkeys can be added by writing a new callback handler that sets the same `user_id` session key. `RequireAuth` middleware does not change.

**API scoping:** Bearer token API operates on user 1's data. Per-user API tokens are not implemented -- can be added by mapping tokens to user IDs.
</ARCHITECTURE>

<CONVENTIONS>
- `tracker.NewUserStore` filters by `user_id`; `NewSharedStore` never does. Two constructors make intent explicit -- no conditional SQL.
- `user_id DEFAULT 1` in `tracker_items` means existing rows auto-belong to the first user with no data migration.
- Inline metadata tags (`[status: ...]`, `[tags: ...]`) are parsed from checkbox lines only. Titles containing bracket patterns are a known limitation.
- `auth.UserName(ctx)` checks first name, then falls back to email local part. Redefining this single function propagates display names to all 13+ handler data maps and templates automatically.
- Uploads are shared (not per-user). Random hex filenames with no ownership tracking.
- POST for destructive actions (delete, triage) -- HTML forms only support GET/POST.
- Flash messages use query params (`?msg=key`) mapped to display text. No session-based flash needed.
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
