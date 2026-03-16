# Projects

A dashboard for monitoring local git repositories. Scans a directory of projects, displays git metadata (activity sparklines, last push, sync status), and provides per-project backlog and plan management.

## Setup

```bash
cp .env.example .env
# Edit .env -- set PROJECTS_DIR to your projects directory
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `PROJECTS_DIR` | `/data/projects` | Directory containing git repositories (required) |
| `DB_PATH` | `/data/db/projects.db` | SQLite database path |
| `ADDR` | `:8080` | Server listen address |

Each subdirectory of `PROJECTS_DIR` that contains a `README.md` is treated as a project.

## Running

```bash
# Development
make run

# Or directly
PROJECTS_DIR=~/projects go run ./cmd/projects

# Build binary
make build
./bin/projects
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

The compose file mounts `~/projects` as the projects directory and `./data` for the database.

## Features

- Scans git repositories and displays commit activity, last push, sync status
- 30-day activity sparklines per project
- Per-project README and backlog editing (markdown)
- Plan file viewer (`plans/*.md` within each project)
- Project status management (active/paused/archived)
- Live reload via SSE on file changes
- Git remote fetch and sync status display
- Catppuccin dark/light theme

## Routes

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Project dashboard |
| `POST` | `/sync` | Fetch all remotes and refresh |
| `GET` | `/projects/{slug}` | Project detail (README, backlog, plans) |
| `POST` | `/projects/{slug}/save/{filename}` | Save README.md or backlog.md |
| `PUT` | `/projects/{slug}/status` | Update project status |
| `GET` | `/events` | SSE endpoint for live reload |

## Stack

Go, chi, SQLite (modernc), goldmark, fsnotify, htmx.
