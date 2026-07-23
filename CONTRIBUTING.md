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

Create feature branches from `dev` and target pull requests to `dev`. External
contributors normally work from a fork; trusted collaborators may create a
branch in this repository. Production release PRs use a short-lived repository
`release/*` branch based on `main` whose resulting Git tree exactly matches the
successfully deployed `dev` tree. CI rejects every other source or tree.

Use `make format` and focused Dockerized test targets during development. Before
pushing, run `make preflight` and confirm `make hooks-check` passes.
Pull-request CI adds backend integration or Chromium E2E according to changed
paths. The exact merged `dev` revision runs `make verify` once before it is
published and deployed. Do not duplicate that complete gate locally unless
changing deployment, test infrastructure, or the gates themselves.

Both `dev` and `main` require signed, verified commits. GitHub's protected
squash merge creates the verified commit accepted by those branches. Never use
`--no-verify`, remove hooks, weaken a gate, or suppress a failure.

Report the exact targets and results, commit IDs, and CI evidence. Do not claim
production readiness unless the exact revision passed `make verify` (see
[docs/testing.md](docs/testing.md) for the gate matrix).

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
