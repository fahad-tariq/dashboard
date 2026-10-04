# Plan 1: Foundations and module framework

## Goal

Make the dashboard safe, fast and pleasant to use. Then restructure it so a new feature area (a "module", such as exercise) can be added as one self-contained package plus one registration line, instead of edits spread across a dozen files.

This is the first of three plans:

| Plan | Scope | Status |
|---|---|---|
| 1 (this) | Fixes from the design and software reviews, wiring simplification, module framework, existing features migrated to modules, interaction rework | Ready |
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
- A design-token overhaul (spacing scale, type scale, `.btn` base, `@layer`).
- Removing multi-user or admin code.
- **Known WCAG gaps, accepted for a single keyboard-shortcut user and recorded in the backlog:**
  - single-key shortcut opt-out (2.1.4)
  - keyboard rescheduling in the calendar week view
- MCP Python tooling (uv, 3.14, ty). This moves to Plan 3.

---

## Phase 1: Toolchain, dependencies and test harness

**Purpose:** a trustworthy baseline. Later phases depend on lint, vuln checks, benchmarks and browser tests working.

- [x] **Go version.** Use one Go version across `go.mod`, the Dockerfile builder and CI, the latest stable (verify it; locally 1.27.1 is installed). Run `go mod tidy`.
- [ ] **Linting.** Rebuild `golangci-lint` for that Go version. Commit a `.golangci.yml` with the default linters plus `gosec`, `errorlint`, `bodyclose` and `gocyclo` at a threshold of 15. Mark existing offenders with a named `//nolint:gocyclo // reduced in Phase 5` comment; `main` is at 68 and `renderHomePage` at 38. `make lint` MUST pass.
- [ ] **Dependencies.** Upgrade chi (5.3.x or later), goldmark (1.7.17 or later), modernc sqlite, `x/crypto`, `x/net` and fsnotify. `govulncheck ./...` MUST report no called vulnerabilities. The review's chi and goldmark findings were not reproduced because the sandbox blocked it.
- [x] **Remove `goldmark-highlighting`.** bluemonday strips its inline styles, so it has no visible effect. Add a golden test of rendered markdown for a fenced code block before removing it, then update the golden.
- [x] **htmx.** Upgrade the vendored `htmx.min.js` and `htmx-sse.js` to the latest 2.0.x. Vendor `idiomorph-ext.min.js`, unused until Phase 8.
- [x] **Makefile targets.** Add `vuln`, `cover`, `e2e`, `bench` and `fmt`.
  - `bench` runs `BenchmarkMutate200` (a new benchmark: `UpdatePriority` on a 200-item tracker) with `-count=10`.
  - Record the baseline `benchstat` output in Working Notes.
- [ ] **Playwright smoke suite in `e2e/`.** Bun or npm are both installed.

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
- [ ] **CI** (`.github/workflows/build.yml`):
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

- [ ] **Explicit auth mode.**
  - Replace implicit "no hash and no users means open" with `DASHBOARD_AUTH=disabled`.
  - The server MUST refuse to start when auth is off without that switch.
  - It MUST also refuse when the switch is set and `ADDR` is not a loopback address.
  - A lost data volume must never produce an open dashboard.
- [ ] **Container exposure.**
  - The dashboard port MUST be bound to `127.0.0.1`, or not published if Caddy reaches it over the Docker network. Record which in the README.
  - Put the compose network on a fixed subnet (ipam) so trusted-proxy configuration is stable.
- [ ] **Login rate limiting.**
  - Remove `middleware.RealIP`.
  - Use the client IP from the rightmost `X-Forwarded-For` entry only when the direct peer is in `DASHBOARD_TRUSTED_PROXIES` (CIDRs, default empty, meaning use `RemoteAddr`).
  - Document the Caddy configuration.
  - Bound the limiter map with least-recently-used eviction. It must never reset wholesale.
  - Add per-account progressive *delay* (never a lockout, which would let an attacker lock out the only user).
  - Test that a spoofed `X-Forwarded-For` or `X-Real-IP` from an untrusted peer is ignored.
- [ ] **Login timing.** Unknown emails run bcrypt against a fixed dummy hash. Stop logging submitted emails.
- [ ] **API and MCP tokens.**
  - If `DASHBOARD_API_TOKEN` is unset or shorter than 32 characters, `/api/v1` is not mounted and an error is logged.
  - Add a separate inbound `MCP_TOKEN` for the sidecar, and rate-limit failed bearer attempts.
  - Destructive MCP tools are disabled unless `MCP_ALLOW_DESTRUCTIVE=true`: `delete_todo`, `remove_substep`, `clear_carried_plan` and `delete_commentary`. They also get `destructiveHint` annotations. Update the MCP smoke tests.
- [ ] **Cross-origin protection.**
  - Wrap the whole web router, including `/login`, `/logout` and `/upload`, in `http.NewCrossOriginProtection()`.
  - Mount `/api/v1` on a separate subrouter outside it. Do not use bypass patterns.
  - Test that a mismatched `Origin` and `Sec-Fetch-Site: cross-site` are rejected, and that htmx and form posts from the same origin pass.
- [ ] **Security headers middleware:**
  - `Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self'`
  - `X-Content-Type-Options: nosniff`
  - `Referrer-Policy: same-origin`

  Set `htmx.config.allowEval=false` via `<meta name="htmx-config">`. Record tightening `script-src` as a follow-up; there are 98 inline handlers today.
- [ ] **HTTP server.**
  - Use an `http.Server` with `ReadHeaderTimeout`, `ReadTimeout` and `IdleTimeout`.
  - The SSE handler clears its deadlines via `http.ResponseController` and sends a heartbeat comment every 30s.
  - Broker subscribers close on `RegisterOnShutdown`.
  - Graceful shutdown on SIGTERM uses the existing `shutdownCtx`.
  - Set `sm.IdleTimeout` on the session manager.
- [ ] **Container hardening.**
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

- [ ] **Backup first.**
  - Add a `make backup` target that copies every markdown file and the DB to a timestamped directory.
  - Run it and document restoring in the README.
- [ ] **Round-trip fixtures.**
  - Add real-shaped fixtures for each parser: blank lines in idea bodies, indented headings, sub-steps, image captions, all inline metadata, soft-deleted items and maintenance logs.
  - Parse-then-write MUST be byte-identical (or match a documented normalisation).
  - This guards every later change.
- [ ] **SQLite configuration.**
  - Set the pragmas through the modernc DSN so every pooled connection gets them: `_pragma=busy_timeout(5000)`, `foreign_keys(1)`, `journal_mode(WAL)`, `synchronous(NORMAL)`.
  - Remove `?_busy_timeout=5000` and the `db.Exec("PRAGMA ...")` loop.
  - Test: open three pooled connections and assert the pragma values on each.
- [ ] **Atomic file writes.** Add one shared helper that:
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
- [ ] **`MoveToList` ordering.** Add to the target before deleting from the source, so a failed add cannot lose the item.
- [ ] **Change events from services, not the watcher.**
  - Inject a publisher interface into the tracker, ideas and house services. After every successful write, from web, API or MCP, they publish a change event for their category.
  - Each service records a content hash of its last write.
  - Watcher callbacks return whether the file changed. When the hash matches the service's last write, the watcher skips `Resync` and does not broadcast.
  - The watcher today sends `file-changed` before running callbacks; reorder that.
  - Test seams: a fake publisher counting events, and a `Resync` counter on the service.
  - Test that one web mutation and one API mutation each produce exactly one event and zero `Resync` calls, and that an external edit produces one of each.
- [ ] **Drop the SQLite tracker mirror.**
  - Remove `store.ReplaceAll` from `mutate`, `AddItem`, `PermanentDelete` and `Resync`, and compute `Summary()` from the cache.
  - Remove the `store` parameter from `tracker.NewService` (about 30 call sites in 10 test files).
  - Remove `NewUserStore`, `NewSharedStore` and `ReplaceAllWithAttribution` and their tests in `registry_test`.
  - Adjust `admin_test`'s `tracker_items` cascade assertion.
  - Check `multiuser_test` against the in-memory count, since the SQL unique index used to collapse duplicate slugs.
  - The table stays in the schema, unused.
- [ ] **Static asset versioning.**
  - Hash each embedded static file at startup.
  - A `static "theme.css"` template function returns `/static/theme.css?v=<hash>`, and every template uses it.
  - `immutable` caching applies only to versioned requests.

**Verification:**
- `make bench` shows `BenchmarkMutate200` at least 3x faster than the Phase 1 baseline (`benchstat`, same machine).
- Round-trip, fault-injection and event-count tests pass.
- Playwright passes.

**Self-review**, then continue.

---

## Phase 4: Accessibility and visual correctness

**Purpose:** fix WCAG 2.2 AA failures, and lay the shared toast and id foundations that Phase 8 reuses.

- [ ] **Seasonal accent contrast.**
  - `internal/seasonal` computes a per-hue lightness and returns a concrete colour per theme, which the layout injects.
  - A test calls that same function for every day of 2028 (a leap year). It asserts at least 4.5:1 against the `--base` and `--mantle` values, and at least 3:1 for the focus ring.
  - The test reads token values parsed from `theme.css`, not duplicated constants.
- [ ] **Light-theme text tokens.**
  - Add `--success-fg`, `--warning-fg`, `--danger-fg`, `--priority-medium-fg` and `--on-accent`, darkened in light mode only.
  - Point text usages at them. Catppuccin hues remain for borders and fills.
  - The same token-parsing test asserts at least 4.5:1. Measured failures today: green 2.75, teal 3.08, peach 2.64, yellow 2.31, medium priority 2.92.
- [ ] **Stable id scheme.**
  - `item-{slug}` for tracker rows, `plan-{list}-{slug}` for plan rows, `{row-id}-substep-{n}-{action}` for sub-step buttons.
  - Record it in Working Notes; Phase 8 morph swaps depend on it.
  - Renaming an item changes its id. This is accepted until Plan 3's stable IDs.
- [ ] **Plan rows.**
  - Replace `role="button" tabindex="0"` on `.plan-item` with a real `<button aria-expanded aria-controls>` toggle. The row click stays as a mouse convenience.
  - Render the reorder buttons on all pointers, with an `aria-label` that names the task.
- [ ] **Announcer and toast.**
  - Add one `#announcer` polite live region and a toast partial to the layout. The toast shows text only, set via `textContent`.
  - Reorder announces the new position and returns focus to the moved control.
  - Remove the duplicate flash in `homepage.html`.
  - Flash messages use `role="status"`, or `role="alert"` for `flashErrorKeys` only.
  - Remove `aria-live` from the MCP footer badge.
- [ ] **Target size and focus.**
  - Every interactive control is at least 24x24 CSS px; 44 px on coarse pointers.
  - Add `scroll-padding` for the sticky nav and the bulk bar.
  - Restore a visible focus outline on `.form-input` and `.search-input`.
- [ ] **Semantics.**
  - Add a skip link to `<main>`.
  - The homepage always has an `<h1>`.
  - Add labels (visible or `aria-label`) to the goal, idea and tracker edit fields.
  - Declare `color-scheme: light dark`.
  - The first-visit theme follows `prefers-color-scheme`.
- [ ] **Accessibility tests.**
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

## Phase 9: Documentation

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

**Final self-review against the success criteria. STOP and wait for human review**, including a manual VoiceOver pass.

---

## Success criteria

| Criterion | Measure |
|---|---|
| No reachable vulns | `make vuln` reports zero called vulnerabilities |
| Gates | `make lint test vuln e2e` green locally and in CI; CI gates image builds |
| Security | Spoofed forwarding headers ignored; no API without a token; auth cannot be off by accident or off-loopback; cross-origin POST rejected; CSP present; destructive MCP tools off by default |
| Crash-safe writes | Fault-injection and concurrent-reader tests pass; round-trip fixtures byte-identical |
| No self-triggered work | One mutation (web or API) produces one event and zero `Resync` calls |
| Speed | `BenchmarkMutate200` at least 3x faster than the Phase 1 baseline |
| Accessibility | axe has zero serious or critical violations across pages, themes and interactive states; contrast holds every day of 2028 |
| Extensibility | The fixture module passes the contract test; the Phase 7 evidence shows only registration lines outside module directories |
| Simpler wiring | Single-user branch deleted; no `gocyclo` exclusions left on `main` or `renderHomePage`; one `toTask` |
| No stale UI | An external edit while an item is expanded appears without collapsing it |
| Behaviour preserved | Route golden diffs are all explained in Working Notes |

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
- Dependencies: chi 5.2.1 -> 5.3.2, goldmark 1.7.12 -> 1.8.6, x/crypto 0.49 -> 0.57, x/net 0.51 -> 0.59, fsnotify 1.9 -> 1.10.1. modernc sqlite still 1.36.3 (blocked, see above).
- `goldmark-highlighting` removed; golden `test/testdata/markdown/fenced_code.html` confirmed it only emitted unstyled `<span>`s. Removing it also dropped chroma, regexp2 and an old `x/exp`.
- htmx 2.0.8 -> 2.0.11; `htmx-sse.js` was already byte-identical to `htmx-ext-sse` 2.2.4 (latest), so unchanged; `idiomorph-ext.min.js` 0.8.0 vendored, unreferenced until Phase 8. npm tarball shasums verified against the registry. Static assets are still served `immutable` without versioning until Phase 3, so an open browser may need a hard refresh after deploy.
- Router construction moved from `cmd/dashboard/main.go` to `internal/app.NewRouter` so `test/routes_test.go` can call it (the test package cannot import `main`). The legacy admin auto-create moved with it. `authEnabledFlag` global replaced by a closure argument to `buildFuncMap`. The gocyclo hotspot is now `NewRouter` (62), not `main`.
- gocyclo > 15 found eight functions, not two. `NewRouter` and `renderHomePage` are marked `reduced in Phase 5`. The other six (`runMigrateData`, `parseItemLine`, `writeItem`, `insights.Digest`, `admin UpdateUser`, `ParseMaintenance`) are flat per-field branches and are marked accepted with a reason; they are not Phase 5 work.
- Route goldens: `test/testdata/routes_auth.golden` (149 lines) and `routes_noauth.golden` (134). The diff between them is exactly `/login`, `/logout`, `/account*` and `/admin/*`. `r.Handle` routes (`/static/*`, `/uploads/*`) appear once per HTTP method; that is how `chi.Walk` reports them.
- CI: `pull_request` trigger; jobs lint, vuln, test (race), test-mcp, e2e; image builds need all five and run only on push. Actions pinned to SHAs of their latest releases (checkout v7.0.1, setup-go v7.0.0, setup-python v7.0.0, setup-node v7.0.0, upload-artifact v7.0.1, docker login v4.6.0, metadata v6.2.0, build-push v7.4.0, golangci-lint-action v9.3.0 running golangci-lint v2.14.0). govulncheck pinned at v1.8.0. Dependabot covers actions, gomod, pip (`/mcp`) and npm (`/e2e`). The e2e job has no npm cache until `e2e/package-lock.json` exists (it could not be generated in the sandbox); commit the lockfile from the first successful run, then switch the job to `npm ci` caching.
- Benchmark baseline (Apple M5 Max, `make bench`, raw output in `docs/plans/bench-phase1.txt` for Phase 3's `benchstat` comparison): `BenchmarkMutate200` 3.070 ms ± 2%, 1.128 MiB/op, 9.486k allocs/op. Phase 3 target: 1.02 ms or less.
- Playwright suite (`e2e/`): written but never executed. The sandbox blocks binding any local port, `~/.npm` is read-only, the ms-playwright cache dir is denied, and the proxy 403s scoped npm packages. Versions of `@axe-core/playwright` and `@types/node` are caret ranges because the registry metadata could not be read; `@playwright/test` 1.63.0 matches the reachable unscoped `playwright` release. User 1's personal and ideas files live under `USER_DATA_DIR/1/` in auth mode, so fixtures seed `users/1/`.
- Existing bug found while writing the suite (not fixed; Phase 8 removes the flags): `tracker.js`'s `htmx:beforeSwap` guard only matches targets with `.tracker-page`, so `planDetailExpanded` and `planDragInProgress` never suppress SSE swaps on the homepage (`.homepage-page`) or house page. CLAUDE.md claims otherwise. The e2e test for it is `test.fixme`.
- e2e flakiness risks to watch on the first real run: specs wait a fixed 1.5s (`waitForSseSettle`) before expanding rows on the homepage and house page, because the app's own writes trigger an unsuppressed SSE refresh about 500ms later (removed in Phase 8); the watcher broadcasts before it resyncs (reordered in Phase 3); the reorder test relies on `hasTouch` making Chromium report `pointer: coarse` and checks that first; the suite uses 3 of the 5 logins per minute the rate limiter allows, so other specs must reuse the stored session. `retries: 0` is deliberate.

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
- Open redirects (fixed, test-first in `test/redirect_test.go`): `tracker.redirectBack` used the `Referer` path unchecked, so a path like `//evil.example/x` became a protocol-relative redirect. The login `next` check rejected `//` but accepted `/\evil.example` and `/<tab>/evil.example`, which browsers normalise to `//`. Both now use `httputil.IsLocalPath`, which rejects `//`, any backslash and any control character. Impact was low: the tracker redirect needs a POST that `SameSite=Lax` blocks cross-site, and login needs a victim to follow a crafted link.
- Git operations (branches, commits) need the owner's explicit approval.
- Follow-ups:
  - tighten CSP `script-src` after moving inline handlers
  - drop the `tracker_items` table in a later migration
  - `Plannable` capability (Plan 2)
  - SQLite migration hook (Plan 2)
  - stable IDs and slug collisions (Plan 3)
  - MCP uv, Python 3.14 and ty (Plan 3)
  - shortcut opt-out and calendar keyboard reschedule (backlog)
  - per-user SSE routing if multi-user returns
