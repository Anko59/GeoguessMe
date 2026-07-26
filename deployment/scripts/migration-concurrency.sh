#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Deterministic migration test: representative legacy fixture, concurrent run,
# idempotent rerun, duplicate survivor verification, and rollback expectations.
#
# Starts from a realistic pre-migration database state (every legacy column
# subset, backfill edge case, and NULL variant).  Runs all migrations, verifies
# every legacy row survives with correct transformations, proves the migration
# 003 duplicate-survivor ORDER BY logic, and documents forward-only rollback
# expectations.  No host DB tools, sleeps, retries, or secrets.
# ==============================================================================

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

PROJECT="${GEOGUESSME_MIGRATION_PROJECT:-geoguessme-migration-test}"
DB_PORT="${GEOGUESSME_MIGRATION_DB_PORT:-15433}"
DB_URL="postgres://test:test@host.docker.internal:${DB_PORT}/geoguessme_test?sslmode=disable"
TOOLS_PSQL=(docker compose -p geoguessme-tools -f deployment/compose.tools.yaml
    --project-directory "$REPO" run --rm --no-deps go-security
    psql "$DB_URL" -v ON_ERROR_STOP=1)
FIXTURE="/workspace/deployment/scripts/legacy-migration-fixture.sql"

cleanup() {
    local status=$?
    docker compose -f deployment/compose.test.yaml --project-directory "$REPO" \
        -p "$PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true
    exit "$status"
}
trap cleanup EXIT

# -- helpers ----------------------------------------------------------------
psql_exec() { "${TOOLS_PSQL[@]}" -Atc "$1"; }
psql_query() { "${TOOLS_PSQL[@]}" -Atc "$1"; }
assert_eq() {
    local got="$1" want="$2" label="$3"
    if [ "$got" != "$want" ]; then
        printf 'FAIL: %s\n  expected: %s\n  got:      %s\n' "$label" "$want" "$got" >&2
        exit 1
    fi
    printf '  PASS: %s = %s\n' "$label" "$got"
}

# -- 1. start DB and apply legacy fixture -----------------------------------
echo "=== Starting test database ==="
export GEOGUESSME_TEST_DB_PORT="$DB_PORT"
docker compose -f deployment/compose.test.yaml --project-directory "$REPO" \
    -p "$PROJECT" up -d db --wait

echo "=== Applying legacy fixture (pre-migration state) ==="
"${TOOLS_PSQL[@]}" -f "$FIXTURE"

# Verify fixture was applied
assert_eq "$(psql_query "SELECT count(*) FROM users")" 4 "fixture user count"
assert_eq "$(psql_query "SELECT count(*) FROM groups")" 2 "fixture group count"
assert_eq "$(psql_query "SELECT count(*) FROM group_members")" 4 "fixture member count"
assert_eq "$(psql_query "SELECT count(*) FROM photos")" 4 "fixture photo count"
assert_eq "$(psql_query "SELECT count(*) FROM guesses")" 3 "fixture guess count"
assert_eq "$(psql_query "SELECT count(*) FROM messages")" 4 "fixture message count"
# schema_migrations must not exist before MigrateUp
assert_eq "$(psql_query "SELECT to_regclass('public.schema_migrations') IS NULL")" \
    "t" "schema_migrations absent before MigrateUp"

# -- 2. concurrent migration run --------------------------------------------
echo "=== Running two concurrent migrations ==="
first="$(docker compose -f deployment/compose.test.yaml --project-directory "$REPO" \
    -p "$PROJECT" run -d --no-deps migration | tail -n 1)"
second="$(docker compose -f deployment/compose.test.yaml --project-directory "$REPO" \
    -p "$PROJECT" run -d --no-deps migration | tail -n 1)"

first_status="$(docker wait "$first")"
second_status="$(docker wait "$second")"
test "$first_status" -eq 0 || {
    echo "first migration failed with status $first_status"
    exit 1
}
test "$second_status" -eq 0 || {
    echo "second migration failed with status $second_status"
    exit 1
}
docker rm "$first" "$second" >/dev/null
echo "  PASS: both concurrent migrations succeeded"

# All 4 migrations must be recorded
assert_eq "$(psql_query "SELECT count(*) FROM schema_migrations")" 4 \
    "schema_migrations entries after concurrent run"

# -- 3. verify every backfill transformation --------------------------------
echo "=== Verifying legacy data transformations ==="

# --- users: email backfill, email_normalized, score column dropped, auth_version
assert_eq "$(psql_query "SELECT email FROM users WHERE id='legacy-001'")" \
    "player_one@legacy.invalid" \
    "legacy-001 email backfill (NULL→username@legacy.invalid)"
assert_eq "$(psql_query "SELECT email_normalized FROM users WHERE id='legacy-001'")" \
    "player_one@legacy.invalid" \
    "legacy-001 email_normalized"
assert_eq "$(psql_query "SELECT email FROM users WHERE id='legacy-002'")" \
    "Player_Two@legacy.invalid" \
    "legacy-002 email backfill (preserves case in email, normalized separately)"
assert_eq "$(psql_query "SELECT email_normalized FROM users WHERE id='legacy-002'")" \
    "player_two@legacy.invalid" \
    "legacy-002 email_normalized lowercased"
assert_eq "$(psql_query "SELECT email FROM users WHERE id='legacy-003'")" \
    "  spaced_user  @legacy.invalid" \
    "legacy-003 email backfill (username with spaces)"
assert_eq "$(psql_query "SELECT email_normalized FROM users WHERE id='legacy-003'")" \
    "spaced_user  @legacy.invalid" \
    "legacy-003 email_normalized trimmed+lowered"
assert_eq "$(psql_query "SELECT email FROM users WHERE id='legacy-004'")" \
    "player_four@legacy.invalid" \
    "legacy-004 email backfill"
# score column must be gone
assert_eq "$(psql_query "SELECT count(*) FROM information_schema.columns WHERE table_name='users' AND column_name='score'")" \
    "0" "score column dropped"
# auth_version default
assert_eq "$(psql_query "SELECT auth_version FROM users WHERE id='legacy-001'")" \
    "0" "legacy-001 auth_version=0"
# updated_at and deleted_at exist
assert_eq "$(psql_query "SELECT count(*) FROM information_schema.columns WHERE table_name='users' AND column_name='updated_at'")" \
    "1" "updated_at column exists"
assert_eq "$(psql_query "SELECT count(*) FROM information_schema.columns WHERE table_name='users' AND column_name='deleted_at'")" \
    "1" "deleted_at column exists"

# --- photos: storage_key, mime_type, byte_size, lifecycle_status, retention_at
assert_eq "$(psql_query "SELECT storage_key FROM photos WHERE id='photo-001'")" \
    "abc123.jpg" \
    "photo-001 storage_key stripped from url"
assert_eq "$(psql_query "SELECT storage_key FROM photos WHERE id='photo-002'")" \
    "nested/def456.png" \
    "photo-002 storage_key with nested path"
assert_eq "$(psql_query "SELECT storage_key FROM photos WHERE id='photo-003'")" \
    "" \
    "photo-003 storage_key empty (NULL url→empty regexp result→COALESCE→'')"
assert_eq "$(psql_query "SELECT storage_key FROM photos WHERE id='photo-004'")" \
    "ghi789.webp" \
    "photo-004 storage_key from url"
assert_eq "$(psql_query "SELECT mime_type FROM photos WHERE id='photo-001'")" \
    "image/jpeg" \
    "photo-001 mime_type default"
assert_eq "$(psql_query "SELECT byte_size FROM photos WHERE id='photo-001'")" \
    "0" \
    "photo-001 byte_size default"
assert_eq "$(psql_query "SELECT lifecycle_status FROM photos WHERE id='photo-001'")" \
    "ready" \
    "photo-001 lifecycle_status default"
# retention_at for photo with explicit expires_at: COALESCE(retention_at, COALESCE(expires_at, created_at+30d))
# expires_at is set, so retention_at = expires_at
assert_eq "$(psql_query "SELECT retention_at FROM photos WHERE id='photo-001'")" \
    "2024-01-16 10:00:00+00" \
    "photo-001 retention_at = expires_at"
# photo-004 has NULL expires_at AND NULL retention_at.
# retention_at backfill runs FIRST and falls through to created_at+30d
# because expires_at is still NULL.  Then expires_at backfill runs.
assert_eq "$(psql_query "SELECT expires_at FROM photos WHERE id='photo-004'")" \
    "2024-04-02 10:00:00+00" \
    "photo-004 expires_at backfilled (created_at+24h)"
assert_eq "$(psql_query "SELECT retention_at FROM photos WHERE id='photo-004'")" \
    "2024-05-01 10:00:00+00" \
    "photo-004 retention_at = created_at+30d (expires_at still NULL when retention backfill runs)"
# indexes exist
assert_eq "$(psql_query "SELECT count(*) FROM pg_indexes WHERE indexname='photos_retention_idx'")" \
    "1" "photos_retention_idx exists"
assert_eq "$(psql_query "SELECT count(*) FROM pg_indexes WHERE indexname='photos_expiry_idx'")" \
    "1" "photos_expiry_idx exists"

# --- guesses: group_id backfilled from photos
assert_eq "$(psql_query "SELECT group_id FROM guesses WHERE id='guess-001'")" \
    "group-001" \
    "guess-001 group_id backfilled from photo"
assert_eq "$(psql_query "SELECT group_id FROM guesses WHERE id='guess-002'")" \
    "group-001" \
    "guess-002 group_id backfilled from photo"
assert_eq "$(psql_query "SELECT group_id FROM guesses WHERE id='guess-003'")" \
    "group-002" \
    "guess-003 group_id backfilled from photo"
assert_eq "$(psql_query "SELECT count(*) FROM pg_indexes WHERE indexname='guesses_photo_user_key'")" \
    "1" "guesses_photo_user_key index exists"
assert_eq "$(psql_query "SELECT count(*) FROM pg_indexes WHERE indexname='guesses_group_idx'")" \
    "1" "guesses_group_idx index exists"

# --- messages: kind default, photo_id nullable
assert_eq "$(psql_query "SELECT kind FROM messages WHERE id='msg-001'")" \
    "text" \
    "msg-001 kind default 'text'"
assert_eq "$(psql_query "SELECT kind FROM messages WHERE id='msg-004'")" \
    "text" \
    "msg-004 kind default 'text'"
# photo_id column exists and is NULL
assert_eq "$(psql_query "SELECT photo_id FROM messages WHERE id='msg-001'")" \
    "" \
    "msg-001 photo_id is NULL"
assert_eq "$(psql_query "SELECT count(*) FROM pg_indexes WHERE indexname='messages_group_created_idx'")" \
    "1" "messages_group_created_idx index exists"

# --- tables created from scratch by 001 must exist
for tbl in challenge_views refresh_sessions email_verification_tokens password_reset_tokens websocket_tickets; do
    assert_eq "$(psql_query "SELECT to_regclass('public.$tbl') IS NOT NULL")" \
        "t" "table $tbl exists"
done

# --- migration 002: auth_version index, media_deletion_jobs table
assert_eq "$(psql_query "SELECT count(*) FROM pg_indexes WHERE indexname='users_auth_version_idx'")" \
    "1" "users_auth_version_idx exists"
assert_eq "$(psql_query "SELECT to_regclass('public.media_deletion_jobs') IS NOT NULL")" \
    "t" "media_deletion_jobs table exists"
assert_eq "$(psql_query "SELECT count(*) FROM pg_indexes WHERE indexname='media_deletion_jobs_pending_idx'")" \
    "1" "media_deletion_jobs_pending_idx exists"

# --- migration 003: unique partial index
assert_eq "$(psql_query "SELECT count(*) FROM pg_indexes WHERE indexname='media_deletion_jobs_active_storage_key_idx'")" \
    "1" "media_deletion_jobs_active_storage_key_idx exists"

# -- 4. idempotent rerun ----------------------------------------------------
echo "=== Verifying idempotent rerun ==="
docker compose -f deployment/compose.test.yaml --project-directory "$REPO" \
    -p "$PROJECT" run --rm migration >/dev/null
assert_eq "$(psql_query "SELECT count(*) FROM schema_migrations")" 4 \
    "schema_migrations count unchanged after rerun"
# Row counts must be identical (no duplicates or deletions from re-run)
assert_eq "$(psql_query "SELECT count(*) FROM users")" 4 "user count unchanged"
assert_eq "$(psql_query "SELECT count(*) FROM groups")" 2 "group count unchanged"
assert_eq "$(psql_query "SELECT count(*) FROM photos")" 4 "photo count unchanged"
assert_eq "$(psql_query "SELECT count(*) FROM guesses")" 3 "guess count unchanged"
assert_eq "$(psql_query "SELECT count(*) FROM messages")" 4 "message count unchanged"
echo "  PASS: idempotent rerun preserved all data"

# -- 5. duplicate survivor test (migration 003 dedup logic) -----------------
echo "=== Verifying duplicate survivor behavior ==="

# To test the dedup logic, temporarily drop the partial unique index that 003
# created -- this simulates the pre-003 state where duplicates could exist.
psql_exec "DROP INDEX IF EXISTS media_deletion_jobs_active_storage_key_idx"

# Insert three active jobs for the same storage key with deterministic
# ordering: the survivor must be the one with earliest created_at
# (then next_attempt_at, then id as tie-breakers per 003's ROW_NUMBER).
psql_exec "
INSERT INTO media_deletion_jobs(id, storage_key, source, created_at, next_attempt_at)
VALUES
    ('dup-002', 'shared/key.jpg', 'manual',  '2025-06-01T10:00:00Z', '2025-06-02T10:00:00Z'),
    ('dup-001', 'shared/key.jpg', 'account', '2025-06-01T09:00:00Z', '2025-06-02T10:00:00Z'),
    ('dup-003', 'shared/key.jpg', 'group',   '2025-06-01T11:00:00Z', '2025-06-02T10:00:00Z');
"
assert_eq "$(psql_query "SELECT count(*) FROM media_deletion_jobs WHERE storage_key='shared/key.jpg' AND completed_at IS NULL")" \
    "3" "three active duplicates inserted"

# Run the exact dedup DELETE from migration 003
psql_exec "
DELETE FROM media_deletion_jobs
WHERE id IN (
    SELECT dupes.id FROM (
        SELECT id,
            ROW_NUMBER() OVER (
                PARTITION BY storage_key
                ORDER BY created_at, next_attempt_at, id
            ) AS rn
        FROM media_deletion_jobs
        WHERE completed_at IS NULL
    ) AS dupes
    WHERE dupes.rn > 1
);
"
# Survivor must be dup-001 (earliest created_at: 09:00 < 10:00 < 11:00)
assert_eq "$(psql_query "SELECT id FROM media_deletion_jobs WHERE storage_key='shared/key.jpg' AND completed_at IS NULL")" \
    "dup-001" "duplicate survivor is earliest created_at row"

# Re-create the partial unique index (as 003 does with CREATE UNIQUE INDEX
# IF NOT EXISTS).  This also verifies no lingering duplicates exist.
psql_exec "
CREATE UNIQUE INDEX media_deletion_jobs_active_storage_key_idx
    ON media_deletion_jobs (storage_key)
    WHERE completed_at IS NULL
"

# The partial unique index must reject a new duplicate insert (ON CONFLICT DO
# NOTHING at runtime becomes a silent no-op).  Verify with a direct INSERT.
psql_exec "
INSERT INTO media_deletion_jobs(id, storage_key, source)
VALUES ('dup-new', 'shared/key.jpg', 'retention')
ON CONFLICT (storage_key) WHERE completed_at IS NULL DO NOTHING;
"
assert_eq "$(psql_query "SELECT id FROM media_deletion_jobs WHERE storage_key='shared/key.jpg' AND completed_at IS NULL")" \
    "dup-001" "ON CONFLICT DO NOTHING prevents duplicate active job"

# Clean up test rows
psql_exec "DELETE FROM media_deletion_jobs WHERE storage_key='shared/key.jpg'"
echo "  PASS: duplicate survivor logic matches 003 ORDER BY"

# -- 6. rollback expectations (documented, not automated) -------------------
echo ""
echo "=== Rollback expectations (documented) ==="
echo "  Migration system: forward-only, idempotent, advisory-locked."
echo "  Rollback method: deploy previous binary, restore from backup."
echo "  No automated downgrade SQL exists.  Each migration runs in a single"
echo "  transaction; a failure rolls the entire migration back.  Fix the"
echo "  cause and re-run 'make migrate-up'."

# -- all done ----------------------------------------------------------------
echo ""
echo "migration-test PASSED: legacy fixture transformed, concurrent+idempotent"
echo "runs verified, duplicate survivor behavior confirmed, rollback documented."
