#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

PROJECT="${GEOGUESSME_TEST_PROJECT:-geoguessme-e2e}"
WEB_PORT="${GEOGUESSME_TEST_WEB_PORT:-8080}"
MAILPIT_PORT="${GEOGUESSME_TEST_MAILPIT_PORT:-8025}"
PUBLIC_URL="${GEOGUESSME_TEST_PUBLIC_URL:-http://localhost:${WEB_PORT}}"
COMPOSE_FILE="deployment/compose.test.yaml"

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

docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" up -d --build --wait
"$REPO/deployment/scripts/wait-for-health.sh" "$PUBLIC_URL" 120

test_args=(test --project=desktop --project=mobile)
if [ "${1:-}" = "--ui" ]; then
    test_args+=(--ui)
fi
if [ -n "${GEOGUESSME_E2E_SPEC:-}" ]; then
    test_args+=("${GEOGUESSME_E2E_SPEC}")
fi

mkdir -p "$REPO/frontend/test-results" "$REPO/frontend/playwright-report"
container_name="${PROJECT}-playwright"
docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run -d --no-deps --name "$container_name" playwright sh -c \
    "cd /workspace/frontend && PLAYWRIGHT_BASE_URL=http://host.docker.internal:${WEB_PORT} MAILPIT_BASE_URL=http://host.docker.internal:${MAILPIT_PORT} PLAYWRIGHT_OUTPUT_DIR=/tmp/playwright/test-results PLAYWRIGHT_REPORT_DIR=/tmp/playwright/playwright-report PLAYWRIGHT_LAST_RUN_OUTPUT_FILE=/tmp/playwright/test-results/.last-run.json ./node_modules/.bin/playwright ${test_args[*]}"

status="$(docker wait "$container_name")"
docker logs "$container_name"
if docker cp "$container_name:/tmp/playwright/test-results/." "$REPO/frontend/test-results/"; then
    :
else
    echo "warning: Playwright test artifacts were not produced" >&2
fi
if docker cp "$container_name:/tmp/playwright/playwright-report/." "$REPO/frontend/playwright-report/"; then
    :
else
    echo "warning: Playwright HTML report was not produced" >&2
fi
docker rm "$container_name" >/dev/null
exit "$status"
