# Dashboard

A personal dashboard for tasks, goals, ideas and house upkeep, backed by markdown files. The homepage is a daily planner over those tasks. Server-rendered Go with htmx; open tabs update live when a file changes, including edits made outside the app.

Each feature area (todos, family, ideas, house) is a module: one package plus one registration line. See [Adding a module](#adding-a-module).

## Setup

Docker Compose is the supported deployment. Compose reads `.env` only for the values it substitutes (`DASHBOARD_PASSWORD_HASH`, `DASHBOARD_API_TOKEN`, `SESSION_LIFETIME`, `DASHBOARD_SECURE_COOKIES`, `DASHBOARD_TIMEZONE`, `DASHBOARD_PORT`, `DASHBOARD_UID`/`DASHBOARD_GID`, `VERSION`). Paths, `ADDR` and `DASHBOARD_TRUSTED_PROXIES` are fixed in `docker-compose.yml`; edit them there.

### Fresh install

1. Copy `.env.example` to `.env`, then set `DASHBOARD_PASSWORD_HASH` (it creates the first admin):
   ```bash
   htpasswd -nbBC 10 "" 'your-password' | cut -d: -f2
   ```
   Leave `DASHBOARD_API_TOKEN` empty unless you want the REST API.
2. Create the data directories owned by the user the container runs as (`DASHBOARD_UID`/`DASHBOARD_GID`, default 10001), or Docker creates them owned by root and every write fails:
   ```bash
   mkdir -p data users && sudo chown 10001:10001 data users
   ```
3. Start it: `docker compose up --build dashboard`.
4. Open `http://127.0.0.1:8081` (`DASHBOARD_PORT`) and log in as `admin@localhost`. Behind HTTPS keep `DASHBOARD_SECURE_COOKIES=true`. For plain HTTP set it to `false`: browsers drop `Secure` cookies over HTTP, and only some treat `localhost` as an exception.
5. Change your email and add users at `/admin/users`.

### Starting over

To reset accounts and sessions but keep your markdown data:

```bash
docker compose down
rm -f data/dashboard.db data/dashboard.db-wal data/dashboard.db-shm
docker compose up -d dashboard
```

`admin@localhost` is recreated from `DASHBOARD_PASSWORD_HASH`. Tasks, ideas and house data live in `data/` and `users/` (bind mounts) and are untouched; delete those directories too for an empty install.

## Configuration

These are the binary's defaults. Under Compose, the paths are set in `docker-compose.yml` to `/data/db/...` (host `./data`) and `/data/users` (host `./users`).

| Variable | Default | Description |
|---|---|---|
| `ADDR` | `:8080` | Listen address |
| `DB_PATH` | `/data/db/dashboard.db` | SQLite database: users, sessions, schema version |
| `USER_DATA_DIR` | `/data/users` | Per-user files: `{id}/personal.md`, `{id}/ideas.md` |
| `FAMILY_PATH` | `/data/family.md` | Shared family tasks. Its directory is also where modules keep their own shared files |
| `MAINTENANCE_PATH` | `/data/maintenance.md` | Shared house maintenance |
| `HOUSE_PROJECTS_PATH` | `/data/house-projects.md` | Shared house projects |
| `UPLOADS_DIR` | `/data/uploads` | Uploaded images (shared) |
| `PERSONAL_PATH` | `/data/personal.md` | User 1's tasks in no-auth mode. With auth on it is unused, but a skeleton file is still created there |
| `IDEAS_PATH` | `/data/ideas.md` | User 1's ideas in no-auth mode. Same caveat |
| `IDEAS_DIR` | (empty) | Legacy: if set and `IDEAS_PATH` is not, `IDEAS_PATH` becomes `ideas.md` beside it |
| `DASHBOARD_TIMEZONE` | the process's local zone (UTC in the container) | IANA zone for "today", planner dates, age badges and the digest. Compose defaults it to `Australia/Sydney` |
| `DASHBOARD_PASSWORD_HASH` | (empty) | Bcrypt hash; creates `admin@localhost` when no user can log in |
| `DASHBOARD_AUTH` | `enabled` | `disabled` serves every request as user 1 (created as `local@localhost`), for local development; refused unless `ADDR` is loopback. With auth on and no users or hash, the server refuses to start |
| `DASHBOARD_TRUSTED_PROXIES` | (empty) | CIDRs or IPs of proxies whose rightmost `X-Forwarded-For` entry is used as the client IP for login and bearer-token rate limiting |
| `DASHBOARD_API_TOKEN` | (empty) | Optional break-glass bearer token for `/api/v1`, acting as user 1; at least 32 characters, and a shorter value is ignored with a warning. Device tokens minted on `/account` are the normal way in |
| `SESSION_LIFETIME` | `720h` | Session lifetime |
| `DASHBOARD_SECURE_COOKIES` | `true` | `false` for plain-HTTP development. Any value that is not a boolean stops the server at start-up |
| `DASHBOARD_PORT` | `8081` | Compose only: loopback port published for the dashboard |
| `DASHBOARD_UID`, `DASHBOARD_GID` | `10001` | Compose only: user the container runs as; must own `./data` and `./users` |
| `VERSION` | `dev` | Build arg: the git SHA shown in the footer (`make build` sets it) |

## Running locally

Requires Go 1.27 (see `go.mod`). The defaults point at `/data`, so set every path. This runs without auth against a scratch directory that git ignores:

```bash
DEV=./.dev; mkdir -p "$DEV"
DASHBOARD_AUTH=disabled ADDR=127.0.0.1:8080 DASHBOARD_SECURE_COOKIES=false \
  DB_PATH="$DEV/dashboard.db" USER_DATA_DIR="$DEV/users" UPLOADS_DIR="$DEV/uploads" \
  PERSONAL_PATH="$DEV/personal.md" IDEAS_PATH="$DEV/ideas.md" FAMILY_PATH="$DEV/family.md" \
  MAINTENANCE_PATH="$DEV/maintenance.md" HOUSE_PROJECTS_PATH="$DEV/house-projects.md" \
  go run ./cmd/dashboard
```

Do not point it at `./data` or `./users`: those are the Compose bind mounts. `make run` is `go run` with the defaults, so it needs the same variables. `make build` writes `bin/dashboard`.

### CLI commands

`useradd` reads only `DB_PATH`.

```bash
# Create a user (afterwards use /admin/users)
./bin/dashboard useradd --email alice@example.com --password secret123
```

## Docker

```bash
# Pass the git SHA so the footer shows the build version (same short form as make build)
VERSION=$(git rev-parse --short HEAD) docker compose up --build dashboard
```

The compose file mounts `./data` for the database, shared lists and uploads, and `./users` for per-user data (personal tasks, ideas). Nothing else is writable: the container runs read-only, unprivileged, with all capabilities dropped.

### Deploying behind a reverse proxy

The reference `docker-compose.yml` assumes a proxy on the same host (here Caddy with `network_mode: host`) that terminates TLS:

```caddyfile
dash.example.net {
    reverse_proxy localhost:8081
}
```

- Ports are published on `127.0.0.1` only. Docker-published ports bypass ufw, so a `0.0.0.0` binding would expose the plain-HTTP app to the LAN.
- The compose network has a fixed subnet (`172.30.81.0/24`). Requests through the published port reach the container from its gateway, `172.30.81.1`, which is therefore `DASHBOARD_TRUSTED_PROXIES`. Caddy sets `X-Forwarded-For` to the client address it saw (it replaces any client-supplied value unless Caddy itself has `trusted_proxies`), and the app uses the rightmost entry for login rate limiting. If another proxy such as a CDN ever sits in front of Caddy, configure Caddy's `trusted_proxies` too, or every client will look like the CDN. Change both values together if the subnet clashes with another network.
- Set `DASHBOARD_UID`/`DASHBOARD_GID` to the owner of `./data` and `./users`. Before switching an existing deployment, make sure that user owns everything (once, as root): `chown -R <uid>:<gid> data users`.
- `DASHBOARD_SECURE_COOKIES` stays `true`: the browser talks HTTPS to the proxy.

## Features

### Tasks and goals
- Personal (`/todos`) and family (`/family`) lists; goals (`/goals`) are personal only
- Quick add with tags and priority (high, medium, low), from fields or typed into the title as `#tag` and `!high`; inline edit of title, notes, tags and priority
- Sub-steps as body checkboxes, with progress in the row; a sub-step can be promoted to its own task
- Filter by tag, priority or "stale" (two weeks or older); on wide screens the filters sit in a left rail with counts
- Select mode for bulk complete, plan, tag, priority and trash
- Move tasks between personal and family
- Goals track current/target with a unit, an optional deadline and a pace indicator
- Personal and family tasks take an optional due date, shown as a row badge ("due Fri", "overdue 2d"); due dates never add a task to the plan

### Daily planner
- The homepage shows today's plan, with tasks from personal, family and house projects
- Plan a task from its row ("today"), the homepage picker or in bulk; unfinished plans carry over, labelled with the day they came from
- Tick and untick on the plan; select mode completes, moves to tomorrow, drops or trashes several rows at once
- Tasks due today or overdue say so in their plan row, and a "Due soon" widget lists up to 5 tasks overdue or due within 3 days (the count covers all), each with a "Plan today" button, or a "planned" badge once it is on today's plan
- Reorder by drag-and-drop or the up/down buttons; `/plan/calendar` shows week and month views, and the week view reschedules by drag
- Module widgets (open tasks oldest first, goals, overdue maintenance, untriaged ideas, tags) sit beside the plan on wide screens
- `/digest` summarises activity for this week, last week or this month

### Ideas
- Triage: untriaged, parked, dropped or converted
- Convert to a personal, family or house task; tags carry over and both sides link to each other
- Research notes and rich markdown bodies (blank lines preserved)

### House
- Recurring maintenance with a cadence (`[cadence: 6m]`), a dated log and overdue status
- Projects with status (todo, active, done, drop), budget and actual cost

### Everywhere
- Trash with undo: deleting shows a toast with an undo button (or press `u`); trashed items are purged after 7 days
- Images on any item via file picker or clipboard paste, with captions (PNG, JPEG, GIF, WebP; 10 MB)
- Search across tasks, goals, ideas and house (`/`, `Ctrl+K` or `Cmd+K`)
- Keyboard: `g` then `h` home, `t` todos, `o` goals, `i` ideas, `u` house, `f` family, `d` digest, `c` calendar; `j`/`k` move between rows; `?` lists them all
- Three styles (Cards, Paper, Document), each in light and dark, chosen per device from the nav. All six meet WCAG 2.2 AA contrast, checked by tests
- Changes appear in other open tabs without a reload, and without collapsing expanded rows

### Accounts and security
- Email and password login (bcrypt), server-side sessions in SQLite
- Login rate limit of 5 attempts per minute per IP, plus a growing per-account delay (never a lockout)
- Cross-origin POSTs rejected; CSP and other security headers on every response
- Per-user personal tasks, goals and ideas; family and house lists are shared. Admin UI at `/admin/users`, password change on `/account`
- API tokens per device, minted and revoked on `/account` and stored only as SHA-256 hashes

## File formats

All lists are markdown checkbox items with inline metadata. Body lines are indented by 2 spaces; indented checkboxes are sub-steps (or log entries for maintenance), not new items.

Every item ends with `[id: xxxxxxxx]`: 8 characters of consonants (no `y`) and digits. The app adds an ID to any item without one, and repairs duplicates within a file, whenever it loads or writes the file; URLs and links between ideas and tasks use the ID, so it survives renames and moves. Titles cannot set metadata: the app strips anything shaped like an inline tag from every title it writes.

The files are safe to edit by hand, but the app rewrites the whole file on its next change: section headings (`## ...`) outside item bodies are discarded, blank lines are dropped from task and maintenance bodies, and extra indentation from task bodies (ideas keep both), and inline metadata is put back in a fixed order.

### Tasks and goals (`personal.md`, `family.md`)

```markdown
# Personal

- [ ] Run 5km !high [added: 2026-03-10] [deadline: 2026-10-09] [planned: 2026-10-06] [tags: fitness, health] [id: r5kmx7q2]
  - [x] Buy shoes
  - [ ] Map a route
- [ ] Reach 90kg [goal: 85.5/90 kg] [added: 2026-03-01] [deadline: 2026-06-30] [tags: health] [id: g90kgz4m]
- [ ] Document setup [from-idea: c4ddxv3n] [tags: infra] [images: screenshot.png|Rack layout] [id: d0csr8p1]
- [x] Finish book club pick [completed: 2026-03-15] [tags: books] [id: bkc1zbp9]
- [ ] Old errand [added: 2026-01-02] [deleted: 2026-10-01] [id: 0ldrrnd5]
```

Goals are supported in `personal.md` only. On a task `[deadline:]` is the due date; on a goal it is the target date. `[plan-order: N]` records manual plan order.

### Ideas (`ideas.md`)

```markdown
# Ideas

- [ ] Try Caddy instead of nginx [status: parked] [tags: infra, homelab] [project: homelabs] [added: 2026-03-14] [id: c4ddxv3n]
  Replace nginx reverse proxy with Caddy for automatic HTTPS.

  ## Research
  Caddy auto-provisions TLS certs via ACME.
```

Status values: `untriaged` (default), `parked`, `dropped`, `converted`. Converted ideas carry `[converted-to: <task id>]`, and the task carries `[from-idea: <idea id>]`. Blank lines in bodies are preserved.

### House (`maintenance.md`, `house-projects.md`)

```markdown
# Maintenance

- [ ] Clean gutters [cadence: 6m] [tags: outdoor] [added: 2025-11-01] [id: gtt3rs6m]
  Use the long ladder from the shed.
  - [x] 2026-09-14 - Lots of leaves after the storm
  - [x] 2026-03-10
```

Cadence is `Nd`, `Nw`, `Nm` or `Ny`. Log entries are newest first; the next due date counts from the first one, and an item with no log entries shows as overdue. Projects use the task format plus `[status: todo|active|done|drop]`, `[budget: N]` and `[actual: N]`.

## Routes

Browser routes need a session (or no-auth mode). Each list's mutations are POSTs under its own path; `test/testdata/routes_auth.golden` is the full list.

| Path | Page |
|---|---|
| `/` | Homepage and daily planner |
| `/todos`, `/family`, `/goals` | Task and goal lists |
| `/ideas`, `/ideas/{id}` | Ideas list and detail |
| `/house` | Maintenance and projects |
| `/plan/calendar` | Planner week and month views |
| `/digest` | Activity digest |
| `/search?q=` | Search results (HTML fragment) |
| `/events` | Server-sent events for live refresh |
| `/login`, `/account`, `/admin/users` | Accounts (logout and password change are POSTs) |
| `/upload`, `/uploads/*` | Image upload and serving |

`/personal`, `/exploration` and `/exploration/*` redirect to `/todos` and `/ideas`.

### API

Always mounted under `/api/v1`. Send `Authorization: Bearer <token>` (the header only: a token in the URL or a session cookie is not accepted, and a bearer token does not open browser pages). Two kinds of token act as user 1:

- **Device tokens.** On `/account`, under "API tokens", name a device and press "Create token". The token (64 hex characters) is shown once, so copy it then; the server keeps only its SHA-256 hash. The list shows when each token was created and last used, and "revoke" stops a token at once. Changing the password does not revoke tokens. Only user 1's tokens are accepted, matching the rest of the API. In no-auth mode anyone who can reach the loopback address can mint one; use it only with development databases.
- **`DASHBOARD_API_TOKEN`**, if set to 32 or more characters: a break-glass token for when no device token is at hand.

With neither, every request gets 401. The start-up log says `API mounted` with `env_token` and `device_tokens` (the count), and each write logs `api write` with the token's name, method, route and status, never the token or an item's title. Writes (anything but GET, HEAD and OPTIONS) share one limit of 60 per minute across all callers, and more than 10 bad tokens per minute from one IP get 429. Request and response bodies are JSON; every error, 401 and 429 included, is `{"error": "..."}`.

Items are addressed by their ID (`{id}` is 8 characters from `bcdfghjklmnpqrstvwxz0123456789`; anything else gets the router's plain-text 404). Todo routes need a list: `personal` (or its alias `todos`) or `family`; anything else, including `house`, gets a 400. `GET /todos/{id}` takes it as `?list=`; every other todo route that names an ID takes `"list"` in the JSON body. Plan routes take the same names plus `house`, so one list name, `personal`, works for both a create and the plan request after it.

**Responses.** Every todo and plan item has the same shape, and every idea another; `test/testdata/api/` holds a request and response for each route, which `TestAPIGolden` keeps current. An item carries its `list`, `type` (`task` or `goal`), `tags` and `images` (always arrays; an image is `filename` or `filename|caption`), `body` (sub-step lines included) and `sub_steps` as `[{"text", "done"}]`, `deadline`, `planned` and `plan_order`, and `deleted` (the date) only when it is in the trash; goals add `current`, `target` and `unit`, house projects `status`, `budget` and `actual`. Every route that changes items answers with them as stored: one item, or `{"items": [...]}` for reorder and clear-carried. `GET /todos` returns `{"personal": [...], "family": [...]}` with goals and done items. `GET /plan` returns `date`, `today` (the server's date in `DASHBOARD_TIMEZONE`), `personal`, `family`, `house`, and `overdue`: open tasks from every list planned before `date`, each keeping its `planned` day.

**Creating.** `POST /todos` and `POST /ideas` accept an optional `id` (8 characters from the alphabet above). A new one is used and answered 201; one the list already holds, trashed or not, writes nothing and answers 200 with the stored item, so a client that retries after a lost response creates one item. Any other non-empty `id` is a 400.

**Quick-add syntax.** The title of a new task (API or web form, not goals, house projects or ideas) may carry `#tag` and `!priority` (`!high`, `!medium`, `!low`, any case, with a space allowed after the `!` as dictation writes it). Tags are taken from the title only when the request sends no `tags`, and the priority only when it sends no `priority` (or an empty one); a trailing `.`, `,`, `;`, `:`, `!` or `?` on such a word is dropped. "Buy milk #groceries ! High" becomes "Buy milk", tag `groceries`, priority high. `#42` becomes tag `42`; a lone `#` or `!` stays in the title.

| Method | Path | Body | Description |
|---|---|---|---|
| `GET` | `/api/v1/todos` | | List personal and family items |
| `POST` | `/api/v1/todos` | `title, list`; optional `id, body, tags, priority` | Add a task |
| `GET` | `/api/v1/todos/{id}?list=` | | Get one item, trashed or not |
| `PUT` | `/api/v1/todos/{id}` | `list`; any of `title, body, tags, images, deadline` | Change those fields; omitted fields and an empty title keep their current values, and an empty `body`, `tags`, `images` or `deadline` clears it. The ID does not change |
| `DELETE` | `/api/v1/todos/{id}` | `list` | Move to the trash |
| `POST` | `/api/v1/todos/{id}/complete` | `list` | Complete |
| `POST` | `/api/v1/todos/{id}/uncomplete` | `list` | Reopen |
| `PUT` | `/api/v1/todos/{id}/priority` | `priority, list` | Set priority |
| `PUT` | `/api/v1/todos/{id}/tags` | `tags, list` | Set tags (at most 10) |
| `POST` | `/api/v1/todos/{id}/substeps` | `text, list` | Add a sub-step |
| `PUT` | `/api/v1/todos/{id}/substeps/{index}` | `list` | Toggle a sub-step (0-based) |
| `DELETE` | `/api/v1/todos/{id}/substeps/{index}` | `list` | Remove a sub-step |
| `GET` | `/api/v1/ideas` | | List ideas not in the trash |
| `POST` | `/api/v1/ideas` | `title`; optional `id, body, tags` | Add an idea |
| `PUT` | `/api/v1/ideas/{id}/triage` | `action`: `park`, `drop` or `untriage` | Triage |
| `POST` | `/api/v1/ideas/{id}/research` | `content` | Append to the body, adding a `## Research` heading if there is none |
| `GET` | `/api/v1/plan?date=` | | Plan for a date (default today) |
| `PUT` | `/api/v1/plan/{id}` | `list`; optional `date` (default today) | Plan a task |
| `DELETE` | `/api/v1/plan/{id}` | `list` | Unplan a task |
| `POST` | `/api/v1/plan/reorder` | `ids, list` | Set plan order; answers with the items in that order |
| `POST` | `/api/v1/plan/clear-carried` | | Unplan every carried-over task; answers with them |

## Capture from an iPhone

Three Shortcuts call the API. Before building them, mint a token named after the phone on `/account` and keep the page open to copy it. In each "Get Contents of URL" action, open "Show More": set the method, add a header `Authorization` with the value `Bearer ` followed by the token, and choose Request Body `JSON` where there is one. The phone reaches `https://dash.home.caalador.net` only over the home Wi-Fi or Tailscale.

**Add task** (Siri: "Add task"; also the share sheet):
1. Receive Text input from Share Sheet; if there is no input, Ask For Input (Text, prompt "Task?"). Dictation can carry tags and priority: "Buy milk hashtag groceries exclamation mark high" arrives as "Buy milk #groceries ! High" and becomes "Buy milk", tag `groceries`, priority high.
2. Choose from Menu "Priority" with None, High, Medium, Low. In High, add a Text action `!high` (likewise `!medium`, `!low`); in None, add an empty Text action, because a branch with no action passes its input on as the Menu Result.
3. Text: the Shortcut Input (or the Ask For Input answer), a space, then the Menu Result. The server reads the marker from the title because the request sends no `priority`.
4. Get Contents of URL `https://dash.home.caalador.net/api/v1/todos`, method POST, JSON body `title` = that Text, `list` = `personal`.
5. Get Dictionary Value `error` from Contents of URL; If it has any value, Show Alert with it and Stop This Shortcut.
6. Get Dictionary Value `id` (set variable `TaskID`) and `title` (set variable `Title`).
7. Choose from Menu "Plan for today?": Yes runs Get Contents of URL `https://dash.home.caalador.net/api/v1/plan/` followed by `TaskID`, method PUT, JSON body `list` = `personal`; No does nothing.
8. Show Notification "Added: " followed by `Title`.

For the Action button (Settings > Action Button > Shortcut), duplicate it as "Quick task", delete steps 2, 3 and 7, and set step 4's `title` to the Shortcut Input (or the Ask For Input answer), since the Text it pointed at is gone. A press then asks one question, and dictated `#tag` and `! High` still work.

**Capture idea** (share sheet from Safari or any app sharing a link or text):
1. Receive URLs, Safari web pages and Text input from Share Sheet.
2. If the Shortcut Input is a URL or web page, Get Details of Safari Web Page: Name, from it; otherwise use the Shortcut Input as the name. (Get Details of Safari Web Page may fail on plain text, hence the If.)
3. Get Contents of URL `https://dash.home.caalador.net/api/v1/ideas`, method POST, JSON body `title` = the name, `body` = the Shortcut Input.
4. Get Dictionary Value `error` from Contents of URL; If it has any value, Show Alert with it and Stop This Shortcut.
5. Show Notification "Idea saved".

**What's on today** (Siri: "What's on today"):
1. Get Contents of URL `https://dash.home.caalador.net/api/v1/plan`, method GET (no body).
2. Get Dictionary Value `error` from Contents of URL; If it has any value, Speak Text "The dashboard said: " followed by it and Stop This Shortcut, so a failed call is not read out as an empty day.
3. For each of `personal`, `family`, `house` and `overdue`: Get Dictionary Value of that key, Repeat with Each item: Get Dictionary Value `title` from the Repeat Item and Add to Variable `Titles`.
4. If `Titles` has any value, Speak Text "Today: " followed by `Titles` (Combine Text with ", "); otherwise Speak Text "Nothing planned".

Plan items include ones already done (each has `done`); overdue items are the ones carried from earlier days and do not repeat in the other lists. The steps above have not yet been tried on a phone; the action names follow iOS 18.

**Things to know.**
- The token lives inside the Shortcut and syncs through iCloud with it. Mint one token per device, never share or export these Shortcuts, and if one is shared by mistake, revoke its token on `/account` and mint a new one.
- Siri can run these Shortcuts from the locked phone.
- With Tailscale down off the home Wi-Fi, a run fails and only that capture is lost; run it again.
- A 429 means too many writes (60 a minute for everything) or too many bad tokens from one address; wait a minute.

## Data storage

| Data | Location | Format |
|---|---|---|
| Personal tasks and goals | `USER_DATA_DIR/{id}/personal.md` | Markdown |
| Ideas | `USER_DATA_DIR/{id}/ideas.md` | Markdown |
| Family tasks | `FAMILY_PATH` | Markdown, shared |
| House maintenance and projects | `MAINTENANCE_PATH`, `HOUSE_PROJECTS_PATH` | Markdown, shared |
| Uploaded images | `UPLOADS_DIR` | Image files, shared |
| Users and sessions | `DB_PATH` | SQLite |

Markdown is the source of truth and is safe to edit by hand while the app runs: writes are atomic (temp file, fsync, rename), and the file watcher picks up outside edits and refreshes open tabs. In no-auth mode, user 1's files are `PERSONAL_PATH` and `IDEAS_PATH` instead.

## Adding a module

A module is a package in `internal/modules/<name>/` plus templates in `web/templates/<Manifest.Templates, or the ID>/`, registered with one line in the `modules` list in `internal/app/app.go`. Copy `internal/module/moduletest` and `web/templates/moduletest/page.html`: together they are a complete working example (page, quick-add, nav item, shortcut, search, widget, API route and watched file). `TestModuleContract` tests only that fixture, so write equivalent tests for a new module.

1. Implement `module.Module`. `Manifest()` returns the ID, the nav items (label, path, order, group, shortcut key), the route prefixes, and optionally `Flash`/`FlashErrors` messages and a `Templates` directory. `Routes(r)` registers browser routes on an authenticated, cross-origin-protected router.
2. Add any optional capabilities:
   - `APIRouter`: routes under `/api/v1/<module-id>`
   - `Watcher`: `WatchSpec`s naming exact files; the reload function returns whether the file changed
   - `Searcher`: results for the search overlay
   - `HomeWidget`: data for homepage cards (title, count, up to 5 items, link)
3. Build from the `module.Deps` the factory receives: renderer (`Render.Page`), location, config, `DataDir` for shared files, and `Publish(id)`, which tells open pages the module changed. Keep the module's storage in its own service and call `Publish` after each write; the core services registry only covers the built-in lists.
4. Each page file defines `{{define "content"}}`. Its refreshing container carries `data-live hx-select="[data-live]" hx-target="this" hx-swap="morph" hx-push-url="false"` and includes `{{template "live-refresh" (dict "Path" "/<page>" "Trigger" "sse:changed:<module-id>")}}`. Use the shared partials in `web/templates/_components/`. Templates must not use inline `on*` handlers.
5. Regenerate the route goldens (`go test ./test -run TestRouteGolden -update`) and check the diff.

Constraints checked at start-up:
- IDs, route prefixes and shortcut keys must be unique, and every route and nav path must sit under the module's own prefixes.
- Core prefixes (`module.ReservedPrefixes`) and keys `h c d / ? n k` are reserved.
- At most 6 primary nav links, and the built-ins already use all 6, so a new module's nav item must use `module.More`.

Not automatic yet: the trash purge loop and select mode only know the built-in lists (see `docs/backlog.md`). A module with its own JavaScript adds a file to `web/static/` (assets are one flat directory).

## Development

| Command | What it does |
|---|---|
| `make lint` | golangci-lint |
| `make test` | Go tests with the race detector (`INTEGRATION=1` adds the concurrent-reader test) |
| `make vuln` | govulncheck |
| `make e2e` | Builds and starts the server against seed data, then runs Playwright (accessibility scans in every style and theme) |
| `make cover`, `make bench` | Coverage summary; `BenchmarkMutate200` with benchstat |

CI runs lint, vuln, the race suite and Playwright on pushes to `main` and on pull requests. Images build after all of them pass, and are pushed only from `main`. Screenshots of every page in every style run as a separate job and upload as the `screenshots` artefact. To re-run part of the browser suite on a pushed branch: `gh workflow run e2e-grep.yml --ref <branch> -f grep="search"` (without `--ref` it runs on the default branch).

## Backup

User data is markdown under the data and users directories, plus the SQLite database, which is authoritative for users and sessions. Back up all three.

`scripts/backup.sh` (also `make backup`) writes `dashboard-backup-<timestamp>.tar.gz` containing `data/`, `users/` and a database snapshot taken with SQLite's online backup API, which is safe while the server runs. A plain `cp` or `tar` of a live WAL database is not. It checks the snapshot's integrity, prunes archives older than `RETENTION_DAYS` (default 14), and exits non-zero on any failure.

```bash
make backup                                   # ./data, ./users -> ./backups
DATA_DIR=/srv/dash/data USERS_DIR=/srv/dash/users BACKUP_DIR=/srv/dash/backups bash scripts/backup.sh
```

Run it from the repo root (the defaults are relative) or set the directories, from cron for scheduled backups, and copy the archives off the host. It expects the database at `DATA_DIR/dashboard.db`. The restore below assumes the default directory names `data` and `users`, which become the archive's top-level folders.

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

Go, chi, SQLite (modernc), goldmark, bluemonday, fsnotify, htmx with the SSE and idiomorph extensions. Playwright and axe-core for browser tests.
