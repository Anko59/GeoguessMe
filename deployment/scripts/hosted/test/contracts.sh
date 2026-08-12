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
RESTIC_DOCKERFILE="$ROOT/deployment/docker/restic-tools.Dockerfile"
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
assert_contains "$FORCED" '[ "$2" = "$allowed_environment" ]'
assert_contains "$FORCED" 'bin/verify-deployment-hashes.sh'

# Both workflows must match the forced command's arity. This protocol is
# provisioned root-owned on the host and must stay compatible between releases.
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
assert_contains "$ROOT/.release-version" '0.3.0'
assert_contains "$ROOT/.github/workflows/release.yml" 'release_version=$(tr -d'
assert_contains "$ROOT/.github/workflows/release.yml" 'tag="v$release_version"'
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
assert_contains "$HEALTH" 'verify-deployment-hashes.sh" "$environment"'
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
assert_contains "$FRONTEND_DOCKERFILE" 'caddy:2.11.4-builder-alpine@sha256:8e89605351333ad2cc2f3bcc95275a2ccc427f88914050e86a5fde0fd77a63c4'
assert_contains "$FRONTEND_DOCKERFILE" "golang.org/x/net@v0.55.0=golang.org/x/net@v0.56.0"
assert_contains "$FRONTEND_DOCKERFILE" 'org.opencontainers.image.base.name="caddy:2.11.4-alpine"'
assert_contains "$FRONTEND_DOCKERFILE" 'org.opencontainers.image.base.digest="sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648"'
assert_contains "$RESTIC_DOCKERFILE" '6aa3a516ce654808a1f28f9fa21e9b7c8e6e90bf'
assert_contains "$RESTIC_DOCKERFILE" "golang.org/x/net@v0.55.0=golang.org/x/net@v0.56.0"
assert_contains "$RESTIC_DOCKERFILE" 'org.opencontainers.image.base.name="restic/restic:0.19.1"'
assert_contains "$BACKEND_DOCKERFILE" 'org.opencontainers.image.base.name="alpine:3.24"'
assert_contains "$BACKEND_DOCKERFILE" 'org.opencontainers.image.base.digest="sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b"'
assert_contains "$BACKEND_DOCKERFILE" 'apk add --no-cache ffmpeg=8.1.2-r0'
assert_contains "$BACKEND_DOCKERFILE" 'USER appuser:appuser'

# Terraform plans may contain sensitive values. Re-running the target must
# repair an existing directory's permissions, and a successful apply removes
# the exact reviewed plan.
assert_contains "$ROOT/tools/make/deployment.mk" 'install -d -m 0700 infra/terraform/.tfplan'
assert_contains "$ROOT/tools/make/deployment.mk" 'chmod 0600 infra/terraform/.tfplan/geoguessme.tfplan'
assert_contains "$ROOT/tools/make/deployment.mk" 'apply .tfplan/geoguessme.tfplan'
assert_contains "$ROOT/tools/make/deployment.mk" 'rm -f infra/terraform/.tfplan/geoguessme.tfplan'

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

# The runtime hash check compares the installed root-owned host definitions
# (bin scripts, config compose files) against a root-owned manifest and must
# fail when any installed file was modified out-of-band. A deploy-writable
# release copy must not be able to change the expected baseline.
VERIFY="$ROOT/deployment/scripts/hosted/verify-deployment-hashes.sh"
hash_root=$(mktemp -d)
trap 'rm -f "$marker"; rm -rf "$test_root" "$hash_root"' EXIT INT TERM
# The environment's app revision may differ from the shared host-runtime
# revision; integrity must use the latter for both environments.
app_revision=$(printf 'a%.0s' $(seq 1 40))
runtime_revision=$(printf 'b%.0s' $(seq 1 40))
mkdir -p "$hash_root/app/releases/$runtime_revision/deployment/scripts/hosted" \
    "$hash_root/app/bin" "$hash_root/app/config" \
    "$hash_root/state/releases/dev"
printf 'REVISION=%s\n' "$app_revision" >"$hash_root/state/releases/dev/current.env"
printf '%s\n' "$runtime_revision" >"$hash_root/app/config/runtime-revision"
for script in common deploy forced-command verify-deployment-hashes backup restore-rehearsal health-check alert; do
    cp "$ROOT/deployment/scripts/hosted/$script.sh" \
        "$hash_root/app/releases/$runtime_revision/deployment/scripts/hosted/$script.sh"
    cp "$ROOT/deployment/scripts/hosted/$script.sh" "$hash_root/app/bin/$script.sh"
done
cp "$ROOT/deployment/compose.production.yaml" \
    "$hash_root/app/releases/$runtime_revision/deployment/compose.production.yaml"
cp "$ROOT/deployment/compose.production.yaml" "$hash_root/app/config/compose.production.yaml"
cp "$ROOT/deployment/compose.hosted.yaml" \
    "$hash_root/app/releases/$runtime_revision/deployment/compose.hosted.yaml"
cp "$ROOT/deployment/compose.hosted.yaml" "$hash_root/app/config/compose.hosted.yaml"
{
    for script in common deploy forced-command verify-deployment-hashes backup restore-rehearsal health-check alert; do
        sha256sum "$hash_root/app/bin/$script.sh" |
            awk -v path="bin/$script.sh" '{print $1 "  " path}'
    done
    sha256sum "$hash_root/app/config/compose.production.yaml" |
        awk '{print $1 "  config/compose.production.yaml"}'
    sha256sum "$hash_root/app/config/compose.hosted.yaml" |
        awk '{print $1 "  config/compose.hosted.yaml"}'
} >"$hash_root/app/config/runtime-hashes"
chmod 0444 "$hash_root/app/config/runtime-hashes"
run_verify() {
    GEOGUESSME_APP_ROOT="$hash_root/app" \
        GEOGUESSME_STATE_ROOT="$hash_root/state" \
        GEOGUESSME_SECRET_ROOT="$hash_root/secrets" \
        GEOGUESSME_LOCK_ROOT="$hash_root/locks" \
        "$VERIFY" dev
}
if ! run_verify >/dev/null 2>&1; then
    fail 'runtime hash check rejected matching host definitions'
fi
printf '\n# deploy-writable release copy must not alter the expected baseline\n' \
    >>"$hash_root/app/releases/$runtime_revision/deployment/compose.production.yaml"
if ! run_verify >/dev/null 2>&1; then
    fail 'runtime hash check trusted a mutable release copy as its baseline'
fi
if SSH_ORIGINAL_COMMAND='verify production' \
    GEOGUESSME_APP_ROOT="$hash_root/app" \
    GEOGUESSME_STATE_ROOT="$hash_root/state" \
    GEOGUESSME_SECRET_ROOT="$hash_root/secrets" \
    GEOGUESSME_LOCK_ROOT="$hash_root/locks" \
    "$FORCED" dev >/dev/null 2>&1; then
    fail 'dev forced command accepted a production integrity request'
fi
if ! SSH_ORIGINAL_COMMAND='verify dev' \
    GEOGUESSME_APP_ROOT="$hash_root/app" \
    GEOGUESSME_STATE_ROOT="$hash_root/state" \
    GEOGUESSME_SECRET_ROOT="$hash_root/secrets" \
    GEOGUESSME_LOCK_ROOT="$hash_root/locks" \
    "$FORCED" dev >/dev/null 2>&1; then
    fail 'dev forced command rejected its own integrity request'
fi
printf '\n# tampered verifier\n' >>"$hash_root/app/bin/verify-deployment-hashes.sh"
if run_verify >/dev/null 2>&1; then
    fail 'runtime hash check accepted a tampered installed verifier'
fi
cp "$ROOT/deployment/scripts/hosted/verify-deployment-hashes.sh" \
    "$hash_root/app/bin/verify-deployment-hashes.sh"
printf '\n# tampered out-of-band\n' >>"$hash_root/app/config/compose.production.yaml"
if run_verify >/dev/null 2>&1; then
    fail 'runtime hash check accepted a tampered host definition'
fi
cp "$ROOT/deployment/compose.production.yaml" \
    "$hash_root/app/config/compose.production.yaml"
printf '%s\n' invalid >"$hash_root/app/config/runtime-revision"
if run_verify >/dev/null 2>&1; then
    fail 'runtime hash check accepted an invalid root-owned runtime revision'
fi
if GEOGUESSME_APP_ROOT="$hash_root/app" GEOGUESSME_STATE_ROOT="$hash_root/state" \
    GEOGUESSME_SECRET_ROOT="$hash_root/secrets" GEOGUESSME_LOCK_ROOT="$hash_root/locks" \
    "$VERIFY" >/dev/null 2>&1; then
    fail 'runtime hash check accepted a missing environment'
fi

printf 'hosted deployment contracts passed\n'
