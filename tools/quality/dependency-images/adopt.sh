#!/usr/bin/env bash
# Adopt already-scanned dependency bytes for an application revision. Build
# provenance and dependency-input signatures remain attached to the same digest.
set -euo pipefail
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=tools/quality/dependency-images/common.sh
. "$SCRIPT_DIR/common.sh"
revision=${GITHUB_SHA:?GITHUB_SHA is required}
[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || fail 'invalid adoption revision'
[[ "${GITHUB_REF:-}" == refs/heads/dev ]] || fail 'dependency adoption requires the protected dev workflow'
[[ -n "${GITHUB_OUTPUT:-}" ]] || fail 'GITHUB_OUTPUT is required'
# shellcheck source=tools/quality/dependency-images/registry.sh
. "$SCRIPT_DIR/registry.sh"
start_registry_session
for component in $(component_names); do
    load_component "$component"
    source_image=${!ENV_KEY:?resolved immutable dependency reference is required}
    [[ "$source_image" == "$REMOTE_REF"@sha256:* ]] || fail 'adoption must use the reviewed content-keyed artifact'
    digest=${source_image##*@}
    valid_digest "$digest" || fail 'invalid dependency digest'
    remote_operation "adopt-verify-$component" cosign verify --certificate-oidc-issuer https://token.actions.githubusercontent.com \
        --certificate-identity-regexp '^https://github.com/Anko59/GeoguessMe/\.github/workflows/(deploy|security)\.yml@refs/heads/dev$' \
        --annotations "dependency-inputs=$INPUT_HASH" --annotations dependency-build=true "$source_image" >/dev/null
    target="ghcr.io/anko59/geoguessme-$component:dev-$revision"
    remote_operation "adopt-alias-$component" docker buildx imagetools create --tag "$target" "$source_image"
    remote_operation "adopt-inspect-$component" docker buildx imagetools inspect "$target" --format '{{json .Manifest.Digest}}'
    adopted=$(jq -er . "$TEMP/stdout")
    [[ "$adopted" == "$digest" ]] || fail 'adoption changed dependency content'
    ref="$target@$digest"
    remote_operation "adopt-sign-$component" cosign sign --yes -a "revision=$revision" -a "dependency-inputs=$INPUT_HASH" "$ref"
    printf '%s=%s\n' "$OUTPUT_KEY" "$ref" >>"$GITHUB_OUTPUT"
done
