#!/usr/bin/env bash
set -euo pipefail

CHECKER="$(cd "$(dirname "$0")/.." && pwd)/coverage-threshold"
PASS=0
FAIL=0

run_check() {
    local expected=$1
    local name=$2
    local input=$3
    local status=0
    echo "$input" | "$CHECKER" >/dev/null 2>&1 || status=$?
    if [ "$expected" = pass ] && [ "$status" -eq 0 ]; then
        PASS=$((PASS + 1))
    elif [ "$expected" = fail ] && [ "$status" -ne 0 ]; then
        PASS=$((PASS + 1))
    else
        echo "FAIL: $name"
        FAIL=$((FAIL + 1))
    fi
}

echo "coverage-threshold regression tests:"

# ── All thresholds met ───────────────────────────────────────────────────────
run_check pass all-above-thresholds "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/config	0.019s	coverage: 96.0% of statements
ok  	geoguessme/internal/database	0.015s	coverage: 64.5% of statements
ok  	geoguessme/internal/email	0.137s	coverage: 72.7% of statements
ok  	geoguessme/internal/game	0.006s	coverage: 100.0% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
total:							(statements)			75.0%
EOF
)"

# ── Global below threshold ───────────────────────────────────────────────────
run_check fail global-below-threshold "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
total:							(statements)			55.0%
EOF
)"

# ── Required package below floor ─────────────────────────────────────────────
run_check fail handler-below-floor "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 55.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
total:							(statements)			75.0%
EOF
)"

run_check fail auth-below-floor "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 65.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
total:							(statements)			75.0%
EOF
)"

run_check fail middleware-below-floor "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 69.9% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
total:							(statements)			75.0%
EOF
)"

run_check fail repository-below-floor "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 60.0% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
total:							(statements)			75.0%
EOF
)"

run_check fail storage-below-floor "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 50.0% of statements
total:							(statements)			75.0%
EOF
)"

run_check fail chat-below-floor "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 45.0% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
total:							(statements)			75.0%
EOF
)"

run_check fail media-below-floor "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 40.0% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
total:							(statements)			75.0%
EOF
)"

# ── Non-required package below floor (should still pass) ─────────────────────
run_check pass non-required-below-floor "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/config	0.019s	coverage: 30.0% of statements
ok  	geoguessme/internal/database	0.015s	coverage: 40.0% of statements
ok  	geoguessme/internal/email	0.137s	coverage: 50.0% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
total:							(statements)			75.0%
EOF
)"

# ── Exact boundary: 70.0% ────────────────────────────────────────────────────
run_check pass exact-floor-boundary "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 70.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 70.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 70.0% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.0% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 70.0% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 70.0% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 70.0% of statements
total:							(statements)			70.0%
EOF
)"

# ── Missing required package ─────────────────────────────────────────────────
run_check fail missing-required-package "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
total:							(statements)			75.0%
EOF
)"

# ── Cached test output format ────────────────────────────────────────────────
run_check pass cached-output-format "$(
    cat <<'EOF'
ok  	geoguessme/handlers	(cached)	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	(cached)	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	(cached)	coverage: 82.6% of statements
ok  	geoguessme/internal/media	(cached)	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	(cached)	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	(cached)	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	(cached)	coverage: 77.2% of statements
total:							(statements)			75.0%
EOF
)"

# ── Realistic mixed output (with function-level lines from cover -func) ──────
run_check pass mixed-func-lines "$(
    cat <<'EOF'
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
geoguessme/handlers/auth.go:47:				userResponse			100.0%
geoguessme/handlers/auth.go:55:				writeSession			66.7%
geoguessme/internal/auth/auth.go:32:			Init				100.0%
geoguessme/internal/chat/client.go:36:			readPump			86.4%
total:							(statements)			75.0%
EOF
)"

# ── No-test-files package line should not cause false match ──────────────────
run_check pass no-test-files-ignored "$(
    cat <<'EOF'
?   	geoguessme/internal/models	[no test files]
ok  	geoguessme/handlers	0.048s	coverage: 75.0% of statements
ok  	geoguessme/internal/auth	0.219s	coverage: 72.0% of statements
ok  	geoguessme/internal/chat	0.035s	coverage: 82.6% of statements
ok  	geoguessme/internal/media	0.015s	coverage: 70.4% of statements
ok  	geoguessme/internal/middleware	0.032s	coverage: 79.6% of statements
ok  	geoguessme/internal/repository	0.018s	coverage: 71.1% of statements
ok  	geoguessme/internal/storage	0.027s	coverage: 77.2% of statements
total:							(statements)			75.0%
EOF
)"

echo "results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
