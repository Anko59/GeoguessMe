# GeoGuessMe Documentation

GeoGuessMe is a real-time multiplayer location game: group members share
private, short-lived photo challenges, view each photo for ten seconds, and
submit one server-timed guess.

## Audience guide

| Role                    | Recommended reading                                                                                                                                                               |
| ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Operator / deployer** | [deployment](deployment.md), [configuration](configuration.md), [operations](operations.md), [database-migrations](database-migrations.md), [troubleshooting](troubleshooting.md) |
| **Developer**           | [local-development](local-development.md), [architecture](architecture.md), [testing](testing.md), [configuration](configuration.md)                                              |
| **API consumer**        | [api](api.md), [authentication](authentication.md), [openapi.yaml](openapi.yaml)                                                                                                  |
| **Security reviewer**   | [security-and-privacy](security-and-privacy.md), [authentication](authentication.md), [architecture](architecture.md)                                                             |

## Documentation map

- [Architecture](architecture.md) — system components, trust boundaries, request
  flows
- [Local development](local-development.md) — prerequisites, `make dev`, hot
  reload
- [Configuration](configuration.md) — every environment variable, defaults,
  validation
- [Gameplay](gameplay.md) — challenge lifecycle, scoring, result visibility
- [Authentication](authentication.md) — access/refresh tokens, verification,
  account deletion
- [API reference](api.md) — endpoint conventions, error format, rate limits
- [OpenAPI specification](openapi.yaml) — machine-readable API contract
- [Testing](testing.md) — unit, integration, E2E, CI equivalence
- [Deployment](deployment.md) — images, topologies, upgrade, rollback
- [Operations](operations.md) — health, metrics, backups, incident response
- [Database migrations](database-migrations.md) — rules, execution, recovery
- [Security and privacy](security-and-privacy.md) — model, data inventory,
  operator obligations
- [Troubleshooting](troubleshooting.md) — frequent issues and solutions
- [Hosted deployment runbook](runbooks/hosted-deployment.md) — Hetzner,
  Cloudflare, CI/CD, launch, and recovery checklist

## Repository

See [README.md](../README.md) for the project overview,
[CONTRIBUTING.md](../CONTRIBUTING.md) for contribution guidelines,
[SECURITY.md](../SECURITY.md) for vulnerability reporting, and
[LICENSE](../LICENSE) for licensing.
