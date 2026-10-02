#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=deployment/scripts/hosted/common.sh
. "$SCRIPT_DIR/common.sh"

environment=${1:-}
image=${2:-}
revision=${3:-}
[ "$#" -eq 3 ] || die 'expected environment IMAGE REVISION'
validate_environment "$environment"
valid_release_revision "$revision" || die 'revision must be a lowercase 40-character Git commit'

case "$environment" in
    dev)
        expected_tag="dev-$revision"
        identity='^https://github.com/Anko59/GeoguessMe/.github/workflows/deploy\.yml@refs/heads/dev$'
        ;;
    production)
        expected_tag="release-$revision"
        identity='^https://github.com/Anko59/GeoguessMe/.github/workflows/release\.yml@refs/heads/main$'
        ;;
esac
validate_socket_proxy_image_reference "$image" "$expected_tag"

exec 9>"$LOCK_ROOT/geoguessme-deploy.lock"
flock -n 9 || die 'another host deployment is already running'

# Verify provenance before pulling or changing the independent watch project.
docker run --rm -v "$HOME/.docker:/root/.docker:ro" "$COSIGN_IMAGE" verify \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity-regexp "$identity" \
    --annotations "revision=$revision" "$image" >/dev/null
docker pull "$image"

watch_state_dir="$STATE_ROOT/watch"
watch_state_file="$watch_state_dir/current.env"
if [ -e "$watch_state_dir" ] || [ -L "$watch_state_dir" ]; then
    if [ ! -d "$watch_state_dir" ] || [ -L "$watch_state_dir" ]; then
        die 'watch state directory must be a real directory'
    fi
else
    mkdir -p "$watch_state_dir"
fi

state_present=false
previous_image=''
if [ -e "$watch_state_file" ] || [ -L "$watch_state_file" ]; then
    if [ ! -f "$watch_state_file" ] || [ -L "$watch_state_file" ]; then
        die 'watch state must be a regular non-symlink file'
    fi
    state_mode=$(stat -c '%a' "$watch_state_file")
    [ "$state_mode" = 600 ] || die 'watch state must have mode 0600'
    state_count=$(grep -c '^SOCKET_PROXY_IMAGE=' "$watch_state_file" || true)
    extra_lines=$(grep -vc '^SOCKET_PROXY_IMAGE=' "$watch_state_file" || true)
    if [ "$state_count" -ne 1 ] || [ "$extra_lines" -ne 0 ]; then
        die 'watch state must contain exactly one SOCKET_PROXY_IMAGE entry'
    fi
    previous_image=$(sed -n 's/^SOCKET_PROXY_IMAGE=//p' "$watch_state_file")
    validate_socket_proxy_state_image "$previous_image"
    state_present=true
fi

watch_gateway=$(watch_gateway_image)
watch_compose() {
    selected_image=$1
    shift
    COMPOSE_PROJECT_NAME=geoguessme-watch \
        WEB_IMAGE="$watch_gateway" \
        SOCKET_PROXY_IMAGE="$selected_image" \
        GEOGUESSME_WATCH_METRICS_DIR=${GEOGUESSME_WATCH_METRICS_DIR:-/etc/geoguessme/watch-metrics} \
        GEOGUESSME_WATCH_AGENT_ENV=${GEOGUESSME_WATCH_AGENT_ENV:-/etc/geoguessme/watch-agent.env} \
        docker compose --project-directory "$CONFIG_ROOT" \
        -f "$CONFIG_ROOT/compose.watch.yaml" "$@"
}

container_id=$(watch_compose "$image" ps --quiet socket-proxy)
if [ -n "$container_id" ]; then
    running_image=$(docker inspect --format '{{.Config.Image}}' "$container_id")
    if [ "$state_present" = true ]; then
        [ "$running_image" = "$previous_image" ] ||
            die 'watch state does not match the running socket-proxy image; inspect before retrying'
    else
        validate_socket_proxy_bootstrap_image "$running_image" ||
            die 'watch has no state file and is not running a recognized bootstrap image'
        previous_image=$running_image
    fi
else
    # A disabled watch stack is not started as a side effect of an image update.
    # Persist the verified digest so its next operator-approved start uses it.
    temporary=$(mktemp "$watch_state_file.XXXXXX")
    trap 'rm -f "$temporary"' EXIT INT TERM
    umask 077
    printf 'SOCKET_PROXY_IMAGE=%s\n' "$image" >"$temporary"
    chmod 0600 "$temporary"
    mv -f "$temporary" "$watch_state_file"
    trap - EXIT INT TERM
    printf 'watch image staged: environment=%s image=%s (watch service is not running)\n' "$environment" "$image"
    exit 0
fi

write_watch_state() {
    state_image=$1
    temporary=$(mktemp "$watch_state_file.XXXXXX") || return 1
    if ! printf 'SOCKET_PROXY_IMAGE=%s\n' "$state_image" >"$temporary"; then
        rm -f "$temporary"
        return 1
    fi
    if ! chmod 0600 "$temporary" || ! mv -f "$temporary" "$watch_state_file"; then
        rm -f "$temporary"
        return 1
    fi
}

rollback_watch() {
    status=$?
    trap - EXIT INT TERM
    if [ "$status" -ne 0 ]; then
        printf 'watch image update failed; restoring previous socket-proxy image %s\n' "$previous_image" >&2
        write_watch_state "$previous_image" ||
            printf 'watch rollback could not restore previous state file\n' >&2
        if ! watch_compose "$previous_image" up -d --no-deps --wait --wait-timeout 90 socket-proxy; then
            printf 'watch rollback could not restore the previous socket-proxy service\n' >&2
        elif ! SOCKET_PROXY_IMAGE="$previous_image" "$APP_ROOT/bin/watch-health.sh"; then
            printf 'watch rollback restored the proxy but the complete watch health check still fails\n' >&2
        fi
    fi
    exit "$status"
}
trap rollback_watch EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

write_watch_state "$image"
watch_compose "$image" up -d --no-deps --wait --wait-timeout 90 socket-proxy
SOCKET_PROXY_IMAGE="$image" "$APP_ROOT/bin/watch-health.sh"
trap - EXIT INT TERM
printf 'watch image deployed: environment=%s image=%s revision=%s\n' "$environment" "$image" "$revision"
