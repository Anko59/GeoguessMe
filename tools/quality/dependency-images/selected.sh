#!/usr/bin/env bash
# Select already-prepared bytes, not a dependency tag another checkout can retag.
# Containers consume image IDs. Local Dockerfile FROM uses an ID-addressed alias;
# CI registry-digest inputs are preserved verbatim in all modes.
set -euo pipefail
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=tools/quality/dependency-images/common.sh
. "$SCRIPT_DIR/common.sh"
[[ $# -ge 1 && $# -le 2 ]] || fail 'usage: selected.sh COMPONENT [build|audit]'
load_component "$1"
mode=${2:-container}
case "$mode" in container | build | audit) ;; *) fail 'unknown selection mode' ;; esac
selection=${!ENV_KEY:-}
if [[ -n "$selection" && "$selection" != "$LOCAL_REF" ]]; then
    # Explicit immutable CI/Make inputs always outrank unrelated local state.
    valid_saved_reference "$COMPONENT" "$selection" || fail 'consumer requires an immutable reviewed reference'
else
    environment="$DEPENDENCY_ROOT/.local/security-images.env"
    [[ ! -L "$DEPENDENCY_ROOT/.local" && -f "$environment" && ! -L "$environment" ]] ||
        fail "no prepared selection for $COMPONENT; run explicit dependency preparation"
    selection=''
    declare -A seen=()
    while IFS='=' read -r key value || [[ -n "$key" || -n "$value" ]]; do
        [[ -n "$key" && -n "$value" && "$value" != *[[:space:]]* ]] || fail 'invalid saved dependency environment'
        case "$key" in KEYCLOAK_IMAGE | RESTIC_IMAGE | SOPS_IMAGE | SOCKET_PROXY_IMAGE | POSTGRES_IMAGE | CLOUDFLARED_IMAGE | CADDY_RUNTIME_IMAGE) ;; *) fail 'unknown saved dependency key' ;; esac
        [[ -z "${seen[$key]:-}" ]] || fail 'duplicate saved dependency key'
        seen[$key]=1
        saved_component=$(printf '%s' "${key%_IMAGE}" | tr '[:upper:]_' '[:lower:]-')
        valid_saved_reference "$saved_component" "$value" || fail 'invalid saved dependency reference'
        if [[ "$key" == "$ENV_KEY" ]]; then selection=$value; fi
    done <"$environment"
    [[ -n "$selection" ]] || fail "component is not prepared: $COMPONENT"
fi
# Audits construct a required immutable reference set without pre-pulling CI
# outputs. Their protected producer already verifies trust; the scanner owns
# authenticated pulling and byte/platform/provenance validation. This mode must
# never be used to launch containers or construct a local Dockerfile FROM.
if [[ "$mode" == audit && "$selection" == *@sha256:* ]]; then
    if [[ "$selection" == "ghcr.io/anko59/geoguessme-$COMPONENT:dependency-"* ]]; then
        [[ "$selection" == "$REMOTE_REF"@sha256:* ]] || fail 'audit input key is stale'
    fi
    printf '%s\n' "$selection"
    exit 0
fi
# The current key, runtime platform and pinned upstream must match the saved
# bytes. A changed dependency input or removed image fails, never falls back to
# a tag and never builds/pulls/signs as a side effect of consumption.
verify_local "$selection"
selected_id=$IMAGE_ID
if valid_digest "$selection"; then
    [[ "$selection" == "$selected_id" ]] || fail 'saved image ID resolved to different bytes'
    if [[ "$mode" == build ]]; then
        alias="geoguessme/$COMPONENT:config-${selected_id#sha256:}"
        if inspection=$(docker image inspect "$alias" 2>&1); then
            alias_id=$(jq -er '.[0].Id' <<<"$inspection") || fail 'invalid build alias metadata'
            [[ "$alias_id" == "$selected_id" ]] || fail 'conflicting ID-addressed build alias; refusing to retag'
        else
            printf '%s\n' "$inspection" | grep -Eiq 'No such image|No such object' || fail 'build alias lookup failed; refusing to tag'
            docker image tag "$selected_id" "$alias" >&2 || fail 'cannot create ID-addressed build alias'
        fi
        verify_local "$alias"
        [[ "$IMAGE_ID" == "$selected_id" ]] || fail 'build alias changed selected bytes'
        selection=$alias
    fi
fi
printf '%s\n' "$selection"
