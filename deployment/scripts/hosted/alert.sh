#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=deployment/scripts/hosted/common.sh
. "$SCRIPT_DIR/common.sh"

environment=${1:-}
validate_environment "$environment"
require_secret_file "$environment"
secret_file=$(environment_env_file "$environment")

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
    printf 'Subject: [GeoGuessMe] %s host check failed\r\n' "$environment"
    printf '\r\nThe %s host check failed at %s. Review the systemd journal and GitHub health workflow.\r\n' \
        "$environment" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} >"$message"

curl --fail --silent --show-error --ssl-reqd \
    --url "smtp://$host:$port" --user "$username:$password" \
    --mail-from "$sender" --mail-rcpt jeancollette138@gmail.com \
    --upload-file "$message"
