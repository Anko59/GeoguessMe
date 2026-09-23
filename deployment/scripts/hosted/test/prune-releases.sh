#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../.." && pwd)
COMMON="$ROOT/deployment/scripts/hosted/common.sh"
DEPLOY="$ROOT/deployment/scripts/hosted/deploy.sh"
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT INT TERM

fail() {
    printf 'release cleanup test failed: %s\n' "$1" >&2
    exit 1
}

assert_exists() {
    [ -e "$1" ] || fail "expected to retain $1"
}

assert_absent() {
    [ ! -e "$1" ] || fail "expected to remove $1"
}

revision() {
    printf '%040d' "$1"
}

prune_line=$(grep -n 'prune_releases.*APP_ROOT.*STATE_ROOT.*CONFIG_ROOT.*revision' "$DEPLOY" | cut -d: -f1)
staging_line=$(grep -n "archive=\$(mktemp)" "$DEPLOY" | cut -d: -f1)
if [ -z "$prune_line" ] || [ -z "$staging_line" ] || [ "$prune_line" -ge "$staging_line" ]; then
    fail 'stale releases must be pruned before creating the next staging archive'
fi

app_root="$test_root/app"
state_root="$test_root/state"
config_root="$app_root/config"
releases_root="$app_root/releases"
incoming=$(revision 1)
active_dev=$(revision 2)
previous_dev=$(revision 3)
active_production=$(revision 4)
previous_production=$(revision 5)
runtime_revision=$(revision 6)
stale=$(revision 7)
mkdir -p \
    "$app_root/dev" "$app_root/production" "$config_root" \
    "$state_root/releases/dev" "$state_root/releases/production" \
    "$releases_root/$incoming" "$releases_root/$active_dev" \
    "$releases_root/$previous_dev" "$releases_root/$active_production" \
    "$releases_root/$previous_production" "$releases_root/$runtime_revision" \
    "$releases_root/$stale" "$releases_root/.staging.abandoned"
ln -s "$releases_root/$active_dev" "$app_root/dev/current"
ln -s "$releases_root/$active_production" "$app_root/production/current"
printf 'BACKEND_IMAGE=dev\nWEB_IMAGE=dev\nREVISION=%s\n' "$active_dev" >"$state_root/releases/dev/current.env"
printf 'BACKEND_IMAGE=dev-old\nWEB_IMAGE=dev-old\nREVISION=%s\n' "$previous_dev" >"$state_root/releases/dev/previous.env"
printf 'BACKEND_IMAGE=prod\nWEB_IMAGE=prod\nREVISION=%s\n' "$active_production" >"$state_root/releases/production/current.env"
printf 'BACKEND_IMAGE=prod-old\nWEB_IMAGE=prod-old\nREVISION=%s\n' "$previous_production" >"$state_root/releases/production/previous.env"
printf '%s\n' "$runtime_revision" >"$config_root/runtime-revision"

. "$COMMON"
prune_releases "$app_root" "$state_root" "$config_root" "$incoming"
for retained in \
    "$incoming" "$active_dev" "$previous_dev" "$active_production" \
    "$previous_production" "$runtime_revision"; do
    assert_exists "$releases_root/$retained"
done
assert_absent "$releases_root/$stale"
assert_absent "$releases_root/.staging.abandoned"

second_stale=$(revision 8)
mkdir -p "$releases_root/$second_stale"
printf 'incomplete metadata from a failed deployment\n' >"$state_root/releases/dev/current.env"
prune_releases "$app_root" "$state_root" "$config_root" "$incoming"
assert_absent "$releases_root/$second_stale"

third_stale=$(revision 9)
mkdir -p "$releases_root/$third_stale"
printf 'BACKEND_IMAGE=prod-old\nWEB_IMAGE=prod-old\nREVISION=bad\n' \
    >"$state_root/releases/production/previous.env"
if prune_releases "$app_root" "$state_root" "$config_root" "$incoming"; then
    fail 'malformed rollback metadata must fail closed'
fi
assert_exists "$releases_root/$third_stale"

printf 'release cleanup tests passed\n'
