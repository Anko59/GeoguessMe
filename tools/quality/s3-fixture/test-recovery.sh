#!/usr/bin/env bash
# Isolated command/state mocks: never starts archived software or accesses data.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
TEMP=$(mktemp -d /tmp/geoguessme-s3-recovery.XXXXXX)
cleanup() { case "$TEMP" in /tmp/geoguessme-s3-recovery.*) rm -rf -- "$TEMP" ;; *) exit 1 ;; esac }
trap cleanup EXIT
fail() {
    printf 'S3 recovery contract failed: %s\n' "$*" >&2
    exit 1
}
mkdir -p "$TEMP/tools/quality/s3-fixture" "$TEMP/bin" "$TEMP/state"
cp "$ROOT/tools/quality/s3-fixture/recovery.sh" "$TEMP/tools/quality/s3-fixture/recovery.sh"
cp "$ROOT/tools/quality/s3-fixture/lock.sh" "$TEMP/tools/quality/s3-fixture/lock.sh"
cp "$ROOT/tools/quality/s3-fixture/snapshot.sh" "$TEMP/tools/quality/s3-fixture/snapshot.sh"
export RECOVERY_STATE="$TEMP/state" RECOVERY_TRACE="$TEMP/trace"
printf 'original-volume-data\n' >"$RECOVERY_STATE/original"
export RECOVERY_IMAGE='quay.io/thanos/minio:RELEASE.2025-09-07T16-13-09Z@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e'
cat >"$TEMP/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker:%s\n' "$*" >>"$RECOVERY_TRACE"
case "$1" in
    volume)
        [[ "${MISSING_VOLUME:-0}" == 0 ]] || exit 1
        [[ "${*: -1}" == geoguessme-dev_geoguessme_dev_minio ]] || exit 1
        printf 'geoguessme-dev_geoguessme_dev_minio|%s\n' "${LEGACY_CREATED:-2026-01-01T00:00:00Z}"
        ;;
    ps)
        case "$*" in
            *'volume='*) [[ "${BUSY_VOLUME:-0}" == 1 || -f "$RECOVERY_STATE/reader" ]] && printf 'busy-source\n' || true ;;
            *publish=9000*) [[ "${BUSY_PORT:-0}" == 1 || -f "$RECOVERY_STATE/reader" ]] && printf 'busy-port\n' || true ;;
            *) [[ -f "$RECOVERY_STATE/reader" || "${FOREIGN_NAME:-0}" == 1 ]] && printf 'geoguessme-s3-legacy-source\n' || true ;;
        esac
        ;;
    image) printf 'sha256:%064d\n' 1 ;;
    network)
        case "$2" in
            ls) [[ ! -f "$RECOVERY_STATE/network" ]] || printf 'geoguessme-s3-legacy-isolated\n' ;;
            inspect) [[ "${FOREIGN_NETWORK:-0}" == 0 ]] && printf 'true|true\n' || printf 'false|false\n' ;;
            create) [[ "$*" == *'--internal --label geoguessme.local-s3-recovery=true'* ]] || exit 1; : >"$RECOVERY_STATE/network" ;;
            rm) [[ ! -f "$RECOVERY_STATE/reader" ]] || exit 1; rm -f "$RECOVERY_STATE/network" ;;
            *) exit 1 ;;
        esac
        ;;
    inspect)
        case "$*" in
            *geoguessme.local-s3-recovery*) [[ "${FOREIGN_LABEL:-0}" == 0 ]] && printf 'true\n' || printf 'false\n' ;;
            *Config.Image*) printf '%s\n' "${READER_IMAGE_OVERRIDE:-$RECOVERY_IMAGE}" ;;
            *Mounts*) printf '%s\n' "${READER_VOLUME_OVERRIDE:-geoguessme-dev_geoguessme_dev_minio}" ;;
            *com.docker.compose.project*) printf 'geoguessme-dev\n' ;;
            *com.docker.compose.service*) printf 'minio\n' ;;
            *) exit 1 ;;
        esac
        ;;
    run)
        if [[ "$*" == *'--network none'* ]]; then
            [[ "$*" == *'--cap-add DAC_OVERRIDE --cap-add CHOWN'* && "$*" != *DAC_READ_SEARCH* ]] || exit 1
            [[ "$*" == *'type=volume,src=geoguessme-dev_geoguessme_dev_minio,dst=/source,readonly'* ]] || exit 1
            [[ "$*" == *'tar -czpf /snapshot/legacy.tar.gz -C /source .'* && "$*" == *'sha256sum -c legacy.tar.gz.sha256'* && "$*" == *'chmod 0600'* && "$*" == *chown* ]] || exit 1
            [[ "${FAIL_SNAPSHOT:-0}" == 0 ]] || exit 1
            snapshot=''
            for argument; do
                case "$argument" in type=bind,src=*,dst=/snapshot) snapshot=${argument#type=bind,src=}; snapshot=${snapshot%,dst=/snapshot} ;; esac
            done
            [[ "$snapshot" == /tmp/geoguessme-s3-recovery.*/.local/s3-fixture-migration/snapshot.* ]] || exit 1
            printf 'raw snapshot bytes\n' >"$snapshot/legacy.tar.gz"
            (cd "$snapshot"; sha256sum legacy.tar.gz >legacy.tar.gz.sha256; sha256sum -c legacy.tar.gz.sha256 >/dev/null)
            chmod 600 "$snapshot/legacy.tar.gz" "$snapshot/legacy.tar.gz.sha256"
            : >"$RECOVERY_STATE/snapshot"
        else
            [[ -f "$RECOVERY_STATE/snapshot" ]] || exit 1
            [[ "$MINIO_ROOT_USER" == "${EXPECTED_SOURCE_KEY:-minioadmin}" && "$MINIO_ROOT_PASSWORD" == "${EXPECTED_SOURCE_SECRET:-minioadmin}" ]] || exit 1
            [[ "$*" == *'-e MINIO_ROOT_USER -e MINIO_ROOT_PASSWORD'* && "$*" != *'mock-source-secret'* ]] || exit 1
            completed=0
            for marker in "$(dirname "$RECOVERY_STATE")"/.local/s3-fixture-migration/snapshot.*/complete.env; do
                [[ ! -f "$marker" ]] || completed=$((completed+1))
            done
            [[ "$completed" -gt 0 && "$*" == *"$RECOVERY_IMAGE"* && "$*" == *'-p 127.0.0.1:9000:9000'* ]] || exit 1
            [[ "$*" == *'--read-only --cap-drop ALL --security-opt no-new-privileges:true --pids-limit 128'* && "$*" != *'--cap-add'* ]] || exit 1
            [[ "$*" == *'--console-address 127.0.0.1:9001'* && "$*" != *'-p 9001'* && "$*" != *'-p 0.0.0.0'* ]] || exit 1
            [[ "$*" == *'type=volume,src=geoguessme-dev_geoguessme_dev_minio,dst=/data'* ]] || exit 1
            : >"$RECOVERY_STATE/reader"
            printf 'managed-reader-id\n'
        fi
        ;;
    stop) [[ "${FAIL_STOP:-0}" == 0 ]] || exit 1; : >"$RECOVERY_STATE/stopped" ;;
    rm) [[ -f "$RECOVERY_STATE/stopped" ]] || exit 1; rm -f "$RECOVERY_STATE/reader" ;;
    *) exit 1 ;;
esac
DOCKER
chmod 755 "$TEMP/bin/docker"
export PATH="$TEMP/bin:$PATH"
recovery="$TEMP/tools/quality/s3-fixture/recovery.sh"
lock="$TEMP/.local/s3-fixture-migration/.migration-lock"
if bash "$recovery" start >"$TEMP/result" 2>&1; then fail 'unconfirmed archived reader accepted'; fi
[[ ! -f "$RECOVERY_TRACE" && ! -d "$lock" ]] || fail 'unconfirmed recovery reached Docker or retained a lock'
mkdir -p "$lock"
printf 'other-writer\n' >"$lock/owner"
if CONFIRM=legacy-s3-recovery bash "$recovery" start >"$TEMP/result" 2>&1; then fail 'foreign transition lock ignored'; fi
[[ $(<"$lock/owner") == other-writer && ! -f "$RECOVERY_TRACE" ]] || fail 'foreign transition lock was removed or recovery reached Docker'
rm -f "$lock/owner"
rmdir "$lock"
for rejection in 'MISSING_VOLUME=1' 'LEGACY_CREATED=invalid' 'BUSY_VOLUME=1' 'BUSY_PORT=1' 'FOREIGN_NAME=1'; do
    if env "$rejection" CONFIRM=legacy-s3-recovery bash "$recovery" start >"$TEMP/result" 2>&1; then fail "invalid recovery admitted: $rejection"; fi
    [[ ! -f "$RECOVERY_STATE/snapshot" && ! -f "$RECOVERY_STATE/reader" && ! -d "$lock" ]] || fail 'invalid source created snapshot/reader or retained lock'
done
if FAIL_SNAPSHOT=1 CONFIRM=legacy-s3-recovery bash "$recovery" start >"$TEMP/result" 2>&1; then fail 'failed snapshot accepted'; fi
[[ ! -f "$RECOVERY_STATE/reader" && ! -d "$lock" ]] || fail 'reader launched after failed snapshot or retained lock'
SOURCE_S3_FIXTURE_ACCESS_KEY=mock-source-key SOURCE_S3_FIXTURE_SECRET_KEY=mock-source-secret \
    EXPECTED_SOURCE_KEY=mock-source-key EXPECTED_SOURCE_SECRET=mock-source-secret \
    CONFIRM=legacy-s3-recovery bash "$recovery" start >"$TEMP/result" 2>&1 || {
    sed -n '1,120p' "$TEMP/result" >&2
    fail 'confirmed snapshot/reader start failed'
}
[[ -f "$RECOVERY_STATE/reader" && ! -d "$lock" ]] || fail 'reader missing or owned lock retained'
[[ $(<"$RECOVERY_STATE/original") == original-volume-data ]] || fail 'original source data was mutated by orchestration'
for rejection in 'FOREIGN_LABEL=1' 'READER_IMAGE_OVERRIDE=foreign:image' 'READER_VOLUME_OVERRIDE=foreign-volume'; do
    if env "$rejection" bash "$recovery" stop >"$TEMP/result" 2>&1; then fail "foreign reader ownership admitted: $rejection"; fi
    [[ -f "$RECOVERY_STATE/reader" && ! -d "$lock" ]] || fail 'foreign reader was removed or own lock retained'
done
if FAIL_STOP=1 bash "$recovery" stop >"$TEMP/result" 2>&1; then fail 'reader stop failure accepted'; fi
[[ -f "$RECOVERY_STATE/reader" && ! -d "$lock" ]] || fail 'failed stop removed reader or retained own lock'
bash "$recovery" stop >"$TEMP/result" 2>&1 || fail 'managed reader could not be stopped'
[[ ! -f "$RECOVERY_STATE/reader" && ! -f "$RECOVERY_STATE/network" && ! -d "$lock" ]] || fail 'managed resources or owned lock retained after stop'
if grep -Eq 'volume (rm|prune)|rm .*--volumes|rm .*-v' "$RECOVERY_TRACE"; then fail 'recovery issued a volume deletion'; fi
printf 'Offline snapshot, explicit archived-reader recovery and owned cleanup contracts passed\n'
