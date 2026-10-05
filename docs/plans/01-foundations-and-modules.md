# Plan 1: Foundations and module framework

## Goal

Make the dashboard safe, fast and pleasant to use. Then restructure it so a new feature area (a "module", such as exercise) can be added as one self-contained package plus one registration line, instead of edits spread across a dozen files.

This is the first of three plans:

| Plan | Scope | Status |
|---|---|---|
| 1 (this) | Fixes from the design and software reviews, wiring simplification, module framework, existing features migrated to modules, interaction rework, visual design uplift | In progress: Phases 1-8 deployed; next is Phase 9 |
| 2 | Exercise module on the framework, plus the SQLite migration hook it needs. Exercise scope is not yet decided | Not written |
| 3 | Product features: quick capture, stable item IDs (which also fix slug collisions), due dates, recurring tasks merged with maintenance cadence, agent-proposed daily plan, reminders, weekly review, MCP tooling and possibly a Go MCP server | Not written; write after Phase 7 lands |

## Context

In October 2026, eight review agents (design: visual, UX, accessibility, product; software: performance, architecture, security, quality) reviewed the repo. Five further agents reviewed this plan. Findings were checked against the code. Read `CLAUDE.md` before starting; its gotchas still apply.

**Adding a feature today touches about 12 places:**
- the nav in `layout.html`
- the `g x` switch in `shortcuts.js`
- the page list in `parseTemplates`
- the positional parameters of `mountAppRoutes`
- both auth branches in `main.go`
- `services.Registry`
- the filename prefixes in `watcher.classifyEventWithUser`
- the 5-tuple `search.ServiceResolver`
- the parameter list of `renderHomePage`
- the API section of `main.go`
- `docker-compose.yml`
- the MCP server

**The interaction model is the main source of UX bugs.** The cycle:
1. A mutation is a form POST, a 303 and a full page render.
2. The watcher sees the app's own write, re-parses, rewrites SQLite and broadcasts an unscoped `file-changed`.
3. Every open tab swaps its page container.

Three JS flags (`planDetailExpanded`, `trackerExpandedItems`, `planDragInProgress`) discard those swaps, so the UI goes stale. Focus is lost after swaps.

### Decisions already made

Do not reopen these while executing this plan.

- **Single user.** Auth stays on because the app is reachable remotely. No-auth mode is for local development only. It is enabled explicitly, restricted to loopback, and served by injecting user 1 instead of a separate wiring path. Admin and multi-user code is frozen: no new work, no deletion.
- **Storage.** Modules choose their storage. This plan builds the markdown path. The SQLite migration hook is deferred to Plan 2, where exercise is its first consumer.
- **Markdown remains the source of truth** for tasks, ideas and house.
- **Keep:** chi, the separate parsers, the house two-file split, `insights` as pure functions, sequential search locking.
- **htmx stays on 2.x** (latest 2.0.x). htmx 4 is a future migration.
- **Slugs.** Slug collisions and rename instability are fixed by stable IDs in Plan 3, not here. Slugs are recomputed from titles on every parse, so suffix-based de-duplication cannot persist.

## Requirements

1. All existing behaviour MUST keep working: tasks, family, goals, ideas, house, planner, calendar, digest, search, commentary, REST API, MCP.
2. Every bug fix MUST start with a failing test, committed before the fix.
3. `make lint test vuln e2e` MUST pass at the end of every phase. Each phase MUST leave `main` deployable.
4. After Phase 7, a module with a page, nav entry, shortcut, search results, home widget and live refresh MUST need changes only in its own package and template directory, plus one registration line.
5. Markdown files MUST NOT be left truncated or partially written by a crash, and concurrent readers MUST never see a partial file.
6. Both themes MUST meet WCAG 2.2 AA:
   - text contrast at least 4.5:1
   - non-text contrast, including the focus ring, at least 3:1
   - these MUST hold on every day of a leap year
7. The only new dependencies allowed are the idiomorph htmx extension (runtime), and Playwright and `@axe-core/playwright` (dev).

**Test conventions:**
- New Go tests go in `test/` and use map-based table tests.
- The existing temp-dir and in-memory SQLite suite stays ungated.
- Only Playwright and fault-injection tests that need real fsync or process kill are gated behind `INTEGRATION=1`.

## Non-goals

- Exercise, the SQLite migration hook, and new product features (Plans 2 and 3).
- Generalising the daily planner beyond tracker items.
- A design-token overhaul (spacing scale, type scale, `.btn` base, `@layer`) before Phase 9. It is part of Phase 9, not earlier phases.
- Removing multi-user or admin code.
- **Known WCAG gaps, accepted for a single keyboard-shortcut user and recorded in the backlog:**
  - single-key shortcut opt-out (2.1.4)
  - keyboard rescheduling in the calendar week view
- MCP Python tooling (uv, 3.14, ty). This moves to Plan 3.

---

## Phase 1: Toolchain, dependencies and test harness

**Purpose:** a trustworthy baseline. Later phases depend on lint, vuln checks, benchmarks and browser tests working.

- [x] **Go version.** Use one Go version across `go.mod`, the Dockerfile builder and CI, the latest stable (verify it; locally 1.27.1 is installed). Run `go mod tidy`.
- [x] **Linting.** Rebuild `golangci-lint` for that Go version. Commit a `.golangci.yml` with the default linters plus `gosec`, `errorlint`, `bodyclose` and `gocyclo` at a threshold of 15. Mark existing offenders with a named `//nolint:gocyclo // reduced in Phase 5` comment; `main` is at 68 and `renderHomePage` at 38. `make lint` MUST pass.
- [x] **Dependencies.** Upgrade chi (5.3.x or later), goldmark (1.7.17 or later), modernc sqlite, `x/crypto`, `x/net` and fsnotify. `govulncheck ./...` MUST report no called vulnerabilities. The review's chi and goldmark findings were not reproduced because the sandbox blocked it.
- [x] **Remove `goldmark-highlighting`.** bluemonday strips its inline styles, so it has no visible effect. Add a golden test of rendered markdown for a fenced code block before removing it, then update the golden.
- [x] **htmx.** Upgrade the vendored `htmx.min.js` and `htmx-sse.js` to the latest 2.0.x. Vendor `idiomorph-ext.min.js`, unused until Phase 8.
- [x] **Makefile targets.** Add `vuln`, `cover`, `e2e`, `bench` and `fmt`.
  - `bench` runs `BenchmarkMutate200` (a new benchmark: `UpdatePriority` on a 200-item tracker) with `-count=10`.
  - Record the baseline `benchstat` output in Working Notes.
- [x] **Playwright smoke suite in `e2e/`.** Bun or npm are both installed.

  Harness:
  - copy seed data from `e2e/fixtures/` into a temp dir
  - create a user from a test `DASHBOARD_PASSWORD_HASH`
  - take the port from env
  - `make e2e` builds, starts and stops the server
  - run with `DASHBOARD_SECURE_COOKIES=false`

  It MUST cover current behaviour:
  - login
  - add, complete, move to trash and restore a task
  - plan a task and complete it from the homepage
  - expand a plan item
  - reorder plan items with the arrow buttons
  - add, toggle and remove a sub-step
  - add and triage an idea
  - log house maintenance
  - search
  - confirm modal
  - SSE refresh after an external file edit
- [x] **Route golden file.** Add a test that builds the full router via `chi.Walk` and compares the method and path pairs against `test/testdata/routes.golden`, for both auth modes. Extract router construction from `main()` into a function so the test can call it. Any later change to the golden MUST be explained in Working Notes.
- [x] **CI** (`.github/workflows/build.yml`):
  - add a `pull_request` trigger
  - add jobs for lint, govulncheck, the race suite, MCP pytest and Playwright
  - image builds `needs:` all test jobs
  - pin actions to SHAs
  - add Dependabot for actions, Go modules and pip

**Verification:** `make lint test vuln e2e` passes locally and in CI on a PR.

**Self-review**, then continue.

---

## Phase 2: Security hardening

**Purpose:** close confirmed exposure paths. Every task starts with a failing test.

- [x] **Explicit auth mode.**
  - Replace implicit "no hash and no users means open" with `DASHBOARD_AUTH=disabled`.
  - The server MUST refuse to start when auth is off without that switch.
  - It MUST also refuse when the switch is set and `ADDR` is not a loopback address.
  - A lost data volume must never produce an open dashboard.
- [x] **Container exposure.**
  - The dashboard port MUST be bound to `127.0.0.1`, or not published if Caddy reaches it over the Docker network. Record which in the README.
  - Put the compose network on a fixed subnet (ipam) so trusted-proxy configuration is stable.
- [x] **Login rate limiting.**
  - Remove `middleware.RealIP`.
  - Use the client IP from the rightmost `X-Forwarded-For` entry only when the direct peer is in `DASHBOARD_TRUSTED_PROXIES` (CIDRs, default empty, meaning use `RemoteAddr`).
  - Document the Caddy configuration.
  - Bound the limiter map with least-recently-used eviction. It must never reset wholesale.
  - Add per-account progressive *delay* (never a lockout, which would let an attacker lock out the only user).
  - Test that a spoofed `X-Forwarded-For` or `X-Real-IP` from an untrusted peer is ignored.
- [x] **Login timing.** Unknown emails run bcrypt against a fixed dummy hash. Stop logging submitted emails.
- [x] **API and MCP tokens.**
  - If `DASHBOARD_API_TOKEN` is unset or shorter than 32 characters, `/api/v1` is not mounted and an error is logged.
  - Add a separate inbound `MCP_TOKEN` for the sidecar, and rate-limit failed bearer attempts.
  - Destructive MCP tools are disabled unless `MCP_ALLOW_DESTRUCTIVE=true`: `delete_todo`, `remove_substep`, `clear_carried_plan` and `delete_commentary`. They also get `destructiveHint` annotations. Update the MCP smoke tests.
- [x] **Cross-origin protection.**
  - Wrap the whole web router, including `/login`, `/logout` and `/upload`, in `http.NewCrossOriginProtection()`.
  - Mount `/api/v1` on a separate subrouter outside it. Do not use bypass patterns.
  - Test that a mismatched `Origin` and `Sec-Fetch-Site: cross-site` are rejected, and that htmx and form posts from the same origin pass.
- [x] **Security headers middleware:**
  - `Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self'`
  - `X-Content-Type-Options: nosniff`
  - `Referrer-Policy: same-origin`

  Set `htmx.config.allowEval=false` via `<meta name="htmx-config">`. Record tightening `script-src` as a follow-up; there are 98 inline handlers today.
- [x] **HTTP server.**
  - Use an `http.Server` with `ReadHeaderTimeout`, `ReadTimeout` and `IdleTimeout`.
  - The SSE handler clears its deadlines via `http.ResponseController` and sends a heartbeat comment every 30s.
  - Broker subscribers close on `RegisterOnShutdown`.
  - Graceful shutdown on SIGTERM uses the existing `shutdownCtx`.
  - Set `sm.IdleTimeout` on the session manager.
- [x] **Container hardening.**
  - Add bind mounts for `IDEAS_PATH` and `UPLOADS_DIR`. Today they sit on the container filesystem, and `config.go` creates them at boot.
  - Run both images as a non-root user.
  - Add a documented one-off chown step for existing root-owned data.
  - Set `read_only: true` with all data paths mounted read-write, `cap_drop: [ALL]` and `no-new-privileges`.
  - Pin base images by digest.
  - Verify by running `docker compose up` against a copy of real data.

**Verification:** new tests pass. Playwright passes. A compose smoke run starts, logs in and writes a task.

**Self-review**, then continue.

---

## Phase 3: Data integrity and performance

**Purpose:** crash-safe writes, no self-triggered work, correct SQLite configuration and cache-safe assets.

- [x] **Backup first.**
  - Add a `make backup` target that copies every markdown file and the DB to a timestamped directory.
  - Run it and document restoring in the README.
- [x] **Round-trip fixtures.**
  - Add real-shaped fixtures for each parser: blank lines in idea bodies, indented headings, sub-steps, image captions, all inline metadata, soft-deleted items and maintenance logs.
  - Parse-then-write MUST be byte-identical (or match a documented normalisation).
  - This guards every later change.
- [x] **SQLite configuration.**
  - Set the pragmas through the modernc DSN so every pooled connection gets them: `_pragma=busy_timeout(5000)`, `foreign_keys(1)`, `journal_mode(WAL)`, `synchronous(NORMAL)`.
  - Remove `?_busy_timeout=5000` and the `db.Exec("PRAGMA ...")` loop.
  - Test: open three pooled connections and assert the pragma values on each.
- [x] **Atomic file writes.** Add one shared helper that:
  1. creates a temp file in the target's directory with mode `0600`, named with a leading `.` and a non-`.md` suffix
  2. writes and fsyncs it
  3. chmods it to the target's mode
  4. renames it over the target
  5. fsyncs the directory

  Use it in:
  - the tracker, ideas and house writers
  - skeleton creation in `config.go` and `services/registry.go`
  - `cmd/dashboard/migrate.go`

  Also remove stale temp files at startup. Tests:
  - inject failure after the write, at fsync and at rename; the original bytes are unchanged and the temp file is removed
  - a concurrent-reader test (`INTEGRATION=1`) hammers reads during writes and never sees a partial file
- [x] **`MoveToList` ordering.** Add to the target before deleting from the source, so a failed add cannot lose the item.
- [x] **Change events from services, not the watcher.**
  - Inject a publisher interface into the tracker, ideas and house services. After every successful write, from web, API or MCP, they publish a change event for their category.
  - Each service records a content hash of its last write.
  - Watcher callbacks return whether the file changed. When the hash matches the service's last write, the watcher skips `Resync` and does not broadcast.
  - The watcher today sends `file-changed` before running callbacks; reorder that.
  - Test seams: a fake publisher counting events, and a `Resync` counter on the service.
  - Test that one web mutation and one API mutation each produce exactly one event and zero `Resync` calls, and that an external edit produces one of each.
- [x] **Drop the SQLite tracker mirror.**
  - Remove `store.ReplaceAll` from `mutate`, `AddItem`, `PermanentDelete` and `Resync`, and compute `Summary()` from the cache.
  - Remove the `store` parameter from `tracker.NewService` (about 30 call sites in 10 test files).
  - Remove `NewUserStore`, `NewSharedStore` and `ReplaceAllWithAttribution` and their tests in `registry_test`.
  - Adjust `admin_test`'s `tracker_items` cascade assertion.
  - Check `multiuser_test` against the in-memory count, since the SQL unique index used to collapse duplicate slugs.
  - The table stays in the schema, unused.
- [x] **Static asset versioning.**
  - Hash each embedded static file at startup.
  - A `static "theme.css"` template function returns `/static/theme.css?v=<hash>`, and every template uses it.
  - `immutable` caching applies only to versioned requests.

**Verification:**
- `make bench` shows `BenchmarkMutate200` at least 3x faster than the Phase 1 baseline (`benchstat`, same machine), measured excluding the atomic-write fsyncs (owner decision 2026-10-04; see Phase 3 notes).
- Round-trip, fault-injection and event-count tests pass.
- Playwright passes.

**Self-review**, then continue.

---

## Phase 4: Accessibility and visual correctness

**Purpose:** fix WCAG 2.2 AA failures, and lay the shared toast and id foundations that Phase 8 reuses.

- [x] **Seasonal accent contrast.**
  - `internal/seasonal` computes a per-hue lightness and returns a concrete colour per theme, which the layout injects.
  - A test calls that same function for every day of 2028 (a leap year). It asserts at least 4.5:1 against the `--base` and `--mantle` values, and at least 3:1 for the focus ring.
  - The test reads token values parsed from `theme.css`, not duplicated constants.
- [x] **Light-theme text tokens.**
  - Add `--success-fg`, `--warning-fg`, `--danger-fg`, `--priority-medium-fg` and `--on-accent`, darkened in light mode only.
  - Point text usages at them. Catppuccin hues remain for borders and fills.
  - The same token-parsing test asserts at least 4.5:1. Measured failures today: green 2.75, teal 3.08, peach 2.64, yellow 2.31, medium priority 2.92.
- [x] **Stable id scheme.**
  - `item-{slug}` for tracker rows, `plan-{list}-{slug}` for plan rows, `{row-id}-substep-{n}-{action}` for sub-step buttons.
  - Record it in Working Notes; Phase 8 morph swaps depend on it.
  - Renaming an item changes its id. This is accepted until Plan 3's stable IDs.
- [x] **Plan rows.**
  - Replace `role="button" tabindex="0"` on `.plan-item` with a real `<button aria-expanded aria-controls>` toggle. The row click stays as a mouse convenience.
  - Render the reorder buttons on all pointers, with an `aria-label` that names the task.
- [x] **Announcer and toast.**
  - Add one `#announcer` polite live region and a toast partial to the layout. The toast shows text only, set via `textContent`.
  - Reorder announces the new position and returns focus to the moved control.
  - Remove the duplicate flash in `homepage.html`.
  - Flash messages use `role="status"`, or `role="alert"` for `flashErrorKeys` only.
  - Remove `aria-live` from the MCP footer badge.
- [x] **Target size and focus.**
  - Every interactive control is at least 24x24 CSS px; 44 px on coarse pointers.
  - Add `scroll-padding` for the sticky nav and the bulk bar.
  - Restore a visible focus outline on `.form-input` and `.search-input`.
- [x] **Semantics.**
  - Add a skip link to `<main>`.
  - The homepage always has an `<h1>`.
  - Add labels (visible or `aria-label`) to the goal, idea and tracker edit fields.
  - Declare `color-scheme: light dark`.
  - The first-visit theme follows `prefers-color-scheme`.
- [x] **Accessibility tests.**
  - Add `@axe-core/playwright` scans on every main page in both themes, and in these states: item expanded, select mode, modal open, mobile nav open, toast visible.
  - Add a 320px-wide run asserting no horizontal page scroll except the house table.
  - Add a `reducedMotion: 'reduce'` run.
  - Add a keyboard-only test that expands, reorders and completes a plan item.

**Verification:**
- axe reports zero serious or critical violations.
- The contrast tests pass.
- Record a manual VoiceOver pass over the homepage and todos in Working Notes.

**Self-review. STOP and wait for human review.** The owner should use the fixed app before structural work begins.

---

## Phase 5: Simplify wiring

**Purpose:** remove the duplicated wiring before building the framework. The route golden from Phase 1 guards this phase.

- [x] **Collapse no-auth mode.**
  - With `DASHBOARD_AUTH=disabled`, a middleware injects user 1 into the context, including the name and admin fields that `auth.TemplateData` reads. It wraps every route, including `/events`.
  - Delete `SingleUserPlanHandlers`, `HomePageSingle`, `DigestPageSingle`, `CalendarPageSingle` and the single-user branch.
  - The expected route-golden diff (no-auth mode gains `/login`, `/account` and `/admin/*`, or they are excluded deliberately) is explained in Working Notes.
- [x] **Local-dev data paths.**
  - Verify how no-auth mode resolves files today (`PERSONAL_PATH` and `IDEAS_PATH`, versus `USER_DATA_DIR/1/`).
  - If they differ, a registry override for user 1 MUST cover service paths, the watcher's watch spec and skeleton creation.
  - Ensure a user-1 row exists, so the purge loop, which iterates `auth.AllUsers`, still runs.
- [x] **Commentary scoping.**
  - Web commentary and the ideas handler use `auth.UserID(r.Context())` instead of a hard-coded `1`.
  - This is safe only after the previous task, because no-auth requests currently carry user 0.
- [x] **One `toTask` factory.**
  - Replace the three `ToTaskFunc` closures with one function over the personal, family and house services.
  - `AddItem` returns the slug it assigned; callers, including `APIAddTodo`, stop recomputing it.
  - No de-duplication (see Decisions).
- [x] **API through the resolver.** The API resolves services through the same registry and resolver as the web handlers, with the API token mapped to user 1. Delete the duplicated API handler construction.
- [x] **Route mounting.** Replace the positional parameters of `mountAppRoutes` with a struct, and move API registration into a function next to it.
- [x] **Remove the Phase 1 `//nolint:gocyclo` markers** on `main` and `renderHomePage` by splitting them up.

**Verification:**
- The route golden matches, with the documented diff.
- `gocyclo -over 15` reports nothing outside the documented exclusions.
- The race suite and Playwright pass.

**Self-review**, then continue.

---

## Phase 6: Module framework

**Purpose:** define the module contract and the core plumbing that consumes it, proven by a permanent test fixture module.

The shape below is guidance. Names MAY change; the outcomes MUST hold. Use small optional capability interfaces:

```go
// internal/module
type Module interface {
    Manifest() Manifest  // ID, Title, NavItems{Label, Path, Order, Group, Shortcut}, template dir
    Routes(r chi.Router) // receives an already-authenticated, CSRF-protected sub-router
}
type APIRouter  interface { APIRoutes(r chi.Router) }        // receives the bearer-auth API sub-router
type Watcher    interface { Watches() []WatchSpec }           // exact filenames, reload func returning changed bool
type Searcher   interface { Search(ctx context.Context, userID int64, q string) []SearchResult }
type HomeWidget interface { Widget(ctx context.Context, userID int64, now time.Time) (WidgetData, bool) }
```

Modules receive one `module.Deps` struct: location, templates, change publisher, service registry, config and data paths. Do not use globals.

- [x] **Registry and validation.** At startup the registry rejects:
  - duplicate module IDs
  - duplicate or overlapping route prefixes
  - reserved prefixes (`/api`, `/login`, `/logout`, `/admin`, `/account`, `/static`, `/uploads`, `/events`, `/search`, `/plan`, `/digest`)
  - duplicate shortcut keys
  - keys reserved by core: `h c d / ? n k`, Escape and the arrows. `n` is held for Plan 3 quick capture.

  Ordering is deterministic.
- [x] **Templates and shared components.**
  - Module templates live in `web/templates/<module-id>/`. Widen the embed pattern.
  - Add `web/templates/_components/` with partials for: page header, empty state, item row, quick-add disclosure, card, error banner and toast.
  - A render helper injects the layout data (`auth.TemplateData`, nav, flash).
  - Module templates MUST NOT use inline `on*` handlers.
  - FuncMap functions returning `template.HTML` stay limited to core sanitised markdown and `linkify`.
- [x] **Nav and shortcuts.**
  - The layout renders nav from the registry. `aria-current="page"` is set by path prefix, so `/ideas/x` highlights ideas.
  - `Group` is primary or more. At most 6 primary links; the rest go behind a "more" disclosure button.
  - Links carry `data-shortcut`. `shortcuts.js` builds the `g x` map from those attributes, and `?` help lists them.
  - The hamburger closes on Escape and returns focus.
  - Check the nav at 320px.
- [x] **Watcher and SSE.**
  - The watcher is driven by `WatchSpec`s with exact filenames, replacing the `HasPrefix(subpath, "personal")` matching.
  - Events are named `changed:<module-id>` and carry no item content.
  - No per-user SSE routing (single user; record this as a known limitation).
  - Services publish through `Deps`.
- [x] **Search.** Search iterates registered `Searcher`s sequentially and never holds two module locks at once. Remove `search.ServiceResolver`.
- [x] **Home widgets.**
  - `WidgetData` is data, not HTML: title, count, up to 5 items, link, empty text and severity.
  - One core partial renders each widget as `<section aria-labelledby>` with an `<h2>`, below the plan section, in registry order.
- [x] **Fixture module.**
  - Add `internal/module/moduletest`, a permanent fixture with one page using the shared partials, one nav item, one shortcut, one search result, one home widget and one watched file.
  - The contract test asserts:
    - each capability appears in the right place
    - an unauthenticated request to its route redirects to login
    - editing its watched file emits `changed:moduletest`
    - its empty state and error banner render through the shared partials

**Verification:** the contract test, the route golden (unchanged), the race suite and Playwright pass.

**Self-review**, then continue.

---

## Phase 7: Migrate existing features to modules

**Purpose:** one way to build features. `main.go` only registers modules and wires core services.

- [x] **`todos` module:** the personal tracker plus goals (Plan 3 decides whether goals fold into tasks). Routes, templates, nav (`g t`, `g o`), watch spec, search, widget and API.
- [x] **`family` module:** the shared tracker. `tracker.html` keeps scoping routes by `.ListName`.
- [x] **`ideas` module:** list, detail, triage, research and to-task conversion via the Phase 5 factory.
- [x] **`house` module:** maintenance and projects, keeping the two-file split.
- [x] **Core pages stay outside modules:** home and planner, calendar, digest, search, auth, account, admin, uploads and commentary. Record in Working Notes which could become capabilities later, such as a `Plannable` capability for Plan 2.
- [x] **Delete the replaced wiring:**
  - hard-coded nav links
  - the `shortcuts.js` switch
  - the `parseTemplates` page list
  - the watcher prefixes
  - module-specific `renderHomePage` parameters
- [x] **Rename SSE triggers** to `sse:changed:<id>` in templates *and* JS (`tracker.js` re-triggers `sse:file-changed` today). The homepage listens to every module that contributes to it.
- [x] **Requirement 4 evidence.** In Working Notes, list the files outside `internal/<module>/` and `web/templates/<module>/` that each migration touched beyond the registration line. Investigate anything other than the registration line and either fix it or justify it.

**Verification:**
- The route golden is unchanged.
- Playwright passes.
- A new Playwright test shows that an external edit to `ideas.md` refreshes `/ideas` and the homepage but not `/todos`.

**Self-review. STOP and wait for human review.**

---

## Phase 8: Interaction rework

**Purpose:** replace POST, redirect and full-swap cycles, and the suppression flags, with targeted updates that keep UI state.

- [x] **Stable SSE connection.** Move `sse-connect` to a layout element that is never swapped. It is currently on swapped containers such as `tracker.html` and `homepage.html`, so each event reconnects.
- [x] **Morph swaps.** Enable idiomorph. Live-refresh containers morph using the Phase 4 id scheme, so expanded items, select mode, focus and in-progress drags survive refreshes.
- [x] **Remove the suppression flags.**
  - Remove `planDetailExpanded`, `trackerExpandedItems`, `planDragInProgress` and the `htmx:beforeSwap` discard logic.
  - Any refresh that must wait (mid-drag) is queued and applied afterwards, never dropped.
  - Playwright: an external edit made while an item is expanded appears without collapsing it.
- [x] **Fragment responses.**
  - With `HX-Request`, handlers return the updated fragment, or 204 plus `HX-Trigger`. The 303 stays for plain posts.
  - Keep the existing 204 behaviour for planner XHR (`home/handler.go` already checks `HX-Request` and `X-Requested-With`).
  - Cover:
    - plan complete, clear, set and reorder
    - tracker complete, uncomplete, priority, tags, edit, delete and restore
    - idea triage
    - maintenance log
  - Build `HX-Trigger` with `json.Marshal` carrying flash *keys*, not titles.
  - Fragment responses send `Vary: HX-Request` and `Cache-Control: no-store`.
  - `HX-Request` is never treated as proof of origin.
- [x] **Focus after change.** When a row is completed, trashed or moved out of view, focus moves to the next row, or to the list heading if the list is now empty. Add a Playwright keyboard assertion.
- [x] **Undo instead of confirm.**
  - Soft delete shows the Phase 4 toast with an undo button posting to `/restore`.
  - The toast stays visible at least 10s, pauses on hover or focus, and is reachable by keyboard.
  - Purge and user deletion keep the confirm.
- [x] **Native dialogs.**
  - The confirm modal, search overlay and shortcut help become `<dialog>` with `showModal()`.
  - Remove the focus trap in `dialog.js` and simplify the Escape chain in `shortcuts.js`.
  - Search uses the combobox and listbox pattern with `aria-activedescendant`.
- [x] **Submission feedback.**
  - Add `hx-disabled-elt` on mutating forms, inherited from `<body>` where possible, and a per-row indicator with `aria-busy`.
  - Idea triage surfaces errors instead of reloading on failure.

**Verification:**
- Playwright covers each converted action.
- From network logs, one mutation causes one fragment request and no full-page GET.
- axe is still clean in the interactive states.

**Self-review**, then continue.

---

## Phase 9: Design uplift

**Purpose:** one coherent visual design, applied once to the Phase 6 shared components and the Phase 7 module templates, after Phase 8 has settled the interaction patterns (toasts, inline updates, native dialogs). Requested by the owner after Phase 4. Doing it earlier would mean restyling templates that Phases 6 and 7 rewrite.

- [ ] **Review and direction.**
  - Capture a baseline screenshot set: every main page in both themes at 1280px and 375px, plus the key states (item expanded, select mode, dialog open, toast). Generate it with a Playwright job whose screenshots CI uploads as an artefact, because browsers cannot run in the local sandbox.
  - Run a design review (Design Reviewer agent) over the screenshots and templates covering hierarchy, density, typography, spacing rhythm, consistency between pages, the mobile layouts, and the homepage as a daily planning hub.
  - Write a short direction proposal for the owner: what changes and what stays, with mockups or a styled prototype page.
  - **Owner decision required:** keep and refine the monospace, terminal-style Catppuccin identity, or allow a broader visual change.
  - **STOP for the owner to approve the direction** before implementing.
- [ ] **Design tokens.**
  - Spacing scale, type scale (sizes, weights, line heights), radii, borders, elevation, and motion durations.
  - A `.btn` base with variants (primary, secondary, quiet, danger, icon) that replaces the per-feature button classes.
  - Organise `theme.css` with `@layer` (reset, tokens, base, components, pages, utilities). Split it into files if that helps; the static asset hashing already handles several files.
  - Replace hard-coded sizes and colours with tokens.
  - Phase 4's colour tokens and contrast tests stay authoritative. New colours are added as tokens and pass `TestThemeRuleTextContrast`.
- [ ] **Components.** Restyle the shared partials and core UI to the direction:
  - page header, item row, card, quick-add, empty state, error banner, toast, dialogs, nav (including the "more" disclosure), filters, bulk bar, badges and forms
  - keep the Phase 4 id scheme and the Phase 8 morph behaviour intact
- [ ] **Pages.**
  - Homepage hierarchy, with the plan as the primary section and the widgets below it.
  - List density on todos, family, goals and ideas, the house table, the calendar and the digest.
  - Mobile layouts, including tracker row height under the 44px touch targets (the owner flagged taller rows in Phase 4).
- [ ] **Clean-up.** Remove CSS that no template uses, and record the size of `theme.css` before and after in Working Notes.

**Verification:**
- axe, contrast, reflow, target-size, reduced-motion and keyboard tests all pass in both themes.
- The PR includes the before and after screenshot sets.
- No behaviour changes: the route golden is unchanged and Playwright passes.

**Self-review. STOP and wait for human review.** The owner uses the redesigned app before the documentation is written.

---

## Phase 10: Documentation

- [ ] Update `CLAUDE.md` using the `claude-md-authoring` skill. Cover:
  - the module contract and how to add a module
  - auth modes
  - atomic writes and service-published events
  - morph swaps, which replace the SSE-suppression gotchas (delete those)
  - removal of the SQLite mirror ("DB-backed via Store for summary counts" is no longer true)
  - the single wiring path
  - the id scheme
- [ ] Update `README.md`:
  - current features
  - the full API table
  - env vars: `DASHBOARD_AUTH`, `DASHBOARD_TRUSTED_PROXIES`, `MCP_TOKEN`, `MCP_ALLOW_DESTRUCTIVE`
  - the Caddy configuration
  - backup and restore
  - an "Adding a module" section
- [ ] Update `docs/backlog.md`: remove completed items (CSRF, focus traps, ServiceMap refactor) and add the follow-ups from Working Notes.

**Final self-review against the success criteria. STOP and wait for human review.**

---

## Success criteria

| Criterion | Measure |
|---|---|
| No reachable vulns | `make vuln` reports zero called vulnerabilities |
| Gates | `make lint test vuln e2e` green locally and in CI; CI gates image builds |
| Security | Spoofed forwarding headers ignored; no API without a token; auth cannot be off by accident or off-loopback; cross-origin POST rejected; CSP present; destructive MCP tools off by default |
| Crash-safe writes | Fault-injection and concurrent-reader tests pass; round-trip fixtures byte-identical |
| No self-triggered work | One mutation (web or API) produces one event and zero `Resync` calls |
| Speed | `BenchmarkMutate200` work at least 3x faster than the Phase 1 baseline, excluding atomic-write fsyncs |
| Accessibility | axe has zero serious or critical violations across pages, themes and interactive states; contrast holds every day of 2028 |
| Extensibility | The fixture module passes the contract test; the Phase 7 evidence shows only registration lines outside module directories |
| Simpler wiring | Single-user branch deleted; no `gocyclo` exclusions left on `main` or `renderHomePage`; one `toTask` |
| No stale UI | An external edit while an item is expanded appears without collapsing it |
| Behaviour preserved | Route golden diffs are all explained in Working Notes |
| Design | Direction approved by the owner; tokens and `@layer` in place; before and after screenshots in the Phase 9 PR; accessibility gates still green |

## Risks

- **Ownership change.** Moving to non-root images needs the existing data chowned. A missed path makes writes fail. The compose smoke run against copied real data catches this.
- **Morph and id stability.** Morph reuses elements by id. Duplicate ids reuse the wrong element, and renames lose state until Plan 3. Playwright covers the expanded, select and drag states.
- **CSP stays permissive for scripts** until the inline handlers are moved, which is a follow-up.
- **Events published by services** must fire for API and MCP writes too. If any write path is missed, open tabs stop refreshing for it. The event-count tests cover web and API.
- **Plan size.** Finish each phase's verification before starting the next. Do not interleave phases.

## Working notes

_For the executing agent. Record decisions, deviations, measurements and follow-ups here._

- Bash in this environment needs `dangerouslyDisableSandbox: true` (nono). `govulncheck` needs write access to `~/go/pkg/sumdb`. If blocked, ask the owner to restart with that path allowed; do not work around it.
  - Resolved: the clono launcher now sets `GOPATH`/`GOBIN` to `~/.cache/clono/shared/gopath`, which is writable. `GOBIN` is not on `PATH`; the Makefile resolves it via `go env`.
  - The clono network allowlist blocks `storage.googleapis.com` (proxy.golang.org redirects large module zips there), `vuln.go.dev`, `github.com` and Docker Hub. Blocked as a result: the modernc sqlite upgrade, building golangci-lint (module `github.com/MirrexOne/unqueryvet`), `make vuln`, and verifying the `golang:1.27.1-alpine` tag. `api.github.com`, `proxy.golang.org` (small zips) and `registry.npmjs.org` work.

### Phase 1 notes

- Go 1.27.1 everywhere (`go.mod`, Dockerfile builder; CI reads `go.mod`).
- Dependencies: chi 5.2.1 -> 5.3.2, goldmark 1.7.12 -> 1.8.6, x/crypto 0.49 -> 0.57, x/net 0.51 -> 0.59, fsnotify 1.9 -> 1.10.1. modernc sqlite 1.36.3 -> 1.60.1 (libc 1.77.1), run by the owner outside the sandbox and verified by CI.
- `goldmark-highlighting` removed; golden `test/testdata/markdown/fenced_code.html` confirmed it only emitted unstyled `<span>`s. Removing it also dropped chroma, regexp2 and an old `x/exp`.
- htmx 2.0.8 -> 2.0.11; `htmx-sse.js` was already byte-identical to `htmx-ext-sse` 2.2.4 (latest), so unchanged; `idiomorph-ext.min.js` 0.8.0 vendored, unreferenced until Phase 8. npm tarball shasums verified against the registry. Static assets are still served `immutable` without versioning until Phase 3, so an open browser may need a hard refresh after deploy.
- Router construction moved from `cmd/dashboard/main.go` to `internal/app.NewRouter` so `test/routes_test.go` can call it (the test package cannot import `main`). The legacy admin auto-create moved with it. `authEnabledFlag` global replaced by a closure argument to `buildFuncMap`. The gocyclo hotspot is now `NewRouter` (62), not `main`.
- gocyclo > 15 found eight functions, not two. `NewRouter` and `renderHomePage` are marked `reduced in Phase 5`. The other six (`runMigrateData`, `parseItemLine`, `writeItem`, `insights.Digest`, `admin UpdateUser`, `ParseMaintenance`) are flat per-field branches and are marked accepted with a reason; they are not Phase 5 work.
- Route goldens: `test/testdata/routes_auth.golden` (149 lines) and `routes_noauth.golden` (134). The diff between them is exactly `/login`, `/logout`, `/account*` and `/admin/*`. `r.Handle` routes (`/static/*`, `/uploads/*`) appear once per HTTP method; that is how `chi.Walk` reports them.
- CI: `pull_request` trigger; jobs lint, vuln, test (race), test-mcp, e2e; image builds need all five and run only on push. Actions pinned to SHAs of their latest releases (checkout v7.0.1, setup-go v7.0.0, setup-python v7.0.0, setup-node v7.0.0, upload-artifact v7.0.1, docker login v4.6.0, metadata v6.2.0, build-push v7.4.0, golangci-lint-action v9.3.0 running golangci-lint v2.14.0). govulncheck pinned at v1.8.0. Dependabot covers actions, gomod, pip (`/mcp`) and npm (`/e2e`). `e2e/package-lock.json` was generated outside the sandbox; `run.sh` uses `npm ci` and the e2e job caches npm on it.
- Benchmark baseline (Apple M5 Max, `make bench`, raw output in `docs/plans/bench-phase1.txt` for Phase 3's `benchstat` comparison): `BenchmarkMutate200` 3.070 ms ± 2%, 1.128 MiB/op, 9.486k allocs/op. Phase 3 target: 1.02 ms or less.
- Playwright suite (`e2e/`): runs in CI only. Locally the sandbox blocks binding any local port, `~/.npm` is read-only, the ms-playwright cache dir is denied, and the proxy 403s scoped npm packages. Green twice in a row on PR #1 (16 passed, 1 fixme). Versions of `@axe-core/playwright` and `@types/node` are caret ranges because the registry metadata could not be read; `@playwright/test` 1.63.0 matches the reachable unscoped `playwright` release. User 1's personal and ideas files live under `USER_DATA_DIR/1/` in auth mode, so fixtures seed `users/1/`.
- Existing bug found while writing the suite (not fixed; Phase 8 removes the flags): `tracker.js`'s `htmx:beforeSwap` guard only matches targets with `.tracker-page`, so `planDetailExpanded` and `planDragInProgress` never suppress SSE swaps on the homepage (`.homepage-page`) or house page. CLAUDE.md claims otherwise. The e2e test for it is `test.fixme`.
- e2e flakiness risks to watch on the first real run: specs wait for SSE quiet (`waitForSseSettle`, see below) before expanding rows on the homepage and house page, because the app's own writes trigger an unsuppressed SSE refresh about 500ms later (removed in Phase 8); the watcher broadcasts before it resyncs (reordered in Phase 3); the reorder test relies on `hasTouch` making Chromium report `pointer: coarse` and checks that first; the suite uses 3 of the 5 logins per minute the rate limiter allows, so other specs must reuse the stored session. `retries: 0` is deliberate.
- `test/routes_test.go` builds the real router, which starts a file watcher with no stop hook; each subtest leaves one behind on a removed temp dir until the test binary exits. Harmless today; Phase 6 replaces the watcher and should give it a context.

#### First CI run (PR #1)

- vuln and test passed. govulncheck reports no called vulnerabilities, including on modernc sqlite 1.36.3.
- test-mcp failed for a pre-existing reason: `mcp>=1.27` resolved to mcp 2.3.0, which renamed `FastMCP`. The MCP Docker image installs the same file, so the next image build from `main` would have shipped a broken sidecar. Pinned `mcp>=1.27,<2`; migrating to 2.x belongs with Plan 3's MCP work.
- e2e: 15 passed, 2 failed, 1 skipped (the fixme), then the job hung for 29 minutes. Cause: `main` catches SIGTERM via `signal.NotifyContext` but never shuts the server down, so the process ignores `kill` and `run.sh`'s `wait` blocked forever. `docker stop` hits the same bug (it falls back to SIGKILL after 10s). Phase 2's graceful shutdown fixes the app; `run.sh` now escalates to SIGKILL after 5s and the job has `timeout-minutes: 15`. Failing specs to diagnose from the next run's traces: `tasks.spec.ts` sub-step Toggle reported "not visible" after the add swap succeeded, and `ideas.spec.ts` add-and-triage (error not captured).
- lint: 75 findings. Handled as follows:
  - errcheck (50): the `std-error-handling` preset covers Close, Flush, os.Remove and fmt.Fprint*. The rest are fixed: test setup calls now `t.Fatal` on error, deferred `Rollback` is explicit, the house page logs template errors like the other handlers, and the watcher logs failures to watch new paths.
  - gosec (19): G304, G301 and G306 are excluded in `.golangci.yml` with reasons. G114 is fixed with an `http.Server` carrying `ReadHeaderTimeout` (Phase 2 adds the rest). G203, G202 and the G710 on house have `nolint` with reasons. The tracker and login G710s were real; see below.
  - staticcheck: `middleware.RealIP` (SA1019, spoofable) is marked for removal in Phase 2. Until then, anyone can bypass the login rate limit by sending a forged `X-Forwarded-For`.
  - gocyclo in `_test.go` files is excluded; table tests are long by design.
- Second CI run: 46 more lint findings. The first run was truncated by golangci-lint's default caps (50 issues per linter, 3 identical issues). `.golangci.yml` now sets both caps to 0. About 180 unchecked errors in tests were wrapped in `if err := ...; err != nil { t.Fatal(err) }` by an AST-based rewrite, checked by standalone `errcheck` (zero remaining) and the race suite. gosec G104 is excluded (it duplicates errcheck), and gosec is off for `_test.go` files (template-func stubs and temp-dir fixtures).
- Idea triage buttons never worked (fixed, test-first in `TestIdeasTriageActionMultipart`): `triageAnimate` posts `new FormData(form)` (multipart), but `TriageAction` called `r.ParseForm()`, which ignores multipart bodies, and `FormValue` then skips multipart parsing because `r.Form` is already set. The action arrived empty, the handler returned 400, and the JS reloaded the page regardless, hiding the failure. Found by the e2e suite on its first real run. Phase 8's "idea triage surfaces errors" item covers the swallowed error.
- e2e timing: the app's own writes broadcast `file-changed` about 500ms later, and the resulting container swap closes open `<details>`, collapses rows and detaches any form the confirm modal is holding (so Confirm silently does nothing). The last one is a real user-facing bug whenever any write lands while the modal is open, including one from the API or MCP. Phase 3 does not remove it (services still publish every write, debounced); Phase 8 (morph swaps, native dialogs) does. `waitForSseSettle` now waits for 1.2s of quiet, tracked through `htmx:sseMessage`, `htmx:beforeRequest` and `htmx:afterSettle`, instead of sleeping a fixed 1.5s, and the add/plan helpers call it after every navigation that follows a write.
- Sub-step actions collapsed the expanded task (fixed; the failing e2e spec `add, toggle and remove a sub-step` is the test). Diagnosed from the CI trace: the item was `minimised` 20ms after the targeted sub-step swap, before any SSE request. The server renders every item `minimised`; `tracker.js` re-expanded in `htmx:afterSwap`, but htmx's settle step then re-applies the new element's `class` attribute. Re-expansion now runs in `htmx:afterSettle`. CLAUDE.md's "afterSwap re-expands them" is out of date; Phase 10 rewrites that section. A brief collapsed flash remains until Phase 8's morph swaps.
- gosec G120 does not recognise `r.Body = http.MaxBytesReader(...)`, so the triage handler's `ParseMultipartForm` carries a `nolint` noting the cap.
- Search shortcut swallowed after Escape (fixed, test-first: `search.spec.ts` now asserts the input loses focus). `closeSearch()` hid the overlay but left focus on the hidden `#search-input`, so `isInputFocused()` ignored the next `/` until Chrome's focus fixup ran; intermittent in CI for that reason. `closeSearch()` now blurs the input and returns focus to the element focused before search opened. Phase 8's native `<dialog>` handles this itself.
- Open redirects (fixed, test-first in `test/redirect_test.go`): `tracker.redirectBack` used the `Referer` path unchecked, so a path like `//evil.example/x` became a protocol-relative redirect. The login `next` check rejected `//` but accepted `/\evil.example` and `/<tab>/evil.example`, which browsers normalise to `//`. Both now use `httputil.IsLocalPath`, which rejects `//`, any backslash and any control character. Impact was low: the tracker redirect needs a POST that `SameSite=Lax` blocks cross-site, and login needs a victim to follow a crafted link.
- Git operations (branches, commits) need the owner's explicit approval.
### Phase 2 notes

- Explicit auth mode: `config.CheckAuthMode` runs in `NewRouter` once `HasUsers` is known. `DASHBOARD_AUTH` accepts only empty, `enabled` or `disabled`. The noauth route golden now builds with `DASHBOARD_AUTH=disabled` and `ADDR=127.0.0.1:0`; its contents are unchanged.
- Client IP: `httputil.ClientIP` uses the peer address (port stripped) unless the peer is in `DASHBOARD_TRUSTED_PROXIES` (CIDRs or bare IPs), then the rightmost `X-Forwarded-For` entry. `X-Real-IP` is never read. Removing `middleware.RealIP` would otherwise have broken login rate limiting outright: `RemoteAddr` includes the source port, so every attempt landed in a fresh bucket.
- Login limiter and account delay are separate instances of one small LRU type (evicts least recently seen, no bulk reset). The account delay keeps real accounts (64) and unknown emails (4096) in separate LRUs so an email flood cannot evict a real account; both follow the same schedule, so the delay does not reveal which accounts exist. IPv6 clients are rate limited per /64. Account delay: three free failures, then 1s, 2s, 4s, capped at 8s, forgotten after 15 quiet minutes, cleared on success. Two tests sleep through real delays, adding about 10s to the race suite.
- Unknown emails are compared against a real cost-10 dummy hash (a malformed hash would return instantly and defeat the point). Login logs carry the IP and, on success, the user ID; never the submitted email.
- API: not mounted (error logged) unless `DASHBOARD_API_TOKEN` is at least 32 characters. Failed bearer attempts: 10 per minute per client IP, then 429; a valid token always passes. Same rule in the MCP sidecar.
- MCP: `MCP_TOKEN` (inbound) and `DASHBOARD_API_TOKEN` (outbound) must both be at least 32 characters and differ. Destructive tools are unregistered unless `MCP_ALLOW_DESTRUCTIVE=true`. The smoke tests were written without being run (uv blocked locally); CI is the first run.
- Cross-origin protection: browser routes are registered through `root.With(http.NewCrossOriginProtection().Handler)`, so `chi.Walk` still sees them and the route goldens are unchanged; `/api/v1` is on `root`. Security headers apply to every response, API included. `htmx.config.allowEval=false` via a meta tag; no template used eval features.
- Shutdown: SIGTERM now calls `http.Server.Shutdown` (8s budget, inside Docker's 10s). The broker closes via `context.AfterFunc(shutdownCtx, broker.Close)` rather than `RegisterOnShutdown`, because `main` does not hold the broker; the effect is the same. `e2e/run.sh` fails the run if the server ignores SIGTERM. Server timeouts: read header 10s, read and write 60s (room for a 10 MB upload), idle 120s. SSE clears both deadlines via `http.ResponseController` and pings every 30s. Session idle timeout: 7 days.
- The SSE stream tests need a real loopback listener, which the local sandbox forbids; they skip there and run in CI.
- Containers: both images run as UID 10001 by default; compose sets `user` from `DASHBOARD_UID`/`DASHBOARD_GID` (fliptronic already runs as `1007:1007`, the data owner). Compose runs both read-only with `/tmp` as tmpfs, `cap_drop: [ALL]` and `no-new-privileges`, binds ports to `127.0.0.1`, and uses a fixed subnet `172.30.81.0/24` whose gateway `172.30.81.1` is `DASHBOARD_TRUSTED_PROXIES`. Base images pinned by digest (Dependabot `docker` ecosystem tracks them). Images now build, without pushing, on pull requests.
- fliptronic facts (read via the homelab repo's `homelab-ssh`): the dashboard published `0.0.0.0:8081` and `[::]:8081`, which Docker routes past ufw; the MCP sidecar is deployed on `127.0.0.1:9100` behind Caddy's `/mcp*`; uploads already live inside the `data` mount; `DASHBOARD_SECURE_COOKIES` defaults to `false` there; Caddy's `security_headers` snippet sets HSTS, nosniff, X-Frame-Options and Referrer-Policy but no CSP. No chown is needed: the only root-owned files (`data/personal.md`, legacy `ideas/`) are not written in auth mode.
- Compose smoke run (local, real data copy): passed after two findings. (1) With `-wal`/`-shm` present SQLite opened a non-writable database read-only; the server started and failed on the first save. `db.Open` now does a no-op write after migrations and refuses to start with a message naming the owner mismatch (test-first, `TestOpenRefusesReadOnlyDatabase`). (2) The owner's local Docker runtime shares `~` read-only into its VM (`touch` gave "Read-only file system"), so the smoke run used Docker volumes; production bind mounts are native and unaffected.
- Self-review fixes: account-delay eviction (above), IPv6 /64 buckets, IPv4-mapped trusted proxies normalised, `Retry-After` and 429 on rate-limited logins, the DB write probe only blames ownership for read-only errors, a second signal during shutdown kills the process, MCP write tools carry `destructiveHint=False` and MCP tokens must be ASCII (compared as bytes).
- Accepted limits: the failed-bearer limiter cannot slow guessing, because a valid token is never blocked (a deliberate trade-off; the 32-character minimum is the real protection, and the API is unmounted in production). The account delay is a per-request sleep, so parallel requests from many IPs wait concurrently. `sm.IdleTimeout` makes scs rewrite the session row and send Set-Cookie on every authenticated request; negligible for one user, watch it in Phase 3. `main` does not wait for background goroutines at shutdown; Phase 3 atomic writes make an interrupted write harmless.
- Owner decision (2026-10-04): MCP is not used. The fliptronic deploy removes the `dashboard-mcp` service and Caddy's `/mcp*` route, and drops `DASHBOARD_API_TOKEN` so `/api/v1` is not mounted. Both stay in the repo and can be re-enabled. The footer MCP badge will then report MCP as unavailable; revisit it in Phase 9 (design) or Phase 10.
- Deployed to fliptronic 2026-10-04 (image from merge `909b7cb`): compose and Caddyfile backed up (`.bak.1791110956`), MCP service and `/mcp*` route removed, API token dropped, hardening applied. Verified through Caddy: `/login` 200, `/api/v1/*` and `/mcp/*` 404, CSP/HSTS present, cross-site login POST 403, `172.16.61.9:8081` no longer reachable, `verify-stack.sh` all green (a first run failed two checks only because Caddy was still starting). Homelab docs updated (uncommitted in that repo for the owner).
- Deploy actions (superseded by the owner decision above): with MCP and the API removed on fliptronic, no token work was needed; `DASHBOARD_TRUSTED_PROXIES=172.30.81.1` is set in the production compose.

### Phase 3 notes

- Backup: fliptronic's cron `backup.sh` archived `data/tracker.md` (gone), the legacy `ideas/` dir and the compose file, with errors sent to `/dev/null`: 804-byte archives, no user data, no database, for months. A manual full snapshot was taken first (`backups/manual-full-20261004-105705.tar.gz`), then `scripts/backup.sh` replaced it (old script kept as `backup.sh.bak.<epoch>`; cron line now `cd`s into the dashboard dir). The script tars `data/` and `users/` and adds a DB snapshot from SQLite's online backup API (Python `sqlite3`, present on the host) with an integrity check, excluding the live `-wal`/`-shm`. `make backup` wraps it; README documents restore. Tests run the script against temp dirs.

- SQLite pragmas moved into the DSN. Before, `foreign_keys` and `synchronous` reached only the connection that ran the `PRAGMA`, so `ON DELETE CASCADE` depended on which pooled connection a query used.
- Static assets: `static "name"` emits `/static/name?v=<sha256 prefix>`; only a request with the current hash gets `immutable`, everything else `no-cache`. A test forbids hard-coded `/static/` URLs in templates. Uploads keep `immutable` (random names, never rewritten).
- `MoveToList` now adds to the target before deleting from the source; a failed add used to lose the item (reproduced in `TestMoveToListKeepsItemWhenTargetWriteFails`).
- Atomic writes: `internal/atomicfile` (temp `.<name>.atomic-*.tmp` at 0600, write, fsync, chmod to the target's mode, rename, fsync dir). Used by the tracker, ideas and house writers, skeleton creation in config and the registry, and `migrate-data`. `config.Load` removes stale temp files first. Fault-injection tests cover failure after write, at fsync and at rename; the concurrent-reader test is gated by `INTEGRATION=1`, which the CI test job sets.
- SQLite mirror dropped: `Summary()` (only ever called by tests) counts from the cache; `tracker.NewService` lost its store parameter; `store.go` deleted with the registry store tests. `tracker_items` stays in the schema.
- Change events: each service embeds `changes.Recorder` and writes through one helper (render, atomic write, record SHA-256, publish `file-changed` with its category). The registry gets the publisher via `SetPublisher` and applies it to shared services and to each user's services as they are created. `ResyncIfChanged` hashes the file and re-parses only on a difference; the watcher runs callbacks first and broadcasts only when one reports a real change (or when no callback exists). Verified end to end through a real watcher and broker: one web or API mutation gives exactly one event and zero re-parses; an external edit gives one of each. The event name stays `file-changed` until Phase 7 renames it.
- Round-trip fixtures (`test/roundtrip_test.go`, `test/testdata/roundtrip/`): byte-identical round trips for personal, family and house-project trackers, ideas and maintenance, covering every inline tag, sub-steps, captions, soft deletes, blank lines and space-indented headings (tab indentation was added afterwards); goldens pin the writers' documented normalisations (tracker drops blank lines and flattens deeper indents, ideas collapse blank-line runs and add a missing status, metadata is reordered). They were committed inside `9d0a87f` by an over-broad `git add`, ahead of the fixes. Two real data-loss bugs found and fixed: an indented `## heading` in a tracker body ended the item and dropped the rest of its body; an indented checklist in an idea body split into new top-level ideas and lost its done state (production `ideas.md` had no indented checkboxes, so nothing re-parses differently). A third, in hand-edited files: tab-indented body lines vanished in ideas and maintenance, and a tab-indented heading ended the item; a tab now counts as one indent level and only unindented `#` lines are headings in all three parsers (no production file has tab-indented lines).
- Self-review fixes: atomic writes follow symlinks; a failed post-rename directory fsync is logged, not returned (the file was replaced); `CleanStale` skips unreadable directories instead of blocking startup; an externally deleted file empties the cache and broadcasts; an indented checkbox before any idea is still an idea; `backup.sh` resolves relative directories, keeps the archive on GNU tar's "file changed" exit 1 and URI-escapes the snapshot path; the DB ping carries the ownership hint. Service events now go through `Broker.Debounced` (500ms per category), restoring the gap the watcher debounce used to give, so the writing tab finishes its own request before the refresh.
- Accepted or deferred: a user whose services are first created by a watcher event does not get that first event broadcast (no tab can be open for them); deleting a user via admin lets the watcher recreate their directory (pre-existing; admin is frozen; backlog); MoveToList's `-<unix>` collision suffix is dead because `AddItem` re-slugifies (pre-existing; Plan 3 stable IDs); `sm.IdleTimeout` session rewrites per request remain (negligible for one user). Phase 3 Playwright verification is the CI run on this branch.
- Speed (owner decision 2026-10-04: keep fsync, judge the work): `BenchmarkMutate200` is 8.93 ms with durable writes (`docs/plans/bench-phase3.txt`), 2.9x slower than the 3.07 ms baseline, because Go's `File.Sync` is `F_FULLFSYNC` on macOS and each write fsyncs the file and its directory. With both fsyncs disabled (measured once, not committed) it is 0.53 ms, 5.8x faster than baseline; allocations fell 51% and bytes 43%. Linux `fsync` is expected to be much cheaper.

### Phase 4 notes

- Contrast: `internal/theme` parses the two token blocks from the embedded `theme.css` (resolving `var()` chains) and computes WCAG contrast. `seasonal.AccentFor(now, tokens)` keeps the seasonal hue and saturation and steps lightness away from the background until the accent reaches 4.5:1 on `--base` and `--mantle` and under `--on-accent`. The layout injects the resulting hex per theme. `NewRouter` checks every day of 2028 at startup, so a token edit that breaks contrast fails at boot rather than on some later date. The test calls the same function with the same parsed tokens. The light fallback `--accent` is now `#214fab` (the January value); `var(--blue)` was 4.04:1 on `--mantle`.
- Light-theme failures went beyond the five the plan measured: red and `--priority-high` (4.46 on `--mantle`), mauve (4.45), `--priority-low` (3.97), `--overlay0` (3.25) and `--subtext0` (4.06) as text, `--base` text on green, teal and medium-priority fills (about 3:1), the fallback accent, and opacity-dimmed done, deleted and footer text. New tokens: `--success-fg`, `--warning-fg`, `--attention-fg` (peach, for the stale age badge), `--danger-fg`, `--tag-fg` (teal), `--priority-medium-fg` and `--on-accent`. In dark mode each one aliases its Catppuccin hue. Filled badges use the `-fg` token as the fill so `--base` text keeps its contrast. `--priority-high` and `--priority-low` already had light-only overrides and were darkened in place.
- Two guards in `test/contrast_test.go` go beyond the plan. `TestThemeRuleTextContrast` checks every `color:` rule in `theme.css` against the rule's own background, or against `--base` and `--mantle` if it sets none, in both themes; it covers about 300 pairs. `TestThemeNoOpacityDimming` forbids `opacity` below 1 except on the drag ghost and the loading pulse, because opacity blends text below its token's contrast. Done rows now dim with `--fg-muted`.
- Id scheme (Phase 8 morph swaps depend on it): tracker rows and house projects `item-{slug}`, ideas `idea-{slug}`, maintenance `maint-{slug}`, plan rows `plan-{list}-{slug}` (list is `todos`, `family` or `house`). Suffixes: `-toggle`, `-detail`, `-up`, `-down`, `-done`, `-drop`, and `-substep-{n}-{toggle|promote|remove}`. Done and deleted tracker rows gained ids. `TestPagesHaveNoRoleButtonAndUniqueIDs` asserts unique ids on every main page. House rows used the bare slug before, so the plan row's `/house#item-{slug}` link never matched; search and the homepage maintenance card now link to `#item-` and `#maint-`.
- Toggles: the plan says plan rows, but tracker, goal and idea headers and house `<tr>`s had the same `role="button"` pattern with checkboxes, badges and forms inside, which axe reports as `nested-interactive` (serious). All of them now use a `<button>` toggle with `aria-expanded` and `aria-controls`; tracker toggles are named by the item title. A row click still toggles (`planItemClick`, `itemHeaderClick`, `houseRowClick`), except on the row's other controls. Clickable badges are `<button>`s. The three plan-row copies became one `plan-row` template (with a new `dict` template func).
- Reorder buttons render on every pointer (CLAUDE.md's "mobile fallback, coarse pointer only" is stale; Phase 10). After a move, focus returns to the pressed button and `#announcer` says "Moved X to position N of M", or "X is already first/last".
- Live regions: `#announcer` (polite) plus the server flash (`role="status"`, or `role="alert"` for `flashErrorKeys`). The per-page "updating…" indicators were also polite live regions, so every SSE refresh was announced; they are now `aria-hidden`. The MCP badge carries its state as sr-only text instead of a live region. `showToast(msg)` sets text via `textContent` and announces through `#announcer`; the toast is not itself a live region. It hides after 6s; Phase 8 adds the undo button, the 10s minimum and pause on hover or focus.
- Target size: global minimums of 24px for `button`, `select`, `summary`, clickable badges and checkboxes, and 44px under `pointer: coarse`. On phones the tracker header's badge buttons are 44px tall, which makes rows taller; check this during the owner's review.
- Theme: the first visit follows `prefers-color-scheme`, which also means Playwright, whose default is light, now runs light unless a spec sets the theme. `color-scheme` is declared per theme block plus a `<meta>`. `html` no longer hard-codes `data-theme="dark"`. The login page gets the same treatment, plus `<main>`, `<h1>` and a keyboard-reachable "Change" button (it was an `<a>` without `href`).
- Go render tests (`test/a11y_render_test.go`) build the real router in no-auth mode with seeded files and assert the markup contract: skip link, landmarks, live regions, plan and tracker row ids and names, one flash, and an accessible name on every form field.
- The Phase 1 homepage suppression bug is fixed. `tracker.js`'s `beforeSwap` guard now also holds `.homepage-page` refreshes while a plan item is expanded or dragged, and any SSE refresh moves focus to the new copy of the focused element by id instead of letting it fall to `<body>`. The keyboard reorder test depends on both. The `sse.spec` homepage test is no longer `fixme`. CLAUDE.md's claim about homepage suppression is now true.
- Playwright (`e2e/tests/a11y.spec.ts`, written by a sub-agent against the markup contract above; type-checked with `bunx -p typescript@5 tsc`, not yet run):
  - axe scans with the `wcag2a`/`aa`, `wcag21a`/`aa` and `wcag22aa` tags, failing on serious or critical violations and printing all of them, for every main page in both themes and in these states: item expanded, plan item expanded, select mode, confirm modal, search, shortcut help, mobile nav and toast
  - a 320px reflow check, a reduced-motion check, the skip link, and target-size checks (24px, and 44px with `hasTouch`)
  - a keyboard-only plan flow: expand, move up with focus kept and the move announced, then complete
  - existing specs moved to the new accessible names

  Things to watch on the first CI run:
  - the strict 44px check covers badges, filter tags and `summary` elements
  - the toast hides after 6s, so a slow scan can miss it
  - the keyboard test tabs up to 400 times, so it slows as the shared plan list grows
- First CI run (PR #7): 70 of 74 passed, and the four failures were real:
  - The idea breadcrumb link was told apart by colour alone (axe `link-in-text-block`). Breadcrumb and "From idea" links are now underlined.
  - The search-hint `<kbd>` keys inherited `--fg-muted` onto `--surface0` (3.68:1 in light). The rule audit can't see inherited colours, so `TestThemeTextBackgroundsSetColour` now requires any rule with a text-bearing background to set its own colour.
  - The homepage overflowed at 320px. Its phone one-column rule sat earlier in `theme.css` than the base grid rules with the same specificity, so phones never got one column. This bug predates Phase 4.
- VoiceOver pass: waived. The owner does not use a screen reader (owner decision 2026-10-05). The automated checks (axe, keyboard, contrast, target size) remain the accessibility gate, here and in Phase 10.
- Deployed to fliptronic 2026-10-04 14:22 UTC after PR #7 merged. A backup was taken first (`backups/dashboard-backup-20261004-141739.tar.gz`). The live `/login` serves the Phase 4 markup. The owner saw little change in dark mode on desktop, which is expected: dark already passed contrast.
- Owner decision (2026-10-05): a visual design uplift goes into this plan as Phase 9, between interaction rework and documentation, so it is done once on the Phase 6 shared components and after Phase 8 settles the interaction patterns. Documentation becomes Phase 10.
- Phase 4 STOP review: the owner tried the deployed app on 2026-10-05 and is moving on. Next is Phase 5.

### Phase 5 notes

- No-auth mode is now a middleware, not a branch: `auth.InjectUser(db, 1)` replaces `RequireAuth` and `RequireAuthAPI` when `DASHBOARD_AUTH=disabled`, reading user 1's row on every request so name and admin come from the database. `prepareUsers` runs `auth.EnsureUser(db, 1, "local@localhost")` in no-auth mode, so the purge loop and `/account` have a row. That row is an admin with the password hash `!`, which matches no password. Start-up and the first-user-is-admin rule count only users who can log in (`auth.LoginUserCount`), so a database first used in no-auth mode starts in auth mode just as an empty one would: it bootstraps an admin from `DASHBOARD_PASSWORD_HASH` or refuses to start (test-first, `TestAuthModeIgnoresLocalPlaceholderUser`; found in self-review, where it started with nobody able to log in). bcrypt rejects `!` without hashing, so a login attempt as `local@localhost` returns faster than for other accounts. It only exists in local databases, so this is accepted.
- Route golden diff: `routes_noauth.golden` gained exactly the 15 `/login`, `/logout`, `/account*` and `/admin/*` lines and is now identical to `routes_auth.golden`. They were kept rather than excluded so that both modes register one route tree. The nav still hides logout and the user link in no-auth mode (`authEnabled` is false).
- Data paths differed as the plan suspected: no-auth read `PERSONAL_PATH`/`IDEAS_PATH`, auth read `USER_DATA_DIR/1/`. `Registry.SetUserPaths(1, ...)` covers service paths and skeleton creation (`EnsureUserDirs` now creates the files at the override paths). The watcher watches the two files as file categories whose callbacks resync user 1's services. `TestNoAuthUsesLegacyPaths` guards this.
- Commentary (test-first, `TestCommentaryScopedToUser`): the web commentary endpoint and idea detail used user 1 for everyone, so user 2 saw user 1's notes. Both now use `auth.UserID`. The API commentary handlers still use user 1, which is correct because the token acts as user 1. As a result, commentary on the shared family and house lists, which the API always writes as user 1, is no longer shown to other users. Moot for a single owner; revisit if multi-user returns.
- One `appServices.toTask` replaces the three closures. `tracker.Service.AddItem` returns the slug it assigned; `APIAddTodo` and the ideas test helper stop recomputing it.
- API: `bearerAuth` puts user 1 in the request context, so the API uses the web handlers' resolvers. Deleted: the separate API ideas handler and its `toTask`, and both sets of tracker and plan API construction. The plan API functions are now `home.Handler` methods. The tracker API functions take a `tracker.ServiceResolver` returning (personal, family).
- `home.Handler` takes a `home.Resolver` returning `home.Lists` instead of the registry, so tests pass a closure over their own services instead of using the deleted `HomePageSingle`/`DigestPageSingle`/`CalendarPageSingle`. `search.ServiceResolver` keeps its 5-tuple for Phase 6.
- `internal/app` is split into `app.go` (start-up: users, services, watcher, sessions, purge), `routes.go` (a `handlers` struct replaces `mountAppRoutes`' 17 positional parameters; `mountBrowserRoutes`, `mountAPIRoutes`, middleware) and `templates.go` (func map and parsing, moved unchanged). `renderHomePage` is split by plan section. `gocyclo -over 15` reports only the six accepted Phase 1 exclusions.
- Behaviour changes in no-auth mode only: it now resyncs house projects at start-up and also watches `USER_DATA_DIR`, as auth mode already did. Both are harmless.
- `tracker.NewHandler` and `ideas.NewHandler` (static services) are now used only by tests. They were kept to avoid churning six test files ahead of Phase 6's handler changes.
- CLAUDE.md still describes `SingleUserPlanHandlers`, "planner dual-mode handlers" and three `ToTaskFunc` closures. Phase 10 rewrites it; until then, these Phase 5 notes are the current description.
- Verification: lint 0 issues; `INTEGRATION=1 go test -race ./...` green; Playwright runs in CI on the PR.
- First CI run (PR #8): the other four jobs passed, including e2e; `test` failed in `TestCommentaryScopedToUser`. The test was flaky, not the fix: `authRouter` created users from a map, so in a random half of runs `two@test.com` got ID 1. Users are now an ordered slice; 50 repeated runs pass.
- First `main` run after merge (`aaea9da`): e2e failed once on axe `color-contrast` for `#confirm-modal-title` (light theme, confirm modal open). It passed on the PR runs. The modal fades in from opacity 0 over 150ms and the scan started as soon as `.visible` was set, so axe measured a blended colour. The final colours pass. `expectNoSeriousViolations` now waits for every finite animation to finish before scanning. The next `main` run (`a704381`) passed.
- Deployed to fliptronic 2026-10-05 00:00 UTC from image `f8143ab`, which is Phase 5 plus Dependabot's alpine 3.21 to 3.24 runtime bump (PR #5). A backup was taken first (`backups/dashboard-backup-20261004-235950.tar.gz`). Verified through Caddy: `/login` 200; `/`, `/todos`, `/account` and `/admin/users` redirect to login with `next`; `/events` 401; `/api/v1/*` 404; CSP and HSTS present; cross-site login POST 403; `verify-stack.sh` all green. Dependabot PRs #2 (`@types/node` 26) and #3 (pip group, including mcp 2.x) were closed, and `dependabot.yml` now ignores major bumps of both.
- From Phase 6 on, work is committed directly to `main` (owner decision 2026-10-05); there are no more phase branches or PRs.

### Phase 6 notes

- Contract (`internal/module`): `Module` (`Manifest`, `Routes`) plus optional `APIRouter`, `Watcher`, `Searcher`, `HomeWidget`. `Manifest` carries the ID, title, nav items, route `Prefixes` (what validation checks; chi cannot report a group's routes) and flash messages. Modules get one `module.Deps` (location, config, service registry, renderer, `DataDir` = the directory of `FAMILY_PATH`, and `Publish(id)`, which sends a debounced `changed:<id>`). `app.modules` is the built-in list and is empty until Phase 7; `app.NewRouterWith(..., app.Options{Modules, Broker})` lets tests add modules and listen to the broker. `NewRouter` is unchanged for `main` and the other tests.
- Validation (`module.NewRegistry`, table-tested in `test/module_registry_test.go`): it rejects malformed or duplicate IDs, overlapping prefixes at a path-segment boundary (`/a` overlaps `/a/b`, not `/ab`), reserved prefixes, core nav paths and `/personal` and `/exploration`, duplicate or reserved keys (`h c d / ? n k`), multi-letter keys, unknown groups and more than 6 primary links. `/commentary` and `/upload` were added to the plan's reserved list, because the core owns them too. Nav is sorted by `Order`; ties keep core first, then registration order.
- Core nav until Phase 7: home, todos, goals, ideas, house and family are primary; digest and calendar sit behind "more". Calendar has a nav link for the first time (before, it was reachable only by `g c`). Digest moved behind "more" because the plan caps primary links at 6; revisit in Phase 9.
- Templates: the embed is now `templates/*.html templates/_components/*.html templates/*/*.html`. Every page, core or module, clones the layout plus `_components/components.html` (`page-header`, `empty-state`, `error-banner`, `item-row`, `quick-add`, `card`, `widget`, `toast`), so Phase 9 can move core pages onto them. Module pages render through `Deps.Render.Page(w, r, manifest, file, data)`, which merges `auth.TemplateData` and resolves `?msg=` through the manifest. `auth.TemplateData` now includes `CurrentPath`, which the layout uses for `aria-current="page"` by path prefix; the JS nav highlighter is gone. Guard tests: no inline `on*` handlers in module or component templates, and only `linkify` returns `template.HTML` from the func map (checked by parsing `templates.go`).
- Nav and shortcuts: links carry `data-shortcut`; `shortcuts.js` builds the `g x` map from them, and the `?` help rows render from the same registry. The "more" disclosure is a button with `aria-expanded`; it closes on outside click or Escape, which returns focus to it. Escape on the open hamburger menu now returns focus to the hamburger. In the open mobile menu, "more" links list inline and the button is hidden. Playwright: `e2e/tests/nav.spec.ts` and an axe state for the open "more" menu (type-checked; first run in CI).
- Watcher: `watcher.Watch(userDataDir, []Spec, broker)` matches exact files only, either a shared `Path` or `USER_DATA_DIR/{id}/{UserFile}`. The prefix matching is gone, so `personal-old.md`, temp files from atomic writes and the legacy nested `ideas/untriaged/*.md` layout no longer trigger anything (table test `TestWatcherClassify`). Core lists still send `file-changed` with their category; module files send `changed:<id>` with the ID as data and no item content. Phase 7 renames the core events. Known limitation: events go to every connected client, with no per-user SSE routing (single user).
- Search: `search.NewHandler([]module.Searcher)` queries searchers one at a time, so no two services' locks are held together. `search.ServiceResolver` is gone. The lists that are not modules yet use adapters (`search.Tracker`, `search.Maintenance`, `search.Ideas`) in the old order, which Phase 7 moves into the modules.
- Home widgets: `home.Handler.SetWidgets(reg.Widgets)`. Each widget is `module.WidgetData` (title, count, up to 5 items, link, empty text, severity), rendered by the `widget` partial as `<section aria-labelledby>` with an `<h2>`, below the plan section. Deviation: widgets also render under the empty-homepage message, which has no plan section; otherwise a module's data would be invisible until the user adds a task. The contract test found this.
- Fixture (`internal/module/moduletest`, template `web/templates/moduletest/page.html`, shipped in the binary but never registered in production): a line list in `DataDir/moduletest.md` with a page, quick-add, nav item under "more" with key `x`, search, widget, API route and watched file. `TestModuleContract` covers nav placement, `aria-current`, the help row, shared partials, the login redirect, the flash, search, widget placement, API token protection, `changed:moduletest` on its own write and on an external edit, and the error banner (the file replaced by a directory).
- Verification: the route golden is unchanged; lint 0 issues; `INTEGRATION=1 go test -race ./...` green; gocyclo only lists the six accepted exclusions (`NewRegistry` was split into a validator to get there).
- Self-review (independent reviewer, no production defects found) led to two changes, each test-first:
  - With digest behind "more", `/digest` and `/plan/calendar` lost their visible current-section marker, because their links sit in the closed menu. The "more" button now takes the `nav-active` style when one of its links is current (`TestMoreButtonMarksCurrentSection`).
  - A module could point a nav link at a core page, or register an API path that silently replaced a core handler (chi lets a later route win). Nav paths must now sit under the module's own prefixes, and `APIRoutes` mount under `/api/v1/<module-id>`, which closes off API clashes by construction.
- CI on `main` (`4a54f43`) passed in full, including the new Playwright nav spec and the "more" menu axe state. Deployed to fliptronic 2026-10-05 01:58 UTC (backup `dashboard-backup-20261005-015758.tar.gz`). Verified through Caddy: auth redirects, `/events` 401, `/moduletest` 404 (fixture not registered in production), cross-site login POST 403, `verify-stack.sh` green.
- Dependabot opened PR #9, the pip group with mcp 2.x again, despite the `update-types` ignore. The ignores now name version ranges (`mcp >=2`, `@types/node >=25`).

- Follow-ups:
  - tighten CSP `script-src` after moving inline handlers
  - drop the `tracker_items` table in a later migration
  - `Plannable` capability (Plan 2)
  - SQLite migration hook (Plan 2)
  - stable IDs and slug collisions (Plan 3)
  - MCP uv, Python 3.14 and ty (Plan 3)
  - shortcut opt-out and calendar keyboard reschedule (backlog)
  - per-user SSE routing if multi-user returns
  - admin user deletion: the watcher recreates the deleted user's directory (admin frozen)
  - MoveToList slug collisions (dead `-<unix>` suffix) until Plan 3 stable IDs
  - remove the test-only static `tracker.NewHandler`/`ideas.NewHandler` constructors (Phase 6)

### Phase 7 notes

- Layout: module adapters live in `internal/modules/` (`tasks` holds both `todos` and `family`, plus `ideas` and `house`), registered in `app.modules`. They cannot live in `internal/ideas` or `internal/house`: `module` imports `services`, which imports `ideas` and `tracker`, so an `ideas` package importing `module` would be a cycle. The adapters wrap the existing libraries (`tracker`, `ideas`, `house`), which keep their handlers, services and parsers. Templates: `web/templates/tracker/` (tracker and goals pages, shared by todos and family through the new `Manifest.Templates`), `ideas/` and `house/`. Registration order (todos, family, house, ideas) keeps the old search order and sets widget order; nav order still comes from each item's `Order`, so the nav is unchanged.
- Contract changes: `Manifest.Templates` (template directory, defaults to the ID). `HomeWidget` returns `[]WidgetData`, so todos can show a todos card and a goals card; the registry IDs them `<module>` or `<module>-<id>`. `WidgetData` gained `CountLabel` and `Note`, and `WidgetItem` gained `Priority`, `Severity` and `Progress`, which is what the old cards used. `Deps` gained `Commentary`. Handlers take `httputil.PageLookup` instead of a template map, because module pages are parsed after modules are built; `Renderer.Lookup(dir)` resolves at request time.
- Route checks: module browser routes are registered on a scratch router, checked against the manifest's `Prefixes` (at a path-segment boundary), then copied onto the real router with their middleware (`mountModule`). API routes are written relative to `/api/v1` and must sit under `/<module-id>`. Phase 6 mounted them with `r.Route`, but `chi.Walk` then reports `/api/v1/todos/` with a trailing slash, which would have changed the route golden; the copy keeps the goldens byte-identical and fails start-up for a stray route (`TestModuleRoutesMustStayUnderTheirPrefixes`).
- Events: services publish `changed:<module-id>` through one debounced publisher shared with `Deps.Publish`; the registry maps personal to `todos`, and house projects and maintenance to `house`. The maintenance service moved into `services.Registry` so the core purge loop still reaches it. `file-changed` and `Broker.Debounced` are gone. Each page listens to its own module (`tracker.html` uses `changed:{{.ListName}}`); the homepage listens to the widget modules plus the four modules whose services the core reads itself (planner and tag summary). `tracker.js` re-triggers the element's own first `sse:` event after the completion animation. New tests: `TestModuleFilesSendTheirOwnEvent`, `TestModuleWritesSendTheirOwnEvent`, `TestPagesRefreshOnTheirModulesEvents`; Playwright `sse.spec.ts` checks that an `ideas.md` edit refreshes `/ideas` and `/` but not `/todos` (the context fixture now installs the activity tracker, so extra pages get it).
- Homepage: the todos, family, goals, maintenance and ideas cards are module widgets (`TestHomepageWidgetsComeFromModules`). Visible differences: widgets sit in the auto-fit `.homepage-widgets` grid instead of the two-column grid, with the tag card last; goals and overdue maintenance cap at 5 rows plus "+N more"; goal titles link to their row. The page counts as empty when nothing can be planned and no widget shows; before, overdue maintenance or house projects alone also showed the empty message. Tracker links from widgets and search now use `#item-<slug>`, like the planner's "open in list" links. The old `#<slug>` links worked too (`tracker.js` handles both), so this is consistency, not a fix, although the commit describing the failing tests calls it one.
- Inline handlers: the five migrated templates use a delegated dispatcher in `tracker.js` (`data-action`, `data-stop-click`, `data-confirm`, `data-submit`, `data-autosubmit`), as the Phase 6 contract requires of module templates. `house.html`'s inline script is now `web/static/house.js`. One bug was found and fixed before commit: `house.js` runs before `tracker.js` on first load, so it now creates the shared `clickActions` table. Known change: clicks on controls that used to `stopPropagation` (badges, bulk checkboxes) now also close an open nav "more" menu. Core templates (`homepage.html`, `layout.html`, calendar, account, login) keep their inline handlers until the CSP follow-up.
- Core pages stay outside modules: home and planner, calendar, digest, search, auth, account, admin, uploads and commentary. Candidates for capabilities later:
  - `Plannable` (Plan 2): items with planned dates, set/clear/complete/reorder. It would replace `home.Lists`' three tracker services, `listService` and `coreHomeEvents`.
  - `DigestSource` and a tag source: the digest and the homepage tag card read ideas through `home.Lists`.
  - `Purger`: the purge loop names the shared and per-user services.
  - Commentary list names: `httputil.validate` allows `todos`, `family`, `house`, `ideas`; a manifest field could declare them.
- Requirement 4 evidence. Files outside `internal/modules/<x>/` and `web/templates/<x>/` touched in `66b683d` and `3acc5e9`:
  - Registration: the four lines in `app.modules`.
  - Framework, once for all modules: `internal/module/*` (contract, registry, renderer, fixture), `internal/app/*` (route checks, template parsing, deleting the core feature wiring), `internal/sse/broker.go`, `internal/httputil/pages.go`, `_components/components.html` and the widget classes in `theme.css`.
  - Deleting the replaced wiring: `home/handler.go` and `homepage.html` (cards and their data), core nav, the core watcher specs, search adapters, the `parseTemplates` page list, `mountTrackerRoutes`. The `shortcuts.js` switch and the watcher prefixes had already gone in Phase 6.
  - Each feature's existing library: `tracker/handler.go` (`PageLookup`, `Mount`), `ideas/handler.go` and `house/handler.go` (`PageLookup`).
  - `services/registry.go`: maintenance moved in, and module IDs for the publisher. Justified: the registry is the core's store for the per-user and shared markdown lists the planner and purge also read. A new module that owns its storage (exercise, Plan 2) does not touch it.
  - `web/static/house.js` and `tracker.js`: static assets are one flat embedded directory. A new module with its own JS would add a file there; a per-module static directory is a follow-up if exercise needs one. `tracker.js` also hard-codes `.tracker-page` and `.ideas-page` for SSE suppression and select mode, which Phase 8's morph swaps remove.
  - So a new module needs only its package, its template directory and one registration line, unless it needs page JS (a file in `web/static/`), plannable items (Plan 2) or commentary.
- Self-review (independent reviewer): no user-facing regressions found. Fixed in `3acc5e9`: `search.spec.ts` still expected `/todos#renew-passport`; the homepage trigger only covered widget modules, so a list the planner reads would stop refreshing if its module dropped its widget; core pages were detected by matching the text `{{define "content"}}`, so a differently spaced define would have skipped a page until request time (now parsed and checked with `Lookup`).
- Verification: lint 0 issues; `INTEGRATION=1 go test -race ./...` green; route goldens unchanged; gocyclo only lists the six accepted exclusions; e2e type-checks. Playwright runs in CI on push, and is the first browser run of the handler conversion.
- CI on `main` (`06214b1`) passed in full, including e2e, so the handler conversion and the new `ideas.md` refresh test pass in a browser. Deployed to fliptronic 2026-10-05 03:56 UTC (backup `dashboard-backup-20261005-035538.tar.gz`, image revision `06214b1`). Verified through Caddy: `/login` 200; `/`, `/todos`, `/goals`, `/ideas`, `/house` and `/family` redirect to login with `next`; `/events` 401; `/api/v1/todos` 404; `/static/house.js` 200; cross-site login POST 403; CSP and HSTS present; `verify-stack.sh` green.
- Phase 7 STOP review: the owner accepted the deployed app, including the homepage widget layout, on 2026-10-05. Next is Phase 8.
- Follow-ups: per-module static assets if a module needs JS; the capabilities above; the remaining inline handlers in core templates; remove the test-only static `tracker.NewHandler`/`ideas.NewHandler` (still open).

### Phase 8 notes

- **Stable SSE connection.** `<body hx-ext="sse,morph" sse-connect="/events">` in the layout; the live containers no longer carry `hx-ext` or `sse-connect`, so an event no longer reconnects the stream. Each page has one live container marked `data-live`, refreshed with `hx-get` on its modules' `sse:changed:<id>` events, `hx-select="[data-live]"`, `hx-target="this"`, `hx-swap="morph"` and `hx-push-url="false"` (`TestLiveContainersMorphUnderOneConnection`).
- **Morph and client state.** idiomorph 0.8 (vendored in Phase 1) is now loaded. One global `beforeNodeMorphed` callback in `live.js` keeps the state the client owns, declared in markup: `data-keep-class` (`minimised`, `select-mode`, `house-detail-hidden`, the note popover's `open`, the filter buttons' `active`) and `data-keep-attr` (`aria-expanded`, `open` on `<details>`, `style` on filtered rows). Text fields the user has changed and changed checkboxes keep their values; a form's own successful submission resets it first, so the server's values win there. `ignoreActiveValue` keeps the focused field. Chevrons are CSS (`::before`, driven by `aria-expanded`), so no text node needs keeping. Plan rows' `draggable` is recomputed after each settle (minimised and not done).
- **Flags removed** (`TestNoRefreshSuppressionFlags`): `planDetailExpanded`, `trackerExpandedItems`, `planDragInProgress`, the `htmx:beforeSwap` discard logic, `userIsEditing`, the focus-by-id restore and the 400ms re-trigger. Refreshes that must wait go through `liveRefresh.hold/release` in `live.js`: a drag holds until it ends (and, for a drop, until the reorder POST finishes, or the refresh could fetch the old order), and the confirm dialog holds while open. Held refreshes are queued, one per container, and run on release.
- **Fragment responses: one middleware instead of per-handler fragments** (deviation). `internal/app/fragment.go` wraps the browser routes after cross-origin protection. For a POST with `HX-Request`, it buffers the handler's response; a 303 to a local path is replayed as an internal GET through the root router (fresh chi route context, same session cookie, no `Accept-Encoding`), and that page is returned with `HX-Trigger`, `Vary: HX-Request`, `Cache-Control: no-store` and `Dash-Revisions`. htmx selects and morphs the live container. Handlers are unchanged and plain posts keep their 303. Errors and non-redirects pass through. Rendering the whole page costs bytes, not requests; a `content`-only render is possible later if it matters. Mutating forms carry `hx-boost="true"` and inherit target, select and swap from the container; `scrollIntoViewOnBoost` is off. Covered: plan complete, clear, set and clear-carried; tracker complete, uncomplete, plan-for-today, priority, tags (via edit), edit, sub-steps, move, trash, restore and purge; goals progress; idea triage, convert, edit, trash, restore and purge; maintenance log, edit and trash; project status, edit and trash; the quick-add forms. Bulk actions stay plain posts: they end select mode, which a full render does naturally.
- `HX-Trigger` is `{"dash:flash":{"key","message","error","undo"}}`, built with `json.Marshal`. The message is the static flash text the replayed page rendered (read from the layout's `#flash` element), never an item title. `HX-Request` only selects the response format.
- A lapsed session: `RequireAuth`'s redirect to `/login` is not replayed (the login page has no live container and the morph would blank the list). The middleware answers `HX-Redirect: /login?expired=1&next=<current page>`, using `HX-Current-URL` so the user returns to the page rather than the POST URL (`TestFragmentReplayKeepsTheSession`, written for this and failing first).
- **No echo refresh.** The debounced publisher bumps a per-module revision on every write and sends `changed:<id>` with data `"<id> <revision>"`; `Broker.Revisions` reports the current values. The fragment response's `Dash-Revisions` header tells the writing tab which revisions it already has, and `live.js` skips SSE events at or below them in `htmx:confirm`. Watcher events for external edits carry no revision and always refresh. So one mutation is one request in the writing tab, and every other tab still refreshes.
- Planner XHR: `planner.js` now sends `X-Requested-With` rather than a fake `HX-Request`, and `SetPlanned` returns 204 for it like `ReorderPlanned`, so the calendar drag no longer follows a redirect to a homepage it throws away before reloading (`TestPlannerXHRGetsNoContent`).
- **Focus after change.** Rows carry `data-row` (with an id) and `data-row-focus` on the control to focus; lists carry `data-row-list` (with an id) and `data-row-heading`. A list is the row's nearest list ancestor, so no markup was wrapped. Before a form request, `live.js` records the row, its list and the rows after and before it. After the response settles, if focus was lost (the focused element is gone or hidden): the row's control if the row is still in its list, else the next row, else the previous, else the list heading, else the page `<h1>` (now `id="page-heading" tabindex="-1"`). Ideas sections and the Done and Recently Deleted `<details>` are lists with their own headings. House detail rows name their row with `data-row-of`. Goals rows have no toggle, so their title (`tabindex="-1"`) is the focus target. `TestRowsAndListsHaveIDs` guards the ids.
- **Undo instead of confirm.** Tracker, goal, idea, maintenance and project trash have no confirm; their handlers call `httputil.OfferUndo(w, restorePath)`, which the middleware puts in the trigger. The toast (`announce.js`) gains an undo button and a dismiss button, stays at least 10s, pauses while hovered or focused, and announces "Press U to undo"; `u` undoes from anywhere outside a text field. Undo posts to the restore route with `htmx.ajax` into the live container. Purge, user deletion, "move to <other list>" and bulk trash keep the confirm. Bulk trash keeps it because there is no bulk restore route; its text no longer claims the soft delete cannot be undone. The confirm dialog only shows the red "cannot be undone" warning for permanent actions now.
- **Native dialogs.** Confirm, search and shortcut help are `<dialog>`s opened with `showModal()`, closing on Escape and on a backdrop click (the dialog box is exactly the content, so a click on the dialog element came from the backdrop). The focus trap in `dialog.js` is gone. `shortcuts.js`'s Escape chain no longer handles dialogs: while any dialog is open, page shortcuts are ignored except Ctrl+K closing search. htmx forms with `data-confirm` ask through `htmx:confirm` and proceed with `issueRequest(true)`; plain forms still use the submit listener. Search is a combobox (`role="combobox"`, `aria-controls`, `aria-expanded`, `aria-activedescendant`) over a `role="listbox"` of `role="option"` links with ids; the result template lost its inline `onclick`. The help dialog gained a Close button and the `u` row.
- **Submission feedback** (deviation). Buttons are disabled and the row gets `aria-busy` (with an accent bar) from `htmx:beforeRequest` to `htmx:afterRequest`, in `live.js`, for every htmx form request. `hx-disabled-elt` was not inherited from `<body>`, because the live container's own refresh request would then disable its first button and drop focus on every SSE refresh. Failed requests show the server's short error text (or a generic message) as an error toast, and undo the row's completion or triage animation; network failures say nothing was changed. Idea triage is an htmx form with a 300ms fade, so it no longer reloads the page and its errors surface.
- Mobile: the item actions menu still starts collapsed on phones; swaps keep its state, and only rows new to the page are collapsed (idiomorph's `afterNodeAdded`, relayed as a `live:added` event).
- Known limits: rows are matched by slug ids, so renaming an item loses its expanded state until Plan 3's stable IDs (as the plan's risks say). An unsubmitted change to a `<select>` resets on refresh.
- Self-review (two independent reviewers; the first stalled without reporting). My own auth-mode test first found the lapsed-session blanking above. The second reviewer found, and these are fixed: select mode lost its bulk bar, count and toggle label on any refresh (now kept by `data-keep-class` plus an after-settle resync; Playwright `select mode, ticked rows and the bulk bar survive an external edit`); saving an edit with Enter left the focused field showing its old value, which a later save would write back (the field is blurred before the form resets and refocused after the swap); after a restart revisions count from zero again, so a long-open tab would skip real changes (seen revisions are cleared on every SSE open); a refresh queued behind the confirm dialog could race the confirmed POST (refreshes now also wait while any form request is in flight); pass-through redirects lost their `Location` (now dropped only when replaying); a toast paused by a click in Safari never auto-hid again (`hideToast` clears the pause).
- The first reviewer's report arrived after the commit; fixed in the follow-up commit:
  - A failed request reported revisions, so a tab whose POST failed could skip another tab's change. `Dash-Revisions` is now sent only with a page that rendered (read before the replay), and recorded only for successful requests (`TestFragmentResponsesLeavePlainPostsAndErrors`, failing first).
  - A morph cleared text typed into an unfocused field that has no `value` attribute (the add-task title, "Add a step"), because idiomorph empties such fields and only the focused one was protected. Fields the user types in are flagged on `input`, and a `beforeAttributeUpdated` callback refuses value updates to them; a form reset clears the flags (Playwright `text typed in a field without focus survives another row's action`).
  - SSE listeners piled up: htmx re-initialises an element when any attribute changes (a morph changes the container's classes), and the SSE extension then adds another EventSource listener without removing the old one, so each event refreshed once more per swap. The SSE trigger moved to a `live-refresh` child (`_components`) whose attributes never change; the container keeps target, select, swap and push for its forms to inherit.
  - `upload.js` state (unsaved images, captions) was lost on any refresh. Elements marked `data-morph-skip` are never morphed or removed (the upload area, the commentary slot), `data-client-*` attributes survive, and scripts mark the values they write with `liveRefresh.keepValue`. When the form's own save is accepted, `live.js` fires `live:form-accepted` and the area is rebuilt from the server's images. The commentary slot keeps its loaded content.
  - Forms were cleared even when the replayed flash was an error (an invalid cadence lost the typed title); they are now left as typed.
  - Disabling the focused submit button dropped focus to `<body>`; busy buttons are now `aria-disabled`, and a second submit of a form in flight is refused in `htmx:confirm`.
  - Already fixed in the first commit: select mode and the dropped `Location`.
- First CI run of Phase 8 (`a1d8b77`): 87 of 92 Playwright tests passed, including every axe state. The five failures:
  - A real bug from the review-fix commit: idiomorph pairs a new element with any old element of the same tag that has no id. The `data-morph-skip` nodes (commentary slot, upload area) had no ids, so a newly rendered `<div>` (the first `.tracker-item-body` after an edit, the first `.substep-list`) was paired with the commentary slot, which refused the morph: the new content never appeared and the nodes in between were removed. Skip nodes now carry ids (`item-<slug>-commentary`, `upload-area-<n>`), so they only ever match themselves; `TestMorphSkipElementsHaveIDs` (failing first) guards it. This failed `tracker actions update in place` (edit) and `add, toggle and remove a sub-step`.
  - Two test assumptions: a row keeps its expanded state when a morph moves it between lists (trash then restore, triage into another section), so the helper's toggle click collapsed rows that were already open. `expandTrackerItem` now clicks only when the row is closed. Keeping the state across moves is the intended behaviour.
  - The search test's `getByRole('option')` matched an `<option>` in the page's priority `<select>`; it is scoped to the results listbox.
- Second CI run (`2373b6e`): 91 of 92 passed. The failure was real: parking the only idea in Dropped moved its card to Parked, and idiomorph kept the focused button alive inside the moved card, so focus was never "lost" and stayed on what was now a different action. Focus after change now keys on the row leaving its list: focus that is lost or still inside the moved row goes to the next row or the heading.
- Third CI run (`5edbe5a`): 91 of 92 again. Limiting the refocus to text fields left the clicked button focused during the morph, and idiomorph, which avoids pairing past the focused element and its parents, then built the idea card moved to Dropped without its "park" button. Any focused control in the submitted form is blurred again before the swap; the moved-row rule from the second run stays, so a button refocused inside a moved row still sends focus on to the next row or the heading.
- Fourth CI run (`87d54e3`): all jobs green, all 92 Playwright tests pass.
- Deployed to fliptronic 2026-10-05 10:05 UTC (backup `dashboard-backup-20261005-100425.tar.gz`, image revision `87d54e3`). Verified through Caddy: `/login` 200; `/`, `/todos`, `/ideas` and `/house` redirect to login with `next`; `/events` 401; `/api/v1/todos` 404; `/static/live.js` and `/static/idiomorph-ext.min.js` 200; cross-site login POST 403; CSP and HSTS present; `verify-stack.sh` green.
- Owner decision (2026-10-05): deploy whenever `main`'s CI is green, then record it, without waiting to be asked. Next is Phase 9 (design uplift), which starts with the direction decision and STOP.
- Verification so far: lint 0 issues; `INTEGRATION=1 go test -race ./...` green; e2e type-checks. Playwright (written against the markup, not yet run; CI is the first browser run) covers: an external edit while a `/todos` item or a homepage plan row is expanded appears without collapsing it; one mutation makes one POST and no GET; each converted action in place without a reload; the undo toast's timing, hover pause, button and `u`; select mode across a refresh; keyboard focus after complete (next row, previous row, emptied list to its heading); the search combobox; a failed triage showing an error toast; axe on the undo toast and the native confirm dialog.

### Phase 9 notes

- **Direction (owner decision 2026-10-05).** Reached through a five-round questionnaire, a Design Reviewer critique of the brief and a static three-way comparison page. The owner liked all three directions and chose to ship all three as user-selectable styles.
  - Brief: drop the terminal look and Catppuccin, keeping only the `dash>_` wordmark in monospace. Calm and warm, after Bear, Craft, Things 3 and Notion. Warm paper palette; light and dark used equally. System sans for UI and titles, monospace for meta (dates, counts, `kbd`, code). Comfortable density: title line plus one quiet meta line. Round tick button in the collapsed row (the in-row complete button already existed, so this is a restyle, not a behaviour change). Single-column journal homepage. Quiet top bar, hamburger on phones, "more" kept. Shortcuts keep working; hints only in the `?` dialog. Gentle motion (120-200ms). Footer keeps the build string; the MCP badge goes. Must keep: the theme toggle and rows starting collapsed.
  - Shared foundations, from the review: type scale 13/15/16/18/22/28 (from 17 sizes); spacing 4/8/12/16/24/32/48; one `.btn` base; one progress style. Meta line: "high" as a word, age, sub-steps, at most two tags as non-interactive spans then "+N" (tags as 44px buttons were why phone rows grew). Priority is a 3px left edge (high and medium; low has none) plus sr-only text. Carried-over plan items lose their dotted peach edge for a "from Tue" label. `--accent` becomes fixed per style and theme (links, focus ring, primary button); the seasonal hue moves to a decoration-only `--seasonal` (wordmark caret, plan progress), checked at 3:1 non-text. Widths: home 680px, lists 760px, calendar and house 1120px.
  - Styles: **Cards** (default; white rounded cards on cream, ink-blue accent), **Paper** (no cards, hairline dividers, terracotta) and **Document** (borderless, uppercase section labels, inline meta on desktop, sage). Same markup; styles differ only in tokens and a small set of rules. Selected with a control in the nav next to the theme toggle, stored per device in `localStorage` like the theme, applied before first paint via `data-style` on `<html>`.
  - Contrast tests cover all six style and theme combinations; axe and screenshots run in all three styles.
- **Baseline screenshots.** `e2e/tests/screenshots.spec.ts` captures every main page in both themes at 1280px and 375px, plus item expanded, select mode, confirm dialog and toast. CI uploads `e2e/screenshots` as the `screenshots` artefact on every run. The commit that adds it changes no CSS, so its CI run is the "before" set.
- **Implementation (first pass).**
  - `theme.css` is one file in `@layer reset, tokens, base, components, pages, utilities`: 2,735 lines and 63 KB before, 2,050 lines and 75 KB after. Bytes went up because of the layer indentation and four extra token blocks. One file was kept so the contrast tests and the startup check read a single source.
  - Tokens: six colour blocks, with names now semantic (`--bg`, `--surface`, `--surface-2`, `--line`, `--ring`, `--text`, `--text-muted`, `--accent`, `--on-accent`, `--accent-soft`, `--success`/`--warning`/`--danger` with `-soft` fills, `--attention`, `--edge-high`, `--edge-medium`, `--pill`, `--seasonal`). Plus type, spacing, radius, motion and width tokens, and per-style structure tokens (`--card-*`, `--row-divider`, `--nav-bg`, `--heading-case`). Catppuccin names are gone.
  - `internal/theme` reads the six blocks (`theme.Blocks()`); a style's block falls back to the default style's block for the same theme, as the cascade does.
  - `seasonal.ColourFor` replaces `AccentFor`. It returns `--seasonal` at 4.5:1 against `--bg`, `--surface` and `--surface-2` of every style (the wordmark caret is a text glyph). One value per theme serves all three styles, so the layout still injects two colours.
  - Contrast tests now cover every block. `TestThemeNonTextTokensContrast` (new) holds `--ring`, the priority edges and the focus ring at 3:1. The rule audit checks about 3x as many pairs.
  - One `.btn` base with primary, secondary, quiet, danger, solid-danger, icon and small variants replaced `action-btn`, `form-btn`, `action-drop`, `action-assign`, `form-btn-sm`, `action-btn-sm` and `confirm-btn-danger` in every template, `upload.js` and `dialog.js`.
  - Found while writing it: under `@layer`, the 44px coarse-pointer rule in `base` lost to every component's own `min-height`, so it moved to `utilities`. Sub-step promote and remove buttons used `visibility: hidden` until hover, so keyboard users could not reach them; they are now transparent until hover or focus-within, and always visible on touch.
- **Behaviour changes, approved with the direction.**
  - Collapsed rows show a round tick (the existing complete button), title and a meta line. "Do today" (tracker), reorder and drop (plan rows) moved into the expanded panel. Specs that clicked them on a collapsed row now expand it first. Drag-and-drop still reorders collapsed plan rows.
  - Tags in rows are plain spans, at most two plus "+N". Clicking a tag no longer filters; the filter bar does.
  - Done rows use a filled tick as "mark not done". Idea cards drop the status badge (the section heading says it). Goals show "due" in the meta line.
  - The completion swap delay is 200ms, down from 400ms, and idea triage is 200ms, down from 300ms.
  - The footer MCP badge and its health poll are gone.
  - The style selector (`#style-select`) sits at the end of the nav links, so it moves into the hamburger menu on phones. The choice is stored per device in `localStorage.style` and applied with the theme by the shared `appearance-script` partial, which the login page also uses.
- CI: the e2e job's timeout rose from 15 to 30 minutes, because axe and screenshots now run in three styles.
- **CI fixes and feedback loop.**
  - Three CI runs to green. Failures, in order:
    - Stale assertions (old class name, old progress text).
    - Phone screenshots not opening the folded row actions.
    - Axe `target-size` on filter chips half under the sticky nav after focus-after-change scrolled the page. Axe scans now set the nav static.
    - One real bug: at 320px the calendar week's unwrapped titles widened the page. Fixed with `min-width: 0` on the task list.
  - `TestE2ESelectorsExist` (Go, runs locally) fails when a spec selects a class or id that no template or app JS defines. It would have caught the stale class before a push.
  - Screenshots are their own CI job (`screenshots`, not gating the image build). The `e2e` job sets `E2E_SKIP_SCREENSHOTS`.
  - `.github/workflows/e2e-grep.yml` re-runs only the tests matching a `--grep` pattern on a pushed ref: `gh workflow run e2e-grep.yml -f grep="..."`.
  - Locally, `bash e2e/run.sh <spec or --grep ...>` works outside the nono sandbox.
- **Design review of the screenshots** (Design Reviewer agent; before set 52 shots from `9f80d92` run locally by the owner, its two phone state shots missing to the old spec bug; after set 168 shots from CI run #81). The before commit never got its own CI run because it was pushed together with the redesign.
  - Fixed:
    - the bulk-bar tag input lacked `form-input`
    - the month calendar's leading empty cells kept the week cell height
    - the house table clipped on phones (notes now live in the expanded row below 768px)
    - account cards were centred against a left-aligned heading
    - filter separators dangled (expand and select now sit in `.filter-actions`; separators are hidden on phones)
    - a lone "·" wrapped in the ideas legend (now a `::before`)
    - "Done (n)" had no disclosure marker
    - goal actions were misaligned
    - today's calendar cell had a doubled top edge
    - label casing was mixed
  - Brief misses addressed:
    - the Cards homepage is one column like the other styles
    - age is coloured only once stale
    - Paper's accent moved from terracotta to clay (`#8a5427` / `#d9a066`) because it read as the danger colour
    - Paper draws pills as outlines
    - an empty Today says "Nothing planned yet"
    - select mode pads the page so the bulk bar does not cover the last rows
    - link-styled chips and buttons get 44px on touch screens
  - Not taken:
    - dropping the mono `dash>_` wordmark (the owner chose to keep it)
    - one page width everywhere (three widths are deliberate)
    - the picker repeating tasks that also appear in the summary cards (a behaviour question, raised with the owner)
- CI on `main` (`3e81500`) passed in full, including e2e and the separate screenshots job. Deployed to fliptronic 2026-10-05 14:25 UTC (backup `dashboard-backup-20261005-142445.tar.gz`, image revision `3e81500`). Verified through Caddy: `/login` 200 and serves the new markup (`data-style`, caret); `/` and `/todos` 303 to login; `/events` 401; `/api/v1/todos` 404; cross-site login POST 403; CSP and HSTS present; `verify-stack.sh` green.
- Owner decision (2026-10-05): the homepage task picker may keep listing tasks that the summary cards below also show.
- **Phase 9 STOP:** the owner uses the redesigned app (all three styles, both themes, phone and desktop) before Phase 10 starts. Still open in the Phase 9 checklist until then.
