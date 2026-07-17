#!/usr/bin/env sh
# Restore a PostgreSQL database from a gzipped dump. DESTRUCTIVE: the dump is
# written with --clean --if-exists, so existing objects are dropped.
#
# Usage: restore-postgres.sh <backup.sql.gz>
# Required env: DATABASE_URL or PGHOST/PGUSER/PGDATABASE/PGPASSWORD.
set -eu

if [ "$#" -lt 1 ]; then
    echo "usage: $0 <backup.sql.gz>" >&2
    exit 2
fi
path="$1"
if [ ! -f "$path" ]; then
    echo "error: backup file not found: $path" >&2
    exit 1
fi
if [ -z "${DATABASE_URL:-}" ] && [ -z "${PGHOST:-}${PGDATABASE:-}" ]; then
    echo "error: set DATABASE_URL (or PGHOST/PGUSER/PGDATABASE) before running restore-postgres.sh" >&2
    exit 2
fi

printf 'WARNING: restoring %s will DROP and recreate objects in the target database.\n' "$path"
printf 'Target: %s\n' "${DATABASE_URL:-${PGUSER:-}@${PGHOST:-}/${PGDATABASE:-}}"
printf 'Type the database name to confirm: '
read -r confirm
if [ "$confirm" != "${PGDATABASE:-geoguessme}" ]; then
    echo "confirmation did not match; aborting"
    exit 1
fi

export PGPASSWORD="${PGPASSWORD:-}"
echo "restoring $path"
if [ -n "${DATABASE_URL:-}" ]; then
    # PostgreSQL 18's pg_dump emits transaction_timeout, which older servers
    # do not recognize. It is a session-only setting, so remove that generated
    # line to keep dumps portable across supported PostgreSQL major versions.
    gunzip -c "$path" | sed '/^SET transaction_timeout = 0;$/d' | psql "$DATABASE_URL"
else
    gunzip -c "$path" | sed '/^SET transaction_timeout = 0;$/d' | psql
fi
echo "restore complete"
