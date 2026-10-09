#!/usr/bin/env bash
# Give the offline native parser only reviewed definitions, never operator files.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
[[ ! -L "$ROOT/.local" ]] || {
    echo 'cloud-init-test: unsafe local directory' >&2
    exit 2
}
mkdir -p "$ROOT/.local"
TEMP=$(mktemp -d "$ROOT/.local/cloud-init-test.XXXXXXXX")
cleanup() {
    case "$TEMP" in "$ROOT/.local/cloud-init-test."*)
        [[ -d "$TEMP" && ! -L "$TEMP" ]] || return
        resolved=$(CDPATH='' cd -- "$TEMP" && pwd -P)
        [[ "$resolved" == "$TEMP" ]] || return
        rm -rf -- "$TEMP"
        ;;
    esac
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
copy_source() {
    local file=$1 destination=$2
    [[ "$file" != /* && "$file" != *'..'* && -f "$ROOT/$file" && ! -L "$ROOT/$file" ]] || {
        echo 'cloud-init-test: unsafe reviewed source' >&2
        exit 2
    }
    mkdir -p "$destination/$(dirname -- "$file")"
    cp -- "$ROOT/$file" "$destination/$file"
}
# Small build context: only the Dockerfile and the existing provider pins.
for file in tools/quality/cloud-init/Dockerfile infra/terraform/versions.tf infra/terraform/.terraform.lock.hcl; do
    copy_source "$file" "$TEMP/context"
done
# Keep untracked operator state, tfvars, dotenv, Git credentials and backups out.
while IFS= read -r -d '' file; do
    copy_source "$file" "$TEMP/workspace"
done < <(git -C "$ROOT" ls-files -z -- 'infra/terraform/*.tf' 'infra/terraform/.terraform.lock.hcl' \
    'infra/terraform/tests/*.tftest.hcl' 'infra/cloud-init/*' \
    'deployment/scripts/hosted/*.sh' 'deployment/compose.production.yaml' \
    'deployment/compose.hosted.yaml' 'deployment/compose.watch.yaml' \
    'deployment/watch/*' 'deployment/s3-fixture/credentials.json' 'deployment/images/host-tools.json')
copy_source tools/quality/cloud-init/test-user-data.py "$TEMP/workspace"
read -r -a build_flags <<<"${DOCKER_BUILD_FLAGS:-}"
if docker buildx version >/dev/null 2>&1; then
    # The hosted docker-container driver does not load images unless requested.
    docker buildx build --load "${build_flags[@]}" --iidfile "$TEMP/image.id" \
        -f "$TEMP/context/tools/quality/cloud-init/Dockerfile" -t geoguessme/cloud-init-tools:local "$TEMP/context"
else
    docker build "${build_flags[@]}" --iidfile "$TEMP/image.id" \
        -f "$TEMP/context/tools/quality/cloud-init/Dockerfile" -t geoguessme/cloud-init-tools:local "$TEMP/context"
fi
image=$(<"$TEMP/image.id")
[[ "$image" =~ ^sha256:[0-9a-f]{64}$ ]] || {
    echo 'cloud-init-test: invalid tool image ID' >&2
    exit 2
}
docker run --rm --network none --mount "type=bind,src=$TEMP/workspace,dst=/workspace,readonly" -w /workspace "$image"
