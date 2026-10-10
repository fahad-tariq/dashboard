# Backlog

Plan 3 (`docs/plans/03-stable-ids-and-due-dates.md`, done) added stable item IDs and due dates on tasks; this file lists smaller follow-ups and parked work.

## Parked features

Scoped out of Plan 3 on 2026-10-07 because the owner had not missed them. Revisit only when one of them actually bites.

- **Recurring tasks.** Undecided: fixed schedule ("bins every Tuesday") or from completion ("two weeks after I last did it"). Working proposal: one item with `[repeat: 2w]` (reusing maintenance's `ParseCadence`) that, on completion, writes a done copy (so the digest counts it) and moves its `[planned:]` date forward; no deadline. Do not merge maintenance into it: maintenance has its own log, overdue logic and widget.
- **Weekly review.** Proposal: a `/review` page replacing `/digest`, with last week's completions, carried-over plan items, deadlines in the next 7 days that are not planned, stale tasks and an untriaged-ideas count, plus a "last reviewed" nudge on the homepage. Never agreed.
- **Quick capture.** `n` stays reserved in `module.ReservedKeys`. `tracker.ParseQuickAdd` (`title #tag !priority`) exists but has no production caller.
- **Reminders.** There is no outbound channel (email, push, ntfy, webhooks); choose one first.
- **Agent-proposed daily plan.** Nothing in the app calls an LLM, and there is no MCP server (see "MCP removed").
- **Storage.** The owner never hand-edits the markdown files, which is the main reason they are the source of truth. IDs, deadlines, recurrence and review queries would each be simpler in SQLite. Not worth a rewrite now; revisit if storage starts to hurt.

Plan 2 (exercise module) is parked: gym sessions are logged in Hevy. Its two framework pieces wait for a real consumer rather than being built speculatively:
- **SQLite migration hook.** `internal/db/migrations.go` is one global list and `module.Deps` has no database handle. Build the per-module hook with the first module that stores data in SQLite.
- **`Plannable` capability.** `home.Lists` hard-codes the three tracker services. Build it with the first non-tracker module whose items belong in the daily planner.

## MCP removed

**Decision (2026-10-07):** the Python MCP sidecar (`mcp/`, 24 tools, its CI jobs, image and compose service) was deleted. Rebuild it when the owner actually wants to edit the lists from an agent on the Mac or iPhone.

Why:
- It had been off in production since 2026-10-04, and the owner did not use it.
- It cost 1,300 lines of Python, two CI jobs, a second image and Dependabot entries.
- Its smoke tests mocked the dashboard API with respx, so an API change could break every tool while CI stayed green.
- Plan 3's ID switch would have meant rewriting its 18 slug-based tools for nobody.

The last version is in git history: the parent of the commit that removed `mcp/`.

**Before rebuilding:**
- **Turn the API on in production first.** That means a `DASHBOARD_API_TOKEN` of 32+ characters and a Caddy route. The API addresses items by ID.
- **Consider a Go MCP server inside the dashboard binary** instead of a Python sidecar. It would remove the second image, the second token, and the drift between tools and API. Plan 1 floated this; weigh it against the maturity of the Go MCP SDK at the time.
- **Test the tools against the real API** (a test server from `app.NewRouterWith`), not mocks.
- **Cover what the old tools lacked:** house, goals, move, full edit and search.

**What the old sidecar taught us:**
- **Network and tokens.** It sat behind Caddy as `handle_path /mcp/*` to `127.0.0.1:9100`, in a read-only, unprivileged container. It used two tokens: inbound `MCP_TOKEN` and outbound `DASHBOARD_API_TOKEN`, both at least 32 characters and different, so a leaked client token cannot call the API directly.
- **Destructive tools were opt-in.** Delete todo, remove sub-step, clear carried plan and delete commentary were registered only with `MCP_ALLOW_DESTRUCTIVE=true`.
- **FastMCP lifespan.** With `stateless_http=True`, the lifespan runs per request, so keep the HTTP client a module-level singleton. `StreamableHTTPSessionManager.run()` runs once per instance, so tests must share one module-scoped `TestClient`. Clients must send `Accept: application/json, text/event-stream`.
- **Version pinning.** The `mcp` 2.x Python SDK renamed `FastMCP`. A grouped Dependabot update widened `<2` despite `update-types` (PR #9); only an ignore with `versions: [">=2"]` held.
- **API input.** The API strips inline tags from LLM-written titles and bodies (`httputil.StripInlineMetadata`). `PUT /api/v1/todos/{id}` keeps omitted fields, so a tool can send only what it means to change. Keep both properties.

## House

### Test house plan rows
Reorder and calendar drag post `list=house`, and `home.listService` maps it to house projects, so it works end to end. Tick, untick and the plan bulk actions are tested with house, but `/plan/reorder` and `/plan/set` are not. A Go handler test next to `test/fragment_test.go` is enough.

### itemToAPI Budget/Actual/Status
`itemToAPI` in `tracker/api.go` does not expose Budget, Actual or Status, and the tracker API refuses `list=house`. No API exposes them; `planItemsToAPI` returns only id, title, priority, done, planned, tags and list.

### MoveToList for house projects
Moving items between personal/family and house is not wired. `tracker.Handler.MoveToList` is bound to a fixed pair of lists, so it needs a target parameter; moving in must set `Status = "todo"` (as `toTask` does); `house.html` needs move controls.

## Security

### Tighten CSP `script-src`
`script-src` still allows `'unsafe-inline'`. Module templates use delegated `data-action` handlers, but inline `on*` handlers remain in `layout.html`, `homepage.html`, `login.html` and `admin-users.html`, and inline `<script>` blocks in `layout.html`, `login.html`, `account.html`, `admin-password.html` and the `appearance-script` partial. The appearance script must run before first paint, so give it a CSP hash or load it as a blocking script in `<head>`. `style-src 'unsafe-inline'` has to stay: templates use `style=""` and the seasonal colour is an injected `<style>` block.

### Image filename validation in templates
Image filenames are stored unchecked (`httputil.ReconstructImages` from forms, `images` in `APIUpdateTodo`) and rendered into `img src`. `http.Dir` prevents file-serving traversal, but the `src` could point at any same-origin GET. Validate against the shape `upload/handler.go` produces (32 hex characters plus `.png`, `.jpg`, `.gif` or `.webp`) on write and in `splitImageCaption`.

## Accessibility

### Calendar drag-and-drop keyboard alternative
Calendar week tasks reschedule only by drag. Add a keyboard alternative (WCAG 2.5.7; accepted in Plan 1 for a single owner).

### Single-key shortcut opt-out
`j`, `k`, `u`, `/`, `?` and the `g` + letter sequences are character-key shortcuts with no way to turn them off (WCAG 2.1.4). Accepted for the single owner.

## Architecture

### Capabilities for core pages
Candidates from Plan 1 Phase 7: `DigestSource` and a tag source (digest and the homepage tag card read ideas through `home.Lists`), `Purger` (the purge loop names services directly), and a manifest field for commentary list names (`httputil` hard-codes `todos`, `family`, `house`, `ideas`).

### Per-module static assets
`web/static/` is one flat embedded directory. A module with its own JS adds a file there. Give modules their own static directory if exercise needs one.

### Fragment responses render the whole page
`internal/app/fragment.go` replays the redirect and returns the full page, which htmx narrows with `hx-select`. Rendering only the `content` block would save bytes if it ever matters.

### Remove test-only handler constructors
`tracker.NewHandler` and `ideas.NewHandler` (static services and a template map) are used only by tests. Move those tests to the resolver constructors and delete them.

### Watcher has no stop hook
`watcher.Watch` takes no context, so shutdown cannot stop it and each router-building test leaks one watcher.

### Select mode is tied to two page classes
`tracker.js` `toggleSelectMode`/`exitSelectMode` look up `.tracker-page, .ideas-page`. A new module cannot get select mode without reusing those classes.

## Known limitations

- **Bulk trash has no undo.** There is no bulk restore route, so bulk trash keeps its confirm dialog.
- **Unsubmitted `<select>` changes reset on a live refresh.** Text fields and checkboxes are kept; selects are not.
- **SSE events go to every client.** No per-user routing; revisit if multi-user returns.
- **Admin user deletion:** the watcher recreates the deleted user's directory, and the user's `item_commentary` rows stay behind. Admin code is frozen.
- **Commentary on shared lists** written through the API (as user 1) is not shown to other users.
- **No-auth then auth database:** the `local@localhost` placeholder keeps id 1, so the bootstrapped admin gets a later id while the API and user-1 paths still point at the placeholder. Dev databases only.
- **Accepted in Plan 1:** the failed-bearer limiter cannot slow guessing (a valid token always passes); the login account delay is a per-request sleep; `sm.IdleTimeout` rewrites the session row and sends Set-Cookie on every request; a user whose services are first created by a watcher event misses that first broadcast.
- **Converted-idea link:** the "Converted to a task" link on the idea page and the ideas list always points at `/todos#item-<id>`, even when the task went to family or house.
- **`APIReorderPlan`** answers 500 for a well-formed ID that is not in the list (API off in production).
- **Items without an ID** (a failed load-time save) cannot be addressed, share the key `""` in the planner's exclude maps and give duplicate DOM ids, until the next write to that file assigns them IDs.
- **Commentary cleanup across users:** `DeleteItems` and `Copy` act on an item ID for every user, so a chance ID collision between two users' files could touch the other user's commentary. Needs a second user.
- **Reference relinking runs once:** `[from-idea:]`/`[converted-to:]` slugs are turned into IDs only on the load that assigns IDs; a crash between the two leaves slugs that `{id}` routes cannot resolve. Production has none.
- **Rollback to a pre-ID image:** its writer moves `[id:]` ahead of other tags, after which the current parser no longer reads it and assigns fresh IDs, losing every ID and reference. Roll back by restoring a backup with the matching image, never by running an older image on rewritten files.
