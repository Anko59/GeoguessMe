#!/usr/bin/env bash
# Promote the exact signed development socket-proxy image without rebuilding it.
set -euo pipefail

dev_sha=${1:?usage: promote-socket-proxy-image.sh DEV_SHA}
release_sha=${GITHUB_SHA:?GITHUB_SHA is required}
repository_owner=${GITHUB_REPOSITORY_OWNER:?GITHUB_REPOSITORY_OWNER is required}
output_file=${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}
repository_owner=$(printf '%s' "$repository_owner" | tr '[:upper:]' '[:lower:]')
image_repository="ghcr.io/$repository_owner/geoguessme-socket-proxy"
source_tag="${image_repository}:dev-$dev_sha"
release_tag="${image_repository}:release-$release_sha"

[[ "$dev_sha" =~ ^[0-9a-f]{40}$ ]] || {
    echo 'Development revision must be a lowercase 40-character Git SHA.' >&2
    exit 1
}
[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || {
    echo 'Release revision must be a lowercase 40-character Git SHA.' >&2
    exit 1
}
digest=$(docker buildx imagetools inspect "$source_tag" \
    --format '{{json .Manifest.Digest}}' | tr -d '"')
[[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || {
    echo 'Registry returned an invalid socket-proxy digest.' >&2
    exit 1
}
source_image="$source_tag@$digest"
cosign verify \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity-regexp '^https://github.com/Anko59/GeoguessMe/.github/workflows/deploy\.yml@refs/heads/dev$' \
    --annotations "revision=$dev_sha" "$source_image" >/dev/null

docker buildx imagetools create --tag "$release_tag" "$source_image"
promoted_digest=$(docker buildx imagetools inspect "$release_tag" \
    --format '{{json .Manifest.Digest}}' | tr -d '"')
[[ "$promoted_digest" == "$digest" ]] || {
    echo 'Promoted socket-proxy digest differs from the signed development image.' >&2
    exit 1
}
printf 'socket_proxy_image=%s\nsocket_proxy_digest=%s\nsocket_proxy_source=%s\n' \
    "$release_tag" "$digest" "$source_image" >>"$output_file"
