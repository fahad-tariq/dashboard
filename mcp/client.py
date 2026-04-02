"""Dashboard REST API client.

Module-level httpx.AsyncClient singleton -- must NOT live in a FastMCP
lifespan because stateless_http=True runs lifespan per-request.
"""

import os
import re
import sys

import httpx

_SLUG_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")


class DashboardError(Exception):
    """Raised when the dashboard API returns a non-2xx response or is unreachable."""

    def __init__(self, status: int, message: str) -> None:
        self.status = status
        self.message = message
        super().__init__(f"HTTP {status}: {message}")


def _init_token() -> str:
    token = os.environ.get("DASHBOARD_API_TOKEN", "")
    if not token:
        sys.exit("DASHBOARD_API_TOKEN is not set")
    if len(token) < 32:
        sys.exit("DASHBOARD_API_TOKEN must be at least 32 characters")
    return token


TOKEN = _init_token()

_client = httpx.AsyncClient(
    base_url=os.environ.get("DASHBOARD_API_URL", "http://dashboard:8080/api/v1"),
    headers={"Authorization": f"Bearer {TOKEN}"},
    timeout=30.0,
)


def validate_slug(slug: str) -> None:
    """Reject slugs that could cause path traversal or unexpected routing."""
    if not slug or not _SLUG_RE.match(slug):
        raise DashboardError(400, f"invalid slug: {slug!r}")


async def request(
    method: str,
    path: str,
    json: dict | None = None,
    params: dict | None = None,
) -> dict:
    """Send a request to the dashboard API and return parsed JSON."""
    try:
        resp = await _client.request(method, path, json=json, params=params)
    except httpx.HTTPError:
        raise DashboardError(502, "dashboard API unavailable")

    if resp.status_code >= 400:
        try:
            body = resp.json()
            msg = body.get("error", resp.text)
        except Exception:
            msg = resp.text or f"HTTP {resp.status_code}"
        raise DashboardError(resp.status_code, msg)

    if not resp.content:
        return {"status": "ok"}

    return resp.json()
