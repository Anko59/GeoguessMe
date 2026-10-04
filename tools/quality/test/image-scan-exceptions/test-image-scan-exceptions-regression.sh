#!/usr/bin/env bash
# Regression tests for image-scan-exceptions-check.sh (F-01 gate machinery).
#
# Tests:
#   1. A fully-populated exception record validates (exit 0) and
#      `--emit REF OUT` writes its CVE id and owner comment to the ignorefile.
#   2. A record missing a required field is rejected.
#   3. An exception expiring in the past is rejected.
#   4. An exception expiring more than 30 days out is rejected.
#   5. An unapproved exception (approved: false) is rejected.
#   6. An image/digest mismatch is rejected.
#   7. Legacy append mode preserves exact direct-image exceptions; production
#      inheritance uses package/version-scoped native policies instead.
#   8. Multiple exception files are validated and emitted together.
#   9. The nightly Buildx verification loads local images before image scanning.
#  10. Fixed SOPS libexpat findings are removed by the patched image, never excepted.
#  11. Fixed Alpine PCRE2 findings are removed from shipped images, never excepted.
set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")/../.." && pwd)/image-scan-exceptions-check.sh"
REPO_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
PASS=0
FAIL=0
TMP=""
DIGEST=""

cleanup() {
    if [ -n "$TMP" ]; then
        case "$TMP" in /tmp/tmp.*) rm -rf -- "$TMP" ;; *)
            echo 'unexpected regression temporary path' >&2
            return 1
            ;;
        esac
    fi
}
trap cleanup EXIT

pass() {
    echo "PASS: $*"
    PASS=$((PASS + 1))
}
fail() {
    echo "FAIL: $*"
    FAIL=$((FAIL + 1))
}

# run_validator <yaml-file> [args...] -- run the checker against a fixture file;
# returns its exit code (stdout/stderr suppressed).
run_validator() {
    local file=$1 rc
    shift
    IMAGE_SCAN_EXCEPTIONS="$file" bash "$SCRIPT" "$@" >/dev/null 2>&1 && rc=0 || rc=$?
    return "$rc"
}

# validator_output <yaml-file> [args...] -- echo the checker's combined output.
validator_output() {
    local file=$1
    shift
    IMAGE_SCAN_EXCEPTIONS="$file" bash "$SCRIPT" "$@" 2>&1
}

date_utc_days_from_today() {
    local days=$1
    local epoch=$(($(date -u +%s) + days * 86400))
    date -u -d "@$epoch" +%F 2>/dev/null || date -u -r "$epoch" +%F
}

TMP="$(mktemp -d)"
DIGEST="$(printf 'a%.0s' {1..64})"
IMAGE="postgres:15-alpine@sha256:${DIGEST}"
VALID_EXPIRY="$(date_utc_days_from_today 7)"
PAST_EXPIRY="$(date_utc_days_from_today -1)"
FAR_EXPIRY="$(date_utc_days_from_today 31)"

echo "image-scan exceptions regression tests:"

# ── Fixtures ────────────────────────────────────────────────────────────────
cat >"$TMP/valid.yaml" <<EOF
- id: CVE-2026-00001
  image: $IMAGE
  digest: sha256:$DIGEST
  owner: platform@geoguessme.dev
  reachable: regression fixture rationale
  approved: true
  expires: $VALID_EXPIRY
EOF

cat >"$TMP/missing-owner.yaml" <<EOF
- id: CVE-2026-00002
  image: $IMAGE
  digest: sha256:$DIGEST
  reachable: regression fixture rationale
  approved: true
  expires: $VALID_EXPIRY
EOF

cat >"$TMP/past-expiry.yaml" <<EOF
- id: CVE-2026-00003
  image: $IMAGE
  digest: sha256:$DIGEST
  owner: platform@geoguessme.dev
  reachable: regression fixture rationale
  approved: true
  expires: $PAST_EXPIRY
EOF

cat >"$TMP/far-expiry.yaml" <<EOF
- id: CVE-2026-00004
  image: $IMAGE
  digest: sha256:$DIGEST
  owner: platform@geoguessme.dev
  reachable: regression fixture rationale
  approved: true
  expires: $FAR_EXPIRY
EOF

cat >"$TMP/unapproved.yaml" <<EOF
- id: CVE-2026-00005
  image: $IMAGE
  digest: sha256:$DIGEST
  owner: platform@geoguessme.dev
  reachable: regression fixture rationale
  approved: false
  expires: $VALID_EXPIRY
EOF

cat >"$TMP/digest-mismatch.yaml" <<EOF
- id: CVE-2026-00006
  image: $IMAGE
  digest: sha256:$(printf 'b%.0s' {1..64})
  owner: platform@geoguessme.dev
  reachable: regression fixture rationale
  approved: true
  expires: $VALID_EXPIRY
EOF

cat >"$TMP/valid-second.yaml" <<EOF
- id: CVE-2026-00007
  image: $IMAGE
  digest: sha256:$DIGEST
  owner: platform@geoguessme.dev
  reachable: second regression fixture rationale
  approved: true
  expires: $VALID_EXPIRY
EOF

# ── Test 1: valid record validates and emits its ignorefile entry ───────────
echo "--- Test 1: valid record validates and emits ---"
if run_validator "$TMP/valid.yaml"; then
    pass "valid record validates (exit 0)"
else
    fail "valid record rejected"
fi
if validator_output "$TMP/valid.yaml" | grep -q 'image-scan exceptions OK (1 records)'; then
    pass "validation reports 1 record OK"
else
    fail "validation summary missing"
fi

ignore="$TMP/ignore.trivy"
if IMAGE_SCAN_EXCEPTIONS="$TMP/valid.yaml" bash "$SCRIPT" --emit "$IMAGE" "$ignore" >/dev/null 2>&1; then
    pass "emit mode exits 0"
else
    fail "emit mode failed"
fi
if [ -f "$ignore" ] && grep -qx 'CVE-2026-00001' "$ignore"; then
    pass "emit writes CVE-2026-00001 to ignorefile"
else
    fail "ignorefile missing CVE-2026-00001"
fi
if [ -f "$ignore" ] && grep -q '^# exception CVE-2026-00001 (owner platform@geoguessme.dev' "$ignore"; then
    pass "ignorefile entry carries owner comment"
else
    fail "ignorefile owner comment missing"
fi

# ── Test 2: missing required field ──────────────────────────────────────────
echo "--- Test 2: missing required field ---"
if run_validator "$TMP/missing-owner.yaml"; then
    fail "record missing owner accepted"
else
    pass "record missing owner rejected"
fi
out=$(validator_output "$TMP/missing-owner.yaml") || true
if printf '%s\n' "$out" | grep -q '^ERROR:'; then
    pass "reports a validation error"
else
    fail "missing-field error message absent"
fi

# ── Test 3: expired exception ───────────────────────────────────────────────
echo "--- Test 3: past expiry ---"
if run_validator "$TMP/past-expiry.yaml"; then
    fail "past-expiry exception accepted"
else
    pass "past-expiry exception rejected"
fi
out=$(validator_output "$TMP/past-expiry.yaml") || true
if printf '%s\n' "$out" | grep -q 'expires in the past'; then
    pass "reports past expiry"
else
    fail "past-expiry error message absent"
fi

# ── Test 4: expiry more than 30 days out ────────────────────────────────────
echo "--- Test 4: expiry beyond 30 days ---"
if run_validator "$TMP/far-expiry.yaml"; then
    fail "far-expiry exception accepted"
else
    pass "far-expiry exception rejected"
fi
out=$(validator_output "$TMP/far-expiry.yaml") || true
if printf '%s\n' "$out" | grep -q 'more than 30 days out'; then
    pass "reports far expiry"
else
    fail "far-expiry error message absent"
fi

# ── Test 5: unapproved exception ────────────────────────────────────────────
echo "--- Test 5: approved must be true ---"
if run_validator "$TMP/unapproved.yaml"; then
    fail "unapproved exception accepted"
else
    pass "unapproved exception rejected"
fi
out=$(validator_output "$TMP/unapproved.yaml") || true
if printf '%s\n' "$out" | grep -q 'not approved'; then
    pass "reports not-approved"
else
    fail "not-approved error message absent"
fi

# ── Test 6: image reference and digest field must agree ────────────────────
echo "--- Test 6: image/digest mismatch ---"
if run_validator "$TMP/digest-mismatch.yaml"; then
    fail "image/digest mismatch accepted"
else
    pass "image/digest mismatch rejected"
fi
out=$(validator_output "$TMP/digest-mismatch.yaml") || true
if printf '%s\n' "$out" | grep -q 'image digest does not match'; then
    pass "reports image/digest mismatch"
else
    fail "image/digest mismatch error message absent"
fi

# ── Test 7: legacy append of exact direct-image exceptions ─────────────────
echo "--- Test 7: append mode preserves existing entries ---"
printf '%s\n' 'CVE-2026-DIRECT' >"$ignore"
if IMAGE_SCAN_EXCEPTIONS="$TMP/valid.yaml" bash "$SCRIPT" --append "$IMAGE" "$ignore" >/dev/null 2>&1; then
    pass "append mode exits 0"
else
    fail "append mode failed"
fi
if grep -qx 'CVE-2026-DIRECT' "$ignore" && grep -qx 'CVE-2026-00001' "$ignore"; then
    pass "append preserves exact direct entries"
else
    fail "append did not preserve and extend ignorefile"
fi

# ── Test 8: validate and emit multiple exception files together ────────────
echo "--- Test 8: multiple exception files ---"
multiple_ignore="$TMP/multiple-ignore.trivy"
if IMAGE_SCAN_EXCEPTIONS="$TMP/valid.yaml $TMP/valid-second.yaml" bash "$SCRIPT" --emit "$IMAGE" "$multiple_ignore" >/dev/null 2>&1; then
    pass "multiple exception files validate and emit"
else
    fail "multiple exception files rejected"
fi
if grep -qx 'CVE-2026-00001' "$multiple_ignore" && grep -qx 'CVE-2026-00007' "$multiple_ignore"; then
    pass "emit includes records from every exception file"
else
    fail "emit omitted a record from multiple exception files"
fi

# ── Test 9: nightly Buildx verification loads images for audit-images ────────
echo "--- Test 9: nightly verification loads Buildx images before scanning ---"
nightly_build_flags="$(grep -E '^[[:space:]]*DOCKER_BUILD_FLAGS=' "$REPO_ROOT/.github/workflows/nightly.yml" || true)"
if [[ "$nightly_build_flags" == *"--load"* && "$nightly_build_flags" == *"--cache-from type=local"* && "$nightly_build_flags" == *"--cache-to type=local"* ]]; then
    pass "nightly Buildx flags load locally audited images while retaining cache"
else
    fail "nightly Buildx flags must load local images before audit-images"
fi

# ── Test 10: SOPS libexpat findings are fixed, not excepted ──────────────────
echo "--- Test 10: SOPS libexpat CVEs are not allowlisted ---"
sops_exceptions="$REPO_ROOT/tools/quality/image-scan-exceptions-sops.yaml"
sops_dockerfile="$REPO_ROOT/deployment/docker/sops-tools/Dockerfile"
libexpat_cves=(
    CVE-2024-28757 CVE-2025-59375 CVE-2026-25210 CVE-2026-45186
    CVE-2026-66046 CVE-2026-93990 CVE-2026-56408 CVE-2026-76957
)
for cve in "${libexpat_cves[@]}"; do
    if grep -Fq "$cve" "$sops_exceptions"; then
        fail "$cve must be remediated, not excepted"
    else
        pass "$cve is absent from the SOPS exception list"
    fi
done
if grep -Fq 'libexpat1=2.5.0-1+deb12u4' "$sops_dockerfile"; then
    pass "SOPS derivative pins the fixed Debian libexpat package"
else
    fail "SOPS derivative does not pin the scanner-reported libexpat fix"
fi

# ── Test 11: Alpine PCRE2 findings are fixed, not excepted ───────────────────
echo "--- Test 11: Alpine PCRE2 CVE is not allowlisted ---"
pcre2_cve=CVE-2026-103111
exception_files=(
    "$REPO_ROOT/tools/quality/image-scan-exceptions.yaml"
    "$REPO_ROOT/tools/quality/image-scan-exceptions-keycloak.yaml"
    "$REPO_ROOT/tools/quality/image-scan-exceptions-oauth2-proxy.yaml"
    "$REPO_ROOT/tools/quality/image-scan-exceptions-cloudflared.yaml"
    "$REPO_ROOT/tools/quality/image-scan-exceptions-sops.yaml"
)
for exception_file in "${exception_files[@]}"; do
    if grep -Fq "$pcre2_cve" "$exception_file"; then
        fail "$pcre2_cve must be remediated, not excepted"
    else
        pass "$pcre2_cve is absent from $(basename "$exception_file")"
    fi
done
for dockerfile in \
    "$REPO_ROOT/deployment/docker/backend.Dockerfile" \
    "$REPO_ROOT/deployment/docker/security/caddy-runtime.Dockerfile" \
    "$REPO_ROOT/deployment/docker/restic-tools.Dockerfile" \
    "$REPO_ROOT/deployment/docker/socket-proxy-tools/Dockerfile"; do
    if grep -Fq "pcre2=10.49-r0" "$dockerfile" &&
        grep -Fq "apk info -v | grep -Fxq 'pcre2-10.49-r0'" "$dockerfile"; then
        pass "$(basename "$dockerfile") pins and asserts fixed PCRE2"
    else
        fail "$(basename "$dockerfile") must pin and assert fixed PCRE2"
    fi
done
if grep -Fq 'SOCKET_PROXY_IMAGE' "$REPO_ROOT/tools/make/dependency-images.mk" &&
    grep -Fq 'image-audit/audit.sh' "$REPO_ROOT/tools/make/dependency-images.mk"; then
    pass 'the exact socket-proxy derivative remains in the scan-only image audit'
else
    fail 'the socket-proxy derivative is not included in the scan-only gate'
fi

# ── Test 12: immutable matching and scoped inheritance ─────────────────────
echo '--- Test 12: strict digest matching and native package/version scopes ---'
sed 's|postgres:15-alpine|geoguessme/fixture:local|' "$TMP/valid.yaml" >"$TMP/local.yaml"
new_ref="geoguessme/fixture:local@sha256:$(printf 'b%.0s' {1..64})"
if run_validator "$TMP/local.yaml" --emit-policy "$new_ref" "$TMP/policy.rego" &&
    ! grep -q 'input.VulnerabilityID' "$TMP/policy.rego"; then
    pass 'same geoguessme image name cannot authorize a different digest'
else
    fail 'name-only digest exception bypass remains'
fi
if run_validator "$TMP/valid.yaml" --emit-policy "$IMAGE" "$TMP/policy.rego" &&
    grep -Fq 'input.VulnerabilityID == "CVE-2026-00001"' "$TMP/policy.rego"; then
    pass 'exact final image generates a native policy'
else
    fail 'exact final native policy emission failed'
fi
if run_validator "$TMP/valid.yaml" --inherit-policy "$IMAGE" "$TMP/policy.rego"; then
    fail 'unscoped base exception was inherited'
else
    pass 'unscoped base exception cannot be inherited'
fi
sed '/  owner:/i\  package: pcre2\n  installed_version: 10.48-r0' "$TMP/valid.yaml" >"$TMP/scoped.yaml"
if run_validator "$TMP/scoped.yaml" --emit-policy "$IMAGE" "$TMP/policy.rego" &&
    run_validator "$TMP/scoped.yaml" --inherit-policy "$IMAGE" "$TMP/policy.rego" &&
    grep -Fq 'input.PkgName == "pcre2"' "$TMP/policy.rego" &&
    grep -Fq 'input.InstalledVersion == "10.48-r0"' "$TMP/policy.rego"; then
    pass 'native inherited rules require exact package and installed version'
else
    fail 'scoped base policy emission failed'
fi
if run_validator "$TMP/scoped.yaml" --emit "$IMAGE" "$ignore"; then
    fail 'scoped exception leaked into global legacy CVE ignorefile'
else
    pass 'scoped exceptions cannot become legacy CVE-wide ignores'
fi
sed '/  installed_version:/d' "$TMP/scoped.yaml" >"$TMP/partial-scope.yaml"
if run_validator "$TMP/partial-scope.yaml"; then fail 'partial package scope accepted'; else pass 'partial package scope rejected'; fi
sed 's/  package: pcre2/  package:/' "$TMP/scoped.yaml" >"$TMP/empty-scope.yaml"
if run_validator "$TMP/empty-scope.yaml"; then fail 'empty package scope accepted'; else pass 'empty package scope rejected'; fi
sed 's/  package: pcre2/  package: bad"injection/' "$TMP/scoped.yaml" >"$TMP/unsafe-scope.yaml"
if run_validator "$TMP/unsafe-scope.yaml"; then fail 'unsafe native-policy string accepted'; else pass 'unsafe native-policy string rejected'; fi
printf '  owner: duplicate-owner\n' >>"$TMP/valid-second.yaml"
if run_validator "$TMP/valid-second.yaml"; then fail 'duplicate approval field accepted'; else pass 'duplicate approval field rejected'; fi
sed 's/expires: .*/expires: 2026-02-30/' "$TMP/valid.yaml" >"$TMP/calendar.yaml"
if run_validator "$TMP/calendar.yaml"; then fail 'invalid calendar expiry accepted'; else pass 'invalid calendar expiry rejected'; fi

# ── Summary ─────────────────────────────────────────────────────────────────
echo ""
if [ "$FAIL" -eq 0 ]; then
    echo "image-scan exceptions regression tests PASSED ($PASS checks)"
else
    echo "image-scan exceptions regression tests FAILED ($FAIL failure(s))"
    exit 1
fi
