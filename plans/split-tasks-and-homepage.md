# Plan: Split Tasks into Personal/Family + Add Homepage

## Context

The single tasks page mixes personal and family concerns in one list. As tasks grow, this becomes unwieldy. This change:
1. Splits tasks into two pages (personal, family) backed by separate markdown files
2. Adds a summary homepage at `/` that aggregates all sections at a glance
3. Adds a cross-list move action so tasks can be transferred between personal and family
4. Separates exploration CSS from ideas CSS (currently piggybacks on `.ideas-page`)

Goals are personal only -- family goals are intentionally unsupported. If a goal-type item ends up in `family.md`, it will not be visible in the UI.

## Requirements

1. Personal and family tasks are independent lists backed by `personal.md` and `family.md`
2. All existing task workflows (add, complete, uncomplete, delete, edit, priority, tags, progress, notes) work on both lists
3. Goals remain in `personal.md` only, served at `/goals`
4. A "move to personal/family" action lets users transfer tasks between lists
5. The homepage at `/` shows a read-only summary of all sections
6. SSE file-change refresh works for both task pages and the homepage
7. Ideas "to task" targets the personal list, with button text making the destination explicit
8. Exploration page uses its own CSS wrapper class instead of `.ideas-page`

## Success Criteria

- [ ] `go build ./cmd/dashboard` succeeds with no warnings
- [ ] `go test ./...` passes (existing and new tests)
- [ ] Linting passes
- [ ] All 6 nav links work: home, personal, family, goals, ideas, exploration
- [ ] Adding/completing/deleting tasks works on both personal and family pages
- [ ] Moving a task from personal to family (and back) works, including when a same-titled item exists in the target list
- [ ] Goals page works (add, progress, priority, delete, edit)
- [ ] Ideas "to task" creates a task in personal, button says "to personal"
- [ ] SSE refresh works on homepage, personal, family, and goals pages
- [ ] Homepage cards show correct counts and top items, links navigate correctly
- [ ] Homepage hides cards for empty sections; shows fallback message when all sections are empty
- [ ] `docker-compose.yml` and `.env.example` updated
- [ ] No hardcoded `/tracker/` routes remain in templates or Go code

---

## Phase 1: Data Layer

Config, store, migration, and data file changes. No UI or routing changes yet.

### 1.1 Config

**File:** `internal/config/config.go`

- Replace `TrackerPath` with `PersonalPath` and `FamilyPath`
- Env vars: `PERSONAL_PATH` (default `/data/personal.md`), `FAMILY_PATH` (default `/data/family.md`)
- Remove `TRACKER_PATH`
- Duplicate the skeleton-file creation in `validate()` for both paths, using `# Personal` and `# Family` as headings respectively

### 1.2 Store -- list column discriminator

**File:** `internal/tracker/store.go`

Instead of two tables with string-interpolated names, add a `list` column to the existing `tracker_items` table. This avoids `fmt.Sprintf` in SQL and needs only one migration.

- Add `listName string` field to `Store` struct
- `NewStore` accepts `(db *sql.DB, listName string)`
- `ReplaceAll` uses `DELETE FROM tracker_items WHERE list = ?` (not a blanket DELETE)
- `ReplaceAll` INSERT adds `list` as a 15th column in the INSERT statement (the current INSERT has 14 columns -- slug through images)
- `Summary` uses `WHERE list = ?`

### 1.3 DB migration

**File:** `internal/db/migrations.go`

Append a new migration (index 3) to the `migrations` slice:

```sql
ALTER TABLE tracker_items ADD COLUMN list TEXT NOT NULL DEFAULT 'personal'
```

Existing rows get `'personal'` as default, which is correct since the current `tracker.md` becomes `personal.md`.

### 1.4 WriteTracker heading

**File:** `internal/tracker/tracker.go`

`WriteTracker` currently hardcodes `# Tracker\n\n` (line 213). Change the signature to accept a heading string: `WriteTracker(path, heading string, items []Item)`. The heading is written as `# {heading}\n\n`.

### 1.5 Service -- pass heading through

**File:** `internal/tracker/service.go`

- Add a `heading` field (e.g. "Personal", "Family") to `Service`
- `NewService` accepts `(trackerPath, heading string, store *Store)`
- **Every method that calls `WriteTracker` must pass `s.heading`.** There are 14 call sites in `service.go` that currently call `WriteTracker(s.trackerPath, items)`:
  - `AddItem` (line 58)
  - `UpdateNotes` (line 85)
  - `Complete` (line 113)
  - `Uncomplete` (line 141)
  - `Delete` (line 168)
  - `UpdatePriority` (line 195)
  - `UpdateTags` (line 222)
  - `UpdateEdit` (line 251)
  - `SetProgress` (line 281)
  - `UpdateProgress` (line 311)

  All must change to `WriteTracker(s.trackerPath, s.heading, items)`. Missing any one will cause that method to revert the file heading to whatever the old default was.

### 1.6 Data migration

Rename existing `tracker.md` to `personal.md`, create empty `family.md`. This is a manual step -- document it in the commit message and `.env.example`.

### 1.7 Environment files

- `.env.example`: replace `TRACKER_PATH` with `PERSONAL_PATH` and `FAMILY_PATH`
- `docker-compose.yml`: replace `TRACKER_PATH` env var with `PERSONAL_PATH` and `FAMILY_PATH`. Note: the current docker-compose overrides the default path to `/data/db/tracker.md` because the volume mount is `./data:/data/db`. The new env vars must follow the same pattern: `PERSONAL_PATH=/data/db/personal.md` and `FAMILY_PATH=/data/db/family.md` to match the existing volume mount. Alternatively, add a volume mount at `/data` and use the config defaults.

### 1.8 Update existing tests

**File:** `test/tracker_test.go`

Tests that call `WriteTracker` directly (e.g. `TestWriteTrackerRoundTrip`, `TestWriteTrackerPreservesBody`) will break due to the new heading parameter. Update them to pass a heading string. Tests that create a `Store` must pass a `listName` to `NewStore` -- if `listName` is empty, `WHERE list = ?` matches no rows (existing rows default to `'personal'`).

### Phase 1 verification

- [ ] Builds successfully
- [ ] All existing tests pass (updated for new signatures)
- [ ] Two `Store` instances can be created on the same DB with different `listName` values
- [ ] `ReplaceAll` on one store does not affect the other store's data
- [ ] `Summary` returns counts scoped to each list

**STOP and wait for human review.**

---

## Phase 2: Handler + Routes + Templates

Parameterise the handler and rewire all routes. Both task pages and goals must work end-to-end.

### 2.1 Handler parameterisation

**File:** `internal/tracker/handler.go`

- Add `listName string` field to `Handler` struct
- `NewHandler` accepts `(svc *Service, templates map[string]*template.Template, listName string)`
- `TrackerPage` passes `ListName` and `OtherListName` to template data
- `QuickAdd` redirect changes from `/` to `"/"+h.listName` (currently hardcoded at line 187)
- `AddGoal` redirect stays at `/goals` (line 223) -- goals are personal only
- Convert `redirectBack` from a package-level function to a method on `Handler`, so it can use `h.listName` as the fallback instead of `/`. Currently `redirectBack` (line 226) is a standalone function that defaults to `/` when there is no Referer. As a method, it defaults to `"/"+h.listName`.

### 2.2 Move action

**File:** `internal/tracker/handler.go`

Add `otherSvc *Service` field to `Handler` and a `MoveToList` handler method:

- Reads the item by slug from the current service via `h.svc.Get(slug)`
- Checks for slug collision: call `h.otherSvc.Get(slug)` -- if it returns an item (no error), append a suffix to avoid duplicate slugs (e.g. `slug + "-2"`, or re-slugify with a timestamp). This prevents two items with the same slug in one list, which would make the second item unreachable by all slug-based actions.
- Adds the item to the target service via `h.otherSvc.AddItem(item)`
- Deletes it from the current service via `h.svc.Delete(slug)`
- Redirects back to the current list page

**Atomicity note:** The move is not atomic -- if add succeeds but delete fails, the item exists in both lists. This is acceptable for a personal dashboard; the user can delete the duplicate manually.

Add `onclick="return confirm('Move to {{$.OtherListName}}?')"` to the move button for consistency with the delete confirmation pattern.

Wire `otherSvc` in `main.go`:

```
personalHandler := tracker.NewHandler(personalSvc, familySvc, templates, "personal")
familyHandler   := tracker.NewHandler(familySvc, personalSvc, templates, "family")
```

### 2.3 Route changes

**File:** `cmd/dashboard/main.go`

New routes:

```
GET  /                           -> homepageHandler (Phase 4)
GET  /personal                   -> personalHandler.TrackerPage
GET  /family                     -> familyHandler.TrackerPage
GET  /goals                      -> personalHandler.GoalsPage

POST /personal/add               -> personalHandler.QuickAdd
POST /personal/add-goal          -> personalHandler.AddGoal
POST /personal/{slug}/complete   -> personalHandler.Complete
POST /personal/{slug}/uncomplete -> personalHandler.Uncomplete
POST /personal/{slug}/progress   -> personalHandler.UpdateProgress
POST /personal/{slug}/notes      -> personalHandler.UpdateNotes
POST /personal/{slug}/delete     -> personalHandler.Delete
POST /personal/{slug}/priority   -> personalHandler.UpdatePriority
POST /personal/{slug}/tags       -> personalHandler.UpdateTags
POST /personal/{slug}/edit       -> personalHandler.UpdateEdit
POST /personal/{slug}/move       -> personalHandler.MoveToList

POST /family/add                 -> familyHandler.QuickAdd
POST /family/{slug}/complete     -> familyHandler.Complete
... (same pattern as personal, excluding add-goal)
POST /family/{slug}/move         -> familyHandler.MoveToList
```

Remove all `/tracker/*` routes.

Note: `/family/add-goal` is intentionally omitted -- goals are personal only.

### 2.4 Template parameterisation -- tracker.html

**File:** `web/templates/tracker.html`

All hardcoded `/tracker/` paths must use `{{.ListName}}`. The simplest verification: grep for `/tracker/` in the template after changes -- there should be zero matches.

Add move button in the item actions area (inside the expanded detail section, alongside priority and delete):

```html
<form method="POST" action="/{{$.ListName}}/{{.Slug}}/move" class="inline-triage">
    <button type="submit" class="action-btn" onclick="return confirm('Move to {{$.OtherListName}}?')">move to {{$.OtherListName}}</button>
</form>
```

Pass `OtherListName` in template data ("family" when on personal, "personal" when on family).

Page title: pass `"Personal Tasks"` or `"Family Tasks"` as `Title`.

### 2.5 Template parameterisation -- goals.html

**File:** `web/templates/goals.html`

All hardcoded `/tracker/` paths must use `/personal/`. Goals are always personal, so these can be hardcoded rather than parameterised. Grep for `/tracker/` after changes -- zero matches.

### 2.6 Navigation

**File:** `web/templates/layout.html`

Update nav links:

```html
<a href="/">home</a>
<a href="/personal">personal</a>
<a href="/family">family</a>
<a href="/goals">goals</a>
<a href="/ideas">ideas</a>
<a href="/exploration">exploration</a>
```

The existing nav-active JS logic (lines 46-55) will work for all new paths because:
- `/` with `path === '/'` correctly highlights home
- `/personal`, `/family`, etc. use the `startsWith` branch

### 2.7 tracker.js filter key namespacing

**File:** `web/static/tracker.js`

Filter state is stored in `localStorage` under shared keys (`trackerFilterType`, `trackerFilterValue`). Filtering on the personal page would leak to the family page.

Namespace the keys by including the current page path:

```js
var pageKey = window.location.pathname.replace(/\//g, '_');
localStorage.setItem(pageKey + '_filterType', type);
localStorage.setItem(pageKey + '_filterValue', value);
```

Update `clearTrackerFilter` and the restore-on-load logic to use the same namespaced keys. Note: `clearTrackerFilter` is called from `onsubmit` handlers in both `tracker.html` and `goals.html` -- both call sites must work with the namespaced keys.

### Phase 2 verification

- [ ] Builds successfully
- [ ] `/personal` shows personal tasks, `/family` shows family tasks
- [ ] All task actions work on both pages (add, complete, delete, priority, tags, edit, notes)
- [ ] Move action transfers a task between lists
- [ ] Move action handles slug collision (item with same title exists in target list)
- [ ] Goals page works at `/goals` with all actions
- [ ] No `/tracker/` routes return 200 (they should 404)
- [ ] SSE `hx-get` on tracker.html fetches the correct page (not homepage)
- [ ] `redirectBack` without Referer redirects to the current list page, not homepage
- [ ] Filter state is isolated per page
- [ ] All tests pass

**STOP and wait for human review.**

---

## Phase 3: Watcher + Ideas + Exploration CSS

Fix the file watcher, retarget ideas-to-task, and separate exploration CSS.

### 3.1 File watcher redesign

**File:** `internal/watcher/watcher.go`

The watcher has two problems:
1. `classifyEvent` hardcodes `tracker.md` at line 100 -- neither `personal.md` nor `family.md` will match
2. Both files live in the same directory, but `dirCategories` maps directories to a single category, so it can't distinguish which file changed

Fix: add a `fileCategories map[string]string` parameter that maps specific file paths to categories.

```go
func Watch(dirCategories map[string]string, fileCategories map[string]string, broker *sse.Broker, callbacks map[string]func()) error
```

In `Watch`, ensure the parent directory of each `fileCategories` entry is watched by fsnotify: call `addRecursive(w, filepath.Dir(filePath))` for each file path. This is necessary because fsnotify watches directories, not individual files. Without this, removing the old `tracker` directory category would leave the parent directory unwatched and no events would fire for `personal.md` or `family.md`.

In `classifyEvent`, remove the hardcoded `tracker.md` check. Check `fileCategories[absPath]` before falling through to directory matching.

**File:** `cmd/dashboard/main.go`

Wire up file-level categories:

```go
fileCategories := map[string]string{
    cfg.PersonalPath: "personal",
    cfg.FamilyPath:   "family",
}
callbacks := map[string]func(){
    "personal": func() { personalSvc.Resync() },
    "family":   func() { familySvc.Resync() },
}
```

Remove the old `tracker` directory category entry from `dirCategories`.

Also address the single `pending` variable issue: rapid changes to both files within the debounce window could lose an event. Low risk for a personal dashboard, but for correctness, track pending events per category using `map[string]bool` instead of a single `string`. When the timer fires, drain all pending categories.

### 3.2 Ideas "to task" retargeting

**File:** `cmd/dashboard/main.go`

Change the `ToTaskFunc` closure to target `personalSvc.AddItem` instead of `trackerSvc.AddItem`.

**File:** `web/templates/ideas.html` (line 27, inside the `idea-card` template)

Change button text from "to task" to "to personal" so the destination is explicit.

### 3.3 Exploration CSS separation

**File:** `web/templates/exploration.html`

Change `class="ideas-page"` to `class="exploration-page"`. This class is used as the `hx-select` target for SSE swaps. Update the `hx-select` attribute to match.

Note: there are no `.ideas-page` CSS rules in the stylesheet -- the class is only used as an htmx selector. Inner classes (`.idea-card`, `.idea-header`, `.idea-title`, `.idea-actions`, `.idea-form`) are shared component styles and should stay as-is. Only the page wrapper class changes.

**File:** `web/templates/exploration-detail.html`

Check if it uses `.ideas-page` as a wrapper. If not, no change needed.

### Phase 3 verification

- [ ] SSE refresh works: editing `personal.md` directly refreshes the personal page and homepage
- [ ] SSE refresh works: editing `family.md` directly refreshes the family page and homepage
- [ ] Ideas "to task" creates a task in personal list
- [ ] Button text says "to personal"
- [ ] Exploration page renders correctly with updated wrapper class
- [ ] Exploration SSE refresh still works after class rename
- [ ] All tests pass

**STOP and wait for human review.**

---

## Phase 4: Homepage

New homepage at `/` with read-only summary cards.

### 4.1 Homepage handler

**File:** `cmd/dashboard/main.go` (inline closure, not a separate package)

Aggregates data from all services. Use `Summary()` for counts (it queries the SQLite cache, avoiding full markdown file parses). Still need `List()` for the top-N item previews -- `Summary()` only saves the count computation, not the item fetch.

Template data structure:

```go
data := map[string]any{
    "Title":              "Home",
    "PersonalTasks":      topTasks(personalItems, 5),
    "PersonalTaskCount":  personalSummary.OpenTasks,
    "FamilyTasks":        topTasks(familyItems, 5),
    "FamilyTaskCount":    familySummary.OpenTasks,
    "Goals":              activeGoals(personalItems),
    "UntriagedIdeas":     filterUntriaged(ideas, 3),
    "UntriagedCount":     countUntriaged(ideas),
    "RecentExplorations": recent(explorations, 3),
    "ExplorationCount":   len(explorations),
}
```

Helper functions (`topTasks`, `activeGoals`, `filterUntriaged`, `recent`) are simple filter/slice operations defined locally in `main.go`.

### 4.2 Homepage template

**File:** `web/templates/homepage.html` (new)

Single-column card stack, read-only. Five cards:

1. **Personal Tasks** -- count badge, top 5 by priority with left-border colour, "+N more" link to `/personal`
2. **Family Tasks** -- same layout, link to `/family`
3. **Goals** -- count badge, each goal with inline progress bar (8px height), link to `/goals`
4. **Ideas** -- untriaged count badge, latest 3 titles as links to `/ideas/{slug}`
5. **Exploration** -- entry count, latest 3 with dates, links to `/exploration/{slug}`

Empty state: hide the card entirely when a section has zero items. If ALL sections are empty (fresh install), show a single fallback message: "Nothing here yet. Use the nav above to get started."

SSE refresh: wrap in div with `hx-get="/" hx-select=".homepage-page" hx-target="this" hx-swap="outerHTML" hx-trigger="sse:file-changed"`.

Task item links use `/personal#{{.Slug}}` and `/family#{{.Slug}}` (not `#item-{{.Slug}}`). The existing `tracker.js` hash-expand logic does `getElementById('item-' + hash)`, so the hash must be just the slug -- the JS prepends `item-`.

Each card header links to the full section page. "view all" link at bottom of each card.

See `docs/claude_dashboard_homepage_design.md` for CSS patterns and visual reference (note: that doc's template code predates the personal/family split -- follow this plan for link URLs and hash format).

### 4.3 Homepage CSS

**File:** `web/static/theme.css`

Minimal additions (~50 lines), all using existing CSS variables:
- `.homepage-page` -- page wrapper
- `.homepage-card` -- card container
- `.homepage-card-header` -- flex row with title + count badge
- `.homepage-task` -- compact single-line task with priority left-border
- `.homepage-goal` -- goal row with inline thinner progress bar
- `.homepage-idea`, `.homepage-exploration` -- compact entry rows

### 4.4 Route + template registration

**File:** `cmd/dashboard/main.go`

- `GET /` -> homepage handler
- Add `"homepage.html"` to the `pages` slice in `parseTemplates()`

### Phase 4 verification

- [ ] Homepage renders at `/`
- [ ] Each card shows correct counts and top items
- [ ] Cards for empty sections are hidden
- [ ] All-empty homepage shows fallback message
- [ ] All card links navigate to correct pages
- [ ] Task links use correct hash format (`/personal#slug`, not `/personal#item-slug`)
- [ ] Clicking a homepage task link expands the correct item on the task page
- [ ] SSE refresh works on homepage when any backing file changes
- [ ] All tests pass

**STOP and wait for human review.**

---

## Phase 5: Tests + Final Review

### 5.1 New tests

- Store isolation: two `Store` instances with different `listName` values on the same DB; `ReplaceAll` on one does not affect the other; `Summary` returns scoped counts
- Handler redirect: call handler methods without `Referer` header, verify redirect goes to `/{listName}` not `/`
- Move action: verify item appears in target list and is removed from source
- Move action slug collision: verify moving an item when the target list has a same-titled item does not create duplicate slugs
- Watcher: verify `classifyEvent` correctly maps `personal.md` and `family.md` to their respective categories

### 5.2 Final verification

- [ ] `go build ./cmd/dashboard` -- no warnings
- [ ] `go test ./...` -- all pass
- [ ] Lint clean
- [ ] Manual walkthrough: all 6 nav links, all task actions on both lists, move between lists, goals, ideas-to-task, exploration, homepage
- [ ] SSE works on all pages
- [ ] No debug statements or `console.log` remain
- [ ] No `/tracker/` references remain in templates or Go code
- [ ] `docker-compose.yml` and `.env.example` correct
- [ ] No hardcoded `tracker.md` or `# Tracker` strings remain

**STOP and wait for human review.**

---

## Files to modify

| File | Change |
|------|--------|
| `internal/config/config.go` | Replace `TrackerPath` with `PersonalPath` + `FamilyPath`, dual skeleton creation |
| `internal/db/migrations.go` | Add migration: `ALTER TABLE tracker_items ADD COLUMN list TEXT` |
| `internal/tracker/store.go` | Add `listName` field, add `list` as 15th INSERT column, scope all SQL with `WHERE list = ?` |
| `internal/tracker/tracker.go` | Add `heading` param to `WriteTracker` |
| `internal/tracker/service.go` | Add `heading` field, update all 14 `WriteTracker` call sites to pass `s.heading` |
| `internal/tracker/handler.go` | Add `listName` + `otherSvc`, convert `redirectBack` to method, add `MoveToList` with slug dedup |
| `internal/watcher/watcher.go` | Add `fileCategories` param, watch parent dirs, remove hardcoded `tracker.md`, per-category pending |
| `cmd/dashboard/main.go` | Two instances, homepage handler, new routes, watcher rewiring |
| `web/templates/layout.html` | 6 nav links |
| `web/templates/tracker.html` | Parameterise all form actions with `{{.ListName}}`, add move button with confirm |
| `web/templates/goals.html` | Replace `/tracker/` with `/personal/` in all form actions |
| `web/templates/exploration.html` | Change `ideas-page` wrapper to `exploration-page` |
| `web/static/theme.css` | Add homepage card styles |
| `web/static/tracker.js` | Namespace localStorage filter keys by page path, update `clearTrackerFilter` |
| `web/templates/homepage.html` | **New** -- homepage template with `#{{.Slug}}` hash links |
| `web/templates/ideas.html` | Change "to task" button text to "to personal" |
| `docker-compose.yml` | Replace `TRACKER_PATH` with `PERSONAL_PATH=/data/db/personal.md` + `FAMILY_PATH=/data/db/family.md` |
| `.env.example` | Replace `TRACKER_PATH` with `PERSONAL_PATH` + `FAMILY_PATH` |
| `test/tracker_test.go` | Update `WriteTracker` calls for new heading param, pass `listName` to `NewStore` |
