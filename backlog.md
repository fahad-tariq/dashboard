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

### Tailscale integration documentation
- priority: medium
- added: 2026-03-15

Document how to expose the dashboard over Tailscale for remote API access from ironclaw.

### Error handling and edge cases
- priority: low
- added: 2026-03-15

Audit error paths: malformed markdown, missing directories, concurrent file operations.


## Done

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
