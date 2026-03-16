# Tracker

A personal task, goal, and idea tracker backed by a single markdown file. Tasks and goals live in `tracker.md`, ideas are file-per-idea in status directories. Web UI with htmx for live updates.

## Setup

```bash
cp .env.example .env
# Edit .env as needed
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `IDEAS_DIR` | `/data/ideas` | Directory for idea files (auto-created) |
| `TRACKER_PATH` | `/data/tracker.md` | Path to the tracker markdown file |
| `DB_PATH` | `/data/db/tracker.db` | SQLite database path (cache) |
| `DASHBOARD_API_TOKEN` | (empty) | Bearer token for API auth (optional) |
| `ADDR` | `:8080` | Server listen address |

## Running

```bash
# Development
make run

# Or directly
IDEAS_DIR=./ideas TRACKER_PATH=./data/tracker.md go run ./cmd/tracker

# Build binary
make build
./bin/tracker
```

## Docker

```bash
make docker-build
make docker-run
```

Or manually:

```bash
docker compose up
```

The compose file mounts `./ideas` for idea files and `./data` for the database and tracker.md.

## Features

### Tasks
- Quick add with optional tags and priority (high/medium/low)
- Inline notes, tag, and priority editing
- Filter by tag or priority
- Expand/collapse all
- Complete/uncomplete/delete
- Stored as checkbox items in `tracker.md`

### Goals
- Progress tracking with current/target and unit (e.g. 12/40 books)
- Progress bar visualisation
- Increment (+1/-1) or set absolute value
- Same priority and tag system as tasks

### Ideas
- File-per-idea with frontmatter metadata
- Triage workflow: untriaged -> parked / dropped
- Convert idea to task
- Research notes per idea
- Quick add from web UI

### General
- Live reload via SSE on file changes
- Catppuccin dark/light theme
- REST API with optional bearer token auth

## Tracker file format

Tasks and goals are stored in `tracker.md` as checkbox items:

```markdown
# Tracker

## Health
- [ ] Run 5km !high [added: 2026-03-10] [tags: fitness]
- [ ] Reach 90kg [goal: 85.5/90 kg] [added: 2026-03-01]

## Reading
- [x] Finish book club pick [completed: 2026-03-15] [tags: books]
```

## Idea file format

Each idea is a markdown file in `IDEAS_DIR/{status}/`:

```markdown
---
type: feature
date: 2026-03-14
---

# Idea title

Description here.
```

Status directories: `untriaged/`, `parked/`, `dropped/`, `research/`.

## Routes

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Tasks page |
| `GET` | `/goals` | Goals page |
| `GET` | `/ideas` | Ideas inbox |
| `GET` | `/ideas/{slug}` | Idea detail |
| `GET` | `/events` | SSE endpoint for live reload |

### API

All API routes are under `/api/v1` and require a bearer token if `DASHBOARD_API_TOKEN` is set.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/ideas` | List all ideas |
| `POST` | `/api/v1/ideas` | Create idea (JSON body) |
| `PUT` | `/api/v1/ideas/{slug}/triage` | Triage idea (park/drop/untriage) |
| `POST` | `/api/v1/ideas/{slug}/research` | Add research content |

## Stack

Go, chi, SQLite (modernc), goldmark, fsnotify, htmx.
