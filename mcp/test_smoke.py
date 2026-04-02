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
_TEST_TOKEN = "test-token-that-is-at-least-thirty-two-characters-long"
os.environ.setdefault("DASHBOARD_API_TOKEN", _TEST_TOKEN)
os.environ.setdefault("DASHBOARD_API_URL", "http://dashboard:8080/api/v1")

from starlette.testclient import TestClient  # noqa: E402

from server import app  # noqa: E402


@pytest.fixture(scope="module")
def cli():
    """Module-scoped client with lifespan.

    The MCP StreamableHTTPSessionManager.run() can only be called once per
    instance, so we must keep a single TestClient alive for all tests.
    """
    with TestClient(app, raise_server_exceptions=False) as client:
        yield client


@pytest.fixture
def cli_no_lifespan():
    """Per-test client WITHOUT lifespan -- for auth-rejection tests that
    never reach the MCP session manager."""
    return TestClient(app, raise_server_exceptions=False)


def _auth_headers():
    return {
        "Authorization": f"Bearer {_TEST_TOKEN}",
        "Accept": "application/json, text/event-stream",
    }


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


# ---- Tool discovery ----


class TestToolDiscovery:
    def test_tools_list_count(self, cli):
        headers = _init_session(cli)
        resp = cli.post("/", json=_mcp_request("tools/list"), headers=headers)
        assert resp.status_code == 200
        body = resp.json()
        tools = body.get("result", {}).get("tools", [])
        tool_names = sorted(t["name"] for t in tools)
        assert len(tools) == 24, f"Expected 24 tools, got {len(tools)}: {tool_names}"

    def test_all_expected_tools_present(self, cli):
        headers = _init_session(cli)
        resp = cli.post("/", json=_mcp_request("tools/list"), headers=headers)
        tools = resp.json().get("result", {}).get("tools", [])
        names = {t["name"] for t in tools}
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
    def test_delete_todo(self, cli):
        respx.delete("http://dashboard:8080/api/v1/todos/my-task").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
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
    def test_clear_carried_plan(self, cli):
        respx.post("http://dashboard:8080/api/v1/plan/clear-carried").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
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
    def test_delete_commentary(self, cli):
        respx.delete("http://dashboard:8080/api/v1/commentary/family/my-task").mock(
            return_value=httpx.Response(200, json={"status": "ok"})
        )
        headers = _init_session(cli)
        resp = cli.post(
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
