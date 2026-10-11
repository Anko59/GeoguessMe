#!/usr/bin/env bash
# CI runner installation only. Hosted provisioning consumes the same reviewed
# release checksum through Terraform; this does not update existing live hosts.
set -euo pipefail
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd)
version=$(jq -er '.cloudflared.version' "$ROOT/deployment/images/host-tools.json")
digest=$(jq -er '.cloudflared.debSha256' "$ROOT/deployment/images/host-tools.json")
[[ "$version" =~ ^[0-9]{4}\.[0-9]+\.[0-9]+$ && "$digest" =~ ^[0-9a-f]{64}$ ]] || exit 1
[[ "$(dpkg --print-architecture)" == amd64 ]] || exit 1
temporary=$(mktemp -d)
cleanup() {
    [[ -d "$temporary" && ! -L "$temporary" && "$temporary" == /tmp/* ]] || return
    rm -rf -- "$temporary"
}
trap cleanup EXIT
bash "$ROOT/tools/quality/image-audit/retry.sh" 'Cloudflared package download' "$temporary/download.log" \
    curl --fail --silent --show-error --location --max-time 120 \
    "https://github.com/cloudflare/cloudflared/releases/download/$version/cloudflared-linux-amd64.deb" \
    --output "$temporary/cloudflared.deb"
printf '%s  %s\n' "$digest" "$temporary/cloudflared.deb" | sha256sum --check
sudo dpkg -i "$temporary/cloudflared.deb"
