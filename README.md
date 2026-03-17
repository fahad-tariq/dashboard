# Dashboard

A personal task, goal, and idea dashboard backed by markdown files. Tasks and goals are split into personal (`personal.md`) and family (`family.md`) lists. Ideas are stored in a single `ideas.md` flat file with inline metadata. Web UI with htmx for live updates. Supports image upload and clipboard paste across all content types.

## Setup

```bash
cp .env.example .env
# Edit .env as needed
```

### Fresh install

1. Set `DASHBOARD_PASSWORD_HASH` in `.env` (generates the first admin user automatically):
   ```bash
   # Generate a bcrypt hash
   htpasswd -nbBC 10 "" 'your-password' | cut -d: -f2
   ```
2. Start the app: `docker compose up --build`
3. Log in as `admin@localhost` with your password
4. Go to `/admin/users` to create real users and update your email

### Starting over

If you want a clean slate (new database, no existing data):

```bash
docker compose down
rm data/dashboard.db    # Remove the database
docker compose up --build
```

The app auto-creates `admin@localhost` from `DASHBOARD_PASSWORD_HASH` on first start. All data entered after this point persists in Docker volumes across rebuilds.

### Migrating legacy data

If you have data from before the flat-file migration (directory-based ideas at `/data/ideas/untriaged/` etc., or explorations at `/data/explorations/`), convert them to the new format:

```bash
docker exec <container> /usr/local/bin/dashboard migrate-data --user-id 1
```

This reads old-format idea and exploration files, merges research notes into idea bodies, and writes a single `ideas.md` per user. Explorations are migrated as parked ideas. Slug collisions are handled by suffixing `-exp`.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `IDEAS_PATH` | `/data/ideas.md` | Ideas flat file (single-user mode) |
| `IDEAS_DIR` | (empty) | Legacy: if set, derives `IDEAS_PATH` from parent directory |
| `UPLOADS_DIR` | `/data/uploads` | Directory for uploaded images (shared, auto-created) |
| `PERSONAL_PATH` | `/data/personal.md` | Legacy personal tasks file (pre-multi-user) |
| `FAMILY_PATH` | `/data/family.md` | Shared family tasks file |
| `USER_DATA_DIR` | `/data/users` | Per-user data directory (auto-created) |
| `DB_PATH` | `/data/db/dashboard.db` | SQLite database path |
| `DASHBOARD_PASSWORD_HASH` | (empty) | Bcrypt hash for auto-creating first admin user |
| `DASHBOARD_API_TOKEN` | (empty) | Bearer token for API auth (optional) |
| `SESSION_LIFETIME` | `720h` | Session cookie lifetime (30 days) |
| `DASHBOARD_SECURE_COOKIES` | `true` | Set `false` for local HTTP development |
| `ADDR` | `:8080` | Server listen address |

The build version (git SHA) is injected at compile time via `-ldflags` and displayed in the page footer. Set via `VERSION` build arg in Docker or `make build`.

## Running

```bash
# Development
make run

# Or directly
IDEAS_PATH=./ideas.md PERSONAL_PATH=./data/personal.md FAMILY_PATH=./data/family.md go run ./cmd/dashboard

# Build binary
make build
./bin/dashboard
```

### CLI commands

```bash
# Create a user (bootstrap only -- use /admin/users in the browser)
./dashboard useradd --email alice@example.com --password secret123

# Migrate legacy data to a user's directory
./dashboard migrate-data --user-id 1 [--ideas-dir /old/ideas] [--explorations-dir /old/explorations]
```

## Docker

```bash
# Pass the git SHA so the footer shows the build version
VERSION=$(git rev-parse HEAD) docker compose up --build
```

The compose file mounts `./data` for the database and family tasks, and `./users` for per-user data (personal tasks, ideas).

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
- Single flat file (`ideas.md`) with checkbox items and inline metadata
- Triage workflow: untriaged -> parked / dropped
- Convert idea to personal task (tags carry over)
- Research notes stored inline in idea body
- Quick add with `#tag` syntax
- Optional project field for grouping

### Image upload
- Attach images to any task, goal, or idea
- Upload via file picker or clipboard paste (Ctrl+V in any textarea)
- MIME-based validation (PNG, JPEG, GIF, WebP only)
- Canonical extension mapping (ignores original filename extension)
- 10 MB size limit per upload

### Authentication and multi-user
- Email + password login with bcrypt, server-side sessions (SQLite-backed)
- Multi-user: each user gets isolated personal tasks, goals, and ideas
- Shared family task list visible to all users
- Two roles: `admin` (can manage users) and `user`
- Admin UI at `/admin/users` for creating, editing, and deleting users
- Self-service password change at `/account/password`
- Session invalidation on role change, password reset, and user deletion
- Rate limiting on login (5 attempts/minute per IP)
- First user auto-created from `DASHBOARD_PASSWORD_HASH` env var

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

Ideas are stored in a single `ideas.md` file with checkbox items and inline metadata:

```markdown
# Ideas

- [ ] Try Caddy instead of nginx [status: parked] [tags: infra, homelab] [project: homelabs] [added: 2026-03-14]
  Replace nginx reverse proxy with Caddy for automatic HTTPS.

  ## Research
  Caddy auto-provisions TLS certs via ACME. Simpler config than nginx.

- [ ] Dashboard mobile PWA [status: untriaged] [tags: dashboard] [added: 2026-03-16] [images: pwa-sketch.png]
  Add a manifest.json and service worker for offline support.
```

Status values: `untriaged` (default), `parked`, `dropped`. The `[project: ...]` field is optional. Body lines are indented with 2 spaces; blank lines within bodies are preserved.

## Routes

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Homepage (summary of all sections) |
| `GET` | `/todos` | Personal tasks page |
| `GET` | `/family` | Family tasks page |
| `GET` | `/goals` | Goals page (personal only) |
| `GET` | `/ideas` | Ideas list (grouped by status) |
| `GET` | `/ideas/{slug}` | Idea detail |
| `GET` | `/exploration` | Redirects to `/ideas` (301) |
| `POST` | `/upload` | Image upload (multipart, returns JSON) |
| `GET` | `/uploads/{filename}` | Serve uploaded images |
| `GET` | `/events` | SSE endpoint for live reload |
| `GET` | `/login` | Login page |
| `POST` | `/login` | Login submission |
| `POST` | `/logout` | Logout |
| `GET` | `/account` | Self-service account settings |
| `GET` | `/admin/users` | Admin: user list (admin only) |
| `GET` | `/admin/users/new` | Admin: create user form |
| `GET` | `/admin/users/{id}/edit` | Admin: edit user form |
| `GET` | `/admin/users/{id}/password` | Admin: reset password form |

### API

All API routes are under `/api/v1` and require a bearer token if `DASHBOARD_API_TOKEN` is set.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/ideas` | List all ideas |
| `POST` | `/api/v1/ideas` | Create idea (JSON body) |
| `PUT` | `/api/v1/ideas/{slug}/triage` | Triage idea (park/drop/untriage) |
| `POST` | `/api/v1/ideas/{slug}/research` | Add research content to idea body |

## Data storage

With multi-user, personal data is stored per-user under `USER_DATA_DIR/{user_id}/`. Family data is shared.

| Data | Location | Format |
|---|---|---|
| Personal tasks and goals | `USER_DATA_DIR/{id}/personal.md` | Markdown flat file |
| Ideas | `USER_DATA_DIR/{id}/ideas.md` | Markdown flat file |
| Family tasks | `FAMILY_PATH` | Shared markdown file |
| Uploaded images | `UPLOADS_DIR` | Shared image files |
| Database | `DB_PATH` | SQLite (users, sessions, tracker cache) |

## Backup

All user data is plain markdown files. The SQLite database stores users, sessions, and a tracker cache. The tracker cache is rebuilt from markdown on startup, but the users table is authoritative.

To back up the dashboard, copy the markdown files and database. A few options:

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

Go, chi, SQLite (modernc), goldmark, bluemonday, fsnotify, htmx.
