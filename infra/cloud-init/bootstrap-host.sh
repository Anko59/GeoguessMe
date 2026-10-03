#!/bin/sh
set -eu

# Ubuntu cloud-init installs these packages before runcmd and decodes the three
# gzip+base64 files. Fail early rather than partially configuring a host.
for tool in gzip install mktemp seq dd wc chown chmod mv sha256sum cut \
    mkdir rm systemd-tmpfiles age-keygen curl dpkg fallocate mkswap swapon ufw systemctl; do
    command -v "$tool" >/dev/null 2>&1 || {
        echo "host bootstrap requires $tool" >&2
        exit 1
    }
done
mkdir -p /etc/geoguessme/age /etc/geoguessme/watch-metrics \
    /opt/geoguessme/bin /opt/geoguessme/config/watch /opt/geoguessme/dev \
    /opt/geoguessme/production /opt/geoguessme/releases \
    /var/lib/geoguessme/backups /var/lib/geoguessme/releases
/usr/local/sbin/geoguessme-install-runtime-bundle /tmp/geoguessme-runtime-bundle
rm -f /usr/local/sbin/geoguessme-install-runtime-bundle /tmp/geoguessme-runtime-bundle
chown -R deploy:deploy /opt/geoguessme/dev /opt/geoguessme/production \
    /opt/geoguessme/releases /var/lib/geoguessme
systemd-tmpfiles --create /etc/tmpfiles.d/geoguessme.conf
chmod 0555 /opt/geoguessme/bin /opt/geoguessme/config /opt/geoguessme/config/watch
chown root:docker /etc/geoguessme/watch-metrics
chmod 0750 /etc/geoguessme/watch-metrics
chown deploy:deploy /etc/geoguessme
(
    umask 077
    age-keygen -o /etc/geoguessme/age/dev.txt
    age-keygen -y /etc/geoguessme/age/dev.txt >/etc/geoguessme/age/dev-recipient.txt
    age-keygen -o /etc/geoguessme/age/production.txt
    age-keygen -y /etc/geoguessme/age/production.txt >/etc/geoguessme/age/production-recipient.txt
)
chmod 0600 /etc/geoguessme/age/dev.txt /etc/geoguessme/age/production.txt
chmod 0644 /etc/geoguessme/age/dev-recipient.txt /etc/geoguessme/age/production-recipient.txt
chown -R deploy:deploy /etc/geoguessme/age
curl -fsSLo /tmp/cloudflared.deb https://github.com/cloudflare/cloudflared/releases/download/2026.7.2/cloudflared-linux-amd64.deb
echo '88195157a136199a86977c122a22084dae6907480bbe3640222b7b55834afc3a  /tmp/cloudflared.deb' | sha256sum -c -
dpkg -i /tmp/cloudflared.deb
rm -f /tmp/cloudflared.deb
fallocate -l 2G /swapfile
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile
echo '/swapfile none swap sw 0 0' >>/etc/fstab
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
# Monitoring remains disabled until the operator completes capacity and secrets.
systemctl daemon-reload
# Self-clean only after success; runcmd must retain a bootstrap failure status.
rm -f /usr/local/sbin/geoguessme-bootstrap-host
