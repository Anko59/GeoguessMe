#!/bin/sh
set -eu

state_root=${GEOGUESSME_WATCH_CAPACITY_ROOT:-/var/lib/geoguessme/watch-capacity}
install -d -m 0750 "$state_root"
timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
mem_available_kib=$(awk '/^MemAvailable:/ { print $2 }' /proc/meminfo)
swap_in=$(awk '/^pswpin / { print $2 }' /proc/vmstat)
swap_out=$(awk '/^pswpout / { print $2 }' /proc/vmstat)
disk_use=$(df -P / | awk 'NR == 2 { gsub(/%/, "", $5); print $5 }')
disk_free_kib=$(df -P / | awk 'NR == 2 { print $4 }')
disk_use=${disk_use:-100}
disk_free_kib=${disk_free_kib:-0}
load_average=$(awk '{ print $1 "," $2 "," $3 }' /proc/loadavg)
printf '%s mem_available_kib=%s swap_in=%s swap_out=%s disk_use_percent=%s disk_free_kib=%s load=%s\n' \
    "$timestamp" "$mem_available_kib" "$swap_in" "$swap_out" "$disk_use" "$disk_free_kib" "$load_average" \
    >>"$state_root/samples.log"
tail -n 10000 "$state_root/samples.log" >"$state_root/samples.log.tmp"
mv "$state_root/samples.log.tmp" "$state_root/samples.log"
