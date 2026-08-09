---
name: geoguessme-api-contract
description:
    Use when changing the GeoGuessMe API contract, the OpenAPI specification,
    handler request/response DTOs, contract tests, or the frontend API client
    and wire types. Activates for endpoint, schema, error-envelope, and
    wire-type tasks.
---

# GeoGuessMe API contract

## Workflow

1. Follow the bounded maintenance scan and change-impact workflow in
   [docs/agent-engineering.md](../../../docs/agent-engineering.md).
2. Read the canonical contract: [docs/api.md](../../../docs/api.md) and
   `docs/openapi.yaml` (split sources under `docs/openapi/`).
3. Locate the endpoint: the OpenAPI path entry, the matching handler in
   `backend/handlers/`, and its client call in `frontend/src/api.ts`.
4. Change the contract, the handler DTOs, and the frontend wire types together
   in the same PR. Wire types in `frontend/src/types/` are generated from the
   OpenAPI schemas (`frontend/src/types/openapi.generated.ts`); after a schema
   change, run `make openapi-generate` to regenerate them and add/adjust narrow
   view-model aliases in `frontend/src/types/index.ts` only when the client
   needs a shape the wire contract does not express.
5. Keep the JSON error envelope and status codes stable unless the PR explicitly
   removes a compatibility path.
6. Add or update contract tests: handler tests for request parsing and response
   mapping, plus the `hosted-contract-test` gate.
7. Run `make impact BASE=origin/dev`, `make lint-openapi`, the focused handler
   tests, `make preflight`, and (for compatibility removals) `make verify`.

## Inputs

- The changed endpoint or schema description.
- Existing OpenAPI sources under `docs/openapi/` and handler DTOs.

## Outputs

- Updated `docs/openapi.yaml` and split sources.
- Updated handler DTOs and frontend client/wire types.
- Contract regression tests and lint evidence.

## References

- [docs/api.md](../../../docs/api.md) — endpoint conventions and error format.
- [docs/authentication.md](../../../docs/authentication.md) — auth headers and
  cookies.
- [docs/index.md](../../../docs/index.md) — documentation index.
- `backend/handlers/response.go` — response envelope helpers.
- `make lint-openapi`, `make openapi-generate`, `make openapi-check`,
  `make preflight`.
