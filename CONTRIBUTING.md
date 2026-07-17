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

The repository-wide structural limit is 500 lines per human-authored file and 14
direct code/configuration children per directory. The tracked structural checker
reports every violation with its observed value, limit, and classification.

## Development and handoff

Use make format, focused Dockerized test targets, and make quality during
development. Before handoff, run make verify, confirm make hooks-check passes,
commit all intended changes in coherent commits, and leave git status --short
empty. Do not use --no-verify, remove hooks, weaken a gate, or suppress a
failure.

Report the exact targets and results. Do not claim production readiness unless
the complete verification gate passes.

## Test expectations

Add regression coverage for every behavior change. Prefer role, label, and
stable test-ID selectors. Synchronize on observable application state; do not
use unconditional sleeps, positional selectors, or retries to conceal flaky
behavior. See docs/testing.md.

## Repository map

- Backend and unit tests: backend/
- Frontend and unit tests: frontend/
- Isolated integration tests: backend/integration_test/
- Playwright scenarios: frontend/e2e/
- API contract: docs/openapi.yaml and docs/openapi/
- Deployment: deployment/README.md
- Operations: docs/operations.md
- Repository rules: AGENTS.md

Use Conventional Commits and keep commits logically scoped.
