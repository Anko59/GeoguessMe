#!/usr/bin/env bash
set -euo pipefail; cd "$(dirname "$0")/../.."
if ! command -v caddy >/dev/null; then echo "caddy: skip"; exit 0; fi
fail=0; f="deployment/caddy/Caddyfile"
caddy fmt --diff "$f" || fail=1
caddy validate --config "$f" || fail=1
[ "$fail" -ne 0 ] && { echo "lint-caddy FAILED"; exit 1; }
echo "lint-caddy PASSED"
