#!/bin/sh
set -eu

APP_ROOT=${GEOGUESSME_APP_ROOT:-/opt/geoguessme}
CONFIG_ROOT=${GEOGUESSME_CONFIG_ROOT:-$APP_ROOT/config}
COMPOSE_PROJECT_NAME=geoguessme-watch
export COMPOSE_PROJECT_NAME

compose() {
    GEOGUESSME_WATCH_METRICS_DIR=${GEOGUESSME_WATCH_METRICS_DIR:-/etc/geoguessme/watch-metrics} \
        GEOGUESSME_WATCH_AGENT_ENV=${GEOGUESSME_WATCH_AGENT_ENV:-/etc/geoguessme/watch-agent.env} \
        docker compose --project-directory "$CONFIG_ROOT" \
        -f "$CONFIG_ROOT/compose.watch.yaml" "$@"
}

die() {
    printf 'ERROR: %s\n' "$*" >&2
    exit 1
}

for service in gateway beszel beszel-agent socket-proxy victoria-logs victoria-metrics vector; do
    container_id=$(compose ps --quiet "$service")
    [ -n "$container_id" ] || die "watch service is missing: $service"
    state=$(docker inspect --format '{{.State.Status}}' "$container_id")
    [ "$state" = running ] || die "watch service is $state: $service"
    health=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}running{{end}}' "$container_id")
    case "$health" in
        healthy | running) ;;
        *) die "watch service health is $health: $service" ;;
    esac
done

available_kib=$(awk '/^MemAvailable:/ { print $2 }' /proc/meminfo)
[ "${available_kib:-0}" -ge 524288 ] ||
    die "host available memory is below 512 MiB: ${available_kib:-0} KiB"

disk_use=$(df -P / | awk 'NR == 2 { gsub(/%/, "", $5); print $5 }')
disk_free_kib=$(df -P / | awk 'NR == 2 { print $4 }')
disk_use=${disk_use:-100}
disk_free_kib=${disk_free_kib:-0}
[ "$disk_use" -lt 80 ] || die "root filesystem usage is ${disk_use}%"
[ "$disk_free_kib" -ge 8388608 ] ||
    die "root filesystem has less than 8 GiB free: ${disk_free_kib} KiB"

origin=${GEOGUESSME_WATCH_ORIGIN:-http://127.0.0.1:8084}
curl --fail --silent --show-error --max-time 10 "$origin/api/health" >/dev/null
curl --fail --silent --show-error --max-time 10 "$origin/logs/-/healthy" >/dev/null
curl --fail --silent --show-error --max-time 10 "$origin/metrics/-/healthy" >/dev/null

metric_response=$(curl --fail --silent --show-error --max-time 10 \
    -G --data-urlencode 'query=up{job="geoguessme-production"}' \
    "$origin/metrics/api/v1/query")
printf '%s' "$metric_response" | grep -Fq '"value"' ||
    die 'production metrics target has not been scraped successfully'

log_response=$(curl --fail --silent --show-error --max-time 10 \
    -G --data-urlencode 'query=container_name:geoguessme-production-backend-1 _time:45m' \
    "$origin/logs/select/logsql/query")
printf '%s' "$log_response" | grep -Fq '"_msg"' ||
    die 'recent production backend logs are not present in VictoriaLogs'

printf 'watch health check passed: memory=%sKiB disk=%s%% free=%sKiB\n' \
    "$available_kib" "$disk_use" "$disk_free_kib"
