#!/usr/bin/env bash
# One reviewed interrupted publication; never a general unsigned-artifact mode.
set -euo pipefail
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=tools/quality/dependency-images/common.sh
. "$SCRIPT_DIR/common.sh"
[[ $# == 0 ]] || fail 'Keycloak recovery accepts no artifact arguments'
[[ "${GITHUB_ACTIONS:-}" == true && "${GITHUB_EVENT_NAME:-}" == workflow_dispatch &&
    "${GITHUB_REPOSITORY:-}" == Anko59/GeoguessMe && "${GITHUB_REF:-}" == refs/heads/dev &&
    "${GITHUB_REF_PROTECTED:-}" == true &&
    "${GITHUB_WORKFLOW_REF:-}" == Anko59/GeoguessMe/.github/workflows/security.yml@refs/heads/dev ]] ||
    fail 'Keycloak recovery requires the manually dispatched protected dev Security workflow'
readonly REVIEWED_INPUTS=cb1dd9163b630bbda840ba2906310e4b402f55e69bc81b4bc4c8f95c26511689
readonly REVIEWED_INDEX=sha256:f3b02924f109607058d1238eab1c06ec7fdc9f6151beb5252ec765f3fe5060f3
readonly REVIEWED_RUNTIME=sha256:41e201ab66e028e3130f18b3c0073564846d5aeeff40c7a6ba3462dc0d708377
readonly REVIEWED_REVISION=a228769dbaf98ce8a5aff7fb0946456f34f8e21b
readonly REVIEWED_RUN=37242418190 REVIEWED_JOB=111553605968
load_component keycloak
[[ "$INPUT_HASH" == "$REVIEWED_INPUTS" &&
    "$FINAL_BASE_NAME" == quay.io/keycloak/keycloak:26.7.5 &&
    "$FINAL_BASE_DIGEST" == sha256:37dbaf6f0722c9ec246335f36e1ef8b2e6cb960f7c27e0d8c615121a3d475a85 ]] ||
    fail 'reviewed Keycloak recovery inputs are stale'
# shellcheck source=tools/quality/dependency-images/registry.sh
. "$SCRIPT_DIR/registry.sh"
start_registry_session

# Authenticated GitHub evidence binds the unsigned index to the original trusted
# publisher. Missing/expired logs and API/auth failures require operator review.
remote_operation recovery-run gh api "repos/Anko59/GeoguessMe/actions/runs/$REVIEWED_RUN" || fail 'cannot read original publisher run'
jq -e --arg revision "$REVIEWED_REVISION" --argjson run "$REVIEWED_RUN" '
    .id == $run and .head_sha == $revision and .head_branch == "dev" and
    .event == "push" and .path == ".github/workflows/security.yml" and
    .repository.full_name == "Anko59/GeoguessMe" and .conclusion == "failure"
' "$TEMP/stdout" >/dev/null || fail 'original publisher run does not match the reviewed origin'
remote_operation recovery-job gh api "repos/Anko59/GeoguessMe/actions/jobs/$REVIEWED_JOB" || fail 'cannot read original publisher job'
jq -e --argjson run "$REVIEWED_RUN" --argjson job "$REVIEWED_JOB" '
    .id == $job and .run_id == $run and .name == "audit-images" and .conclusion == "failure" and
    any(.steps[]; .name == "Explicit dependency preparation (only changed inputs build)" and .conclusion == "failure")
' "$TEMP/stdout" >/dev/null || fail 'original publisher job does not match the reviewed failure'
# Preserve original ANSI log bytes only in the private evidence file, never a terminal.
remote_operation recovery-log gh api --allow-escape-sequences "repos/Anko59/GeoguessMe/actions/jobs/$REVIEWED_JOB/logs" || fail 'cannot read original publication log'
if ! grep -Fq "pushing manifest for $REMOTE_REF@$REVIEWED_INDEX" "$TEMP/stdout" ||
    ! grep -Fq 'dependency-images: missing BuildKit SLSA provenance: keycloak' "$TEMP/stdout"; then
    fail 'original log does not prove this exact interrupted publication'
fi
resolve_remote || fail 'reviewed Keycloak registry artifact is unavailable'
[[ "$REGISTRY_DIGEST" == "$REVIEWED_INDEX" ]] || fail 'reviewed Keycloak input tag changed digest'
verify_artifact

# Inspect the original index, not a rebuilt image or an operator-supplied alias.
remote_operation recovery-index docker buildx imagetools inspect "$IMMUTABLE_REF" --raw || fail 'cannot inspect reviewed Keycloak index'
jq -e --arg runtime "$REVIEWED_RUNTIME" '
    [.manifests[] | select(.platform.os == "linux" and .platform.architecture == "amd64")] |
    length == 1 and .[0].digest == $runtime
' "$TEMP/stdout" >/dev/null || fail 'reviewed Keycloak runtime linkage changed'
remote_operation recovery-provenance docker buildx imagetools inspect "$IMMUTABLE_REF" --format '{{json .Provenance}}' || fail 'cannot read reviewed Keycloak provenance'
jq -e --arg inputs "$REVIEWED_INPUTS" --arg revision "$REVIEWED_REVISION" --arg base "${FINAL_BASE_DIGEST#sha256:}" '
    .["linux/amd64"].SLSA as $slsa |
    $slsa.buildDefinition.externalParameters.request.root.request.args as $args |
    $slsa.buildDefinition.buildType == "https://github.com/moby/buildkit/blob/master/docs/attestations/slsa-definitions.md" and
    $slsa.buildDefinition.externalParameters.configSource.path == "Dockerfile" and
    $args["build-arg:DEPENDENCY_INPUTS"] == $inputs and
    $args["label:dev.geoguessme.dependency-inputs"] == $inputs and
    $args["vcs:revision"] == $revision and $args["vcs:source"] == "https://github.com/Anko59/GeoguessMe" and
    $args["vcs:localdir:context"] == "deployment/docker/keycloak-patched" and
    $args["vcs:localdir:dockerfile"] == "deployment/docker/keycloak-patched" and
    any($slsa.buildDefinition.resolvedDependencies[]; .digest.sha256 == $base and
        (.uri | startswith("pkg:docker/quay.io/keycloak/keycloak@26.7.5?"))) and
    $slsa.runDetails.metadata.buildkit_metadata.vcs.revision == $revision and
    $slsa.runDetails.metadata.buildkit_metadata.vcs.source == "https://github.com/Anko59/GeoguessMe" and
    ([$slsa.runDetails.metadata.buildkit_metadata.source.infos[] | select(.filename == "Dockerfile")] | length) == 1
' "$TEMP/stdout" >/dev/null || fail 'reviewed Keycloak provenance origin or inputs changed'
jq -er '.["linux/amd64"].SLSA.runDetails.metadata.buildkit_metadata.source.infos[] |
    select(.filename == "Dockerfile") | .data' "$TEMP/stdout" | base64 -d >"$TEMP/Dockerfile" || fail 'cannot decode reviewed Keycloak recipe'
cmp "$TEMP/Dockerfile" "$DEPENDENCY_ROOT/$DOCKERFILE" >/dev/null || fail 'reviewed Keycloak embedded recipe changed'

make -C "$DEPENDENCY_ROOT" audit-image-set "IMAGE_AUDIT_REFS=$IMMUTABLE_REF" >&2 || fail 'reviewed Keycloak native audit failed'

# Idempotency is allowed only for an already-valid original build signature.
# An invalid signature or operational failure is not permission to add trust.
if remote_operation recovery-signature cosign verify \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity-regexp '^https://github.com/Anko59/GeoguessMe/\.github/workflows/(deploy|security)\.yml@refs/heads/dev$' \
    --annotations "dependency-inputs=$INPUT_HASH" --annotations dependency-build=true "$IMMUTABLE_REF"; then
    verify_remote
    printf 'dependency-images: reviewed Keycloak publication already has its valid build signature\n'
    exit 0
fi
# Only the exact observed missing-signature diagnostic permits this reviewed
# exception. The shared transport helper never retries trust-policy failures.
if ! grep -Eq '^(Error: |error during command execution: )?no signatures found[[:space:]]*$' "$TEMP/registry.log" ||
    grep -Evq '^[[:space:]]*$|^(Error: |error during command execution: )?no signatures found[[:space:]]*$' "$TEMP/registry.log"; then
    fail 'Keycloak recovery refuses invalid signatures or incomplete signature verification'
fi
# Recheck under the shared publication lock after the scan, before the only write.
resolve_remote || fail 'reviewed Keycloak registry recheck failed'
[[ "$REGISTRY_DIGEST" == "$REVIEWED_INDEX" ]] || fail 'reviewed Keycloak input tag changed during audit'
remote_operation recovery-sign cosign sign --yes -a "dependency-inputs=$INPUT_HASH" -a dependency-build=true "$IMMUTABLE_REF" || fail 'reviewed Keycloak signing failed'
verify_remote
printf 'dependency-images: recovered reviewed original Keycloak index %s\n' "$IMMUTABLE_REF"
