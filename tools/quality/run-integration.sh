#!/usr/bin/env bash
set -euo pipefail
REPO="$(cd "$(dirname "$0")/../.." && pwd)"

PROJECT="${GEOGUESSME_TEST_PROJECT:-geoguessme-integration}"
WEB_PORT="${GEOGUESSME_TEST_WEB_PORT:-8080}"
MAILPIT_PORT="${GEOGUESSME_TEST_MAILPIT_PORT:-8025}"
TEST_BASE_URL="http://localhost:${WEB_PORT}"

cleanup() {
    docker compose -f "$REPO/deployment/compose.test.yaml" --project-directory "$REPO" -p "$PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

export GEOGUESSME_TEST_WEB_PORT="$WEB_PORT" GEOGUESSME_TEST_MAILPIT_PORT="$MAILPIT_PORT"
docker compose -f "$REPO/deployment/compose.test.yaml" --project-directory "$REPO" -p "$PROJECT" up -d --build --wait
cd "$REPO/backend" && TEST_BASE_URL="$TEST_BASE_URL" go test ./integration_test -count=1
