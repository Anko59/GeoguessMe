#!/bin/sh
set -eu

repository=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
output=$repository/.local/caddy
certificate=$output/local-cert.pem
key=$output/local-key.pem
root_ca=$output/rootCA.pem

if ! command -v mkcert >/dev/null 2>&1; then
    echo "mkcert is required for local HTTPS; install it and run: mkcert -install" >&2
    exit 1
fi

ca_root=$(mkcert -CAROOT)
if [ ! -r "$ca_root/rootCA.pem" ]; then
    echo "mkcert's local CA is not installed; run: mkcert -install" >&2
    exit 1
fi

mkdir -p "$output"
chmod 700 "$output"

if [ ! -s "$certificate" ] || [ ! -s "$key" ] ||
    ! openssl x509 -in "$certificate" -noout -text 2>/dev/null | grep -Fq 'DNS:auth-dev.geoguessme.com'; then
    mkcert -cert-file "$certificate" -key-file "$key" \
        geoguessme.localhost auth.geoguessme.localhost auth-dev.geoguessme.com
fi

cp "$ca_root/rootCA.pem" "$root_ca"
chmod 600 "$certificate" "$key" "$root_ca"
