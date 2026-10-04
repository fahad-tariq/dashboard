"""Dashboard MCP server.

Exposes the dashboard REST API as MCP tools over Streamable HTTP transport.

Inbound MCP requests authenticate with MCP_TOKEN; outbound calls to the
dashboard API use DASHBOARD_API_TOKEN. Destructive tools are only registered
when MCP_ALLOW_DESTRUCTIVE=true.
"""

from __future__ import annotations

import hmac
import json as json_mod
import logging
import os
import sys
import time
from collections import OrderedDict
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from typing import Annotated, Literal

import uvicorn
from mcp.server.fastmcp import FastMCP
from mcp.server.transport_security import TransportSecuritySettings
from mcp.types import ToolAnnotations
from pydantic import Field

import client

log = logging.getLogger("dashboard-mcp")

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class Config:
    mcp_token: str
    allow_destructive: bool = False


def load_config(env: Mapping[str, str] = os.environ) -> Config:
    """Read and validate server configuration; raises client.ConfigError."""
    mcp_token = client.require_token("MCP_TOKEN", env)
    api_token = client.require_token("DASHBOARD_API_TOKEN", env)
    if hmac.compare_digest(mcp_token, api_token):
        raise client.ConfigError("MCP_TOKEN must differ from DASHBOARD_API_TOKEN")
    allow = env.get("MCP_ALLOW_DESTRUCTIVE", "").strip().lower() == "true"
    return Config(mcp_token=mcp_token, allow_destructive=allow)


# ---------------------------------------------------------------------------
# ASGI bearer token middleware
# ---------------------------------------------------------------------------


class FailedAuthLimiter:
    """Fixed-window counter of failed bearer attempts per client key.

    Tracks at most max_clients keys, evicting the least recently failed.
    """

    def __init__(
        self,
        limit: int = 10,
        window: float = 60.0,
        max_clients: int = 1024,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self.limit = limit
        self.window = window
        self.max_clients = max_clients
        self.clock = clock
        self._windows: OrderedDict[str, tuple[float, int]] = OrderedDict()

    def record_failure(self, key: str) -> bool:
        """Record a failure; returns True once key has exceeded the limit."""
        now = self.clock()
        start, count = self._windows.pop(key, (now, 0))
        if now - start >= self.window:
            start, count = now, 0
        count += 1
        self._windows[key] = (start, count)
        while len(self._windows) > self.max_clients:
            self._windows.popitem(last=False)
        return count > self.limit


class BearerAuthMiddleware:
    """ASGI middleware that validates Authorization: Bearer <token>.

    A valid token is always accepted, even from a client that is currently
    rate limited: behind Docker port publishing or a reverse proxy every
    client can share one source address, and an attacker must not be able
    to lock the real agent out.
    """

    def __init__(self, app, token: str, limiter: FailedAuthLimiter | None = None) -> None:
        self.app = app
        self.token = token.encode()
        self.limiter = limiter or FailedAuthLimiter()

    async def __call__(self, scope, receive, send):
        if scope["type"] == "lifespan":
            await self.app(scope, receive, send)
            return

        if scope["type"] != "http":
            await _send_json(send, 403, {"error": "forbidden"})
            return

        # Allow unauthenticated health checks.
        path = scope.get("path", "")
        if path == "/health":
            await _send_json(send, 200, {"status": "ok"})
            return

        # Validate bearer token.
        token = _extract_bearer(scope)
        # Both sides are bytes: compare_digest raises on non-ASCII str input.
        if not token or not hmac.compare_digest(token, self.token):
            client_addr = scope.get("client")
            key = client_addr[0] if client_addr else "unknown"
            if self.limiter.record_failure(key):
                await _send_json(send, 429, {"error": "too many failed attempts"})
            else:
                await _send_json(send, 401, {"error": "unauthorized"})
            return

        response_started = False
        original_send = send

        async def guarded_send(message):
            nonlocal response_started
            if message["type"] == "http.response.start":
                response_started = True
            await original_send(message)

        try:
            await self.app(scope, receive, guarded_send)
        except Exception:
            log.exception("unhandled exception in downstream app")
            if not response_started:
                await _send_json(original_send, 500, {"error": "internal server error"})


def _extract_bearer(scope) -> bytes | None:
    for name, value in scope.get("headers", []):
        if name == b"authorization":
            if value.startswith(b"Bearer "):
                return value[7:]
    return None


async def _send_json(send, status: int, body: dict) -> None:
    payload = json_mod.dumps(body).encode()
    await send(
        {
            "type": "http.response.start",
            "status": status,
            "headers": [
                [b"content-type", b"application/json"],
                [b"content-length", str(len(payload)).encode()],
            ],
        }
    )
    await send({"type": "http.response.body", "body": payload})


# ---------------------------------------------------------------------------
# Tool definitions (registered in build_server)
# ---------------------------------------------------------------------------

# Type aliases for readability.
TodoList = Annotated[
    Literal["personal", "family"],
    Field(description="Which list: 'personal' or 'family'"),
]
PlanList = Annotated[
    Literal["personal", "family", "house"],
    Field(description="Which list: 'personal', 'family', or 'house'"),
]
CommentaryList = Annotated[
    Literal["personal", "family", "house", "ideas"],
    Field(description="Which list: 'personal', 'family', 'house', or 'ideas'"),
]
Slug = Annotated[str, Field(description="URL-friendly item identifier (e.g. 'buy-groceries')")]
Priority = Annotated[
    Literal["high", "medium", "low", ""],
    Field(description="Priority level: 'high', 'medium', 'low', or '' (none)"),
]


async def _call(method: str, path: str, **kwargs) -> str:
    """Call the dashboard API and return a JSON string for MCP."""
    try:
        result = await client.request(method, path, **kwargs)
        return json_mod.dumps(result, indent=2)
    except client.DashboardError as exc:
        return json_mod.dumps({"error": exc.message, "status": exc.status})


def _validate(slug: str) -> str | None:
    """Validate a slug; returns an error JSON string or None."""
    try:
        client.validate_slug(slug)
        return None
    except client.DashboardError as exc:
        return json_mod.dumps({"error": exc.message, "status": exc.status})


# ---------------------------------------------------------------------------
# Todo tools (12)
# ---------------------------------------------------------------------------


async def list_todos() -> str:
    """List all todos grouped by 'personal' and 'family' lists."""
    return await _call("GET", "/todos")


async def get_todo(slug: Slug, list: TodoList) -> str:
    """Get a single todo by its slug."""
    if err := _validate(slug):
        return err
    return await _call("GET", f"/todos/{slug}", params={"list": list})


async def add_todo(
    title: Annotated[str, Field(description="Task title")],
    list: TodoList,
    body: Annotated[str, Field(description="Task body/description")] = "",
    tags: Annotated[list[str] | None, Field(description="Tags (max 10)")] = None,
    priority: Priority = "",
) -> str:
    """Create a new todo task."""
    payload: dict = {"title": title, "list": list}
    if body:
        payload["body"] = body
    if tags:
        payload["tags"] = tags
    if priority:
        payload["priority"] = priority
    return await _call("POST", "/todos", json=payload)


async def update_todo(
    slug: Slug,
    list: TodoList,
    title: Annotated[str, Field(description="New title (empty to keep current)")] = "",
    body: Annotated[str, Field(description="New body (empty to keep current)")] = "",
    tags: Annotated[list[str] | None, Field(description="New tags (null to keep current)")] = None,
    images: Annotated[list[str] | None, Field(description="Image filenames (null to keep current)")] = None,
) -> str:
    """Update a todo's title, body, tags, or images."""
    if err := _validate(slug):
        return err
    payload: dict = {"list": list}
    if title:
        payload["title"] = title
    if body:
        payload["body"] = body
    if tags is not None:
        payload["tags"] = tags
    if images is not None:
        payload["images"] = images
    return await _call("PUT", f"/todos/{slug}", json=payload)


async def complete_todo(slug: Slug, list: TodoList) -> str:
    """Mark a todo as done."""
    if err := _validate(slug):
        return err
    return await _call("POST", f"/todos/{slug}/complete", json={"list": list})


async def uncomplete_todo(slug: Slug, list: TodoList) -> str:
    """Mark a todo as not done."""
    if err := _validate(slug):
        return err
    return await _call("POST", f"/todos/{slug}/uncomplete", json={"list": list})


async def delete_todo(slug: Slug, list: TodoList) -> str:
    """Move a todo to trash (soft delete)."""
    if err := _validate(slug):
        return err
    return await _call("DELETE", f"/todos/{slug}", json={"list": list})


async def update_todo_priority(slug: Slug, list: TodoList, priority: Priority) -> str:
    """Set the priority on a todo."""
    if err := _validate(slug):
        return err
    return await _call("PUT", f"/todos/{slug}/priority", json={"priority": priority, "list": list})


async def update_todo_tags(
    slug: Slug,
    list: TodoList,
    tags: Annotated[list[str], Field(description="New set of tags (replaces existing)")],
) -> str:
    """Replace all tags on a todo."""
    if err := _validate(slug):
        return err
    return await _call("PUT", f"/todos/{slug}/tags", json={"tags": tags, "list": list})


async def add_substep(
    slug: Slug,
    list: TodoList,
    text: Annotated[str, Field(description="Sub-step text (max 500 chars)")],
) -> str:
    """Add a sub-step checkbox to a todo."""
    if err := _validate(slug):
        return err
    return await _call("POST", f"/todos/{slug}/substeps", json={"text": text, "list": list})


async def toggle_substep(
    slug: Slug,
    list: TodoList,
    index: Annotated[int, Field(ge=0, description="Zero-based sub-step index")],
) -> str:
    """Toggle a sub-step's done/undone state."""
    if err := _validate(slug):
        return err
    return await _call("PUT", f"/todos/{slug}/substeps/{index}", json={"list": list})


async def remove_substep(
    slug: Slug,
    list: TodoList,
    index: Annotated[int, Field(ge=0, description="Zero-based sub-step index")],
) -> str:
    """Remove a sub-step from a todo."""
    if err := _validate(slug):
        return err
    return await _call("DELETE", f"/todos/{slug}/substeps/{index}", json={"list": list})


# ---------------------------------------------------------------------------
# Idea tools (4)
# ---------------------------------------------------------------------------


async def list_ideas() -> str:
    """List all ideas."""
    return await _call("GET", "/ideas")


async def add_idea(
    title: Annotated[str, Field(description="Idea title")],
    tags: Annotated[list[str] | None, Field(description="Tags for categorisation")] = None,
    body: Annotated[str, Field(description="Idea body/description")] = "",
) -> str:
    """Create a new idea."""
    payload: dict = {"title": title}
    if tags:
        payload["tags"] = tags
    if body:
        payload["body"] = body
    return await _call("POST", "/ideas", json=payload)


async def triage_idea(
    slug: Slug,
    action: Annotated[
        Literal["park", "drop", "untriage"],
        Field(description="Triage action: 'park' (shelve), 'drop' (discard), or 'untriage' (reset)"),
    ],
) -> str:
    """Change an idea's triage status."""
    if err := _validate(slug):
        return err
    return await _call("PUT", f"/ideas/{slug}/triage", json={"action": action})


async def add_idea_research(
    slug: Slug,
    content: Annotated[str, Field(description="Research content to append to the idea body")],
) -> str:
    """Append research notes to an idea."""
    if err := _validate(slug):
        return err
    return await _call("POST", f"/ideas/{slug}/research", json={"content": content})


# ---------------------------------------------------------------------------
# Plan tools (5)
# ---------------------------------------------------------------------------


async def get_plan(
    date: Annotated[str, Field(description="Date in YYYY-MM-DD format (empty for today)")] = "",
) -> str:
    """Get planned items for a date, grouped by list, plus overdue items."""
    params = {}
    if date:
        params["date"] = date
    return await _call("GET", "/plan", params=params or None)


async def set_plan(
    slug: Slug,
    list: PlanList,
    date: Annotated[str, Field(description="Date in YYYY-MM-DD format (empty for today)")] = "",
) -> str:
    """Plan a todo for a specific date."""
    if err := _validate(slug):
        return err
    payload: dict = {"list": list}
    if date:
        payload["date"] = date
    return await _call("PUT", f"/plan/{slug}", json=payload)


async def clear_plan(slug: Slug, list: PlanList) -> str:
    """Remove a todo from the daily plan."""
    if err := _validate(slug):
        return err
    return await _call("DELETE", f"/plan/{slug}", json={"list": list})


async def reorder_plan(
    slugs: Annotated[list[str], Field(description="Ordered list of todo slugs")],
    list: PlanList,
) -> str:
    """Reorder planned items within a list."""
    for s in slugs:
        if err := _validate(s):
            return err
    return await _call("POST", "/plan/reorder", json={"slugs": slugs, "list": list})


async def clear_carried_plan() -> str:
    """Clear all overdue carried-over items from all lists."""
    return await _call("POST", "/plan/clear-carried")


# ---------------------------------------------------------------------------
# Commentary tools (3)
# ---------------------------------------------------------------------------


async def get_commentary(list: CommentaryList, slug: Slug) -> str:
    """Get AI commentary for an item."""
    if err := _validate(slug):
        return err
    return await _call("GET", f"/commentary/{list}/{slug}")


async def set_commentary(
    list: CommentaryList,
    slug: Slug,
    content: Annotated[str, Field(description="Commentary text (max 5000 chars)")],
) -> str:
    """Set AI commentary on an item."""
    if err := _validate(slug):
        return err
    return await _call("PUT", f"/commentary/{list}/{slug}", json={"content": content})


async def delete_commentary(list: CommentaryList, slug: Slug) -> str:
    """Delete AI commentary from an item."""
    if err := _validate(slug):
        return err
    return await _call("DELETE", f"/commentary/{list}/{slug}")


# ---------------------------------------------------------------------------
# Server and app assembly
# ---------------------------------------------------------------------------

_READ_ONLY = ToolAnnotations(readOnlyHint=True)
_DESTRUCTIVE = ToolAnnotations(readOnlyHint=False, destructiveHint=True)
# The spec defaults destructiveHint to true when readOnlyHint is false.
_WRITE = ToolAnnotations(readOnlyHint=False, destructiveHint=False)

READ_ONLY_TOOLS = [list_todos, get_todo, list_ideas, get_plan, get_commentary]
WRITE_TOOLS = [
    add_todo, update_todo, complete_todo, uncomplete_todo,
    update_todo_priority, update_todo_tags, add_substep, toggle_substep,
    add_idea, triage_idea, add_idea_research,
    set_plan, clear_plan, reorder_plan,
    set_commentary,
]
DESTRUCTIVE_TOOLS = [delete_todo, remove_substep, clear_carried_plan, delete_commentary]


def build_server(allow_destructive: bool) -> FastMCP:
    server = FastMCP(
        "Dashboard",
        stateless_http=True,
        json_response=True,
        streamable_http_path="/",
        host="0.0.0.0",
        port=9100,
        transport_security=TransportSecuritySettings(
            enable_dns_rebinding_protection=False,
        ),
    )
    for fn in READ_ONLY_TOOLS:
        server.add_tool(fn, annotations=_READ_ONLY)
    for fn in WRITE_TOOLS:
        server.add_tool(fn, annotations=_WRITE)
    if allow_destructive:
        for fn in DESTRUCTIVE_TOOLS:
            server.add_tool(fn, annotations=_DESTRUCTIVE)
    return server


def create_app(config: Config):
    """Build a fresh authenticated ASGI app.

    Each call creates a new FastMCP instance, since its session manager can
    only be run once.
    """
    server = build_server(config.allow_destructive)
    return BearerAuthMiddleware(server.streamable_http_app(), config.mcp_token)


try:
    app = create_app(load_config())
except client.ConfigError as exc:
    sys.exit(str(exc))

if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(levelname)s %(message)s")
    uvicorn.run(app, host="0.0.0.0", port=9100)
