#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"
PROJECT="${GEOGUESSME_SMOKE_PROJECT:-geoguessme-smoke-rehearsal}"
WEB_PORT="${GEOGUESSME_SMOKE_WEB_PORT:-18082}"
MAILPIT_PORT="${GEOGUESSME_SMOKE_MAILPIT_PORT:-18027}"
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
deployment/scripts/smoke-test.sh "$PUBLIC_URL"
echo "smoke-rehearsal PASSED: disposable stack passed liveness, readiness, auth, and WebSocket ticket checks"
