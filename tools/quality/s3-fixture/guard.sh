#!/usr/bin/env bash
# Fail closed before dev replaces an existing legacy S3 volume with an empty one.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
LEGACY_VOLUME=geoguessme-dev_geoguessme_dev_minio
TARGET_VOLUME=geoguessme-dev_geoguessme_dev_s3_fixture
RECEIPT="$ROOT/.local/s3-fixture-migration/receipt.env"
fail() {
    printf 'Local S3 migration required: %s. Run make dev-s3-migrate; do not delete/reset the legacy volume.\n' "$*" >&2
    exit 1
}
[[ -z ${COMPOSE_PROJECT_NAME+x} || "$COMPOSE_PROJECT_NAME" == geoguessme-dev ]] || fail 'only the default geoguessme-dev project is supported'
[[ ! -e "$ROOT/.local/s3-fixture-migration/.migration-lock" && ! -L "$ROOT/.local/s3-fixture-migration/.migration-lock" ]] || fail 'migration or recovery is in progress'
volumes=$(docker volume ls --format '{{.Name}}') || fail 'cannot inspect Docker volumes'
if ! printf '%s\n' "$volumes" | grep -Fxq "$LEGACY_VOLUME"; then
    exit 0
fi
[[ ! -L "$ROOT/.local" && ! -L "$(dirname -- "$RECEIPT")" && ! -L "$RECEIPT" && -f "$RECEIPT" ]] || fail 'verified migration receipt is missing or unsafe'
legacy=$(docker volume inspect --format '{{.Name}}|{{.CreatedAt}}' "$LEGACY_VOLUME") || fail 'legacy volume identity unavailable'
target=$(docker volume inspect --format '{{.Name}}|{{.CreatedAt}}' "$TARGET_VOLUME") || fail 'new S3 volume does not exist'
[[ "$legacy" == "$LEGACY_VOLUME|"[0-9]* && "$target" == "$TARGET_VOLUME|"[0-9]* ]] || fail 'volume creation identities are incomplete'
[[ $(wc -l <"$RECEIPT") -eq 6 ]] || fail 'migration receipt is malformed'
for expected in 'FORMAT=geoguessme-local-s3-migration-v1' "SOURCE_VOLUME=$legacy" "TARGET_VOLUME=$target" 'BUCKET=geoguessme-media'; do
    [[ $(grep -Fxc "$expected" "$RECEIPT") -eq 1 ]] || fail 'receipt does not match current volume identities'
done
[[ $(grep -Ec '^OBJECTS=(0|[1-9][0-9]{0,6})$' "$RECEIPT") -eq 1 ]] || fail 'receipt lacks verified object count'
[[ $(grep -Ec '^MANIFEST_SHA256=[0-9a-f]{64}$' "$RECEIPT") -eq 1 ]] || fail 'receipt lacks verified object manifest'
