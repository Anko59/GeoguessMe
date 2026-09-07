#!/bin/sh
set -eu

# This file is sourced; consumers use different subsets of these constants.
# shellcheck disable=SC2034
readonly APP_ROOT="${GEOGUESSME_APP_ROOT:-/opt/geoguessme}"
# Root-owned deployment definitions are deliberately separate from downloaded
# application releases so a dev deployment cannot alter host mounts or names.
readonly CONFIG_ROOT="${GEOGUESSME_CONFIG_ROOT:-$APP_ROOT/config}"
# shellcheck disable=SC2034
readonly STATE_ROOT="${GEOGUESSME_STATE_ROOT:-/var/lib/geoguessme}"
# shellcheck disable=SC2034
readonly SECRET_ROOT="${GEOGUESSME_SECRET_ROOT:-/etc/geoguessme}"
# shellcheck disable=SC2034
readonly LOCK_ROOT="${GEOGUESSME_LOCK_ROOT:-/run/lock/geoguessme}"
# This is the exact signed Restic image published and scanned with the
# currently deployed development revision. Keep the reference immutable; the
# release workflow scans this exact value before production promotion.
readonly RESTIC_IMAGE='ghcr.io/anko59/geoguessme-restic:dev-485231d90e45dfd0e5988c93f24412d9eb2ba04e@sha256:321ffe05069a513cecb5022d29beaa82a380fc7333abb6e9412091b423980c53'
# shellcheck disable=SC2034
readonly COSIGN_IMAGE='ghcr.io/sigstore/cosign/cosign:v2.6.5@sha256:ad281047f85c5e1fc6ffbc30c2b55be3b07b4032bef715a12122ce5829619aca'
# shellcheck disable=SC2034
readonly SOPS_IMAGE='ghcr.io/getsops/sops:v3.13.3@sha256:857f5a151ac0b2bfc55c1e4e5581d66fb8e268e4d106b38e74191f3bac9d58ea'

die() {
    printf 'ERROR: %s\n' "$*" >&2
    exit 1
}

validate_environment() {
    case "${1:-}" in
        dev | production) ;;
        *) die 'environment must be dev or production' ;;
    esac
}

valid_image_reference() {
    candidate=$1
    case "$candidate" in
        *@sha256:*) candidate_digest=${candidate##*@sha256:} ;;
        *) return 1 ;;
    esac
    case "$candidate_digest" in *[!0-9a-f]* | '') return 1 ;; esac
    [ "${#candidate_digest}" -eq 64 ]
}

validate_image_reference() {
    reference=$1
    label=$2
    valid_image_reference "$reference" || die "$label image must use an exact lowercase sha256 digest"
}

environment_port() {
    case "$1" in
        production) printf '%s\n' 8081 ;;
        dev) printf '%s\n' 8082 ;;
    esac
}

environment_project() {
    printf 'geoguessme-%s\n' "$1"
}

environment_env_file() {
    printf '%s/%s.env\n' "$SECRET_ROOT" "$1"
}

oidc_enabled() {
    grep -Eq '^OIDC_ENABLED=(true|1)$' "$1"
}

release_dir() {
    printf '%s/releases/%s\n' "$APP_ROOT" "$1"
}

compose() {
    environment=$1
    release=$2
    shift 2
    env_file=$(environment_env_file "$environment")
    backend=${BACKEND_IMAGE:-}
    web=${WEB_IMAGE:-}
    metadata="$STATE_ROOT/releases/$environment/current.env"
    if [ -z "$backend" ] && [ -f "$metadata" ]; then
        backend=$(sed -n 's/^BACKEND_IMAGE=//p' "$metadata")
    fi
    if [ -z "$web" ] && [ -f "$metadata" ]; then
        web=$(sed -n 's/^WEB_IMAGE=//p' "$metadata")
    fi
    [ -n "$backend" ] || die "$environment has no active backend image metadata"
    [ -n "$web" ] || die "$environment has no active web image metadata"
    profiles=''
    if oidc_enabled "$env_file"; then
        profiles=social
    fi
    COMPOSE_PROJECT_NAME=$(environment_project "$environment") \
    COMPOSE_PROFILES="$profiles" \
    GEOGUESSME_ENV_FILE="$env_file" \
    GEOGUESSME_WEB_PORT=$(environment_port "$environment") \
    BACKEND_IMAGE="$backend" \
    WEB_IMAGE="$web" \
        docker compose \
        --project-directory "$release" \
        -f "$CONFIG_ROOT/compose.production.yaml" \
        -f "$CONFIG_ROOT/compose.hosted.yaml" "$@"
}

identity_env_file() {
    printf '%s/identity.env\n' "$SECRET_ROOT"
}

compose_identity() {
    release=$1
    shift
    env_file=$(identity_env_file)
    [ -f "$env_file" ] || die "missing identity secret file: $env_file"
    COMPOSE_PROJECT_NAME=geoguessme-identity \
        GEOGUESSME_IDENTITY_ENV_FILE="$env_file" \
        docker compose \
        --project-directory "$release" \
        -f "$release/deployment/compose.identity.yaml" "$@"
}

backup_age_seconds() {
    marker=$1
    [ -f "$marker" ] || die "missing backup marker: $marker"
    completed=$(cat "$marker")
    case "$completed" in *[!0-9]* | '') die "invalid backup marker: $marker" ;; esac
    now=${GEOGUESSME_NOW_EPOCH:-$(date +%s)}
    case "$now" in *[!0-9]* | '') die 'current epoch must be numeric' ;; esac
    [ "$now" -ge "$completed" ] || die 'backup marker is in the future'
    printf '%s\n' "$((now - completed))"
}

require_secret_file() {
    secret_file=$(environment_env_file "$1")
    [ -f "$secret_file" ] || die "missing secret file: $secret_file"
    mode=$(stat -c '%a' "$secret_file")
    [ "$mode" = 600 ] || die "$secret_file must have mode 0600 (found $mode)"
}

restic() {
    environment=$1
    shift
    secret_file=$(environment_env_file "$environment")
    backup_dir="$STATE_ROOT/backups/$environment"
    mkdir -p "$backup_dir"
    docker run --rm --network host \
        --env-file "$secret_file" \
        -v "$backup_dir:/backup:ro" \
        "$RESTIC_IMAGE" /usr/bin/restic "$@"
}
