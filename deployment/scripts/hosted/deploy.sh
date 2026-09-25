#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=deployment/scripts/hosted/common.sh
. "$SCRIPT_DIR/common.sh"

environment=${1:-}
backend_image=${2:-}
web_image=${3:-}
case "$#" in
    4)
        keycloak_image=''
        revision=$4
        ;;
    5)
        keycloak_image=$4
        revision=$5
        ;;
    *) die 'expected 4 or 5 deployment arguments' ;;
esac
validate_environment "$environment"

validate_image_reference "$backend_image" backend
validate_image_reference "$web_image" web
if [ -n "$keycloak_image" ]; then
    validate_image_reference "$keycloak_image" keycloak
fi
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
prune_releases "$APP_ROOT" "$STATE_ROOT" "$CONFIG_ROOT" "$revision" ||
    die 'cannot safely prune hosted source releases; inspect current and previous metadata'
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
identity_temporary=''
trap 'rm -f "$temporary_secret" "$identity_temporary"' EXIT INT TERM
if [ -f "$encrypted" ]; then
    temporary_secret=$(mktemp "$SECRET_ROOT/$environment.env.XXXXXX")
    docker run --rm \
        -e "SOPS_AGE_KEY_FILE=/age/$environment.txt" \
        -v "$release:/source:ro" -v "$SECRET_ROOT/age:/age:ro" \
        "$SOPS_IMAGE" decrypt --input-type dotenv --output-type dotenv \
        "/source/deployment/secrets/$environment.env.enc" \
        >"$temporary_secret"
    chmod 600 "$temporary_secret"
    if oidc_enabled "$temporary_secret"; then
        normalize_oauth2_proxy_cookie_secret "$temporary_secret"
    fi
fi

registry_secret=$secret_file
if [ -n "$temporary_secret" ]; then
    registry_secret=$temporary_secret
fi
[ -f "$registry_secret" ] || die "missing secret file: $registry_secret"
oidc_enabled=false
if oidc_enabled "$registry_secret"; then
    oidc_enabled=true
    identity_encrypted="$release/deployment/secrets/identity.env.enc"
    [ -f "$identity_encrypted" ] || die 'OIDC is enabled but deployment/secrets/identity.env.enc is missing'
    identity_temporary=$(mktemp "$SECRET_ROOT/identity.env.XXXXXX")
    docker run --rm \
        -e "SOPS_AGE_KEY_FILE=/age/$environment.txt" \
        -v "$release:/source:ro" -v "$SECRET_ROOT/age:/age:ro" \
        "$SOPS_IMAGE" decrypt --input-type dotenv --output-type dotenv \
        "/source/deployment/secrets/identity.env.enc" \
        >"$identity_temporary"
    chmod 600 "$identity_temporary"
    identity_file=$(identity_env_file)
    if [ -f "$identity_file" ] && ! cmp -s "$identity_file" "$identity_temporary"; then
        die 'shared identity secrets differ from the installed stack; follow the Keycloak rotation runbook'
    fi
    case "$environment" in
        dev) identity_client_key=GEOGUESSME_DEV_OIDC_CLIENT_SECRET ;;
        production) identity_client_key=GEOGUESSME_PRODUCTION_OIDC_CLIENT_SECRET ;;
    esac
    app_client_secret=$(sed -n 's/^OIDC_CLIENT_SECRET=//p' "$registry_secret" | tail -1)
    identity_client_secret=$(sed -n "s/^$identity_client_key=//p" "$identity_temporary" | tail -1)
    if [ -z "$app_client_secret" ] || [ "$app_client_secret" != "$identity_client_secret" ]; then
        die "$environment OAuth2 Proxy client secret does not match the shared Keycloak realm"
    fi
fi
registry_username=$(sed -n 's/^GHCR_USERNAME=//p' "$registry_secret" | tail -1)
registry_token=$(sed -n 's/^GHCR_TOKEN=//p' "$registry_secret" | tail -1)
if [ -z "$registry_username" ] || [ -z "$registry_token" ]; then
    die 'GHCR_USERNAME and GHCR_TOKEN are required'
fi
printf '%s' "$registry_token" | docker login ghcr.io \
    --username "$registry_username" --password-stdin >/dev/null
verify_image_signature() {
    image=$1
    docker run --rm -v "$HOME/.docker:/root/.docker:ro" "$COSIGN_IMAGE" verify \
        --certificate-oidc-issuer https://token.actions.githubusercontent.com \
        --certificate-identity-regexp "$identity" \
        --annotations "revision=$revision" "$image" >/dev/null
}
verify_image_signature "$backend_image"
verify_image_signature "$web_image"
if [ -n "$keycloak_image" ]; then verify_image_signature "$keycloak_image"; fi

metadata_dir="$STATE_ROOT/releases/$environment"
mkdir -p "$metadata_dir" "$APP_ROOT/$environment"
current="$metadata_dir/current.env"
previous="$metadata_dir/previous.env"
if [ -f "$current" ]; then
    cp "$current" "$previous"
fi
previous_keycloak_image=''
identity_update_started=false
if [ -f "$current" ]; then
    previous_keycloak_image=$(sed -n 's/^KEYCLOAK_IMAGE=//p' "$current" | tail -1)
fi
if [ "$oidc_enabled" = true ] && [ "$environment" = production ] &&
    [ -n "$keycloak_image" ] &&
    ! valid_image_reference "$previous_keycloak_image"; then
    active_identity_container=$(compose_identity "$release" ps -q keycloak 2>/dev/null || true)
    if [ -n "$active_identity_container" ]; then
        previous_keycloak_image=$(docker inspect --format '{{.Config.Image}}' "$active_identity_container")
        valid_image_reference "$previous_keycloak_image" ||
            die 'cannot safely roll back the shared Keycloak image; current image reference is not digest-pinned'
    elif [ -d "$APP_ROOT/production/current" ]; then
        die 'cannot safely roll back the shared Keycloak image; production metadata is missing and no current container exists'
    fi
    if [ -n "$active_identity_container" ] && [ ! -f "$previous" ]; then
        die 'cannot safely roll back the shared Keycloak image; production release metadata is missing'
    fi
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
                if [ "$environment" = production ] && [ "$oidc_enabled" = true ] &&
                    [ "$identity_update_started" = true ]; then
                    GEOGUESSME_KEYCLOAK_IMAGE=$previous_keycloak_image \
                        compose_identity "$old_release" up -d --wait keycloak-db keycloak ||
                        printf 'deployment failed; previous Keycloak image did not restart\n' >&2
                    GEOGUESSME_KEYCLOAK_IMAGE=$previous_keycloak_image \
                        compose_identity "$old_release" run --rm --no-deps keycloak-config ||
                        printf 'deployment failed; previous Keycloak realm config did not reconcile\n' >&2
                fi
                if oidc_enabled "$secret_file"; then
                    BACKEND_IMAGE=$old_backend WEB_IMAGE=$old_web \
                        compose "$environment" "$old_release" up -d --wait backend oauth2-proxy web postgres || true
                else
                    BACKEND_IMAGE=$old_backend WEB_IMAGE=$old_web \
                        compose "$environment" "$old_release" up -d --wait backend web postgres || true
                fi
            fi
            printf 'deployment failed; previous images and secrets were restored; database was not restored\n' >&2
        else
            printf 'initial deployment failed; candidate secrets were removed; database was not restored\n' >&2
        fi
    fi
    [ -z "$temporary_secret" ] || rm -f "$temporary_secret"
    [ -z "$identity_temporary" ] || rm -f "$identity_temporary"
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

if [ "$oidc_enabled" = true ]; then
    identity_file=$(identity_env_file)
    if [ ! -f "$identity_file" ]; then
        mv "$identity_temporary" "$identity_file"
        identity_temporary=''
        chmod 600 "$identity_file"
    fi
    if [ "$environment" = production ] && [ -n "$keycloak_image" ]; then
        GEOGUESSME_KEYCLOAK_IMAGE=$keycloak_image \
            compose_identity "$release" pull keycloak keycloak-db
        identity_update_started=true
        GEOGUESSME_KEYCLOAK_IMAGE=$keycloak_image \
            compose_identity "$release" up -d --wait keycloak-db keycloak
    fi
    if [ -n "$keycloak_image" ]; then
        GEOGUESSME_KEYCLOAK_IMAGE=$keycloak_image \
            compose_identity "$release" run --rm --no-deps keycloak-config
    else
        compose_identity "$release" run --rm --no-deps keycloak-config
    fi
fi

export BACKEND_IMAGE="$backend_image" WEB_IMAGE="$web_image"
if [ "$oidc_enabled" = true ]; then
    compose "$environment" "$release" pull backend web oauth2-proxy postgres
else
    compose "$environment" "$release" pull backend web postgres
fi
compose "$environment" "$release" run --rm migration migrate up
if [ "$oidc_enabled" = true ]; then
    compose "$environment" "$release" up -d --wait postgres backend oauth2-proxy web
else
    compose "$environment" "$release" up -d --wait postgres backend web
fi
curl --fail --silent --show-error --max-time 10 \
    "http://127.0.0.1:$(environment_port "$environment")/health/ready" >/dev/null

umask 077
{
    printf 'BACKEND_IMAGE=%s\n' "$backend_image"
    printf 'WEB_IMAGE=%s\n' "$web_image"
    if [ "$environment" = production ] && [ "$oidc_enabled" = true ] &&
        [ -n "$keycloak_image" ]; then
        printf 'KEYCLOAK_IMAGE=%s\n' "$keycloak_image"
    elif [ -n "$previous_keycloak_image" ]; then
        printf 'KEYCLOAK_IMAGE=%s\n' "$previous_keycloak_image"
    fi
    printf 'REVISION=%s\n' "$revision"
    printf 'DEPLOYED_AT=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} >"$current"
ln -sfn "$release" "$APP_ROOT/$environment/current"
trap - EXIT INT TERM
[ -z "$old_secret" ] || rm -f "$old_secret"
[ -z "$identity_temporary" ] || rm -f "$identity_temporary"
printf 'deployment completed: environment=%s revision=%s\n' "$environment" "$revision"
