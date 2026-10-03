#!/bin/sh
set -eu

bundle=${1:?runtime bundle path is required}
manifest=$(mktemp /opt/geoguessme/config/runtime-hashes.XXXXXX)
temporary=''
trap 'rm -f "$manifest" "$temporary"' EXIT INT TERM
exec 3<"$bundle"
IFS= read -r magic <&3
[ "$magic" = GEOGUESSME_RUNTIME_BUNDLE_V1 ]
lengths=''
for _ in $(seq 1 32); do
    IFS= read -r size <&3
    case "$size" in *[!0-9]* | '') exit 1 ;; esac
    lengths="$lengths $size"
done
# The runtime lengths are validated decimal tokens; split them into arguments.
# shellcheck disable=SC2086
set -- $lengths
while IFS=' ' read -r path mode; do
    [ -n "$path" ] || continue
    size=$1
    shift
    case "$path" in
        /opt/geoguessme/bin/* | /opt/geoguessme/config/* | /etc/systemd/system/geoguessme-*) ;;
        *) exit 1 ;;
    esac
    install -d -m 0755 "$(dirname "$path")"
    temporary=$(mktemp "$path.XXXXXX")
    # Duplicate the open descriptor: reopening /dev/fd/3 resets a regular
    # bundle file to byte zero instead of sharing the parsed stream offset.
    dd bs=1 count="$size" <&3 >"$temporary" 2>/dev/null
    [ "$(wc -c <"$temporary")" -eq "$size" ]
    chown root:root "$temporary"
    chmod "$mode" "$temporary"
    mv -f "$temporary" "$path"
    relative=${path#/opt/geoguessme/}
    case "$path" in /etc/systemd/system/geoguessme-*) relative=units/${path##*/} ;; esac
    printf '%s  %s\n' "$(sha256sum "$path" | cut -d' ' -f1)" \
        "$relative" >>"$manifest"
done <<'FILES'
/opt/geoguessme/bin/common.sh 0755
/opt/geoguessme/bin/deploy.sh 0755
/opt/geoguessme/bin/forced-command.sh 0755
/opt/geoguessme/bin/watch-deploy.sh 0755
/opt/geoguessme/bin/verify-deployment-hashes.sh 0755
/opt/geoguessme/bin/backup.sh 0755
/opt/geoguessme/bin/restore-rehearsal.sh 0755
/opt/geoguessme/bin/health-check.sh 0755
/opt/geoguessme/bin/alert.sh 0755
/opt/geoguessme/bin/watch-health.sh 0755
/opt/geoguessme/bin/watch-refresh-metrics-token.sh 0755
/opt/geoguessme/bin/watch-capacity.sh 0755
/opt/geoguessme/config/compose.production.yaml 0444
/opt/geoguessme/config/compose.hosted.yaml 0444
/opt/geoguessme/config/compose.watch.yaml 0444
/opt/geoguessme/config/watch/Caddyfile 0444
/opt/geoguessme/config/watch/vector.yaml 0444
/opt/geoguessme/config/watch/victoria-metrics.yaml 0444
/etc/systemd/system/geoguessme-backup@.service 0644
/etc/systemd/system/geoguessme-backup@.timer 0644
/etc/systemd/system/geoguessme-health@.service 0644
/etc/systemd/system/geoguessme-health@.timer 0644
/etc/systemd/system/geoguessme-restore-rehearsal@.service 0644
/etc/systemd/system/geoguessme-restore-rehearsal@.timer 0644
/etc/systemd/system/geoguessme-alert@.service 0644
/etc/systemd/system/geoguessme-watch.service 0644
/etc/systemd/system/geoguessme-watch-health.service 0644
/etc/systemd/system/geoguessme-watch-health.timer 0644
/etc/systemd/system/geoguessme-watch-refresh-metrics-token.service 0644
/etc/systemd/system/geoguessme-watch-refresh-metrics-token.timer 0644
/etc/systemd/system/geoguessme-watch-capacity.service 0644
/etc/systemd/system/geoguessme-watch-capacity.timer 0644
FILES
chown root:root "$manifest"
chmod 0444 "$manifest"
mv -f "$manifest" /opt/geoguessme/config/runtime-hashes
trap - EXIT INT TERM
