#!/usr/bin/env bash
# Deterministic recovery regressions; no daemon, registry, signer or credentials.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../../.." && pwd)
FIXTURE="$ROOT/tools/quality/dependency-images/test/recovery"
OWNED=$(mktemp -d /tmp/reviewed-keycloak-recovery.XXXXXX)
cleanup() {
    [[ "$OWNED" == /tmp/reviewed-keycloak-recovery.* && "$(realpath "$OWNED")" == "$OWNED" && ! -L "$OWNED" ]] || exit 1
    rm -rf -- "$OWNED"
}
trap cleanup EXIT
mkdir -p "$OWNED/repo/tools/quality/dependency-images" "$OWNED/repo/tools/quality/image-audit" "$OWNED/repo/deployment/images" "$OWNED/repo/deployment/docker/keycloak-patched" "$OWNED/bin"
cp "$ROOT/tools/quality/dependency-images/"{common,registry,recover-keycloak}.sh "$OWNED/repo/tools/quality/dependency-images/"
cp "$ROOT/tools/quality/image-audit/retry.sh" "$OWNED/repo/tools/quality/image-audit/"
cp "$ROOT/deployment/images/dependencies.tsv" "$OWNED/repo/deployment/images/"
cp "$ROOT/deployment/docker/keycloak-patched/Dockerfile" "$OWNED/repo/deployment/docker/keycloak-patched/"
cp "$ROOT/.dockerignore" "$OWNED/repo/"
for command in gh docker cosign make sleep; do ln -s "$FIXTURE/fake-command.sh" "$OWNED/bin/$command"; done
export PATH="$OWNED/bin:$PATH" TEST_FIXTURE="$FIXTURE" TEST_STATE="$OWNED/state"
script="$OWNED/repo/tools/quality/dependency-images/recover-keycloak.sh"
reset_case() {
    [[ "$TEST_STATE" == "$OWNED/state" && "$(realpath "$OWNED")" == "$OWNED" ]] || exit 1
    rm -rf -- "$TEST_STATE"
    mkdir "$TEST_STATE"
    : >"$TEST_STATE/trace"
    export TEST_CASE=success TEST_PROVENANCE_FILTER='.'
    export GITHUB_ACTIONS=true GITHUB_EVENT_NAME=workflow_dispatch GITHUB_REPOSITORY=Anko59/GeoguessMe
    export GITHUB_REF=refs/heads/dev GITHUB_REF_PROTECTED=true
    export GITHUB_WORKFLOW_REF=Anko59/GeoguessMe/.github/workflows/security.yml@refs/heads/dev
}
fail_test() {
    printf 'recovery regression failed: %s\n' "$*" >&2
    exit 1
}
assert_no_sign() { ! grep -q '^cosign sign ' "$TEST_STATE/trace" || fail_test 'unexpected sign'; }
reject() {
    if bash "$script" "$@" >"$TEST_STATE/output" 2>&1; then fail_test "accepted $TEST_CASE"; fi
    assert_no_sign
    printf 'recovery guard OK: %s\n' "$TEST_CASE"
}
reset_case
bash "$script" >"$TEST_STATE/output" 2>&1 || {
    sed -n '1,120p' "$TEST_STATE/output"
    fail_test 'valid original publication'
}
[[ $(grep -c '^cosign sign ' "$TEST_STATE/trace") == 1 ]] || fail_test 'sign count'
audit_line=$(grep -n '^make .*audit-image-set' "$TEST_STATE/trace" | cut -d: -f1)
sign_line=$(grep -n '^cosign sign ' "$TEST_STATE/trace" | cut -d: -f1)
verify_line=$(grep -n '^cosign verify ' "$TEST_STATE/trace" | tail -1 | cut -d: -f1)
[[ "$audit_line" -lt "$sign_line" && "$sign_line" -lt "$verify_line" ]] || fail_test 'audit/sign/verify ordering'
! grep -Eq 'docker (build |buildx build |tag |image rm )|imagetools create' "$TEST_STATE/trace" || fail_test 'artifact mutation'
[[ $(grep -c '^docker .*Provenance' "$TEST_STATE/trace") == 2 ]] || fail_test 'recovery redundantly refetched provenance'
printf 'recovery exact original single-platform audit/sign/verify order OK\n'
reset_case
export TEST_PROVENANCE_FILTER='{"linux/amd64":.}'
bash "$script" >"$TEST_STATE/output" 2>&1 || fail_test 'explicit AMD64 platform-map compatibility'
[[ $(grep -c '^cosign sign ' "$TEST_STATE/trace") == 1 ]] || fail_test 'mapped provenance sign count'
printf 'recovery explicit AMD64 platform-map compatibility OK\n'
reset_case
touch "$TEST_STATE/signed"
bash "$script" >"$TEST_STATE/output" 2>&1 || fail_test 'already valid original signature'
assert_no_sign
grep -q '^make .*audit-image-set' "$TEST_STATE/trace" || fail_test 'idempotency skipped fresh audit'
printf 'recovery already-valid signature fresh-audit idempotency OK\n'
reset_case
export TEST_CASE=signature-wrapper
bash "$script" >"$TEST_STATE/output" 2>&1 || fail_test 'actual Cosign missing-signature diagnostic'
[[ $(grep -c '^cosign sign ' "$TEST_STATE/trace") == 1 ]] || fail_test 'actual Cosign diagnostic sign count'
printf 'recovery observed Cosign missing-signature diagnostic OK\n'

for field in GITHUB_ACTIONS GITHUB_EVENT_NAME GITHUB_REPOSITORY GITHUB_REF GITHUB_REF_PROTECTED GITHUB_WORKFLOW_REF; do
    reset_case
    export "$field=wrong"
    TEST_CASE=$field reject
    [[ ! -s "$TEST_STATE/trace" ]] || fail_test 'identity guard performed external operation'
done
reset_case
TEST_CASE=artifact-argument reject arbitrary-digest
reset_case
printf '# stale reviewed inputs\n' >>"$OWNED/repo/deployment/docker/keycloak-patched/Dockerfile"
TEST_CASE=stale-inputs reject
[[ ! -s "$TEST_STATE/trace" ]] || fail_test 'stale inputs performed external operation'
cp "$ROOT/deployment/docker/keycloak-patched/Dockerfile" "$OWNED/repo/deployment/docker/keycloak-patched/"
for scenario in api-denied run-origin run-workflow run-branch job-origin job-failure log-denied log-digest log-failure registry-denied registry-missing tag-digest attestation-link multiple-runtime direct-multi-runtime runtime-digest local-inputs local-platform local-base repo-digest; do
    reset_case
    export TEST_CASE=$scenario
    reject
    ! grep -q '^make ' "$TEST_STATE/trace" || fail_test "audit preceded $scenario rejection"
done
for filter in \
    '. + {"linux/amd64":.}' \
    '{SLSA1:.SLSA}' \
    '{"linux/arm64":.}' \
    '{"linux/amd64":null}' \
    '.SLSA=null' \
    '.SLSA=[]' \
    '.SLSA="invalid"' \
    '.SLSA={}' \
    '.SLSA' \
    'null' \
    '[]' \
    '"invalid"' \
    '.SLSA.buildDefinition.buildType="https://untrusted.example/buildkit"' \
    '.SLSA.buildDefinition.externalParameters.request.root.request.args["vcs:revision"]="wrong"' \
    '.SLSA.buildDefinition.externalParameters.request.root.request.args["vcs:source"]="wrong"' \
    '.SLSA.buildDefinition.externalParameters.request.root.request.args["build-arg:DEPENDENCY_INPUTS"]="wrong"' \
    '.SLSA.buildDefinition.externalParameters.request.root.request.args["label:dev.geoguessme.dependency-inputs"]="wrong"' \
    '.SLSA.buildDefinition.externalParameters.request.root.request.args["vcs:localdir:context"]="wrong"' \
    '.SLSA.buildDefinition.externalParameters.request.root.request.args["vcs:localdir:dockerfile"]="wrong"' \
    '.SLSA.buildDefinition.externalParameters.configSource.path="wrong"' \
    '(.SLSA.buildDefinition.resolvedDependencies[] | select(.uri | contains("quay.io/keycloak"))).digest.sha256="wrong"' \
    '.SLSA.runDetails.metadata.buildkit_metadata.vcs.revision="wrong"' \
    '.SLSA.runDetails.metadata.buildkit_metadata.vcs.source="wrong"' \
    '.SLSA.runDetails.metadata.buildkit_metadata.source.infos=[]' \
    '.SLSA.runDetails.metadata.buildkit_metadata.source.infos += [.SLSA.runDetails.metadata.buildkit_metadata.source.infos[0]]' \
    '.SLSA.runDetails.metadata.buildkit_metadata.source.infos[0].data="bm90LXRoZS1yZWNpcGU="' \
    '.SLSA.runDetails.metadata.buildkit_metadata.source.infos[0].data="invalid!"'; do
    reset_case
    export TEST_CASE=provenance-mismatch TEST_PROVENANCE_FILTER=$filter
    reject
    ! grep -q '^make ' "$TEST_STATE/trace" || fail_test 'audit preceded provenance rejection'
done
for scenario in audit-failed invalid-signature signature-denied signature-network signature-mixed signature-empty tag-race sign-failed; do
    reset_case
    export TEST_CASE=$scenario
    if bash "$script" >"$TEST_STATE/output" 2>&1; then fail_test "accepted $scenario"; fi
    if [[ "$scenario" != sign-failed ]]; then assert_no_sign; fi
    printf 'recovery audit/transport/signature guard OK: %s\n' "$scenario"
done
reset_case
export TEST_CASE=final-verify
if bash "$script" >"$TEST_STATE/output" 2>&1; then fail_test 'accepted final verification failure'; fi
[[ $(grep -c '^cosign sign ' "$TEST_STATE/trace") == 1 ]] || fail_test 'final verification did not follow sign'
printf 'recovery final verification failure is surfaced OK\n'
printf 'reviewed Keycloak recovery regressions PASS\n'
