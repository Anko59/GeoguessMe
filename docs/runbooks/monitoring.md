---
status: primary
---

# Monitoring stack runbook

The hosted monitoring stack runs on the existing Hetzner VM as an independent
Compose project named `geoguessme-watch`. It has no public listener of its own.
Cloudflare Tunnel routes `watch.geoguessme.com` to the loopback gateway only
after the owner-only Cloudflare Access application is present.

The stack is deliberately lightweight:

| Service              | Role                                        | Retention / limit               |
| -------------------- | ------------------------------------------- | ------------------------------- |
| Beszel hub and agent | Host, Docker, resource, and alert dashboard | Beszel data volume              |
| VictoriaLogs         | Production Docker log search                | 7 days, 2 GiB                   |
| VictoriaMetrics      | Production application metrics and query UI | 14 days                         |
| Vector               | Production Docker log collection            | bounded container memory        |
| Docker socket proxy  | Read-only Docker API for Beszel and Vector  | no write methods                |
| Caddy                | One loopback gateway                        | no persistent application state |

Monitoring is colocated with the application. It cannot report a complete VM
failure while that VM is down. The scheduled GitHub hosted-health workflow
remains the independent detector for production readiness and tunnel outages.

## Capacity gate

Do not enable the monitoring stack until the host has at least 24 hours of
representative capacity evidence, including the normal traffic peak and
database-backup timers.

Install the reviewed host definitions, then enable only the sampler:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now geoguessme-watch-capacity.timer
```

The sampler appends bounded records to
`/var/lib/geoguessme/watch-capacity/samples.log`. Review:

- available memory stays above 1.5 GiB;
- root filesystem free space stays above 12 GiB;
- swap-in and swap-out counters do not increase persistently;
- load and application latency do not show a sustained regression;
- no Docker or kernel OOM kill occurs.

The monitoring Compose project has a combined configured memory ceiling below 1
GiB. Never reduce the existing application limits to make this stack fit. If the
gate fails, keep monitoring disabled and use the external health workflow.

## First setup

The source definitions are installed root-owned by the runtime-hardening
procedure. On the existing stateful host, Terraform does not rewrite
`/opt/geoguessme` after the initial cloud-init run. Apply the complete reviewed
runtime set through the Access-protected operator SSH procedure documented in
[the runtime-hardening runbook](runtime-hardening.md), then verify both hosted
application environments.

Create the two live secret inputs as the `deploy` user:

```sh
sudo install -o deploy -g deploy -m 0600 /dev/null /etc/geoguessme/watch-agent.env
sudo install -d -o root -g docker -m 0750 /etc/geoguessme/watch-metrics
```

Put the Beszel Add System values in `/etc/geoguessme/watch-agent.env`:

```dotenv
KEY=<public key from the Beszel Hub>
TOKEN=<agent token from the Beszel Hub>
```

Do not commit either value. Do not mount the production application dotenv into
the monitoring project. The token-refresh timer copies only `METRICS_TOKEN` into
the dedicated metrics directory.

After the production environment file exists, refresh the dedicated token and
start monitoring:

```sh
sudo /opt/geoguessme/bin/watch-refresh-metrics-token.sh
sudo systemctl enable --now geoguessme-watch.service
sudo systemctl enable --now geoguessme-watch-refresh-metrics-token.timer
sudo systemctl enable --now geoguessme-watch-health.timer
```

The gateway takes `WEB_IMAGE` from the root-owned production release metadata at
`/var/lib/geoguessme/releases/production/current.env`. That value is an
immutable digest for the already patched and scanned production Caddy runtime;
the watch project mounts its own Caddyfile and does not mount the production
dotenv. A production release does not recreate the watch project.

Open the loopback gateway through the Access route only after the private checks
pass. Create the first Beszel administrator through the Hub UI. Keep Beszel
password authentication enabled; Cloudflare Access is an outer authorization
boundary, not a replacement for the Hub account.

In Beszel, add the local system using the Unix socket
`/beszel_socket/beszel.sock`. Configure alerts for host status, memory, swap,
disk usage, and high load. Configure the notification channel only after testing
the existing production SMTP path.

## URLs

After Cloudflare DNS and Access are applied:

- Beszel: `https://watch.geoguessme.com/`
- VictoriaLogs: `https://watch.geoguessme.com/logs/select/vmui/`
- VictoriaMetrics: `https://watch.geoguessme.com/metrics/vmui/`

The ingestion APIs and application metrics endpoint remain private. Local
operator checks use `http://127.0.0.1:8084`; they must never be changed to a
public bind address.

## Backups and restore

Beszel's data volume contains its administrator, system, and alert state.
Configure Beszel's built-in S3-compatible backup in its administrator settings
using a private Cloudflare R2 destination under the dedicated
`monitoring/beszel/` prefix of the existing backup bucket. Use credentials
restricted to that prefix; never reuse a broad media or database credential.
Schedule a daily backup, execute one manual backup, and verify that the object
appears in R2.

Rehearse restoration into a disposable Beszel hub before enabling the scheduled
backup as an acceptance gate. Restore the data, verify the administrator and
system record, then remove only the disposable project and volume.

VictoriaLogs and VictoriaMetrics history is operationally useful but
recreatable. Their history is not treated as the authoritative backup after a
complete VM loss; the documented recovery is to recreate the volumes and resume
ingestion. The Hetzner VM backup and application database backups remain
separate recovery controls.

## Token rotation

Production deployment secret rotation changes the application `METRICS_TOKEN`.
The root-owned `geoguessme-watch-refresh-metrics-token.timer` refreshes the
dedicated file every five minutes using an atomic rename. VictoriaMetrics reads
that file through a directory bind mount, so the monitoring container does not
receive the full production dotenv and does not need to be recreated.

After rotation, confirm the next scrape is successful in the VictoriaMetrics UI
and that the watch health timer remains green. If scrapes fail, run the refresh
script manually and inspect the VictoriaMetrics and timer journals.

## Health and rollback

The 15-minute watch health check verifies:

- every monitoring container is running and healthy where a container health
  command exists;
- host memory remains above 512 MiB and root disk remains below 80% used with at
  least 8 GiB free;
- Beszel, VictoriaLogs, and VictoriaMetrics respond through the loopback
  gateway;
- the production metrics target has a current successful scrape; and
- recent production backend logs are present in VictoriaLogs.

A failure triggers the existing SMTP alert unit with a monitoring-specific
subject. Inspect:

```sh
sudo journalctl -u geoguessme-watch.service
sudo journalctl -u geoguessme-watch-health.service
sudo docker compose -p geoguessme-watch -f /opt/geoguessme/config/compose.watch.yaml ps
sudo docker compose -p geoguessme-watch -f /opt/geoguessme/config/compose.watch.yaml logs --since=30m
```

If the capacity or latency acceptance gate fails, stop only the monitoring
project and preserve its volumes:

```sh
sudo systemctl stop geoguessme-watch-health.timer geoguessme-watch.service
```

Do not stop the application Compose projects. Reverting the monitoring revision
restores the previous monitoring definitions; it does not alter application
images, migrations, or database state.
