# Deployment guide

The supported deployment workflow is documented in
[deployment/README.md](../deployment/README.md). It covers first deploy,
migrations, immutable image upgrades, rollback, backup/restore, restart
behavior, health checks, secrets, outage response, and rehearsal evidence.

The concrete hosted implementation and launch checklist is in the
[hosted deployment runbook](runbooks/hosted-deployment.md). It covers the
Hetzner CX23, Cloudflare Tunnel/Access/R2, SOPS age keys, GitHub environments,
signed digest deployments, Brevo, monitoring, and recovery.

All operational actions use Dockerized Make targets:

```text
make compose-validate
make prod-config
make prod-migrate
make prod-up
make smoke BASE_URL=https://your-domain.example
make prod-logs
make prod-container-verify
```

`make prod-container-verify` builds the pinned production images, validates
non-root users, image healthchecks, read-only filesystems, and Compose
configuration, then starts a disposable production-like local stack with
test-only credentials, polls health and readiness, verifies representative HTTP
behavior (liveness, readiness, auth enforcement, WebSocket auth), and tears down
all resources. It is safe for local/CI use because it uses the `local-db`,
`local-minio`, and `local-smtp` Compose profiles and never touches production
infrastructure. The gateway uses port `18083` and the disposable Mailpit UI uses
`18085` by default; set `GEOGUESSME_PROD_VERIFY_WEB_PORT` or
`GEOGUESSME_PROD_VERIFY_SMTP_PORT` when those ports are occupied.

Compose restart is not zero-downtime rolling deployment. Do not describe this
topology as rolling without adding an orchestrator and its corresponding failure
and rollback evidence.

## Live acceptance

Repository rehearsals remain disposable. Live R2, Access, Tunnel, and Brevo must
be validated on dev, followed by a 24-hour soak and an isolated production
backup restore, before the first production tag.

## See also

- [deployment/README.md](../deployment/README.md) — rehearsal evidence table,
  tool architecture, first-deploy steps
- [configuration.md](configuration.md) — every environment variable, defaults,
  production validation
- [operations.md](operations.md) — health, metrics, backups, secret rotation,
  incident response
- [testing.md](testing.md) — comprehensive gate listing with expected results
