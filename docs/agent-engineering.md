# Agent engineering guide

This guide gives AI coding agents the same orientation a senior engineer has:
where responsibilities live, which files a task touches, which rules are
invariants, and how to prove a change is safe. Human readers should prefer the
[documentation index](index.md). This guide links to canonical documents instead
of copying them.

## Repository navigation

Start with these canonical entry points, then drill into the map below.

- [README](../README.md) — project overview and quick start.
- [Documentation index](index.md) — every human-facing document.
- [Architecture](architecture.md) — components, trust boundaries, request flows.
- [Local development](local-development.md) — `make dev`, prerequisites.
- [Testing](testing.md) — the gate matrix and how to run focused tests.
- [Deployment guide](../deployment/README.md) — images, topologies, upgrades.
- `make help` — the complete Dockerized target list.

### Package and component map

| Area                | Location                                                                 | Responsibility                                                                                                                                                                |
| ------------------- | ------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Backend entry       | `backend/main.go`                                                        | Process lifecycle: config, pool, workers, server, shutdown                                                                                                                    |
| Backend composition | `backend/app.go`                                                         | Dependency composition root: the `App` type holds every injected runtime dependency (auth service, repositories, store, mailer, push, hub, clock) and owns route registration |
| Backend routing     | `backend/routes.go`                                                      | Route table and middleware wiring                                                                                                                                             |
| HTTP transport      | `backend/handlers/`, `backend/handlers/auth/`                            | Request parsing, authorization delegation, response mapping (migrated slices: `GroupAPI`, `ChatAPI`, `GameAPI`, `AuthAPI`)                                                    |
| Authentication      | `backend/internal/auth/`                                                 | Token issuance/validation, sessions, authorization checks                                                                                                                     |
| Realtime chat       | `backend/internal/chat/`                                                 | WebSocket hub, clients, message dispatch                                                                                                                                      |
| Chat persistence    | `backend/internal/repository/chat/`                                      | Messages, reactions, chat media, WebSocket tickets (injected pool)                                                                                                            |
| Configuration       | `backend/internal/config/`                                               | Sole environment-variable reader; validated settings                                                                                                                          |
| Database            | `backend/internal/database/`                                             | Pool access and forward-only SQL migrations                                                                                                                                   |
| Ranking math        | `backend/internal/elo/`                                                  | Elo updates for guesses                                                                                                                                                       |
| Email               | `backend/internal/email/`                                                | Mailer (verification, resets)                                                                                                                                                 |
| Scoring             | `backend/internal/game/`                                                 | Guess scoring and timing rules                                                                                                                                                |
| Media processing    | `backend/internal/media/`                                                | Image normalization and validation                                                                                                                                            |
| HTTP middleware     | `backend/internal/middleware/`                                           | CORS, rate limits, security headers                                                                                                                                           |
| Domain types        | `backend/internal/models/`                                               | Shared wire and domain structs                                                                                                                                                |
| Progression         | `backend/internal/progression/`                                          | Rank badge tiers                                                                                                                                                              |
| Web Push            | `backend/internal/push/`                                                 | VAPID keys, notifier, sender                                                                                                                                                  |
| Persistence         | `backend/internal/repository/`                                           | PostgreSQL queries per aggregate (users, groups, messages, photos, tickets, cleanup)                                                                                          |
| Object storage      | `backend/internal/storage/`                                              | Local and S3-compatible media storage                                                                                                                                         |
| Validation          | `backend/internal/validation/`                                           | Input validation helpers                                                                                                                                                      |
| Backend integration | `backend/integration_test/`                                              | Live-database integration tests                                                                                                                                               |
| Frontend pages      | `frontend/src/pages/`                                                    | Route-level views (account, auth, groups, home, profile)                                                                                                                      |
| Frontend components | `frontend/src/components/`                                               | Feature components: camera, chat, common, game, leaderboard, map, navigation, progression, pwa, settings, ui                                                                  |
| Frontend hooks      | `frontend/src/hooks/`                                                    | Cross-component logic (for example `useGroupMessages`)                                                                                                                        |
| Frontend context    | `frontend/src/context/`                                                  | AuthProvider and auth state                                                                                                                                                   |
| API client          | `frontend/src/api.ts`                                                    | HTTP and WebSocket client, request shaping                                                                                                                                    |
| Wire types          | `frontend/src/types/index.ts`, `frontend/src/types/openapi.generated.ts` | Wire DTOs generated from the OpenAPI contract, re-exported under stable names; narrow client view-model aliases                                                               |
| Frontend utilities  | `frontend/src/utils/`                                                    | `pwaSessionCache` and similar focused helpers                                                                                                                                 |
| Push subscriptions  | `frontend/src/push/`                                                     | Web Push registration                                                                                                                                                         |
| E2E suite           | `frontend/e2e/`                                                          | Playwright scenarios keyed to user journeys                                                                                                                                   |
| Compose/deploy      | `deployment/`                                                            | Compose files, Caddy, Docker images, deploy scripts                                                                                                                           |
| Hosted infra        | `infra/terraform/`, `infra/cloud-init/`                                  | Terraform and cloud-init for the hosted deployment                                                                                                                            |
| Quality gates       | `tools/quality/`                                                         | structure-check, durable architecture checker (`tools/quality/archcheck`), hooks, regression tests                                                                            |

### Dependency direction and ownership

- The backend dependency composition root is `backend/app.go`: the `App` struct
  holds every injected dependency and registers routes. `main.go` validates
  configuration, creates the pool, starts workers, and owns process lifecycle.
  Handlers delegate persistence to `backend/internal/repository/` (and its
  responsibility sub-packages such as `backend/internal/repository/chat/`) and
  business rules to the owning internal package. Never add SQL to a handler and
  never read an environment variable outside `backend/internal/config/`.
- Mutable package globals are banned. All backend slices read their dependencies
  from the composition root: the `database.DB` global, the handler runtime
  globals, and the package-level auth token state were removed in the PR 7
  migration. Only `backend/internal/config/` reads environment variables. The
  explicit allowlist `tools/quality/archcheck/mutable-globals.allowlist`
  contains only the mutex-protected process-wide rate-limit store and private
  static lookup tables that Go cannot express as constants. Every entry carries
  its ownership justification and is enforced by the durable architecture
  checker (`make archcheck`, `make test-archcheck-regression`).
- Frontend data flows down: pages compose components, components use hooks, and
  hooks call `api.ts`. Wire shapes live in `frontend/src/types/`; view models
  and local UI state are separate from wire types.
- Every quality gate is a Dockerized Make target. There is no host-side tool
  invocation path in this repository.

## Change-impact matrix

| Task                            | Primary files                                                                                                                       | Focused tests                                               | Gates                                                                                |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| API contract or endpoint change | `docs/openapi.yaml`, `docs/openapi/`, `backend/handlers/`, `backend/internal/models/`, `frontend/src/api.ts`, `frontend/src/types/` | Handler tests, `hosted-contract-test`                       | `make lint-openapi`, `make openapi-generate`, `make openapi-check`, `make preflight` |
| Backend behavior change         | `backend/handlers/`, `backend/internal/*/`                                                                                          | Package unit tests, `backend/integration_test/`             | `make test-backend`, `make test-race`, `make test-integration`                       |
| Backend composition change      | `backend/app.go`, `backend/main.go`, `backend/handlers/`, `backend/internal/*/`                                                     | Composition tests, archcheck fixtures                       | `make archcheck`, `make test-archcheck-regression`, `make preflight`, `make verify`  |
| Database migration              | `backend/internal/database/migrations/` (new file only), `backend/internal/repository/`                                             | Migration fixtures, repository DB tests                     | `make lint-sql`, `make verify`                                                       |
| Chat behavior                   | `frontend/src/components/chat/`, `frontend/src/hooks/useGroupMessages.ts`, `backend/handlers/chat.go`, `backend/internal/chat/`     | `Chat.test.tsx`, `useGroupMessages.test.ts`, `chat.spec.ts` | `make test-frontend`, `make test-e2e`                                                |
| Camera behavior                 | `frontend/src/components/camera/`                                                                                                   | Camera lifecycle and capture tests                          | `make test-frontend`, `video-challenge` E2E                                          |
| Game flow                       | `frontend/src/components/game/`, `backend/handlers/game.go`                                                                         | `Game.test.tsx`, `challenge.spec.ts`                        | `make test-frontend`, `make test-e2e`                                                |
| Configuration change            | `backend/internal/config/`, `backend/main.go`                                                                                       | Config validation tests                                     | `make preflight`, `make verify` (startup behavior)                                   |
| Deployment change               | `deployment/`, `infra/terraform/`, `infra/cloud-init/`                                                                              | Terraform tests, rehearsal scripts                          | `make terraform-fmt-check`, `make terraform-test`, `make lint-caddy`, `make verify`  |
| Documentation change            | `docs/`                                                                                                                             | docs/agent-config checker regression tests                  | `make lint-docs`                                                                     |

The Gates column lists the complete gate set, including steps the post-merge
development pipeline runs on the merged revision. A `make verify` entry in that
column is executed there (as the parallel gate jobs), not locally; the local
budget is defined in [Local verification budget](#local-verification-budget).

## Canonical commands

Everything below runs in Docker through Make. Never invoke Go, Node, npm,
linters, Playwright, or migration tools directly on the host. Targets that boot
a disposable stack (`make test-integration`, `make test-e2e`) are listed for
reference: PR CI selects them from changed paths and the dev gate runs them on
the merged revision, so they are never part of the local budget.

- API work: `make lint-openapi` validates the split OpenAPI contract with
  Redocly; `make preflight` runs `hosted-contract-test` against the running
  stack.
- Backend work: `make test-backend`, `make test-race`, `make test-integration`,
  `make lint-go`, `make build-backend`.
- Frontend work: `make test-frontend`, `make lint-frontend`, `make type-check`,
  `make build-frontend`, `make test-e2e`. After any OpenAPI schema change,
  regenerate the frontend wire types with `make openapi-generate`;
  `make openapi-check` is the drift gate that fails when the committed contract
  (`frontend/src/types/openapi.generated.ts`) no longer matches
  `docs/openapi.yaml`.
- Database work: add a forward-only migration in
  `backend/internal/database/migrations/` and run `make lint-sql`;
  `migration-test` runs inside `make verify` in the dev gate.
- Infrastructure work: `make terraform-fmt-check`, `make terraform-test`,
  `make lint-caddy`; release rehearsals live in `make verify`
  (`backup-rehearsal`, `restart-rehearsal`, `reconnect-rehearsal`).
- Documentation work: `make lint-docs` runs Markdownlint and the
  [docs/agent-config checker](agent-engineering.md#docsagent-config-checker);
  `make format-check` enforces Prettier and shfmt.
- Complete gates: `make preflight` is the fast local and pull-request gate;
  `make verify` is the complete release gate. Application code or its tests
  never require a local `make verify`: PR CI runs the selected live-stack suites
  and the dev gate runs the complete suite on the merged revision before
  deployment. Test-only changes run the same focused suites as the code they
  test. `make verify` is reserved for changes to `deployment/`, `infra/`, the CI
  workflows, or the quality gates themselves. Install and verify hooks with
  `make hooks-install` and `make hooks-check`.
- Architecture rules: `make archcheck` runs the durable backend architecture
  checker (no production package-level mutable application dependencies, no SQL
  in HTTP handlers, no environment reads outside `backend/internal/config`). It
  is wired into `make preflight` and `make quality`; contract currentness and
  agent/documentation reference validity are enforced by `make openapi-check`
  and `make lint-docs` in the same gates.

## Local verification budget

Run the minimum targets that match the change below, then `make preflight`
before pushing. Anything beyond the table — the live integration stack, browser
E2E, rehearsals, the full release gate — is deferred to PR CI and the dev gate
on the merged revision. That deferral is a deliberate trust boundary, not a
shortcut: the exact merged tree is verified in full before development
deployment, and again before nightly.

| Change type                      | Minimum local targets                                                               |
| -------------------------------- | ----------------------------------------------------------------------------------- |
| Backend behavior or its tests    | `make test-backend`, `make lint-go`; add `make test-race` when touching concurrency |
| Frontend behavior or its tests   | `make test-frontend`, `make lint-frontend`, `make type-check`                       |
| API contract                     | `make lint-openapi`, `make openapi-generate`, `make openapi-check`                  |
| Database migration               | `make lint-sql` and the focused migration fixture tests                             |
| Documentation                    | `make preflight-docs`                                                               |
| Deployment/infra/workflows/gates | Focused targets plus `make verify` — the one change type that requires it locally   |
| Every non-docs change            | `make preflight` before pushing (enforced by the pre-push hook)                     |

## CI waiting protocol

- Push at most once per meaningful state. Accumulate related edits and commits
  into one push instead of pushing after every commit; every push re-runs the PR
  gate.
- After pushing, continue with other work. Do not poll CI: never run
  `gh run watch` loops or repeated `gh pr checks` calls.
- Consume CI results at the next natural touchpoint: when you return to the PR
  to address feedback, when another task completes, or when a check failure
  notification arrives.
- When a PR is believed green, prefer enabling auto-merge over watching it
  merge.

## Cross-cutting invariants

Treat these as laws of the repository. Changing one is a contract change that
requires its own review, tests, and documentation.

- Authentication: access and refresh tokens use httpOnly cookies with rotation;
  email verification and reset flows are required for account recovery. See
  [authentication](authentication.md).
- Private media access: challenge photos and game media are readable only by
  members of the target group, and only while the media lifecycle allows it.
  Media access checks must not be duplicated with subtly different rules.
- Object URL cleanup: every `URL.createObjectURL` in the frontend must have one
  owner that revokes it on replacement, eviction, or unmount. Leaked object URLs
  are regressions.
- Cursor ordering: chat pagination uses an opaque cursor with stable ordering.
  Forward catch-up sends the `stable_cursor` of the last received page as the
  `cursor` parameter; the legacy `after_id` parameter was removed by the
  compatibility-removal PR and must not be reintroduced.
- Challenge expiry: reveal timing derives from `location_reveals_at` on the wire
  contract. The UI must not hardcode durations that the API already publishes.
- Configuration ownership: `backend/internal/config/` is the only reader of
  environment variables. The composition root is the only constructor of
  application dependencies.
- Forward-only migrations: database migrations in
  `backend/internal/database/migrations/` are append-only. Never edit an applied
  migration; add a new one.
- Error contract: the HTTP JSON error envelope, status codes, ordering, and
  pagination shapes are stable. Do not change them outside an explicit
  compatibility-removal PR.
- Structure limits: no tracked human-authored file exceeds 500 lines and no
  directory holds more than 14 direct code or configuration files. Generated
  files need the committed allowlist marker.
- Quality interface: all formatting, linting, testing, and building happens
  through Dockerized Make targets. Never bypass hooks or gates.

## Bounded maintenance scan

Agents own the quality of the code immediately surrounding their change, not
only the shortest implementation of the prompt. Before handoff, inspect the
changed symbols, their callers, tests, configuration, and nearby comments for:

- duplicated rules or data transformations;
- dead branches, stale compatibility code, abandoned bug-fix scaffolding, and
  comments that no longer describe behavior;
- hardcoded policy that already has a canonical configuration or wire value;
- responsibilities in the wrong layer or dependencies that cross the package map
  in the wrong direction;
- missing failure, cleanup, concurrency, boundary, or regression coverage.

An adjacent cleanup may join the current PR only when all of these are true:

1. It is behavior-preserving and contained in the same owning module.
2. It changes no API, schema, migration, dependency, deployment, or
   configuration contract.
3. The current focused tests cover it, or a deterministic regression test is
   added.
4. It is a small minority of the diff and does not obscure the requested change.

Otherwise, record the finding in the pull request with an issue or a durable
roadmap/compatibility-ledger reference. Do not leave a bare `TODO` or `FIXME`,
silently discard the finding, or broaden the task without authorization.

Deferred code work uses an owned marker such as `TODO(#123)`,
`FIXME(https://github.com/OWNER/REPOSITORY/issues/123)`, or
`HACK(docs/agent-engineering.md#compatibility-ledger)`. The lint gate rejects
bare markers in code and configuration. `make maintenance-report` prints a
compact, agent-readable view of owned markers and files or directories nearing
the structural limits; the nightly workflow publishes it in the run summary. The
frontend dead-code gate covers unused files, exports, dependencies, unlisted
imports, and unresolved imports. Its small dependency exception list contains
only tools invoked externally by Dockerized Make recipes.

## Change-impact manifest

Run `make impact BASE=origin/dev` before selecting focused validation. The
manifest maps changed paths to capabilities (`backend_unit`, `frontend_unit`,
`backend_integration`, `browser_e2e`, and `operational`) and preserves the
conservative `backend`, `frontend`, and `full` decisions consumed by CI. Unknown
paths deliberately request the complete scope. The capability list IS the local
budget: run the focused targets for the selected capabilities and leave the rest
to PR CI and the dev gate; capabilities not selected by changed paths are
deferred to CI, exactly as the workflow does. The manifest helps explain why a
gate is needed; it never authorizes skipping a gate required by this guide, a
scoped `AGENTS.md`, or CI.

## Frontend cache and object-URL ownership

Every module-scoped frontend cache has a single owner that documents its key,
lifetime, eviction, and object-URL revocation. Blob URLs must be revoked exactly
once (see the Object URL cleanup invariant above).

| Cache                      | Owner                                                                                    | Key                            | Lifetime / invalidation                                            | Eviction bound                                         | URL revocation                                                                      |
| -------------------------- | ---------------------------------------------------------------------------------------- | ------------------------------ | ------------------------------------------------------------------ | ------------------------------------------------------ | ----------------------------------------------------------------------------------- |
| Avatar photos              | `frontend/src/components/common/avatarCache.ts` (shared `utils/objectUrlCache.ts` store) | user id                        | `bustAvatarCache` on avatar upload; session-scoped otherwise       | none (bounded by distinct users rendered per session)  | `store.bust` revokes the blob URL                                                   |
| Group photos               | `frontend/src/pages/groups/groupPhotoCache.ts` (shared store)                            | group id                       | `bustGroupPhotoCache` on photo update                              | none (bounded by distinct groups rendered per session) | `store.bust` revokes the blob URL; the static `/logo.png` fallback is never revoked |
| Leaderboards               | `frontend/src/components/leaderboard/leaderboardCache.ts`                                | `userID:groupID:period:metric` | 60s TTL; `clearLeaderboardCache` is the explicit invalidation path | max 50 entries, oldest evicted                         | n/a (plain data)                                                                    |
| PWA session + message hint | `frontend/src/utils/pwaSessionCache.ts`                                                  | localStorage keys              | `clearCachedSession` on logout                                     | 200 cached messages per user                           | n/a                                                                                 |
| Challenge media (Game)     | `Game` component (`mediaUrlRef` in `frontend/src/components/game/Game.tsx`)              | per challenge                  | replaced on state transition, close, or unmount                    | n/a                                                    | revoked exactly once when replaced, cleared, or unmounted                           |
| Chat attachments           | `ChatAttachment`                                                                         | per media id                   | replaced on media id change                                        | n/a                                                    | revoked on change/unmount                                                           |
| Recorded video             | `useVideoRecording` (camera)                                                             | per recording                  | replaced per take                                                  | n/a                                                    | revoked on replace/unmount                                                          |

The shared `createObjectUrlStore` (`frontend/src/utils/objectUrlCache.ts`) is
used only by caches with identical lifecycle semantics (avatar and group
photos); data caches with different lifetimes (leaderboard, PWA session) must
not use it.

## Compatibility ledger

Temporary compatibility paths are listed here with their tests, replacement, and
removal condition. Remove an entry only when its code, schema, docs, and tests
are gone together.

The PR 7 entries for the `CleanupAuthTokens` no-op reference, the `InitSchema`
compatibility helper, and the `database.DB` package global were removed in PR 7:
the code, its tests, and the composition-root replacement landed together.

The application compatibility PR entries for the `after_id` message cursor, the
reaction `emoji` request/response alias, and the singular challenge `group_id`
form field were removed: the OpenAPI spec, generated TypeScript contract,
handlers, models, frontend, and tests are removed together. The legacy reaction
database column and synchronization objects remain until a separate
cleanup-migration PR lands after the compatible application revision has
deployed and left its rollback window. Forward catch-up uses the opaque
`stable_cursor` anchor plus the `cursor` parameter. The staged rollout is
documented in [deployment.md](deployment.md). No temporary application
compatibility entries remain.

## Residual risks

- The frontend video-recording guard (`MAX_VIDEO_BYTES` in
  `frontend/src/components/camera/capture/useVideoRecording.ts`) mirrors the
  backend `UPLOAD_MAX_BYTES` default (10 MiB) as a pre-upload UX stop. The
  server stays authoritative (`http.MaxBytesReader`); the client cap exists only
  because no bootstrap/config endpoint serves the effective limit today.
  Sourcing the limit from a dedicated config/bootstrap endpoint is a candidate
  follow-up PR (roadmap item J in PR 3); it was deliberately not added to the
  config refactor because no such endpoint exists yet.

## Where to start

| Task                | Start here                                                                                 | Then                                                                                                                |
| ------------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------- |
| Bug report          | Reproduction path in `docs/troubleshooting.md` and the owning component in the package map | Write a failing test, fix, run focused tests and `make preflight`                                                   |
| New feature         | Change-impact matrix row matching the subsystem                                            | Follow the scoped `AGENTS.md` for the layer, add tests, run the matching gates                                      |
| Refactor            | The owning package plus its tests and the [compatibility ledger](#compatibility-ledger)    | Move one responsibility at a time; keep contracts stable unless the PR says otherwise                               |
| Production incident | `docs/operations.md` and `docs/runbooks/`                                                  | Follow the runbook; document the incident as an archival record                                                     |
| Dependency update   | `frontend/package.json`, `backend/go.mod`                                                  | Use `make deps-npm-security-update` or `make deps-go-security-update`; run `make audit` and the full relevant gates |

## Scoped instructions and skills

- Scoped rules: [backend/AGENTS.md](../backend/AGENTS.md),
  [frontend/AGENTS.md](../frontend/AGENTS.md),
  [deployment/AGENTS.md](../deployment/AGENTS.md).
- Repository skills: `.agents/skills/` — API contract, backend change, frontend
  change, and deployment change. Skills load their full instructions only when
  their activation description matches the task.

## docs/agent-config checker

`tools/quality/docs-agent-config/check-docs-agent-config.sh` runs inside
`make lint-docs` and enforces, deterministically:

- Every internal Markdown link in a tracked document resolves to an existing
  path. URLs, fragments, and application-root paths are excluded.
- Backtick repository-path references in agent-config files (this guide, the
  `AGENTS.md` files, and the skills) resolve to real files or directories.
- Skills under `.agents/skills/` carry the required frontmatter and sections.
- Every canonical document under `docs/` is reachable from `docs/index.md`.
- Runbooks under `docs/runbooks/` declare `status: archival` or
  `status: primary`; archival runbooks are never linked from agent instructions.

Run its regression suite with `make test-docs-agent-config`.

## Architecture checker

`tools/quality/archcheck` (Go, stdlib only) enforces the durable backend
architecture rules; run it with `make archcheck` and exercise it with
`make test-archcheck-regression`:

- **No production package-level mutable application dependencies**
  (`mutable-globals`): `var _` assertions, `embed.FS` values, `errors.New` /
  `fmt.Errorf` sentinels, basic literals, and byte-string literals are exempt by
  construction; everything else must be listed with an ownership justification
  in `tools/quality/archcheck/mutable-globals.allowlist`.
- **No SQL in HTTP handlers** (`sql-in-handlers`): `backend/handlers/` must not
  import `database/sql` or `pgx`, and must not contain SQL command string
  literals.
- **No direct environment reads outside configuration** (`env-read`): only
  `backend/internal/config/` may call `os.Getenv`, `os.LookupEnv`, or
  `syscall.Getenv`.

The checker deliberately does not duplicate the OpenAPI drift gate
(`make openapi-check`) or the docs/agent-config checker (`make lint-docs`);
`make preflight` and `make quality` run all three together. Its regression suite
(the clean real repository plus dirty fixtures) lives in
`tools/quality/archcheck/main_test.go`.
