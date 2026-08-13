# Black-box QA agent

The repository includes a source-blind exploratory QA agent for deployed
development revisions. It interacts only through the browser, using visible
roles, labels, URLs, response status, console output, and page state. It does
not import application source or implementation-side E2E helpers.

## Scope

The release-level journeys cover:

- authentication, logout, refresh, pending recovery-email dispatch state, and
  security headers;
- group creation, invite joining, unauthorized access, and invalid invites;
- browser WebSocket connection, live chat delivery, reload catch-up, and
  client-IP rate limiting;
- group media upload/read through the hosted storage path; and
- mobile layout without horizontal overflow.

Every journey records a screenshot and browser diagnostics. A failed journey is
a reproducible finding with its project, build revision, observed error, and
artifact paths. The agent never changes application code; fixes remain a
separate implementation task.

## Running it

The Dockerized target requires a deployed URL. Cloudflare Access headers are
passed only through the process environment and are never written to reports:

```text
export QA_BASE_URL=https://dev.geoguessme.com
export QA_ACCESS_CLIENT_ID=...
export QA_ACCESS_CLIENT_SECRET=...
export QA_BUILD_SHA=$(git rev-parse origin/dev)
make qa-agent QA_REPORT_DIR=qa-artifacts
```

The `Black-box QA` workflow runs automatically after a successful development
deployment and can also be dispatched for an explicit revision. It uploads the
JSON/Markdown report, screenshots, traces, and diagnostics for seven days. Its
production job invokes the authenticated, fixed-form `restore-rehearsal`
command, which restores the latest encrypted production backup into a disposable
database container and removes that container on exit.

Email delivery is intentionally reported as a scope limitation unless a mailbox
integration is configured: the browser verifies that the application accepted
the recovery address and displays the pending-verification state, but it must
not claim delivery merely from an HTTP 2xx response.

## Release policy

The fixed 24-hour quarantine/soak delay is not required. Production promotion
does require a successful Black-box QA workflow for the exact deployed `dev`
revision, in addition to the complete development gate, signed image scans, live
health, and the existing backup/rollback controls. Reproducible findings block
promotion until a separate fix and rerun produce a clean report.
