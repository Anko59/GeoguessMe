#!/usr/bin/env bash
# Regression guard for the k6 load profile's signup payload.
#
# Signup enforces a hard 15+ attestation (`age_attested: true`). Every identity
# the load profile creates during setup must send it; otherwise setup never
# authenticates, the whole profile collapses to 100% failures, and only the
# nightly operational gate catches it. This mirrors the reconnect-rehearsal
# guard so the omission fails pull-request CI instead of the nightly run.
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/../../../.." && pwd)
profile="$repo_root/tools/load/k6.js"
failures=0

pass() {
    echo "PASS: $*"
}

fail() {
    echo "FAIL: $*"
    failures=$((failures + 1))
}

# Print the signup request bodies in a k6 profile: everything from each line
# opening a POST to /api/v1/auth/signup through the statement's closing `);`.
signup_bodies() {
    awk '
        /\/api\/v1\/auth\/signup/ { capturing = 1 }
        capturing { print }
        capturing && /\);[[:space:]]*$/ { capturing = 0 }
    ' "$1"
}

# Print "<attested>/<calls>" and succeed only when at least one signup request
# exists and every one of them carries the attestation.
attestation_coverage() {
    local path=$1 bodies calls attested
    bodies=$(signup_bodies "$path")
    calls=$(grep -c '/api/v1/auth/signup' "$path" || true)
    attested=$(printf '%s\n' "$bodies" | grep -c 'age_attested: true' || true)
    printf '%s/%s' "$attested" "$calls"
    [ -n "$bodies" ] && [ "$calls" -gt 0 ] && [ "$attested" -ge "$calls" ]
}

if coverage=$(attestation_coverage "$profile"); then
    pass "load profile attests age on every signup (attested/calls = $coverage)"
else
    fail "load profile signup attestation incomplete (attested/calls = $coverage)"
fi

# Self-tests: the checker must reject the two regressions it exists to catch.
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT
sed 's/age_attested: true,//' "$profile" >"$tmpdir/omitted.js"
sed 's/age_attested: true/age_attested: false/' "$profile" >"$tmpdir/false.js"
for fixture in omitted false; do
    if coverage=$(attestation_coverage "$tmpdir/$fixture.js"); then
        fail "self-test: $fixture attestation was accepted (attested/calls = $coverage)"
    else
        pass "self-test: $fixture attestation rejected (attested/calls = $coverage)"
    fi
done

if [ "$failures" -gt 0 ]; then
    echo "load-harness regression FAILED ($failures failure(s))"
    exit 1
fi

echo "load-harness regression PASSED"
