#!/bin/sh
set -eu

environment=${TARGET_ENV:-}
case "$environment" in
    dev | production) ;;
    *)
        printf 'TARGET_ENV must be dev or production\n' >&2
        exit 2
        ;;
esac

: "${BREVO_SMTP_USERNAME:?BREVO_SMTP_USERNAME is required}"
: "${BREVO_SMTP_PASSWORD:?BREVO_SMTP_PASSWORD is required}"
: "${GHCR_USERNAME:?GHCR_USERNAME is required}"
: "${GHCR_TOKEN:?GHCR_TOKEN is required}"
: "${MEDIA_ACCESS_KEY_ID:?MEDIA_ACCESS_KEY_ID is required}"
: "${MEDIA_SECRET_ACCESS_KEY:?MEDIA_SECRET_ACCESS_KEY is required}"
: "${BACKUP_ACCESS_KEY_ID:?BACKUP_ACCESS_KEY_ID is required}"
: "${BACKUP_SECRET_ACCESS_KEY:?BACKUP_SECRET_ACCESS_KEY is required}"
: "${CLOUDFLARE_ACCOUNT_ID:?CLOUDFLARE_ACCOUNT_ID is required}"

random_hex() {
    bytes=$1
    od -An -N "$bytes" -tx1 /dev/urandom | tr -d ' \n'
}

random_base64() {
    bytes=$1
    head -c "$bytes" /dev/urandom | base64 | tr -d '\n'
}

postgres_password=$(random_hex 32)
jwt_secret=$(random_base64 48)
metrics_token=$(random_hex 32)
restic_password=$(random_base64 48)
template="deployment/env/$environment.env.example"

while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
        POSTGRES_PASSWORD=*) printf 'POSTGRES_PASSWORD=%s\n' "$postgres_password" ;;
        DATABASE_URL=*)
            printf 'DATABASE_URL=postgres://geoguessme:%s@postgres/geoguessme?sslmode=disable\n' \
                "$postgres_password"
            ;;
        JWT_SECRET=*) printf 'JWT_SECRET=%s\n' "$jwt_secret" ;;
        S3_ENDPOINT=*)
            printf 'S3_ENDPOINT=https://%s.r2.cloudflarestorage.com\n' \
                "$CLOUDFLARE_ACCOUNT_ID"
            ;;
        S3_ACCESS_KEY=*) printf 'S3_ACCESS_KEY=%s\n' "$MEDIA_ACCESS_KEY_ID" ;;
        S3_SECRET_KEY=*) printf 'S3_SECRET_KEY=%s\n' "$MEDIA_SECRET_ACCESS_KEY" ;;
        SMTP_USERNAME=*) printf 'SMTP_USERNAME=%s\n' "$BREVO_SMTP_USERNAME" ;;
        SMTP_PASSWORD=*) printf 'SMTP_PASSWORD=%s\n' "$BREVO_SMTP_PASSWORD" ;;
        METRICS_TOKEN=*) printf 'METRICS_TOKEN=%s\n' "$metrics_token" ;;
        GHCR_USERNAME=*) printf 'GHCR_USERNAME=%s\n' "$GHCR_USERNAME" ;;
        GHCR_TOKEN=*) printf 'GHCR_TOKEN=%s\n' "$GHCR_TOKEN" ;;
        RESTIC_REPOSITORY=*)
            printf 'RESTIC_REPOSITORY=s3:https://%s.r2.cloudflarestorage.com/geoguessme-database-backups/%s\n' \
                "$CLOUDFLARE_ACCOUNT_ID" "$environment"
            ;;
        AWS_ACCESS_KEY_ID=*) printf 'AWS_ACCESS_KEY_ID=%s\n' "$BACKUP_ACCESS_KEY_ID" ;;
        AWS_SECRET_ACCESS_KEY=*) printf 'AWS_SECRET_ACCESS_KEY=%s\n' "$BACKUP_SECRET_ACCESS_KEY" ;;
        RESTIC_PASSWORD=*) printf 'RESTIC_PASSWORD=%s\n' "$restic_password" ;;
        *) printf '%s\n' "$line" ;;
    esac
done <"$template"
