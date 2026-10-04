#!/usr/bin/env bash
# Explicit lifecycle boundary: preparation builds; resolution never builds.
set -euo pipefail
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=tools/quality/dependency-images/common.sh
. "$SCRIPT_DIR/common.sh"
[[ $# -ge 1 ]] || fail 'usage: lifecycle.sh prepare-local|publish|resolve [COMPONENT ...]'
MODE=$1
shift
case "$MODE" in prepare-local | publish | resolve) ;; *) fail 'unknown dependency lifecycle mode' ;; esac
if [[ $# == 0 ]]; then mapfile -t COMPONENTS < <(component_names); else COMPONENTS=("$@"); fi
[[ ${#COMPONENTS[@]} -gt 0 ]] || fail 'empty dependency inventory'
[[ ! -L "$DEPENDENCY_ROOT/.local" ]] || fail 'local state directory must not be a symlink'
mkdir -p "$DEPENDENCY_ROOT/.local"
TEMP=$(mktemp -d "$DEPENDENCY_ROOT/.local/.dependency-run.XXXXXX")
ENV_FILE="$DEPENDENCY_ROOT/.local/security-images.env"
ENV_LOCK=''
[[ ! -L "$ENV_FILE" ]] || fail 'dependency environment must not be a symlink'
cleanup() {
    # mktemp created this exact directory beneath the known state root.
    [[ "$TEMP" == "$DEPENDENCY_ROOT/.local/.dependency-run."* && -d "$TEMP" && ! -L "$TEMP" ]] || return
    rm -rf -- "$TEMP"
    if [[ "$ENV_LOCK" == "$DEPENDENCY_ROOT/.local/.dependency-environment.lock" && -d "$ENV_LOCK" && ! -L "$ENV_LOCK" ]]; then
        rmdir -- "$ENV_LOCK" || printf 'dependency-images: cannot remove owned environment lock\n' >&2
    fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# shellcheck source=tools/quality/dependency-images/registry.sh
. "$SCRIPT_DIR/registry.sh"
: >"$TEMP/updates.env"
: >"$TEMP/github-output"
declare -A SEEN=()

prepare_local() {
    local existing=${!ENV_KEY:-} rc built_id
    if [[ -n "$existing" && "$existing" == *@sha256:* ]]; then
        valid_saved_reference "$COMPONENT" "$existing" || fail 'supplied dependency repository is not trusted'
        REGISTRY_DIGEST=${existing##*@}
        valid_digest "$REGISTRY_DIGEST" || fail 'supplied dependency digest is invalid'
        IMMUTABLE_REF=$existing
        verify_remote
        SELECTED_REF=$existing
        return
    fi
    if valid_digest "$existing"; then
        verify_local "$existing"
        [[ "$IMAGE_ID" == "$existing" ]] || fail 'supplied local ID resolved to different bytes'
        SELECTED_REF=$IMAGE_ID
        return
    fi
    if docker image inspect "$LOCAL_REF" >"$TEMP/local.json" 2>"$TEMP/local.log"; then
        verify_local "$LOCAL_REF"
    else
        rc=$?
        grep -Eiq 'No such image|No such object' "$TEMP/local.log" || fail "local daemon lookup failed (exit $rc); refusing build"
        docker build --platform "$DEPENDENCY_PLATFORM" --file "$DEPENDENCY_ROOT/$DOCKERFILE" \
            --build-arg "DEPENDENCY_INPUTS=$INPUT_HASH" --label "$DEPENDENCY_INPUT_LABEL=$INPUT_HASH" \
            --iidfile "$TEMP/local-build.id" --tag "$LOCAL_REF" "$DEPENDENCY_ROOT/$CONTEXT" >&2 || fail "local dependency build failed: $COMPONENT"
        built_id=$(<"$TEMP/local-build.id")
        valid_digest "$built_id" || fail 'local build did not return an immutable image ID'
        verify_local "$built_id"
        [[ "$IMAGE_ID" == "$built_id" ]] || fail 'local build ID resolved to different bytes'
    fi
    # Docker image IDs, unlike tags, cannot race another checkout's retagging.
    SELECTED_REF=$IMAGE_ID
}

for name in "${COMPONENTS[@]}"; do
    load_component "$name"
    [[ -z "${SEEN[$name]:-}" ]] || fail 'duplicate component request'
    SEEN[$name]=1
    case "$MODE" in
        prepare-local) prepare_local ;;
        publish | resolve)
            if resolve_remote; then
                verify_remote
            else
                rc=$?
                [[ "$rc" == 4 ]] || fail 'unexpected dependency-resolution result'
                [[ "$MODE" == publish ]] || fail "signed dependency artifact absent: $COMPONENT; run explicit publication"
                publish_missing
            fi
            SELECTED_REF=$IMMUTABLE_REF
            ;;
    esac
    printf '%s=%s\n' "$ENV_KEY" "$SELECTED_REF" >>"$TEMP/updates.env"
    printf '%s=%s\n' "$OUTPUT_KEY" "$SELECTED_REF" >>"$TEMP/github-output"
    printf 'dependency-images: %s %s (%s)\n' "$MODE" "$COMPONENT" "$SELECTED_REF" >&2
done

# Preserve other component selections when a caller prepares a subset. Never
# source an old environment file: parse only the finite inventory's keys.
# No polling or sleeps: simultaneous writers fail explicitly instead of losing
# selections. A crashed writer's retained lock likewise requires operator review.
if mkdir "$DEPENDENCY_ROOT/.local/.dependency-environment.lock" 2>/dev/null; then
    ENV_LOCK="$DEPENDENCY_ROOT/.local/.dependency-environment.lock"
else
    fail 'another dependency environment writer holds the merge lock; retry explicit preparation after it completes'
fi
[[ ! -L "$ENV_FILE" ]] || fail 'dependency environment became a symlink'
: >"$TEMP/environment"
if [[ -f "$ENV_FILE" ]]; then
    while IFS='=' read -r key value || [[ -n "$key" || -n "$value" ]]; do
        [[ -n "$key" && -n "$value" && "$value" != *[[:space:]]* ]] || fail 'invalid saved dependency environment'
        case "$key" in KEYCLOAK_IMAGE | RESTIC_IMAGE | SOPS_IMAGE | SOCKET_PROXY_IMAGE | POSTGRES_IMAGE | CLOUDFLARED_IMAGE | CADDY_RUNTIME_IMAGE) ;; *) fail 'unknown saved dependency key' ;; esac
        if ! grep -q "^${key}=" "$TEMP/updates.env"; then
            saved_component=$(printf '%s' "${key%_IMAGE}" | tr '[:upper:]_' '[:lower:]-')
            valid_saved_reference "$saved_component" "$value" || fail 'invalid saved dependency reference'
            printf '%s=%s\n' "$key" "$value" >>"$TEMP/environment"
        fi
    done <"$ENV_FILE"
fi
while IFS= read -r update; do printf '%s\n' "$update" >>"$TEMP/environment"; done <"$TEMP/updates.env"
chmod 0600 "$TEMP/environment"
# Both checked targets are inside the same filesystem; publication is atomic.
[[ "$ENV_FILE" == "$DEPENDENCY_ROOT/.local/security-images.env" && ! -L "$ENV_FILE" ]] || fail 'unsafe environment publication path'
mv -- "$TEMP/environment" "$ENV_FILE"
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    while IFS= read -r output; do printf '%s\n' "$output" >>"$GITHUB_OUTPUT"; done <"$TEMP/github-output"
fi
while IFS= read -r update; do printf '%s\n' "$update"; done <"$TEMP/updates.env"
