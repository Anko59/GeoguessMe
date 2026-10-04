#!/usr/bin/env bash
set -euo pipefail

: "${GEOGUESSME_TOOLS_PROJECT:?Run through Make}"

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

PROJECT="${GEOGUESSME_TEST_PROJECT:-${GEOGUESSME_TOOLS_PROJECT}-e2e-$$}"
WEB_PORT="${GEOGUESSME_TEST_WEB_PORT:?Run through Make}"
MAILPIT_PORT="${GEOGUESSME_TEST_MAILPIT_PORT:?Run through Make}"
PUBLIC_URL="${GEOGUESSME_TEST_PUBLIC_URL:-http://localhost:${WEB_PORT}}"
COMPOSE_FILE="deployment/compose.test.yaml"
FRONTEND_DIR="$(cd "$REPO/frontend" && pwd -P)"

# shellcheck disable=SC1091 # Repository-relative helper resolved after cd above.
source "$REPO/tools/quality/e2e/arguments.sh"

test_args=()
build_e2e_test_args \
    "${GEOGUESSME_E2E_PROJECTS:-desktop,firefox,mobile}" \
    "${GEOGUESSME_E2E_SHARD:-}" \
    "${GEOGUESSME_E2E_SPEC:-}" \
    "${1:-}"
test_args=("${E2E_TEST_ARGS[@]}")

# Clear stale artifacts so only the current invocation's output is retained.
if [ -e "$REPO/frontend/test-results" ] || [ -e "$REPO/frontend/playwright-report" ]; then
    rm -rf "$REPO/frontend/test-results" "$REPO/frontend/playwright-report" || {
        echo "unable to remove stale Playwright artifacts; run make artifacts-clean with matching Docker user" >&2
        exit 1
    }
fi
mkdir -p "$REPO/frontend/test-results" "$REPO/frontend/playwright-report"
STAGING_DIR="$(mktemp -d "$FRONTEND_DIR/.playwright-run-$$.XXXXXX")"

cleanup_staging() {
    [ -d "$STAGING_DIR" ] || return 0
    local resolved
    resolved="$(realpath "$STAGING_DIR")"
    case "$resolved" in
        "$FRONTEND_DIR/.playwright-run-$$."*) ;;
        *)
            echo 'refusing unexpected Playwright staging cleanup path' >&2
            return 1
            ;;
    esac
    [ "$resolved" = "$STAGING_DIR" ] && [ ! -L "$STAGING_DIR" ] || return 1
    rm -rf -- "$resolved"
}

# shellcheck disable=SC2317 # Invoked indirectly by EXIT trap below.
cleanup() {
    status=$?
    if [ "$status" -ne 0 ]; then
        echo "E2E stack state (failure):" >&2
        docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" ps --all >&2 || true
        echo "E2E stack logs (failure):" >&2
        docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" \
            logs --no-color --tail=80 web backend migration db minio toxiproxy mailpit >&2 || true
    fi
    cleanup_status=0
    docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" down -v --remove-orphans || cleanup_status=$?
    if [ "$status" -eq 0 ]; then
        status=$cleanup_status
    fi
    if ! cleanup_staging; then
        echo 'unable to remove owned Playwright staging directory' >&2
        [ "$status" -ne 0 ] || status=1
    fi
    exit "$status"
}
trap cleanup EXIT

export GEOGUESSME_TEST_WEB_PORT="$WEB_PORT"
export GEOGUESSME_TEST_MAILPIT_PORT="$MAILPIT_PORT"
export GEOGUESSME_TEST_PUBLIC_URL="$PUBLIC_URL"
export GEOGUESSME_TEST_ALLOWED_ORIGINS="$PUBLIC_URL,http://host.docker.internal:${WEB_PORT}"

docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" up -d --wait
"$REPO/deployment/scripts/wait-for-health.sh" "$PUBLIC_URL" 120

# Run Playwright inside the pinned image without unsafe sh -c interpolation.
# Environment variables and arguments are passed directly; output directories
# are host-mounted so artifacts land deterministically.
run_status=0
docker compose -p "${GEOGUESSME_TOOLS_PROJECT:?Run through Make}" -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run -T --rm --no-deps --user "$(id -u):$(id -g)" \
    -w /workspace/frontend \
    -e "PLAYWRIGHT_BASE_URL=http://host.docker.internal:${WEB_PORT}" \
    -e "MAILPIT_BASE_URL=http://host.docker.internal:${MAILPIT_PORT}" \
    -e "PLAYWRIGHT_OUTPUT_DIR=/tmp/playwright/test-results" \
    -e "PLAYWRIGHT_REPORT_DIR=/tmp/playwright/report" \
    -e "PLAYWRIGHT_LAST_RUN_OUTPUT_FILE=/tmp/playwright/test-results/.last-run.json" \
    -e "HOME=/tmp/playwright" \
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
if ! cleanup_staging; then
    echo 'unable to remove owned Playwright staging directory' >&2
    [ "$run_status" -ne 0 ] || run_status=1
fi
exit "$run_status"
