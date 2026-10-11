#!/usr/bin/env bash
# Mock local orchestration only; never accesses a real Docker daemon or data.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
TEMP=$(mktemp -d /tmp/geoguessme-s3-migration.XXXXXX)
cleanup() { case "$TEMP" in /tmp/geoguessme-s3-migration.*) rm -rf -- "$TEMP" ;; *) exit 1 ;; esac }
trap cleanup EXIT
fail() {
    printf 'S3 migration contract failed: %s\n' "$*" >&2
    exit 1
}
mkdir -p "$TEMP/tools/quality/s3-fixture" "$TEMP/bin"
cp "$ROOT/tools/quality/s3-fixture/guard.sh" "$TEMP/tools/quality/s3-fixture/guard.sh"
cp "$ROOT/tools/quality/s3-fixture/migrate.sh" "$TEMP/tools/quality/s3-fixture/migrate.sh"
cp "$ROOT/tools/quality/s3-fixture/lock.sh" "$TEMP/tools/quality/s3-fixture/lock.sh"
export FIXTURE_STATE="$TEMP/state" FIXTURE_TRACE="$TEMP/trace"
mkdir -p "$FIXTURE_STATE"
cat >"$TEMP/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker:%s\n' "$*" >>"$FIXTURE_TRACE"
case "$1" in
    volume)
        if [[ "$2" == ls ]]; then
            [[ ! -f "$FIXTURE_STATE/legacy" ]] || printf 'geoguessme-dev_geoguessme_dev_minio\n'
            [[ ! -f "$FIXTURE_STATE/target" ]] || printf 'geoguessme-dev_geoguessme_dev_s3_fixture\n'
        else
            volume=${*: -1}
            if [[ "$volume" == *dev_minio ]]; then
                [[ -f "$FIXTURE_STATE/legacy" ]] || exit 1
                printf '%s|2026-01-01T00:00:00Z\n' "$volume"
            else
                [[ -f "$FIXTURE_STATE/target" ]] || exit 1
                printf '%s|%s\n' "$volume" "${TARGET_CREATED:-2026-02-01T00:00:00Z}"
            fi
        fi
        ;;
    ps)
        case "$*" in
            *'volume=geoguessme-dev_geoguessme_dev_minio'*) [[ -f "$FIXTURE_STATE/no-source" ]] || printf 'source-container\n' ;;
            *'volume='*) [[ ! -f "$FIXTURE_STATE/running-target" ]] || printf 'target-container\n' ;;
            *) [[ ! -f "$FIXTURE_STATE/no-source" ]] && printf 'source-container\n' || true ;;
        esac
        ;;
    inspect)
        case "$*" in
            *geoguessme.local-s3-recovery*) if [[ "${RECOVERY_SOURCE:-0}" == 1 ]]; then printf 'true\n'; else printf '<no value>\n'; fi ;;
            *Mounts*)
                if [[ "$*" == *target-container* ]]; then
                    printf '%s\n' "${TARGET_MOUNT:-geoguessme-dev_geoguessme_dev_s3_fixture}"
                else
                    printf '%s\n' "${SOURCE_VOLUME:-geoguessme-dev_geoguessme_dev_minio}"
                fi
                ;;
            *Ports*)
                if [[ "$*" == *target-container* ]]; then printf '%s\n' "${TARGET_PORT:-127.0.0.1:19000}"; else printf '9000\n'; fi
                ;;
            *) exit 1 ;;
        esac
        ;;
    compose)
        [[ "$*" == *'stop backend frontend'* ]] || exit 1
        : >"$FIXTURE_STATE/writers-stopped"
        ;;
    *) exit 1 ;;
esac
DOCKER
cat >"$TEMP/bin/make" <<'MAKE'
#!/usr/bin/env bash
set -euo pipefail
printf 'make:%s\n' "$*" >>"$FIXTURE_TRACE"
case "$*" in
    *dev-s3-recovery-source-stop*)
        [[ "${FAIL_RECOVERY_STOP:-0}" == 0 ]] || exit 1
        [[ -n "${S3_FIXTURE_LOCK_PARENT:-}" && $(<"$(dirname "$FIXTURE_STATE")/.local/s3-fixture-migration/.migration-lock/owner") == "$S3_FIXTURE_LOCK_PARENT" ]] || exit 1
        [[ ! -f "$FIXTURE_STATE/running-target" && ! -f "$(dirname "$FIXTURE_STATE")/.local/s3-fixture-migration/receipt.env" ]] || exit 1
        [[ "${RECOVERY_STOP_NOOP:-0}" == 0 ]] || exit 0
        : >"$FIXTURE_STATE/no-source"
        ;;
    *dev-s3-stage-stop*)
        [[ "${FAIL_STOP:-0}" == 0 ]] || exit 1
        [[ "${STOP_NOOP:-0}" == 0 ]] || exit 0
        rm -f "$FIXTURE_STATE/running-target"
        ;;
    *dev-s3-stage*)
        [[ -f "$FIXTURE_STATE/writers-stopped" ]] || exit 1
        : >"$FIXTURE_STATE/target"
        : >"$FIXTURE_STATE/running-target"
        ;;
    *'copy geoguessme-media geoguessme-media'*)
        [[ "$SOURCE_S3_FIXTURE_ENDPOINT" == http://127.0.0.1:9000 && "$S3_FIXTURE_ENDPOINT" == http://127.0.0.1:19000 ]] || exit 1
        [[ "$SOURCE_S3_FIXTURE_ACCESS_KEY" == "${EXPECTED_SOURCE_KEY:-minioadmin}" && "$SOURCE_S3_FIXTURE_SECRET_KEY" == "${EXPECTED_SOURCE_SECRET:-minioadmin}" ]] || exit 1
        [[ -f "$FIXTURE_STATE/writers-stopped" && -f "$FIXTURE_STATE/running-target" ]] || exit 1
        [[ "${FAIL_COPY:-0}" == 0 ]] || exit 1
        ;;
    *'verify geoguessme-media geoguessme-media'*)
        [[ "${FAIL_VERIFY:-0}" == 0 ]] || exit 1
        printf '{"source_bucket":"geoguessme-media","target_bucket":"geoguessme-media","objects":1,"manifest_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}\n'
        ;;
    *) exit 1 ;;
esac
MAKE
chmod 755 "$TEMP/bin/docker" "$TEMP/bin/make"
export PATH="$TEMP/bin:$PATH"
guard="$TEMP/tools/quality/s3-fixture/guard.sh"
migrate="$TEMP/tools/quality/s3-fixture/migrate.sh"
receipt="$TEMP/.local/s3-fixture-migration/receipt.env"
bash "$guard" || fail 'fresh development was blocked'
: >"$FIXTURE_TRACE"
if COMPOSE_PROJECT_NAME=another-project bash "$guard" >"$TEMP/result" 2>&1; then fail 'custom project escaped the fixed-volume guard'; fi
[[ ! -s "$FIXTURE_TRACE" ]] || fail 'custom project reached Docker before rejection'
lock="$TEMP/.local/s3-fixture-migration/.migration-lock"
mkdir -p "$lock"
printf 'other-writer\n' >"$lock/owner"
if bash "$migrate" >"$TEMP/result" 2>&1; then fail 'concurrent migration lock was ignored'; fi
[[ -f "$lock/owner" && $(<"$lock/owner") == other-writer && ! -s "$FIXTURE_TRACE" ]] || fail 'foreign lock removed or concurrent invocation reached Docker'
if bash "$guard" >"$TEMP/result" 2>&1; then fail 'dev guard allowed an active transition'; fi
rm -f "$lock/owner"
rmdir "$lock"
: >"$FIXTURE_STATE/legacy"
if bash "$guard" >"$TEMP/result" 2>&1; then fail 'existing legacy volume accepted without proof'; fi
SOURCE_VOLUME=another-volume bash "$migrate" >"$TEMP/result" 2>&1 && fail 'wrong source mount admitted'
[[ ! -f "$FIXTURE_STATE/target" ]] || fail 'wrong source initialized a target'
: >"$FIXTURE_STATE/no-source"
if bash "$migrate" >"$TEMP/result" 2>&1; then fail 'offline source was silently assumed empty'; fi
rm -f "$FIXTURE_STATE/no-source"
if FAIL_COPY=1 bash "$migrate" >"$TEMP/result" 2>&1; then fail 'copy failure accepted'; fi
[[ ! -f "$receipt" && ! -f "$FIXTURE_STATE/running-target" ]] || fail 'copy failure published proof or left target running'
if FAIL_VERIFY=1 bash "$migrate" >"$TEMP/result" 2>&1; then fail 'verification failure accepted'; fi
[[ ! -f "$receipt" && ! -f "$FIXTURE_STATE/running-target" ]] || fail 'verification failure published proof or left target running'
if FAIL_STOP=1 bash "$migrate" >"$TEMP/result" 2>&1; then fail 'unclean shutdown accepted'; fi
[[ ! -f "$receipt" ]] || fail 'unclean shutdown published receipt'
rm -f "$FIXTURE_STATE/running-target"
if STOP_NOOP=1 bash "$migrate" >"$TEMP/result" 2>&1; then fail 'unobserved target shutdown accepted'; fi
[[ ! -f "$receipt" ]] || fail 'running target received completion proof'
rm -f "$FIXTURE_STATE/running-target"
if TARGET_MOUNT=another-volume bash "$migrate" >"$TEMP/result" 2>&1; then fail 'proof bound an unused target volume'; fi
[[ ! -f "$receipt" ]] || fail 'wrong target mount issued proof'
if TARGET_PORT=0.0.0.0:19000 bash "$migrate" >"$TEMP/result" 2>&1; then fail 'public migration listener accepted'; fi
[[ ! -f "$receipt" ]] || fail 'public migration listener issued proof'
bash "$migrate" >"$TEMP/result" 2>&1 || {
    sed -n '1,100p' "$TEMP/result" >&2
    fail 'verified local copy failed'
}
bash "$guard" || fail 'verified receipt rejected'
[[ -f "$FIXTURE_STATE/legacy" && ! -f "$FIXTURE_STATE/running-target" ]] || fail 'migration removed source or left target running'
if TARGET_CREATED=2026-03-01T00:00:00Z bash "$guard" >"$TEMP/result" 2>&1; then fail 'recreated target accepted stale receipt'; fi
# A failed re-verification invalidates an old proof before attempting new writes.
if FAIL_VERIFY=1 bash "$migrate" >"$TEMP/result" 2>&1; then fail 're-verification failure accepted'; fi
[[ ! -f "$receipt" ]] || fail 'failed re-migration retained stale proof'
if bash "$guard" >"$TEMP/result" 2>&1; then fail 'guard accepted failed re-migration'; fi
SOURCE_S3_FIXTURE_ACCESS_KEY=mock-source-key SOURCE_S3_FIXTURE_SECRET_KEY=mock-source-secret \
    EXPECTED_SOURCE_KEY=mock-source-key EXPECTED_SOURCE_SECRET=mock-source-secret \
    SOURCE_S3_FIXTURE_ENDPOINT=https://not-a-local-source.invalid S3_FIXTURE_ENDPOINT=https://not-a-local-target.invalid \
    bash "$migrate" >"$TEMP/result" 2>&1 || fail 'explicit source credentials lost or loopback endpoints not enforced'
if RECOVERY_SOURCE=1 FAIL_RECOVERY_STOP=1 bash "$migrate" >"$TEMP/result" 2>&1; then fail 'managed source stop failure accepted'; fi
[[ ! -f "$receipt" && ! -d "$lock" ]] || fail 'source stop failure issued proof or retained owned lock'
if RECOVERY_SOURCE=1 RECOVERY_STOP_NOOP=1 bash "$migrate" >"$TEMP/result" 2>&1; then fail 'unobserved source shutdown accepted'; fi
[[ ! -f "$receipt" ]] || fail 'running managed source received completion proof'
RECOVERY_SOURCE=1 bash "$migrate" >"$TEMP/result" 2>&1 || {
    sed -n '1,100p' "$TEMP/result" >&2
    fail 'managed recovery completion failed'
}
[[ -f "$receipt" && -f "$FIXTURE_STATE/no-source" && ! -d "$lock" ]] || fail 'managed source was not stopped before receipt publication'
if grep -Eq 'down .*--volumes|down .*-v|volume (rm|prune)' "$FIXTURE_TRACE"; then fail 'migration issued a volume-deletion command'; fi
printf 'Local S3 preservation, quiesce, verification and receipt contracts passed\n'
