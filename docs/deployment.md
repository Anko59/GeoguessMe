# Deployment guide

The supported deployment workflow is documented in deployment/README.md. It
covers first deploy, migrations, immutable image upgrades, rollback,
backup/restore, restart behavior, health checks, secrets, and outage response.

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
infrastructure.

Compose restart is not zero-downtime rolling deployment. Do not describe this
topology as rolling without adding an orchestrator and its corresponding failure
and rollback evidence.
