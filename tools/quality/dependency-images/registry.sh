#!/usr/bin/env bash
# Registry operations are retried only by the shared operational-error helper.
# Sourced after common.sh. Lifecycle owns its TEMP; adoption/promotion explicitly
# initialize a separate transport session with the same output isolation.
set -euo pipefail

registry_session_cleanup() {
    [[ "$TEMP" == "$DEPENDENCY_ROOT/.local/.dependency-transport."* && -d "$TEMP" && ! -L "$TEMP" ]] || return 0
    rm -rf -- "$TEMP" || {
        printf 'dependency-images: cannot remove transport session\n' >&2
        exit 1
    }
}

start_registry_session() {
    [[ ! -L "$DEPENDENCY_ROOT/.local" ]] || fail 'local transport directory must not be a symlink'
    mkdir -p "$DEPENDENCY_ROOT/.local"
    TEMP=$(mktemp -d "$DEPENDENCY_ROOT/.local/.dependency-transport.XXXXXX")
    trap registry_session_cleanup EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
}

remote_operation() {
    local label=$1
    shift
    : >"$TEMP/stdout"
    : >"$TEMP/registry.log"
    bash "$DEPENDENCY_ROOT/tools/quality/image-audit/retry.sh" "$label" "$TEMP/registry.log" \
        bash -c 'output=$1; shift; "$@" >"$output"' _ "$TEMP/stdout" "$@" >&2
}

resolve_remote() {
    local rc
    if remote_operation "resolve-$COMPONENT" docker buildx imagetools inspect "$REMOTE_REF" --format '{{json .Manifest.Digest}}'; then
        REGISTRY_DIGEST=$(jq -er 'select(type == "string")' "$TEMP/stdout") || fail 'invalid registry digest response'
        valid_digest "$REGISTRY_DIGEST" || fail 'invalid registry immutable digest'
        IMMUTABLE_REF="$REMOTE_REF@$REGISTRY_DIGEST"
        return 0
    else
        rc=$?
    fi
    # Only a definitive missing manifest is permission to create an artifact.
    # Authentication, rate limits, timeouts and other unknown errors stay closed.
    if ! grep -Eiq 'unauthorized|forbidden|denied|\b(401|403|429|5[0-9]{2})\b|too many requests|timeout' "$TEMP/registry.log" &&
        { grep -Eiq 'manifest[ _-]unknown|MANIFEST_UNKNOWN' "$TEMP/registry.log" ||
            grep -Fq "$REMOTE_REF: not found" "$TEMP/registry.log"; }; then
        return 4
    fi
    fail "registry resolution incomplete for $COMPONENT (exit $rc); refusing to rebuild"
}

verify_artifact() {
    local raw provenance
    SLSA_PROVENANCE=''
    remote_operation "manifest-$COMPONENT" docker buildx imagetools inspect "$IMMUTABLE_REF" --raw || fail 'cannot inspect dependency manifest'
    raw=$(<"$TEMP/stdout")
    jq -e '
        [.manifests[] | select(.platform.os == "linux" and .platform.architecture == "amd64")] as $runtime |
        ($runtime | length) == 1 and
        any(.manifests[]; .annotations["vnd.docker.reference.type"] == "attestation-manifest" and
            .annotations["vnd.docker.reference.digest"] == $runtime[0].digest)
    ' <<<"$raw" >/dev/null || fail "missing AMD64 OCI provenance attestation: $COMPONENT"
    remote_operation "provenance-$COMPONENT" docker buildx imagetools inspect "$IMMUTABLE_REF" --format '{{json .Provenance}}' || fail 'cannot read dependency provenance'
    provenance=$(<"$TEMP/stdout")
    # Buildx projects a sole runtime directly; platform maps stay explicit.
    SLSA_PROVENANCE=$(jq -cse --argjson manifest "$raw" '
        select(length == 1) | .[0] | select(type == "object") |
        if keys == ["SLSA"] then
            select(([$manifest.manifests[] |
                select(.annotations["vnd.docker.reference.type"] != "attestation-manifest")] | length) == 1) |
            .SLSA
        elif (has("SLSA") | not) and (.["linux/amd64"] | type) == "object" then
            .["linux/amd64"].SLSA
        else empty end |
        select(type == "object")
    ' <<<"$provenance") || fail "invalid BuildKit SLSA projection: $COMPONENT"
    jq -e '
        . as $slsa |
        ($slsa | type) == "object" and
        ($slsa.buildType == "https://mobyproject.org/buildkit@v1" or
         (($slsa.buildDefinition | type) == "object" and
          ($slsa.buildDefinition.buildType == "https://mobyproject.org/buildkit@v1" or
           $slsa.buildDefinition.buildType == "https://github.com/moby/buildkit/blob/master/docs/attestations/slsa-definitions.md")))
    ' <<<"$SLSA_PROVENANCE" >/dev/null || fail "missing BuildKit SLSA provenance: $COMPONENT"
    remote_operation "pull-$COMPONENT" docker pull --platform "$DEPENDENCY_PLATFORM" "$IMMUTABLE_REF" || fail 'cannot pull verified dependency artifact'
    verify_local "$IMMUTABLE_REF"
    # Bind local pulled content to the signed index, not merely a mutable tag.
    docker image inspect "$IMMUTABLE_REF" --format '{{json .RepoDigests}}' |
        jq -e --arg digest "$REGISTRY_DIGEST" 'any(.[]; endswith("@" + $digest))' >/dev/null || fail 'pulled dependency digest mismatch'
}

verify_remote() {
    remote_operation "verify-$COMPONENT" cosign verify \
        --certificate-oidc-issuer https://token.actions.githubusercontent.com \
        --certificate-identity-regexp '^https://github.com/Anko59/GeoguessMe/\.github/workflows/(deploy|security)\.yml@refs/heads/dev$' \
        --annotations "dependency-inputs=$INPUT_HASH" --annotations dependency-build=true "$IMMUTABLE_REF" || fail "untrusted dependency artifact: $COMPONENT"
    verify_artifact
}

publish_missing() {
    local rc
    # CI callers share a publication concurrency group. Recheck under that group
    # before creating anything; failed or unsigned existing content is not replaced.
    if resolve_remote; then
        verify_remote
        return
    else
        rc=$?
        [[ "$rc" == 4 ]] || fail 'unexpected dependency-resolution result'
    fi
    local metadata="$TEMP/build.json"
    remote_operation "build-$COMPONENT" docker buildx build \
        --platform "$DEPENDENCY_PLATFORM" --file "$DEPENDENCY_ROOT/$DOCKERFILE" \
        --build-arg "DEPENDENCY_INPUTS=$INPUT_HASH" --label "$DEPENDENCY_INPUT_LABEL=$INPUT_HASH" \
        --label "org.opencontainers.image.base.name=$FINAL_BASE_NAME" \
        --label "org.opencontainers.image.base.digest=$FINAL_BASE_DIGEST" \
        --tag "$REMOTE_REF" --push --sbom=true --provenance=mode=max \
        --metadata-file "$metadata" "$DEPENDENCY_ROOT/$CONTEXT" || fail "dependency publication failed: $COMPONENT"
    REGISTRY_DIGEST=$(jq -er '.["containerimage.digest"]' "$metadata") || fail 'build returned no immutable digest'
    valid_digest "$REGISTRY_DIGEST" || fail 'build returned invalid immutable digest'
    IMMUTABLE_REF="$REMOTE_REF@$REGISTRY_DIGEST"
    verify_artifact
    # Never sign unscanned bytes. Scanner owns policy and all registry credentials
    # remain on the host; it exports this authenticated digest for tar scanning.
    make -C "$DEPENDENCY_ROOT" audit-image-set "IMAGE_AUDIT_REFS=$IMMUTABLE_REF" >&2 || fail "dependency vulnerability audit failed: $COMPONENT"
    remote_operation "sign-$COMPONENT" cosign sign --yes -a "dependency-inputs=$INPUT_HASH" -a dependency-build=true "$IMMUTABLE_REF" || fail 'dependency signing failed'
    verify_remote
}
