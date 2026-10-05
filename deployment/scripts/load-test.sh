#!/usr/bin/env bash
set -euo pipefail

: "${GEOGUESSME_TOOLS_PROJECT:?Run through Make}"

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"
PROJECT="${GEOGUESSME_LOAD_PROJECT:-geoguessme-load-${GEOGUESSME_TOOLS_PROJECT}-$$}"
WEB_PORT="${GEOGUESSME_TEST_WEB_PORT:?Run through Make}"
MAILPIT_PORT="${GEOGUESSME_TEST_MAILPIT_PORT:?Run through Make}"
PUBLIC_URL="http://localhost:${WEB_PORT}"
TOOLS_UID="${TOOLS_UID:-$(id -u)}"
TOOLS_GID="${TOOLS_GID:-$(id -g)}"
# The pinned k6 account cannot traverse an owner-only checkout. Use the same
# non-root owner mapping as canonical Make tools, never widen workspace modes.
if [[ ! "$TOOLS_UID" =~ ^[1-9][0-9]*$ || ! "$TOOLS_GID" =~ ^(0|[1-9][0-9]*)$ ]]; then
    echo 'load-test requires a numeric non-root TOOLS_UID and numeric TOOLS_GID' >&2
    exit 2
fi

compose() {
    docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" "$@"
}

cleanup() {
    status=$?
    cleanup_status=0
    # compose.test contains only fake fixture environment values and resources
    # from this managed project; never dump environments or a peer stack's logs.
    if [ "$status" -ne 0 ]; then
        compose ps --all || echo 'load fixture status diagnostic failed' >&2
        compose logs --no-color --tail 100 || echo 'load fixture logs diagnostic failed' >&2
    fi
    compose down -v --remove-orphans || cleanup_status=$?
    if [ "$cleanup_status" -ne 0 ]; then
        echo "load-test teardown failed for managed project $PROJECT (exit $cleanup_status)" >&2
    fi
    if [ "$status" -eq 0 ]; then status=$cleanup_status; fi
    exit "$status"
}
trap cleanup EXIT

export GEOGUESSME_TEST_WEB_PORT="$WEB_PORT"
export GEOGUESSME_TEST_MAILPIT_PORT="$MAILPIT_PORT"
export GEOGUESSME_TEST_PUBLIC_URL="$PUBLIC_URL"
compose up -d --wait

docker compose -p "${GEOGUESSME_TOOLS_PROJECT:?Run through Make}" -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run -T --rm --no-deps --user "$TOOLS_UID:$TOOLS_GID" loadtest k6 run \
    -e BASE_URL="http://host.docker.internal:${WEB_PORT}" \
    -e VUS="${LOAD_VUS:-5}" -e DURATION="${LOAD_DURATION:-30s}" \
    /workspace/tools/load/k6.js
