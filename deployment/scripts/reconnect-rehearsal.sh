#!/usr/bin/env bash
set -euo pipefail
#
# reconnect-rehearsal — deterministic reconnect/load rehearsal for GeoGuessMe.
#
# Spins up a disposable test stack, runs the Go rehearsal harness exercising
# concurrent WebSocket clients, disconnect/reconnect, cursor catch-up, live
# delivery, and exact-once message behavior, captures bounded latency/error/
# throughput evidence without sleeps or retry masking, then tears down.
#
# Prerequisites: built images (make build-images).  The Make target handles
# that prerequisite automatically.

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"
PROJECT="${GEOGUESSME_RECONNECT_PROJECT:-geoguessme-reconnect-rehearsal}"
WEB_PORT="${GEOGUESSME_RECONNECT_WEB_PORT:-18082}"
MAILPIT_PORT="${GEOGUESSME_RECONNECT_MAILPIT_PORT:-18027}"
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
export GEOGUESSME_TEST_ALLOWED_ORIGINS="http://localhost:${WEB_PORT},http://host.docker.internal:${WEB_PORT}"

echo "=== Reconnect rehearsal: starting test stack ==="
docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" up -d --wait

echo "=== Reconnect rehearsal: running harness ==="
docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run -T --rm --no-deps go-tools-write sh -c \
    "cd /workspace/tools/load/reconnect-rehearsal && go run . -base-url 'http://host.docker.internal:${WEB_PORT}'"

echo "=== Reconnect rehearsal PASSED ==="
