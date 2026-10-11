# Plan 4: API in production and capture from the phone

## Goal

Make the REST API a surface a phone can rely on and switch it on in production, so the owner can capture tasks by voice, from the share sheet and from the Action button through iOS Shortcuts. Clear dead code first, fix the API's correctness gaps, add per-device tokens, and close the small phone-web gaps that have a clear defect behind them. No Swift in this plan: the native app (widgets, App Intents, notifications, sync) is Plan 5 and builds on everything here.

| Plan | Scope | Status |
|---|---|---|
| 1 | Foundations and module framework | Done (2026-10-06) |
| 2 | Exercise module | Parked (2026-10-07): the owner uses Hevy |
| 3 | Stable item IDs, due dates on tasks | Done (2026-10-10) |
| 4 (this) | Cleanup, API correctness, device tokens, Shortcuts capture, small phone-web fixes | Proposed |
| 5 | Native iPhone app: sync endpoint, `ios/` package, Today and Due widgets, App Intents, local notifications | Waits on Plan 4, the Shortcuts usage measure, and the owner's Xcode and Developer Program setup |

## Context

On 2026-10-11 four councils (native design, iOS engineering, backend, product; twelve agents) analysed the repo, the fixture servers, the homelab deployment and the CI screenshots for a native iPhone app. The owner then chose: backend and Shortcuts before any Swift, house stays in scope for a later phase, Lock Screen widgets may show task titles, and cleanup comes first. Five review agents (design, security, simplification, code impact, quality) reviewed this plan against the code before it was accepted. The council files lived in `/tmp/dashboard-analysis/` and may be gone; everything this plan depends on is restated here.

Read `CLAUDE.md` before starting; its gotchas apply throughout.

**What the councils measured.**
- The API covers 32 of the 110 browser POST routes directly, 8 more indirectly. House has no API; goals, move, restore, bulk, idea edit and conversion, uploads, search and `/events` have none.
- A retried `POST /api/v1/todos` creates a duplicate. `AddItem` (`internal/tracker/service.go`) and `ideas.Service.Add` replace a taken ID with a fresh one and append a second item, and `MoveToList` depends on that behaviour, so idempotent create is a new service method, not a handler change.
- `GET /api/v1/plan` builds the `overdue` array with `planItemsToAPI(overdue, "")`, so carried items have no `list`. `itemsToAPI` drops `private`-tagged items but `GET /todos/{id}` and the plan serialiser do not. `GET /todos/{id}` returns a trashed item with no marker. Sub-steps come back as counts only. `UpdateTags` assigns raw values to the cache while `writeItem` cleans them for the file. `APIReorderPlan` answers 500 for a well-formed unknown ID because it never calls `httputil.IsNotFound`, which the tracker API handlers use.
- The API strips `!high` from a title through `CleanTitle` and keeps `#tag` as text: `tracker.ParseQuickAdd` has no caller, so a dictated "Buy milk #groceries !high" loses both.
- Nothing pins the JSON shapes: every API test decodes into `map[string]any`. `APIListPlan`, `APIReorderPlan`, `APIClearCarried`, `APIClearPlan` and `planItemsToAPI` have 0% coverage.
- HTML pages are never gzip-compressed although `middleware.Compress(5)` is on: no page handler sets `Content-Type` before writing, so chi's compressor sees an empty type and passes the body through. `httputil.ExecutePage` has five callers; fourteen other page renders (home, calendar, digest, account, admin, login) call `ExecuteTemplate` directly. JSON, `/search`, module pages and the fragment replies set the type and do compress. `/todos` is 47 KB on the wire at 7 items and would be 7 KB.
- Page cost is linear in open items because every row renders its expanded detail: 1.15 MB at 200 items. Production holds about 35 personal and 8 family tasks, so this does not bite yet and is not in this plan.
- On the phone: capture is 5 taps after the site loads (hamburger, todos, "Add task", focus, submit); search opens only on `/` or Ctrl+K and is unreachable by touch; a tab resumed in the morning shows yesterday's plan until an event arrives (`live.js` only resets `seenRevision` on `htmx:sseOpen`); with nothing planned the picker opens by default and lists every open task, pushing "Due soon" (the only home widget with an action) under the fold. The web app has no manifest or `apple-touch-icon`. Touch targets are already 44px on real phones (`theme.css` utilities layer, coarse pointer).
- Commentary: 0 rows in production, its only writer (Ironclaw) is not deployed, `docs/ironclaw-integration.md` is stale, and a root-level `backlog.md` is a leftover from the same era. `migrate-data` has run in production. `internal/slug` exports only `Slugify`, which `services/refs.go` still needs.
- Security: bearer auth is header-only and constant-time, outside cookie and cross-origin protection, not logged; the failed-bearer limiter keys on the real client IP behind Caddy. Turning the API on at `dash.home.caalador.net` (LAN and Tailscale only) adds no High finding. One trap: with `DASHBOARD_API_TOKEN` unset, `bearerAuth` would compare against an empty string, so a bare `Authorization: Bearer ` header would authenticate; today the 32-character mount gate prevents it.
- Tooling: this Mac has no Xcode; the nono session cannot write `~/Library/Developer` or `~/Library/Caches`; Playwright browsers cannot launch inside nono. The free Apple tier has no push or Siri capability and 7-day installs.

### Decisions already made

Do not reopen these while executing this plan.

- **No Swift in this plan.** iOS Shortcuts calling the API are the phone client. They test the capture hypothesis and remain useful after the app exists.
- **Per-device API tokens, minted on the account page.**
  - A table `api_tokens` (id, user_id, name, token_hash, created_at, last_used_at, revoked_at) is appended to `internal/db/migrations.go`.
  - The owner, logged in on `/account`, names a device and gets the plaintext token once, pastes it into the Shortcut, and can revoke it there later.
  - No pairing code and no unauthenticated API route: a Shortcut pastes a token either way. Plan 5 may add a QR rendering of a fresh token for the app.
  - The store lives in `internal/auth`; the page in `internal/account`. The admin and multi-user freeze stands: every row is user 1, the lookup filters on it, revoke is scoped to the session user.
- **`/api/v1` is always mounted.** `bearerAuth` accepts the env token only when `DASHBOARD_API_TOKEN` is at least 32 characters, then any unrevoked device token by SHA-256 lookup; with neither configured every bearer request is 401. The token is accepted in the `Authorization` header only. The "API not mounted" log line is replaced by an "API mounted" line with `env_token=true|false` and the device token count. `DASHBOARD_API_TOKEN` stays as break-glass.
- **The `private` tag no longer hides items from the API.** Its only consumer was `itemsToAPI`, written for the Ironclaw agent. Every token belongs to the owner, who decided their own titles may show on the phone. The tag keeps no other meaning; the filter is deleted rather than extended to the three paths that lack it.
- **Idempotent create.** `POST /api/v1/todos` and `POST /api/v1/ideas` accept an optional `id` that must pass `itemid.Valid`. A new service method (not `AddItem`, whose replace-a-taken-ID behaviour `MoveToList` and `toTask` rely on) returns the existing item without writing when the ID is already present, and the handler answers 200 with it; a new ID is created as today and answered 201. No idempotency-key store.
- **Quick-add syntax is parsed in one place for every caller.**
  - One helper in package `tracker`, called by the web add handlers and `APIAddTodo`, runs before `CleanTitle`.
  - It parses `#tag` tokens only when the caller supplied no tags, and `!priority` tokens only when the caller supplied no priority (an empty `priority` value counts as none). The two rules are independent, so a Shortcut that sends a priority still gets its dictated tags parsed.
  - Parsed tags go through `CleanMetaList`. Priority markers are case-insensitive and tolerate a space after `!` (`ParseQuickAdd` splits on whitespace, so this needs a one-token lookahead; a lone `!` not followed by a priority word stays in the title), because dictation produces "! High". `#42` becomes tag `42`; accepted.
  - The backlog parked quick capture on 2026-10-07 because the `n` key was not missed, and Plan 3 decided "no quick-add syntax"; the new reason is a single dictated string from a device with no form fields. Phase 5 strikes both earlier entries.
- **The plan endpoints already accept `personal` as an alias of `todos`** (`listService` in `internal/home/handler.go`); Phase 2 pins it with a test row and the README says so, so the create and plan requests in a Shortcut use one list name.
- **Compression is fixed by a middleware on the browser chain** (the `root.With` call in `internal/app/app.go` beside `fragmentResponses`, which also covers `/login`) that sets `Content-Type: text/html; charset=utf-8` when a handler reaches `Write` or `WriteHeader` with no type set, rather than editing nineteen render sites or removing the compress middleware. The wrapper implements `Unwrap`, because the SSE broker flushes through `http.NewResponseController`.
- **Change detection, sync and events wait for Plan 5**, where their consumer is written in the same phase. Decided shape, recorded for Plan 5: `changes.Recorder` exposes its content hash; one `GET /api/v1/sync` returns every list with an `ETag` over those hashes and answers 304 to `If-None-Match`; no per-item modified stamp and no change log. Per-list ETags, a `/changes` endpoint and a bearer SSE stream are built only if the app's sync proves to need them.
- **Phone-web layout work waits for the Shortcuts week.** The `?date=` home view, a bottom navigation bar and collapsing home widgets are each design-judged and larger than they look (the home page's live-refresh path is hard-coded to `/`, `ListOverdue` would show today's plan as carried on a tomorrow view, the bar must place six primary links and the style select, and two Playwright specs select the hamburger). They become a short follow-on plan sized by whether the owner still opens the web app on the phone for capture and planning once Siri works.
- **Commentary is removed**, with its table (a `DROP TABLE` migration), routes, store, `Deps.Commentary`, the template slot, `loadCommentary`, its tests, `docs/ironclaw-integration.md` and the root-level `backlog.md`. The last version is in git history.
- **`migrate-data` is removed.** `internal/slug` stays as it is.
- **The ES5 rule is dropped.** New JavaScript may use ES2015 syntax; existing files are not rewritten for it. The document-delegated listener rule stays, because it is about morph, not browser age.
- **Kept:** the three styles (Plan 1), uploads and `[images:]` (the share sheet may capture photos later), the digest (until the parked weekly review replaces it), select mode, the calendar views, the watcher as it is (its stop hook stays in the backlog).
- **Markdown stays the source of truth.** Admin and multi-user code stays frozen.
- **Tailscale stays the only path to the server.** No Funnel. A `tailscale serve` trial for a `*.ts.net` hostname is a homelab task for Plan 5, not this plan.

### Assumptions

- The owner's phone runs Tailscale whenever it is off the home Wi-Fi. A Shortcut run with the tunnel down fails and loses only that run.
- Caddy serves a Let's Encrypt certificate for `dash.home.caalador.net` (DNS-01 through Route53, homelab notes), which iOS trusts. Phase 3 verifies this from the phone before the Shortcuts are built, because "Get Contents of URL" cannot override an untrusted certificate.
- The Action button exists on iPhone 15 Pro and later; the Siri and share-sheet entry points work on any iPhone.
- Shortcuts and the token inside them sync through iCloud with the owner's account.
- Production holds about 35 personal, 8 family and 15 idea items; nothing in this plan is sized for more than a few hundred.

## Requirements

1. All existing behaviour MUST keep working except what this plan removes: tasks, family, goals, ideas, house, planner, calendar, digest, search, uploads, account, admin, REST API, SSE refresh.
2. Every bug fix MUST start with a failing test, committed before the fix.
3. Every phase MUST end with `make lint test` passing (with `INTEGRATION=1`, as CI does) and CI green, Playwright included, before deploying. Each phase MUST leave `main` deployable.
4. Round-trip fixtures in `test/testdata/roundtrip/` MUST stay byte-identical.
5. Route golden diffs MUST be explained in Working notes.
6. A title MUST NOT be able to set or change any inline tag through any input path, the quick-add helper included.
7. A device token MUST never be stored or logged in plaintext, MUST be accepted only in the `Authorization` header, and MUST stop working the moment it is revoked.
8. The API MUST stay unreachable with a session cookie, browser routes MUST stay unreachable with a bearer token, and no `/api/v1` route may run through the session manager.
9. With no env token and no device tokens, every `/api/v1` request MUST be answered 401, including one with an empty bearer value.
10. No new dependencies beyond the standard library and what `go.mod` already holds.

**Test conventions:** new Go tests go in `test/` as map-based table tests, with temp dirs and in-memory SQLite and no external services. Playwright runs only in CI (or `bash e2e/run.sh` outside the nono sandbox).

**Deploying and recording.** Follow the homelab repo's `CLAUDE.md` section "Dashboard (fliptronic)":
1. Run `./backup.sh` as its own call and check its exit code.
2. Pull the image, then `up -d`.
3. Confirm the container's `org.opencontainers.image.revision` label matches the commit.
4. Run the Caddy curl checks and `verify-stack.sh`.

"Record the deploy" means adding to that phase's Working notes: the commit, the image revision, the backup filename, the check results, and anything the phase asks you to measure. To roll back, restore the backup and pin the compose image to the previous revision.

## Non-goals

- Any Swift, Xcode project, widget, App Intent or APNs code (Plan 5).
- ETags, `/api/v1/sync`, `/api/v1/changes`, a bearer SSE stream (Plan 5, first phase).
- House, goals, move, restore, bulk, idea edit or conversion, upload or search routes on the API (sized in Plan 5).
- The `?date=` home view, a bottom navigation bar, collapsing home widgets (follow-on plan after the Shortcuts week).
- Server-sent notifications of any kind (APNs, Home Assistant, ntfy, email).
- Rendering collapsed rows without their detail, cache-backed `mutate`, or a faster parser.
- Removing styles, uploads, the digest or the calendar month view.
- Recurring tasks, weekly review, an agent-proposed plan, moving off markdown (backlog "Parked features").
- Rebuilding MCP in any form.

---

## Phase 1: Cleanup and compression

**Purpose:** remove what earns nothing before building on the API, and stop sending pages uncompressed.

- [ ] **Remove commentary.**
  - Delete `internal/commentary`, `Deps.Commentary` in `internal/module/module.go` and its wiring in `internal/app/app.go` (store construction, `buildModules`, `handlers`, `purgeExpired`), `/commentary` from the reserved prefixes, the browser GET and the three API routes.
  - Delete `commentarySt`, `SetCommentaryStore` and `HasCommentary` from the tracker, house and ideas handlers and the four module adapters that call `SetCommentaryStore`; `ForgetItems` on purge and `Copy` on move go with them.
  - Delete `ValidateListWithIdeas` and `validListsWithIdeas` in `internal/httputil/validate.go` (the commentary handlers are their only callers) together with `TestValidateListWithIdeas` in `test/validate_test.go`, and the comments in `internal/auth/users.go` and `web/static/live.js`.
  - Delete the slot in `tracker/tracker.html` and `ideas/idea.html`, `loadCommentary` in `tracker.js`, and the `.commentary-*` rules in `theme.css`.
  - Append a `DROP TABLE IF EXISTS item_commentary` migration; flip the `item_commentary kept` row in `test/db_open_test.go`.
  - Delete `test/commentary_*_test.go` and `TestCommentaryScopedToUser` in `test/wiring_test.go`. In `test/live_markup_test.go`, `TestMorphSkipElementsHaveIDs` currently fails when no `data-morph-skip` element renders, because the slot was the only one: drop that guard and keep the id check for future skip elements.
  - Delete `docs/ironclaw-integration.md` and the root-level `backlog.md`. Update README (routes, API, data storage), CLAUDE.md (architecture paragraph, the stale claim that the upload area is skip-marked, the Ironclaw gotcha) and `docs/backlog.md` (both commentary entries under "Known limitations").
  - Run `TestRouteGolden -update` and explain the diff.
- [ ] **Remove `migrate-data`.** Delete `cmd/dashboard/migrate.go`, its subcommand in `main.go`, `test/migrate_data_test.go` and the README section "Migrating legacy data". `internal/slug` stays.
- [ ] **Backlog removals.**
  - Delete the test-only constructors `tracker.NewHandler` and `ideas.NewHandler` and move their six test call sites to the resolver constructors (`httputil.PageMap` supplies the lookup). Remove the backlog entry.
  - Apply the five `modernize` suggestions: `maps.Copy` in `module/render.go` and `test/web_security_test.go`, `max` in `insights.go`, range-over-int in `test/itemid_test.go`, `WaitGroup.Go` in `test/atomicfile_test.go`.
- [ ] **Drop the ES5 rule** from CLAUDE.md and the headers of `announce.js`, `live.js`, `planner.js` and `shortcuts.js`. Do not rewrite existing JS.
- [ ] **Compress HTML (bug, test-first).**
  - A table test requests `/`, `/todos`, `/login` and `/account` with `Accept-Encoding: gzip` through `app.NewRouterWith` and expects `Content-Encoding: gzip` and `Vary: Accept-Encoding`; a fragment reply and a static file are control rows.
  - Add the middleware decided above to the `root.With` chain in `app.go`: it sets `Content-Type: text/html; charset=utf-8` in both `Write` and `WriteHeader` when no type is set (the rate-limited login page calls `WriteHeader(429)` before rendering), leaves handlers that set their own type untouched, and implements `Unwrap` so `/events` keeps flushing.
  - Record the wire size of `/`, `/todos` and `/ideas` before and after in Working notes.
- [ ] **Verification.** `make lint test` with `INTEGRATION=1`. `TestE2ESelectorsExist` after the template edits. CI green. Deploy and record it; confirm in production that `/todos` arrives gzipped (curl through Caddy with the session cookie).
- [ ] Perform a critical self-review of the changes with an independent agent and fix what holds up.
- [ ] **STOP and wait for human review.**

---

## Phase 2: API correctness and contract

**Purpose:** the API answers what a phone client needs, in shapes that a test pins.

- [ ] **Plan endpoint (bugs, test-first).** One table-test row each:
  - every item in `GET /api/v1/plan`, overdue ones included, carries `list`;
  - the response carries `today` (the server's date in `config.Location`) and `deadline` per item; overdue items keep their original `planned` date so a client can label "from Tue";
  - `APIReorderPlan` with an unknown ID answers 404: `mutateBatch` already returns a "not found" error and `httputil.IsNotFound` already maps it at seven sites in `tracker/api.go`; the handler just does not call it. Remove the backlog "Known limitations" entry;
  - `PUT /api/v1/plan/{id}` and `DELETE` with `"list": "personal"` behave as with `todos` (already true; the row pins it).
- [ ] **Item shape (bugs, test-first).**
  - `GET /api/v1/todos/{id}` and list items carry `deleted` (the date, or absent), `plan_order`, and `sub_steps` as `[{text, done}]` alongside the existing counts.
  - `UpdateTags` and `ApplyEdit` clean tag and image values in the service before assignment so the cache matches the file.
  - Delete the `private` filter in `itemsToAPI`; flip the assertion in `test/tracker_api_test.go` that expects it filtered, add rows for by-ID and the plan, and drop the README sentence "leaves out items tagged `private`".
- [ ] **One serialiser, named structs.** Replace the `map[string]any` responses in `tracker/api.go` and the plan handlers with named structs with JSON tags, built by one function per item type; move `APIListPlan`, `APISetPlan` and `APIClearPlan` from `home/handler.go` into `home/api.go`, joining `APIReorderPlan` and `APIClearCarried` which are already there. Mutations that change an item return the item, not `{"status":"ok"}`.
- [ ] **Idempotent create.** Add the service method decided above to `tracker.Service` and `ideas.Service`; `APIAddTodo` and `APIAddIdea` accept an optional `id`. Table-test: no ID (201), valid new ID (201, stored), valid existing ID with a different title (200, stored item unchanged, no write), existing ID that belongs to a trashed item (200 with `deleted` set, no write), invalid ID (400).
- [ ] **Quick-add helper.** Implement the helper decided above and call it from the web add handlers and `APIAddTodo`. Table-test: plain title, `#tag`, `!high`, `! High`, a lone `!`, both markers, trailing `#`, `#42`, tags supplied with `#x !high` in the title (`#x` stays, priority parses), priority supplied with `#x` in the title (tag parses), and a title whose parsed remainder still cannot set a tag (requirement 6).
- [ ] **JSON goldens.** A `TestAPIGolden` drives every `/api/v1` route through `app.NewRouterWith` against the e2e fixture files, normalises IDs and dates, and compares with `test/testdata/api/<method>_<path>.golden`, with an `-update` flag like the route goldens. Because it drives every route, the plan API handlers and the serialiser branches (house fields, images, deadline, deleted) are executed by construction; record their coverage in Working notes.
- [ ] **README "Routes > API".** The route list with request bodies, the quick-add syntax, the `id` field and the `personal` alias; response shapes point at `test/testdata/api/`.
- [ ] **Verification.** `make lint test` with `INTEGRATION=1`. Route goldens unchanged. CI green. Deploy and record it (the API is still off in production, so the deploy carries no risk).
- [ ] Perform a critical self-review of the changes with an independent agent and fix what holds up.
- [ ] **STOP and wait for human review.**

---

## Phase 3: Device tokens, API on, Shortcuts

**Purpose:** the owner mints a token for their phone on the account page, the API is live in production, and Siri captures a task.

- [ ] **Token store.** Migration for `api_tokens` as decided. `internal/auth/tokens.go`: `CreateToken(userID, name)` returns the plaintext once (32 random bytes, hex); `LookupToken(plaintext)` matches `sha256` with `revoked_at IS NULL AND user_id = 1` and sets `last_used_at`; `ListTokens`; `RevokeToken(id, userID)`. Names go through `httputil.CleanMetaValue`, are capped at 64 runes, and may not be empty. Table-test create, lookup, revoked lookup, unknown hash, name cleaning.
- [ ] **Bearer middleware (bug, test-first).**
  - Rows: env token unset and header `Bearer ` (401), env token unset and `Bearer x` (401), env token shorter than 32 characters and an empty bearer (401), env token set and matching (200), device token (200), revoked device token (401), session cookie on `/api/v1` (401), bearer on `/account` (303).
  - `bearerAuth` compares the env token only when it is at least 32 characters, then looks up device tokens. `mountAPIRoutes` always mounts; the start-up log line says `API mounted` with `env_token` and the device token count.
  - One `slog.Info` per API write with the token name and route, never a title or token.
- [ ] **Account page.** A form to name a device and mint a token, shown once with a copy control and a `Cache-Control: no-store` response; a list of tokens with name, created and last used; a revoke button with `data-confirm`. Changing the password does not revoke tokens; the page says so. Route goldens regenerated and the diff explained.
- [ ] **Docs.** README: minting and revoking tokens; `DASHBOARD_API_TOKEN` as optional break-glass; a note that no-auth mode lets anyone on loopback mint tokens (dev databases only). `.env.example` updated. The homelab repo's Dashboard table ("API not mounted") is updated at deploy time.
- [ ] **Shortcuts recipes.** A README section "Capture from an iPhone" with three Shortcuts, action by action:
  - "Add task": Ask for Input (or Share Sheet input); Choose from Menu for priority, where "none" sends no `priority` key so a dictated "! High" still parses; Get Contents of URL `POST /api/v1/todos` with the bearer header and `{"title": ..., "list": "personal"}` plus `priority` when chosen; Get Dictionary Value `id` and `title`; Choose from Menu "Plan for today?" where Yes runs `PUT /api/v1/plan/{id}` with `{"list": "personal"}`; Show Result of the title. The same Shortcut is assigned to the Action button, so that run asks one question.
  - "Capture idea": Share Sheet input limited to URLs and text, `POST /api/v1/ideas` with the page title and the URL in `body`.
  - "What's on today": `GET /api/v1/plan`, Repeat over `personal`, `family` and `overdue`, Speak Text.
  - State plainly: the token is stored in the Shortcut and syncs through iCloud with it, so mint one token per device, never share or export these Shortcuts, and revoke from `/account` if one is shared by mistake; Siri runs them from the locked phone; a run with Tailscale down fails and loses only that run; a 429 means wait a minute.
- [ ] **Verification.** `make lint test` with `INTEGRATION=1`. CI green. Deploy and record it. In production: open `https://dash.home.caalador.net` in Safari on the phone and confirm the certificate is trusted; mint a token on `/account`; confirm `GET /api/v1/plan` through Caddy with it and 401 without; confirm the start-up log line; confirm Caddy needs no route change. Record the first Siri capture.
- [ ] Perform a critical self-review of the changes with an independent agent and fix what holds up.
- [ ] **STOP and wait for human review.** The owner builds the three Shortcuts from the README section and uses them for two weeks. Phase 5 measures that period.

---

## Phase 4: Small phone-web fixes

**Purpose:** four defects the owner meets on the phone until Plan 5 ships, each mechanical and testable.

- [ ] **Capture on the web.** Give the add form `<details>` in `tracker.html` the id `add`; `tracker.js` opens it and focuses the title when the hash is `#add`; the `n` key (reserved in `module.ReservedKeys`, unbound in `shortcuts.js`) does the same on list pages and goes to `/todos#add` from the home page. The form accepts the quick-add syntax through the Phase 2 helper. Update the `?` help dialog.
- [ ] **Search by touch.** A nav button with `aria-label="Search"` and `aria-keyshortcuts="/"` opens the existing search dialog through `openSearch()`; `/` and Ctrl+K stay.
- [ ] **Stale tabs.** The home live container carries `data-today`; `live.js` fires the live refresh on `pageshow` with `persisted` and on `visibilitychange` to visible when the page was hidden for more than two minutes or `data-today` is no longer today in the browser's clock. Append a constant custom trigger name to `homeTrigger` (`internal/app/templates.go`) so the live-refresh element carries it from first render (CLAUDE.md: its attributes never change at runtime), and route the stale trigger through the same hold queue as SSE triggers (`live.js` gates only `sse:` names today).
- [ ] **Due soon above the fold.** Render the picker closed on every pointer when nothing is planned (`homepage.html` sets `open` server-side and cannot know the pointer); the "Anything for today?" summary still invites the click. `planFromPicker` in `e2e/tests/helpers.ts` already opens a closed picker, so the planner specs are unchanged. If the owner wants it open on desktop after review, `planner.js` opens it on load under `matchMedia('(pointer: fine)')` and `data-keep-attr="open"` keeps that across morphs.
- [ ] **Installable.** Commit one SVG mark under `web/static/` (no text) and the PNGs rendered from it by a standard-library Go program recorded in Working notes: 180px `apple-touch-icon`, 192px and 512px manifest icons. Serve `manifest.json` (not `.webmanifest`: the Alpine image has no MIME table for it) through `{{static}}` with `display: standalone`; icon paths inside the JSON are plain `/static/` paths, which the template test does not cover and which are served `no-cache`. `theme-color` follows the OS scheme and may disagree with a `data-theme` chosen in the app (accepted). The `appearance-script` partial still runs on first paint in standalone mode.
- [ ] **Playwright.** One spec at 375px with `hasTouch` covering `n`, `#add`, the search button, the closed picker, the manifest (fetch it and check the icon entries) and a stale-tab check (`page.clock.setFixedTime` past midnight, dispatch `pageshow` with `persisted: true`, wait for the refresh `GET /`); `TestE2ESelectorsExist`; axe on the home and todos pages at 375px with `hasTouch` in the six style and theme blocks.
- [ ] **Verification.** `make lint test` with `INTEGRATION=1`. CI green including Playwright. Deploy and record it.
- [ ] Perform a critical self-review of the changes with an independent agent and fix what holds up.
- [ ] **STOP and wait for human review.**

---

## Phase 5: Measure, document, hand over

**Purpose:** know whether phone capture is used, and leave Plan 5 and the follow-on plan everything they need. This phase opens no earlier than two weeks after the Phase 3 deploy; Phase 4 runs in between.

- [ ] **Capture measure.** Two commands recorded in Working notes: a count of the Phase 3 API-write log lines by token name from `docker logs` per ISO week, and a count of `[added: <week dates>]` lines in `personal.md`, `family.md` and `ideas.md`. Plan completion and carry-over come from the digest. Record both counts for each of the two Shortcuts weeks.
- [ ] **Documentation.**
  - README: API, tokens, Shortcuts, manifest.
  - CLAUDE.md: architecture (API always mounted, device tokens, no commentary), conventions (quick-add helper, named API structs and goldens, ES5 rule gone), testing (API goldens).
  - `docs/backlog.md`: strike the "Quick capture" parked entry and note that Plan 3's "no quick-add syntax" decision is superseded; add what this plan found and left (row detail on expand, cache-backed `mutate`, house API, bearer uploads and search, SSE client cap).
- [ ] **Handover notes in the backlog.**
  - "Plan 5 handover": the decided sync shape; the app layout (`ios/` in this repo, `DashboardKit` SwiftPM package tested with `swift test --disable-sandbox` and by contract tests against the Go binary, XcodeGen targets, shared Keychain group at `AfterFirstUnlockThisDeviceOnly`); the sync model (snapshot plus outbox with last-write-wins, cache-first widgets with midnight timelines); first surfaces (Today and Due widgets, `AddTask` and `WhatsOnToday` intents, local notifications first, no Live Activity); the 30-day kill condition.
  - Owner preconditions for Plan 5: Developer Program, Xcode with a simulator runtime, `xcodegen`, a nono session allowing `~/Library/Developer` and `~/Library/Caches`.
  - "Phone-web follow-on": the three deferred items and the design findings recorded under Decisions.
- [ ] **Verification.** Independent agents verify README and CLAUDE.md against the code (the owner does not read docs). All success criteria checked.
- [ ] Perform a critical self-review of the changes with an independent agent and fix what holds up.
- [ ] **STOP and wait for human review.** Decision rule for Plan 5: at least five captures a week from the phone over the two weeks keeps capture intents in Plan 5's first app phase; fewer means Plan 5 starts with widgets only and capture stays on Shortcuts.

---

## Success criteria

1. A retried create with the same `id` yields one item, and a dictated "Buy milk #groceries ! High" becomes a task titled "Buy milk" with tag `groceries` and priority high through the web form and the API, with and without a `priority` field in the request (Go tests).
2. Every `/api/v1` response shape is pinned by a golden; `GET /api/v1/plan` items all carry `list`; an unknown ID on reorder answers 404; a `private`-tagged item appears in the list, by ID and in the plan (Go tests).
3. With no env token and no device tokens, an empty bearer answers 401; a device token minted on `/account` works, is listed there, and answers 401 after revocation; the env token still works; a cookie cannot call the API and a bearer cannot reach `/account` (Go tests plus production curl recorded).
4. `/api/v1` is on in production and the start-up log records how it is configured (deploy record).
5. The three Shortcuts work from Siri, the share sheet and the Action button, and the first capture is recorded (owner's observation).
6. `/`, `/todos`, `/login` and `/account` arrive gzipped in production (production curl recorded).
7. On a 375px touch viewport: `n` and `#add` open the add form, the search button opens the dialog, the picker is closed when nothing is planned, the manifest and `apple-touch-icon` are served through `{{static}}` and the manifest names its icons, and axe passes on home and todos in all six style and theme blocks (Playwright).
8. A tab resumed after midnight refreshes without an SSE event (Go test of the rendered trigger plus the Playwright clock check in Phase 4).
9. Commentary, `migrate-data`, the root `backlog.md` and the ES5 rule are gone; round-trip fixtures are byte-identical; route golden diffs are explained.
10. Two weeks of capture counts are recorded in Working notes before Plan 5 opens.
11. Lint, the race suite with `INTEGRATION=1`, and Playwright in CI pass at every phase.

## Risks

- **A device token leaks** (phone stolen, Shortcut shared). Mitigation: revoke from `/account` in any browser; the server is reachable only over the LAN and Tailscale; tokens are hashed at rest; the token has no scope beyond the owner's lists. Rating low under those assumptions.
- **The empty-bearer bypass** (Context). Requirement 9 and its test rows exist for this; the Phase 3 middleware task must land with them.
- **The commentary removal touches about 38 files.** Mitigation: the compiler finds Go uses, `TestE2ESelectorsExist` and `TestMorphSkipElementsHaveIDs` find template residue, the phase deploys alone.
- **The compression middleware mislabels a non-HTML response.** Only responses with no type set are touched; JSON, SSE, static files and fragments already set theirs. The control rows in the test cover them.
- **Rollback after Phase 1.** `Migrate` runs nothing when the stored version is ahead of an older image's list, so the older image starts, but its commentary routes would fail on the missing table and the hourly purge would log an error. Roll forward rather than back; nothing is lost either way, the table was empty.
- **Rollback after Phase 3.** An older image ignores `api_tokens`; phones get 401 until rolled forward.
- **The quick-add helper changes web titles containing `#`.** "Issue #42" now becomes title "Issue" with tag `42`. Accepted and documented; the owner can add a tag field value to opt out.

---

## Working notes

One `### Phase N notes` section per phase: decisions, measurements, deploys (as defined under Requirements), route golden diffs, and follow-ups.
