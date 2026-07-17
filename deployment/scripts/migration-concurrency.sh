#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"
PROJECT="${GEOGUESSME_MIGRATION_PROJECT:-geoguessme-migration-test}"
DB_PORT="${GEOGUESSME_MIGRATION_DB_PORT:-15433}"
DB_URL="postgres://test:test@host.docker.internal:${DB_PORT}/geoguessme_test?sslmode=disable"

cleanup() {
    status=$?
    docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" down -v --remove-orphans
    exit "$status"
}
trap cleanup EXIT

export GEOGUESSME_TEST_DB_PORT="$DB_PORT"
docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" up -d db --wait
docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run --rm --no-deps go-tools psql "$DB_URL" -v ON_ERROR_STOP=1 -c \
    "CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT UNIQUE NOT NULL, password TEXT NOT NULL, avatar TEXT NOT NULL DEFAULT 'avatar.png', score INTEGER NOT NULL DEFAULT 0); INSERT INTO users (id, username, password) VALUES ('legacy-user', 'legacy_user', 'legacy-password');"

first="$(docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" run -d --no-deps --quiet-pull migration | tail -n 1)"
second="$(docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" run -d --no-deps --quiet-pull migration | tail -n 1)"
first_status="$(docker wait "$first")"
second_status="$(docker wait "$second")"
test "$first_status" = 0
test "$second_status" = 0
docker rm "$first" "$second" >/dev/null
docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" run --rm migration

migration_count="$(docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" run --rm --no-deps go-tools psql "$DB_URL" -Atc 'SELECT count(*) FROM schema_migrations')"
legacy_email="$(docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" run --rm --no-deps go-tools psql "$DB_URL" -Atc "SELECT email_normalized FROM users WHERE id = 'legacy-user'")"
test "$migration_count" = 2
test "$legacy_email" = legacy_user@legacy.invalid
echo "migration-concurrency PASSED: concurrent and idempotent runs preserved legacy data"
