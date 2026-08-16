---
name: geoguessme-frontend-change
description:
    Use when changing GeoGuessMe frontend components, hooks, pages, chat,
    camera, game flow, styles, accessibility behavior, or the API client and
    wire types. Activates for React, TypeScript, Vitest, and E2E work.
---

# GeoGuessMe frontend change

## Workflow

1. Follow [frontend/AGENTS.md](../../../frontend/AGENTS.md), including the
   bounded maintenance scan and change-impact workflow in
   [docs/agent-engineering.md](../../../docs/agent-engineering.md).
2. Locate the component: feature components in `frontend/src/components/`,
   shared logic in `frontend/src/hooks/`, wire types in `frontend/src/types/`
   (generated from the OpenAPI contract into
   `frontend/src/types/openapi.generated.ts`), and the client in
   `frontend/src/api.ts`.
3. Keep data flowing down: pages compose components, components use hooks, and
   hooks call the client. Do not duplicate wire fields in view components.
4. Manage browser resources explicitly: every stream track, animation frame,
   timer, socket, and `URL.createObjectURL` has one owner and one cleanup path.
5. Preserve accessibility: keyboard equivalence, focus management, ARIA
   semantics, and reduced-motion behavior.
6. Add Vitest coverage for behavior; add an E2E scenario in `frontend/e2e/` only
   for a user journey. No `act(...)` warnings, no unconditional sleeps.
7. Run `make impact BASE=origin/dev`, `make test-frontend`,
   `make lint-frontend`, `make type-check`, then `make preflight`. Add an E2E
   scenario only for a user journey; the browser suite itself runs in PR CI and
   the dev gate, not locally.

## Inputs

- The UI or behavior change description and the owning component.

## Outputs

- React/TypeScript changes with Vitest tests.
- E2E scenario updates when a journey changes.
- Gate evidence: `test-frontend`, `lint-frontend`, `type-check`; E2E evidence is
  produced by PR CI.

## References

- [frontend/AGENTS.md](../../../frontend/AGENTS.md) — frontend working rules.
- [docs/testing.md](../../../docs/testing.md) — gate matrix.
- [docs/architecture.md](../../../docs/architecture.md) — request flows.
- `frontend/src/api.ts` — API client.
- `make test-frontend`, `make lint-frontend`, `make type-check`,
  `make openapi-check`, `make test-e2e`.
