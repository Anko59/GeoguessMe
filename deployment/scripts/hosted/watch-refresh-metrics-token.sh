#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=deployment/scripts/hosted/common.sh
. "$SCRIPT_DIR/common.sh"

production_secret=$(environment_env_file production)
require_secret_file production
[ -f "$production_secret" ] || die "missing production secret file: $production_secret"

token=$(sed -n 's/^METRICS_TOKEN=//p' "$production_secret" | tail -1)
printf '%s' "$token" | grep -Eq '^[[:graph:]]{64,}$' ||
    die 'production METRICS_TOKEN must contain at least 64 printable characters'

metrics_dir=${GEOGUESSME_WATCH_METRICS_DIR:-/etc/geoguessme/watch-metrics}
install -d -o root -g docker -m 0750 "$metrics_dir"
temporary=$(mktemp "$metrics_dir/.production-metrics-token.XXXXXX")
trap 'rm -f "$temporary"' EXIT INT TERM
printf '%s\n' "$token" >"$temporary"
chown root:docker "$temporary"
chmod 0640 "$temporary"
mv -f "$temporary" "$metrics_dir/production-metrics-token"
trap - EXIT INT TERM
printf 'production metrics token refreshed\n'
