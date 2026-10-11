#!/usr/bin/env bash
# Shared reviewed-input identity and immutable image validation. Sourced only.
set -euo pipefail

DEPENDENCY_ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)
DEPENDENCY_MANIFEST="$DEPENDENCY_ROOT/deployment/images/dependencies.tsv"
DEPENDENCY_PLATFORM=linux/amd64
DEPENDENCY_INPUT_LABEL=dev.geoguessme.dependency-inputs

# Ordinary development requires Git/Make/Docker, not a host jq installation.
# CI/operator hosts may already provide jq; otherwise use the bootstrapped tool.
if ! command -v jq >/dev/null 2>&1; then
    jq() {
        local project=${GEOGUESSME_TOOLS_PROJECT:-geoguessme-dependency-tools} common_dir
        common_dir=${GEOGUESSME_GIT_COMMON_DIR:-$(git -C "$DEPENDENCY_ROOT" rev-parse --path-format=absolute --git-common-dir)}
        GEOGUESSME_TOOLS_PROJECT="$project" GEOGUESSME_GIT_COMMON_DIR="$common_dir" \
            TOOLS_UID="$(id -u)" TOOLS_GID="$(id -g)" \
            docker compose -p "$project" -f "$DEPENDENCY_ROOT/deployment/compose.tools.yaml" \
            --project-directory "$DEPENDENCY_ROOT" run -T --rm --no-deps go-security jq "$@"
    }
fi

fail() {
    printf 'dependency-images: %s\n' "$*" >&2
    exit 1
}

valid_digest() { [[ "$1" =~ ^sha256:[0-9a-f]{64}$ ]]; }

valid_saved_reference() {
    local component=$1 ref=$2
    valid_digest "$ref" && return 0
    [[ "$ref" =~ ^ghcr\.io/anko59/geoguessme-${component}:[a-zA-Z0-9_.-]+@sha256:[0-9a-f]{64}$ ]]
}

reviewed_file() {
    local path=$1
    [[ "$path" != /* && "$path" != *'..'* && "$path" != frontend/* && "$path" != *$'\n'* ]] ||
        fail "invalid dependency input path: $path"
    [[ -f "$DEPENDENCY_ROOT/$path" && ! -L "$DEPENDENCY_ROOT/$path" ]] ||
        fail "missing or symlinked dependency input: $path"
    # Reject symlinked ancestor directories, too: hashes must describe this tree.
    [[ "$(realpath "$DEPENDENCY_ROOT/$path")" == "$DEPENDENCY_ROOT/$path" ]] ||
        fail "dependency input escapes its reviewed path: $path"
}

# This sourced function returns named identity fields consumed by its callers.
# shellcheck disable=SC2034
load_component() {
    local requested=$1 row count
    [[ "$requested" =~ ^[a-z][a-z0-9-]*$ ]] || fail 'invalid component name'
    count=$(awk -F '\t' -v name="$requested" '$1 == name {n++} END {print n+0}' "$DEPENDENCY_MANIFEST")
    [[ "$count" == 1 ]] || fail "unknown or duplicate component: $requested"
    row=$(awk -F '\t' -v name="$requested" '$1 == name {print}' "$DEPENDENCY_MANIFEST")
    IFS=$'\t' read -r COMPONENT DOCKERFILE CONTEXT EXTRA_INPUTS unexpected <<<"$row"
    [[ -n "$DOCKERFILE" && -n "$CONTEXT" && -n "$EXTRA_INPUTS" && -z "$unexpected" ]] || fail 'invalid dependency manifest row'
    reviewed_file "$DOCKERFILE"
    # Dependency publications adapt metadata/runtime settings only. A new
    # compiler advisory must never silently turn us into an upstream maintainer.
    awk '
        toupper($1) == "FROM" {bases++}
        toupper($1) ~ /^(RUN|COPY|ADD|ONBUILD)(\[|$)/ {bad=1}
        END {exit bad || bases != 1}
    ' "$DEPENDENCY_ROOT/$DOCKERFILE" || fail "dependency must remain an upstream publication envelope: $COMPONENT"
    [[ "$CONTEXT" == . || ("$CONTEXT" != /* && "$CONTEXT" != *'..'*) ]] || fail 'invalid build context'
    [[ -d "$DEPENDENCY_ROOT/$CONTEXT" && ! -L "$DEPENDENCY_ROOT/$CONTEXT" ]] || fail 'missing or symlinked build context'
    [[ "$(realpath "$DEPENDENCY_ROOT/$CONTEXT")" == "$(realpath "$DEPENDENCY_ROOT")" ||
    "$(realpath "$DEPENDENCY_ROOT/$CONTEXT")" == "$DEPENDENCY_ROOT/$CONTEXT" ]] || fail 'build context escapes reviewed tree'
    # Every external FROM must be pinned. Stage-to-stage references are allowed.
    awk '
        toupper($1) == "FROM" {
            i=2; if ($i ~ /^--platform=/) i++
            base=$i
            if (base != "scratch" && !(base in stages)) {
                n=split(base, parts, "@sha256:")
                if (n != 2 || length(parts[2]) != 64 || parts[2] !~ /^[0-9a-f]+$/) exit 1
            }
            if (NF >= i+2 && toupper($(i+1)) == "AS") stages[$(i+2)]=1
            final=base
        }
        END {if (!final) exit 1}
    ' "$DEPENDENCY_ROOT/$DOCKERFILE" || fail "unpinned dependency build input: $DOCKERFILE"
    FINAL_BASE=$(awk 'toupper($1)=="FROM" {i=2; if ($i ~ /^--platform=/) i++; base=$i} END {print base}' "$DEPENDENCY_ROOT/$DOCKERFILE")
    valid_digest "${FINAL_BASE##*@}" || fail 'final runtime must have pinned upstream provenance'
    FINAL_BASE_NAME=${FINAL_BASE%@*}
    FINAL_BASE_DIGEST=${FINAL_BASE##*@}
    INPUTS=("$DOCKERFILE" .dockerignore)
    if [[ "$CONTEXT" != . && -f "$DEPENDENCY_ROOT/$CONTEXT/.dockerignore" ]]; then INPUTS+=("$CONTEXT/.dockerignore"); fi
    if [[ -f "$DEPENDENCY_ROOT/$DOCKERFILE.dockerignore" ]]; then INPUTS+=("$DOCKERFILE.dockerignore"); fi
    if [[ "$EXTRA_INPUTS" != - ]]; then
        local -a extras
        IFS=, read -r -a extras <<<"$EXTRA_INPUTS"
        INPUTS+=("${extras[@]}")
    fi
    # Context files must be enumerated, not accidentally inherited from a large
    # application build context. Unsupported dynamic/glob/directory COPY syntax
    # fails closed and requires extending this reviewed-input contract first.
    awk -v context="$CONTEXT" -v extras="$EXTRA_INPUTS" '
        BEGIN {if (extras != "-") {n=split(extras, paths, ","); for (i=1; i<=n; i++) allowed[paths[i]]=1}}
        $1 !~ /^#/ && /--mount=.*type=bind/ {bad=1}
        toupper($1)=="FROM" && toupper($(NF-1))=="AS" {stages[$NF]=1}
        toupper($1)=="COPY" || toupper($1)=="ADD" {
            if ($0 ~ /--from=/) {
                from=$0; sub(/^.*--from=/, "", from); sub(/[[:space:]].*$/, "", from)
                if (!(from in stages)) bad=1
                next
            }
            line=$0; sub(/^[[:space:]]*[A-Za-z]+[[:space:]]+/, "", line)
            if (line ~ /\\\\/) {bad=1; next}
            gsub(/[\[\]",]/, " ", line)
            n=split(line, fields, /[[:space:]]+/)
            first=1; while (first<=n && (fields[first]=="" || fields[first] ~ /^--/)) first++
            last=n; while (last>first && fields[last]=="") last--
            if (last<=first) bad=1
            for (i=first; i<last; i++) {
                source=fields[i]
                if (source !~ /^[A-Za-z0-9_.\/-]+$/ || source ~ /^\// || source ~ /\.\./) {bad=1; continue}
                path=(context=="." ? source : context "/" source)
                if (!(path in allowed)) bad=1
            }
        }
        END {exit bad}
    ' "$DEPENDENCY_ROOT/$DOCKERFILE" || fail "context inputs are not explicitly reviewed: $COMPONENT"
    local path
    for path in "${INPUTS[@]}"; do reviewed_file "$path"; done
    INPUT_HASH=$(
        {
            printf 'geoguessme-dependency-v1\nplatform=%s\ncomponent=%s\ncontext=%s\n' "$DEPENDENCY_PLATFORM" "$COMPONENT" "$CONTEXT"
            for path in "${INPUTS[@]}"; do
                printf 'path=%s\n' "$path"
                if [[ -x "$DEPENDENCY_ROOT/$path" ]]; then printf 'mode=100755\n'; else printf 'mode=100644\n'; fi
                sha256sum "$DEPENDENCY_ROOT/$path" | awk '{print $1}'
            done
        } | sha256sum | awk '{print $1}'
    )
    LOCAL_REF="geoguessme/$COMPONENT:dependency-$INPUT_HASH"
    REMOTE_REF="ghcr.io/anko59/geoguessme-$COMPONENT:dependency-$INPUT_HASH"
    ENV_KEY=$(printf '%s_IMAGE' "$COMPONENT" | tr '[:lower:]-' '[:upper:]_')
    OUTPUT_KEY=${COMPONENT//-/_}
}

component_names() { awk -F '\t' '!/^#/ && NF {print $1}' "$DEPENDENCY_MANIFEST"; }

verify_local() {
    local ref=$1 inspect
    inspect=$(docker image inspect "$ref") || fail "cannot inspect artifact: $ref"
    jq -e --arg hash "$INPUT_HASH" --arg key "$DEPENDENCY_INPUT_LABEL" \
        --arg name "$FINAL_BASE_NAME" --arg digest "$FINAL_BASE_DIGEST" '
        length == 1 and .[0].Os == "linux" and .[0].Architecture == "amd64" and
        .[0].Config.Labels[$key] == $hash and
        .[0].Config.Labels["org.opencontainers.image.base.name"] == $name and
        .[0].Config.Labels["org.opencontainers.image.base.digest"] == $digest
    ' <<<"$inspect" >/dev/null || fail "conflicting identity, platform or upstream provenance: $ref"
    IMAGE_ID=$(jq -er '.[0].Id' <<<"$inspect")
    valid_digest "$IMAGE_ID" || fail 'invalid local immutable image ID'
}
