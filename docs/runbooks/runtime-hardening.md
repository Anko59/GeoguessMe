---
status: primary
---

# Runtime hardening runbook

This runbook describes the runtime hardening applied to the hosted services, the
mandatory compatibility rehearsal before any production apply, how to verify the
hardening end to end, and the operator closure checklist that remains after the
configuration lands.

## Applied hardening

The production Compose stack (`deployment/compose.production.yaml` +
`deployment/compose.hosted.yaml`) now applies defense-in-depth to every service:

| Service                        | cap_drop | cap_add                                     | no-new-privileges | pids_limit | read_only                                           | user          | network       |
| ------------------------------ | -------- | ------------------------------------------- | ----------------- | ---------- | --------------------------------------------------- | ------------- | ------------- |
| migration                      | ALL      | (none)                                      | yes               | 64         | yes (+tmpfs /tmp)                                   | 65532:65532   | app           |
| backend                        | ALL      | (none)                                      | yes               | 256        | yes (+tmpfs /tmp)                                   | 65532:65532   | app, frontend |
| web (Caddy)                    | ALL      | NET_BIND_SERVICE                            | yes               | 128        | yes (+/data, /config)                               | 1000:1000     | frontend      |
| db (local) / postgres (hosted) | ALL      | CHOWN, DAC_OVERRIDE, FOWNER, SETGID, SETUID | yes               | 256        | yes (+tmpfs /tmp, /var/run/postgresql; data volume) | image-managed | app           |
| minio (local)                  | ALL      | (none)                                      | yes               | 128        | no (data + config home; comment documents why)      | image-managed | app           |
| smtp (local)                   | ALL      | (none)                                      | yes               | 128        | no (+tmpfs /tmp; in-memory)                         | image-managed | app           |

Rationale notes:

- `cap_drop: ["ALL"]` removes the Docker default capability set from every
  service. The only `cap_add` entries are the minimum each image verifiably
  needs: Caddy (UID 1000) binds the unprivileged `:80` port and therefore needs
  `NET_BIND_SERVICE`; PostgreSQL's official entrypoint chowns and re-stats the
  data directory and drops to the `postgres` user, so it keeps the ownership and
  drop-privilege set (`CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `SETGID`, `SETUID`). No
  other service binds a privileged port or elevates privileges.
- `security_opt: ["no-new-privileges:true"]` blocks setuid/gainful exec in the
  container (the backend's media-processing self-trampoline re-executes the same
  binary as the same user, so it is unaffected).
- `pids_limit` bounds fork-bombs and runaway worker processes per service.
- `read_only: true` with explicit `tmpfs` for scratch paths; writable state
  lives only in named volumes (`database`, `geoguessme_prod_*`) and tmpfs. MinIO
  is left writable because it persists configuration under its home directory;
  the comment in the Compose file documents that choice.
- **Segmented networks:** `frontend` (web + backend) and `app` (backend,
  migration, postgres, minio, smtp). The public gateway reaches only the
  backend; the data services are reachable only from the application tier, and
  never from the gateway.

## Mandatory rehearsal before production apply

These directives change container runtime properties. Do **not** apply them to
production from this change alone. Run, in order, against a disposable stack:

```sh
make build-images
make restart-rehearsal   # restart/reconnect compatibility
make prod-container-verify  # non-root + healthcheck + compose + stack + smoke
make smoke-rehearsal     # representative HTTP behavior
```

Only when every rehearsal passes on the exact revision may the hardened Compose
be applied to production, and the prior signed digest must be retained for
rollback. The plan's requirement is explicit: hardening applies "where
rehearsals prove compatibility".

## Verifying the running host

Two authenticated checks verify the deployed host matches the revision:

1. `make deployment-hash-check ENVIRONMENT=dev|production` runs the host-side
   hash check over the Cloudflare Access SSH path (requires
   `TUNNEL_SERVICE_TOKEN_ID`, `TUNNEL_SERVICE_TOKEN_SECRET`,
   `DEPLOY_SSH_PRIVATE_KEY`, `DEPLOY_SSH_KNOWN_HOSTS`). The check compares the
   installed root-owned `/opt/geoguessme/bin` scripts and
   `/opt/geoguessme/config` compose files against the recorded deployed revision
   (`REVISION` in `current.env`) and exits non-zero on any mismatch. The
   `verify` verb is accepted by `forced-command.sh`; provisioning the updated
   forced-command and the verification script onto the host is a live step (see
   below).
2. The `runtime-integrity` job in `.github/workflows/health.yml` runs the same
   check on schedule (and on demand) for production and development. It requires
   the deployment SSH secrets in the `monitoring` environment.

## Operator closure checklist (live steps)

The following remain live operator steps after this configuration merges:

- [ ] Provision the updated `forced-command.sh` and
      `verify-deployment-hashes.sh` onto the host (root-owned
      `/opt/geoguessme/bin`), then verify `verify dev` and `verify production`
      succeed over the Access SSH path.
- [ ] Add `DEPLOY_SSH_PRIVATE_KEY` and `DEPLOY_SSH_KNOWN_HOSTS` to the
      `monitoring` environment for the `runtime-integrity` job.
- [ ] Run the rehearsal sequence above and, once green, apply the hardened
      Compose to production; keep the prior signed digest for rollback.
- [ ] Complete the read-only host inspection (OS update state, SSH/sudo
      configuration, listening sockets/firewall, Docker configuration and
      effective container restrictions, secret file modes, timers, logs, active
      image digests, disk state, backup integrity, compromise indicators). Stop
      and enter incident response if any evidence is suspicious.
- [ ] Reconcile CodeQL alerts against the fixes in this program or document
      narrowly scoped dispositions; do not globally suppress rules.
- [ ] Produce the new dated security assessment with current source revisions,
      exact deployed digests, current scanner databases, live control-plane
      evidence, and status for F-01 through F-12. Preserve the August 2 report
      unchanged as an archival assessment.
