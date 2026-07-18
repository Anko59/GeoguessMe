#!/usr/bin/env bash
# Self-test verifying the go-tools / go-security image split:
#   1. go-tools (lightweight) has format/lint/test tools but no heavy deps
#   2. go-security (heavy) has security/ops tools (govulncheck, psql, CGO)
#   3. Makefile targets reference the correct images
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$REPO"
COMPOSE_TOOLS="docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory ."

failures=0

pass() { echo "PASS: $*"; }
fail() {
    echo "FAIL: $*"
    failures=$((failures + 1))
}

check_present() {
    local image="$1" tool="$2" desc="$3"
    if $COMPOSE_TOOLS run --rm --no-deps "$image" sh -c "$tool" >/dev/null 2>&1; then
        pass "$desc"
    else
        fail "$desc"
    fi
}

check_absent() {
    local image="$1" tool="$2" desc="$3"
    if $COMPOSE_TOOLS run --rm --no-deps "$image" sh -c "command -v $tool" >/dev/null 2>&1; then
        fail "$desc"
    else
        pass "$desc"
    fi
}

check_env() {
    local image="$1" var="$2" expected="$3" desc="$4"
    local actual
    actual=$($COMPOSE_TOOLS run --rm --no-deps "$image" sh -c "go env $var" 2>/dev/null)
    if [ "$actual" = "$expected" ]; then
        pass "$desc"
    else
        fail "$desc (got $actual, expected $expected)"
    fi
}

check_makefile_ref() {
    local pattern="$1" desc="$2"
    if grep -q "$pattern" Makefile; then
        pass "$desc"
    else
        fail "$desc"
    fi
}

check_makefile_no_ref() {
    local pattern="$1" desc="$2"
    if grep "$pattern" Makefile >/dev/null 2>&1; then
        fail "$desc"
    else
        pass "$desc"
    fi
}

# ── go-tools lightweight assertions ──────────────────────────────────────────
echo "--- go-tools (lightweight) assertions ---"

check_present "go-tools" "go version" "go-tools has go"
check_present "go-tools" "goimports </dev/null >/dev/null" "go-tools has goimports"
check_present "go-tools" "golangci-lint version" "go-tools has golangci-lint"
check_present "go-tools" "curl --version" "go-tools has curl"

check_absent "go-tools" "govulncheck" "go-tools correctly missing govulncheck"
check_absent "go-tools" "psql" "go-tools correctly missing psql"
check_absent "go-tools" "gcc" "go-tools correctly missing gcc (no build-base)"

check_env "go-tools" "CGO_ENABLED" "0" "go-tools CGO_ENABLED=0"

# ── go-security heavy assertions ─────────────────────────────────────────────
echo "--- go-security (heavy) assertions ---"

check_present "go-security" "go version" "go-security has go"
check_present "go-security" "govulncheck -version" "go-security has govulncheck"
check_present "go-security" "psql --version" "go-security has psql"
check_present "go-security" "gcc --version" "go-security has gcc (build-base)"
check_present "go-security" "pg_dump --version" "go-security has pg_dump"

check_env "go-security" "CGO_ENABLED" "1" "go-security CGO_ENABLED=1"

# ── Make target image selection assertions ───────────────────────────────────
echo "--- Make target image selection ---"

check_makefile_ref "go-tools sh.*gofmt" "format-check references go-tools"
check_makefile_ref "go-tools sh.*goimports" "format-check goimports references go-tools"
check_makefile_ref "go-tools.*golangci-lint" "lint-go references go-tools"
check_makefile_ref "go-security.*go test -race" "test-race references go-security"
check_makefile_ref "go-security.*govulncheck" "audit references go-security"
check_makefile_ref "go-security.*backup-postgres" "db-backup references go-security"
check_makefile_no_ref "go-tools.*govulncheck" "no go-tools targets reference govulncheck"

# ── Summary ──────────────────────────────────────────────────────────────────
if [ "$failures" -gt 0 ]; then
    echo "tool-image-split self-test FAILED (${failures} failure(s))"
    exit 1
fi
echo "tool-image-split self-test PASSED"
