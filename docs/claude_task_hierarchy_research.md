# Task Hierarchy and Subtask Patterns: Research for Flat-File Markdown Task Trackers

Research conducted 2026-03-28. Focused on personal productivity tools, not enterprise project management.

---

## 1. How Popular Tools Handle Parent/Child Task Relationships

### Linear

**Hierarchy:** Workspace > Teams > Issues (with sub-issues) | Projects (cross-team, with milestones) | Initiatives (strategic, with sub-initiatives)

- Issues are the atomic unit. Sub-issues are full issues linked to a parent via a relation.
- Projects group issues toward a time-bound deliverable; they can span teams.
- Initiatives are a separate, higher-level grouping for strategic goals (enterprise only, up to 5 levels deep).
- Projects, Cycles, and Initiatives are *parallel organisational axes* -- they slice the same issues differently rather than forming a single strict tree.
- Sub-issues have their own status, assignee, and metadata. Progress on the parent auto-rolls-up from child completion counts.

**Key insight:** Linear keeps the issue itself flat (no nested sub-issues within sub-issues for most plans). The hierarchy lives in *grouping constructs* (projects, initiatives) rather than in the issue tree.

Sources: [Linear Conceptual Model](https://linear.app/docs/conceptual-model), [Linear Projects](https://linear.app/docs/projects), [Sub-initiatives changelog](https://linear.app/changelog/2025-07-10-sub-initiatives)

### GitHub Issues

**Hierarchy:** Repository > Issues > Sub-issues (up to 8 levels, max 100 children per parent)

- GitHub originally used **markdown task lists** (`- [ ]` checkboxes inside issue bodies) to track subtasks. This was retired in April 2025.
- Replaced with a dedicated **sub-issues** model: a separate relational table storing parent-child links between full issues.
- Each sub-issue is a first-class issue with its own labels, assignees, status, and comments.
- **Progress rollup** uses a pre-computed aggregation table -- not real-time traversal. When a child issue closes, the parent's progress counter updates automatically.
- Supports 8 levels of nesting, though in practice most usage is 1-2 levels.

**Key insight:** GitHub explicitly moved *away* from inline markdown checkboxes toward structured sub-issues because checkboxes lacked metadata, assignability, and cross-repo linking. The markdown approach was too limited for anything beyond simple checklists.

Sources: [GitHub Sub-issues Blog Post](https://github.blog/engineering/architecture-optimization/introducing-sub-issues-enhancing-issue-management-on-github/), [GitHub Docs: Adding Sub-issues](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/adding-sub-issues)

### Todoist

**Hierarchy:** Projects (with sub-projects) > Sections > Tasks > Sub-tasks (up to 4 indent levels)

- Sub-tasks are full tasks with their own due dates, labels, priorities, and assignees.
- Nesting is controlled by indentation (drag or keyboard shortcut), up to 4 levels deep.
- Completing a parent task completes all its sub-tasks.
- No automatic progress tracking on parent tasks -- you manually check things off.
- Sub-task order is preserved within the parent but can be disrupted by recurring task resets.

**Key insight:** Todoist treats every level identically -- a sub-task is just a task indented under another task. There is no distinct "epic" or "project task" type. This simplicity is both its strength (easy mental model) and weakness (no rollup, no distinguishing a grouping task from an actionable task).

Sources: [Todoist Sub-tasks](https://www.todoist.com/help/articles/introduction-to-sub-tasks-kMamDo), [Todoist Sub-projects](https://www.todoist.com/help/articles/create-a-sub-project-in-todoist-aTA15C70)

### Things 3

**Hierarchy:** Areas > Projects > Headings > To-dos (with checklists inside)

- **Areas** are ongoing responsibilities (e.g. "Health", "Work") -- never completed, just containers.
- **Projects** have a defined outcome and can be completed. They contain to-dos.
- **Headings** subdivide projects into groups (like milestones or phases) but are not tasks themselves.
- **To-dos** are the only actionable items. Each can contain a **checklist** (simple text items, no metadata).
- No nesting of projects within projects. No sub-tasks that are themselves full to-dos.
- Maximum effective depth: Area > Project > Heading > To-do > Checklist item = 5 levels, but only 1 level is actually a "task."

**Key insight:** Things 3 is deliberately shallow. The hierarchy exists for *organisation* (areas, projects, headings) while keeping *actionable work* flat (to-dos with optional checklists). This prevents the "infinite nesting" problem where users endlessly decompose instead of doing.

Sources: [Things Guide](https://culturedcode.com/things/guide/), [Things Headings](https://culturedcode.com/things/support/articles/2803577/)

### Notion

**Hierarchy:** Databases with self-referential relations (unlimited depth)

- Subtasks are implemented as a **self-referential relation** on a database: a "Parent task" relation property pointing to another row in the same database.
- Each subtask is a full database entry with all properties (status, assignee, dates, etc.).
- Progress can be auto-calculated using **rollup properties** (e.g. "% of sub-items where Status is Done").
- No enforced depth limit -- you can nest as deeply as you want.
- Two display modes: **nested in toggles** (tree view) or **flattened list** (all items visible regardless of parent).

**Key insight:** Notion's flexibility is a double-edged sword. The self-referential relation pattern is powerful but requires users to build their own system. Most productive Notion setups cap at 2-3 levels (Goal > Project > Task) and use the flat view for daily execution.

Sources: [Notion Sub-items](https://www.notion.com/help/tasks-and-dependencies), [Notion Subtask Guide](https://www.notion.com/help/guides/tasks-manageable-steps-sub-tasks-dependencies)

### Obsidian (Tasks Plugin)

**Hierarchy:** Indented markdown checkboxes (no enforced structure)

- Tasks are standard markdown checkboxes: `- [ ] Task title`
- "Subtasks" are created by indenting checkboxes under a parent.
- **The Tasks plugin does not understand parent-child relationships.** Indented tasks are visually nested but semantically independent. When queried across files, subtasks appear as standalone items without their parent context.
- Dataview plugin can query tasks with their hierarchy intact, but requires manual setup.
- No progress rollup. No metadata inheritance.

**Key insight:** Obsidian proves that markdown indentation alone is insufficient for meaningful task hierarchy. The plain-text format preserves visual structure but loses semantic relationships when tasks are queried, filtered, or aggregated across files.

Sources: [Obsidian Tasks Plugin](https://github.com/obsidian-tasks-group/obsidian-tasks), [Obsidian Forum Discussion on Subtasks](https://forum.obsidian.md/t/is-there-a-way-to-have-sub-tasks/60397)

### TaskPaper

**Hierarchy:** Projects > Tasks > Sub-tasks (unlimited depth via tab indentation)

- Plain text format: projects end with `:`, tasks start with `- `, notes are anything else.
- Hierarchy is defined by **tab indentation** (must be literal tabs, not spaces).
- Items "own" everything indented beneath them.
- Tags use `@tag(value)` syntax inline.
- No progress tracking. No metadata inheritance. No completion rollup.
- Designed for simplicity: the file IS the UI.

**Key insight:** TaskPaper demonstrates the cleanest plain-text hierarchy model. Tab indentation as ownership is unambiguous and easy to parse. But it intentionally avoids computed properties -- the human manages all state.

Sources: [TaskPaper Getting Started](https://guide.taskpaper.com/getting-started/)

### Logseq

**Hierarchy:** Block-based outliner with TODO/DOING/DONE states

- Every piece of content is a "block" (a bullet point). Hierarchy comes from indentation.
- Tasks are blocks prefixed with `TODO`, `DOING`, or `DONE`.
- Sub-tasks are simply indented TODO blocks under a parent TODO block.
- Queries can find all TODOs across the graph, but hierarchy context is preserved because Logseq is natively an outliner.
- No progress rollup on parent blocks.

**Key insight:** Logseq's outliner-first approach means hierarchy is a natural byproduct of note-taking, not a separate system. Tasks and notes intermix freely. This works well for personal knowledge management but lacks the structure needed for progress tracking.

Sources: [Logseq for Task Management](https://dev.to/vivekkodira/logseq-for-task-management-3d4d)

---

## 2. Naming Conventions Comparison

| Tool | Level 1 (Strategic) | Level 2 (Deliverable) | Level 3 (Actionable) | Level 4 (Sub-step) |
|------|--------------------|-----------------------|---------------------|---------------------|
| Linear | Initiative | Project | Issue | Sub-issue |
| GitHub | Milestone | Epic (large issue) | Issue | Sub-issue |
| Todoist | Project | Section | Task | Sub-task |
| Things 3 | Area | Project | To-do | Checklist item |
| Notion | (user-defined) | (user-defined) | Task | Sub-task |
| Jira | Initiative | Epic | Story/Task | Sub-task |
| Obsidian | (folder/tag) | (note/heading) | Task (checkbox) | Indented checkbox |
| TaskPaper | (n/a) | Project | Task | Sub-task |

**Common patterns:**
- "Epic" is predominantly a Jira/agile term. Most personal productivity tools avoid it.
- "Project" is nearly universal for the "deliverable" level.
- "Task" or "To-do" is universal for the actionable unit.
- "Sub-task" is the most common term for one level below a task.
- Personal tools tend to use "Area" or "Category" for the highest organisational level (not "Initiative" or "Epic").

---

## 3. Flat Checkboxes vs Deep Hierarchy: The Core Trade-off

### Pattern A: Checkboxes in Body (Flat)

```markdown
- [ ] Redesign landing page [tags: website] [added: 2026-03-01]
  Mockup the hero section
  - [ ] Choose colour palette
  - [ ] Write headline copy
  - [x] Gather competitor screenshots
```

**Pros:**
- Simple to parse -- body content is just text under the parent.
- Single item in the task list; sub-steps are detail, not separate tracked entities.
- No need for parent-child linking, progress rollup, or slug management for sub-items.
- Matches how people naturally write notes: a task with some steps jotted underneath.
- Works well for tasks with 2-8 concrete steps.

**Cons:**
- Sub-steps have no metadata (no dates, tags, priorities, or assignees).
- Cannot query/filter/search for individual sub-steps across the system.
- Cannot plan or schedule individual sub-steps independently.
- Progress is not automatically tracked (you must manually count checkboxes).
- Body checkboxes are ambiguous: is `- [ ]` a sub-task or just a note with a checkbox?

**Best for:** Personal productivity where tasks decompose into a short list of concrete actions that will be done in a single session or few sessions.

### Pattern B: Full Sub-tasks (Deep)

```markdown
## Website Redesign

- [ ] Redesign landing page [tags: website] [added: 2026-03-01] [parent: website-redesign]
  Main project for Q2 launch

- [ ] Choose colour palette [tags: website, design] [added: 2026-03-05] [parent: redesign-landing-page]

- [ ] Write headline copy [tags: website, copy] [added: 2026-03-05] [parent: redesign-landing-page]
```

**Pros:**
- Every sub-task is a first-class item with full metadata.
- Can be independently scheduled, tagged, filtered, and searched.
- Progress on parent tasks can be auto-calculated.
- Works for complex, multi-week efforts where sub-tasks are done by different people or on different days.

**Cons:**
- Significantly more complex to implement: parent-child linking, orphan handling, cascading operations (complete parent = complete children?), display in listings.
- The markdown file becomes harder to read -- items lose visual grouping.
- Slug management: renaming a parent requires updating all children's `[parent:]` references.
- Risk of over-decomposition: users create sub-sub-sub-tasks instead of doing work.
- Flat-file format fights against relational data. Every query needs to reconstruct the tree.

**Best for:** Team project management or complex multi-phase personal projects.

### Pattern C: Hybrid (Recommended for Personal Productivity)

```markdown
- [ ] Redesign landing page [tags: website] [added: 2026-03-01]
  Mockup the hero section
  - [ ] Choose colour palette
  - [ ] Write headline copy
  - [x] Gather competitor screenshots
  Progress: 1/3
```

Body checkboxes for sub-steps within a task, **but no `[parent:]` metadata linking**. The parent task's body contains the checklist. If a sub-step grows large enough to need its own metadata, it graduates to a standalone task (possibly with a `[related: parent-slug]` tag for loose association).

**This is what Things 3, Todoist (in practice), and most productive personal systems converge on.**

---

## 4. How Deep Do People Actually Nest?

There is no single peer-reviewed study that conclusively answers this, but converging evidence from tool design and community behaviour:

**Tool defaults tell the story:**
- Things 3 enforces max 1 level of actionable nesting (to-do > checklist). Cultured Code explicitly chose this.
- Todoist allows 4 levels but community guides consistently recommend max 2.
- TaskPaper allows unlimited but the creator's own examples rarely exceed 2 levels.
- GitHub allows 8 levels of sub-issues but their blog post examples show 2.
- Linear's sub-issues for most plans are 1 level deep (sub-issues of issues).

**Observed patterns:**
- **Level 0:** Single task (most tasks). "Buy milk."
- **Level 1:** Task with sub-steps (common for anything taking >1 session). "Plan birthday party" with 5-8 checkboxes.
- **Level 2:** Rare in personal use. Appears in complex projects: "Home renovation > Kitchen > Choose countertops."
- **Level 3+:** Almost never used in personal productivity. When people think they need this, they usually need a project management tool or should restructure into separate projects.

**Design recommendation:** Support 1 level of nesting natively (body checkboxes). If a user consistently needs level 2+, that is a signal they should create separate tasks with a shared tag or project grouping rather than deeper nesting.

---

## 5. How Markdown-Based Tools Handle Hierarchy in Plain Text

### Indentation-Based (TaskPaper, Logseq, Obsidian)

```
Project Name:
	- Task one @tag
		- Sub-task A
		- Sub-task B
	- Task two
```

- **Parser rule:** Tab depth determines parent-child ownership.
- **Advantage:** Human-readable, natural outliner feel.
- **Disadvantage:** Fragile -- mixed tabs/spaces break parsing. Loses structure when items are extracted from context (queries, search results).

### Inline Metadata (Current Dashboard Pattern)

```markdown
- [ ] Task title [tags: foo] [added: 2026-03-01] [planned: 2026-03-28]
  Body text with notes
  - [ ] Sub-step one
  - [x] Sub-step two
```

- **Parser rule:** Top-level `- [ ]` lines are items; indented content is body. Body checkboxes are display-only.
- **Advantage:** Metadata is co-located with the item. File is readable in any text editor.
- **Disadvantage:** Body checkboxes are not semantically tracked.

### Reference-Based Linking (Notion-style in Markdown)

```markdown
- [ ] Parent task [tags: project-x] [added: 2026-03-01]
  Sub-tasks: [[child-task-1]], [[child-task-2]]

- [ ] Child task 1 [parent: parent-task] [added: 2026-03-05]
- [ ] Child task 2 [parent: parent-task] [added: 2026-03-05]
```

- **Parser rule:** `[parent: slug]` establishes the link. Items are still flat in the file; hierarchy is reconstructed at read time.
- **Advantage:** Sub-tasks are full items with metadata. Flat file stays flat.
- **Disadvantage:** Slug renames break links. File becomes harder to read. Circular references possible. Orphan detection needed.

### Section-Based Grouping (GitHub Milestones Pattern)

```markdown
## Website Redesign

- [ ] Design homepage [tags: website] [added: 2026-03-01]
- [ ] Build contact form [tags: website] [added: 2026-03-05]
- [x] Set up hosting [tags: website] [added: 2026-02-15]

## Health

- [ ] Run 5k three times per week [goal: 2/3 runs] [added: 2026-01-01]
```

- **Parser rule:** `##` headings define groups. Items under a heading belong to that group.
- **Advantage:** Very readable. Natural markdown. No linking needed.
- **Disadvantage:** An item can only belong to one section. Cross-cutting concerns (a task relevant to two projects) need tags. Sections are organisational, not actionable -- you cannot "complete" a section heading.

---

## 6. Progress Tracking Patterns

### Auto-Calculated (Count-Based)

```
Parent: 3/7 sub-tasks complete (43%)
```

- Count completed children / total children.
- Used by: GitHub sub-issues, Notion rollups, Jira.
- **Pro:** Zero manual effort. Always accurate.
- **Con:** Treats all sub-tasks as equal weight. "Write 500-word blog post" and "Fix typo" both count as 1.

### Auto-Calculated (Weighted)

```
Parent: 65% complete (weighted by estimated hours)
```

- Each sub-task has an effort estimate; completion is weighted.
- Used by: MS Project, enterprise PM tools.
- **Pro:** More accurate for complex projects.
- **Con:** Requires effort estimates (overhead that personal tools want to avoid).

### Manual Progress

```
- [ ] Learn Spanish [goal: 45/100 lessons]
```

- User manually updates a progress field.
- Used by: Things 3 (implicitly), the current dashboard's goal type.
- **Pro:** User controls the narrative. Works for non-decomposable progress (reading a book, learning a skill).
- **Con:** Easy to forget updating. Can drift from reality.

### Body Checkbox Counting (Hybrid)

```markdown
- [ ] Plan birthday party [added: 2026-03-01]
  - [x] Book venue
  - [x] Send invitations
  - [ ] Order cake
  - [ ] Buy decorations
  Progress: 2/4
```

- Parse checkboxes in the body and display a count. The count is derived at read time, not stored.
- **Pro:** No separate progress field needed. Body checkboxes serve double duty as notes AND progress indicators.
- **Con:** Only works if the task is decomposed into checkboxes. Not all tasks have them.

**Recommendation for a personal flat-file tracker:** Body checkbox counting (derived at read time) for tasks that have sub-steps, combined with the existing `[goal: current/target unit]` pattern for measurable goals. No stored progress field -- calculate it from the body content.

---

## 7. Recommendations for the Dashboard

Given the current architecture (flat markdown files, `- [ ]` checkbox parser, inline metadata tags, in-memory cache), here are ranked options from simplest to most complex:

### Option A: Body Checkbox Awareness (Minimal Change)

Add read-time parsing of `- [ ]` and `- [x]` lines in task bodies to derive a sub-step count. Display as "2/5 steps" on task cards. No new metadata. No parent-child linking. Body checkboxes remain display text that the user edits manually in the markdown.

**What changes:**
- Parser extracts checkbox counts from body text.
- `Item` struct gains `SubStepsDone int` and `SubStepsTotal int` (computed, not stored).
- UI shows a small progress indicator on tasks that have body checkboxes.

**What does NOT change:**
- File format unchanged. No new inline metadata tags.
- No parent-child task linking. No slug references between items.
- Sub-steps are not independently queryable, plannable, or filterable.

**Effort:** Small. Parser change + UI display.

### Option B: Lightweight Grouping via Tags (Moderate Change)

Use a conventional tag prefix (e.g. `project:website-redesign`) to group related tasks. Add a "project view" that filters by tag prefix and shows aggregate progress (done/total tasks with that tag).

**What changes:**
- Convention: tags starting with `project:` are treated as project groupings.
- New view/filter: "Show all tasks tagged project:website-redesign."
- Progress = count of done tasks / total tasks with that project tag.

**What does NOT change:**
- File format unchanged (tags already exist).
- No parent-child linking. No hierarchy in the file.
- Tasks remain flat, independently plannable items.

**Effort:** Moderate. New view + tag convention + progress aggregation.

### Option C: Parent References (Significant Change)

Add `[parent: slug]` inline metadata. Tasks with children become "parent tasks." Progress auto-calculated from children. Parent tasks appear in listings with an expandable children list.

**What changes:**
- New inline metadata: `[parent: slug]`.
- `Item` struct gains `ParentSlug string` and computed `Children []Item`.
- Tree reconstruction at cache load time.
- Cascading operations: completing a parent could complete children (or not -- design choice).
- UI: parent tasks show children inline; children may be hidden from top-level listings.

**What does NOT change:**
- File format stays markdown with inline metadata. Still flat-file.
- Items are still parsed from checkbox lines.

**New complexity:**
- Slug renames must update all children's `[parent:]` references (within same file: manageable; cross-file: harder).
- Orphan detection (parent deleted but children remain).
- Circular reference prevention.
- Display decisions: do children appear in the main list? Only under parent? Both?
- Search: should searching for a parent return children? Vice versa?
- Planner: can you plan a parent task? A child task? Both?

**Effort:** Significant. Touches parser, service, cache, handlers, templates, planner.

### Suggested Path

**Start with Option A.** It provides visible progress for tasks with sub-steps, matches user expectations from Things 3 and similar tools, and requires no format changes. If users (you) find that tag-based grouping would help, add Option B -- it layers on top without conflict. Option C should only be considered if there is a concrete, repeated need for independently-scheduled sub-tasks that share a parent, and even then, consider whether separate tasks with a shared project tag (Option B) would suffice.

The research consistently shows: tools that start simple and add hierarchy reluctantly (Things 3, Linear) produce better user experiences than tools that start with deep hierarchy and try to simplify it (Jira, Asana).

---

## References

- [Linear Conceptual Model](https://linear.app/docs/conceptual-model)
- [Linear Projects](https://linear.app/docs/projects)
- [GitHub Sub-issues Architecture](https://github.blog/engineering/architecture-optimization/introducing-sub-issues-enhancing-issue-management-on-github/)
- [GitHub Sub-issues Docs](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/adding-sub-issues)
- [Todoist Sub-tasks](https://www.todoist.com/help/articles/introduction-to-sub-tasks-kMamDo)
- [Things 3 Guide](https://culturedcode.com/things/guide/)
- [Things 3 Headings](https://culturedcode.com/things/support/articles/2803577/)
- [Notion Sub-items](https://www.notion.com/help/tasks-and-dependencies)
- [Obsidian Tasks Plugin](https://github.com/obsidian-tasks-group/obsidian-tasks)
- [Obsidian Subtasks Discussion](https://forum.obsidian.md/t/is-there-a-way-to-have-sub-tasks/60397)
- [TaskPaper Guide](https://guide.taskpaper.com/getting-started/)
- [Logseq Task Management](https://dev.to/vivekkodira/logseq-for-task-management-3d4d)
- [Hierarchical Task Lists (Medium)](https://medium.com/@socials_61248/hierarchical-task-list-how-to-organize-work-without-overwhelm-52b44dafc0dc)
