#!/bin/sh
# shellcheck disable=SC2016
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../.." && pwd)
WATCH_DEPLOY="$ROOT/deployment/scripts/hosted/watch-deploy.sh"
COMMON="$ROOT/deployment/scripts/hosted/common.sh"
FORCED="$ROOT/deployment/scripts/hosted/forced-command.sh"
WATCH_COMPOSE="$ROOT/deployment/compose.watch.yaml"

fail() {
    printf 'watch deploy contract failed: %s\n' "$1" >&2
    exit 1
}

assert_contains() {
    grep -Fq -e "$2" "$1" || fail "$1 does not contain: $2"
}

line_of() {
    grep -n -m1 "$2" "$1" | cut -d: -f1
}

assert_contains "$FORCED" 'watch-deploy.sh'
assert_contains "$FORCED" 'expected: watch SOCKET_PROXY_IMAGE REVISION'
assert_contains "$WATCH_DEPLOY" 'validate_socket_proxy_image_reference "$image" "$expected_tag"'
assert_contains "$WATCH_DEPLOY" '--certificate-oidc-issuer https://token.actions.githubusercontent.com'
assert_contains "$WATCH_DEPLOY" 'docker pull "$image"'
assert_contains "$WATCH_DEPLOY" 'STATE_ROOT/watch'
assert_contains "$WATCH_DEPLOY" 'up -d --no-deps --wait --wait-timeout 90 socket-proxy'
assert_contains "$WATCH_DEPLOY" 'watch-health.sh'
assert_contains "$WATCH_DEPLOY" 'watch image update failed; restoring previous socket-proxy image'
assert_contains "$ROOT/infra/cloud-init/units/geoguessme-watch.service" 'EnvironmentFile=-/var/lib/geoguessme/watch/current.env'
assert_contains "$ROOT/infra/cloud-init/units/geoguessme-watch-health.service" 'EnvironmentFile=-/var/lib/geoguessme/watch/current.env'
if grep -Eq 'compose.*[[:space:]]down([[:space:]]|$)' "$WATCH_DEPLOY"; then
    fail 'watch image deployment must reconcile only socket-proxy without taking down the project'
fi
verify_line=$(line_of "$WATCH_DEPLOY" 'COSIGN_IMAGE.* verify')
pull_line=$(line_of "$WATCH_DEPLOY" 'docker pull "\$image"')
apply_line=$(line_of "$WATCH_DEPLOY" 'watch_compose "\$image" up')
[ "$verify_line" -lt "$pull_line" ] || fail 'socket-proxy signature verification must precede pull'
[ "$pull_line" -lt "$apply_line" ] || fail 'socket-proxy pull must precede targeted reconciliation'

TMP=$(mktemp -d /tmp/geoguessme-watch-contract.XXXXXX)
cleanup() {
    case "$TMP" in
        /tmp/geoguessme-watch-contract.*) rm -rf -- "$TMP" ;;
        *)
            printf 'refusing to remove unexpected test path: %s\n' "$TMP" >&2
            exit 1
            ;;
    esac
}
trap cleanup EXIT INT TERM
mkdir -p "$TMP/bin" "$TMP/home/.docker" "$TMP/app/bin" "$TMP/app/config/watch" \
    "$TMP/state/releases/production" "$TMP/locks"
cp "$COMMON" "$TMP/app/bin/common.sh"
cp "$WATCH_DEPLOY" "$TMP/app/bin/watch-deploy.sh"
cp "$WATCH_COMPOSE" "$TMP/app/config/compose.watch.yaml"
chmod 0755 "$TMP/app/bin/watch-deploy.sh"
revision=$(printf 'a%.0s' $(seq 1 40))
digest=$(printf 'b%.0s' $(seq 1 64))
candidate="ghcr.io/anko59/geoguessme-socket-proxy:dev-$revision@sha256:$digest"
web_revision=$(printf 'c%.0s' $(seq 1 40))
web_digest=$(printf 'd%.0s' $(seq 1 64))
printf 'WEB_IMAGE=ghcr.io/anko59/geoguessme-web:release-%s@sha256:%s\n' \
    "$web_revision" "$web_digest" >"$TMP/state/releases/production/current.env"

cat >"$TMP/bin/docker" <<'DOCKER'
#!/bin/sh
set -eu
case "$1" in
    run)
        printf 'verify:%s\n' "$*" >>"$TRACE"
        [ "${VERIFY_FAIL:-0}" -eq 0 ]
        ;;
    pull)
        printf 'pull:%s\n' "$2" >>"$TRACE"
        ;;
    compose)
        case " $* " in
            *' ps --quiet socket-proxy '* )
                printf 'ps:%s\n' "$SOCKET_PROXY_IMAGE" >>"$TRACE"
                [ "${NO_CONTAINER:-0}" -eq 1 ] || printf 'container-id\n'
                ;;
            *' up -d --no-deps --wait --wait-timeout 90 socket-proxy '* )
                printf 'up:%s\n' "$SOCKET_PROXY_IMAGE" >>"$TRACE"
                ;;
            *)
                printf 'unexpected-compose:%s\n' "$*" >&2
                exit 90
                ;;
        esac
        ;;
    inspect)
        [ "$3" = '{{.Config.Image}}' ] || exit 91
        printf '%s\n' "$RUNNING_IMAGE"
        ;;
    *)
        printf 'unexpected-docker:%s\n' "$*" >&2
        exit 92
        ;;
esac
DOCKER
cat >"$TMP/bin/flock" <<'FLOCK'
#!/bin/sh
exit 0
FLOCK
cat >"$TMP/app/bin/watch-health.sh" <<'HEALTH'
#!/bin/sh
set -eu
printf 'health:%s\n' "$SOCKET_PROXY_IMAGE" >>"$TRACE"
[ "${FAIL_HEALTH_IMAGE:-}" != "$SOCKET_PROXY_IMAGE" ]
HEALTH
chmod 0755 "$TMP/bin/docker" "$TMP/bin/flock" "$TMP/app/bin/watch-health.sh"

export PATH="$TMP/bin:$PATH"
export HOME="$TMP/home"
export TRACE="$TMP/trace"
export GEOGUESSME_APP_ROOT="$TMP/app"
export GEOGUESSME_CONFIG_ROOT="$TMP/app/config"
export GEOGUESSME_STATE_ROOT="$TMP/state"
export GEOGUESSME_LOCK_ROOT="$TMP/locks"
export GEOGUESSME_SECRET_ROOT="$TMP/secrets"
export RUNNING_IMAGE='lscr.io/linuxserver/socket-proxy:latest@sha256:7f932344a3a66a2a54a34001e8e78e60ec14dcd9c522e74a5b6420ac9db18afd'

# A live bootstrap stack is updated in place, with signature verification before
# pull, a socket-proxy-only reconcile, and a full post-update health check.
: >"$TRACE"
"$TMP/app/bin/watch-deploy.sh" dev "$candidate" "$revision" >/dev/null
state_file="$TMP/state/watch/current.env"
[ "$(cat "$state_file")" = "SOCKET_PROXY_IMAGE=$candidate" ] || fail 'successful update did not atomically record the signed image'
[ "$(stat -c '%a' "$state_file")" = 600 ] || fail 'watch state is not mode 0600'
verify_line=$(line_of "$TRACE" '^verify:')
pull_line=$(line_of "$TRACE" '^pull:')
ps_line=$(line_of "$TRACE" '^ps:')
up_line=$(line_of "$TRACE" '^up:')
health_line=$(line_of "$TRACE" '^health:')
if [ "$verify_line" -ge "$pull_line" ] || [ "$pull_line" -ge "$ps_line" ] ||
    [ "$ps_line" -ge "$up_line" ] || [ "$up_line" -ge "$health_line" ]; then
    fail 'watch update did not verify, pull, reconcile, then check health in order'
fi
grep -Fq 'refs/heads/dev' "$TRACE" || fail 'dev image verification used the wrong workflow identity'

# A mismatched tag/revision is rejected before any registry or Docker action.
: >"$TRACE"
wrong_revision=$(printf 'e%.0s' $(seq 1 40))
wrong_image="ghcr.io/anko59/geoguessme-socket-proxy:dev-$wrong_revision@sha256:$digest"
if "$TMP/app/bin/watch-deploy.sh" dev "$wrong_image" "$revision" >/dev/null 2>&1; then
    fail 'watch deployment accepted an image tag for a different revision'
fi
[ ! -s "$TRACE" ] || fail 'invalid image reference reached Docker or the registry'

# A failed signature check must stop before pulling the candidate.
: >"$TRACE"
VERIFY_FAIL=1
export VERIFY_FAIL
if "$TMP/app/bin/watch-deploy.sh" dev "$candidate" "$revision" >/dev/null 2>&1; then
    fail 'watch deployment accepted a failed signature verification'
fi
unset VERIFY_FAIL
if grep -Fq 'pull:' "$TRACE"; then
    fail 'watch deployment pulled an image before its signature was accepted'
fi

# If the stack is disabled, the verified image is staged without starting any
# monitoring services; systemd will consume the dedicated state on later start.
rm -f "$state_file"
: >"$TRACE"
NO_CONTAINER=1
export NO_CONTAINER
"$TMP/app/bin/watch-deploy.sh" dev "$candidate" "$revision" >/dev/null
unset NO_CONTAINER
[ "$(cat "$state_file")" = "SOCKET_PROXY_IMAGE=$candidate" ] || fail 'inactive watch stack did not stage its image'
if grep -Fq 'up:' "$TRACE" || grep -Fq 'health:' "$TRACE"; then
    fail 'inactive watch stack was started as a side effect of staging'
fi

# A failed health check rolls back only the proxy and records its exact image.
rm -f "$state_file"
: >"$TRACE"
FAIL_HEALTH_IMAGE="$candidate"
export FAIL_HEALTH_IMAGE
if "$TMP/app/bin/watch-deploy.sh" dev "$candidate" "$revision" >/dev/null 2>&1; then
    fail 'watch deployment hid a failed post-update health check'
fi
unset FAIL_HEALTH_IMAGE
[ "$(cat "$state_file")" = "SOCKET_PROXY_IMAGE=$RUNNING_IMAGE" ] ||
    fail 'rollback did not persist the exact previous proxy image'
[ "$(stat -c '%a' "$state_file")" = 600 ] || fail 'rollback state is not mode 0600'
grep -Fq "up:$candidate" "$TRACE" || fail 'failed update did not apply the candidate proxy'
grep -Fq "up:$RUNNING_IMAGE" "$TRACE" || fail 'failed update did not restore the previous proxy'
grep -Fq "health:$RUNNING_IMAGE" "$TRACE" || fail 'rollback did not verify prior watch health'

# The watch project is host-shared: a production release can safely replace a
# previously staged dev tag while the new candidate remains release-signed.
previous_revision=$(printf 'f%.0s' $(seq 1 40))
previous_digest=$(printf '1%.0s' $(seq 1 64))
previous_dev="ghcr.io/anko59/geoguessme-socket-proxy:dev-$previous_revision@sha256:$previous_digest"
release_revision=$(printf '2%.0s' $(seq 1 40))
release_digest=$(printf '3%.0s' $(seq 1 64))
release_image="ghcr.io/anko59/geoguessme-socket-proxy:release-$release_revision@sha256:$release_digest"
printf 'SOCKET_PROXY_IMAGE=%s\n' "$previous_dev" >"$state_file"
chmod 0600 "$state_file"
RUNNING_IMAGE=$previous_dev
export RUNNING_IMAGE
: >"$TRACE"
"$TMP/app/bin/watch-deploy.sh" production "$release_image" "$release_revision" >/dev/null
[ "$(cat "$state_file")" = "SOCKET_PROXY_IMAGE=$release_image" ] ||
    fail 'production release did not replace the shared watch image state'
grep -Fq 'refs/heads/main' "$TRACE" || fail 'production image verification used the wrong workflow identity'

printf 'watch deploy contracts passed\n'
