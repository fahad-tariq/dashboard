# Dashboard Homepage Design Patterns

Research for building a summary/overview homepage that aggregates: Tasks, Goals, Ideas, and Exploration sections. Context: Go server-rendered HTML, htmx, Catppuccin theme, monospace font, no CSS framework.

---

## 1. Layout Patterns

### Recommended: Single-Column Card Stack

Given the existing codebase uses a single-column `max-width: 1100px` container with monospace font, the most natural fit is a **single-column stack of summary cards** -- one card per section, vertically ordered by priority.

This avoids fighting the existing layout and keeps the monospace aesthetic coherent. Multi-column grids work well for analytics dashboards but feel forced for a personal task-oriented dashboard with heterogeneous content types.

**Ordering (top to bottom):**

1. **Tasks** -- highest urgency, daily interaction
2. **Goals** -- progress tracking, weekly check-in
3. **Ideas** -- triage queue, periodic review
4. **Exploration** -- passive discovery, lowest urgency

**Why single-column over grid:**
- Monospace text does not compress well into narrow grid cells
- The existing pages are all single-column; consistency matters
- Mobile-first: a single column is already responsive
- F-pattern scanning works naturally with vertical cards
- Each section has different content shapes (lists vs progress bars vs counts)

**Alternative: 2-column grid for Tasks + Goals side-by-side**

If you want to use horizontal space on wide screens, a `2x2` grid of the top sections works well. CSS Grid with `auto-fit` handles the responsive collapse:

```css
.homepage-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(480px, 1fr));
    gap: 1rem;
}
```

This gives two columns on desktop (1100px container) and collapses to one on mobile. Each card fills its cell. The trade-off is that card heights may differ, creating visual unevenness -- acceptable for 2 columns, messy for 3+.

### Information Hierarchy Within Each Card

Follow the **title, count, content, link** pattern:

```
+--------------------------------------------+
| Section Title              count badge     |
|--------------------------------------------|
| [content: top items / progress / counts]   |
|                                            |
| view all ->                                |
+--------------------------------------------+
```

---

## 2. Information Density

### What to Show Per Section

The 5-second rule: a user should grasp the state of every section within 5 seconds of loading the page. This means showing enough to answer "do I need to act on this?" without showing so much that it becomes the full section page.

**Tasks card:**
- Count of open tasks (you already have this)
- Top 3-5 high-priority tasks as a compact list (title only, priority colour on left border)
- "+N more" link if truncated
- No expand/collapse, no body text, no actions -- just titles

**Goals card:**
- Count of active goals
- Each goal as a single row: title + inline progress bar + fraction text
- Limit to 5 goals max; if more, show top by priority then "+N more"
- Progress bars are the star here -- they give instant visual state

**Ideas card:**
- Count of untriaged ideas (this is the actionable number)
- Count of parked ideas (secondary)
- Latest 3 untriaged idea titles as links
- No triage actions on the homepage -- force navigation to the full page

**Exploration card:**
- Count of entries
- Latest 3 entries as title + date
- This section gets the least space; it is informational, not actionable

### Density Guidelines

| Section     | Metric shown     | Items listed | Actions available |
|-------------|------------------|--------------|-------------------|
| Tasks       | N open, N done   | Top 3-5      | None (view all)   |
| Goals       | N active         | All (max ~5) | None (view all)   |
| Ideas       | N untriaged      | Latest 3     | None (view all)   |
| Exploration | N entries        | Latest 3     | None (view all)   |

The key principle: **the homepage is read-only**. No inline editing, no triage, no quick-add on the summary page. Each card is a signpost, not a workstation. Quick-add and actions belong on the dedicated section pages.

This is a deliberate constraint. Putting actions on the homepage creates decision fatigue and duplicates UI that already exists on each section page.

---

## 3. Visual Patterns

### Summary Cards

Reuse the existing `.summary-stats` and card styling. A summary card is structurally:

```html
<section class="homepage-card">
    <div class="homepage-card-header">
        <a href="/goals" class="homepage-card-title">Goals</a>
        <span class="stat-value">4</span>
    </div>
    <div class="homepage-card-body">
        <!-- section-specific content -->
    </div>
    <a href="/goals" class="homepage-card-link">view all</a>
</section>
```

CSS that fits the existing theme:

```css
.homepage-card {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0.75rem 1rem;
    margin-bottom: 0.75rem;
}

.homepage-card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.5rem;
    padding-bottom: 0.35rem;
    border-bottom: 1px solid var(--border);
}

.homepage-card-title {
    font-weight: 600;
    font-size: 1rem;
    color: var(--fg);
    text-decoration: none;
}
.homepage-card-title:hover {
    color: var(--accent);
}

.homepage-card-link {
    display: block;
    margin-top: 0.5rem;
    font-size: 0.8rem;
    color: var(--fg-dim);
}
.homepage-card-link:hover {
    color: var(--accent);
}
```

### Compact Task List

For the tasks summary, a stripped-down version of the existing `.tracker-item`:

```css
.homepage-task {
    padding: 0.25rem 0;
    border-left: 3px solid transparent;
    padding-left: 0.5rem;
    font-size: 0.85rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}
.homepage-task[data-priority="high"] { border-left-color: var(--priority-high); }
.homepage-task[data-priority="medium"] { border-left-color: var(--priority-medium); }
.homepage-task[data-priority="low"] { border-left-color: var(--priority-low); }
```

### Progress Indicators for Goals

Two options that work well with the existing theme:

**Option A: Inline progress bar (reuse existing)**

Already exists in the codebase as `.progress-bar` / `.progress-fill`. On the homepage, make it thinner:

```css
.homepage-progress .progress-bar {
    height: 8px;
}
```

**Option B: Progress ring using conic-gradient (pure CSS)**

Small circular indicators beside each goal title. No JS required:

```css
.progress-ring {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: conic-gradient(
        var(--green) calc(var(--pct) * 1%),
        var(--surface0) 0
    );
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
}
.progress-ring::after {
    content: "";
    width: 22px;
    height: 22px;
    border-radius: 50%;
    background: var(--bg-card);
}
```

Usage in template:

```html
<div class="progress-ring" style="--pct: {{percentage .Current .Target}}"></div>
```

The inline progress bar (Option A) is the safer choice -- it is already in the codebase, proven to work with the theme, and fits the horizontal layout better. Progress rings look good on analytics dashboards but feel out of place in a monospace text-heavy UI.

**Recommendation: Use inline progress bars for goals. They are already styled and tested.**

### Count Badges

For the header count, reuse `.stat-value`:

```html
<span class="badge" style="color: var(--accent); border-color: var(--accent);">4 active</span>
```

Or simpler -- just the number with the existing stat styling:

```html
<span class="stat-value">4</span> <span class="meta-dim">active</span>
```

---

## 4. Interaction Patterns

### Navigation Model

Each card header and "view all" link navigates to the dedicated section page. This is standard dashboard behaviour and matches how Notion, Things 3, and Todoist handle their overview screens.

```html
<a href="/goals" class="homepage-card-title">Goals</a>
```

Individual items within cards should also be clickable where it makes sense:
- Task titles: link to the full tasks page with an anchor (`/#item-slug`)
- Goal titles: link to `/goals#item-slug`
- Idea titles: link to `/ideas/slug` (detail page already exists)
- Exploration titles: link to `/exploration/slug` (detail page already exists)

### No Quick Actions on Homepage

Do not add complete buttons, triage actions, or quick-add forms to the homepage summary. Reasons:

1. **Cognitive load**: The homepage purpose is orientation, not execution
2. **Duplication**: These actions exist on the section pages
3. **Clutter**: Actions on every row destroy the "at a glance" quality
4. **htmx complexity**: Each action would need its own endpoint and would require refreshing the homepage card state, which means more server logic for a page that should be simple

### htmx: Lazy Loading Cards

If the homepage aggregates data from multiple services (tracker, ideas, exploration), loading them all synchronously on page load could be slow. Use htmx to lazy-load each card independently:

```html
<section class="homepage-card"
         hx-get="/homepage/tasks"
         hx-trigger="load"
         hx-swap="innerHTML">
    <div class="homepage-card-header">
        <span class="homepage-card-title">Tasks</span>
    </div>
    <div class="meta-dim">Loading...</div>
</section>
```

Each partial endpoint returns just the card body HTML. This keeps the initial page load fast and lets each section load independently.

However, for a personal dashboard with markdown-file backends, the data is small enough that **synchronous rendering is fine**. Lazy loading adds complexity for negligible benefit here. Use it only if you notice the homepage becoming slow.

**Recommendation: Render the full homepage in one server response. Use SSE (already wired up) to refresh if the underlying data changes.**

### SSE Integration

The existing `sse:file-changed` trigger can refresh the homepage when any backing file changes:

```html
<div class="homepage-page"
     hx-get="/homepage"
     hx-select=".homepage-page"
     hx-target="this"
     hx-swap="outerHTML"
     hx-trigger="sse:file-changed">
```

This matches the existing pattern used on every other page.

---

## 5. Reference Designs

### Things 3 -- "Today" View

Things 3's "Today" view is the closest analogue to what you are building. It aggregates items from all areas/projects into a single flat list, grouped by area. Key principles:

- **Minimalist density**: Each item is one line -- title, tags, deadline. Nothing else.
- **Grouping by source**: Items are visually grouped under area headings (e.g. "Personal", "Work")
- **No chrome**: No card borders, no backgrounds, just subtle separators
- **Calm palette**: Muted colours, bold used sparingly for headings only

Takeaway: Things 3 succeeds by showing less. A single-line item with a coloured dot (priority) conveys enough.

### Todoist -- "Today" + "Upcoming"

Todoist's overview is noisier than Things 3 but more information-rich:

- Task rows show labels, project, due date, priority flag
- Sections are collapsible
- Quick-add is pinned to the top (floating button)
- Overdue items get a red section header

Takeaway: Todoist shows that per-item metadata (tags, project, date) works on an overview page **if** rows stay single-line. Once rows wrap to two lines (which happens when you add descriptions), the overview feels bloated.

### Notion -- Personal Dashboard Templates

Notion dashboards typically use a 2-3 column layout with:

- A "Quick Capture" widget (inline database view, 3-5 rows)
- Calendar/upcoming events widget
- Habit tracker with checkboxes
- Goals with progress bars
- Bookmarked pages / quick links

Takeaway: Notion dashboards work because each widget is a constrained database view -- not the full database. The constraint (e.g. "show 5 most recent") is what makes the homepage scannable.

### Home Assistant -- Dashboard Cards

Home Assistant uses a card-based grid where each card is a self-contained widget. Cards have a title, a primary value (e.g. temperature, state), and optional detail. The layout is a responsive grid that reflows on resize.

Takeaway: Works well for homogeneous data (sensors, switches). Less applicable here because your sections are heterogeneous -- but the "card with a single primary metric" pattern is useful for the summary counts.

---

## 6. Recommended Implementation

### Template Structure

Create a new template `homepage.html`:

```html
{{define "content"}}
<div class="homepage-page"
     hx-get="/homepage"
     hx-select=".homepage-page"
     hx-target="this"
     hx-swap="outerHTML"
     hx-trigger="sse:file-changed">

<!-- Tasks -->
<section class="homepage-card">
    <div class="homepage-card-header">
        <a href="/" class="homepage-card-title">Tasks</a>
        <span><span class="stat-value">{{.TaskCount}}</span> <span class="meta-dim">open</span></span>
    </div>
    <div class="homepage-card-body">
        {{range .TopTasks}}
        <div class="homepage-task" data-priority="{{.Priority}}">
            <a href="/#item-{{.Slug}}">{{.Title}}</a>
        </div>
        {{end}}
        {{if gt .TaskCount (len .TopTasks)}}
        <a href="/" class="homepage-card-link">+{{subtract .TaskCount (len .TopTasks)}} more</a>
        {{end}}
    </div>
</section>

<!-- Goals -->
<section class="homepage-card">
    <div class="homepage-card-header">
        <a href="/goals" class="homepage-card-title">Goals</a>
        <span><span class="stat-value">{{len .Goals}}</span> <span class="meta-dim">active</span></span>
    </div>
    <div class="homepage-card-body">
        {{range .Goals}}
        <div class="homepage-goal">
            <span class="homepage-goal-title">{{.Title}}</span>
            {{if gt .Target 0.0}}
            <div class="homepage-progress">
                <div class="progress-bar">
                    <div class="progress-fill" style="width: {{percentage .Current .Target}}%"></div>
                </div>
                <span class="progress-text">{{formatNum .Current}}/{{formatNum .Target}}{{if .Unit}} {{.Unit}}{{end}}</span>
            </div>
            {{end}}
        </div>
        {{end}}
    </div>
    <a href="/goals" class="homepage-card-link">view all</a>
</section>

<!-- Ideas -->
<section class="homepage-card">
    <div class="homepage-card-header">
        <a href="/ideas" class="homepage-card-title">Ideas</a>
        <span><span class="stat-value">{{.UntriagedCount}}</span> <span class="meta-dim">untriaged</span></span>
    </div>
    <div class="homepage-card-body">
        {{range .RecentIdeas}}
        <div class="homepage-idea">
            <a href="/ideas/{{.Slug}}">{{.Title}}</a>
        </div>
        {{end}}
    </div>
    <a href="/ideas" class="homepage-card-link">view all</a>
</section>

<!-- Exploration -->
<section class="homepage-card">
    <div class="homepage-card-header">
        <a href="/exploration" class="homepage-card-title">Exploration</a>
        <span><span class="stat-value">{{len .RecentExplorations}}</span> <span class="meta-dim">entries</span></span>
    </div>
    <div class="homepage-card-body">
        {{range .RecentExplorations}}
        <div class="homepage-exploration">
            <a href="/exploration/{{.Slug}}">{{.Title}}</a>
            {{if .Date}}<span class="meta-dim">{{.Date}}</span>{{end}}
        </div>
        {{end}}
    </div>
    <a href="/exploration" class="homepage-card-link">view all</a>
</section>

</div>
{{end}}
```

### Handler Data Structure

The homepage handler assembles data from all services:

```go
type HomepageData struct {
    Title              string
    TaskCount          int
    TopTasks           []tracker.Item  // top 5 by priority
    Goals              []tracker.Item  // all active goals
    UntriagedCount     int
    RecentIdeas        []ideas.Idea    // latest 3 untriaged
    RecentExplorations []exploration.Exploration // latest 3
}
```

### Routing

Currently `/` serves the tasks page. Two options:

**Option A: New `/homepage` route, keep `/` as tasks**
- Add `GET /homepage` for the summary page
- Add a "home" link in nav
- Least disruptive change

**Option B: Move tasks to `/tasks`, make `/` the homepage**
- More conventional (homepage at root)
- Requires updating the nav links and any bookmarks
- The nav would become: `home | tasks | goals | ideas | exploration`

Option B is cleaner long-term. The homepage is the entry point; tasks are one section among several.

### CSS Additions

Minimal new CSS needed, all building on existing variables:

```css
/* Homepage cards */
.homepage-card {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0.75rem 1rem;
    margin-bottom: 0.75rem;
}
.homepage-card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.5rem;
    padding-bottom: 0.35rem;
    border-bottom: 1px solid var(--border);
}
.homepage-card-title {
    font-weight: 600;
    color: var(--fg);
    text-decoration: none;
}
.homepage-card-title:hover { color: var(--accent); }
.homepage-card-link {
    display: block;
    margin-top: 0.5rem;
    font-size: 0.8rem;
    color: var(--fg-dim);
}
.homepage-card-link:hover { color: var(--accent); }

/* Compact task list */
.homepage-task {
    padding: 0.2rem 0;
    padding-left: 0.5rem;
    border-left: 3px solid transparent;
    font-size: 0.85rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.homepage-task a { color: var(--fg); }
.homepage-task a:hover { color: var(--accent); }
.homepage-task[data-priority="high"] { border-left-color: var(--priority-high); }
.homepage-task[data-priority="medium"] { border-left-color: var(--priority-medium); }
.homepage-task[data-priority="low"] { border-left-color: var(--priority-low); }

/* Goal rows with inline progress */
.homepage-goal {
    padding: 0.25rem 0;
    font-size: 0.85rem;
}
.homepage-goal-title {
    font-weight: 600;
    color: var(--fg);
}
.homepage-progress {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.15rem;
}
.homepage-progress .progress-bar { height: 8px; }

/* Idea / exploration rows */
.homepage-idea, .homepage-exploration {
    padding: 0.2rem 0;
    font-size: 0.85rem;
    display: flex;
    justify-content: space-between;
    gap: 0.5rem;
}
```

---

## 7. Anti-Patterns to Avoid

1. **Kitchen sink homepage**: Showing full item details, edit forms, triage actions. The homepage becomes a worse version of each section page.

2. **Dashboard widgets that duplicate section pages**: If clicking "view all" shows the same content in a slightly different layout, one of them is redundant.

3. **Progress rings for text-heavy UIs**: Circular progress indicators work in graphical dashboards (Fitbit, Apple Health) but clash with monospace text layouts. Stick with linear progress bars.

4. **Multi-column grids below 900px**: Columns that are too narrow truncate monospace text aggressively, making items unreadable.

5. **Lazy loading small datasets**: htmx lazy loading each card adds 4 HTTP requests for data that fits in a single response. Synchronous render is simpler and faster for personal-scale data.

6. **Quick-add on the homepage**: Adds a form that competes with the summary content for attention. Keep quick-add on the section pages where context is clear.

---

## Sources

- [Dashboard Design UX Patterns - Pencil & Paper](https://www.pencilandpaper.io/articles/ux-pattern-analysis-data-dashboards)
- [Dashboard Design Best Practices - Justinmind](https://www.justinmind.com/ui-design/dashboard-design-best-practices-ux)
- [Dashboard Design Patterns (academic)](https://dashboarddesignpatterns.github.io/)
- [Circle Progress Bar Pure CSS](https://nikitahl.com/circle-progress-bar-css)
- [htmx Examples](https://htmx.org/examples/)
- [Making a Dashboard with htmx](https://ggoggam.github.io/blog/dashboard)
- [Home Assistant Dashboards](https://www.home-assistant.io/dashboards/)
- [Best Notion Dashboard Templates](https://www.notioneverything.com/blog/notion-dashboard-templates)
- [Things 3 vs Todoist Comparison](https://offlight.work/blog/things3-vs-todoist)
- [Building an Admin Dashboard Layout with CSS](https://webdesign.tutsplus.com/building-an-admin-dashboard-layout-with-css-and-a-touch-of-javascript--cms-33964t)
- [Framework-Free Dashboard using CSS Grid](https://medium.com/mtholla/build-a-framework-free-dashboard-using-css-grid-and-flexbox-53d81c4aee68)
