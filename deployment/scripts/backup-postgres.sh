#!/usr/bin/env sh
# Back up a PostgreSQL database to a gzipped, timestamped dump.
# Required env: DATABASE_URL or PGHOST/PGUSER/PGDATABASE/PGPASSWORD.
# Optional env: BACKUP_DIR (default ./backups).
set -eu

BACKUP_DIR="${BACKUP_DIR:-./backups}"
if [ -z "${DATABASE_URL:-}" ] && [ -z "${PGHOST:-}${PGDATABASE:-}" ]; then
    echo "error: set DATABASE_URL (or PGHOST/PGUSER/PGDATABASE) before running backup-postgres.sh" >&2
    exit 2
fi

mkdir -p "$BACKUP_DIR"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
out="$BACKUP_DIR/geoguessme-$stamp.sql.gz"
if [ -e "$out" ]; then
    echo "error: refusing to overwrite existing backup $out" >&2
    exit 1
fi

export PGPASSWORD="${PGPASSWORD:-}"
echo "writing $out"
if [ -n "${DATABASE_URL:-}" ]; then
    pg_dump --no-owner --clean --if-exists "$DATABASE_URL" | gzip >"$out"
else
    pg_dump --no-owner --clean --if-exists | gzip >"$out"
fi
echo "backup complete: $out"
