#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=deployment/scripts/hosted/common.sh
. "$SCRIPT_DIR/common.sh"

environment=${1:-production}
validate_environment "$environment"
require_secret_file "$environment"
name="geoguessme-restore-${environment}-$$"
dump=$(mktemp)

cleanup() {
    docker rm -f "$name" >/dev/null 2>&1 || true
    rm -f "$dump"
}
trap cleanup EXIT INT TERM

latest=$(restic "$environment" snapshots --tag "$environment" --latest 1 --json |
    sed -n 's/.*"short_id":"\([^"]*\)".*/\1/p')
[ -n "$latest" ] || die "no $environment backup snapshot exists"
path=$(restic "$environment" ls "$latest" --json |
    sed -n 's/.*"path":"\/backup\/\([^"]*\.sql\.gz\)".*/\1/p' | tail -1)
[ -n "$path" ] || die 'snapshot contains no PostgreSQL dump'
restic "$environment" dump "$latest" "/backup/$path" >"$dump"
gzip -t "$dump"

docker run -d --name "$name" \
    -e POSTGRES_USER=geoguessme -e POSTGRES_PASSWORD=rehearsal \
    -e POSTGRES_DB=geoguessme \
    postgres:15-alpine@sha256:3d0f7584ed7d04e27fa050d6683a74746608faf21f202be78460d679cc56461f >/dev/null
attempt=0
until docker exec "$name" pg_isready -U geoguessme -d geoguessme >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 60 ] || die 'temporary restore database did not become ready'
    sleep 1
done
gzip -dc "$dump" | docker exec -i "$name" psql -v ON_ERROR_STOP=1 -U geoguessme geoguessme >/dev/null
docker exec "$name" psql -v ON_ERROR_STOP=1 -U geoguessme geoguessme \
    -c 'SELECT 1' >/dev/null
printf 'restore rehearsal passed: environment=%s snapshot=%s\n' "$environment" "$latest"
