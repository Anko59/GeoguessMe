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
services='postgres backend web'
if oidc_enabled "$(environment_env_file "$environment")"; then
    services="$services oauth2-proxy"
fi
for service in $services; do
    container_id=$(compose "$environment" "$release" ps --quiet "$service")
    [ -n "$container_id" ] || die "$environment $service container is missing"
    health=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id")
    [ "$health" = healthy ] || die "$environment $service is $health"
done

if [ "$environment" = production ] && [ -f "$(identity_env_file)" ]; then
    for service in keycloak-db keycloak; do
        container_id=$(compose_identity "$release" ps --quiet "$service")
        [ -n "$container_id" ] || die "identity $service container is missing"
        health=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id")
        [ "$health" = healthy ] || die "identity $service is $health"
    done
fi

disk_use=$(df -P "$STATE_ROOT" | awk 'NR == 2 { gsub(/%/, "", $5); print $5 }')
[ "$disk_use" -lt 85 ] || die "disk usage is ${disk_use}%"
systemctl is-active --quiet cloudflared || die 'cloudflared is not active'

# The same per-environment systemd timer that checks runtime health also checks
# the root-owned host definitions every 15 minutes. On mismatch the oneshot
# fails and its OnFailure unit sends the normal operator alert.
"$SCRIPT_DIR/verify-deployment-hashes.sh" "$environment"

last_success="$STATE_ROOT/backups/$environment/last-success"
age=$(backup_age_seconds "$last_success")
[ "$age" -le 7200 ] || die "$environment backup is older than two hours"
printf 'health check passed: environment=%s disk=%s%% backup_age=%ss\n' \
    "$environment" "$disk_use" "$age"
