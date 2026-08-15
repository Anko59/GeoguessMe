# Deployment working rules

Compose, Caddy, Docker images, and hosted infrastructure. Applies in addition to
the root [AGENTS.md](../AGENTS.md), the engineering guide
([docs/agent-engineering.md](../docs/agent-engineering.md)), and the human
[deployment guide](README.md).

## Ownership

- Compose topologies: `deployment/compose.*.yaml` (dev, test, production,
  hosted, tools). Caddy: `deployment/caddy/`. Images: `deployment/docker/`.
  Hosted infrastructure: `infra/terraform/` and `infra/cloud-init/`.
- Environment templates, deployment documentation, and
  [docs/configuration.md](../docs/configuration.md) must always agree. A missing
  example is a defect.

## Release invariants

- Production promotes the exact signed image digests that passed the complete
  `dev` gate. Never rebuild source at promotion time.
- Migrations stay forward-only and rollback is staged: deploy code that stops
  old writers before applying cleanup migrations.
- Rehearsals are mandatory: `backup-rehearsal`, `restart-rehearsal`, and
  `reconnect-rehearsal` are part of `make verify`.

## Gates

- `make terraform-fmt-check`, `make terraform-test`, `make lint-caddy`, and
  `make compose-validate` for static checks.
- Any deployment, infrastructure, or image change requires the complete
  `make verify` gate on the exact revision, plus the operational rehearsals.
- Incident records go to `docs/runbooks/` as archival documents, never as
  primary instructions.
