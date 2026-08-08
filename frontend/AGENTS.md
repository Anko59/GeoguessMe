# Frontend working rules

React + TypeScript + Vite at `frontend/`. Applies in addition to the root
[AGENTS.md](../AGENTS.md) and the engineering guide
([docs/agent-engineering.md](../docs/agent-engineering.md)).

## Ownership

- Data flows down: pages compose components, components use hooks, and hooks
  call the client in `frontend/src/api.ts`. Wire shapes live in
  `frontend/src/types/`; do not restate wire fields in ad-hoc component types.
  Keep view models and local UI state separate from wire types.
- One component owns one responsibility. Large orchestration units are split by
  lifecycle (chat, camera, game) rather than accumulating rendering plus state
  plus device access in one file.

## Object URL lifecycle

- Every `URL.createObjectURL` must have exactly one owner that revokes the URL
  on replacement, eviction, or unmount. Pair creation and revocation in the same
  effect or reducer path. Leaked object URLs are regressions.

## Accessibility and behavior

- Preserve keyboard equivalence, focus management, and ARIA semantics for every
  interaction (long-press, swipe, modal, toggles).
- No React `act(...)` warnings and no state updates after unmount. Tests await
  real state transitions; never suppress warnings to make a suite green.

## Commands and tests

- Unit tests run with `make test-frontend` (Vitest). Use deterministic
  fake-clock/socket tests; never unconditional sleeps or retries.
- Lint with `make lint-frontend` (ESLint, zero warnings) and `make type-check`.
- E2E (Playwright) runs with `make test-e2e`. Add an E2E scenario only for a
  user journey; prefer unit tests for isolated behavior.
- Do not add a runtime state-management or data-fetching framework pre-GA.
