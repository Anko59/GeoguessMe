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
grep -Fq "for _ in \$(seq 1 32); do" "$INSTALLER" ||
    fail 'runtime installer must extract every runtime bundle member'
grep -Fq '/opt/geoguessme/bin/watch-deploy.sh 0755' "$INSTALLER" ||
    fail 'runtime installer must install the separate watch-image operator'
grep -Fq 'deployment/scripts/hosted/watch-deploy.sh' "$TERRAFORM" ||
    fail 'Terraform must bundle the separate watch-image operator'
grep -Fq "content: \${runtime_installer}" "$TEMPLATE" ||
    fail 'cloud-init must install the compressed runtime installer'
grep -Fq 'encoding: gzip+base64' "$TEMPLATE" ||
    fail 'cloud-init must decompress the runtime installer'
grep -Fq 'runtime_installer = base64gzip(file(' "$TERRAFORM" ||
    fail 'Terraform must compress the runtime installer into the user-data payload'
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

grep -Eq 'host_bootstrap[[:space:]]*=[[:space:]]*base64gzip\(file\(' "$TERRAFORM" ||
    fail 'Terraform must compress the complete bootstrap without dropping commands'
grep -Fq "content: \${host_bootstrap}" "$TEMPLATE" ||
    fail 'cloud-init must decode the compressed bootstrap'
grep -Fq '[/usr/local/sbin/geoguessme-bootstrap-host]' "$TEMPLATE" ||
    fail 'cloud-init must run the root-owned decoded bootstrap'
sh "$ROOT/deployment/scripts/hosted/test/runtime-bundle-extraction.sh"
sh "$ROOT/deployment/scripts/hosted/test/bootstrap-host.sh"
printf 'runtime bundle tests passed\n'
