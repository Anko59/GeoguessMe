#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"
PROJECT="${GEOGUESSME_RESTART_PROJECT:-geoguessme-restart-rehearsal}"
WEB_PORT="${GEOGUESSME_RESTART_WEB_PORT:-18081}"
MAILPIT_PORT="${GEOGUESSME_RESTART_MAILPIT_PORT:-18026}"
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
docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" up -d --wait
docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" restart backend web
docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" up -d --wait
docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run --rm --no-deps go-tools curl --fail --silent --show-error "http://host.docker.internal:${WEB_PORT}/health/ready"
echo "restart-rehearsal PASSED: backend and gateway recovered with persistent volumes"
