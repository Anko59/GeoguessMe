# LLM-driven exploratory QA agent

GeoGuessMe has a runtime-agnostic, source-blind exploratory QA agent for a
deployed environment. The agent is genuinely LLM-driven: it chooses the next
journey, action, odd sequence, and reproduction attempt from observations
returned by a browser tool contract. It is not a deterministic Playwright
scenario suite and it never changes application code.

## Architecture

The canonical contract lives in `.agents/qa/`:

- `AGENT.md` defines the investigator role, source-blind restrictions, journey
  priorities, evidence rules, and finish protocol.
- `policy.yaml` defines budgets, journey areas, finding categories, and secret
  handling.
- `tools.yaml` defines the provider-neutral browser and evidence operations.
- `mcp.json` is the thin local MCP wiring for the browser provider.

The browser adapter is `tools/qa/browser-mcp.mjs`. It runs in the pinned
Playwright Docker image, keeps isolated browser sessions and tabs, exposes
accessibility/text/state/diagnostic observations first, and writes targeted
screenshots only when the model requests them. It does not expose arbitrary
JavaScript, HTTP, source, shell, or filesystem tools. Chromium supplies a fake
camera device and each context receives a fixed geolocation (Paris by default;
override with `QA_FAKE_LATITUDE`, `QA_FAKE_LONGITUDE`, and
`QA_FAKE_LOCATION_ACCURACY`). `browser_capabilities` performs a controlled
camera/location probe so the report can distinguish an unexercised journey from
a missing test capability.

The same MCP server exposes a high-level disposable mailbox contract:
`mailbox_create`, `mailbox_search`, `mailbox_read`, and `mailbox_open_link`. The
default provider is Mail.tm, which requires no operator mailbox credential; the
gateway keeps provider tokens and mailbox passwords out of the model output, and
tokenized product links are opened server-side without being returned or
recorded. Mailbox accounts are deleted when the MCP process exits. This is
intentionally limited to throwaway QA mailboxes; it must not be used for real
user mail. The provider's public API is documented at
[Mail.tm API documentation](https://docs.mail.tm/).

Visible one-time group invite links have a separate safe handoff contract:
`browser_transfer_link` captures a labeled invite control in the owner session
and returns only an opaque single-use transfer ID;
`browser_open_transferred_link` consumes that ID in the member session. The raw
link never enters model output, diagnostics, screenshots, or the report. A
failed transfer is a QA harness failure and blocks the multi-user journey; it
must not be recorded as an acceptable coverage limitation.

The runtime adapters are deliberately thin:

- `tools/qa/codex-adapter.sh` uses the local authenticated Codex CLI and passes
  the browser MCP server through an ephemeral read-only run.
- `tools/qa/pi-adapter.sh` uses Pi with built-in tools disabled and the same MCP
  configuration. It is useful on machines with a configured Pi provider.

The browser remains Dockerized; an LLM runtime is not installed by the
repository and credentials are never committed or printed. At least one local
runtime must already be authenticated before running the agent.

## Running against deployed dev

“Local” means the LLM process and report collection run on the operator machine.
The browser target is the deployed dev environment:

```text
export QA_BASE_URL=https://dev.geoguessme.com
make qa-agent-full QA_REPORT_DIR=qa-artifacts
```

The local runner uses the existing `CLOUDFLARE_API_TOKEN` and
`CLOUDFLARE_ACCOUNT_ID` to create a one-hour service token and a policy scoped
only to the matching Access application, then removes both on exit. Existing
`QA_ACCESS_CLIENT_ID` and `QA_ACCESS_CLIENT_SECRET` values are also accepted
when an operator already has them. Access values are passed to the browser
context only; they are not put in the prompt or report. The default runtime is
`codex`; select Pi explicitly with `QA_RUNTIME=pi`. Use `make qa-agent-fast` for
a short investigation or `make qa-agent-nightly` for the extended budget.

The default runner rejects HTTP and localhost targets so a release report cannot
accidentally describe a local stack. `QA_ALLOW_LOCAL=1` is reserved for
developing the browser adapter itself and is not release evidence.

Full and nightly runs use the dedicated account pool for an owner, member, and
outsider, then use the opaque invite handoff to exercise invitation, group
conversation, and authorization boundaries in separate browser sessions. The
local operator supplies the pool password through `QA_ACCOUNT_PASSWORD`; the
role usernames default to `qa_release_owner`, `qa_release_member`, and
`qa_release_outsider` and may be overridden with the corresponding
`QA_ACCOUNT_*_USERNAME` variables. The password is passed only to the local
browser provider and is never placed in the prompt, report, or CI. If the pool
or Mail.tm is unavailable, the affected journeys are reported as blocked rather
than silently treated as passed.

The report is written to `QA_REPORT_DIR/qa-report.json` with mode `0600` and
contains the target origin/path, the supplied deployed revision, exercised
journeys, limitations, findings, and artifact paths. `BUG` is the only blocking
finding category; `UX_DEBT`, `VISUAL`, and `PERFORMANCE` remain explicitly
non-blocking. A clean report is not evidence that omitted journeys passed: the
report lists both exercised and unexercised areas.

## Release evidence

Run the agent after the development deployment is healthy, with `QA_BUILD_SHA`
set to the exact deployed `dev` revision (the Make target defaults it to
`origin/dev`). Review the generated report and retain it with the release
record. Reproducible `BUG` findings block promotion until the application is
fixed and the agent is rerun against the new deployed revision.

This is a local acceptance step, not a GitHub Actions workflow. CI does not
receive LLM credentials and the release workflow does not pretend that a missing
hosted workflow run is local exploratory evidence.
