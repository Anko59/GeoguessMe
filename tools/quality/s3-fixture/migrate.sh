#!/usr/bin/env bash
# Explicit local-development migration only. Never deletes a volume or object,
# launches the archived source server, or starts hosted infrastructure.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
LEGACY_VOLUME=geoguessme-dev_geoguessme_dev_minio
TARGET_VOLUME=geoguessme-dev_geoguessme_dev_s3_fixture
DIRECTORY="$ROOT/.local/s3-fixture-migration"
RECEIPT="$DIRECTORY/receipt.env"
# shellcheck source=tools/quality/s3-fixture/lock.sh
. "$ROOT/tools/quality/s3-fixture/lock.sh"
STAGED=false
TEMPORARY=''
fail() {
    printf 'Local S3 migration stopped: %s\nLegacy data remains untouched; do not reset or delete its volume.\n' "$*" >&2
    exit 1
}
cleanup() {
    local status=$?
    if [[ "$STAGED" == true ]]; then
        if ! make -s --no-print-directory -C "$ROOT" dev-s3-stage-stop; then
            printf 'Cannot stop migration target; do not start another server against its volume.\n' >&2
            status=1
        fi
    fi
    if [[ -n "$TEMPORARY" ]]; then
        case "$TEMPORARY" in "$DIRECTORY"/receipt.*) rm -f -- "$TEMPORARY" ;; *) status=1 ;; esac
    fi
    if ! release_migration_lock; then
        printf 'Cannot release owned migration lock; leave it intact for review.\n' >&2
        status=1
    fi
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
[[ ! -L "$ROOT/.local" && ! -L "$DIRECTORY" && ! -L "$RECEIPT" ]] || fail 'unsafe receipt path'
[[ -z ${COMPOSE_PROJECT_NAME+x} || "$COMPOSE_PROJECT_NAME" == geoguessme-dev ]] || fail 'only the default geoguessme-dev project is supported'
acquire_migration_lock || fail 'another migration or recovery holds the local migration lock'
legacy=$(docker volume inspect --format '{{.Name}}|{{.CreatedAt}}' "$LEGACY_VOLUME") || fail 'no legacy local-development S3 volume'
[[ "$legacy" == "$LEGACY_VOLUME|"[0-9]* ]] || fail 'source volume creation identity is incomplete'
# Only an already-running legacy server can be used automatically. Offline
# recovery requires the separately reviewed snapshot/read-only recovery runbook.
ids=$(docker ps --filter label=com.docker.compose.project=geoguessme-dev \
    --filter label=com.docker.compose.service=minio --format '{{.ID}}') || fail 'cannot identify source container'
read -r -a containers <<<"${ids//$'\n'/ }"
[[ ${#containers[@]} -eq 1 ]] || fail 'start the reviewed legacy reader recovery procedure; source must already be running'
source_container=${containers[0]}
source_recovery=$(docker inspect --format '{{index .Config.Labels "geoguessme.local-s3-recovery"}}' "$source_container") || fail 'cannot inspect source recovery ownership'
source_volume=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' "$source_container") || fail 'cannot inspect source mount'
[[ "$source_volume" == "$LEGACY_VOLUME" ]] || fail 'source is not the preserved legacy MinIO volume'
source_port=$(docker inspect --format '{{range (index .NetworkSettings.Ports "9000/tcp")}}{{.HostPort}}{{"\n"}}{{end}}' "$source_container") || fail 'cannot inspect source port'
printf '%s\n' "$source_port" | grep -Fxq 9000 || fail 'legacy source must already be available on local port 9000'
printf 'The legacy source is used only through loopback. Ensure its existing listener is isolated from untrusted networks while migrating.\n' >&2
# Stop all known application writers, preserving the DB and source containers.
docker compose -p geoguessme-dev -f "$ROOT/deployment/compose.dev.yaml" --project-directory "$ROOT" \
    stop backend frontend || fail 'cannot quiesce local application writers'
# Refuse an existing new-service process against the target volume. A second
# SeaweedFS process sharing its store is unsafe, even if ports do not collide.
running_target=$(docker ps --filter "volume=$TARGET_VOLUME" --format '{{.ID}}') || fail 'cannot inspect target users'
[[ -z "$running_target" ]] || fail 'stop the new local S3 server before migrating'
if [[ -e "$RECEIPT" ]]; then
    [[ -f "$RECEIPT" ]] || fail 'receipt is not a regular file'
    backup=$(mktemp "$DIRECTORY/previous.XXXXXX")
    case "$backup" in "$DIRECTORY"/previous.*) mv -- "$RECEIPT" "$backup" ;; *) fail 'unsafe receipt backup path' ;; esac
fi
# Invalidate any prior receipt before writes. Publish a new one only after both
# the full byte/metadata verification and clean target shutdown have succeeded.
STAGED=true
make -s --no-print-directory -C "$ROOT" dev-s3-stage || fail 'cannot start isolated migration target'
target_ids=$(docker ps --filter "volume=$TARGET_VOLUME" --format '{{.ID}}') || fail 'cannot identify staged target'
read -r -a target_containers <<<"${target_ids//$'\n'/ }"
[[ ${#target_containers[@]} -eq 1 ]] || fail 'staged target must be the only process using its volume'
target_container=${target_containers[0]}
target_mount=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' "$target_container") || fail 'cannot inspect target mount'
[[ "$target_mount" == "$TARGET_VOLUME" ]] || fail 'migration server does not use the new development volume'
target_port=$(docker inspect --format '{{range (index .NetworkSettings.Ports "9000/tcp")}}{{.HostIp}}:{{.HostPort}}{{"\n"}}{{end}}' "$target_container") || fail 'cannot inspect target listener'
[[ "$target_port" == '127.0.0.1:19000' ]] || fail 'staged target must expose S3 only on loopback port 19000'
target=$(docker volume inspect --format '{{.Name}}|{{.CreatedAt}}' "$TARGET_VOLUME") || fail 'new target volume identity unavailable'
[[ "$target" == "$TARGET_VOLUME|"[0-9]* ]] || fail 'target volume creation identity is incomplete'
export S3_FIXTURE_ENDPOINT=http://127.0.0.1:19000
export SOURCE_S3_FIXTURE_ENDPOINT=http://127.0.0.1:9000
export S3_FIXTURE_ACCESS_KEY=minioadmin S3_FIXTURE_SECRET_KEY=minioadmin
export SOURCE_S3_FIXTURE_ACCESS_KEY=${SOURCE_S3_FIXTURE_ACCESS_KEY:-minioadmin}
export SOURCE_S3_FIXTURE_SECRET_KEY=${SOURCE_S3_FIXTURE_SECRET_KEY:-minioadmin}
make -s --no-print-directory -C "$ROOT" s3-fixture-host \
    S3_FIXTURE_COMMAND='copy geoguessme-media geoguessme-media' || fail 'object copy or per-object verification failed'
verification=$(make -s --no-print-directory -C "$ROOT" s3-fixture-host \
    S3_FIXTURE_COMMAND='verify geoguessme-media geoguessme-media') || fail 'complete source/target manifest verification failed'
pattern='^\{"source_bucket":"geoguessme-media","target_bucket":"geoguessme-media","objects":([0-9]+),"manifest_sha256":"([0-9a-f]{64})"\}$'
[[ "$verification" =~ $pattern ]] || fail 'invalid verification result; receipt was not issued'
objects=${BASH_REMATCH[1]}
manifest=${BASH_REMATCH[2]}
make -s --no-print-directory -C "$ROOT" dev-s3-stage-stop || fail 'migration target did not shut down cleanly'
remaining_target=$(docker ps --filter "volume=$TARGET_VOLUME" --format '{{.ID}}') || fail 'cannot verify target shutdown'
[[ -z "$remaining_target" ]] || fail 'migration target is still running; receipt was not issued'
STAGED=false
if [[ "$source_recovery" == true ]]; then
    S3_FIXTURE_LOCK_PARENT=$$ make -s --no-print-directory -C "$ROOT" dev-s3-recovery-source-stop ||
        fail 'managed recovery source did not stop; migration receipt was not issued'
    source_users=$(docker ps --filter "volume=$LEGACY_VOLUME" --format '{{.ID}}') || fail 'cannot verify managed source shutdown'
    [[ -z "$source_users" ]] || fail 'managed recovery source is still running; receipt was not issued'
fi
# A recreated volume is not the verified one, even if the name is reused.
[[ $(docker volume inspect --format '{{.Name}}|{{.CreatedAt}}' "$TARGET_VOLUME") == "$target" ]] || fail 'target volume was recreated during migration'
TEMPORARY=$(mktemp "$DIRECTORY/receipt.XXXXXX")
printf 'FORMAT=geoguessme-local-s3-migration-v1\nSOURCE_VOLUME=%s\nTARGET_VOLUME=%s\nBUCKET=geoguessme-media\nOBJECTS=%s\nMANIFEST_SHA256=%s\n' \
    "$legacy" "$target" "$objects" "$manifest" >"$TEMPORARY"
case "$TEMPORARY" in "$DIRECTORY"/receipt.*) mv -- "$TEMPORARY" "$RECEIPT" ;; *) fail 'unsafe receipt publication path' ;; esac
TEMPORARY=''
release_migration_lock || fail 'cannot release migration lock after receipt publication'
bash "$ROOT/tools/quality/s3-fixture/guard.sh" || fail 'published receipt does not match volume identities'
printf 'Local S3 migration verified (%s current objects). Legacy volume preserved; review then run make dev.\n' "$objects"
