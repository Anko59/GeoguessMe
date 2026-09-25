#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=deployment/scripts/hosted/common.sh
. "$SCRIPT_DIR/common.sh"

environment=${1:-}
case "$environment" in
    dev | production)
        validate_environment "$environment"
        require_secret_file "$environment"
        secret_file=$(environment_env_file "$environment")
        subject_environment=$environment
        ;;
    watch)
        # Monitoring has no independent SMTP credential. Reuse the production
        # alert transport, but keep the subject and body distinct so a failed
        # watch check cannot be mistaken for an application check.
        require_secret_file production
        secret_file=$(environment_env_file production)
        subject_environment='watch monitoring'
        ;;
    *)
        die 'environment must be dev, production, or watch'
        ;;
esac

value() {
    sed -n "s/^$1=//p" "$secret_file" | tail -1
}

host=$(value SMTP_HOST)
port=$(value SMTP_PORT)
username=$(value SMTP_USERNAME)
password=$(value SMTP_PASSWORD)
sender=$(value SMTP_FROM)
if [ -z "$host" ] || [ -z "$port" ] || [ -z "$username" ] ||
    [ -z "$password" ] || [ -z "$sender" ]; then
    die 'SMTP alert configuration is incomplete'
fi

message=$(mktemp)
trap 'rm -f "$message"' EXIT INT TERM
{
    printf 'From: %s\r\n' "$sender"
    printf 'To: jeancollette138@gmail.com\r\n'
    printf 'Subject: [GeoGuessMe] %s host check failed\r\n' "$subject_environment"
    printf '\r\nThe %s host check failed at %s. Review the systemd journal and GitHub health workflow.\r\n' \
        "$subject_environment" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} >"$message"

curl --fail --silent --show-error --ssl-reqd \
    --url "smtp://$host:$port" --user "$username:$password" \
    --mail-from "$sender" --mail-rcpt jeancollette138@gmail.com \
    --upload-file "$message"
