# Dashboard

Personal project dashboard that scans `~/projects/` directories, renders their README and backlog files, and provides an ideas inbox with triage workflow. Built to solve three problems: ideas getting lost, context loss when returning to projects after weeks, and no cross-project overview.

## What it does

- Scans a projects directory for git repos containing `README.md`
- Displays projects in a table with git activity sparklines, last push time, recent commit messages, and backlog counts
- Renders `README.md` and `backlog.md` per project with inline editing
- Ideas inbox: capture ideas as individual markdown files, triage them (park, drop, assign to a project's backlog)
- REST API for external integrations (e.g. pushing ideas from an AI assistant via Tailscale)
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
| `PROJECTS_DIR` | `/data/projects` | Directory containing project repos |
| `IDEAS_DIR` | `/data/ideas` | Directory for idea markdown files |
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
