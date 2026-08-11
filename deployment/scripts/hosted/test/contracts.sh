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
FRONTEND_DOCKERFILE="$ROOT/deployment/docker/frontend.Dockerfile"
BACKEND_DOCKERFILE="$ROOT/deployment/docker/backend.Dockerfile"
SECRET_GENERATOR="$ROOT/deployment/scripts/generate-hosted-secret.sh"

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

# Both workflows must match the forced command's arity. PRs #56 and #57 extended
# this command string with VAPID material while forced-command.sh was untouched,
# so every development deployment was rejected with exit 126.
assert_forced_command_arity() {
    workflow=$1
    command_string=$(sed -n 's/.*"\(deploy \$BACKEND \$WEB \$GITHUB_SHA\)".*/\1/p' "$workflow")
    [ -n "$command_string" ] ||
        fail "$workflow must send exactly: deploy \$BACKEND \$WEB \$GITHUB_SHA"
    # shellcheck disable=SC2086
    set -- $command_string
    [ "$#" -eq 4 ] || fail "$workflow sends $# fields; the forced command accepts 4"
}
assert_forced_command_arity "$ROOT/.github/workflows/deploy.yml"
assert_forced_command_arity "$ROOT/.github/workflows/release.yml"

# sshd never interprets an environment-variable prefix in a forced command and
# the forced command rejects extra positional fields, so secrets cannot travel
# this way. They belong in the SOPS-encrypted dotenv.
if grep -nE 'VAPID' "$ROOT/.github/workflows/deploy.yml" \
    "$ROOT/.github/workflows/release.yml" "$DEPLOY"; then
    fail 'VAPID material must travel in the encrypted dotenv, not the deploy protocol'
fi

assert_contains "$COMMON" '-f "$CONFIG_ROOT/compose.production.yaml"'
assert_contains "$COMMON" '-f "$CONFIG_ROOT/compose.hosted.yaml"'
assert_contains "$DEPLOY" 'workflows/deploy\.yml@refs/heads/dev'
assert_contains "$DEPLOY" 'workflows/release\.yml@refs/heads/main'
assert_contains "$DEPLOY" 'github.com/Anko59/GeoguessMe/.github/workflows/deploy'
assert_contains "$DEPLOY" 'github.com/Anko59/GeoguessMe/.github/workflows/release'
assert_contains "$DEPLOY" '--annotations "revision=$revision"'
assert_contains "$ROOT/.github/workflows/deploy.yml" '-a "revision=$GITHUB_SHA"'
assert_contains "$ROOT/.github/workflows/release.yml" '-a "revision=$GITHUB_SHA"'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'cosign-release: v2.6.5'
assert_contains "$ROOT/.github/workflows/release.yml" 'cosign-release: v2.6.5'
assert_contains "$ROOT/.github/workflows/deploy.yml" '049777d30f9bf93da6df8bbe31383460eb2aa51a832c6551824d56f9fcc55974  cloudflared.deb'
assert_contains "$ROOT/.github/workflows/release.yml" '049777d30f9bf93da6df8bbe31383460eb2aa51a832c6551824d56f9fcc55974  cloudflared.deb'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'docker pull "$BACKEND_IMAGE"'
assert_contains "$ROOT/.github/workflows/deploy.yml" 'docker pull "$WEB_IMAGE"'
assert_contains "$ROOT/tools/make/deployment.mk" 'docker image inspect "$$img"'
assert_contains "$ROOT/.github/workflows/release.yml" 'branches: [main]'
assert_contains "$ROOT/.github/workflows/release.yml" 'tag=v0.2.0'
assert_contains "$ROOT/.github/workflows/release.yml" 'tag_name: ${{ steps.source.outputs.tag }}'
assert_contains "$ROOT/.github/workflows/release.yml" 'main_tree=$(git rev-parse "$GITHUB_SHA^{tree}")'
assert_contains "$ROOT/.github/workflows/release.yml" 'cosign verify'
assert_contains "$ROOT/.github/workflows/release.yml" 'imagetools create'
assert_contains "$ROOT/.github/workflows/release.yml" 'actual_backend'

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
assert_contains "$ROOT/deployment/env/dev.env.example" 'geoguessme-database-backups/dev'
assert_contains "$ROOT/deployment/env/production.env.example" 'geoguessme-database-backups/production'

# Both hosted templates declare APP_ENV=production, so the backend enforces every
# production requirement against each of them. Their key sets must stay identical:
# PR #40 added the Web Push keys to production.env.example only, which made the
# hosted development backend refuse to start on every deployment thereafter.
env_keys() {
    sed -n 's/^\([A-Z][A-Z0-9_]*\)=.*/\1/p' "$1" | sort
}
assert_contains "$ROOT/deployment/env/dev.env.example" 'APP_ENV=production'
assert_contains "$ROOT/deployment/env/production.env.example" 'APP_ENV=production'
if [ "$(env_keys "$ROOT/deployment/env/dev.env.example")" != \
    "$(env_keys "$ROOT/deployment/env/production.env.example")" ]; then
    fail 'dev.env.example and production.env.example must declare the same keys'
fi
assert_contains "$SECRET_GENERATOR" 'RESTIC_REPOSITORY=s3:https://%s.r2.cloudflarestorage.com/geoguessme-database-backups/%s'
# The secrets-generate recipe lives in a responsibility fragment, so search
# the public Makefile and all fragments.
if ! cat "$ROOT"/Makefile "$ROOT"/tools/make/*.mk | grep -Fq 'generate-hosted-secret.sh |'; then
    fail "$ROOT/Makefile does not contain: generate-hosted-secret.sh |"
fi
assert_contains "$ROOT/deployment/scripts/hosted/restore-rehearsal.sh" 'docker rm -f'
assert_contains "$HEALTH" 'for service in postgres backend web'
assert_contains "$HEALTH" '.State.Health.Status'
assert_contains "$COMMON" 'BACKEND_IMAGE="$backend"'

# Secret generation fills every external credential, uses an isolated backup
# prefix, and does not leave template placeholders in the deployment payload.
generated=$(TARGET_ENV=dev \
    BREVO_SMTP_USERNAME=smtp-user BREVO_SMTP_PASSWORD=smtp-password \
    GHCR_USERNAME=registry-user GHCR_TOKEN=registry-token \
    MEDIA_ACCESS_KEY_ID=media-key MEDIA_SECRET_ACCESS_KEY=media-secret \
    BACKUP_ACCESS_KEY_ID=backup-key BACKUP_SECRET_ACCESS_KEY=backup-secret \
    CLOUDFLARE_ACCOUNT_ID=account-id \
    VAPID_PUBLIC_KEY=vapid-public VAPID_PRIVATE_KEY=vapid-private \
    VAPID_SUBJECT=mailto:contract@example.invalid "$SECRET_GENERATOR")
case "$generated" in
    *replace-* | *ACCOUNT_ID*) fail 'generated secret payload contains a template placeholder' ;;
esac
printf '%s\n' "$generated" | grep -Fq 'SMTP_USERNAME=smtp-user' ||
    fail 'generated secret payload omitted the SMTP username'
printf '%s\n' "$generated" | grep -Fq 'S3_ACCESS_KEY=media-key' ||
    fail 'generated secret payload omitted the media credential'
printf '%s\n' "$generated" | grep -Fq 'geoguessme-database-backups/dev' ||
    fail 'generated secret payload omitted the isolated backup prefix'
printf '%s\n' "$generated" | grep -Fq 'VAPID_PRIVATE_KEY=vapid-private' ||
    fail 'generated secret payload omitted the supplied Web Push keypair'

# A missing Web Push key must stop generation rather than emit the template
# placeholder, which is non-empty and so would satisfy the backend's production
# validation while breaking every push delivery at runtime.
if VAPID_PUBLIC_KEY='' TARGET_ENV=dev \
    BREVO_SMTP_USERNAME=smtp-user BREVO_SMTP_PASSWORD=smtp-password \
    GHCR_USERNAME=registry-user GHCR_TOKEN=registry-token \
    MEDIA_ACCESS_KEY_ID=media-key MEDIA_SECRET_ACCESS_KEY=media-secret \
    BACKUP_ACCESS_KEY_ID=backup-key BACKUP_SECRET_ACCESS_KEY=backup-secret \
    CLOUDFLARE_ACCOUNT_ID=account-id VAPID_PRIVATE_KEY=vapid-private \
    VAPID_SUBJECT=mailto:contract@example.invalid \
    "$SECRET_GENERATOR" >/dev/null 2>&1; then
    fail 'secret generation accepted a missing Web Push public key'
fi

# Backup age is calculated from epoch markers without wall-clock sleeps.
marker=$(mktemp)
trap 'rm -f "$marker"' EXIT INT TERM
printf '100\n' >"$marker"
age=$(GEOGUESSME_NOW_EPOCH=7301 sh -c '. "$1"; backup_age_seconds "$2"' _ "$COMMON" "$marker")
[ "$age" -eq 7201 ] || fail 'backup marker age calculation is incorrect'

# Cloudflare client IP replaces both forwarding headers at the only gateway.
assert_contains "$CADDY" 'header_up X-Forwarded-For {http.request.header.Cf-Connecting-Ip}'
assert_contains "$CADDY" 'header_up X-Real-IP {http.request.header.Cf-Connecting-Ip}'
assert_contains "$CADDY" "script-src 'self' 'wasm-unsafe-eval'"
assert_contains "$FRONTEND_DOCKERFILE" 'org.opencontainers.image.base.name="caddy:2.11.4-alpine"'
assert_contains "$FRONTEND_DOCKERFILE" 'org.opencontainers.image.base.digest="sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648"'
assert_contains "$BACKEND_DOCKERFILE" 'org.opencontainers.image.base.name="gcr.io/distroless/static-debian12:nonroot"'
assert_contains "$BACKEND_DOCKERFILE" 'org.opencontainers.image.base.digest="sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b"'

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
