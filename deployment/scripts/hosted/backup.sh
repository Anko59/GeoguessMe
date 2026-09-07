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
identity_dump=''
mkdir -p "$backup_dir"
umask 077

cleanup() {
    rm -f "$dump" "$identity_dump"
}
trap cleanup EXIT INT TERM

compose "$environment" "$release_link" exec -T postgres \
    pg_dump --clean --if-exists --no-owner --no-privileges \
    -U geoguessme geoguessme | gzip -9 >"$dump"
gzip -t "$dump"

# Keycloak is shared by both application environments, so production owns its
# single backup copy. This preserves broker identities, optional MFA
# credentials, and the stable subjects referenced by user_identities.
if [ "$environment" = production ] && [ -f "$(identity_env_file)" ]; then
    identity_dump="$backup_dir/keycloak-$timestamp.sql.gz"
    compose_identity "$release_link" exec -T keycloak-db \
        pg_dump --clean --if-exists --no-owner --no-privileges \
        -U keycloak keycloak | gzip -9 >"$identity_dump"
    gzip -t "$identity_dump"
fi

if ! restic "$environment" snapshots --tag "$environment" >/dev/null 2>&1; then
    restic "$environment" init
fi
if [ -n "$identity_dump" ]; then
    restic "$environment" backup "/backup/$(basename "$dump")" \
        "/backup/$(basename "$identity_dump")" \
        --tag "$environment" --tag "reason=$reason" --host geoguessme
else
    restic "$environment" backup "/backup/$(basename "$dump")" \
        --tag "$environment" --tag "reason=$reason" --host geoguessme
fi
restic "$environment" forget --tag "$environment" \
    --keep-hourly 24 --keep-daily 14 --keep-weekly 8 --keep-monthly 6 --prune

date +%s >"$STATE_ROOT/backups/$environment/last-success"
chmod 600 "$STATE_ROOT/backups/$environment/last-success"
printf 'backup completed: environment=%s timestamp=%s reason=%s\n' \
    "$environment" "$timestamp" "$reason"
