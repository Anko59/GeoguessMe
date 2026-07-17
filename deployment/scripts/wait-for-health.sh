#!/usr/bin/env sh
# Wait until the GeoGuessMe gateway reports readiness or time out.
# Usage: wait-for-health.sh [BASE_URL] [TIMEOUT_SECONDS]
set -eu

base="${1:-http://localhost}"
timeout="${2:-120}"
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
        timeout="$2"
        deadline=$((SECONDS + timeout))
        while [ "$SECONDS" -lt "$deadline" ]; do
            code=$(curl -s -o /dev/null -w "%{http_code}" "$base/health/ready" 2>/dev/null || echo 000)
            if [ "$code" = "200" ]; then
                echo "ready: $base"
                exit 0
            fi
            sleep 2
        done
        echo "timed out waiting for $base/health/ready" >&2
        exit 1
    ' -- "$container_base" "$timeout"
