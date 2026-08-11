#!/bin/sh
set -eu

# Authenticated host runtime-integrity check. Compares the installed, root-owned
# host runtime definitions (/opt/geoguessme/bin operator scripts and
# /opt/geoguessme/config compose files) against the same files inside the
# release directory of the root-owned runtime revision. Any mismatch means the
# running host definitions were modified out-of-band after the operator's
# reviewed cutover and the check fails with a non-zero exit code.
#
# Application revisions are environment-specific, while these host definitions
# are shared. CONFIG_ROOT/runtime-revision therefore records the one reviewed
# source revision from which the root-owned files were installed. The matching
# APP_ROOT/releases/<revision> directory is the GitHub archive the deploy flow
# installed, so its files are the expected hashes.
#
# Runs on the host through the Cloudflare Access SSH forced command:
#   verify <environment>
# It is authenticated: forced-command.sh accepts only `deploy` and `verify`
# verbs over the deployment service-token SSH path, never a shell.
#
# Usage: verify-deployment-hashes.sh <environment>   (dev or production)

# Source the INSTALLED (root-owned) common definitions so the check itself does
# not trust a deploy-writable copy. When run from a test harness,
# GEOGUESSME_APP_ROOT points at a disposable host layout.
# shellcheck source=deployment/scripts/hosted/common.sh
. "${GEOGUESSME_APP_ROOT:-/opt/geoguessme}/bin/common.sh"

environment=${1:-}
validate_environment "$environment"

metadata="$STATE_ROOT/releases/$environment/current.env"
[ -f "$metadata" ] || die "no deployment metadata for $environment: $metadata"
app_revision=$(sed -n 's/^REVISION=//p' "$metadata" | tail -1)
case "$app_revision" in
    *[!0-9a-f]* | '') die "invalid REVISION in $metadata" ;;
esac
[ "${#app_revision}" -eq 40 ] || die "REVISION in $metadata must be 40 hex characters"

runtime_revision_file="$CONFIG_ROOT/runtime-revision"
[ -f "$runtime_revision_file" ] || die "runtime revision is missing: $runtime_revision_file"
runtime_revision=$(cat "$runtime_revision_file")
case "$runtime_revision" in
    *[!0-9a-f]* | '') die "invalid runtime revision in $runtime_revision_file" ;;
esac
[ "${#runtime_revision}" -eq 40 ] || die "runtime revision must be 40 hex characters"

release=$(release_dir "$runtime_revision")
[ -d "$release" ] || die "release directory missing: $release"

mismatch=0

# compare <expected-file> <installed-file> <label>
compare() {
    expected_file=$1
    installed_file=$2
    label=$3
    [ -f "$expected_file" ] || {
        printf 'MISSING expected file in runtime revision %s: %s\n' "$runtime_revision" "$expected_file" >&2
        mismatch=1
        return
    }
    if [ ! -f "$installed_file" ]; then
        printf 'MISSING installed file: %s\n' "$installed_file" >&2
        mismatch=1
        return
    fi
    expected_hash=$(sha256sum "$expected_file" | awk '{print $1}')
    actual_hash=$(sha256sum "$installed_file" | awk '{print $1}')
    if [ "$expected_hash" = "$actual_hash" ]; then
        printf '  ok   %s\n' "$label"
    else
        printf 'MISMATCH %s\n' "$label" >&2
        printf '  installed: %s\n' "$installed_file" >&2
        printf '  expected (runtime revision %s): %s\n' "$runtime_revision" "$expected_hash" >&2
        printf '  actual:                  %s\n' "$actual_hash" >&2
        mismatch=1
    fi
}

echo "runtime hash check: environment=$environment app_revision=$app_revision runtime_revision=$runtime_revision"
echo "  comparing installed host definitions against $release"

# Operator scripts installed by provisioning (cloud-init) into /opt/geoguessme/bin.
for script in common deploy forced-command verify-deployment-hashes backup restore-rehearsal health-check alert; do
    compare \
        "$release/deployment/scripts/hosted/$script.sh" \
        "$APP_ROOT/bin/$script.sh" \
        "bin/$script.sh"
done

# Root-owned compose definitions used by the `compose` helper.
compare \
    "$release/deployment/compose.production.yaml" \
    "$CONFIG_ROOT/compose.production.yaml" \
    "config/compose.production.yaml"
compare \
    "$release/deployment/compose.hosted.yaml" \
    "$CONFIG_ROOT/compose.hosted.yaml" \
    "config/compose.hosted.yaml"

# Files without a host-installed counterpart are verified through other
# immutable channels and reported for the operator's review:
#   - deployment/caddy/Caddyfile is baked into the immutable web image whose
#     digest and cosign signature are verified during every deployment, so a
#     host-side copy does not exist to compare.
#   - infra/cloud-init/cloud-config.yaml.tftpl defines the initial
#     provisioning; it is fixed at first boot and its live state is reviewed by
#     the read-only host inspection, not per revision.
caddy_hash=$(sha256sum "$release/deployment/caddy/Caddyfile" 2>/dev/null | awk '{print $1}')
cloud_init_hash=$(sha256sum "$release/infra/cloud-init/cloud-config.yaml.tftpl" 2>/dev/null | awk '{print $1}')
caddy_hash=${caddy_hash:-unavailable}
cloud_init_hash=${cloud_init_hash:-unavailable}
printf '  info  caddy Caddyfile sha256=%s (baked into cosign-verified web image)\n' "$caddy_hash"
printf '  info  cloud-init cloud-config.yaml.tftpl sha256=%s (fixed at provisioning)\n' "$cloud_init_hash"

if [ "$mismatch" -ne 0 ]; then
    echo "runtime hash check FAILED: installed host definitions differ from runtime revision $runtime_revision" >&2
    exit 1
fi
echo "runtime hash check PASSED: installed host definitions match runtime revision $runtime_revision"
