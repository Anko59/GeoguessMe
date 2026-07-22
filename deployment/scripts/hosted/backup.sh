#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=deployment/scripts/hosted/common.sh
. "$SCRIPT_DIR/common.sh"

environment=${1:-}
reason=${2:-scheduled}
validate_environment "$environment"
require_secret_file "$environment"

exec 8>"$LOCK_ROOT/geoguessme-backup-$environment.lock"
flock -n 8 || die "$environment backup is already running"

release_link="$APP_ROOT/$environment/current"
[ -d "$release_link" ] || die "$environment has no active release"
backup_dir="$STATE_ROOT/backups/$environment"
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
dump="$backup_dir/postgres-$timestamp.sql.gz"
mkdir -p "$backup_dir"
umask 077

cleanup() {
    rm -f "$dump"
}
trap cleanup EXIT INT TERM

compose "$environment" "$release_link" exec -T postgres \
    pg_dump --clean --if-exists --no-owner --no-privileges \
    -U geoguessme geoguessme | gzip -9 >"$dump"
gzip -t "$dump"

if ! restic "$environment" snapshots --tag "$environment" >/dev/null 2>&1; then
    restic "$environment" init
fi
restic "$environment" backup "/backup/$(basename "$dump")" \
    --tag "$environment" --tag "reason=$reason" --host geoguessme
restic "$environment" forget --tag "$environment" \
    --keep-hourly 24 --keep-daily 14 --keep-weekly 8 --keep-monthly 6 --prune

date +%s >"$STATE_ROOT/backups/$environment/last-success"
chmod 600 "$STATE_ROOT/backups/$environment/last-success"
printf 'backup completed: environment=%s timestamp=%s reason=%s\n' \
    "$environment" "$timestamp" "$reason"
