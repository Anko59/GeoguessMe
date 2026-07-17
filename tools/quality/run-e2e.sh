#!/usr/bin/env bash
# E2E test lifecycle: build, wait, test playwright, teardown.
set -euo pipefail
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

PROJECT="${GEOGUESSME_TEST_PROJECT:-geoguessme-e2e}"
WEB_PORT="${GEOGUESSME_TEST_WEB_PORT:-8080}"
MAILPIT_PORT="${GEOGUESSME_TEST_MAILPIT_PORT:-8025}"
TEST_BASE_URL="http://localhost:${WEB_PORT}"
COMPOSE_FILE="deployment/compose.test.yaml"

cleanup() {
    docker compose -f "$COMPOSE_FILE" --project-directory . -p "$PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

export GEOGUESSME_TEST_WEB_PORT="$WEB_PORT"
export GEOGUESSME_TEST_MAILPIT_PORT="$MAILPIT_PORT"

docker compose -f "$COMPOSE_FILE" --project-directory . -p "$PROJECT" up -d --build --wait

"$REPO"/deployment/scripts/wait-for-health.sh "$TEST_BASE_URL" 120

cd "$REPO/frontend"
PLAYWRIGHT_BASE_URL="$TEST_BASE_URL" npx playwright test --project=desktop --project=mobile
