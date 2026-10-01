#!/usr/bin/env bash
# Retag the exact verified development application manifests and fail on drift.
set -euo pipefail

promote_image() {
    local name=$1 source=$2 release=$3 expected_digest=$4 actual_digest
    local source_repository release_repository
    [[ "$expected_digest" =~ ^sha256:[0-9a-f]{64}$ && "$source" == *"@$expected_digest" ]] || {
        echo "Invalid or inconsistent $name source digest." >&2
        exit 1
    }
    source_repository=${source%@*}
    source_repository=${source_repository%:*}
    release_repository=${release%:*}
    [[ "$source_repository" == "$release_repository" ]] || {
        echo "$name release tag must stay in the source image repository." >&2
        exit 1
    }
    docker buildx imagetools create --tag "$release" "$source"
    actual_digest=$(docker buildx imagetools inspect "$release" \
        --format '{{json .Manifest.Digest}}' | tr -d '"')
    [[ "$actual_digest" == "$expected_digest" ]] || {
        echo "Promoted $name digest differs from the verified development image." >&2
        exit 1
    }
}

promote_image backend "${BACKEND_SOURCE:?}" "${BACKEND_RELEASE:?}" "${BACKEND_DIGEST:?}"
promote_image web "${WEB_SOURCE:?}" "${WEB_RELEASE:?}" "${WEB_DIGEST:?}"
promote_image keycloak "${KEYCLOAK_SOURCE:?}" "${KEYCLOAK_RELEASE:?}" "${KEYCLOAK_DIGEST:?}"
