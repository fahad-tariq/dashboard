"""Smoke tests for the Dashboard MCP server.

Run: pip install -r requirements-dev.txt && pytest test_smoke.py -v

Uses respx to mock the dashboard API so tests run without a live server.
"""

import json
import os

import httpx
import pytest
import respx

# Set required env before importing server modules.
os.environ.setdefault("DASHBOARD_API_TOKEN", "test-token-that-is-at-least-thirty-two-characters-long")
os.environ.setdefault("MCP_TOKEN", "test-mcp-token-that-is-at-least-thirty-two-characters")
os.environ.setdefault("DASHBOARD_API_URL", "http://dashboard:8080/api/v1")
os.environ.pop("MCP_ALLOW_DESTRUCTIVE", None)
_API_TOKEN = os.environ["DASHBOARD_API_TOKEN"]
_MCP_TOKEN = os.environ["MCP_TOKEN"]

from starlette.testclient import TestClient  # noqa: E402

import client  # noqa: E402
import server  # noqa: E402
from server import app  # noqa: E402

_DESTRUCTIVE_NAMES = {"delete_todo", "remove_substep", "clear_carried_plan", "delete_commentary"}
_READ_ONLY_NAMES = {"list_todos", "get_todo", "list_ideas", "get_plan", "get_commentary"}


@pytest.fixture(scope="module")
def cli():
    """Module-scoped client with lifespan.

    The MCP StreamableHTTPSessionManager.run() can only be called once per
    instance, so we must keep a single TestClient alive for all tests.
    """
    with TestClient(app, raise_server_exceptions=False) as client:
        yield client


@pytest.fixture(scope="module")
def cli_destructive():
    """Module-scoped client for a separate app built with destructive tools enabled."""
    destructive_app = server.create_app(server.Config(mcp_token=_MCP_TOKEN, allow_destructive=True))
    with TestClient(destructive_app, raise_server_exceptions=False) as client:
        yield client


@pytest.fixture
def cli_no_lifespan():
    """Per-test client WITHOUT lifespan, on a fresh app so failed-auth
    counters do not leak between tests. For auth-rejection tests that
    never reach the MCP session manager."""
    fresh = server.create_app(server.Config(mcp_token=_MCP_TOKEN))
    return TestClient(fresh, raise_server_exceptions=False)


def _auth_headers():
    return {
        "Authorization": f"Bearer {_MCP_TOKEN}",
        "Accept": "application/json, text/event-stream",
    }


def _list_tools(cli) -> list[dict]:
    headers = _init_session(cli)
    resp = cli.post("/", json=_mcp_request("tools/list"), headers=headers)
    assert resp.status_code == 200
    return resp.json().get("result", {}).get("tools", [])


def _mcp_request(method: str, params: dict | None = None) -> dict:
    req = {"jsonrpc": "2.0", "id": 1, "method": method}
    if params:
        req["params"] = params
    return req


def _init_session(cli):
    """Send MCP initialize and return headers for subsequent requests."""
    init_resp = cli.post(
        "/",
        json=_mcp_request(
            "initialize",
            {
                "protocolVersion": "2025-03-26",
                "capabilities": {},
                "clientInfo": {"name": "test", "version": "1.0"},
            },
        ),
        headers=_auth_headers(),
    )
    assert init_resp.status_code == 200, f"init failed: {init_resp.status_code} {init_resp.text}"
    headers = {**_auth_headers()}
    session_id = init_resp.headers.get("mcp-session-id", "")
    if session_id:
        headers["mcp-session-id"] = session_id
    return headers


# ---- Auth tests (middleware rejects before MCP, no lifespan needed) ----


class TestAuth:
    def test_missing_token_returns_401(self, cli_no_lifespan):
        resp = cli_no_lifespan.post("/", json=_mcp_request("initialize"))
        assert resp.status_code == 401
        assert resp.json()["error"] == "unauthorized"

    def test_wrong_token_returns_401(self, cli_no_lifespan):
        resp = cli_no_lifespan.post(
            "/",
            json=_mcp_request("initialize"),
            headers={"Authorization": "Bearer wrong-token"},
        )
        assert resp.status_code == 401

    def test_health_no_auth(self, cli_no_lifespan):
        resp = cli_no_lifespan.get("/health")
        assert resp.status_code == 200
        assert resp.json()["status"] == "ok"

    def test_dashboard_api_token_rejected_inbound(self, cli_no_lifespan):
        resp = cli_no_lifespan.post(
            "/",
            json=_mcp_request("initialize"),
            headers={"Authorization": f"Bearer {_API_TOKEN}"},
        )
        assert resp.status_code == 401

    def test_mcp_token_accepted_inbound(self, cli):
        _init_session(cli)


# ---- Startup configuration ----


_VALID_ENV = {"MCP_TOKEN": _MCP_TOKEN, "DASHBOARD_API_TOKEN": _API_TOKEN}


class TestConfig:
    def test_valid_env(self):
        cfg = server.load_config(_VALID_ENV)
        assert cfg.mcp_token == _MCP_TOKEN
        assert cfg.allow_destructive is False

    @pytest.mark.parametrize("name", ["MCP_TOKEN", "DASHBOARD_API_TOKEN"])
    def test_missing_token_refused(self, name):
        env = {k: v for k, v in _VALID_ENV.items() if k != name}
        with pytest.raises(client.ConfigError, match=f"{name} is not set"):
            server.load_config(env)

    @pytest.mark.parametrize("name", ["MCP_TOKEN", "DASHBOARD_API_TOKEN"])
    def test_short_token_refused(self, name):
        env = {**_VALID_ENV, name: "x" * 31}
        with pytest.raises(client.ConfigError, match=f"{name} must be at least 32"):
            server.load_config(env)

    @pytest.mark.parametrize("name", ["MCP_TOKEN", "DASHBOARD_API_TOKEN"])
    def test_non_ascii_token_refused(self, name):
        env = {**_VALID_ENV, name: "\u00e9" * 40}
        with pytest.raises(client.ConfigError, match=f"{name} must be ASCII"):
            server.load_config(env)

    def test_shared_token_refused(self):
        env = {"MCP_TOKEN": _API_TOKEN, "DASHBOARD_API_TOKEN": _API_TOKEN}
        with pytest.raises(client.ConfigError, match="must differ"):
            server.load_config(env)

    @pytest.mark.parametrize(
        ("value", "expected"),
        [("true", True), ("TRUE", True), ("1", False), ("false", False), ("", False)],
    )
    def test_allow_destructive_flag(self, value, expected):
        cfg = server.load_config({**_VALID_ENV, "MCP_ALLOW_DESTRUCTIVE": value})
        assert cfg.allow_destructive is expected


# ---- Failed-auth rate limiting ----


async def _ok_app(scope, receive, send):
    await send({"type": "http.response.start", "status": 200, "headers": []})
    await send({"type": "http.response.body", "body": b"ok"})


class _FakeClock:
    def __init__(self) -> None:
        self.now = 1000.0

    def __call__(self) -> float:
        return self.now


def _limited_client(limiter, client_host="testclient"):
    mw = server.BearerAuthMiddleware(_ok_app, _MCP_TOKEN, limiter)

    async def with_client_addr(scope, receive, send):
        await mw({**scope, "client": (client_host, 50000)}, receive, send)

    return TestClient(with_client_addr)


_BAD = {"Authorization": "Bearer wrong-token"}
_GOOD = {"Authorization": f"Bearer {_MCP_TOKEN}"}


class TestRateLimit:
    def test_429_after_limit(self):
        c = _limited_client(server.FailedAuthLimiter(limit=3))
        assert [c.get("/", headers=_BAD).status_code for _ in range(5)] == [401, 401, 401, 429, 429]

    def test_valid_token_unaffected_when_limited(self):
        c = _limited_client(server.FailedAuthLimiter(limit=2))
        for _ in range(3):
            c.get("/", headers=_BAD)
        assert c.get("/", headers=_BAD).status_code == 429
        assert c.get("/", headers=_GOOD).status_code == 200

    def test_successes_do_not_count(self):
        c = _limited_client(server.FailedAuthLimiter(limit=2))
        for _ in range(5):
            assert c.get("/", headers=_GOOD).status_code == 200
        assert c.get("/", headers=_BAD).status_code == 401

    def test_window_resets(self):
        clock = _FakeClock()
        c = _limited_client(server.FailedAuthLimiter(limit=1, window=60, clock=clock))
        c.get("/", headers=_BAD)
        assert c.get("/", headers=_BAD).status_code == 429
        clock.now += 60
        assert c.get("/", headers=_BAD).status_code == 401

    def test_per_client_ip(self):
        limiter = server.FailedAuthLimiter(limit=1)
        a = _limited_client(limiter, "10.0.0.1")
        b = _limited_client(limiter, "10.0.0.2")
        a.get("/", headers=_BAD)
        assert a.get("/", headers=_BAD).status_code == 429
        assert b.get("/", headers=_BAD).status_code == 401

    def test_tracked_clients_bounded(self):
        limiter = server.FailedAuthLimiter(limit=1, max_clients=3)
        for i in range(10):
            limiter.record_failure(f"10.0.0.{i}")
        assert len(limiter._windows) == 3
        # Oldest entries are evicted, so 10.0.0.0 starts a fresh window.
        assert limiter.record_failure("10.0.0.0") is False

    def test_missing_token_counts_as_failure(self):
        c = _limited_client(server.FailedAuthLimiter(limit=1))
        c.get("/")
        assert c.get("/").status_code == 429


# ---- Tool discovery ----


class TestToolDiscovery:
    def test_tools_list_count(self, cli):
        tools = _list_tools(cli)
        tool_names = sorted(t["name"] for t in tools)
        assert len(tools) == 20, f"Expected 20 tools, got {len(tools)}: {tool_names}"

    def test_destructive_tools_absent_by_default(self, cli):
        names = {t["name"] for t in _list_tools(cli)}
        assert not names & _DESTRUCTIVE_NAMES

    def test_destructive_tools_present_when_enabled(self, cli_destructive):
        tools = {t["name"]: t for t in _list_tools(cli_destructive)}
        assert len(tools) == 24
        for name in _DESTRUCTIVE_NAMES:
            annotations = tools[name].get("annotations") or {}
            assert annotations.get("destructiveHint") is True, f"{name}: {annotations}"
            assert annotations.get("readOnlyHint") is False, f"{name}: {annotations}"

    def test_read_only_annotations(self, cli):
        tools = {t["name"]: t for t in _list_tools(cli)}
        for name, tool in tools.items():
            annotations = tool.get("annotations") or {}
            assert annotations.get("readOnlyHint", False) is (name in _READ_ONLY_NAMES), f"{name}: {annotations}"
            if name in _READ_ONLY_NAMES:
                assert not annotations.get("destructiveHint")
            else:
                # Per the MCP spec destructiveHint defaults to true for
                # non-read-only tools, so write tools must say false.
                assert annotations.get("destructiveHint") is False, f"{name}: {annotations}"

    def test_destructive_tool_not_callable_by_default(self, cli):
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "delete_todo", "arguments": {"slug": "my-task", "list": "personal"}}),
            headers=headers,
        )
        body = resp.json()
        assert "error" in body or body.get("result", {}).get("isError") is True, body

    def test_all_expected_tools_present(self, cli_destructive):
        names = {t["name"] for t in _list_tools(cli_destructive)}
        expected = {
            # Todos (12)
            "list_todos", "get_todo", "add_todo", "update_todo",
            "complete_todo", "uncomplete_todo", "delete_todo",
            "update_todo_priority", "update_todo_tags",
            "add_substep", "toggle_substep", "remove_substep",
            # Ideas (4)
            "list_ideas", "add_idea", "triage_idea", "add_idea_research",
            # Plan (5)
            "get_plan", "set_plan", "clear_plan", "reorder_plan", "clear_carried_plan",
            # Commentary (3)
            "get_commentary", "set_commentary", "delete_commentary",
        }
        assert names == expected, f"Missing: {expected - names}, Extra: {names - expected}"

    def test_tool_has_description(self, cli):
        headers = _init_session(cli)
        resp = cli.post("/", json=_mcp_request("tools/list"), headers=headers)
        tools = resp.json().get("result", {}).get("tools", [])
        for tool in tools:
            assert tool.get("description"), f"Tool {tool['name']} has no description"

    def test_list_param_has_enum(self, cli):
        """Tools with a 'list' param should have enum values in their schema."""
        headers = _init_session(cli)
        resp = cli.post("/", json=_mcp_request("tools/list"), headers=headers)
        tools = resp.json().get("result", {}).get("tools", [])
        tools_with_list = [t for t in tools if "list" in t.get("inputSchema", {}).get("properties", {})]
        assert len(tools_with_list) > 0, "No tools found with list param"
        for tool in tools_with_list:
            list_schema = tool["inputSchema"]["properties"]["list"]
            assert "enum" in list_schema, f"Tool {tool['name']} list param missing enum: {list_schema}"


# ---- Tool calls with mocked dashboard API ----


class TestToolCalls:
    @respx.mock
    def test_list_todos(self, cli):
        mock_data = {"personal": [{"slug": "test-task", "title": "Test"}], "family": []}
        respx.get("http://dashboard:8080/api/v1/todos").mock(
            return_value=httpx.Response(200, json=mock_data)
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "list_todos", "arguments": {}}),
            headers=headers,
        )
        assert resp.status_code == 200
        content = resp.json().get("result", {}).get("content", [])
        assert len(content) > 0
        parsed = json.loads(content[0]["text"])
        assert "personal" in parsed
        assert parsed["personal"][0]["slug"] == "test-task"

    @respx.mock
    def test_get_todo(self, cli):
        mock_item = {"slug": "my-task", "title": "My Task", "done": False}
        respx.get("http://dashboard:8080/api/v1/todos/my-task").mock(
            return_value=httpx.Response(200, json=mock_item)
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "get_todo", "arguments": {"slug": "my-task", "list": "personal"}}),
            headers=headers,
        )
        assert resp.status_code == 200
        content = resp.json().get("result", {}).get("content", [])
        parsed = json.loads(content[0]["text"])
        assert parsed["slug"] == "my-task"

    @respx.mock
    def test_add_todo(self, cli):
        respx.post("http://dashboard:8080/api/v1/todos").mock(
            return_value=httpx.Response(201, json={"slug": "new-task", "title": "New Task"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "add_todo", "arguments": {"title": "New Task", "list": "personal"}}),
            headers=headers,
        )
        assert resp.status_code == 200
        content = resp.json().get("result", {}).get("content", [])
        parsed = json.loads(content[0]["text"])
        assert parsed["slug"] == "new-task"

    @respx.mock
    def test_complete_todo(self, cli):
        respx.post("http://dashboard:8080/api/v1/todos/my-task/complete").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "complete_todo", "arguments": {"slug": "my-task", "list": "personal"}}),
            headers=headers,
        )
        assert resp.status_code == 200
        content = resp.json().get("result", {}).get("content", [])
        parsed = json.loads(content[0]["text"])
        assert parsed["status"] == "ok"

    @respx.mock
    def test_delete_todo(self, cli_destructive):
        respx.delete("http://dashboard:8080/api/v1/todos/my-task").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli_destructive)
        resp = cli_destructive.post(
            "/",
            json=_mcp_request("tools/call", {"name": "delete_todo", "arguments": {"slug": "my-task", "list": "family"}}),
            headers=headers,
        )
        assert resp.status_code == 200

    @respx.mock
    def test_update_todo_priority(self, cli):
        respx.put("http://dashboard:8080/api/v1/todos/my-task/priority").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "update_todo_priority", "arguments": {"slug": "my-task", "list": "personal", "priority": "high"}}),
            headers=headers,
        )
        assert resp.status_code == 200

    @respx.mock
    def test_update_todo_tags(self, cli):
        respx.put("http://dashboard:8080/api/v1/todos/my-task/tags").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "update_todo_tags", "arguments": {"slug": "my-task", "list": "personal", "tags": ["urgent", "work"]}}),
            headers=headers,
        )
        assert resp.status_code == 200

    @respx.mock
    def test_add_substep(self, cli):
        respx.post("http://dashboard:8080/api/v1/todos/my-task/substeps").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "add_substep", "arguments": {"slug": "my-task", "list": "personal", "text": "Do the thing"}}),
            headers=headers,
        )
        assert resp.status_code == 200

    @respx.mock
    def test_list_ideas(self, cli):
        respx.get("http://dashboard:8080/api/v1/ideas").mock(
            return_value=httpx.Response(200, json=[{"slug": "idea-1", "title": "Idea 1"}])
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "list_ideas", "arguments": {}}),
            headers=headers,
        )
        assert resp.status_code == 200
        content = resp.json().get("result", {}).get("content", [])
        parsed = json.loads(content[0]["text"])
        assert parsed[0]["slug"] == "idea-1"

    @respx.mock
    def test_triage_idea(self, cli):
        respx.put("http://dashboard:8080/api/v1/ideas/my-idea/triage").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "triage_idea", "arguments": {"slug": "my-idea", "action": "park"}}),
            headers=headers,
        )
        assert resp.status_code == 200

    @respx.mock
    def test_add_idea_research(self, cli):
        respx.post("http://dashboard:8080/api/v1/ideas/my-idea/research").mock(
            return_value=httpx.Response(201, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "add_idea_research", "arguments": {"slug": "my-idea", "content": "Found some interesting data"}}),
            headers=headers,
        )
        assert resp.status_code == 200

    @respx.mock
    def test_get_plan(self, cli):
        mock_plan = {"date": "2026-04-03", "personal": [], "family": [], "house": [], "overdue": []}
        respx.get("http://dashboard:8080/api/v1/plan").mock(
            return_value=httpx.Response(200, json=mock_plan)
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "get_plan", "arguments": {}}),
            headers=headers,
        )
        assert resp.status_code == 200
        content = resp.json().get("result", {}).get("content", [])
        parsed = json.loads(content[0]["text"])
        assert "personal" in parsed

    @respx.mock
    def test_set_plan(self, cli):
        respx.put("http://dashboard:8080/api/v1/plan/my-task").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "set_plan", "arguments": {"slug": "my-task", "list": "house", "date": "2026-04-05"}}),
            headers=headers,
        )
        assert resp.status_code == 200

    @respx.mock
    def test_clear_plan(self, cli):
        respx.delete("http://dashboard:8080/api/v1/plan/my-task").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "clear_plan", "arguments": {"slug": "my-task", "list": "personal"}}),
            headers=headers,
        )
        assert resp.status_code == 200

    @respx.mock
    def test_clear_carried_plan(self, cli_destructive):
        respx.post("http://dashboard:8080/api/v1/plan/clear-carried").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli_destructive)
        resp = cli_destructive.post(
            "/",
            json=_mcp_request("tools/call", {"name": "clear_carried_plan", "arguments": {}}),
            headers=headers,
        )
        assert resp.status_code == 200

    @respx.mock
    def test_get_commentary(self, cli):
        respx.get("http://dashboard:8080/api/v1/commentary/ideas/my-idea").mock(
            return_value=httpx.Response(200, json={"slug": "my-idea", "list": "ideas", "content": "Good idea"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "get_commentary", "arguments": {"list": "ideas", "slug": "my-idea"}}),
            headers=headers,
        )
        assert resp.status_code == 200
        content = resp.json().get("result", {}).get("content", [])
        parsed = json.loads(content[0]["text"])
        assert parsed["content"] == "Good idea"

    @respx.mock
    def test_set_commentary(self, cli):
        respx.put("http://dashboard:8080/api/v1/commentary/personal/my-task").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "set_commentary", "arguments": {"list": "personal", "slug": "my-task", "content": "Keep going"}}),
            headers=headers,
        )
        assert resp.status_code == 200

    @respx.mock
    def test_delete_commentary(self, cli_destructive):
        respx.delete("http://dashboard:8080/api/v1/commentary/family/my-task").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli_destructive)
        resp = cli_destructive.post(
            "/",
            json=_mcp_request("tools/call", {"name": "delete_commentary", "arguments": {"list": "family", "slug": "my-task"}}),
            headers=headers,
        )
        assert resp.status_code == 200


# ---- Slug validation ----


class TestSlugValidation:
    def test_path_traversal_rejected(self, cli):
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "get_todo", "arguments": {"slug": "../../etc/passwd", "list": "personal"}}),
            headers=headers,
        )
        assert resp.status_code == 200
        content = resp.json().get("result", {}).get("content", [])
        parsed = json.loads(content[0]["text"])
        assert "error" in parsed

    def test_slash_in_slug_rejected(self, cli):
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "complete_todo", "arguments": {"slug": "foo/bar", "list": "personal"}}),
            headers=headers,
        )
        assert resp.status_code == 200
        content = resp.json().get("result", {}).get("content", [])
        parsed = json.loads(content[0]["text"])
        assert "error" in parsed

    @respx.mock
    def test_valid_slug_accepted(self, cli):
        """A well-formed slug should not be rejected by validation."""
        respx.get("http://dashboard:8080/api/v1/todos/my-valid-task-123").mock(
            return_value=httpx.Response(200, json={"slug": "my-valid-task-123"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "get_todo", "arguments": {"slug": "my-valid-task-123", "list": "personal"}}),
            headers=headers,
        )
        content = resp.json().get("result", {}).get("content", [])
        parsed = json.loads(content[0]["text"])
        assert "error" not in parsed


# ---- Error handling ----


class TestErrorHandling:
    @respx.mock
    def test_dashboard_404_returns_error(self, cli):
        respx.get("http://dashboard:8080/api/v1/todos/nonexistent").mock(
            return_value=httpx.Response(404, json={"error": "item not found"})
        )
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "get_todo", "arguments": {"slug": "nonexistent", "list": "personal"}}),
            headers=headers,
        )
        assert resp.status_code == 200
        content = resp.json().get("result", {}).get("content", [])
        parsed = json.loads(content[0]["text"])
        assert parsed["error"] == "item not found"
        assert parsed["status"] == 404

    @respx.mock
    def test_dashboard_unreachable_returns_generic_error(self, cli):
        respx.get("http://dashboard:8080/api/v1/todos").mock(side_effect=httpx.ConnectError("refused"))
        headers = _init_session(cli)
        resp = cli.post(
            "/",
            json=_mcp_request("tools/call", {"name": "list_todos", "arguments": {}}),
            headers=headers,
        )
        assert resp.status_code == 200
        content = resp.json().get("result", {}).get("content", [])
        parsed = json.loads(content[0]["text"])
        assert parsed["error"] == "dashboard API unavailable"
        assert "dashboard:8080" not in parsed["error"]
