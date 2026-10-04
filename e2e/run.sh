#!/usr/bin/env bash
# Builds the dashboard, starts it against a throwaway copy of e2e/fixtures and
# runs the Playwright suite. Extra arguments are passed to `playwright test`.
#
#   bash e2e/run.sh                 # full suite
#   bash e2e/run.sh tests/search.spec.ts --headed
#
# Env: E2E_PORT (default 18080), CI (installs browser system deps when set).
set -euo pipefail

E2E_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$E2E_DIR")"
E2E_PORT="${E2E_PORT:-18080}"

# Test-only credentials. With no users in the DB, the server auto-creates
# admin@localhost from DASHBOARD_PASSWORD_HASH (user id 1).
# Regenerate the hash with: htpasswd -nbBC 10 "" "$E2E_PASSWORD" | cut -d: -f2
export E2E_EMAIL="admin@localhost"
export E2E_PASSWORD="e2e-test-password"
# shellcheck disable=SC2016 # literal bcrypt hash, no expansion wanted
readonly E2E_PASSWORD_HASH='$2y$10$3rpZ.qDcjxeRldc0owwaWe89dxjuyogB2eEHtC3ej/j2TnkfjF/yK'

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/dashboard-e2e.XXXXXX")"
DATA_DIR="$WORK_DIR/data"
SERVER_LOG="$WORK_DIR/server.log"
SERVER_PID=""

# shellcheck disable=SC2329 # invoked via trap
cleanup() {
    local shutdown_failed=false
    if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
        # The server must exit on SIGTERM within its 8s shutdown window, even
        # with SSE streams open. Escalate and fail the run if it does not.
        kill "$SERVER_PID" 2>/dev/null || true
        for _ in $(seq 1 50); do
            kill -0 "$SERVER_PID" 2>/dev/null || break
            sleep 0.2
        done
        if kill -0 "$SERVER_PID" 2>/dev/null; then
            echo "error: server ignored SIGTERM for 10s; killing it" >&2
            kill -9 "$SERVER_PID" 2>/dev/null || true
            shutdown_failed=true
        fi
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    # Keep the server log alongside Playwright's artefacts for CI uploads.
    if [[ -f "$SERVER_LOG" ]]; then
        mkdir -p "$E2E_DIR/test-results"
        cp "$SERVER_LOG" "$E2E_DIR/test-results/server.log" || true
    fi
    rm -rf "$WORK_DIR"
    if [[ "$shutdown_failed" == true ]]; then
        exit 1
    fi
}
trap cleanup EXIT

base_url="http://127.0.0.1:$E2E_PORT"
if curl -fsS -o /dev/null "$base_url/login" 2>/dev/null; then
    echo "error: something is already listening on port $E2E_PORT; set E2E_PORT" >&2
    exit 1
fi

echo "==> building server"
(cd "$REPO_ROOT" && CGO_ENABLED=0 go build -o "$WORK_DIR/dashboard" ./cmd/dashboard)

echo "==> seeding $DATA_DIR"
mkdir -p "$DATA_DIR"
cp -R "$E2E_DIR/fixtures/." "$DATA_DIR/"

echo "==> starting server on $base_url"
(
    cd "$WORK_DIR"
    PERSONAL_PATH="$DATA_DIR/personal.md" \
    FAMILY_PATH="$DATA_DIR/family.md" \
    IDEAS_PATH="$DATA_DIR/ideas.md" \
    MAINTENANCE_PATH="$DATA_DIR/maintenance.md" \
    HOUSE_PROJECTS_PATH="$DATA_DIR/house-projects.md" \
    USER_DATA_DIR="$DATA_DIR/users" \
    UPLOADS_DIR="$DATA_DIR/uploads" \
    DB_PATH="$DATA_DIR/db/dashboard.db" \
    DASHBOARD_SECURE_COOKIES=false \
    DASHBOARD_TIMEZONE=Australia/Sydney \
    DASHBOARD_PASSWORD_HASH="$E2E_PASSWORD_HASH" \
    ADDR="127.0.0.1:$E2E_PORT" \
    exec ./dashboard
) >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

ready=false
for _ in $(seq 1 100); do
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
        break
    fi
    if curl -fsS -o /dev/null "$base_url/login" 2>/dev/null; then
        ready=true
        break
    fi
    sleep 0.2
done
if [[ "$ready" != true ]]; then
    echo "error: server did not become ready; log follows" >&2
    cat "$SERVER_LOG" >&2
    exit 1
fi

cd "$E2E_DIR"
if [[ ! -d node_modules ]]; then
    echo "==> installing node dependencies"
    if [[ -f package-lock.json ]]; then
        npm ci --no-audit --no-fund
    else
        npm install --no-audit --no-fund
    fi
fi
if [[ -n "${CI:-}" ]]; then
    npx playwright install --with-deps chromium
else
    npx playwright install chromium
fi

export E2E_BASE_URL="$base_url"
export E2E_DATA_DIR="$DATA_DIR"

echo "==> running playwright"
status=0
npx playwright test "$@" || status=$?
exit "$status"
