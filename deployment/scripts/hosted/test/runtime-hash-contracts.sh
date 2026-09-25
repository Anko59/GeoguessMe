#!/bin/sh
# shellcheck disable=SC2016
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../.." && pwd)
FORCED="$ROOT/deployment/scripts/hosted/forced-command.sh"

fail() {
    printf 'runtime hash contract failed: %s\n' "$1" >&2
    exit 1
}

# The runtime hash check compares installed root-owned host definitions
# (bin scripts, config compose files, systemd units) against a root-owned manifest and must
# fail when any installed file was modified out-of-band. A deploy-writable
# release copy must not be able to change the expected baseline.
VERIFY="$ROOT/deployment/scripts/hosted/verify-deployment-hashes.sh"
hash_root=$(mktemp -d)
trap 'rm -rf "$hash_root"' EXIT INT TERM
# The environment's app revision may differ from the shared host-runtime
# revision; integrity must use the latter for both environments.
app_revision=$(printf 'a%.0s' $(seq 1 40))
runtime_revision=$(printf 'b%.0s' $(seq 1 40))
mkdir -p "$hash_root/app/releases/$runtime_revision/deployment/scripts/hosted" \
    "$hash_root/app/releases/$runtime_revision/deployment/watch" \
    "$hash_root/app/bin" "$hash_root/app/config/watch" \
    "$hash_root/systemd" \
    "$hash_root/state/releases/dev"
printf 'REVISION=%s\n' "$app_revision" >"$hash_root/state/releases/dev/current.env"
printf '%s\n' "$runtime_revision" >"$hash_root/app/config/runtime-revision"
for script in common deploy forced-command verify-deployment-hashes backup restore-rehearsal health-check alert watch-health watch-refresh-metrics-token watch-capacity; do
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
cp "$ROOT/deployment/compose.watch.yaml" \
    "$hash_root/app/releases/$runtime_revision/deployment/compose.watch.yaml"
cp "$ROOT/deployment/compose.watch.yaml" "$hash_root/app/config/compose.watch.yaml"
for config in Caddyfile vector.yaml victoria-metrics.yaml; do
    cp "$ROOT/deployment/watch/$config" \
        "$hash_root/app/releases/$runtime_revision/deployment/watch/$config"
    cp "$ROOT/deployment/watch/$config" "$hash_root/app/config/watch/$config"
done
for unit in \
    geoguessme-backup@.service geoguessme-backup@.timer \
    geoguessme-health@.service geoguessme-health@.timer \
    geoguessme-restore-rehearsal@.service geoguessme-restore-rehearsal@.timer \
    geoguessme-alert@.service geoguessme-watch.service \
    geoguessme-watch-health.service geoguessme-watch-health.timer \
    geoguessme-watch-refresh-metrics-token.service geoguessme-watch-refresh-metrics-token.timer \
    geoguessme-watch-capacity.service geoguessme-watch-capacity.timer; do
    cp "$ROOT/infra/cloud-init/units/$unit" "$hash_root/systemd/$unit"
done
{
    for script in common deploy forced-command verify-deployment-hashes backup restore-rehearsal health-check alert watch-health watch-refresh-metrics-token watch-capacity; do
        sha256sum "$hash_root/app/bin/$script.sh" |
            awk -v path="bin/$script.sh" '{print $1 "  " path}'
    done
    sha256sum "$hash_root/app/config/compose.production.yaml" |
        awk '{print $1 "  config/compose.production.yaml"}'
    sha256sum "$hash_root/app/config/compose.hosted.yaml" |
        awk '{print $1 "  config/compose.hosted.yaml"}'
    sha256sum "$hash_root/app/config/compose.watch.yaml" |
        awk '{print $1 "  config/compose.watch.yaml"}'
    for config in Caddyfile vector.yaml victoria-metrics.yaml; do
        sha256sum "$hash_root/app/config/watch/$config" |
            awk -v path="config/watch/$config" '{print $1 "  " path}'
    done
    for unit in \
        geoguessme-backup@.service geoguessme-backup@.timer \
        geoguessme-health@.service geoguessme-health@.timer \
        geoguessme-restore-rehearsal@.service geoguessme-restore-rehearsal@.timer \
        geoguessme-alert@.service geoguessme-watch.service \
        geoguessme-watch-health.service geoguessme-watch-health.timer \
        geoguessme-watch-refresh-metrics-token.service geoguessme-watch-refresh-metrics-token.timer \
        geoguessme-watch-capacity.service geoguessme-watch-capacity.timer; do
        sha256sum "$hash_root/systemd/$unit" |
            awk -v path="units/$unit" '{print $1 "  " path}'
    done
} >"$hash_root/app/config/runtime-hashes"
chmod 0444 "$hash_root/app/config/runtime-hashes"
run_verify() {
    GEOGUESSME_APP_ROOT="$hash_root/app" \
        GEOGUESSME_STATE_ROOT="$hash_root/state" \
        GEOGUESSME_SECRET_ROOT="$hash_root/secrets" \
        GEOGUESSME_LOCK_ROOT="$hash_root/locks" \
        GEOGUESSME_SYSTEMD_ROOT="$hash_root/systemd" \
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
printf '\n# out-of-band unit change\n' >>"$hash_root/systemd/geoguessme-watch-health.service"
if run_verify >/dev/null 2>&1; then
    fail 'runtime hash check accepted a changed root-owned systemd unit'
fi
cp "$ROOT/infra/cloud-init/units/geoguessme-watch-health.service" \
    "$hash_root/systemd/geoguessme-watch-health.service"
if SSH_ORIGINAL_COMMAND='verify production' \
    GEOGUESSME_APP_ROOT="$hash_root/app" \
    GEOGUESSME_STATE_ROOT="$hash_root/state" \
    GEOGUESSME_SECRET_ROOT="$hash_root/secrets" \
    GEOGUESSME_LOCK_ROOT="$hash_root/locks" \
    GEOGUESSME_SYSTEMD_ROOT="$hash_root/systemd" \
    "$FORCED" dev >/dev/null 2>&1; then
    fail 'dev forced command accepted a production integrity request'
fi
if ! SSH_ORIGINAL_COMMAND='verify dev' \
    GEOGUESSME_APP_ROOT="$hash_root/app" \
    GEOGUESSME_STATE_ROOT="$hash_root/state" \
    GEOGUESSME_SECRET_ROOT="$hash_root/secrets" \
    GEOGUESSME_LOCK_ROOT="$hash_root/locks" \
    GEOGUESSME_SYSTEMD_ROOT="$hash_root/systemd" \
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

printf 'runtime hash contracts passed\n'
