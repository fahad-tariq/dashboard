# Dashboard

Personal dashboard for tracking tasks, goals, projects, and ideas. Four tabs: tasks (homepage), goals, projects, ideas. Markdown source of truth with SQLite cache, live reload via SSE, Catppuccin themes.

## What it does

- **Tasks** (`/`): personal task tracker with tags, priorities, notes, complete/uncomplete/delete, filter by tag or priority, expand/collapse items
- **Goals** (`/goals`): measurable goals with optional progress bars (current/target), +1/-1 and set controls
- **Projects** (`/projects`): scans a directory for git repos, displays activity sparklines, last push, recent commits, backlog counts, expandable detail rows
- **Ideas** (`/ideas`): capture ideas as markdown files, triage (park, drop, assign to project backlog, convert to task)
- **Graduation**: promote a task or goal to a full project (creates directory with git init + README)
- REST API for external integrations
- Live reload via SSE when files change on disk
- Catppuccin colour themes (Latte for light, Mocha for dark)

## Setup

```bash
cp .env.example .env
# Edit .env — set DASHBOARD_API_TOKEN to a secret value

# Docker (recommended)
make docker-build
make docker-run

# Or run locally
export PROJECTS_DIR=~/projects
export IDEAS_DIR=./ideas
export DB_PATH=./data/dashboard.db
make build && ./bin/dashboard
```

The server starts on `:8080` by default. Set `ADDR` to override.

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PROJECTS_DIR` | `/data/projects` | Directory containing project repos (optional — omit or set empty for tracker-only mode) |
| `IDEAS_DIR` | `/data/ideas` | Directory for idea markdown files |
| `TRACKER_PATH` | `/data/tracker.md` | Path to the tracker markdown file |
| `DB_PATH` | `/data/db/dashboard.db` | SQLite database path |
| `DASHBOARD_API_TOKEN` | _(empty)_ | Bearer token for API auth (disabled if empty) |
| `ADDR` | `:8080` | Listen address |

## Docker volumes

```yaml
volumes:
  - ~/projects:/data/projects    # Project repos
  - ./ideas:/data/ideas          # Ideas markdown files
  - ./data:/data/db              # SQLite database
```

## API

All endpoints under `/api/v1/` require a `Authorization: Bearer <token>` header when `DASHBOARD_API_TOKEN` is set.

```
GET    /api/v1/projects              List projects
GET    /api/v1/projects/{slug}       Project detail + backlog
GET    /api/v1/ideas                 List ideas
POST   /api/v1/ideas                 Add idea
PUT    /api/v1/ideas/{slug}/triage   Triage (action: park, drop, assign)
POST   /api/v1/ideas/{slug}/research Add research content
```

### Add an idea

```bash
curl -X POST http://localhost:8080/api/v1/ideas \
  -H "Authorization: Bearer $DASHBOARD_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Try Caddy instead of nginx","type":"technical-exploration","body":"Replace nginx with Caddy for automatic HTTPS."}'
```

## Tracker storage

Tasks and goals are stored in a single markdown file (`tracker.md`):

```markdown
# Tracker

- [ ] Deploy app !high [added: 2026-03-15] [tags: work, devops]
  Notes and links go here as indented body text
- [ ] Read 40 books [goal: 12/40 books] [added: 2026-03-15] [tags: reading]
- [x] Set up CI [added: 2026-03-10] [completed: 2026-03-15] [tags: work]
```

Inline metadata: `!priority`, `[goal: current/target unit]`, `[added: date]`, `[completed: date]`, `[tags: comma, separated]`, `[graduated]`.

## Ideas storage

Ideas are individual markdown files in status-based directories:

```
ideas/
├── untriaged/
│   └── try-caddy-instead-of-nginx.md
├── parked/
├── dropped/
└── research/
```

Each file uses frontmatter + markdown body:

```markdown
---
type: technical-exploration
suggested-project: homelabs
date: 2026-03-14
---

# Try Caddy instead of nginx

Replace nginx reverse proxy with Caddy for automatic HTTPS and simpler config.
```

Triaging an idea to "assign" moves its content into the target project's `backlog.md` and deletes the idea file.

## Stack

Go, chi v5, html/template, htmx, goldmark, SQLite (modernc.org/sqlite, pure Go), fsnotify.

## Development

```bash
make lint      # golangci-lint
make test      # go test -race
make build     # static binary in bin/
make run       # go run
make tidy      # go mod tidy
```
