#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

PROJECT="${GEOGUESSME_TEST_PROJECT:-geoguessme-integration}"
WEB_PORT="${GEOGUESSME_TEST_WEB_PORT:-8080}"
MAILPIT_PORT="${GEOGUESSME_TEST_MAILPIT_PORT:-8025}"
DB_PORT="${GEOGUESSME_TEST_DB_PORT:-15432}"
TOXIPROXY_PORT="${GEOGUESSME_TEST_TOXIPROXY_PORT:-8474}"
PUBLIC_URL="${GEOGUESSME_TEST_PUBLIC_URL:-http://localhost:${WEB_PORT}}"
COMPOSE_FILE="deployment/compose.test.yaml"

cleanup() {
    status=$?
    docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" down -v --remove-orphans
    exit "$status"
}
trap cleanup EXIT

export GEOGUESSME_TEST_WEB_PORT="$WEB_PORT"
export GEOGUESSME_TEST_MAILPIT_PORT="$MAILPIT_PORT"
export GEOGUESSME_TEST_DB_PORT="$DB_PORT"
export GEOGUESSME_TEST_TOXIPROXY_PORT="$TOXIPROXY_PORT"
export GEOGUESSME_TEST_PUBLIC_URL="$PUBLIC_URL"
export GEOGUESSME_TEST_ALLOWED_ORIGINS="$PUBLIC_URL,http://host.docker.internal:${WEB_PORT}"

docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" up -d --build --wait
"$REPO/deployment/scripts/wait-for-health.sh" "$PUBLIC_URL" 120

docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run --rm --no-deps go-tools sh -c \
    "cd /workspace/backend && TEST_BASE_URL=http://host.docker.internal:${WEB_PORT} MAILPIT_BASE_URL=http://host.docker.internal:${MAILPIT_PORT} TEST_DATABASE_URL=postgres://test:test@host.docker.internal:${DB_PORT}/geoguessme_test?sslmode=disable TOXIPROXY_API_URL=http://host.docker.internal:${TOXIPROXY_PORT} go test ./integration_test -count=1"
