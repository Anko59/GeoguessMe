#!/bin/sh
set -eu

# Authenticated host runtime-integrity check. Compares the installed, root-owned
# host runtime definitions (/opt/geoguessme/bin operator scripts and
# /opt/geoguessme/config compose files) against the root-owned hash manifest.
# Any mismatch means the running host definitions were modified out-of-band
# after the operator's reviewed cutover and the check fails with a non-zero
# exit code.
#
# Application revisions are environment-specific, while these host definitions
# are shared. CONFIG_ROOT/runtime-revision records the one reviewed source
# revision from which the root-owned files were installed, while
# CONFIG_ROOT/runtime-hashes is the immutable expected baseline. The verifier
# never trusts the deploy-writable release archive for expected hashes.
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

manifest="$CONFIG_ROOT/runtime-hashes"
[ -f "$manifest" ] || die "runtime hash manifest is missing: $manifest"

mismatch=0

# compare <manifest-path> <installed-file> <label>
compare() {
    relative_path=$1
    installed_file=$2
    label=$3
    expected_hashes=$(awk -v expected_path="$relative_path" '$2 == expected_path { print $1 }' "$manifest")
    expected_count=$(printf '%s\n' "$expected_hashes" | awk 'NF { count++ } END { print count + 0 }')
    if [ "$expected_count" -ne 1 ]; then
        printf 'MISSING or duplicate expected path in runtime hash manifest: %s\n' "$relative_path" >&2
        mismatch=1
        return
    fi
    expected_hash=$expected_hashes
    case "$expected_hash" in
        *[!0-9a-f]* | '')
            printf 'INVALID expected hash in runtime hash manifest: %s\n' "$relative_path" >&2
            mismatch=1
            return
            ;;
    esac
    [ "${#expected_hash}" -eq 64 ] || {
        printf 'INVALID expected hash length in runtime hash manifest: %s\n' "$relative_path" >&2
        mismatch=1
        return
    }
    if [ ! -f "$installed_file" ]; then
        printf 'MISSING installed file: %s\n' "$installed_file" >&2
        mismatch=1
        return
    fi
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
echo "  comparing installed host definitions against $manifest"

# Operator scripts installed by provisioning (cloud-init) into /opt/geoguessme/bin.
for script in common deploy forced-command verify-deployment-hashes backup restore-rehearsal health-check alert; do
    compare \
        "bin/$script.sh" \
        "$APP_ROOT/bin/$script.sh" \
        "bin/$script.sh"
done

# Root-owned compose definitions used by the `compose` helper.
compare \
    "config/compose.production.yaml" \
    "$CONFIG_ROOT/compose.production.yaml" \
    "config/compose.production.yaml"
compare \
    "config/compose.hosted.yaml" \
    "$CONFIG_ROOT/compose.hosted.yaml" \
    "config/compose.hosted.yaml"

if [ "$mismatch" -ne 0 ]; then
    echo "runtime hash check FAILED: installed host definitions differ from runtime revision $runtime_revision" >&2
    exit 1
fi
echo "runtime hash check PASSED: installed host definitions match runtime revision $runtime_revision"
