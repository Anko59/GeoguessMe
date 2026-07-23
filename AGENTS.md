# Repository working agreement

These rules apply to humans and AI coding agents alike.

## Production quality

All code and configuration must be production-ready. Do not add knowingly
incomplete behavior, placeholder production logic, ignored errors, unsafe
defaults, flaky synchronization, or undocumented operational assumptions.
Consider security, authorization, data integrity, failure handling,
observability, migrations, rollback, and compatibility for every relevant
change.

## Dockerized workflow

Dockerized Make targets are the required repository interface. Use a Make target
whenever one exists. Do not run project compilers, package managers, linters,
formatters, Playwright, migration tools, or test runners directly on the host.
Add or improve a Dockerized Make target instead of documenting a host command.
The supported host prerequisites are Git, Make, Docker, and Docker Compose.

Start with the repository navigation points: [README](README.md),
[documentation index](docs/index.md), [deployment guide](deployment/README.md),
[testing guide](docs/testing.md), and `make help`.

## Hooks

Run `make hooks-install` and `make hooks-check` before making commits. Never use
`git commit --no-verify`, `git push --no-verify`, temporary hook removal, hook
edits, or another bypass. Never weaken, suppress, or skip a failing gate to make
a commit pass; fix the underlying code, test, documentation, or tool
configuration.

The commit hook runs repository-wide formatting, structure, and lint checks. The
push hook runs `make preflight`; it deliberately does not duplicate the complete
operational gate that CI runs after merge to `dev`.

## Structure

No human-authored tracked file may exceed 500 lines. No directory may directly
contain more than 14 code or configuration files. Refactor before crossing
either limit. Generated files are excluded only when the committed allowlist and
generated marker identify them; vendored dependencies and binary media are also
excluded as described by the structural checker.

## Testing

Add tests for every behavior and regression. Use deterministic state-based
synchronization, never unconditional sleeps. Do not use retries to hide flaky
tests. Run focused relevant tests during development and `make preflight` before
handoff. Pull-request CI selects backend integration and Chromium E2E from
changed paths. The complete `make verify` suite runs once on the exact `dev`
revision before development deployment and nightly. Run it locally when changing
deployment, test infrastructure, or the gates themselves. Do not claim
production readiness unless the exact revision has a successful complete gate.

## Handoff

Leave the working tree clean. Commit all intended changes in coherent commits.
Do not commit caches, reports, coverage files, binaries, backups, secrets, or
test data. Report the exact Make targets run and their results, CI evidence for
the exact revision, and commit IDs for the intended handoff.

## Branch and release flow

Feature and dependency pull requests target `dev`. Only the repository `dev`
branch may open a release pull request to `main`. Both protected branches
require signed commits, strict checks, and squash merging; do not push directly
to either branch. Production promotes the exact signed image digests that passed
the complete dev gate instead of rebuilding source.

## Documentation

Update the README, API documentation, deployment instructions, and operational
documentation whenever behavior or interfaces change. Keep local development,
testing, contributing, and deployment documentation aligned with the Docker-only
workflow without duplicating this entire file.
