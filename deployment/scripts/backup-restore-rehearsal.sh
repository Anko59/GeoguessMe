#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"
PROJECT="${GEOGUESSME_REHEARSAL_PROJECT:-geoguessme-backup-rehearsal}"
DB_PORT="${GEOGUESSME_REHEARSAL_DB_PORT:-15432}"
DB_URL="postgres://test:test@host.docker.internal:${DB_PORT}/geoguessme_test?sslmode=disable"
RESTORE_URL="postgres://test:test@host.docker.internal:${DB_PORT}/geoguessme_restore?sslmode=disable"

cleanup() {
    status=$?
    docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" down -v --remove-orphans
    docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" run --rm --no-deps go-tools-write sh -c 'rm -rf /workspace/backups'
    exit "$status"
}
trap cleanup EXIT

export GEOGUESSME_TEST_DB_PORT="$DB_PORT"
docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" up -d db --wait
docker compose -f deployment/compose.test.yaml --project-directory "$REPO" -p "$PROJECT" run --rm migration

docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run --rm --no-deps go-tools psql "$DB_URL" -v ON_ERROR_STOP=1 -c \
    "INSERT INTO users (id, username, password, email, email_normalized) VALUES ('backup-fixture-user', 'backup_fixture', 'fixture', 'backup_fixture@test.local', 'backup_fixture@test.local') ON CONFLICT (username) DO NOTHING;"

docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run --rm --no-deps --user "$(id -u):$(id -g)" go-tools-write \
    env DATABASE_URL="$DB_URL" BACKUP_DIR=/workspace/backups /workspace/deployment/scripts/backup-postgres.sh
backup_file="$(find backups -maxdepth 1 -type f -name '*.sql.gz' -print -quit)"
test -n "$backup_file"

docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run --rm --no-deps go-tools psql "postgres://test:test@host.docker.internal:${DB_PORT}/postgres?sslmode=disable" \
    -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS geoguessme_restore' -c 'CREATE DATABASE geoguessme_restore'
docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" \
    run --rm --no-deps go-tools sh -c "gzip -dc /workspace/${backup_file} | sed '/^SET transaction_timeout = 0;$/d' | psql '$RESTORE_URL' -v ON_ERROR_STOP=1"

source_count="$(docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" run --rm --no-deps go-tools psql "$DB_URL" -Atc "SELECT count(*) FROM users WHERE username = 'backup_fixture'")"
restore_count="$(docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" run --rm --no-deps go-tools psql "$RESTORE_URL" -Atc "SELECT count(*) FROM users WHERE username = 'backup_fixture'")"
source_migrations="$(docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$REPO" run --rm --no-deps go-tools psql "$DB_URL" -Atc 'SELECT count(*) FROM schema_migrations')"
restore_migrations="$(docker compose -p geoguessme-tools -f deployment/compose.tools.yaml run --rm --no-deps go-tools psql "$RESTORE_URL" -Atc 'SELECT count(*) FROM schema_migrations')"
source_constraints="$(docker compose -p geoguessme-tools -f deployment/compose.tools.yaml run --rm --no-deps go-tools psql "$DB_URL" -Atc "SELECT count(*) FROM pg_constraint WHERE connamespace = 'public'::regnamespace")"
restore_constraints="$(docker compose -p geoguessme-tools -f deployment/compose.tools.yaml run --rm --no-deps go-tools psql "$RESTORE_URL" -Atc "SELECT count(*) FROM pg_constraint WHERE connamespace = 'public'::regnamespace")"
source_checksum="$(docker compose -p geoguessme-tools -f deployment/compose.tools.yaml run --rm --no-deps go-tools psql "$DB_URL" -Atc "SELECT md5(coalesce(string_agg(id || ':' || username || ':' || email_normalized, ',' ORDER BY id), '')) FROM users")"
restore_checksum="$(docker compose -p geoguessme-tools -f deployment/compose.tools.yaml run --rm --no-deps go-tools psql "$RESTORE_URL" -Atc "SELECT md5(coalesce(string_agg(id || ':' || username || ':' || email_normalized, ',' ORDER BY id), '')) FROM users")"
test "$source_count" = 1
test "$restore_count" = 1
test "$source_migrations" = "$restore_migrations"
test "$source_constraints" = "$restore_constraints"
test "$source_checksum" = "$restore_checksum"
echo "backup-restore-rehearsal PASSED: fixture rows, checksums, constraints, migrations, and restored schema verified"
