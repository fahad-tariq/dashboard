# Ironclaw -> Dashboard Integration Guide

This document describes how to connect ironclaw (AWS Fargate) to the personal dashboard (local server, already on Tailscale) so ironclaw can read tasks, plan the day, manage items, and write per-item commentary.

## Architecture

```
Slack -> ironclaw (Fargate, ap-southeast-2) --Tailscale--> dashboard (local server)
                                                           http://<tailscale-hostname>:3000
```

The dashboard is already accessible via Tailscale on the local server. Ironclaw needs:
1. Tailscale installed in the Fargate task to join the tailnet
2. The dashboard API token stored as a secret
3. Tool definitions so the LLM can call the dashboard API

## Part 1: Tailscale on Fargate

Add a Tailscale sidecar container to the ironclaw Fargate task definition. Ironclaw needs outbound access only (it calls the dashboard; the dashboard never calls ironclaw).

### Option A: Tailscale sidecar container

Add to your ECS task definition (or docker-compose for local dev):

```json
{
  "name": "tailscale",
  "image": "tailscale/tailscale:latest",
  "essential": true,
  "environment": [
    {"name": "TS_AUTHKEY", "value": "<tailscale-auth-key>"},
    {"name": "TS_HOSTNAME", "value": "ironclaw"},
    {"name": "TS_STATE_DIR", "/var/lib/tailscale"},
    {"name": "TS_USERSPACE", "value": "true"}
  ],
  "linuxParameters": {
    "initProcessEnabled": true
  }
}
```

For docker-compose local dev:

```yaml
tailscale:
  image: tailscale/tailscale:latest
  hostname: ironclaw
  environment:
    - TS_AUTHKEY=${TS_AUTHKEY}
    - TS_HOSTNAME=ironclaw
    - TS_USERSPACE=true
  volumes:
    - tailscale-state:/var/lib/tailscale
  cap_add:
    - NET_ADMIN
    - NET_RAW
```

### Option B: Bake Tailscale into the ironclaw container

If you prefer a single container, add to the ironclaw Dockerfile:

```dockerfile
RUN curl -fsSL https://tailscale.com/install.sh | sh
```

And start it in the entrypoint before the main process:

```bash
tailscaled --state=/var/lib/tailscale/tailscaled.state &
tailscale up --authkey="${TS_AUTHKEY}" --hostname=ironclaw
```

### Auth key

Generate a Tailscale auth key at https://login.tailscale.com/admin/settings/keys:
- Reusable: yes (Fargate tasks restart)
- Ephemeral: yes (auto-removes stale nodes)
- Tags: tag the node for ACL scoping

Store as `TS_AUTHKEY` in your secrets (see Part 2).

### Tailscale ACL

Add to your tailnet ACL policy (https://login.tailscale.com/admin/acls):

```json
{
  "acls": [
    {
      "action": "accept",
      "src": ["tag:ironclaw"],
      "dst": ["<dashboard-tailscale-hostname>:3000"]
    }
  ]
}
```

## Part 2: Secrets

Two secrets ironclaw needs:

| Secret | Purpose | Where to store |
|--------|---------|----------------|
| `TS_AUTHKEY` | Tailscale auth key for joining the tailnet | AWS Secrets Manager or SSM Parameter Store |
| `DASHBOARD_API_TOKEN` | Bearer token for dashboard API auth | AWS Secrets Manager or SSM Parameter Store |

The `DASHBOARD_API_TOKEN` value must match the one configured on the dashboard server. Generate a strong random token:

```bash
openssl rand -hex 32
```

Set the same value on both sides:
- Dashboard: `DASHBOARD_API_TOKEN=<token>` environment variable
- Ironclaw: inject from Secrets Manager into the container environment

### Referencing from ECS task definition

```json
{
  "secrets": [
    {
      "name": "TS_AUTHKEY",
      "valueFrom": "arn:aws:secretsmanager:ap-southeast-2:ACCOUNT:secret:ironclaw/tailscale-key"
    },
    {
      "name": "DASHBOARD_API_TOKEN",
      "valueFrom": "arn:aws:secretsmanager:ap-southeast-2:ACCOUNT:secret:ironclaw/dashboard-token"
    }
  ]
}
```

## Part 3: Dashboard API Reference

**Base URL**: `http://<dashboard-tailscale-hostname>:3000/api/v1`
**Auth**: `Authorization: Bearer <DASHBOARD_API_TOKEN>`
**Content-Type**: `application/json` for all request bodies
**Rate limit**: 60 writes/minute (GET requests are unlimited)

### Task API

#### GET /todos
List all non-deleted items (tasks and goals), grouped by list. Items tagged `private` are excluded.

Response:
```json
{
  "personal": [
    {
      "slug": "fix-auth-bug",
      "title": "Fix auth bug",
      "type": "task",
      "priority": "high",
      "done": false,
      "tags": ["backend"],
      "added": "2026-03-20",
      "planned": "",
      "body": "Notes and context...",
      "sub_steps_done": 1,
      "sub_steps_total": 3,
      "list": "personal"
    }
  ],
  "family": [
    {
      "slug": "renew-rego",
      "title": "Renew car rego",
      "type": "goal",
      "priority": "",
      "done": false,
      "current": 5.0,
      "target": 10.0,
      "unit": "km",
      "deadline": "2026-04-15",
      "tags": [],
      "added": "2026-03-10",
      "planned": "2026-03-28",
      "body": "",
      "sub_steps_done": 0,
      "sub_steps_total": 0,
      "list": "family"
    }
  ]
}
```

#### POST /todos
Create a new task.

Request:
```json
{
  "title": "New task title",
  "body": "Optional notes",
  "tags": ["optional", "tags"],
  "priority": "high",
  "list": "personal"
}
```
- `title` (required, max 500 chars)
- `list` (required: `"personal"` or `"family"`)
- `priority` (optional: `"high"`, `"medium"`, `"low"`, or `""`)
- `tags` (optional, max 10 tags, max 50 chars each)
- `body` (optional, max 10000 chars)

Response: `201 Created` with the created item.

#### GET /todos/{slug}?list=personal
Get a single item. `list` query parameter required.

#### PUT /todos/{slug}
Update a task's title, body, tags, images.

Request:
```json
{
  "title": "Updated title",
  "body": "Updated notes",
  "tags": ["updated"],
  "list": "personal"
}
```

#### POST /todos/{slug}/complete
Mark a task as done.

Request: `{"list": "personal"}`

#### POST /todos/{slug}/uncomplete
Mark a task as not done.

Request: `{"list": "personal"}`

#### DELETE /todos/{slug}
Soft-delete a task (moves to trash, auto-purged after 7 days).

Request: `{"list": "personal"}`

#### PUT /todos/{slug}/priority
Set priority.

Request: `{"priority": "high", "list": "personal"}`

#### PUT /todos/{slug}/tags
Replace tags.

Request: `{"tags": ["new", "tags"], "list": "personal"}`

#### POST /todos/{slug}/substeps
Add a sub-step.

Request: `{"text": "Step description", "list": "personal"}`

#### PUT /todos/{slug}/substeps/{index}
Toggle a sub-step's done state.

Request: `{"list": "personal"}`

Index is 0-based. Returns 400 if out of bounds.

#### DELETE /todos/{slug}/substeps/{index}
Remove a sub-step.

Request: `{"list": "personal"}`

### Plan API

#### GET /plan?date=2026-03-28
List planned items for a date (defaults to today).

Response:
```json
{
  "date": "2026-03-28",
  "personal": [...],
  "family": [...],
  "overdue": [...]
}
```

#### PUT /plan/{slug}
Plan a task for a specific date.

Request:
```json
{
  "date": "2026-03-28",
  "list": "personal"
}
```
Date defaults to today if omitted.

#### DELETE /plan/{slug}
Remove a task from the plan.

Request: `{"list": "personal"}`

#### POST /plan/reorder
Reorder planned items.

Request:
```json
{
  "slugs": ["slug-1", "slug-2", "slug-3"],
  "list": "personal"
}
```

#### POST /plan/clear-carried
Clear all overdue (carried-over) items from both lists. No request body needed.

### Commentary API

Commentary is ironclaw's thoughts and suggestions about items, displayed in the dashboard UI.

#### PUT /commentary/{list}/{slug}
Write or update commentary. `list` can be `personal`, `family`, or `ideas`.

Request:
```json
{
  "content": "This task has been open for a week. Consider breaking it into sub-steps or setting a deadline."
}
```
Max 5000 chars.

#### GET /commentary/{list}/{slug}
Read commentary.

Response:
```json
{
  "slug": "fix-auth-bug",
  "list": "personal",
  "content": "This task has been open for a week..."
}
```
Returns empty `content` if no commentary exists.

#### DELETE /commentary/{list}/{slug}
Remove commentary.

### Ideas API (existing)

#### GET /ideas
List all ideas.

#### POST /ideas
Create an idea.

Request: `{"title": "Idea title", "tags": ["tag"], "body": "Details"}`

#### PUT /ideas/{slug}/triage
Triage an idea.

Request: `{"action": "park"}` (actions: `park`, `drop`, `activate`)

#### POST /ideas/{slug}/research
Append research to an idea.

Request: `{"content": "Research findings..."}`

### Error Responses

All errors return JSON:
```json
{"error": "description of the error"}
```

Status codes:
- `400` - bad input (missing fields, invalid values, out-of-bounds index)
- `401` - missing or invalid bearer token
- `404` - item not found
- `429` - rate limit exceeded (check `Retry-After` header)
- `500` - internal error

### Important Notes

- All mutation endpoints require `"list"` in the request body because slugs can collide across personal and family lists
- The `slug` is derived from the title (lowercase, hyphenated). Creating a task titled "Fix Auth Bug" produces slug `fix-auth-bug`
- Updating a title changes the slug -- any stored references to the old slug become invalid
- `type` is either `"task"` or `"goal"`. Goals have additional fields: `current`, `target`, `unit`, `deadline`
- Items tagged `private` are filtered from GET /todos responses
- Commentary content is rendered as markdown in the dashboard UI. Use markdown formatting for readability
- The dashboard uses AEST/AEDT timezone. Dates are in `YYYY-MM-DD` format

## Part 4: Suggested Ironclaw Tool Definitions

Define these as tools/functions that the LLM can call:

### Core Planning Workflow

1. **list_tasks** - `GET /todos` - see all open items
2. **plan_task** - `PUT /plan/{slug}` - schedule a task for today
3. **clear_carried** - `POST /plan/clear-carried` - dismiss yesterday's leftovers
4. **reorder_plan** - `POST /plan/reorder` - prioritise today's items
5. **get_plan** - `GET /plan` - see what's already planned

### Task Management

6. **create_task** - `POST /todos` - create a new task
7. **complete_task** - `POST /todos/{slug}/complete` - mark done
8. **update_priority** - `PUT /todos/{slug}/priority` - change priority
9. **add_substep** - `POST /todos/{slug}/substeps` - break down a task
10. **delete_task** - `DELETE /todos/{slug}` - trash a task

### Commentary

11. **write_commentary** - `PUT /commentary/{list}/{slug}` - share thoughts on an item
12. **read_commentary** - `GET /commentary/{list}/{slug}` - check existing commentary

### Morning Planning Prompt

When the user says "plan my day", ironclaw should:

1. Call `list_tasks` to see all open items
2. Call `get_plan` to see what's already planned and what's overdue
3. Optionally call `clear_carried` if there are stale carried-over items
4. Select items to plan based on priority, age, deadlines, and tags
5. Call `plan_task` for each selected item
6. Call `reorder_plan` to set the order
7. Optionally call `write_commentary` on items that need attention
