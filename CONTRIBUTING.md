# Contributing

GeoGuessMe uses Dockerized Make targets as its only project-tool interface.
Install Git, Make, Docker, and Docker Compose on the host; do not install or
invoke Go, Node, npm, Playwright, Python, or quality tools directly.

## Setup

```text
make bootstrap
make dev
make hooks-install
make hooks-check
```

## Development and handoff

Use `make format`, focused Dockerized test targets, and `make quality` during
development. Before handoff, run `make verify`, confirm `make hooks-check`
passes, commit all intended changes in coherent commits, and leave
`git status --short` empty. Do not use `--no-verify`, remove hooks, weaken a
gate, or suppress a failure.

Report the exact targets and results. Do not claim production readiness unless
the complete `make verify` gate passes (see [docs/testing.md](docs/testing.md)
for expected results per target).

## Test expectations

Add regression coverage for every behavior change. Prefer role, label, and
stable test-ID selectors. Synchronize on observable application state; do not
use unconditional sleeps, positional selectors, or retries to conceal flaky
behavior. See [docs/testing.md](docs/testing.md).

## Repository map

| Area                | Path                             |
| ------------------- | -------------------------------- |
| Backend code/tests  | backend/                         |
| Frontend code/tests | frontend/                        |
| Integration tests   | backend/integration_test/        |
| E2E Playwright      | frontend/e2e/                    |
| API contract        | docs/openapi.yaml, docs/openapi/ |
| Deployment          | deployment/README.md             |
| Operations          | docs/operations.md               |
| Working agreement   | AGENTS.md                        |

Use [Conventional Commits](https://www.conventionalcommits.org/) and keep
commits logically scoped.

## Repository rules

See [AGENTS.md](AGENTS.md) for the full working agreement: production quality,
Docker-only workflow, hooks, structure limits, testing requirements, and handoff
expectations.
