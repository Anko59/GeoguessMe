#!/bin/sh
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../../../.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf "${fixture:?}"' EXIT INT TERM
mkdir -p "$fixture/root/usr/local/sbin" "$fixture/bin"
gzip -n -c "$ROOT/infra/cloud-init/bootstrap-host.sh" >"$fixture/bootstrap.gz"
gzip -dc "$fixture/bootstrap.gz" >"$fixture/decoded-bootstrap"
cmp "$ROOT/infra/cloud-init/bootstrap-host.sh" "$fixture/decoded-bootstrap"
# Redirect every absolute writable path to a fixture. All OS/network commands
# are shims; this test never runs age, Docker, systemd, firewall or swap tools.
sed -e "s|/etc/|$fixture/root/etc/|g" -e "s|/opt/|$fixture/root/opt/|g" \
    -e "s|/var/|$fixture/root/var/|g" -e "s|/usr/local/sbin/|$fixture/root/usr/local/sbin/|g" \
    -e "s|/tmp/cloudflared.deb|$fixture/root/tmp/cloudflared.deb|g" \
    -e "s|/tmp/geoguessme-runtime-bundle|$fixture/root/tmp/geoguessme-runtime-bundle|g" \
    -e "s|/swapfile|$fixture/root/swapfile|g" \
    "$ROOT/infra/cloud-init/bootstrap-host.sh" >"$fixture/bootstrap.sh"
cat >"$fixture/bin/tool" <<'SHIM'
#!/bin/sh
set -eu
name=${0##*/}
{
    printf '%s' "$name"
    printf ' %s' "$@"
    printf '\n'
} | /bin/sed "s|${BOOTSTRAP_FIXTURE:?}/root||g" >>"${BOOTSTRAP_CALLS:?}"
[ "${BOOTSTRAP_FAIL_COMMAND:-}" != "$name" ] || exit 71
case "$name" in
    mkdir) /bin/mkdir "$@" ;;
    age-keygen)
        if [ "$1" = -o ]; then printf 'fake-private-fixture\n' >"$2"; fi
        if [ "$1" = -y ]; then printf 'fake-public-fixture\n'; fi
        ;;
    sha256sum) IFS= read -r checksum; test "$checksum" = "88195157a136199a86977c122a22084dae6907480bbe3640222b7b55834afc3a  $BOOTSTRAP_FIXTURE/root/tmp/cloudflared.deb" ;;
esac
SHIM
chmod +x "$fixture/bin/tool"
for tool in gzip install mktemp seq dd wc chown chmod mv sha256sum cut mkdir rm \
    systemd-tmpfiles age-keygen curl dpkg fallocate mkswap swapon ufw systemctl; do
    ln -s tool "$fixture/bin/$tool"
done
ln -s "$fixture/bin/tool" "$fixture/root/usr/local/sbin/geoguessme-install-runtime-bundle"
export BOOTSTRAP_FIXTURE="$fixture" BOOTSTRAP_CALLS="$fixture/calls"
PATH="$fixture/bin" /bin/sh "$fixture/bootstrap.sh"
cat >"$fixture/expected" <<'EXPECTED'
mkdir -p /etc/geoguessme/age /etc/geoguessme/watch-metrics /opt/geoguessme/bin /opt/geoguessme/config/watch /opt/geoguessme/dev /opt/geoguessme/production /opt/geoguessme/releases /var/lib/geoguessme/backups /var/lib/geoguessme/releases
geoguessme-install-runtime-bundle /tmp/geoguessme-runtime-bundle
rm -f /usr/local/sbin/geoguessme-install-runtime-bundle /tmp/geoguessme-runtime-bundle
chown -R deploy:deploy /opt/geoguessme/dev /opt/geoguessme/production /opt/geoguessme/releases /var/lib/geoguessme
systemd-tmpfiles --create /etc/tmpfiles.d/geoguessme.conf
chmod 0555 /opt/geoguessme/bin /opt/geoguessme/config /opt/geoguessme/config/watch
chown root:docker /etc/geoguessme/watch-metrics
chmod 0750 /etc/geoguessme/watch-metrics
chown deploy:deploy /etc/geoguessme
age-keygen -o /etc/geoguessme/age/dev.txt
age-keygen -y /etc/geoguessme/age/dev.txt
age-keygen -o /etc/geoguessme/age/production.txt
age-keygen -y /etc/geoguessme/age/production.txt
chmod 0600 /etc/geoguessme/age/dev.txt /etc/geoguessme/age/production.txt
chmod 0644 /etc/geoguessme/age/dev-recipient.txt /etc/geoguessme/age/production-recipient.txt
chown -R deploy:deploy /etc/geoguessme/age
curl -fsSLo /tmp/cloudflared.deb https://github.com/cloudflare/cloudflared/releases/download/2026.7.2/cloudflared-linux-amd64.deb
sha256sum -c -
dpkg -i /tmp/cloudflared.deb
rm -f /tmp/cloudflared.deb
fallocate -l 2G /swapfile
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile
ufw default deny incoming
ufw default allow outgoing
ufw allow in on lo to any
ufw --force enable
systemctl restart ssh
systemctl restart docker
systemctl daemon-reload
systemctl enable --now cloudflared.service
systemctl enable --now geoguessme-backup@dev.timer geoguessme-backup@production.timer
systemctl enable --now geoguessme-health@dev.timer geoguessme-health@production.timer
systemctl enable --now geoguessme-restore-rehearsal@production.timer
systemctl daemon-reload
rm -f /usr/local/sbin/geoguessme-bootstrap-host
EXPECTED
cmp "$fixture/expected" "$BOOTSTRAP_CALLS"
test "$(stat -c %a "$fixture/root/etc/geoguessme/age/dev.txt")" = 600
test "$(stat -c %a "$fixture/root/etc/geoguessme/age/production.txt")" = 600
test "$(sed "s|$fixture/root||g" "$fixture/root/etc/fstab")" = '/swapfile none swap sw 0 0'
: >"$BOOTSTRAP_CALLS"
status=0
BOOTSTRAP_FAIL_COMMAND=sha256sum PATH="$fixture/bin" /bin/sh "$fixture/bootstrap.sh" || status=$?
test "$status" -eq 71
if grep -q '^dpkg\|^ufw\|^systemctl' "$BOOTSTRAP_CALLS"; then
    echo 'bootstrap continued after cloudflared integrity failure' >&2
    exit 1
fi
printf 'host bootstrap tests passed: exact ordered fixture commands, private umask and fail-closed integrity checks\n'
