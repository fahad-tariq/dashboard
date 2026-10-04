# Plan 1: Foundations and module framework

## Goal

Make the dashboard safe, fast and pleasant to use. Then restructure it so a new feature area (a "module", such as exercise) can be added as one self-contained package plus one registration line, instead of edits spread across a dozen files.

This is the first of three plans:

| Plan | Scope | Status |
|---|---|---|
| 1 (this) | Fixes from the design and software reviews, wiring simplification, module framework, existing features migrated to modules, interaction rework, visual design uplift | In progress: Phases 1-4 merged and deployed |
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

- [ ] **Collapse no-auth mode.**
  - With `DASHBOARD_AUTH=disabled`, a middleware injects user 1 into the context, including the name and admin fields that `auth.TemplateData` reads. It wraps every route, including `/events`.
  - Delete `SingleUserPlanHandlers`, `HomePageSingle`, `DigestPageSingle`, `CalendarPageSingle` and the single-user branch.
  - The expected route-golden diff (no-auth mode gains `/login`, `/account` and `/admin/*`, or they are excluded deliberately) is explained in Working Notes.
- [ ] **Local-dev data paths.**
  - Verify how no-auth mode resolves files today (`PERSONAL_PATH` and `IDEAS_PATH`, versus `USER_DATA_DIR/1/`).
  - If they differ, a registry override for user 1 MUST cover service paths, the watcher's watch spec and skeleton creation.
  - Ensure a user-1 row exists, so the purge loop, which iterates `auth.AllUsers`, still runs.
- [ ] **Commentary scoping.**
  - Web commentary and the ideas handler use `auth.UserID(r.Context())` instead of a hard-coded `1`.
  - This is safe only after the previous task, because no-auth requests currently carry user 0.
- [ ] **One `toTask` factory.**
  - Replace the three `ToTaskFunc` closures with one function over the personal, family and house services.
  - `AddItem` returns the slug it assigned; callers, including `APIAddTodo`, stop recomputing it.
  - No de-duplication (see Decisions).
- [ ] **API through the resolver.** The API resolves services through the same registry and resolver as the web handlers, with the API token mapped to user 1. Delete the duplicated API handler construction.
- [ ] **Route mounting.** Replace the positional parameters of `mountAppRoutes` with a struct, and move API registration into a function next to it.
- [ ] **Remove the Phase 1 `//nolint:gocyclo` markers** on `main` and `renderHomePage` by splitting them up.

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

- [ ] **Registry and validation.** At startup the registry rejects:
  - duplicate module IDs
  - duplicate or overlapping route prefixes
  - reserved prefixes (`/api`, `/login`, `/logout`, `/admin`, `/account`, `/static`, `/uploads`, `/events`, `/search`, `/plan`, `/digest`)
  - duplicate shortcut keys
  - keys reserved by core: `h c d / ? n k`, Escape and the arrows. `n` is held for Plan 3 quick capture.

  Ordering is deterministic.
- [ ] **Templates and shared components.**
  - Module templates live in `web/templates/<module-id>/`. Widen the embed pattern.
  - Add `web/templates/_components/` with partials for: page header, empty state, item row, quick-add disclosure, card, error banner and toast.
  - A render helper injects the layout data (`auth.TemplateData`, nav, flash).
  - Module templates MUST NOT use inline `on*` handlers.
  - FuncMap functions returning `template.HTML` stay limited to core sanitised markdown and `linkify`.
- [ ] **Nav and shortcuts.**
  - The layout renders nav from the registry. `aria-current="page"` is set by path prefix, so `/ideas/x` highlights ideas.
  - `Group` is primary or more. At most 6 primary links; the rest go behind a "more" disclosure button.
  - Links carry `data-shortcut`. `shortcuts.js` builds the `g x` map from those attributes, and `?` help lists them.
  - The hamburger closes on Escape and returns focus.
  - Check the nav at 320px.
- [ ] **Watcher and SSE.**
  - The watcher is driven by `WatchSpec`s with exact filenames, replacing the `HasPrefix(subpath, "personal")` matching.
  - Events are named `changed:<module-id>` and carry no item content.
  - No per-user SSE routing (single user; record this as a known limitation).
  - Services publish through `Deps`.
- [ ] **Search.** Search iterates registered `Searcher`s sequentially and never holds two module locks at once. Remove `search.ServiceResolver`.
- [ ] **Home widgets.**
  - `WidgetData` is data, not HTML: title, count, up to 5 items, link, empty text and severity.
  - One core partial renders each widget as `<section aria-labelledby>` with an `<h2>`, below the plan section, in registry order.
- [ ] **Fixture module.**
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

- [ ] **`todos` module:** the personal tracker plus goals (Plan 3 decides whether goals fold into tasks). Routes, templates, nav (`g t`, `g o`), watch spec, search, widget and API.
- [ ] **`family` module:** the shared tracker. `tracker.html` keeps scoping routes by `.ListName`.
- [ ] **`ideas` module:** list, detail, triage, research and to-task conversion via the Phase 5 factory.
- [ ] **`house` module:** maintenance and projects, keeping the two-file split.
- [ ] **Core pages stay outside modules:** home and planner, calendar, digest, search, auth, account, admin, uploads and commentary. Record in Working Notes which could become capabilities later, such as a `Plannable` capability for Plan 2.
- [ ] **Delete the replaced wiring:**
  - hard-coded nav links
  - the `shortcuts.js` switch
  - the `parseTemplates` page list
  - the watcher prefixes
  - module-specific `renderHomePage` parameters
- [ ] **Rename SSE triggers** to `sse:changed:<id>` in templates *and* JS (`tracker.js` re-triggers `sse:file-changed` today). The homepage listens to every module that contributes to it.
- [ ] **Requirement 4 evidence.** In Working Notes, list the files outside `internal/<module>/` and `web/templates/<module>/` that each migration touched beyond the registration line. Investigate anything other than the registration line and either fix it or justify it.

**Verification:**
- The route golden is unchanged.
- Playwright passes.
- A new Playwright test shows that an external edit to `ideas.md` refreshes `/ideas` and the homepage but not `/todos`.

**Self-review. STOP and wait for human review.**

---

## Phase 8: Interaction rework

**Purpose:** replace POST, redirect and full-swap cycles, and the suppression flags, with targeted updates that keep UI state.

- [ ] **Stable SSE connection.** Move `sse-connect` to a layout element that is never swapped. It is currently on swapped containers such as `tracker.html` and `homepage.html`, so each event reconnects.
- [ ] **Morph swaps.** Enable idiomorph. Live-refresh containers morph using the Phase 4 id scheme, so expanded items, select mode, focus and in-progress drags survive refreshes.
- [ ] **Remove the suppression flags.**
  - Remove `planDetailExpanded`, `trackerExpandedItems`, `planDragInProgress` and the `htmx:beforeSwap` discard logic.
  - Any refresh that must wait (mid-drag) is queued and applied afterwards, never dropped.
  - Playwright: an external edit made while an item is expanded appears without collapsing it.
- [ ] **Fragment responses.**
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
- [ ] **Focus after change.** When a row is completed, trashed or moved out of view, focus moves to the next row, or to the list heading if the list is now empty. Add a Playwright keyboard assertion.
- [ ] **Undo instead of confirm.**
  - Soft delete shows the Phase 4 toast with an undo button posting to `/restore`.
  - The toast stays visible at least 10s, pauses on hover or focus, and is reachable by keyboard.
  - Purge and user deletion keep the confirm.
- [ ] **Native dialogs.**
  - The confirm modal, search overlay and shortcut help become `<dialog>` with `showModal()`.
  - Remove the focus trap in `dialog.js` and simplify the Escape chain in `shortcuts.js`.
  - Search uses the combobox and listbox pattern with `aria-activedescendant`.
- [ ] **Submission feedback.**
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
