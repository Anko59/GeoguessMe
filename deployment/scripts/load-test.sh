#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"
PROJECT="${GEOGUESSME_LOAD_PROJECT:-geoguessme-load}"
WEB_PORT="${GEOGUESSME_TEST_WEB_PORT:-18080}"
MAILPIT_PORT="${GEOGUESSME_TEST_MAILPIT_PORT:-18025}"
PUBLIC_URL="http://localhost:${WEB_PORT}"

cleanup() {
    status=$?
    docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" down -v --remove-orphans
    exit "$status"
}
trap cleanup EXIT

export GEOGUESSME_TEST_WEB_PORT="$WEB_PORT"
export GEOGUESSME_TEST_MAILPIT_PORT="$MAILPIT_PORT"
export GEOGUESSME_TEST_PUBLIC_URL="$PUBLIC_URL"
docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" up -d --build --wait

docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run --rm --no-deps loadtest k6 run \
    -e BASE_URL="http://host.docker.internal:${WEB_PORT}" \
    -e VUS="${LOAD_VUS:-5}" -e DURATION="${LOAD_DURATION:-30s}" \
    /workspace/tools/load/k6.js
