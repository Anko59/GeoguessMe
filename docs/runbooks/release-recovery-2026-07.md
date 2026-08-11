---
status: archival
---

# July 2026 release recovery

## Purpose and scope

This record describes the failed attempt to deliver the PWA, Web Push, and
camera-filter work during the 0.2.3 cycle, the safeguards restored during
recovery, and the release sequence that prevents an untested or rebuilt
production artifact. It is an operational record, not a substitute for the
[hosted deployment runbook](hosted-deployment.md).

## What failed

The changes were split across several incomplete pull requests and completed by
different agents. That produced three separate failures:

1. Production secrets from before the Push feature did not contain VAPID keys,
   while an early implementation made every production backend require them.
   Development deployment therefore reached the host but could not start the
   backend.
2. An attempted SSH-deploy recovery changed the forced-command protocol from its
   required four fields. The host authorizes exactly
   `deploy BACKEND WEB REVISION`; repository edits cannot repair the root-owned
   deployed command on an existing host.
3. Test coverage was made to pass by skipping two E2E checks, excluding the Push
   implementation from frontend coverage, and retrying a migration test after
   pruning Docker cache. Those changes concealed regressions instead of fixing
   them.
4. The development host later rebooted. Its deployment and backup lock directory
   lived under `/run`, but cloud-init created it only on first boot. The signed
   development workflow therefore reached the host and failed before it could
   pull images or run migrations with
   `cannot create /run/lock/geoguessme/geoguessme-deploy.lock`.

## Corrected state

The recovery merged the four-field deploy protocol and its contract tests, then
made Push configuration explicit. A production environment with all VAPID
variables absent starts with Push disabled: no Push worker runs and the public
key endpoint returns `503`. A configured environment must provide a valid
`VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, and `VAPID_SUBJECT` together. This
avoids generating an ephemeral production keypair, which would invalidate every
browser subscription after a restart.

The two E2E checks now run again, the migration test has no retry or cache-prune
fallback, and Push/service-worker/bootstrap code is included in coverage with
deterministic browser-API tests. A failure in any of those checks is a release
blocker.

The lock incident was repaired through the existing Cloudflare Access operator
route using a short-lived service-token policy and the existing operator key.
The repair created `/etc/tmpfiles.d/geoguessme.conf`, which recreates
`/run/lock/geoguessme` as `deploy:deploy` with mode `0750` at every boot. The
bootstrap template and Terraform test now enforce the same rule for new hosts.
The CI deploy key remains restricted to the four-field `deploy` command; it is
not broadened into a general-purpose server shell merely to repair host state.

## Access credential recovery

Cloudflare Access service-token client secrets are intentionally shown only at
creation or rotation. Treat a missing secret as unrecoverable: rotate the
Terraform-managed service token, update the `CF_ACCESS_CLIENT_ID` and
`CF_ACCESS_CLIENT_SECRET` GitHub environment secrets, and verify a development
deployment. Never put the secret in Git, a command line, a ticket, or this
document.

For this incident, the GitHub environment secrets were proven valid because the
development workflow authenticated through Cloudflare Access, verified image
signatures, created its backup, and reached the backend startup step. No token
rotation was needed. The Cloudflare API token is sufficient to administer Access
resources, but it cannot recover an already-issued service-token secret.

## Required release sequence

1. Merge each correction into `dev` and wait for its exact `dev` workflow to
   complete the full verification gate and development deployment successfully.
2. Confirm the deployed revision, health checks, backup result, and immutable
   signed backend and web image digests from that workflow.
3. Create a short-lived `release/*` branch from `main`, materialize the exact
   current `dev` tree on it, and open a release PR to `main`. Do not rebuild or
   hand-copy changes.
4. Merge only after the release PR confirms tree equality and required checks.
   The `main` workflow promotes the exact development image digests, signs them
   for production, creates the version declared in `.release-version`, and
   deploys production through Cloudflare Access.
5. Confirm production health, active-release metadata, backup freshness,
   WebSocket connectivity, and the intended Push state. For 0.2.3, Push remains
   intentionally disabled until stable VAPID values are added to the encrypted
   production environment.

## Prevention checklist

- Keep host authorization grammar, cloud-init templates, and deploy scripts in
  one contract test; test an existing-host upgrade path separately from a new
  host template.
- Treat `/run` as volatile: provision lock and socket directories through a
  systemd tmpfiles rule, not only first-boot commands. Test the rule and apply
  bootstrap changes explicitly to existing hosts because Hetzner cannot replay
  cloud-init in place.
- Treat skipped tests, coverage exclusions, broad retries, and destructive cache
  cleanup as defects requiring an explicit root-cause fix and a regression test.
- Preserve VAPID values in the operator secret store before enabling Push;
  configuration changes must state whether missing values disable a feature or
  stop startup.
- Record where nonrecoverable credentials are stored when they are created, and
  test the documented rotation procedure before it is needed in an incident.
- Keep release branches tree-identical to the successfully deployed `dev`
  revision so production promotes signed artifacts rather than rebuilding
  source.
