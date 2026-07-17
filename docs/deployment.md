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
```

Compose restart is not zero-downtime rolling deployment. Do not describe this
topology as rolling without adding an orchestrator and its corresponding failure
and rollback evidence.
