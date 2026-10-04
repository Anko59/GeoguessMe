#!/usr/bin/env bash
# Native daemon-builder smoke through make test-local-dependency-from. All bases
# are tiny FROM-scratch fixtures: no registry, package manager, compiler or push.
# Raw FROM sha256:configID can be interpreted as a registry name by BuildKit;
# production deliberately uses config-ID-addressed aliases without probing it.
set -euo pipefail
TMP=$(mktemp -d)
[[ "$TMP" == /tmp/* && -d "$TMP" && ! -L "$TMP" ]] || exit 1
nonce=${TMP##*/}
repository=geoguessme/local-from-smoke
base_tag="$repository:$nonce-base"
other_tag="$repository:$nonce-other"
consumer_tag="$repository:$nonce-consumer"
alias=''
cleanup() {
    local tag owner
    # Every candidate is uniquely scoped to this mktemp-created test. Verify
    # ownership before removing any image reference, including inherited labels.
    for tag in "$consumer_tag" "$base_tag" "$other_tag" "$alias"; do
        [[ -n "$tag" ]] || continue
        if owner=$(docker image inspect --format '{{index .Config.Labels "dev.geoguessme.local-from-smoke"}}' "$tag" 2>"$TMP/cleanup.log"); then
            [[ "$owner" == "$nonce" ]] || {
                printf 'FAIL: refusing to remove unowned smoke image\n' >&2
                exit 1
            }
            docker image rm "$tag" >/dev/null || {
                printf 'FAIL: cannot remove owned smoke image\n' >&2
                exit 1
            }
        elif ! grep -Eiq 'No such image|No such object' "$TMP/cleanup.log"; then
            printf 'FAIL: cannot inspect smoke image during cleanup\n' >&2
            exit 1
        fi
    done
    [[ "$TMP" == /tmp/* && -d "$TMP" && ! -L "$TMP" ]] || exit 1
    rm -rf -- "$TMP"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir -p "$TMP/base" "$TMP/consumer"
printf 'FROM scratch\nCOPY marker /base-marker\n' >"$TMP/base/Dockerfile"
printf '%s original fixture\n' "$nonce" >"$TMP/base/marker"
printf "ARG RUNTIME\nFROM \${RUNTIME}\nCOPY marker /consumer-marker\n" >"$TMP/consumer/Dockerfile"
printf '%s consumer fixture\n' "$nonce" >"$TMP/consumer/marker"
# CI can select a docker-container builder for application registry outputs.
# This isolated local-store smoke explicitly uses the active daemon's builder.
build=(docker build)
unset BUILDX_BUILDER
if docker buildx version >"$TMP/buildx.log" 2>&1; then
    builder=$(docker context show)
    driver=$(docker buildx inspect "$builder" | awk '$1=="Driver:" {print $2}')
    [[ "$driver" == docker ]] || {
        printf 'FAIL: local FROM smoke requires the active Docker daemon builder\n' >&2
        exit 1
    }
    build+=(--builder "$builder")
else
    # Buildx is not a supported-host prerequisite. Docker installations without
    # the plugin use the legacy daemon builder, exactly as local Make builds do.
    grep -Eiq 'unknown command|not a docker command' "$TMP/buildx.log" || {
        printf 'FAIL: cannot determine Docker builder availability\n' >&2
        exit 1
    }
    export DOCKER_BUILDKIT=0
fi
version=$(docker version --format '{{.Server.Version}}')
"${build[@]}" --platform linux/amd64 --network none --quiet \
    --label "dev.geoguessme.local-from-smoke=$nonce" --tag "$base_tag" \
    --iidfile "$TMP/base.id" "$TMP/base"
base_id=$(<"$TMP/base.id")
[[ "$base_id" =~ ^sha256:[0-9a-f]{64}$ ]] || exit 1
alias="$repository:config-${base_id#sha256:}"
docker image tag "$base_id" "$alias"
base_layer=$(docker image inspect --format '{{index .RootFS.Layers 0}}' "$base_id")
printf '%s competing fixture\n' "$nonce" >"$TMP/base/marker"
"${build[@]}" --platform linux/amd64 --network none --quiet \
    --label "dev.geoguessme.local-from-smoke=$nonce" --tag "$other_tag" \
    --iidfile "$TMP/other.id" "$TMP/base"
other_id=$(<"$TMP/other.id")
[[ "$other_id" =~ ^sha256:[0-9a-f]{64}$ && "$other_id" != "$base_id" ]] || exit 1
docker image tag "$other_id" "$base_tag"
[[ "$(docker image inspect --format '{{.Id}}' "$alias")" == "$base_id" ]] || exit 1
"${build[@]}" --platform linux/amd64 --network none --quiet \
    --build-arg "RUNTIME=$alias" --tag "$consumer_tag" "$TMP/consumer"
[[ "$(docker image inspect --format '{{index .RootFS.Layers 0}}' "$consumer_tag")" == "$base_layer" ]] || {
    printf 'FAIL: consumer inherited retagged bytes instead of the prepared image\n' >&2
    exit 1
}
printf 'PASS: Docker %s local FROM uses frozen config-ID alias despite competing input-tag retag\n' "$version"
