# GeoGuessMe Documentation

GeoGuessMe is a real-time multiplayer location game: group members share
private, short-lived photo or video challenges, view each one for ten seconds,
and submit one server-timed guess.

## Audience guide

| Role                    | Recommended reading                                                                                                                                                                                                        |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Operator / deployer** | [deployment](deployment.md), [configuration](configuration.md), [operations](operations.md), [video-processing](video-processing.md), [database-migrations](database-migrations.md), [troubleshooting](troubleshooting.md) |
| **Developer**           | [local-development](local-development.md), [architecture](architecture.md), [testing](testing.md), [configuration](configuration.md)                                                                                       |
| **API consumer**        | [api](api.md), [authentication](authentication.md), [openapi.yaml](openapi.yaml)                                                                                                                                           |
| **Security reviewer**   | [security-and-privacy](security-and-privacy.md), [authentication](authentication.md), [architecture](architecture.md)                                                                                                      |

## Documentation map

- [Architecture](architecture.md) — system components, trust boundaries, request
  flows
- [Agent engineering guide](agent-engineering.md) — package map, change-impact
  matrix, canonical commands, invariants, and compatibility ledger for AI agents
- [Local development](local-development.md) — prerequisites, `make dev`, hot
  reload
- [Configuration](configuration.md) — every environment variable, defaults,
  validation
- [Gameplay](gameplay.md) — challenge lifecycle, scoring, result visibility
- [Photo filters](photo-filters.md) — camera filters, capture behavior, and
  permissions
- [Rank badges](rank-badges.md) — progression badge artwork and global rank
- [Custom UI icons](custom-icons.md) — branded icon artwork for camera controls,
  lens picker, and leaderboard medals
- [Authentication](authentication.md) — access/refresh tokens, verification,
  account deletion
- [API reference](api.md) — endpoint conventions, error format, rate limits
- [OpenAPI specification](openapi.yaml) — machine-readable API contract
- [Testing](testing.md) — unit, integration, E2E, CI equivalence
- [Deployment](deployment.md) — images, topologies, upgrade, rollback
- [Operations](operations.md) — health, metrics, backups, incident response
- [Video processing](video-processing.md) — async video pipeline, quarantine,
  transcoding, cleanup
- [LLM-driven QA agent](qa-agent.md) — source-blind exploratory release
  acceptance and evidence
- [Database migrations](database-migrations.md) — rules, execution, recovery
- [Security and privacy](security-and-privacy.md) — model, data inventory,
  operator obligations
- [Image security scanning](security-scanning.md) — `make audit-images`,
  blocking semantics, exception policy, pin refresh
- [Troubleshooting](troubleshooting.md) — frequent issues and solutions
- [PWA and Web Push](pwa-and-push.md) — installable app, push notifications, iOS
  guidance, VAPID key generation
- [Hosted deployment runbook](runbooks/hosted-deployment.md) — Hetzner,
  Cloudflare, CI/CD, launch, and recovery checklist
- [Cloudflare Access service-token runbook](runbooks/access-tokens.md) — three
  isolated service tokens, GitHub secrets, rotation
- [HSTS rollout runbook](runbooks/hsts-rollout.md) — staged HSTS max-age
  increase after seven green days
- [DMARC rollout runbook](runbooks/dmarc-rollout.md) — p=none to reject,
  advancing only while aligned mail passes
- [Runtime hardening runbook](runbooks/runtime-hardening.md) — container
  hardening, mandatory rehearsal, host hash verification, closure checklist
- [Social-auth rollout runbook](runbooks/social-auth-rollout.md) — Keycloak
  launch, preservation of legacy user IDs, migration evidence, and the staged
  read-only policy
- [July 2026 release recovery](runbooks/release-recovery-2026-07.md) — incident
  record, corrected safeguards, and release checklist

## Canonical documentation owners

Each subject has exactly one canonical owner document; every other page routes
to it instead of restating it:

| Subject                               | Canonical owner                                                 |
| ------------------------------------- | --------------------------------------------------------------- |
| Deployment, upgrades, rollback        | [deployment](deployment.md)                                     |
| Environment variables and defaults    | [configuration](configuration.md)                               |
| Health, metrics, backups, incidents   | [operations](operations.md)                                     |
| Gates, test strategy, local/CI parity | [testing](testing.md)                                           |
| API conventions, errors, rate limits  | [api](api.md) (machine contract: [openapi.yaml](openapi.yaml))  |
| Package map and agent guidance        | [agent-engineering](agent-engineering.md)                       |
| Local development workflow            | [local-development](local-development.md)                       |
| Troubleshooting                       | [troubleshooting](troubleshooting.md)                           |
| Hosted deployment checklist           | [runbooks/hosted-deployment](runbooks/hosted-deployment.md)     |
| Social-auth account migration         | [runbooks/social-auth-rollout](runbooks/social-auth-rollout.md) |

Pages that used to duplicate a canonical subject route to it instead. Historical
incident records live under `docs/runbooks/` and declare `status: archival`;
they are never linked from agent instructions.

## Repository

See [README.md](../README.md) for the project overview,
[CONTRIBUTING.md](../CONTRIBUTING.md) for contribution guidelines,
[SECURITY.md](../SECURITY.md) for vulnerability reporting, and
[LICENSE](../LICENSE) for licensing.
