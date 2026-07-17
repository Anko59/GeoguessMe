#!/usr/bin/env sh
# Wait until the GeoGuessMe gateway reports readiness or time out.
# Usage: wait-for-health.sh [BASE_URL] [TIMEOUT_SECONDS]
set -eu

base="${1:-http://localhost}"
timeout="${2:-120}"
deadline=$(( $(date +%s) + timeout ))

while [ "$(date +%s)" -lt "$deadline" ]; do
    code=$(curl -s -o /dev/null -w '%{http_code}' "$base/health/ready" 2>/dev/null || echo 000)
    if [ "$code" = "200" ]; then
        echo "ready: $base"
        exit 0
    fi
    sleep 2
done

echo "timed out waiting for $base/health/ready" >&2
exit 1
