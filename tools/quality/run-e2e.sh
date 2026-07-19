#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

PROJECT="${GEOGUESSME_TEST_PROJECT:-geoguessme-e2e}"
WEB_PORT="${GEOGUESSME_TEST_WEB_PORT:-8080}"
MAILPIT_PORT="${GEOGUESSME_TEST_MAILPIT_PORT:-8025}"
PUBLIC_URL="${GEOGUESSME_TEST_PUBLIC_URL:-http://localhost:${WEB_PORT}}"
COMPOSE_FILE="deployment/compose.test.yaml"
STAGING_DIR="$REPO/frontend/.playwright-run"

# Clear stale artifacts so only the current invocation's output is retained.
if [ -e "$REPO/frontend/test-results" ] || [ -e "$REPO/frontend/playwright-report" ]; then
    rm -rf "$REPO/frontend/test-results" "$REPO/frontend/playwright-report" || {
        echo "unable to remove stale Playwright artifacts; run make artifacts-clean with matching Docker user" >&2
        exit 1
    }
fi
mkdir -p "$REPO/frontend/test-results" "$REPO/frontend/playwright-report"
rm -rf "$STAGING_DIR"
mkdir -p "$STAGING_DIR"

# shellcheck disable=SC2317 # Invoked indirectly by EXIT trap below.
cleanup() {
    status=$?
    docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" down -v --remove-orphans
    exit "$status"
}
trap cleanup EXIT

export GEOGUESSME_TEST_WEB_PORT="$WEB_PORT"
export GEOGUESSME_TEST_MAILPIT_PORT="$MAILPIT_PORT"
export GEOGUESSME_TEST_PUBLIC_URL="$PUBLIC_URL"
export GEOGUESSME_TEST_ALLOWED_ORIGINS="$PUBLIC_URL,http://host.docker.internal:${WEB_PORT}"

docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" up -d --wait
"$REPO/deployment/scripts/wait-for-health.sh" "$PUBLIC_URL" 120

test_args=(test --project=desktop --project=firefox --project=mobile)
if [ "${1:-}" = "--ui" ]; then
    test_args+=(--ui)
fi
if [ -n "${GEOGUESSME_E2E_SPEC:-}" ]; then
    test_args+=("${GEOGUESSME_E2E_SPEC}")
fi

# Run Playwright inside the pinned image without unsafe sh -c interpolation.
# Environment variables and arguments are passed directly; output directories
# are host-mounted so artifacts land deterministically.
run_status=0
docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run --rm --no-deps --user "$(id -u):$(id -g)" \
    -w /workspace/frontend \
    -e "PLAYWRIGHT_BASE_URL=http://host.docker.internal:${WEB_PORT}" \
    -e "MAILPIT_BASE_URL=http://host.docker.internal:${MAILPIT_PORT}" \
    -e "PLAYWRIGHT_OUTPUT_DIR=/tmp/playwright/test-results" \
    -e "PLAYWRIGHT_REPORT_DIR=/tmp/playwright/report" \
    -e "PLAYWRIGHT_LAST_RUN_OUTPUT_FILE=/tmp/playwright/test-results/.last-run.json" \
    -v "$STAGING_DIR:/tmp/playwright" \
    playwright node node_modules/.bin/playwright "${test_args[@]}" || run_status=$?

rm -rf "$REPO/frontend/test-results"
if [ -d "$STAGING_DIR/test-results" ]; then
    mv "$STAGING_DIR/test-results" "$REPO/frontend/test-results"
else
    mkdir -p "$REPO/frontend/test-results"
fi
rm -rf "$REPO/frontend/playwright-report"
if [ -d "$STAGING_DIR/report" ]; then
    mv "$STAGING_DIR/report" "$REPO/frontend/playwright-report"
else
    mkdir -p "$REPO/frontend/playwright-report"
fi
rm -rf "$STAGING_DIR"
exit "$run_status"
