#!/usr/bin/env bash
set -euo pipefail

: "${GEOGUESSME_TOOLS_PROJECT:?Run through Make}"

REPO="$(cd "$(dirname "$0")/../.." && pwd -P)"
cd "$REPO"

PROJECT="${GEOGUESSME_TEST_PROJECT:-${GEOGUESSME_TOOLS_PROJECT}-integration-$$}"
for project in "$PROJECT" "$GEOGUESSME_TOOLS_PROJECT"; do
    if [[ ! "$project" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
        echo 'integration project names must match [a-z0-9][a-z0-9_-]*' >&2
        exit 2
    fi
done
WEB_PORT="${GEOGUESSME_TEST_WEB_PORT:?Run through Make}"
MAILPIT_PORT="${GEOGUESSME_TEST_MAILPIT_PORT:?Run through Make}"
DB_PORT="${GEOGUESSME_TEST_DB_PORT:?Run through Make}"
TOXIPROXY_PORT="${GEOGUESSME_TEST_TOXIPROXY_PORT:?Run through Make}"
PUBLIC_URL="${GEOGUESSME_TEST_PUBLIC_URL:-http://localhost:${WEB_PORT}}"
COMPOSE_FILE="deployment/compose.test.yaml"
identity=''

compose() {
    docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" "$@"
}

project_ids() {
    docker ps -aq --filter "label=com.docker.compose.project=$PROJECT"
}

# Inspect only IDs/image IDs and the public Compose source-directory label.
# A project name alone is not ownership: another worktree can recreate it.
owned_identity() {
    local ids id owner
    ids=$(project_ids) || return 1
    for id in $ids; do
        owner=$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project.working_dir"}}' "$id") || return 1
        if [ "$owner" != "$REPO" ]; then
            echo "integration ownership mismatch: project=$PROJECT container=$id owner=$owner expected=$REPO; refusing logs or teardown" >&2
            return 1
        fi
    done
    for id in $ids; do
        docker inspect --format '{{.Id}} {{.Image}}' "$id" || return 1
    done | sort
}

assert_identity() {
    local current
    current=$(owned_identity) || return 1
    if [ -n "$identity" ] && [ "$current" != "$identity" ]; then
        echo "integration container/image identity changed for $PROJECT; refusing logs or teardown" >&2
        return 1
    fi
}

# Never reuse a pre-existing stack, even from this worktree. It may belong to a
# concurrent invocation; a clean namespace is required for a fresh fixture.
initial=$(owned_identity) || exit 1
if [ -n "$initial" ]; then
    echo "integration project $PROJECT already exists; choose a private GEOGUESSME_TEST_PROJECT" >&2
    exit 1
fi
checkout_revision=$(git rev-parse HEAD)
backend_image=$(docker image inspect --format '{{.Id}}' "${BACKEND_IMAGE:?Run through Make}")

cleanup() {
    status=$?
    cleanup_status=0
    if ! assert_identity; then
        echo "integration ownership lost for $PROJECT; leaving all project resources untouched" >&2
        if [ "$status" -eq 0 ]; then status=1; fi
        exit "$status"
    fi
    if [ "$status" -ne 0 ]; then
        echo 'integration owned fixture state (failure):' >&2
        compose ps --all >&2 || echo 'integration status diagnostic failed' >&2
        # Recheck immediately before each project-selected operation.
        if ! assert_identity; then exit "$status"; fi
        compose logs --no-color --tail=80 backend web migration db minio toxiproxy mailpit >&2 ||
            echo 'integration logs diagnostic failed' >&2
    fi
    if ! assert_identity; then
        if [ "$status" -eq 0 ]; then status=1; fi
        exit "$status"
    fi
    compose down -v --remove-orphans || cleanup_status=$?
    if [ "$cleanup_status" -ne 0 ]; then
        echo "integration teardown failed for $PROJECT (exit $cleanup_status)" >&2
    fi
    if [ "$status" -eq 0 ]; then status=$cleanup_status; fi
    exit "$status"
}
trap cleanup EXIT

export GEOGUESSME_TEST_WEB_PORT="$WEB_PORT"
export GEOGUESSME_TEST_MAILPIT_PORT="$MAILPIT_PORT"
export GEOGUESSME_TEST_DB_PORT="$DB_PORT"
export GEOGUESSME_TEST_TOXIPROXY_PORT="$TOXIPROXY_PORT"
export GEOGUESSME_TEST_PUBLIC_URL="$PUBLIC_URL"
export GEOGUESSME_TEST_ALLOWED_ORIGINS="$PUBLIC_URL,http://host.docker.internal:${WEB_PORT}"

compose up -d --wait
identity=$(owned_identity)
[ -n "$identity" ] || {
    echo 'integration startup produced no owned containers' >&2
    exit 1
}
backend=$(compose ps -q backend)
actual_image=$(docker inspect --format '{{.Image}}' "$backend")
if [ "$actual_image" != "$backend_image" ]; then
    echo 'integration backend image differs from the pre-start image ID' >&2
    exit 1
fi
revision=$(docker inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$backend")
if [ -n "$revision" ] && [ "$revision" != '<no value>' ] && [ "$revision" != "$checkout_revision" ]; then
    echo 'integration backend revision label differs from the checkout revision' >&2
    exit 1
fi
echo "integration identity: backend=$backend image=$actual_image checkout=$checkout_revision container_revision=$revision"
# Legacy app images lack revision labels; image ID stability is checked, but a
# missing label is not evidence that an image was built from the checkout SHA.
"$REPO/deployment/scripts/wait-for-health.sh" "$PUBLIC_URL" 120
assert_identity

docker compose -p "${GEOGUESSME_TOOLS_PROJECT:?Run through Make}" -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run -T --rm --no-deps go-tools sh -c \
    "cd /workspace/backend && TEST_BASE_URL=http://host.docker.internal:${WEB_PORT} MAILPIT_BASE_URL=http://host.docker.internal:${MAILPIT_PORT} TEST_DATABASE_URL=postgres://test:test@host.docker.internal:${DB_PORT}/geoguessme_test?sslmode=disable TOXIPROXY_API_URL=http://host.docker.internal:${TOXIPROXY_PORT} go test ./integration_test -count=1"
assert_identity
