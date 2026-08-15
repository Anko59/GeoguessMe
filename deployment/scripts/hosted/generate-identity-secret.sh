#!/bin/sh
set -eu

: "${GOOGLE_OAUTH_CLIENT_ID:?GOOGLE_OAUTH_CLIENT_ID is required}"
: "${GOOGLE_OAUTH_CLIENT_SECRET:?GOOGLE_OAUTH_CLIENT_SECRET is required}"
: "${GITHUB_OAUTH_CLIENT_ID:?GITHUB_OAUTH_CLIENT_ID is required}"
: "${GITHUB_OAUTH_CLIENT_SECRET:?GITHUB_OAUTH_CLIENT_SECRET is required}"
: "${APPLE_OAUTH_CLIENT_ID:?APPLE_OAUTH_CLIENT_ID is required}"
: "${APPLE_OAUTH_CLIENT_SECRET:?APPLE_OAUTH_CLIENT_SECRET is required}"
: "${PRODUCTION_OIDC_CLIENT_SECRET:?PRODUCTION_OIDC_CLIENT_SECRET is required}"
: "${DEV_OIDC_CLIENT_SECRET:?DEV_OIDC_CLIENT_SECRET is required}"

random_hex() {
    bytes=$1
    od -An -N "$bytes" -tx1 /dev/urandom | tr -d ' \n'
}

db_password=$(random_hex 32)
admin_password=$(random_hex 32)

while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
        POSTGRES_PASSWORD=*) printf 'POSTGRES_PASSWORD=%s\n' "$db_password" ;;
        KC_DB_PASSWORD=*) printf 'KC_DB_PASSWORD=%s\n' "$db_password" ;;
        KC_BOOTSTRAP_ADMIN_PASSWORD=*) printf 'KC_BOOTSTRAP_ADMIN_PASSWORD=%s\n' "$admin_password" ;;
        GEOGUESSME_PRODUCTION_OIDC_CLIENT_SECRET=*)
            printf 'GEOGUESSME_PRODUCTION_OIDC_CLIENT_SECRET=%s\n' "$PRODUCTION_OIDC_CLIENT_SECRET"
            ;;
        GEOGUESSME_DEV_OIDC_CLIENT_SECRET=*)
            printf 'GEOGUESSME_DEV_OIDC_CLIENT_SECRET=%s\n' "$DEV_OIDC_CLIENT_SECRET"
            ;;
        GEOGUESSME_GOOGLE_CLIENT_ID=*) printf 'GEOGUESSME_GOOGLE_CLIENT_ID=%s\n' "$GOOGLE_OAUTH_CLIENT_ID" ;;
        GEOGUESSME_GOOGLE_CLIENT_SECRET=*) printf 'GEOGUESSME_GOOGLE_CLIENT_SECRET=%s\n' "$GOOGLE_OAUTH_CLIENT_SECRET" ;;
        GEOGUESSME_GITHUB_CLIENT_ID=*) printf 'GEOGUESSME_GITHUB_CLIENT_ID=%s\n' "$GITHUB_OAUTH_CLIENT_ID" ;;
        GEOGUESSME_GITHUB_CLIENT_SECRET=*) printf 'GEOGUESSME_GITHUB_CLIENT_SECRET=%s\n' "$GITHUB_OAUTH_CLIENT_SECRET" ;;
        GEOGUESSME_APPLE_CLIENT_ID=*) printf 'GEOGUESSME_APPLE_CLIENT_ID=%s\n' "$APPLE_OAUTH_CLIENT_ID" ;;
        GEOGUESSME_APPLE_CLIENT_SECRET=*) printf 'GEOGUESSME_APPLE_CLIENT_SECRET=%s\n' "$APPLE_OAUTH_CLIENT_SECRET" ;;
        *) printf '%s\n' "$line" ;;
    esac
done <deployment/env/identity.env.example
