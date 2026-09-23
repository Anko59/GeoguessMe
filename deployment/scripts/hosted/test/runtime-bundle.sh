#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../.." && pwd)
TEMPLATE="$ROOT/infra/cloud-init/cloud-config.yaml.tftpl"
TERRAFORM="$ROOT/infra/terraform/main.tf"

fail() {
    printf 'runtime bundle test failed: %s\n' "$1" >&2
    exit 1
}

grep -Fq "manifest=\$(mktemp /opt/geoguessme/config/runtime-hashes.XXXXXX)" "$TEMPLATE" ||
    fail 'cloud-init must create a temporary root-owned manifest'
grep -Fq "sha256sum \"\$path\"" "$TEMPLATE" ||
    fail 'cloud-init must hash the installed runtime files'
grep -Fq "mv -f \"\$manifest\" /opt/geoguessme/config/runtime-hashes" "$TEMPLATE" ||
    fail 'cloud-init must atomically install the root-owned runtime manifest'
grep -Fq 'runtime_revision = var.runtime_revision' "$TERRAFORM" ||
    fail 'Terraform must retain the independent runtime revision marker'
if grep -Eq 'runtime_hashes|runtime_hash_files' "$TEMPLATE" "$TERRAFORM"; then
    fail 'the cloud-init manifest must be generated from installed files, not duplicated Terraform data'
fi

printf 'runtime bundle tests passed\n'
