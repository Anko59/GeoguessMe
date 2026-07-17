#!/usr/bin/env sh
# Smoke-test a running GeoGuessMe gateway: liveness, readiness, an authenticated
# 401 on a protected route, and the WebSocket ticket endpoint shape.
# Usage: smoke-test.sh [BASE_URL]   (default http://localhost)
set -eu

base="${1:-http://localhost}"
case "$base" in
    http://localhost*) container_base="http://host.docker.internal${base#http://localhost}" ;;
    https://localhost*) container_base="https://host.docker.internal${base#https://localhost}" ;;
    http://127.0.0.1*) container_base="http://host.docker.internal${base#http://127.0.0.1}" ;;
    https://127.0.0.1*) container_base="https://host.docker.internal${base#https://127.0.0.1}" ;;
    *) container_base="$base" ;;
esac

docker compose -p geoguessme-tools -f deployment/compose.tools.yaml --project-directory "$(pwd)" \
    run --rm --no-deps go-tools bash -c '
        set -eu
        base="$1"
        fail=0
        check() {
            desc="$1"
            expected="$2"
            url="$3"
            code=$(curl -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null || echo 000)
            if [ "$code" = "$expected" ]; then
                echo "ok   $desc ($code)"
            else
                echo "FAIL $desc (got $code, want $expected)"
                fail=1
            fi
        }
        check "liveness" 200 "$base/health/live"
        check "readiness" 200 "$base/health/ready"
        check "protected route rejects anonymous" 401 "$base/api/v1/user/groups"
        check "websocket ticket requires auth" 401 "$base/api/v1/ws/ticket?group_id=00000000-0000-0000-0000-000000000000"
        if [ "$fail" -ne 0 ]; then
            echo "smoke test failed"
            exit 1
        fi
        echo "smoke test passed"
    ' -- "$container_base"
