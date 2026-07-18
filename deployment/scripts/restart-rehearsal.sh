#!/usr/bin/env bash
# Stateful restart rehearsal: seed real data, restart all services without
# deleting persistent volumes, prove schema/data/media continuity and no
# duplicate migrations or jobs, then clean up project resources only.
#
# Uses polling/state checks with deadlines — never unconditional sleeps.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

# ── Configuration ───────────────────────────────────────────────────────────
PROJECT="${GEOGUESSME_RESTART_PROJECT:-geoguessme-restart-rehearsal}"
WEB_PORT="${GEOGUESSME_RESTART_WEB_PORT:-18081}"
DB_PORT="${GEOGUESSME_RESTART_DB_PORT:-15433}"
MAILPIT_PORT="${GEOGUESSME_RESTART_MAILPIT_PORT:-18026}"
PUBLIC_URL="http://localhost:${WEB_PORT}"
CONTAINER_URL="http://host.docker.internal:${WEB_PORT}"
DB_URL="postgres://test:test@host.docker.internal:${DB_PORT}/geoguessme_test?sslmode=disable"
COMPOSE_FILE="deployment/compose.test.yaml"
TOOLS_FILE="deployment/compose.tools.yaml"
PASS=0
FAIL=0

# ── Guard: refuse production ────────────────────────────────────────────────
case "$PROJECT" in
    *rehearsal*) ;;
    *)
        echo "FATAL: project name '$PROJECT' must contain 'rehearsal' for safety" >&2
        exit 2
        ;;
esac

# ── Cleanup ─────────────────────────────────────────────────────────────────
cleanup() {
    local status=$?
    docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" down -v --remove-orphans 2>/dev/null || true
    exit "$status"
}
trap cleanup EXIT

# ── Helpers ─────────────────────────────────────────────────────────────────
pass() {
    echo "  PASS $*"
    PASS=$((PASS + 1))
}
fail() {
    echo "  FAIL $*"
    FAIL=$((FAIL + 1))
}

die() {
    echo "FATAL: $*" >&2
    exit 1
}

# Run curl inside the go-tools container against the test stack.
tool_curl() {
    docker compose -p geoguessme-tools -f "$TOOLS_FILE" --project-directory "$REPO" \
        run --rm --no-deps go-tools curl -s --fail --show-error "$@" 2>/dev/null
}

# Run psql inside the go-security container.
tool_psql() {
    docker compose -p geoguessme-tools -f "$TOOLS_FILE" --project-directory "$REPO" \
        run --rm --no-deps go-security psql "$DB_URL" -v ON_ERROR_STOP=1 "$@"
}

# Run a command inside the running minio container.
minio_exec() {
    docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" \
        exec -T minio "$@" 2>/dev/null
}

# Poll a check function until it succeeds or the deadline expires.
# Usage: poll DEADLINE_SECONDS INTERVAL_SECONDS description check_fn
poll() {
    local deadline_sec="$1" interval="$2" desc="$3" fn="$4"
    local deadline
    deadline=$(($(date +%s) + deadline_sec))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        if "$fn"; then
            pass "$desc"
            return 0
        fi
        sleep "$interval"
    done
    fail "$desc (timed out after ${deadline_sec}s)"
    return 1
}

# ── Phase 1: Start stack ────────────────────────────────────────────────────
echo "=== Phase 1: Start stack ==="

export GEOGUESSME_TEST_WEB_PORT="$WEB_PORT"
export GEOGUESSME_TEST_MAILPIT_PORT="$MAILPIT_PORT"
export GEOGUESSME_TEST_DB_PORT="$DB_PORT"
export GEOGUESSME_TEST_PUBLIC_URL="$PUBLIC_URL"

docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" up -d --wait ||
    die "compose up failed"

# Poll readiness with 120s deadline, 2s interval.
check_ready() {
    local code
    code=$(tool_curl -o /dev/null -w "%{http_code}" "$CONTAINER_URL/health/ready" || echo "000")
    [ "$code" = "200" ]
}
poll 120 2 "stack reports ready" check_ready || die "stack did not become ready"

# ── Phase 2: Seed data ──────────────────────────────────────────────────────
echo "=== Phase 2: Seed real data ==="

# Create bucket and upload a test object directly into MinIO so we can verify
# media continuity across restarts.  Use a dedicated alias to avoid colliding
# with the healthcheck's pre-configured 'local' alias.
minio_exec mc alias set rehearsal http://localhost:9000 minioadmin minioadmin || true
minio_exec mc mb rehearsal/geoguessme-test-media || true
echo -n 'restart-rehearsal-media-object' | minio_exec mc pipe rehearsal/geoguessme-test-media/rehearsal/test-object.dat || true

# Insert fixture rows via psql.  Use idempotent ON CONFLICT so the seed is
# safe to re-run against a partially-seeded database.
tool_psql -c "
INSERT INTO users (id, username, password, email, email_normalized, auth_version)
VALUES
  ('restart-alice-id', 'restart_alice', '\$2a\$10\$rehearsalrehearsalrehearsalrehearsalrehearsalrehearsalrea', 'restart_alice@test.local', 'restart_alice@test.local', 0),
  ('restart-bob-id',   'restart_bob',   '\$2a\$10\$rehearsalrehearsalrehearsalrehearsalrehearsalrehearsalrea', 'restart_bob@test.local',   'restart_bob@test.local',   0)
ON CONFLICT (id) DO NOTHING;

INSERT INTO groups (id, name, code)
VALUES ('restart-group-id', 'Restart Group', 'RESTART-CODE')
ON CONFLICT (id) DO NOTHING;

INSERT INTO group_members (group_id, user_id)
VALUES ('restart-group-id', 'restart-alice-id'),
       ('restart-group-id', 'restart-bob-id')
ON CONFLICT DO NOTHING;

INSERT INTO photos (id, user_id, group_id, storage_key, mime_type, byte_size, lat, long, lifecycle_status, expires_at, retention_at)
VALUES ('restart-photo-id', 'restart-alice-id', 'restart-group-id', 'rehearsal/test-object.dat', 'application/octet-stream', 30, 51.505, -0.09, 'ready',
        CURRENT_TIMESTAMP + interval '24 hours', CURRENT_TIMESTAMP + interval '30 days')
ON CONFLICT (id) DO NOTHING;

INSERT INTO guesses (id, photo_id, user_id, group_id, lat, long, score, distance)
VALUES ('restart-guess-id', 'restart-photo-id', 'restart-bob-id', 'restart-group-id', 51.5, -0.1, 9000, 100.0)
ON CONFLICT DO NOTHING;

INSERT INTO messages (id, group_id, user_id, kind, content)
VALUES ('restart-msg-id', 'restart-group-id', 'restart-alice-id', 'text', 'restart rehearsal message')
ON CONFLICT (id) DO NOTHING;

INSERT INTO challenge_views (photo_id, user_id, accepted_at, view_expires_at)
VALUES ('restart-photo-id', 'restart-bob-id', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP + interval '1 hour')
ON CONFLICT DO NOTHING;
"

# Verify seed took effect.
seed_user_count=$(tool_psql -Atc "SELECT count(*) FROM users WHERE id IN ('restart-alice-id','restart-bob-id')")
if [ "$seed_user_count" = "2" ]; then
    pass "seeded 2 users"
else
    die "seed verification failed: expected 2 users, got $seed_user_count"
fi

# ── Phase 3: Record pre-restart state ───────────────────────────────────────
echo "=== Phase 3: Record pre-restart state ==="

pre_migrations=$(tool_psql -Atc "SELECT count(*) FROM schema_migrations")
pre_users=$(tool_psql -Atc "SELECT count(*) FROM users WHERE id IN ('restart-alice-id','restart-bob-id')")
pre_groups=$(tool_psql -Atc "SELECT count(*) FROM groups WHERE id = 'restart-group-id'")
pre_members=$(tool_psql -Atc "SELECT count(*) FROM group_members WHERE group_id = 'restart-group-id'")
pre_photos=$(tool_psql -Atc "SELECT count(*) FROM photos WHERE id = 'restart-photo-id'")
pre_guesses=$(tool_psql -Atc "SELECT count(*) FROM guesses WHERE id = 'restart-guess-id'")
pre_messages=$(tool_psql -Atc "SELECT count(*) FROM messages WHERE id = 'restart-msg-id'")
pre_views=$(tool_psql -Atc "SELECT count(*) FROM challenge_views WHERE photo_id = 'restart-photo-id'")
pre_users_checksum=$(tool_psql -Atc "SELECT md5(string_agg(id || ':' || username || ':' || email_normalized, ',' ORDER BY id)) FROM users WHERE id IN ('restart-alice-id','restart-bob-id')")
pre_constraints=$(tool_psql -Atc "SELECT count(*) FROM pg_constraint WHERE connamespace = 'public'::regnamespace")
pre_tables=$(tool_psql -Atc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'")

# Record MinIO object presence.
pre_minio_obj=$(minio_exec mc ls rehearsal/geoguessme-test-media/rehearsal/test-object.dat | wc -l)

echo "  pre  migrations=$pre_migrations users=$pre_users groups=$pre_groups members=$pre_members photos=$pre_photos guesses=$pre_guesses msgs=$pre_messages views=$pre_views constraints=$pre_constraints tables=$pre_tables checksum=$pre_users_checksum minio_objs=$pre_minio_obj"

# ── Phase 4: Restart all services without deleting volumes ──────────────────
echo "=== Phase 4: Restart all services ==="

# docker compose down WITHOUT -v preserves named volumes.
docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" down --remove-orphans ||
    die "compose down failed"
pass "all services stopped (volumes preserved)"

# Bring everything back up.  Containers, networks, and health checks are
# recreated from scratch against the existing persistent volumes.
docker compose -f "$COMPOSE_FILE" --project-directory "$REPO" -p "$PROJECT" up -d --wait ||
    die "compose up after restart failed"

# Poll readiness with 120s deadline.
poll 120 2 "stack reports ready after full restart" check_ready || die "stack did not become ready after restart"

# ── Phase 5: Prove continuity ───────────────────────────────────────────────
echo "=== Phase 5: Prove continuity ==="

# --- 5a: Health endpoints ---
live_code=$(tool_curl -o /dev/null -w "%{http_code}" "$CONTAINER_URL/health/live")
if [ "$live_code" = "200" ]; then pass "liveness returns 200"; else fail "liveness returns $live_code (want 200)"; fi

ready_code=$(tool_curl -o /dev/null -w "%{http_code}" "$CONTAINER_URL/health/ready")
if [ "$ready_code" = "200" ]; then pass "readiness returns 200"; else fail "readiness returns $ready_code (want 200)"; fi

# --- 5b: Schema continuity (migrations unchanged, no duplicates) ---
post_migrations=$(tool_psql -Atc "SELECT count(*) FROM schema_migrations")
if [ "$post_migrations" = "$pre_migrations" ]; then
    pass "migration count unchanged ($pre_migrations)"
else
    fail "migration count changed: pre=$pre_migrations post=$post_migrations"
fi

# Verify no duplicate migration entries.
dup_migrations=$(tool_psql -Atc "SELECT count(*) FROM (SELECT version, count(*) AS cnt FROM schema_migrations GROUP BY version HAVING count(*) > 1) AS dupes")
if [ "$dup_migrations" = "0" ]; then
    pass "no duplicate migration entries"
else
    fail "found $dup_migrations duplicate migration version(s)"
fi

# --- 5c: Table structure continuity ---
post_tables=$(tool_psql -Atc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'")
if [ "$post_tables" = "$pre_tables" ]; then
    pass "table count unchanged ($pre_tables)"
else
    fail "table count changed: pre=$pre_tables post=$post_tables"
fi

post_constraints=$(tool_psql -Atc "SELECT count(*) FROM pg_constraint WHERE connamespace = 'public'::regnamespace")
if [ "$post_constraints" = "$pre_constraints" ]; then
    pass "constraint count unchanged ($pre_constraints)"
else
    fail "constraint count changed: pre=$pre_constraints post=$post_constraints"
fi

# --- 5d: Data continuity (row counts match) ---
check_count() {
    local label="$1" pre="$2" post="$3"
    if [ "$post" = "$pre" ]; then
        pass "$label count unchanged ($pre)"
    else
        fail "$label count changed: pre=$pre post=$post"
    fi
}

post_users=$(tool_psql -Atc "SELECT count(*) FROM users WHERE id IN ('restart-alice-id','restart-bob-id')")
check_count "users" "$pre_users" "$post_users"

post_groups=$(tool_psql -Atc "SELECT count(*) FROM groups WHERE id = 'restart-group-id'")
check_count "groups" "$pre_groups" "$post_groups"

post_members=$(tool_psql -Atc "SELECT count(*) FROM group_members WHERE group_id = 'restart-group-id'")
check_count "group_members" "$pre_members" "$post_members"

post_photos=$(tool_psql -Atc "SELECT count(*) FROM photos WHERE id = 'restart-photo-id'")
check_count "photos" "$pre_photos" "$post_photos"

post_guesses=$(tool_psql -Atc "SELECT count(*) FROM guesses WHERE id = 'restart-guess-id'")
check_count "guesses" "$pre_guesses" "$post_guesses"

post_messages=$(tool_psql -Atc "SELECT count(*) FROM messages WHERE id = 'restart-msg-id'")
check_count "messages" "$pre_messages" "$post_messages"

post_views=$(tool_psql -Atc "SELECT count(*) FROM challenge_views WHERE photo_id = 'restart-photo-id'")
check_count "challenge_views" "$pre_views" "$post_views"

# --- 5e: Data checksum continuity ---
post_users_checksum=$(tool_psql -Atc "SELECT md5(string_agg(id || ':' || username || ':' || email_normalized, ',' ORDER BY id)) FROM users WHERE id IN ('restart-alice-id','restart-bob-id')")
if [ "$post_users_checksum" = "$pre_users_checksum" ]; then
    pass "users checksum matches ($pre_users_checksum)"
else
    fail "users checksum mismatch: pre=$pre_users_checksum post=$post_users_checksum"
fi

# --- 5f: Media continuity (MinIO object survives restart) ---
# Re-establish mc alias lost across container restart (stored in ephemeral
# container filesystem, not the /data volume).
minio_exec mc alias set rehearsal http://localhost:9000 minioadmin minioadmin || true
post_minio_obj=$(minio_exec mc ls rehearsal/geoguessme-test-media/rehearsal/test-object.dat | wc -l)
if [ "$post_minio_obj" -ge 1 ]; then
    pass "MinIO media object survives restart"
else
    fail "MinIO media object missing after restart"
fi

# Also verify the object content is intact.
obj_content=$(minio_exec mc cat rehearsal/geoguessme-test-media/rehearsal/test-object.dat || echo "")
if [ "$obj_content" = "restart-rehearsal-media-object" ]; then
    pass "MinIO object content intact"
else
    fail "MinIO object content corrupted or missing"
fi

# --- 5g: No runaway deletion jobs ---
# The storage cleanup worker may have enqueued jobs for expired objects, but
# our seeded photo has a future retention_at, so no new deletion jobs should
# target it.
deletion_jobs=$(tool_psql -Atc "SELECT count(*) FROM media_deletion_jobs WHERE storage_key = 'rehearsal/test-object.dat'")
if [ "$deletion_jobs" = "0" ]; then
    pass "no deletion jobs for active photo"
else
    fail "unexpected $deletion_jobs deletion job(s) targeting active photo"
fi

# --- 5h: Metrics endpoint (no abnormal backlog) ---
metrics_body=$(tool_curl "$CONTAINER_URL/metrics" 2>/dev/null || echo "")
backlog=$(echo "$metrics_body" | grep 'geoguessme_storage_cleanup_backlog' | grep -oE '[0-9]+' || echo "0")
if [ "${backlog:-0}" -le 10 ]; then
    pass "cleanup backlog nominal (${backlog:-0})"
else
    fail "cleanup backlog elevated: ${backlog:-0}"
fi

# ── Phase 6: Summary ────────────────────────────────────────────────────────
echo "=== Summary ==="
if [ "$FAIL" -eq 0 ]; then
    echo "restart-rehearsal PASSED ($PASS checks, $FAIL failures): all services recovered with persistent volumes, schema, data, and media intact"
else
    echo "restart-rehearsal FAILED ($PASS checks, $FAIL failures)"
    exit 1
fi
