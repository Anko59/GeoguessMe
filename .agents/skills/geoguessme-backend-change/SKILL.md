---
name: geoguessme-backend-change
description:
    Use when changing GeoGuessMe backend behavior, handlers, services,
    repositories, database queries or migrations, authentication, chat, storage,
    or configuration. Activates for Go code, SQL, and backend test work.
---

# GeoGuessMe backend change

## Workflow

1. Follow [backend/AGENTS.md](../../../backend/AGENTS.md), including the bounded
   maintenance scan and change-impact workflow in
   [docs/agent-engineering.md](../../../docs/agent-engineering.md).
2. Locate the responsibility: transport in `backend/handlers/`, persistence in
   `backend/internal/repository/`, business rules in the owning
   `backend/internal/*/` package.
3. Add or extend unit tests first; use live-database tests in
   `backend/integration_test/` for repository and cross-package behavior.
4. For schema changes, add a forward-only migration in
   `backend/internal/database/migrations/` and keep `make lint-sql` green.
5. Do not add SQL to handlers, do not read environment variables outside
   `backend/internal/config/`, and do not introduce mutable package globals.
6. Run `make impact BASE=origin/dev`, `make test-backend`, `make test-race`,
   focused integration tests, then `make preflight`. Run `make verify` when
   composition, startup, migrations, or tests change.

## Inputs

- The behavior change description and the affected endpoint or flow.

## Outputs

- Go changes with unit and integration tests.
- Migration files when schema changes, with fixtures.
- Gate evidence: `test-backend`, `test-race`, `test-integration`, `preflight`.

## References

- [backend/AGENTS.md](../../../backend/AGENTS.md) — backend working rules.
- [docs/architecture.md](../../../docs/architecture.md) — components and flows.
- [docs/testing.md](../../../docs/testing.md) — gate matrix.
- [docs/database-migrations.md](../../../docs/database-migrations.md) —
  migration rules.
- `make test-backend`, `make test-race`, `make test-integration`,
  `make preflight`.
