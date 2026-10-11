#!/usr/bin/env bash
# Sourced by the lifecycle regression after its private fake environment exists.
: "${TRACE:?}" "${TEST_DIGEST:?}" "${ENV_FILE:?}" "${FAKE_STATE:?}"
for provenance in legacy legacy-nested mapped-modern modern direct-legacy; do
    new_case
    export FAKE_REGISTRY=missing FAKE_PROVENANCE=$provenance
    run_ok publish sops
    build_line=$(grep -n '^docker buildx build ' "$TRACE" | cut -d: -f1)
    proof_line=$(grep -m1 -n '^docker buildx imagetools inspect .*\.Provenance' "$TRACE" | cut -d: -f1)
    audit_line=$(grep -n '^make .*audit-image-set ' "$TRACE" | cut -d: -f1)
    sign_line=$(grep -n '^cosign sign ' "$TRACE" | cut -d: -f1)
    verify_line=$(grep -m1 -n '^cosign verify ' "$TRACE" | cut -d: -f1)
    assert test "$build_line" -lt "$proof_line"
    assert test "$proof_line" -lt "$audit_line"
    assert test "$audit_line" -lt "$sign_line"
    assert test "$sign_line" -lt "$verify_line"
    assert test "$(grep -c '^docker buildx build ' "$TRACE")" = 1
    assert test "$(grep -c '^cosign sign ' "$TRACE")" = 1
    assert test "$(grep -c '^make .*audit-image-set ' "$TRACE")" = 1
    assert grep -q "@$TEST_DIGEST" "$ENV_FILE"
    pass "$provenance publication preserves build, provenance, audit, sign and verification ordering"
done
for provenance in unknown-url modern-suffix root-modern empty malformed definition-array slsa-array type-array missing-definition malformed-json direct-multi-runtime ambiguous ambiguous-null root-null root-string mapped-null mapped-nonobject wrong-arch slsa1 arbitrary-root direct-extra; do
    new_case
    export FAKE_REGISTRY=missing FAKE_PROVENANCE=$provenance
    run_fail publish sops
    assert test "$(grep -c '^docker buildx build ' "$TRACE")" = 1
    case "$provenance" in
        slsa-array | malformed-json | direct-multi-runtime | ambiguous | ambiguous-null | root-null | root-string | mapped-null | mapped-nonobject | wrong-arch | slsa1 | arbitrary-root | direct-extra) diagnostic='invalid BuildKit SLSA projection' ;;
        *) diagnostic='missing BuildKit SLSA provenance' ;;
    esac
    assert grep -Fq "$diagnostic" "$FAKE_STATE/log"
    assert test ! -f "$FAKE_STATE/scanned"
    assert test ! -f "$FAKE_STATE/signed"
    assert test ! -f "$ENV_FILE"
    assert test "$(grep -Ec '^make |^cosign sign ' "$TRACE")" = 0
    pass "$provenance provenance fails before auditing, signing or output publication"
done
for guard in FAKE_LABEL FAKE_BASE FAKE_ARCH FAKE_DIGEST; do
    new_case
    export FAKE_REGISTRY=missing FAKE_PROVENANCE=modern
    if [[ "$guard" == FAKE_ARCH ]]; then export "$guard=arm64"; else export "$guard=wrong"; fi
    run_fail publish sops
    assert test ! -f "$FAKE_STATE/scanned"
    assert test ! -f "$FAKE_STATE/signed"
    assert test ! -f "$ENV_FILE"
    assert test "$(grep -Ec '^make |^cosign sign ' "$TRACE")" = 0
    pass "modern provenance retains $guard rejection before audit/sign"
done
new_case
export FAKE_PROVENANCE=modern FAKE_SIGNATURE=bad
run_fail publish sops
assert no_build_or_sign
assert test "$(grep -Ec '^make |\.Provenance|^docker pull ' "$TRACE")" = 0
assert test ! -f "$FAKE_STATE/scanned"
assert test ! -f "$FAKE_STATE/signed"
assert test ! -f "$ENV_FILE"
pass 'existing unsigned modern artifact remains refused before build, provenance, scan or signing'
