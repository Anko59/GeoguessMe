#!/usr/bin/env bash
# Explicit, temporary recovery of the original local fixture only. The archived
# reader is not an approved default service and may write internal metadata.
# A verified offline raw snapshot is mandatory before its first access to data.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
DIRECTORY="$ROOT/.local/s3-fixture-migration"
LEGACY_VOLUME=geoguessme-dev_geoguessme_dev_minio
READER_NAME=geoguessme-s3-legacy-source
READER_NETWORK=geoguessme-s3-legacy-isolated
READER_IMAGE='quay.io/thanos/minio:RELEASE.2025-09-07T16-13-09Z@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e'
# shellcheck source=tools/quality/s3-fixture/lock.sh
. "$ROOT/tools/quality/s3-fixture/lock.sh"
# shellcheck source=tools/quality/s3-fixture/snapshot.sh
. "$ROOT/tools/quality/s3-fixture/snapshot.sh"
fail() {
    printf 'Local legacy S3 recovery stopped: %s\nOriginal volume is never deleted. Review retained snapshots before retrying.\n' "$*" >&2
    exit 1
}
cleanup() {
    local status=$?
    if ! release_migration_lock; then
        printf 'Recovery lock ownership changed; leave it intact for review.\n' >&2
        status=1
    fi
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
mode=${1:-}
[[ "$mode" == start || "$mode" == stop ]] || fail 'expected start or stop'
[[ -z ${COMPOSE_PROJECT_NAME+x} || "$COMPOSE_PROJECT_NAME" == geoguessme-dev ]] || fail 'only the default geoguessme-dev project is supported'
[[ ! -L "$ROOT/.local" && ! -L "$DIRECTORY" ]] || fail 'unsafe snapshot directory'
if [[ "$mode" == stop && -n ${S3_FIXTURE_LOCK_PARENT:-} ]]; then
    parent_holds_migration_lock || fail 'caller does not hold the migration mutex'
else
    acquire_migration_lock || fail 'another recovery or migration holds the transition lock'
fi

network_exists() {
    local networks
    networks=$(docker network ls --format '{{.Name}}') || fail 'cannot inspect recovery networks'
    printf '%s\n' "$networks" | grep -Fxq "$READER_NETWORK"
}
validate_network() {
    [[ $(docker network inspect --format '{{.Internal}}|{{index .Labels "geoguessme.local-s3-recovery"}}' "$READER_NETWORK") == 'true|true' ]]
}

if [[ "$mode" == stop ]]; then
    containers=$(docker ps -a --format '{{.Names}}') || fail 'cannot inspect recovery readers'
    if printf '%s\n' "$containers" | grep -Fxq "$READER_NAME"; then
        label=$(docker inspect --format '{{index .Config.Labels "geoguessme.local-s3-recovery"}}' "$READER_NAME") || fail 'cannot inspect reader label'
        image=$(docker inspect --format '{{.Config.Image}}' "$READER_NAME") || fail 'cannot inspect reader image'
        volume=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' "$READER_NAME") || fail 'cannot inspect reader mount'
        project=$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$READER_NAME") || fail 'cannot inspect reader project'
        service=$(docker inspect --format '{{index .Config.Labels "com.docker.compose.service"}}' "$READER_NAME") || fail 'cannot inspect reader service'
        [[ "$label" == true && "$image" == "$READER_IMAGE" && "$volume" == "$LEGACY_VOLUME" &&
            "$project" == geoguessme-dev && "$service" == minio ]] || fail 'reader ownership does not match; no container was removed'
        docker stop --time 30 "$READER_NAME" || fail 'managed reader did not stop'
        docker rm "$READER_NAME" || fail 'managed reader could not be removed'
    fi
    if network_exists; then
        validate_network || fail 'refusing to remove a foreign recovery network'
        docker network rm "$READER_NETWORK" || fail 'owned recovery network is still in use'
    fi
    exit 0
fi

[[ ${CONFIRM:-} == legacy-s3-recovery ]] || fail 'set CONFIRM=legacy-s3-recovery to acknowledge the temporary archived-reader risk'
legacy=$(docker volume inspect --format '{{.Name}}|{{.CreatedAt}}' "$LEGACY_VOLUME") || fail 'exact legacy development volume is missing'
[[ "$legacy" == "$LEGACY_VOLUME|"[0-9]* ]] || fail 'legacy volume creation identity is incomplete'
users=$(docker ps --filter "volume=$LEGACY_VOLUME" --format '{{.ID}}') || fail 'cannot check source volume users'
[[ -z "$users" ]] || fail 'source volume must be offline before a raw snapshot'
ports=$(docker ps --filter publish=9000 --format '{{.ID}}') || fail 'cannot check local S3 port conflicts'
[[ -z "$ports" ]] || fail 'stop the existing container publishing local S3 port 9000 first'
containers=$(docker ps -a --format '{{.Names}}') || fail 'cannot check recovery container names'
! printf '%s\n' "$containers" | grep -Fxq "$READER_NAME" || fail 'review the existing reader; no container will be replaced'
tool_image=$(docker image inspect --format '{{.Id}}' geoguessme/go-tools:1.27.2) || fail 'run make bootstrap-integration before recovery'
[[ "$tool_image" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'snapshot tool must resolve to an immutable local image ID'
uid=${TOOLS_UID:-$(id -u)}
gid=${TOOLS_GID:-$(id -g)}
[[ "$uid" =~ ^[0-9]+$ && "$gid" =~ ^[0-9]+$ ]] || fail 'invalid snapshot owner IDs'
snapshot=$(mktemp -d "$DIRECTORY/snapshot.XXXXXX")
# Source is read-only and has no network; only this fresh snapshot directory is
# writable. Preserve file ownership/modes inside the archive, protect its bytes,
# and compute/verify SHA-256 before publishing a completion manifest.
snapshot_legacy_store "$tool_image" "type=volume,src=$LEGACY_VOLUME,dst=/source,readonly" "$snapshot" "$uid" "$gid" ||
    fail 'offline raw snapshot or checksum verification failed; reader was not launched'
[[ $(docker volume inspect --format '{{.Name}}|{{.CreatedAt}}' "$LEGACY_VOLUME") == "$legacy" ]] || fail 'legacy volume was recreated during snapshot'
[[ -f "$snapshot/legacy.tar.gz" && ! -L "$snapshot/legacy.tar.gz" &&
    -f "$snapshot/legacy.tar.gz.sha256" && ! -L "$snapshot/legacy.tar.gz.sha256" ]] || fail 'snapshot output is missing or unsafe'
printf 'FORMAT=geoguessme-legacy-s3-snapshot-v1\nSOURCE_VOLUME=%s\nTOOL_IMAGE=%s\n' "$legacy" "$tool_image" >"$snapshot/complete.env"
chmod 600 "$snapshot/complete.env"
# Recheck immediately before launching the reader; do not share a live store.
users=$(docker ps --filter "volume=$LEGACY_VOLUME" --format '{{.ID}}') || fail 'cannot recheck source users'
[[ -z "$users" ]] || fail 'source was opened by another process during snapshot'
if network_exists; then
    validate_network || fail 'recovery network is not owned and isolated'
else
    docker network create --internal --label geoguessme.local-s3-recovery=true "$READER_NETWORK" >/dev/null || fail 'cannot create isolated reader network'
fi
[[ $(docker volume inspect --format '{{.Name}}|{{.CreatedAt}}' "$LEGACY_VOLUME") == "$legacy" ]] || fail 'source volume was recreated before reader launch'
export MINIO_ROOT_USER=${SOURCE_S3_FIXTURE_ACCESS_KEY:-minioadmin}
export MINIO_ROOT_PASSWORD=${SOURCE_S3_FIXTURE_SECRET_KEY:-minioadmin}
printf 'WARNING: temporarily starting the archived local reader after verified raw backup. Only loopback S3 is published; its internal metadata may change.\n' >&2
docker run -d --name "$READER_NAME" --network "$READER_NETWORK" \
    --user 0:0 --read-only --cap-drop ALL --security-opt no-new-privileges:true --pids-limit 128 \
    --tmpfs /tmp:rw,nosuid,nodev,noexec,size=64m \
    -p 127.0.0.1:9000:9000 \
    --mount "type=volume,src=$LEGACY_VOLUME,dst=/data" \
    --label geoguessme.local-s3-recovery=true \
    --label com.docker.compose.project=geoguessme-dev --label com.docker.compose.service=minio \
    --label "geoguessme.local-s3-source-volume=$legacy" \
    -e HOME=/tmp -e MINIO_ROOT_USER -e MINIO_ROOT_PASSWORD -e MINIO_UPDATE=off \
    "$READER_IMAGE" server /data --address :9000 --console-address 127.0.0.1:9001 ||
    fail 'reader launch failed; retained offline snapshot is available for review'
printf 'Offline snapshot retained at %s. Once source S3 is ready, run make dev-s3-migrate; explicit cleanup is make dev-s3-recovery-source-stop.\n' "$snapshot"
