# Backlog

Plan 2 (exercise module, SQLite migration hook, a `Plannable` capability) and Plan 3 (stable item IDs, quick capture, recurring tasks, MCP tooling) hold the larger work; this file lists smaller follow-ups.

## House

### Test house plan rows
Reorder and calendar drag post `list=house`, and `home.listService` maps it to house projects, so it works end to end, but no test covers `list=house`. A Go handler test next to `test/fragment_test.go` is enough.

### itemToAPI Budget/Actual/Status
`itemToAPI` in `tracker/api.go` does not expose Budget, Actual or Status, and the tracker API refuses `list=house`. No API exposes them; `planItemsToAPI` returns only slug, title, priority, done, planned, tags and list.

### MoveToList for house projects
Moving items between personal/family and house is not wired. `tracker.Handler.MoveToList` is bound to a fixed pair of lists, so it needs a target parameter; moving in must set `Status = "todo"` (as `toTask` does); `house.html` needs move controls.

## Security

### Tighten CSP `script-src`
`script-src` still allows `'unsafe-inline'`. Module templates use delegated `data-action` handlers, but inline `on*` handlers remain in `layout.html`, `homepage.html`, `login.html` and `admin-users.html`, and inline `<script>` blocks in `layout.html`, `login.html`, `account.html`, `admin-password.html` and the `appearance-script` partial. The appearance script must run before first paint, so give it a CSP hash or load it as a blocking script in `<head>`. `style-src 'unsafe-inline'` has to stay: templates use `style=""` and the seasonal colour is an injected `<style>` block.

### Ideas status CSS class injection
`ideas.Status` is parsed without an allowlist, and `idea.html` renders it into `class="badge-{{.Idea.Status}}"`. It is reachable from input, not just hand edits: a title typed as `Foo [status: x badge-tag]` (web form or `APIAddIdea`) is written as-is and the parser's `statusRe` matches it on re-parse. An unknown status also hides the idea, because `/ideas` renders only the four known groups. Low severity (`html/template` prevents XSS). Fix: allowlist at parse time with a fallback to `untriaged`, and strip inline metadata from idea titles.

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

### Drop the `tracker_items` table
Unused since the SQLite mirror was removed. `internal/db/migrations.go` already takes versioned statements, so a `DROP TABLE` can go there now; the same change must remove `DELETE FROM tracker_items` from `auth.DeleteUser`, or user deletion fails.

### Watcher has no stop hook
`watcher.Watch` takes no context, so shutdown cannot stop it and each router-building test leaks one watcher.

### Select mode is tied to two page classes
`tracker.js` `toggleSelectMode`/`exitSelectMode` look up `.tracker-page, .ideas-page`. A new module cannot get select mode without reusing those classes.

## Known limitations

- **Bulk trash has no undo.** There is no bulk restore route, so bulk trash keeps its confirm dialog.
- **Unsubmitted `<select>` changes reset on a live refresh.** Text fields and checkboxes are kept; selects are not.
- **SSE events go to every client.** No per-user routing; revisit if multi-user returns.
- **Admin user deletion:** the watcher recreates the deleted user's directory. Admin code is frozen.
- **Commentary on shared lists** written through the API (as user 1) is not shown to other users.
- **Duplicate slugs.** `AddItem` re-slugifies and never checks for duplicates, so moving an item or adding one with an existing title leaves two items with one slug, and slug-addressed actions hit the first. `MoveToList`'s `-<unix>` suffix is dead for the same reason. Fixed by Plan 3's stable IDs, as is renamed items losing their expanded state.
- **No-auth then auth database:** the `local@localhost` placeholder keeps id 1, so the bootstrapped admin gets a later id while the API and user-1 paths still point at the placeholder. Dev databases only.
- **Accepted in Plan 1:** the failed-bearer limiter cannot slow guessing (a valid token always passes); the login account delay is a per-request sleep; `sm.IdleTimeout` rewrites the session row and sends Set-Cookie on every request; a user whose services are first created by a watcher event misses that first broadcast.
