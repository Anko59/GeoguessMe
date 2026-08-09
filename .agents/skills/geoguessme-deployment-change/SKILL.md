---
name: geoguessme-deployment-change
description:
    Use when changing GeoGuessMe deployment, Compose topologies, Caddy, Docker
    images, Terraform, cloud-init, environment configuration, or release
    rehearsals. Activates for deployment, infrastructure, and release-flow work.
---

# GeoGuessMe deployment change

## Workflow

1. Follow [deployment/AGENTS.md](../../../deployment/AGENTS.md), including the
   bounded maintenance scan, change-impact workflow, and release invariants in
   [docs/agent-engineering.md](../../../docs/agent-engineering.md).
2. Locate the topology: `deployment/compose.*.yaml`, `deployment/caddy/`,
   `deployment/docker/`, `infra/terraform/`, `infra/cloud-init/`.
3. Keep environment templates, deployment documentation, and
   [docs/configuration.md](../../../docs/configuration.md) in agreement. A
   missing environment example is a defect.
4. Preserve the release invariants: exact signed image digests promoted from a
   green `dev` gate, forward-only migrations, and staged rollback.
5. Update operational documentation
   ([docs/deployment.md](../../../docs/deployment.md),
   [docs/operations.md](../../../docs/operations.md)) with the change.
6. Run `make impact BASE=origin/dev`, `make terraform-fmt-check`,
   `make terraform-test`, `make lint-caddy`, `make compose-validate`, then the
   complete `make verify` gate including the backup, restart, and reconnect
   rehearsals.

## Inputs

- The deployment or infrastructure change description.

## Outputs

- Compose, Caddy, image, Terraform, or cloud-init changes.
- Updated environment templates and operational documentation.
- Gate evidence: `terraform-fmt-check`, `terraform-test`, `lint-caddy`,
  `verify`.

## References

- [deployment/AGENTS.md](../../../deployment/AGENTS.md) — deployment working
  rules.
- [deployment/README.md](../../../deployment/README.md) — human deployment
  guide.
- [docs/operations.md](../../../docs/operations.md) — health, backups,
  incidents.
- [docs/runbooks/hosted-deployment.md](../../../docs/runbooks/hosted-deployment.md)
  — hosted runbook.
- `make terraform-fmt-check`, `make terraform-test`, `make lint-caddy`,
  `make verify`.
