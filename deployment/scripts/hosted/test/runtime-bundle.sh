#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../.." && pwd)
TEMPLATE="$ROOT/infra/cloud-init/cloud-config.yaml.tftpl"
INSTALLER="$ROOT/infra/cloud-init/install-runtime-bundle.sh"
TERRAFORM="$ROOT/infra/terraform/main.tf"

fail() {
    printf 'runtime bundle test failed: %s\n' "$1" >&2
    exit 1
}

grep -Fq "manifest=\$(mktemp /opt/geoguessme/config/runtime-hashes.XXXXXX)" "$INSTALLER" ||
    fail 'runtime installer must create a temporary root-owned manifest'
grep -Fq "sha256sum \"\$path\"" "$INSTALLER" ||
    fail 'runtime installer must hash the installed runtime files'
grep -Fq "mv -f \"\$manifest\" /opt/geoguessme/config/runtime-hashes" "$INSTALLER" ||
    fail 'runtime installer must atomically install the root-owned runtime manifest'
grep -Eq 'runtime_revision[[:space:]]*=[[:space:]]*var.runtime_revision' "$TERRAFORM" ||
    fail 'Terraform must retain the independent runtime revision marker'
grep -Fq "for _ in \$(seq 1 33); do" "$INSTALLER" ||
    fail 'runtime installer must extract every runtime bundle member'
grep -Fq '/opt/geoguessme/bin/watch-deploy.sh 0755' "$INSTALLER" ||
    fail 'runtime installer must install the separate watch-image operator'
grep -Fq 'deployment/scripts/hosted/watch-deploy.sh' "$TERRAFORM" ||
    fail 'Terraform must bundle the separate watch-image operator'
grep -Fq "\${indent(4, chomp(runtime_installer))}" "$TEMPLATE" ||
    fail 'cloud-init must retain the complete raw runtime installer'
grep -Fq "\${indent(4, chomp(runtime_bundle))}" "$TEMPLATE" ||
    fail 'cloud-init must retain the complete framed raw runtime bundle'
grep -Fq 'runtime_installer = file(' "$TERRAFORM" ||
    fail 'Terraform must use the canonical runtime installer bytes'
grep -Fq 'base64gzip(local.runtime_cloud_archive)' "$TERRAFORM" ||
    fail 'Terraform must compress the complete standard archive in one stream'
grep -Fq '#cloud-config-archive' "$TERRAFORM" ||
    fail 'the standard archive must preserve native UTF-8 cloud-init dispatch'
grep -Fq '/etc/systemd/system/geoguessme-*' "$INSTALLER" ||
    fail 'runtime installer must constrain bundled units to the GeoGuessMe namespace'
for unit in \
    geoguessme-backup@.service geoguessme-backup@.timer \
    geoguessme-health@.service geoguessme-health@.timer \
    geoguessme-restore-rehearsal@.service geoguessme-restore-rehearsal@.timer \
    geoguessme-alert@.service geoguessme-watch.service \
    geoguessme-watch-health.service geoguessme-watch-health.timer \
    geoguessme-watch-refresh-metrics-token.service geoguessme-watch-refresh-metrics-token.timer \
    geoguessme-watch-capacity.service geoguessme-watch-capacity.timer; do
    grep -Fq "../cloud-init/units/$unit" "$TERRAFORM" ||
        fail "Terraform does not include systemd unit $unit in the bundle"
    grep -Fq "/etc/systemd/system/$unit 0644" "$INSTALLER" ||
        fail "runtime installer does not install systemd unit $unit"
done
if grep -Eq 'runtime_hashes|runtime_hash_files' "$TEMPLATE" "$TERRAFORM"; then
    fail 'the cloud-init manifest must be generated from installed files, not duplicated Terraform data'
fi

grep -Fq '/opt/geoguessme/config/s3-fixture/credentials.json 0644' "$INSTALLER" ||
    fail 'runtime installer must install the root-owned local S3 fixture configuration'
grep -Fq 'deployment/s3-fixture/credentials.json' "$TERRAFORM" ||
    fail 'Terraform must bundle the optional local S3 fixture configuration'
grep -Fq 'install -d -o root -g root -m 0755' "$INSTALLER" ||
    fail 'runtime installer must explicitly own its directories as root'
if ! grep -Fq 'length(base64encode(content)) / 4 * 3' "$TERRAFORM" ||
    ! grep -Fq 'length(regexall("=", base64encode(content)))' "$TERRAFORM"; then
    fail 'Terraform must serialize UTF-8 byte lengths, not Unicode character counts'
fi

# Exercise the actual installer in an isolated temporary root. Only the
# hard-coded installation prefixes are rewritten; no live runtime is touched.
TMP=$(mktemp -d /tmp/geoguessme-runtime-bundle.XXXXXX)
case "$TMP" in /tmp/geoguessme-runtime-bundle.??????) ;; *) fail 'unsafe fixture directory' ;; esac
trap 'rm -rf -- "${TMP:?}"' EXIT INT TERM
mkdir -p "$TMP/root/opt/geoguessme/config" "$TMP/parts"
sed -e "s|/opt/geoguessme|$TMP/root/opt/geoguessme|g" \
    -e "s|/etc/systemd/system|$TMP/root/etc/systemd/system|g" \
    "$INSTALLER" >"$TMP/install.sh"
for i in $(seq 1 32); do printf 'fixture member %s\n' "$i" >"$TMP/parts/$i"; done
cp "$ROOT/deployment/s3-fixture/credentials.json" "$TMP/parts/33"

make_bundle() {
    members=$1 output=$2 corruption=${3:-none}
    printf 'GEOGUESSME_RUNTIME_BUNDLE_V1\n' >"$output"
    for i in $(seq 1 "$members"); do
        if [ "$i" -eq 33 ] && [ "$corruption" = malformed ]; then
            printf 'invalid-length\n' >>"$output"
        elif [ "$i" -eq 33 ] && [ "$corruption" = empty ]; then
            printf '0\n' >>"$output"
        else
            printf '%s\n' "$(wc -c <"$TMP/parts/$i")" >>"$output"
        fi
    done
    for i in $(seq 1 "$members"); do
        [ "$corruption" != empty ] || [ "$i" -ne 33 ] || continue
        cat "$TMP/parts/$i" >>"$output"
    done
}
make_bundle 33 "$TMP/valid"
sh "$TMP/install.sh" "$TMP/valid" || fail 'the complete 33-member bundle failed to install'
fixture="$TMP/root/opt/geoguessme/config/s3-fixture/credentials.json"
manifest="$TMP/root/opt/geoguessme/config/runtime-hashes"
cmp "$TMP/parts/1" "$TMP/root/opt/geoguessme/bin/common.sh" ||
    fail 'the installer must consume payloads after the length header, not restart at byte zero'
cmp "$TMP/parts/32" "$TMP/root/etc/systemd/system/geoguessme-watch-capacity.timer" ||
    fail 'the 32 existing bundle indices must remain unchanged'
cmp "$TMP/parts/33" "$fixture" || fail 'fixture configuration contents changed during installation'
[ "$(stat -c '%u:%g:%a' "$fixture")" = 0:0:644 ] || fail 'fixture must be root-owned mode 644'
[ "$(stat -c '%u:%g:%a' "$(dirname "$fixture")")" = 0:0:755 ] || fail 'fixture directory must be root-owned mode 755'
[ "$(stat -c '%u:%g:%a' "$manifest")" = 0:0:444 ] || fail 'runtime manifest must remain root-owned mode 444'
[ "$(wc -l <"$manifest")" -eq 33 ] || fail 'runtime manifest must contain exactly 33 members'
expected_hash=$(sha256sum "$fixture" | cut -d' ' -f1)
[ "$(tail -1 "$manifest")" = "$expected_hash  config/s3-fixture/credentials.json" ] ||
    fail 'the appended fixture must retain its root-relative manifest name'
manifest_hash=$(sha256sum "$manifest" | cut -d' ' -f1)
cp "$TMP/parts/1" "$TMP/old-first"
cp "$TMP/parts/32" "$TMP/old-unit"
# Early candidate members intentionally differ: rejecting a bad final member
# or trailing byte must not leave an already-updated first member behind.
printf 'changed candidate first member\n' >"$TMP/parts/1"
printf 'changed candidate last unit\n' >"$TMP/parts/32"
make_bundle 33 "$TMP/candidate"

reject_bundle() {
    if sh "$TMP/install.sh" "$1" >"$TMP/rejected.log" 2>&1; then fail "$2 was accepted"; fi
    cmp "$TMP/old-first" "$TMP/root/opt/geoguessme/bin/common.sh" || fail "$2 changed the first installed member"
    cmp "$TMP/old-unit" "$TMP/root/etc/systemd/system/geoguessme-watch-capacity.timer" || fail "$2 changed the last installed unit"
    cmp "$TMP/parts/33" "$fixture" || fail "$2 changed the installed fixture"
    [ "$(sha256sum "$manifest" | cut -d' ' -f1)" = "$manifest_hash" ] ||
        fail "$2 published a new complete runtime manifest"
    while IFS=' ' read -r hash relative; do
        case "$relative" in
            units/*) installed="$TMP/root/etc/systemd/system/${relative#units/}" ;;
            bin/* | config/*) installed="$TMP/root/opt/geoguessme/$relative" ;;
            *) fail 'unexpected installed manifest member' ;;
        esac
        [ "$(sha256sum "$installed" | cut -d' ' -f1)" = "$hash" ] || fail "$2 changed an installed runtime member"
    done <"$manifest"
    # All unpublished staging and manifest allocations must be cleaned up.
    for allocation in "$TMP/root/opt/geoguessme/config"/runtime-stage.* \
        "$TMP/root/opt/geoguessme/config"/runtime-hashes.*; do
        [ ! -e "$allocation" ] || fail "$2 leaked an unpublished runtime allocation"
    done
}
make_bundle 32 "$TMP/stale"
reject_bundle "$TMP/stale" 'obsolete 32-member bundle'
make_bundle 33 "$TMP/malformed" malformed
reject_bundle "$TMP/malformed" 'malformed fixture length'
make_bundle 33 "$TMP/empty" empty
reject_bundle "$TMP/empty" 'empty required fixture'
bytes=$(wc -c <"$TMP/candidate")
dd if="$TMP/candidate" of="$TMP/truncated" bs=1 count="$((bytes - 1))" 2>/dev/null
reject_bundle "$TMP/truncated" 'truncated fixture payload'
cp "$TMP/candidate" "$TMP/appended"
printf 'unexpected trailing data' >>"$TMP/appended"
reject_bundle "$TMP/appended" 'appended bundle payload'

# Non-ASCII JSON proves member boundaries use native UTF-8 byte lengths.
printf '{"identities":[{"name":"local-é-測試","credentials":[{"accessKey":"minioadmin","secretKey":"minioadmin"}],"actions":["Admin"]}]}\n' >"$TMP/parts/33"
make_bundle 33 "$TMP/utf8"
sh "$TMP/install.sh" "$TMP/utf8" || fail 'the UTF-8 runtime bundle failed to install'
cmp "$TMP/parts/1" "$TMP/root/opt/geoguessme/bin/common.sh" || fail 'a valid complete bundle did not update the first member'
cmp "$TMP/parts/33" "$fixture" || fail 'UTF-8 configuration bytes were changed or misaligned'
expected_hash=$(sha256sum "$fixture" | cut -d' ' -f1)
[ "$(tail -1 "$manifest")" = "$expected_hash  config/s3-fixture/credentials.json" ] ||
    fail 'UTF-8 fixture bytes do not match the installed manifest'

sh "$ROOT/deployment/scripts/hosted/test/bootstrap-host.sh"

printf 'runtime bundle tests passed (33 members, UTF-8, ownership, and five whole-runtime corruption cases)\n'
