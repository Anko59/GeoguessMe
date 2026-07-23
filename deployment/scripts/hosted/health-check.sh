#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=deployment/scripts/hosted/common.sh
. "$SCRIPT_DIR/common.sh"

environment=${1:-}
validate_environment "$environment"
release="$APP_ROOT/$environment/current"
[ -d "$release" ] || die "$environment has no active release"

curl --fail --silent --show-error --max-time 10 \
    "http://127.0.0.1:$(environment_port "$environment")/health/ready" >/dev/null
for service in postgres backend web; do
    container_id=$(compose "$environment" "$release" ps --quiet "$service")
    [ -n "$container_id" ] || die "$environment $service container is missing"
    health=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id")
    [ "$health" = healthy ] || die "$environment $service is $health"
done

disk_use=$(df -P "$STATE_ROOT" | awk 'NR == 2 { gsub(/%/, "", $5); print $5 }')
[ "$disk_use" -lt 85 ] || die "disk usage is ${disk_use}%"
systemctl is-active --quiet cloudflared || die 'cloudflared is not active'

last_success="$STATE_ROOT/backups/$environment/last-success"
age=$(backup_age_seconds "$last_success")
[ "$age" -le 7200 ] || die "$environment backup is older than two hours"
printf 'health check passed: environment=%s disk=%s%% backup_age=%ss\n' \
    "$environment" "$disk_use" "$age"
