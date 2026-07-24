#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=deployment/scripts/hosted/common.sh
. "$SCRIPT_DIR/common.sh"

environment=${1:-}
backend_image=${2:-}
web_image=${3:-}
revision=${4:-}
vapid_public="${VAPID_PUBLIC_KEY:-}"
vapid_private="${VAPID_PRIVATE_KEY:-}"
vapid_subject="${VAPID_SUBJECT:-}"
validate_environment "$environment"

validate_image_reference "$backend_image" backend
validate_image_reference "$web_image" web
case "$revision" in
    *[!0-9a-f]* | '') die 'revision must be a lowercase hexadecimal Git commit' ;;
esac
[ "${#revision}" -eq 40 ] || die 'revision must contain exactly 40 hexadecimal characters'

exec 9>"$LOCK_ROOT/geoguessme-deploy.lock"
flock -n 9 || die 'another host deployment is already running'

case "$environment" in
    dev)
        identity='^https://github.com/Anko59/GeoguessMe/.github/workflows/deploy\.yml@refs/heads/dev$'
        ;;
    production)
        identity='^https://github.com/Anko59/GeoguessMe/.github/workflows/release\.yml@refs/heads/main$'
        ;;
esac

release=$(release_dir "$revision")
if [ ! -d "$release" ]; then
    archive=$(mktemp)
    staging=$(mktemp -d "$APP_ROOT/releases/.staging.XXXXXX")
    trap 'rm -f "$archive"; rm -rf "$staging"' EXIT INT TERM
    curl --fail --silent --show-error --location \
        "https://github.com/Anko59/GeoguessMe/archive/$revision.tar.gz" -o "$archive"
    tar -xzf "$archive" --strip-components=1 -C "$staging"
    mv "$staging" "$release"
    rm -f "$archive"
    trap - EXIT INT TERM
fi

encrypted="$release/deployment/secrets/$environment.env.enc"
secret_file=$(environment_env_file "$environment")
temporary_secret=''
old_secret=''
secret_replaced=false
if [ -f "$encrypted" ]; then
    temporary_secret=$(mktemp "$SECRET_ROOT/$environment.env.XXXXXX")
    trap 'rm -f "$temporary_secret"' EXIT INT TERM
    docker run --rm \
        -e "SOPS_AGE_KEY_FILE=/age/$environment.txt" \
        -v "$release:/source:ro" -v "$SECRET_ROOT/age:/age:ro" \
        "$SOPS_IMAGE" decrypt --input-type dotenv --output-type dotenv \
        "/source/deployment/secrets/$environment.env.enc" \
        >"$temporary_secret"
    chmod 600 "$temporary_secret"
fi

registry_secret=$secret_file
if [ -n "$temporary_secret" ]; then
    registry_secret=$temporary_secret
fi
[ -f "$registry_secret" ] || die "missing secret file: $registry_secret"
registry_username=$(sed -n 's/^GHCR_USERNAME=//p' "$registry_secret" | tail -1)
registry_token=$(sed -n 's/^GHCR_TOKEN=//p' "$registry_secret" | tail -1)
if [ -z "$registry_username" ] || [ -z "$registry_token" ]; then
    die 'GHCR_USERNAME and GHCR_TOKEN are required'
fi
printf '%s' "$registry_token" | docker login ghcr.io \
    --username "$registry_username" --password-stdin >/dev/null
for image in "$backend_image" "$web_image"; do
    docker run --rm -v "$HOME/.docker:/root/.docker:ro" "$COSIGN_IMAGE" verify \
        --certificate-oidc-issuer https://token.actions.githubusercontent.com \
        --certificate-identity-regexp "$identity" \
        --annotations "revision=$revision" "$image" >/dev/null
done

metadata_dir="$STATE_ROOT/releases/$environment"
mkdir -p "$metadata_dir" "$APP_ROOT/$environment"
current="$metadata_dir/current.env"
previous="$metadata_dir/previous.env"
if [ -f "$current" ]; then
    cp "$current" "$previous"
fi

if [ -d "$APP_ROOT/$environment/current" ] &&
    compose "$environment" "$APP_ROOT/$environment/current" ps --status running postgres --quiet | grep -q .; then
    "$SCRIPT_DIR/backup.sh" "$environment" pre-deploy
else
    printf 'first deployment: no database exists to back up\n'
fi

rollback() {
    status=$?
    trap - EXIT INT TERM
    if [ "$status" -ne 0 ]; then
        if [ "$secret_replaced" = true ]; then
            if [ -n "$old_secret" ] && [ -f "$old_secret" ]; then
                mv "$old_secret" "$secret_file"
            else
                rm -f "$secret_file"
            fi
        fi
        if [ -f "$previous" ]; then
            old_backend=$(sed -n 's/^BACKEND_IMAGE=//p' "$previous")
            old_web=$(sed -n 's/^WEB_IMAGE=//p' "$previous")
            old_revision=$(sed -n 's/^REVISION=//p' "$previous")
            case "$old_revision" in *[!0-9a-f]* | '') valid_revision=false ;; *) valid_revision=true ;; esac
            if valid_image_reference "$old_backend" &&
                valid_image_reference "$old_web" &&
                [ "$valid_revision" = true ] && [ "${#old_revision}" -eq 40 ]; then
                old_release=$(release_dir "$old_revision")
                BACKEND_IMAGE=$old_backend WEB_IMAGE=$old_web \
                    compose "$environment" "$old_release" up -d --wait backend web postgres || true
            fi
            printf 'deployment failed; previous images and secrets were restored; database was not restored\n' >&2
        else
            printf 'initial deployment failed; candidate secrets were removed; database was not restored\n' >&2
        fi
    fi
    [ -z "$temporary_secret" ] || rm -f "$temporary_secret"
    [ -z "$old_secret" ] || rm -f "$old_secret"
    exit "$status"
}
trap rollback EXIT INT TERM

if [ -n "$temporary_secret" ]; then
    if [ -f "$secret_file" ]; then
        old_secret=$(mktemp "$SECRET_ROOT/$environment.env.previous.XXXXXX")
        cp "$secret_file" "$old_secret"
        chmod 600 "$old_secret"
    fi
    mv "$temporary_secret" "$secret_file"
    temporary_secret=''
    secret_replaced=true
fi
require_secret_file "$environment"

export BACKEND_IMAGE="$backend_image" WEB_IMAGE="$web_image"
# Inject VAPID keys from CI environment (optional — deployment without them
# disables push notifications).
if [ -n "$vapid_public" ]; then
    env_file=$(environment_env_file "$environment")
    {
        printf 'VAPID_PUBLIC_KEY=%s\n' "$vapid_public"
        printf 'VAPID_PRIVATE_KEY=%s\n' "$vapid_private"
        printf 'VAPID_SUBJECT=%s\n' "$vapid_subject"
    } >>"$env_file"
fi
compose "$environment" "$release" pull backend web postgres
compose "$environment" "$release" run --rm migration migrate up
compose "$environment" "$release" up -d --wait postgres backend web
curl --fail --silent --show-error --max-time 10 \
    "http://127.0.0.1:$(environment_port "$environment")/health/ready" >/dev/null

umask 077
{
    printf 'BACKEND_IMAGE=%s\n' "$backend_image"
    printf 'WEB_IMAGE=%s\n' "$web_image"
    printf 'REVISION=%s\n' "$revision"
    printf 'DEPLOYED_AT=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} >"$current"
ln -sfn "$release" "$APP_ROOT/$environment/current"
trap - EXIT INT TERM
[ -z "$old_secret" ] || rm -f "$old_secret"
printf 'deployment completed: environment=%s revision=%s\n' "$environment" "$revision"
