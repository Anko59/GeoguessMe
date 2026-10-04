#!/usr/bin/env bash
# Promote an adopted dependency digest, retaining the original build provenance.
set -euo pipefail
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=tools/quality/dependency-images/common.sh
. "$SCRIPT_DIR/common.sh"
component=${1:?usage: promote.sh COMPONENT DEV_SHA}
dev_sha=${2:?usage: promote.sh COMPONENT DEV_SHA}
release_sha=${GITHUB_SHA:?GITHUB_SHA is required}
[[ "$dev_sha" =~ ^[0-9a-f]{40}$ && "$release_sha" =~ ^[0-9a-f]{40}$ ]] || fail 'invalid promotion revision'
load_component "$component"
# shellcheck source=tools/quality/dependency-images/registry.sh
. "$SCRIPT_DIR/registry.sh"
start_registry_session
repository="ghcr.io/anko59/geoguessme-$component"
source_tag="$repository:dev-$dev_sha"
target="$repository:release-$release_sha"
remote_operation "promote-source-$component" docker buildx imagetools inspect "$source_tag" --format '{{json .Manifest.Digest}}'
digest=$(jq -er . "$TEMP/stdout")
valid_digest "$digest" || fail 'invalid development dependency digest'
source_image="$source_tag@$digest"
remote_operation "promote-verify-$component" cosign verify --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity-regexp '^https://github.com/Anko59/GeoguessMe/\.github/workflows/deploy\.yml@refs/heads/dev$' \
    --annotations "revision=$dev_sha" --annotations "dependency-inputs=$INPUT_HASH" "$source_image" >/dev/null
# Verify the independent original dependency signature, not just adoption.
remote_operation "promote-verify-$component" cosign verify --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity-regexp '^https://github.com/Anko59/GeoguessMe/\.github/workflows/(deploy|security)\.yml@refs/heads/dev$' \
    --annotations "dependency-inputs=$INPUT_HASH" --annotations dependency-build=true "$source_image" >/dev/null
remote_operation "promote-alias-$component" docker buildx imagetools create --tag "$target" "$source_image"
remote_operation "promote-inspect-$component" docker buildx imagetools inspect "$target" --format '{{json .Manifest.Digest}}'
promoted=$(jq -er . "$TEMP/stdout")
[[ "$promoted" == "$digest" ]] || fail 'promotion changed dependency digest'
printf '%s=%s\n' "$OUTPUT_KEY" "$target@$digest" >>"${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}"
