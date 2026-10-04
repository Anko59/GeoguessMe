#!/bin/sh
set -eu

bundle=${1:?runtime bundle path is required}
manifest=$(mktemp /opt/geoguessme/config/runtime-hashes.XXXXXX)
case "$manifest" in /opt/geoguessme/config/runtime-hashes.??????) ;; *) exit 1 ;; esac
staging=''
cleanup() {
    case "$manifest" in /opt/geoguessme/config/runtime-hashes.??????) ;; *) return 1 ;; esac
    rm -f -- "${manifest:?}"
    if [ -n "$staging" ]; then
        case "$staging" in /opt/geoguessme/config/runtime-stage.??????) ;; *) return 1 ;; esac
        rm -rf -- "${staging:?}"
    fi
}
trap cleanup EXIT INT TERM
staging=$(mktemp -d /opt/geoguessme/config/runtime-stage.XXXXXX)
case "$staging" in /opt/geoguessme/config/runtime-stage.??????) ;; *) exit 1 ;; esac
chown root:root "$staging"
chmod 0700 "$staging"
exec 3<"$bundle"
IFS= read -r magic <&3
[ "$magic" = GEOGUESSME_RUNTIME_BUNDLE_V1 ]
lengths=''
for _ in $(seq 1 33); do
    IFS= read -r size <&3
    case "$size" in *[!0-9]* | '') exit 1 ;; esac
    [ "$size" -gt 0 ]
    lengths="$lengths $size"
done
# The runtime lengths are validated decimal tokens; split them into arguments.
# shellcheck disable=SC2086
set -- $lengths
# This trusted destination list is reused for staging and installation, so
# neither member order nor the root-owned paths depend on bundle content.
cat >"$staging/paths" <<'FILES'
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
/opt/geoguessme/config/s3-fixture/credentials.json 0644
FILES
member=0
while IFS=' ' read -r path mode; do
    [ -n "$path" ] || continue
    size=$1
    shift
    member=$((member + 1))
    case "$path" in
        /opt/geoguessme/bin/* | /opt/geoguessme/config/* | /etc/systemd/system/geoguessme-*) ;;
        *) exit 1 ;;
    esac
    # Duplicate the open descriptor: reopening /dev/fd/3 on Linux starts the
    # regular bundle file at offset zero rather than consuming its next member.
    dd of="$staging/$member" bs=1 count="$size" <&3 2>/dev/null
    [ "$(wc -c <"$staging/$member")" -eq "$size" ]
done <"$staging/paths"
# Validate the entire stream before touching any installed runtime member.
[ "$#" -eq 0 ]
[ "$(dd bs=1 count=1 <&3 2>/dev/null | wc -c)" -eq 0 ]
exec 3<&-
member=0
while IFS=' ' read -r path mode; do
    [ -n "$path" ] || continue
    member=$((member + 1))
    install -d -o root -g root -m 0755 "$(dirname "$path")"
    temporary=$(mktemp "$path.XXXXXX")
    case "$temporary" in "$path".??????) ;; *) exit 1 ;; esac
    # Keep each rename on the destination's filesystem, including systemd
    # units when /etc and /opt are on different mounts.
    cp "$staging/$member" "$temporary"
    chown root:root "$temporary"
    chmod "$mode" "$temporary"
    mv -f "$temporary" "$path"
    relative=${path#/opt/geoguessme/}
    case "$path" in /etc/systemd/system/geoguessme-*) relative=units/${path##*/} ;; esac
    printf '%s  %s\n' "$(sha256sum "$path" | cut -d' ' -f1)" \
        "$relative" >>"$manifest"
done <"$staging/paths"
chown root:root "$manifest"
chmod 0444 "$manifest"
mv -f "$manifest" /opt/geoguessme/config/runtime-hashes
cleanup
trap - EXIT INT TERM
