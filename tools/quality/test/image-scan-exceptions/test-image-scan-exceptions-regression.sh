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
#   7. Append mode preserves direct-image exceptions while adding base-image
#      exceptions for a derived image.
set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")/../.." && pwd)/image-scan-exceptions-check.sh"
PASS=0
FAIL=0
TMP=""
DIGEST=""

cleanup() {
    if [ -n "$TMP" ]; then
        rm -rf "$TMP" 2>/dev/null || true
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

TMP="$(mktemp -d)"
DIGEST="$(printf 'a%.0s' {1..64})"
IMAGE="postgres:15-alpine@sha256:${DIGEST}"
VALID_EXPIRY="$(date -u -d "+7 days" +%F)"
PAST_EXPIRY="$(date -u -d "-1 day" +%F)"
FAR_EXPIRY="$(date -u -d "+31 days" +%F)"

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

# ── Test 7: append base-image exceptions to a derived-image file ────────────
echo "--- Test 7: append mode preserves existing entries ---"
printf '%s\n' 'CVE-2026-DIRECT' >"$ignore"
if IMAGE_SCAN_EXCEPTIONS="$TMP/valid.yaml" bash "$SCRIPT" --append "$IMAGE" "$ignore" >/dev/null 2>&1; then
    pass "append mode exits 0"
else
    fail "append mode failed"
fi
if grep -qx 'CVE-2026-DIRECT' "$ignore" && grep -qx 'CVE-2026-00001' "$ignore"; then
    pass "append preserves direct entry and adds base entry"
else
    fail "append did not preserve and extend ignorefile"
fi

# ── Summary ─────────────────────────────────────────────────────────────────
echo ""
if [ "$FAIL" -eq 0 ]; then
    echo "image-scan exceptions regression tests PASSED ($PASS checks)"
else
    echo "image-scan exceptions regression tests FAILED ($FAIL failure(s))"
    exit 1
fi
