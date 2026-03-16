# Plan: Split Tasks into Personal/Family + Add Homepage

## Context

The single tasks page will become unwieldy as tasks grow across personal and family concerns. This change splits tasks into two separate pages (personal, family) backed by separate markdown files, and adds a summary homepage at `/` that aggregates all sections at a glance.

---

## Part 1: Split Tasks into Personal and Family

### 1.1 Config changes

**File:** `internal/config/config.go`

- Replace single `TrackerPath` with `PersonalPath` and `FamilyPath`
- Env vars: `PERSONAL_PATH` (default `/data/personal.md`), `FAMILY_PATH` (default `/data/family.md`)
- Validation creates skeleton files for both (reuse existing pattern)
- Remove old `TRACKER_PATH` env var

### 1.2 Two service instances

**File:** `internal/tracker/service.go` -- no changes needed

The `Service` struct already takes a `trackerPath` string. Just instantiate two:

```go
personalSvc := tracker.NewService(cfg.PersonalPath, personalStore)
familySvc   := tracker.NewService(cfg.FamilyPath, familyStore)
```

**File:** `internal/tracker/store.go` -- needs table name parameterisation or two separate stores. Simplest: use a single DB with a `list` column, or create two `Store` instances with a table prefix. Given the store is a cache rebuilt from file, the cleanest approach is to add a `listName` field to `Store` and prefix the table name (e.g. `personal_items`, `family_items`).

### 1.3 Handler changes

**File:** `internal/tracker/handler.go`

- Create two `Handler` instances (one per service), or a single handler that takes both services and routes by path prefix
- Recommended: two handler instances, each knowing its own `listName` for redirects and template data

```go
personalHandler := tracker.NewHandler(personalSvc, templates, "personal")
familyHandler   := tracker.NewHandler(familySvc, templates, "family")
```

- Add `listName` field to `Handler` struct
- `TrackerPage` passes `ListName` to template data (for form actions and redirect targets)
- `QuickAdd` redirect changes from hardcoded `/` to `/{listName}`
- `redirectBack` already uses `Referer` header, so most actions work as-is

### 1.4 Route changes

**File:** `cmd/dashboard/main.go`

```
GET  /personal           -> personalHandler.TrackerPage
GET  /family             -> familyHandler.TrackerPage
POST /personal/add       -> personalHandler.QuickAdd
POST /family/add         -> familyHandler.QuickAdd
POST /personal/{slug}/*  -> personalHandler.*
POST /family/{slug}/*    -> familyHandler.*
```

Goals stay on `/goals`. They remain in `personal.md` for now -- they're personal goals. Can split later if needed.

### 1.5 Template changes

**File:** `web/templates/tracker.html`

- Form action changes from `/tracker/add` to `/{{.ListName}}/add`
- Item action URLs change from `/tracker/{slug}/*` to `/{{.ListName}}/{slug}/*`
- Page title changes to use `{{.Title}}` (already does this)

### 1.6 Navigation

**File:** `web/templates/layout.html`

Nav links update:

```
home | personal | family | goals | ideas | exploration
```

### 1.7 File watcher

**File:** `cmd/dashboard/main.go`

Watch directories for both `personal.md` and `family.md` paths, each with its own resync callback.

### 1.8 Ideas "to task" integration

**File:** `internal/ideas/handler.go`

The `ToTask` function currently adds to the single tracker. It needs to target a specific list -- default to personal (most likely destination). Could add a form field later to choose.

### 1.9 Data migration

Rename existing `tracker.md` to `personal.md`, create empty `family.md`. User can manually move items between them.

---

## Part 2: Homepage

### 2.1 Homepage handler

**File:** `cmd/dashboard/main.go` (or new `internal/homepage/handler.go`)

A lightweight handler that aggregates data from all services:

```go
func HomePage(w http.ResponseWriter, r *http.Request) {
    personalItems, _ := personalSvc.List()
    familyItems, _ := familySvc.List()
    ideas, _ := ideaSvc.List()
    explorations, _ := explorationSvc.List()

    data := map[string]any{
        "Title":              "Home",
        "PersonalTasks":      topTasks(personalItems, 5),
        "PersonalTaskCount":  countOpen(personalItems),
        "FamilyTasks":        topTasks(familyItems, 5),
        "FamilyTaskCount":    countOpen(familyItems),
        "Goals":              activeGoals(personalItems),
        "UntriagedIdeas":     filterUntriaged(ideas, 3),
        "UntriagedCount":     countUntriaged(ideas),
        "RecentExplorations": recent(explorations, 3),
        "ExplorationCount":   len(explorations),
    }
}
```

Helper functions (`topTasks`, `countOpen`, `activeGoals`, etc.) are simple filters/slicers -- define locally or as package-level helpers.

### 2.2 Homepage template

**File:** `web/templates/homepage.html` (new)

Single-column card stack, read-only (no actions, no quick-add). Five cards:

1. **Personal Tasks** -- count badge, top 5 by priority, priority colour left-border, "+N more" link
2. **Family Tasks** -- same layout
3. **Goals** -- count badge, each goal with inline progress bar (reuse existing `.progress-bar` at 8px height), fraction text
4. **Ideas** -- untriaged count badge, latest 3 titles as links
5. **Exploration** -- entry count, latest 3 with dates

Each card header links to the full section page. "view all" link at bottom of each card.

SSE refresh: wrap in div with `hx-get="/" hx-trigger="sse:file-changed"` pattern.

See `docs/claude_dashboard_homepage_design.md` for detailed template HTML, CSS, and design rationale.

### 2.3 Homepage CSS

**File:** `web/static/theme.css`

Minimal additions (~50 lines), all using existing CSS variables:
- `.homepage-card` -- card container (bg-card, border, padding)
- `.homepage-card-header` -- flex row with title + count badge
- `.homepage-task` -- compact single-line task with priority left-border
- `.homepage-goal` -- goal row with inline thinner progress bar
- `.homepage-idea`, `.homepage-exploration` -- compact entry rows

### 2.4 Route

**File:** `cmd/dashboard/main.go`

```
GET / -> homepageHandler
```

### 2.5 Template registration

**File:** `cmd/dashboard/main.go`

Add `"homepage.html"` to the `pages` slice in `parseTemplates()`.

---

## Files to modify

| File | Change |
|------|--------|
| `internal/config/config.go` | Replace `TrackerPath` with `PersonalPath` + `FamilyPath` |
| `internal/tracker/store.go` | Add `listName` param for table name isolation |
| `internal/tracker/handler.go` | Add `listName` field, parameterise routes in redirects |
| `cmd/dashboard/main.go` | Two tracker instances, homepage handler, updated routes |
| `web/templates/layout.html` | Updated nav links |
| `web/templates/tracker.html` | Parameterise form actions with `{{.ListName}}` |
| `web/static/theme.css` | Add homepage card styles |
| `web/templates/homepage.html` | **New** -- homepage template |
| `internal/ideas/handler.go` | Update `ToTask` to target personal service |
| `.env.example` | Update env var names |

## Files to reuse (no changes)

| File | What to reuse |
|------|---------------|
| `internal/tracker/service.go` | Instantiate twice, no code changes |
| `internal/tracker/tracker.go` | Parsing/serialisation unchanged |
| `web/static/tracker.js` | Client-side filtering works as-is |

## Verification

1. **Build:** `go build ./cmd/dashboard`
2. **Run:** Start server, verify all 6 nav links work (home, personal, family, goals, ideas, exploration)
3. **Homepage:** Check each card shows correct counts and top items, links navigate to correct pages
4. **Personal tasks:** Add/complete/delete tasks, verify they persist to `personal.md`
5. **Family tasks:** Same operations, verify they persist to `family.md`
6. **Goals:** Verify goals still work on `/goals` page
7. **Ideas to task:** Convert an idea to task, verify it appears in personal tasks
8. **SSE:** Edit a markdown file directly, verify homepage and section pages auto-refresh
9. **Tests:** `go test ./...`
