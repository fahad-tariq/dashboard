# Dashboard

A personal task, goal, idea, and exploration dashboard backed by markdown files. Tasks and goals are split into personal (`personal.md`) and family (`family.md`) lists. Ideas and explorations are file-per-item in their own directories. Web UI with htmx for live updates. Supports image upload and clipboard paste across all content types.

## Setup

```bash
cp .env.example .env
# Edit .env as needed
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `IDEAS_DIR` | `/data/ideas` | Directory for idea files (auto-created) |
| `EXPLORATION_DIR` | `/data/explorations` | Directory for exploration files (auto-created) |
| `UPLOADS_DIR` | `/data/uploads` | Directory for uploaded images (auto-created) |
| `PERSONAL_PATH` | `/data/personal.md` | Path to the personal tasks markdown file |
| `FAMILY_PATH` | `/data/family.md` | Path to the family tasks markdown file |
| `DB_PATH` | `/data/db/dashboard.db` | SQLite database path (cache) |
| `DASHBOARD_API_TOKEN` | (empty) | Bearer token for API auth (optional) |
| `ADDR` | `:8080` | Server listen address |

## Running

```bash
# Development
make run

# Or directly
IDEAS_DIR=./ideas PERSONAL_PATH=./data/personal.md FAMILY_PATH=./data/family.md go run ./cmd/dashboard

# Build binary
make build
./bin/dashboard
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

The compose file mounts `./ideas` for idea files and `./data` for the database and task files.

## Features

### Tasks
- Separate personal and family task lists
- Quick add with optional tags and priority (high/medium/low)
- Inline notes, tag, and priority editing
- Filter by tag or priority
- Expand/collapse all
- Complete/uncomplete/delete
- Move tasks between personal and family lists
- Stored as checkbox items in `personal.md` and `family.md`

### Goals
- Progress tracking with current/target and unit (e.g. 12/40 books)
- Progress bar visualisation
- Increment (+1/-1) or set absolute value
- Same priority and tag system as tasks

### Ideas
- File-per-idea with frontmatter metadata
- Tag-based categorisation (replaces fixed type dropdown)
- Triage workflow: untriaged -> parked / dropped
- Convert idea to task (tags carry over)
- Research notes per idea
- Quick add with `#tag` syntax

### Exploration
- File-per-item for open-ended explorations and research
- Tag-based categorisation with `#tag` syntax
- Markdown body with rendered detail view
- Inline editing of body, tags, and images

### Image upload
- Attach images to any task, goal, idea, or exploration
- Upload via file picker or clipboard paste (Ctrl+V in any textarea)
- MIME-based validation (PNG, JPEG, GIF, WebP only)
- Canonical extension mapping (ignores original filename extension)
- 10 MB size limit per upload

### General
- Live reload via SSE on file changes
- Catppuccin dark/light theme
- REST API with optional bearer token auth

## Task file format

Tasks and goals are stored in `personal.md` and `family.md` as flat checkbox lists. Tags are inline via `[tags: ...]`. Section headers (`## ...`) are ignored by the parser. Goals are supported in `personal.md` only.

```markdown
# Personal

- [ ] Run 5km !high [added: 2026-03-10] [tags: fitness, health]
- [ ] Reach 90kg [goal: 85.5/90 kg] [added: 2026-03-01] [tags: health]
- [ ] Document setup [tags: infra] [images: screenshot.png]
- [x] Finish book club pick [completed: 2026-03-15] [tags: books]
```

## Idea file format

Each idea is a markdown file in `IDEAS_DIR/{status}/`:

```markdown
---
tags: feature, exploration
date: 2026-03-14
images: abc123.png
---

# Idea title

Description here.
```

Status directories: `untriaged/`, `parked/`, `dropped/`. Research notes are stored separately in `research/`. Legacy files with `type:` frontmatter are automatically migrated to `tags:` on read.

## Exploration file format

Each exploration is a markdown file in `EXPLORATION_DIR/`:

```markdown
---
tags: rust, systems
date: 2026-03-16
---

# Exploring Rust for CLI tools

Notes and findings here.
```

## Routes

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Homepage (summary of all sections) |
| `GET` | `/personal` | Personal tasks page |
| `GET` | `/family` | Family tasks page |
| `GET` | `/goals` | Goals page (personal only) |
| `GET` | `/ideas` | Ideas inbox |
| `GET` | `/ideas/{slug}` | Idea detail |
| `GET` | `/exploration` | Exploration list |
| `GET` | `/exploration/{slug}` | Exploration detail |
| `POST` | `/upload` | Image upload (multipart, returns JSON) |
| `GET` | `/uploads/{filename}` | Serve uploaded images |
| `GET` | `/events` | SSE endpoint for live reload |

### API

All API routes are under `/api/v1` and require a bearer token if `DASHBOARD_API_TOKEN` is set.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/ideas` | List all ideas |
| `POST` | `/api/v1/ideas` | Create idea (JSON body) |
| `PUT` | `/api/v1/ideas/{slug}/triage` | Triage idea (park/drop/untriage) |
| `POST` | `/api/v1/ideas/{slug}/research` | Add research content |

## Backup

All user data is plain markdown files. The SQLite database is a read cache rebuilt automatically on startup -- it does not need to be backed up.

| Data | Location | Format |
|---|---|---|
| Personal tasks and goals | `PERSONAL_PATH` | Markdown file |
| Family tasks | `FAMILY_PATH` | Markdown file |
| Ideas and research | `IDEAS_DIR` | Directory of markdown files |
| Explorations | `EXPLORATION_DIR` | Directory of markdown files |
| Uploaded images | `UPLOADS_DIR` | Image files |
| Database | `DB_PATH` | SQLite (disposable cache) |

To back up the dashboard, copy the task files and ideas directory. A few options:

**Docker volume snapshot**

```bash
docker run --rm -v dashboard_data:/data -v "$(pwd)":/backup alpine \
  tar czf /backup/dashboard-backup-$(date +%F).tar.gz /data
```

**Scheduled sync to cloud storage**

Use `rclone` or `rsync` on a cron schedule to sync the data directory to S3, GCS, Backblaze, or a remote host:

```bash
# Example: sync to an rclone remote every 6 hours
0 */6 * * * rclone sync /data remote:dashboard-backup
```

**Git-based version history**

Initialise a git repo in your data directory to track changes over time:

```bash
cd /data
git init && git add -A && git commit -m "initial"
# Add a cron job to auto-commit periodically
```

## Stack

Go, chi, SQLite (modernc), goldmark, fsnotify, htmx.
