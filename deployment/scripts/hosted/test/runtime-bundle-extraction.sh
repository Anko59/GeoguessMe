#!/bin/sh
set -eu
# Invoked only by the Dockerized hosted contract target; destinations are inside
# the disposable tools container, never host /opt or live systemd state.
[ -f /.dockerenv ] || {
    echo 'runtime extraction test requires Docker' >&2
    exit 1
}
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../.." && pwd)
INSTALLER="$ROOT/infra/cloud-init/install-runtime-bundle.sh"
fixture=$(mktemp -d)
trap 'rm -rf "${fixture:?}"' EXIT INT TERM
mkdir -p /opt/geoguessme/config
awk '
    /runtime_bundle_files = \[/ { inside = 1; next }
    inside && /^  ]/ { exit }
    inside && /file\(/ {
        sub(/^.*file\("\$\{path.module\}/, ""); sub(/"\),.*$/, ""); print
    }
' "$ROOT/infra/terraform/main.tf" >"$fixture/sources"
awk '/^\/opt\/geoguessme\/|^\/etc\/systemd\/system\/geoguessme-/ { print }' \
    "$INSTALLER" >"$fixture/destinations"
test "$(wc -l <"$fixture/sources")" -eq 32
test "$(wc -l <"$fixture/destinations")" -eq 32
printf 'GEOGUESSME_RUNTIME_BUNDLE_V1\n' >"$fixture/raw"
while IFS= read -r source; do
    wc -c <"$ROOT/infra/terraform$source" | tr -d ' '
done <"$fixture/sources" >>"$fixture/raw"
while IFS= read -r source; do
    cat "$ROOT/infra/terraform$source"
done <"$fixture/sources" >>"$fixture/raw"
cp "$fixture/raw" "$fixture/envelope"
# gzip's metadata-free representation round-trips all bytes; cloud-init does
# this decoding before running the root-owned executable envelope.
gzip -n -c "$fixture/envelope" >"$fixture/envelope.gz"
gzip -n -c "$fixture/envelope" >"$fixture/envelope-again.gz"
cmp "$fixture/envelope.gz" "$fixture/envelope-again.gz"
gzip -dc "$fixture/envelope.gz" >"$fixture/decoded"
cmp "$fixture/envelope" "$fixture/decoded"
sh "$INSTALLER" "$fixture/decoded"
# The installed verifier maps units to /etc/systemd/system. Mirror that mapping
# when validating its relative manifest with sha256sum in this container.
ln -s /etc/systemd/system /opt/geoguessme/units
verify_members() {
    exec 4<"$fixture/sources"
    while IFS=' ' read -r path mode; do
        IFS= read -r source <&4
        cmp "$ROOT/infra/terraform$source" "$path"
        test "0$(stat -c %a "$path")" = "$mode"
        test "$(stat -c %u:%g "$path")" = 0:0
    done <"$fixture/destinations"
    exec 4<&-
    test "$(wc -l </opt/geoguessme/config/runtime-hashes)" -eq 32
    test "$(stat -c %a /opt/geoguessme/config/runtime-hashes)" = 444
    (cd /opt/geoguessme && sha256sum -c config/runtime-hashes >/dev/null)
}
verify_members
# Backward compatibility: a standalone installer still accepts a raw V1 file.
sh "$INSTALLER" "$fixture/raw"
verify_members
cp /opt/geoguessme/config/runtime-hashes "$fixture/expected-manifest"
# Corrupt/missing framing and truncated payloads must not install a new manifest.
printf 'missing marker\n' >"$fixture/missing"
printf 'GEOGUESSME_RUNTIME_BUNDLE_V1\nnot-a-length\n' >"$fixture/invalid"
head -c 500 "$fixture/raw" >"$fixture/truncated"
for kind in missing invalid truncated; do
    if sh "$INSTALLER" "$fixture/$kind" >"$fixture/$kind.log" 2>&1; then
        echo "runtime extraction accepted $kind payload" >&2
        exit 1
    fi
    cmp "$fixture/expected-manifest" /opt/geoguessme/config/runtime-hashes
done
# Missing bootstrap tools fail loudly before writing any runtime state.
mkdir "$fixture/no-tools"
if PATH="$fixture/no-tools" /bin/sh "$ROOT/infra/cloud-init/bootstrap-host.sh" >"$fixture/no-tools.log" 2>&1; then
    echo 'runtime extraction accepted missing bootstrap tools' >&2
    exit 1
fi
grep -q 'host bootstrap requires gzip' "$fixture/no-tools.log"
cmp "$fixture/expected-manifest" /opt/geoguessme/config/runtime-hashes
printf 'runtime bundle extraction passed: 32 byte-identical members, modes/ownership/hashes, gzip round-trip, raw V1 compatibility and failure guards\n'
