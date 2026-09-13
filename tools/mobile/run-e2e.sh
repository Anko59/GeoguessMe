#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
compose=(docker compose -p geoguessme-mobile-e2e -f "$repo/deployment/compose.test.yaml" --project-directory "$repo")
web_port=${GEOGUESSME_MOBILE_WEB_PORT:-18081}
mailpit_port=${GEOGUESSME_MOBILE_MAILPIT_PORT:-18026}
artifact_dir="$repo/.local/mobile/artifacts"
environment_file="$repo/.local/mobile/e2e.env"

mkdir -p "$artifact_dir"
find "$artifact_dir" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +

cleanup() {
    "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

export GEOGUESSME_TEST_WEB_PORT=$web_port
export GEOGUESSME_MOBILE_WEB_PORT=$web_port
export GEOGUESSME_TEST_MAILPIT_PORT=$mailpit_port
export GEOGUESSME_TEST_PUBLIC_URL="http://localhost:$web_port"
export GEOGUESSME_TEST_ALLOWED_ORIGINS="http://localhost:$web_port"

"${compose[@]}" up -d --build --wait
"$repo/tools/mobile/seed-e2e.sh" "http://localhost:$web_port" "$environment_file"

make -C "$repo" mobile-build \
    CAPACITOR_SERVER_URL="http://localhost:$web_port" \
    MOBILE_API_ORIGIN= MOBILE_WEB_ORIGIN="http://localhost:$web_port"

set -a
# Generated test-only values are intentionally sourced from the ignored file.
# shellcheck disable=SC1090
source "$environment_file"
set +a

if ! make -C "$repo" mobile-test; then
    echo "Mobile E2E failed. Diagnostics: $artifact_dir" >&2
    exit 1
fi
