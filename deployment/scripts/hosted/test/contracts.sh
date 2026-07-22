#!/bin/sh
# shellcheck disable=SC2016
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../.." && pwd)
DEPLOY="$ROOT/deployment/scripts/hosted/deploy.sh"
BACKUP="$ROOT/deployment/scripts/hosted/backup.sh"
FORCED="$ROOT/deployment/scripts/hosted/forced-command.sh"
COMMON="$ROOT/deployment/scripts/hosted/common.sh"
HEALTH="$ROOT/deployment/scripts/hosted/health-check.sh"
COMPOSE="$ROOT/deployment/compose.production.yaml"
HOSTED="$ROOT/deployment/compose.hosted.yaml"
CADDY="$ROOT/deployment/caddy/Caddyfile"

fail() {
    printf 'contract test failed: %s\n' "$1" >&2
    exit 1
}

assert_contains() {
    grep -Fq -e "$2" "$1" || fail "$1 does not contain: $2"
}

line_of() {
    grep -n -m1 "$2" "$1" | cut -d: -f1
}

# Environment isolation and loopback-only ingress.
assert_contains "$COMPOSE" 'name: ${COMPOSE_PROJECT_NAME:-geoguessme-prod}'
assert_contains "$COMPOSE" '127.0.0.1:${GEOGUESSME_WEB_PORT:-8081}:80'
assert_contains "$HOSTED" 'database:/var/lib/postgresql/data'
assert_contains "$HOSTED" '${GEOGUESSME_ENV_FILE:-deployment/env/production.env}'

# Forced commands cannot select another environment or obtain a shell.
assert_contains "$FORCED" '[ "$#" -eq 4 ]'
assert_contains "$FORCED" '[ "$1" = deploy ]'
assert_contains "$FORCED" 'deploy.sh "$allowed_environment"'
assert_contains "$COMMON" '-f "$CONFIG_ROOT/compose.production.yaml"'
assert_contains "$COMMON" '-f "$CONFIG_ROOT/compose.hosted.yaml"'
assert_contains "$DEPLOY" 'workflows/deploy\.yml@refs/heads/dev'
assert_contains "$DEPLOY" 'workflows/release\.yml@refs/tags/v'
assert_contains "$DEPLOY" '--annotations "revision=$revision"'
assert_contains "$ROOT/.github/workflows/deploy.yml" '-a "revision=$GITHUB_SHA"'
assert_contains "$ROOT/.github/workflows/release.yml" '-a "revision=$GITHUB_SHA"'

# Signature verification and backup happen before pull and migration.
verify_line=$(line_of "$DEPLOY" 'COSIGN_IMAGE.*verify')
backup_line=$(line_of "$DEPLOY" 'backup.sh.*pre-deploy')
secret_line=$(line_of "$DEPLOY" 'mv.*temporary_secret.*secret_file')
pull_line=$(line_of "$DEPLOY" 'pull backend web postgres')
migrate_line=$(line_of "$DEPLOY" 'migration migrate up')
[ "$verify_line" -lt "$backup_line" ] || fail 'signature verification must precede backup'
[ "$verify_line" -lt "$pull_line" ] || fail 'signature verification must precede pull'
[ "$backup_line" -lt "$pull_line" ] || fail 'pre-deploy backup must precede pull'
[ "$backup_line" -lt "$secret_line" ] || fail 'backup must precede candidate secret activation'
[ "$secret_line" -lt "$pull_line" ] || fail 'candidate secret activation must precede pull'
[ "$pull_line" -lt "$migrate_line" ] || fail 'pull must precede migration'

# One host-wide deployment lock, image rollback only, and no automatic restore.
assert_contains "$DEPLOY" 'geoguessme-deploy.lock'
assert_contains "$DEPLOY" 'previous images and secrets were restored; database was not restored'
assert_contains "$DEPLOY" 'secret_replaced=true'
if grep -Eq 'restore-postgres|pg_restore' "$DEPLOY"; then
    fail 'deploy script must never restore a database automatically'
fi

# Backup retention and isolated restore requirements.
assert_contains "$BACKUP" '--keep-hourly 24 --keep-daily 14 --keep-weekly 8 --keep-monthly 6'
assert_contains "$BACKUP" 'gzip -t'
assert_contains "$ROOT/deployment/scripts/hosted/restore-rehearsal.sh" 'docker rm -f'
assert_contains "$HEALTH" 'for service in postgres backend web'
assert_contains "$HEALTH" '.State.Health.Status'
assert_contains "$COMMON" 'BACKEND_IMAGE="$backend"'

# Backup age is calculated from epoch markers without wall-clock sleeps.
marker=$(mktemp)
trap 'rm -f "$marker"' EXIT INT TERM
printf '100\n' >"$marker"
age=$(GEOGUESSME_NOW_EPOCH=7301 sh -c '. "$1"; backup_age_seconds "$2"' _ "$COMMON" "$marker")
[ "$age" -eq 7201 ] || fail 'backup marker age calculation is incorrect'

# Cloudflare client IP replaces both forwarding headers at the only gateway.
assert_contains "$CADDY" 'header_up X-Forwarded-For {http.request.header.Cf-Connecting-Ip}'
assert_contains "$CADDY" 'header_up X-Real-IP {http.request.header.Cf-Connecting-Ip}'

# Reject malformed image input before touching Docker or secrets.
if "$DEPLOY" dev latest latest 0123456789012345678901234567890123456789 >/dev/null 2>&1; then
    fail 'mutable image references were accepted'
fi
short='example.invalid/app@sha256:aa'
if "$DEPLOY" dev "$short" "$short" 0123456789012345678901234567890123456789 >/dev/null 2>&1; then
    fail 'short image digests were accepted'
fi

# Forced lock and signature failures stop before any deployment operation.
test_root=$(mktemp -d)
trap 'rm -f "$marker"; rm -rf "$test_root"' EXIT INT TERM
mkdir -p "$test_root/app/releases/0123456789012345678901234567890123456789" \
    "$test_root/secrets" "$test_root/state" "$test_root/locks" "$test_root/bin" "$test_root/home/.docker"
printf 'GHCR_USERNAME=test\nGHCR_TOKEN=test\n' >"$test_root/secrets/dev.env"
chmod 600 "$test_root/secrets/dev.env"
cat >"$test_root/bin/flock" <<'EOF'
#!/bin/sh
[ "${FAKE_FLOCK_FAILURE:-0}" -eq 0 ]
EOF
cat >"$test_root/bin/docker" <<'EOF'
#!/bin/sh
case " $* " in *' verify '*) exit 42 ;; esac
exit 0
EOF
chmod 755 "$test_root/bin/flock" "$test_root/bin/docker"
digest=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
image="example.invalid/app@sha256:$digest"
run_deploy() {
    PATH="$test_root/bin:$PATH" \
        HOME="$test_root/home" \
        GEOGUESSME_APP_ROOT="$test_root/app" \
        GEOGUESSME_STATE_ROOT="$test_root/state" \
        GEOGUESSME_SECRET_ROOT="$test_root/secrets" \
        GEOGUESSME_LOCK_ROOT="$test_root/locks" \
        "$DEPLOY" dev "$image" "$image" 0123456789012345678901234567890123456789
}
if FAKE_FLOCK_FAILURE=1 run_deploy >/dev/null 2>&1; then
    fail 'deployment continued after lock rejection'
fi
if run_deploy >/dev/null 2>&1; then
    fail 'deployment continued after signature rejection'
fi

# Secrets must never be traced or dumped by operator scripts.
if grep -En 'set -x|printenv|env[[:space:]]*$' "$ROOT"/deployment/scripts/hosted/*.sh; then
    fail 'operator scripts contain a secret-dumping primitive'
fi

printf 'hosted deployment contracts passed\n'
