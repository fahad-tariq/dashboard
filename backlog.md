# Backlog

## Active

### Amber and custom theme support
- priority: low
- added: 2026-03-15

Currently only Catppuccin Latte/Mocha. Add theme switcher extensibility for custom colour schemes.

### Mobile layout refinements
- priority: medium
- added: 2026-03-15

Touch targets, single-column layout adjustments, disable glow effects on mobile.

### Tailscale: install client on dashboard host
- priority: low
- added: 2026-03-15

Install and configure Tailscale client on the machine running the dashboard so it is reachable on the tailnet.

### Tailscale: install client on ironclaw
- priority: low
- added: 2026-03-15

Install and configure Tailscale client on ironclaw so it can reach the dashboard over the tailnet.

### Backups and visual inspection of markdown files
- priority: medium
- added: 2026-03-15

Add a mechanism to back up tracker.md and ideas files before writes. Consider a visual diff or preview of the raw markdown in the UI for manual inspection.

### Make PROJECTS_DIR optional
- priority: high
- added: 2026-03-15

Gracefully degrade when PROJECTS_DIR is empty or missing. Tracker, ideas, and goals work as normal. Projects page shows empty state. Allows running on a server without git repos.

### CI: GitHub Actions image build and ghcr.io push
- priority: high
- added: 2026-03-15

On push to main, build Docker image and push to ghcr.io. Both macOS and home server pull the same image with different docker-compose configs.

### Syncthing data sync between instances
- priority: high
- added: 2026-03-15

Use Syncthing over Tailscale to sync tracker.md and ideas/ between macOS and home server. Exclude SQLite DB files (*.db, *.db-wal, *.db-shm) — each instance rebuilds its own cache via Resync(). fsnotify already detects changes and triggers SSE updates.

### Error handling and edge cases
- priority: low
- added: 2026-03-15

Audit error paths: malformed markdown, missing directories, concurrent file operations.


## Done

### Tracker feature: tasks and goals
- added: 2026-03-15
- done: 2026-03-15

Full personal tracker with tasks (/) and goals (/goals) pages. Features: tags, priorities, progress bars, quick-add forms, complete/uncomplete/delete, graduation to projects, inline notes/tag/priority editing, filter bar with persistence, SSE live reload, expand/collapse animation, idea-to-task conversion. Markdown source of truth (tracker.md) with SQLite cache.

### Rename project directory
- added: 2026-03-15
- done: 2026-03-15

Move from `~/projects/research/dashboard` to `~/projects/dashboard`.

### Replace status column with git sync status
- added: 2026-03-15
- done: 2026-03-15

Replace redundant active/inactive column with git sync status showing ahead/behind/clean/diverged vs remote.

### Project scanning and dashboard table
- added: 2026-03-14
- done: 2026-03-14

Scan projects dir, display in table with sparklines, expandable detail rows.

### Markdown rendering with goldmark
- added: 2026-03-14
- done: 2026-03-14

README and backlog rendering with syntax highlighting.

### Ideas system with triage workflow
- added: 2026-03-14
- done: 2026-03-14

File-per-idea storage, web UI, REST API, triage actions (park/drop/assign).

### SSE live reload
- added: 2026-03-14
- done: 2026-03-14

fsnotify watcher with debounce, SSE broker, htmx auto-refresh.

### REST API with bearer auth
- added: 2026-03-14
- done: 2026-03-14

API endpoints for projects and ideas, constant-time bearer token validation.

### Catppuccin themes with dark/light toggle
- added: 2026-03-14
- done: 2026-03-14

Latte (light) and Mocha (dark) with localStorage persistence.

### Tabbed project view with inline editing
- added: 2026-03-15
- done: 2026-03-15

README.md and backlog.md in tabs, editable via textarea, auto-create backlog.md if missing.

### Docker build and deployment
- added: 2026-03-14
- done: 2026-03-15

Multi-stage Dockerfile, docker-compose with volume mounts, .env configuration.
