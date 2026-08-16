# Backend working rules

Go backend at `backend/`. Applies in addition to the root
[AGENTS.md](../AGENTS.md) and the engineering guide
([docs/agent-engineering.md](../docs/agent-engineering.md)).

## Composition and dependencies

- `backend/app.go` is the dependency composition root: it holds every injected
  runtime dependency (auth service, repositories, store, mailer, push, hub,
  clock) and owns route registration. `backend/main.go` validates configuration,
  opens the pool, starts background workers, and owns process lifecycle.
- Do not add mutable package-level globals. The `database.DB` global, the
  handler runtime globals, and the package-level auth token state were removed
  in PR 7; new code must reach dependencies through the composition root. The
  sole allowlisted exception is the rate-limiter singleton in
  `backend/internal/middleware/rate_limit.go` (process-global rate-limiting
  infra; to be formalized in PR 14's architecture-checker allowlist).
- Persistence belongs in `backend/internal/repository/` (and its responsibility
  sub-packages such as `backend/internal/repository/chat/`,
  `backend/internal/repository/groups/`). Handlers perform transport parsing,
  authorization delegation, service calls, and response writing. Never add SQL
  to a handler.
- `backend/internal/config/` is the only package that reads environment
  variables. Store construction and runtime packages must not call `os.Getenv`.

## Data and migrations

- Migrations in `backend/internal/database/migrations/` are forward-only. Never
  edit an applied migration; add a new file. Keep the `migration-test` gate
  green by including fixtures for new migrations.
- Preserve the JSON error envelope, status codes, ordering, and pagination
  shapes unless a PR explicitly changes the contract.

## Commands and tests

- Focused Go tests: `make test-backend` (unit) and `make test-race`. Live
  database integration lives in `backend/integration_test/` and runs with
  `make test-integration`.
- Lint with `make lint-go`; validate migrations with `make lint-sql`.
- Run `make preflight` before handoff. `make verify` is reserved for changes to
  deployment, infrastructure, CI workflows, or the quality gates themselves;
  application code and its tests are covered by PR CI and the dev gate and never
  require a local `make verify`.

## Structure

Keep production files below 500 lines and directories below 14 direct
code/config files. Split by responsibility, never into a shared `utils`/`common`
dumping ground.
