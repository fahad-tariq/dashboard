# Combine Ideas and Explorations with Flat-File Storage

## Overview

Merge explorations into ideas and migrate ideas from multi-directory file storage to a single flat-file format (like the tracker system). This eliminates the `internal/exploration/` package, simplifies the ideas storage from 4 subdirectories per user to a single `ideas.md` file, and creates architectural consistency with the existing tracker (todos/family) system.

## Current State

**Problems Identified:**
- Two packages (`internal/ideas/`, `internal/exploration/`) with heavily duplicated code: frontmatter parsing, slug generation, tag parsing, write logic, CRUD services, and handlers
- Ideas use a complex multi-directory storage model (`untriaged/`, `parked/`, `dropped/`, `research/`) where status transitions require moving files between directories
- The tracker (todos/family) uses a simpler, proven flat-file model. Ideas should follow the same pattern for consistency
- Separate templates, routes, config fields, directory provisioning, and watcher registrations for two near-identical concepts
- The homepage and nav treat ideas and explorations as distinct entities

**Technical Context:**
- Go + chi router, server-rendered HTML templates with htmx
- Tracker system uses single `.md` files with checkbox items and inline metadata (e.g. `[tags: ...]`, `!priority`). Source of truth is the markdown; no database cache for ideas
- Ideas currently use individual `.md` files with YAML frontmatter, stored in status-based subdirectories. The `Idea.Body` field includes a `# Title` heading as the first line
- Explorations use a flat directory of individual `.md` files (no status)
- Per-user service isolation via `services.Registry` (auth-enabled mode) and singleton services (auth-disabled mode)
- Admin handler (`internal/admin/handler.go`) displays per-user exploration counts via `svc.Explorations.List()`
- File watcher (`internal/watcher/watcher.go`) classifies `explorations/*` paths as "exploration" category
- API handler for auth-enabled mode hardcodes user 1 path: `cfg.UserDataDir+"/1/ideas"` (main.go line ~394)

## Requirements

**Functional Requirements:**
1. The system MUST have a single "Ideas" concept -- no separate explorations section
2. Ideas MUST be stored in a single `ideas.md` file per user, following the flat-file pattern used by the tracker
3. Each idea MUST support: title, status (`untriaged`/`parked`/`dropped`), tags, images, project (optional), date added, and a multi-line body
4. New ideas added via quick-add MUST default to `untriaged` status
5. The system MUST retain park/drop/untriage triage actions (implemented as inline status edits, not file moves)
6. The system MUST retain the "to todos" action (moves an idea to `personal.md` as a task, then deletes it from `ideas.md`)
7. Research content MUST be stored inline as part of the idea body (no separate research files)
8. All existing API endpoints (`/api/v1/ideas/*`) MUST continue to work with the new storage format
9. The `/exploration` routes MUST redirect to `/ideas` (301) for bookmarks
10. The homepage Ideas card MUST show both untriaged count and total idea count

**Technical Constraints:**
1. The `internal/exploration/` package MUST be deleted entirely
2. The `EXPLORATION_DIR` config field MUST be removed
3. The `IDEAS_DIR` env var MUST be handled with backwards compatibility: derive the `ideas.md` file path from it (e.g. `IDEAS_DIR` value + `.md`, or a new `IDEAS_PATH` env var that takes precedence)
4. Exploration templates (`exploration.html`, `exploration-detail.html`) MUST be removed
5. The ideas parser MUST be separate from the tracker parser (ideas have status, project, and date fields that tasks don't; tasks have priority, goals, and progress that ideas don't)
6. The ideas parser MUST preserve blank lines within multi-line bodies (unlike the tracker parser which drops them -- ideas contain rich markdown with paragraph breaks)
7. Shared utility functions (slug generation, CSV parsing, tag extraction) SHOULD be reused
8. The `EnsureUserDirs` directory structure MUST be simplified -- no more `ideas/untriaged/`, `ideas/parked/`, `ideas/dropped/`, `ideas/research/` subdirectories
9. The nav MUST have 5 links in order: home, todos, goals, ideas, family
10. The ideas service MUST use `sync.RWMutex` with `RLock` for reads and `Lock` for writes

**Ideas File Format:**
```markdown
# Ideas

- [ ] Try Caddy instead of nginx [status: parked] [tags: infra, homelab] [project: homelabs] [added: 2026-03-14]
  Replace nginx reverse proxy with Caddy for automatic HTTPS and simpler config.

  ## Research
  Caddy auto-provisions TLS certs via ACME. Much simpler config than nginx.

- [ ] Dashboard mobile PWA [status: untriaged] [tags: dashboard] [added: 2026-03-16] [images: pwa-sketch.png]
  Add a manifest.json and service worker for offline support.
```

Each idea is a `- [ ]` checkbox line with inline metadata, followed by optional indented body lines. Body lines preserve blank lines between paragraphs. The title is on the checkbox line only (NOT duplicated as a `# Title` heading in the body). Inline metadata tags (`[status: ...]`, `[tags: ...]`, etc.) are parsed from the checkbox line only.

**Data Migration Rules:**
- Old idea bodies have `# Title` as the first line -- strip this during migration (title moves to the checkbox line)
- Research files (`research/slug.md`) are appended to the matching idea's body under a `## Research` heading
- Orphaned research files (no matching idea) become standalone untriaged ideas
- Exploration files are migrated as `parked` status (not `untriaged`) to avoid swamping the triage inbox
- Slug collisions between ideas and explorations MUST be resolved by suffixing the exploration slug (e.g. `my-idea` becomes `my-idea-exp`)

## Unknowns & Assumptions

**Unknowns:**
- Whether any external systems (e.g. Ironclaw) currently POST to exploration-specific endpoints (assumed: no, since explorations have no API)

**Assumptions:**
- The number of ideas per user will remain manageable in a single file (under a few hundred)
- The `suggested-project` frontmatter key maps to `[project: ...]` in the new format

## Success Criteria

1. `internal/exploration/` package is deleted; no imports of it remain
2. Ideas are stored in a single `ideas.md` file per user, following the flat-file pattern
3. The ideas parser is separate from the tracker parser, with its own `ParseIdeas()`/`WriteIdeas()` functions
4. The ideas parser preserves blank lines in multi-line bodies (round-trip fidelity test passes)
5. All exploration routes redirect to `/ideas` with 301
6. Homepage shows a single "Ideas" card with untriaged count and total count
7. Nav has 5 links: home, todos, goals, ideas, family
8. Quick-add creates ideas with `[status: untriaged]` in `ideas.md`
9. Park/drop/untriage are inline status edits in `ideas.md`
10. "To todos" moves an idea from `ideas.md` to `personal.md` as a tracker item
11. API endpoints function correctly with the new storage format (including auth-enabled mode)
12. `migrate-data` CLI converts old directory-based ideas (stripping `# Title` from bodies), merges research files into bodies, and converts explorations (as `parked`, with slug collision handling)
13. Admin user list shows idea count (no separate exploration count)
14. All tests pass, linting passes, application builds and runs without errors

---

## Development Plan

### Phase 1: New Ideas Parser and Model

Build the new flat-file parser for ideas. This phase creates new code alongside the existing package -- nothing is deleted yet.

- [ ] Define the new `Idea` struct with fields: Slug, Title, Status, Tags, Images, Project, Added, Body
- [ ] Implement `ParseIdeas(path string) ([]Idea, error)` -- reads `ideas.md` and returns all ideas. Parses checkbox lines with inline metadata: `[status: x]`, `[tags: a, b]`, `[project: x]`, `[added: YYYY-MM-DD]`, `[images: a.png]`
- [ ] Body parsing MUST preserve blank lines within indented blocks (divergence from tracker parser which drops them). A body line is any line indented with 2+ spaces, or a blank line that appears between indented lines
- [ ] Implement `WriteIdeas(path string, heading string, ideas []Idea) error` -- reconstructs the full `ideas.md` from a slice of ideas, preserving body content including blank lines
- [ ] Handle edge cases: empty file, file with just the heading, ideas with no body, ideas with multi-line body including blank lines, missing optional fields (project, images)
- [ ] Write tests: basic parsing, round-trip fidelity (including blank lines in body), all metadata fields, missing optional fields, empty file
- [ ] Verify the project compiles: `go build ./...`
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 2: New Ideas Service

Replace the directory-scanning service with a flat-file service following the tracker's read-modify-write pattern.

- [ ] Rewrite `Service` to operate on a single `ideas.md` file path (replace `ideasDir` with `ideasPath`). Use `sync.RWMutex` -- `RLock` for read operations, `Lock` for writes
- [ ] Implement `List()` -- acquires `RLock`, parses `ideas.md`, returns all ideas
- [ ] Implement `Get(slug)` -- acquires `RLock`, parses and finds by slug
- [ ] Implement `Add(idea)` -- acquires `Lock`, parses, appends, writes back
- [ ] Implement `Triage(slug, action)` -- acquires `Lock`, parses, updates the `[status: ...]` value, writes back
- [ ] Implement `Edit(slug, body, tags, images)` -- acquires `Lock`, parses, updates fields, writes back
- [ ] Implement `Delete(slug)` -- acquires `Lock`, parses, removes from slice, writes back
- [ ] Implement `AddResearch(slug, content)` -- acquires `Lock`, parses, appends content to the idea's body (under a `## Research` heading if not already present), writes back
- [ ] Implement `GetResearch(slug)` -- returns the idea's body (research is now inline; this is for API compatibility)
- [ ] Write tests: CRUD operations, triage state transitions, add-research appends correctly, concurrent access safety
- [ ] Verify the project compiles
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 3: Update Handler, Routes, and Templates

Update the ideas handler, consolidate templates, and remove exploration routes.

- [ ] Update the ideas handler to work with the new service API. Key change: `ToTask` no longer needs to strip `# Title` from the body (the title is on the checkbox line in the new format, not in the body)
- [ ] Update `ideas.html` template: keep status-grouped layout (untriaged/parked/dropped sections with triage buttons)
- [ ] Update `idea.html` template: remove the separate `{{if .ResearchHTML}}` research section (research is now part of the body)
- [ ] Delete `exploration.html` and `exploration-detail.html` templates
- [ ] Remove `exploration.html` and `exploration-detail.html` from `parseTemplates()` pages slice
- [ ] Delete `internal/exploration/` directory entirely
- [ ] In `main.go`, remove all exploration handler creation, route registration, and imports in both auth-enabled and auth-disabled branches
- [ ] Add 301 redirects: `/exploration` -> `/ideas`, `/exploration/{slug}` -> `/ideas/{slug}`
- [ ] Verify the project compiles and ideas pages render correctly
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 4: Update Integration Points

Update all code outside the ideas package that references explorations or the old ideas directory structure.

- [ ] **Config** (`config.go`): Remove `ExplorationDir` field and `EXPLORATION_DIR` env var. Remove exploration directory creation from `validate()`. Replace `IdeasDir` with `IdeasPath` (file path to `ideas.md`, default `/data/ideas.md`). For backwards compatibility, if `IDEAS_DIR` is set and `IDEAS_PATH` is not, derive the path as `IDEAS_DIR + ".md"` or similar. Remove ideas subdirectory creation from `validate()`. In `validate()`, create parent directory and write skeleton `ideas.md` (`# Ideas\n\n`) if it does not exist
- [ ] **Registry** (`services/registry.go`): Remove `Explorations` field from `UserServices`. Remove exploration import. Simplify `EnsureUserDirs()` -- remove the 4 ideas subdirectories and `explorations/` directory. Instead, create `ideas.md` skeleton file if it does not exist. Update `ForUser()` to create ideas service with `{base}/ideas.md` path
- [ ] **Admin handler** (`admin/handler.go`): Remove `Explorations` field from `UserStats`. Remove the `svc.Explorations.List()` call (lines 84-85). Update `admin-users.html` template: change `{{.PersonalTasks}} tasks · {{.Ideas}} ideas · {{.Explorations}} explorations` to `{{.PersonalTasks}} tasks · {{.Ideas}} ideas`
- [ ] **Homepage** (`main.go`): Update `homePageWithRegistry()` -- remove `explorations` fetching, remove `RecentExplorations`/`ExplorationCount` template data. Add `TotalIdeaCount` to template data (total ideas across all statuses). Update `homePage()` function -- remove `explorationSvc` parameter and its call site (line ~365). Remove the `recentExplorations()` helper function. Update `homepage.html`: remove exploration card, update Ideas card to show both untriaged and total counts (e.g. "3 untriaged / 12 total"). Update empty-state condition to use total idea count instead of just untriaged
- [ ] **Navigation** (`layout.html`): Remove the exploration link. Reorder to: home, todos, goals, ideas, family
- [ ] **Watcher** (`watcher.go`): Remove the `case strings.HasPrefix(subpath, "explorations")` branch from `classifyEventWithUser()`. The existing `case strings.HasPrefix(subpath, "ideas")` branch still works for `ideas.md` since it starts with "ideas"
- [ ] **Watcher registration** (`main.go`): In auth-disabled mode, remove the `ExplorationDir` entry from `dirCategories`. Move ideas from `dirCategories` to `fileCategories` (watch the `ideas.md` file path, not a directory)
- [ ] **API handler** (`main.go` lines ~391-405): Update the auth-enabled API handler to use the correct `ideas.md` file path: change `cfg.UserDataDir+"/1/ideas"` to `cfg.UserDataDir+"/1/ideas.md"` (or use registry to resolve user 1's ideas service)
- [ ] **migrate-data CLI** (`main.go`): Rewrite to convert old format to new flat-file format:
  - For each user: read all idea files from `untriaged/`, `parked/`, `dropped/` directories. For each, strip the `# Title` heading from the body, map `suggested-project` to `[project: ...]`, set status from the source directory name
  - Read all `research/slug.md` files and append content to matching idea bodies under a `## Research` heading. Log warning for orphaned research files and create them as standalone untriaged ideas
  - Read all exploration files from `explorations/` directory. Set status to `parked`. Check for slug collisions with existing ideas -- suffix with `-exp` if collision detected
  - Write combined `ideas.md` flat file
  - Remove old exploration directory creation from the migration directory list
- [ ] Verify the project compiles
- [ ] Perform a critical self-review of your changes and fix any issues found
- [ ] STOP and wait for human review

### Phase 5: Update Tests and Final Review

- [ ] Delete `test/exploration_test.go`
- [ ] Update `test/registry_test.go`: remove `"1/explorations"` and ideas subdirectories from expected structure, verify `ideas.md` skeleton is created instead
- [ ] Rewrite `test/ideas_test.go` for the new flat-file format: parser round-trip (including blank line preservation), service CRUD, triage transitions, add-research, slug collision handling
- [ ] Update `test/watcher_test.go`: remove `TestClassifyEventPerUserExploration`, update ideas classification test for `{uid}/ideas.md` file path
- [ ] Run full test suite and verify all tests pass
- [ ] Run linter (`go vet ./...`) and fix any issues
- [ ] Build application and verify no errors or warnings
- [ ] Grep for any remaining references to "exploration" in Go code, templates, and tests -- clean up stale ones
- [ ] Verify all success criteria are met
- [ ] Perform a critical self-review of all changes

---

## Notes

- The ideas parser follows the same flat-file pattern as the tracker (read-modify-write with RWMutex) but diverges on body parsing: it MUST preserve blank lines in indented body blocks. The tracker drops blank lines, which is acceptable for short task notes but not for rich idea content with markdown paragraphs.
- The backlog item #2 (reorder top navigation) is completed as part of Phase 4 (nav order: home, todos, goals, ideas, family).
- The `[project: ...]` field replaces the old `suggested-project` frontmatter key.
- Research is no longer a separate concept -- it's body text under a `## Research` markdown heading. The API `AddResearch` endpoint appends to the body.
- Inline metadata tags (`[status: ...]`, `[tags: ...]`, etc.) are parsed from the checkbox line only. If a title happens to contain text matching the metadata pattern (e.g. "Check [status: parked] indicator"), this is a known limitation shared with the tracker's `[tags: ...]` syntax.

---

## Working Notes (for executing agent)

