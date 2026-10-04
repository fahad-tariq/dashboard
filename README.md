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
| `IDEAS_PATH` | `/data/ideas.md` | User 1's ideas file when `DASHBOARD_AUTH=disabled`; ignored with auth on |
| `IDEAS_DIR` | (empty) | Legacy: if set, derives `IDEAS_PATH` from parent directory |
| `UPLOADS_DIR` | `/data/uploads` | Directory for uploaded images (shared, auto-created) |
| `PERSONAL_PATH` | `/data/personal.md` | User 1's personal tasks file when `DASHBOARD_AUTH=disabled`, and the source file for `migrate-data` |
| `FAMILY_PATH` | `/data/family.md` | Shared family tasks file |
| `USER_DATA_DIR` | `/data/users` | Per-user data directory (auto-created) |
| `DB_PATH` | `/data/db/dashboard.db` | SQLite database path |
| `DASHBOARD_PASSWORD_HASH` | (empty) | Bcrypt hash for auto-creating first admin user |
| `DASHBOARD_AUTH` | `enabled` | `disabled` turns auth off for local development: every request is served as user 1 (created as `local@localhost` if missing); refused unless `ADDR` is loopback. With auth on and no users or hash, the server refuses to start |
| `DASHBOARD_TRUSTED_PROXIES` | (empty) | CIDRs or IPs of reverse proxies whose `X-Forwarded-For` is used for login rate limiting |
| `DASHBOARD_API_TOKEN` | (empty) | Bearer token for `/api/v1`; at least 32 characters, or the API is not mounted |
| `MCP_TOKEN` | (empty) | MCP sidecar only: token MCP clients send; at least 32 characters, different from `DASHBOARD_API_TOKEN` |
| `MCP_ALLOW_DESTRUCTIVE` | `false` | MCP sidecar only: `true` exposes delete and clear tools |
| `SESSION_LIFETIME` | `720h` | Session cookie lifetime (30 days) |
| `DASHBOARD_SECURE_COOKIES` | `true` | Set `false` for local HTTP development |
| `ADDR` | `:8080` | Server listen address |

The build version (git SHA) is injected at compile time via `-ldflags` and displayed in the page footer. Set via `VERSION` build arg in Docker or `make build`.

## Running

```bash
# Development
make run

# Or directly, without auth (loopback only)
DASHBOARD_AUTH=disabled ADDR=127.0.0.1:8080 DASHBOARD_SECURE_COOKIES=false \
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

The compose file mounts `./data` for the database, shared lists and uploads, and `./users` for per-user data (personal tasks, ideas). Nothing else is writable: both containers run read-only, unprivileged, with all capabilities dropped.

### Deploying behind a reverse proxy

The reference `docker-compose.yml` assumes a proxy on the same host (here Caddy with `network_mode: host`) that terminates TLS:

```caddyfile
dash.example.net {
    handle_path /mcp* {
        reverse_proxy localhost:9100
    }
    handle {
        reverse_proxy localhost:8081
    }
}
```

- Ports are published on `127.0.0.1` only. Docker-published ports bypass ufw, so a `0.0.0.0` binding would expose the plain-HTTP app to the LAN.
- The compose network has a fixed subnet (`172.30.81.0/24`). Requests through the published port reach the container from its gateway, `172.30.81.1`, which is therefore `DASHBOARD_TRUSTED_PROXIES`. Caddy sets `X-Forwarded-For` to the client address it saw (it replaces any client-supplied value unless Caddy itself has `trusted_proxies`), and the app uses the rightmost entry for login rate limiting. If another proxy such as a CDN ever sits in front of Caddy, configure Caddy's `trusted_proxies` too, or every client will look like the CDN. Change both values together if the subnet clashes with another network.
- Set `DASHBOARD_UID`/`DASHBOARD_GID` to the owner of `./data` and `./users`. Before switching an existing deployment, make sure that user owns everything (once, as root): `chown -R <uid>:<gid> data users`.
- `DASHBOARD_SECURE_COOKIES` stays `true`: the browser talks HTTPS to the proxy.

## Features

### Tasks
- Separate personal and family task lists
- Quick add with optional tags and priority (high/medium/low)
- Inline title, notes, tag, and priority editing
- Filter by tag or priority
- Expand/collapse all
- Complete/uncomplete/delete
- Move tasks between personal and family lists
- Stored as checkbox items in `personal.md` and `family.md`

### Goals
- Progress tracking with current/target and unit (e.g. 12/40 books)
- Progress bar visualisation with colour shift based on deadline proximity (green/yellow/orange/red)
- Optional deadline with pace indicator ("On pace", "Behind pace", projected completion date)
- Increment (+1/-1) or set absolute value
- Inline title editing
- Same priority and tag system as tasks

### Ideas
- Single flat file (`ideas.md`) with checkbox items and inline metadata
- Triage workflow: untriaged -> parked / dropped / converted
- Convert idea to personal task with bidirectional linkage (tags carry over, provenance preserved)
- Status badges on list cards (untriaged/parked/dropped/converted)
- Filter by tag
- Inline title and body editing
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

### Homepage insights
- Time-of-day greeting (Good morning/afternoon/evening)
- Weekly velocity ("5 completed this week, up from 3 last week")
- Completion streaks and milestone badges (10/50/100/500)
- Tag aggregation across tasks, goals, and ideas (top 5 tags with cross-section counts)
- Age badges on open tasks and untriaged ideas (fresh/ageing/stale/old)

### Search and keyboard shortcuts
- `Ctrl+K` / `Cmd+K` / `/` opens search overlay
- Full-text search across titles and body content (tasks, goals, ideas)
- `?` shows keyboard shortcut help
- `g h/t/o/i/f` for navigation (home, todos, goals, ideas, family)
- Arrow keys + Enter for search result selection

### General
- Live reload via SSE on file changes
- Catppuccin dark/light theme with WCAG AA contrast compliance
- Themed confirmation modals (replacing browser confirm dialogs)
- Task completion celebration animation
- Idea triage transition animations
- Success flash messages on all mutations
- Contextual error messages with correlation IDs for 500 errors
- Session expiry toast before login redirect
- REST API with optional bearer token auth

## Task file format

Tasks and goals are stored in `personal.md` and `family.md` as flat checkbox lists. Tags are inline via `[tags: ...]`. Section headers (`## ...`) are ignored by the parser. Goals are supported in `personal.md` only.

```markdown
# Personal

- [ ] Run 5km !high [added: 2026-03-10] [tags: fitness, health]
- [ ] Reach 90kg [goal: 85.5/90 kg] [added: 2026-03-01] [deadline: 2026-06-30] [tags: health]
- [ ] Document setup [tags: infra] [images: screenshot.png] [from-idea: document-setup-idea]
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

Status values: `untriaged` (default), `parked`, `dropped`, `converted`. The `[project: ...]` field is optional. Converted ideas include `[converted-to: task-slug]` linking to the resulting task. Body lines are indented with 2 spaces; blank lines within bodies are preserved.

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
| `GET` | `/search` | Search across tasks, goals, ideas (HTML fragment) |
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

User data is markdown under the data and users directories, plus the SQLite database, which is authoritative for users, sessions and commentary. Back up all three.

`scripts/backup.sh` (also `make backup`) writes `dashboard-backup-<timestamp>.tar.gz` containing `data/`, `users/` and a database snapshot taken with SQLite's online backup API, which is safe while the server runs. A plain `cp` or `tar` of a live WAL database is not. It checks the snapshot's integrity, prunes archives older than `RETENTION_DAYS` (default 14), and exits non-zero on any failure.

```bash
make backup                                   # ./data, ./users -> ./backups
DATA_DIR=/srv/dash/data USERS_DIR=/srv/dash/users BACKUP_DIR=/srv/dash/backups bash scripts/backup.sh
```

Run it from cron for scheduled backups, and copy the archives off the host.

**Restore**

```bash
docker compose stop dashboard
mkdir restore && tar -xzf backups/dashboard-backup-<timestamp>.tar.gz -C restore
rm -f data/dashboard.db-wal data/dashboard.db-shm
cp -a restore/data/. data/ && cp -a restore/users/. users/
cp restore/dashboard.db data/dashboard.db
chown -R <uid>:<gid> data users   # the user the container runs as
docker compose start dashboard
```

## Stack

Go, chi, SQLite (modernc), goldmark, bluemonday, fsnotify, htmx.
