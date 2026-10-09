#!/usr/bin/env bash
# Native Docker/filesystem regression; only unique disposable dummy directories
# are mounted. Never touches a legacy volume or starts the archived reader.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
# shellcheck source=tools/quality/s3-fixture/snapshot.sh
. "$ROOT/tools/quality/s3-fixture/snapshot.sh"
fail() {
    printf 'S3 snapshot permission regression failed: %s\n' "$*" >&2
    exit 1
}
uid=${TOOLS_UID:-$(id -u)}
gid=${TOOLS_GID:-$(id -g)}
[[ "$uid" =~ ^[1-9][0-9]*$ && "$gid" =~ ^[0-9]+$ && $(id -u) == "$uid" && $(id -g) == "$gid" ]] ||
    fail 'run this native gate as the non-root operator matching TOOLS_UID and TOOLS_GID'
umask 077
TEMP=$(mktemp -d /tmp/geoguessme-s3-snapshot.XXXXXX)
cleanup() {
    local status=$?
    case "$TEMP" in /tmp/geoguessme-s3-snapshot.*) rm -rf -- "$TEMP" ;; *) status=1 ;; esac
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir -p "$TEMP/source/private" "$TEMP/output"
printf 'fixture-snapshot-bytes\n' >"$TEMP/source/private/contents"
chmod 700 "$TEMP/source" "$TEMP/source/private" "$TEMP/output"
chmod 600 "$TEMP/source/private/contents"
[[ $(stat -c '%u:%g:%a' "$TEMP/output") == "$uid:$gid:700" ]] || fail 'output is not genuinely operator-owned 0700'
tool_image=$(docker image inspect --format '{{.Id}}' geoguessme/go-tools:1.26.9) || fail 'build the pinned Go tools image first'
[[ "$tool_image" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'snapshot runner did not resolve to an immutable local image ID'
source_mount="type=bind,src=$TEMP/source,dst=/source,readonly"
# Exact shared production command, not a mocked or permissive duplicate.
snapshot_legacy_store "$tool_image" "$source_mount" "$TEMP/output" "$uid" "$gid" || fail 'production snapshot command could not write the private operator directory'
for file in legacy.tar.gz legacy.tar.gz.sha256; do
    [[ $(stat -c '%u:%g:%a' "$TEMP/output/$file") == "$uid:$gid:600" ]] || fail 'archive output owner or mode differs from its contract'
done
[[ $(stat -c '%u:%g:%a' "$TEMP/source/private/contents") == "$uid:$gid:600" ]] || fail 'source permissions changed'
# Same constrained production container launcher. DAC_OVERRIDE must not bypass
# the read-only source/root mounts; the private output is the sole writable bind.
# This literal program expands variables only inside the constrained container.
# shellcheck disable=SC2016
snapshot_container "$tool_image" "$source_mount" "$TEMP/output" sh -ec '
    grep -Eq "^CapEff:[[:space:]]+0*3$" /proc/self/status
    grep -Eq "^NoNewPrivs:[[:space:]]+1$" /proc/self/status
    test "$(wc -l </proc/net/route)" -eq 1
    if (printf mutation >/source/private/contents) 2>/dev/null; then exit 1; fi
    if touch /snapshot-rootfs-write-probe 2>/dev/null; then exit 1; fi
    test "$(cat /source/private/contents)" = fixture-snapshot-bytes
    cd /snapshot
    sha256sum -c legacy.tar.gz.sha256
    test "$(tar -xOzf legacy.tar.gz ./private/contents)" = fixture-snapshot-bytes
' || fail 'checksum, archived bytes, capabilities, isolation or source/root read-only protection failed'
printf 'Native snapshot writes operator 0700; outputs owned 0600; source/root remain read-only with no network\n'
