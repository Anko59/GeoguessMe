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

| Service                         | cap_drop | cap_add                                     | no-new-privileges | pids_limit | read_only                                           | user          | network       |
| ------------------------------- | -------- | ------------------------------------------- | ----------------- | ---------- | --------------------------------------------------- | ------------- | ------------- |
| migration                       | ALL      | (none)                                      | yes               | 64         | yes (+tmpfs /tmp)                                   | 65532:65532   | app           |
| backend                         | ALL      | (none)                                      | yes               | 256        | yes (+tmpfs /tmp)                                   | 65532:65532   | app, frontend |
| web (Caddy)                     | ALL      | NET_BIND_SERVICE                            | yes               | 128        | yes (+/data, /config)                               | 1000:1000     | frontend      |
| db (local) / postgres (hosted)  | ALL      | CHOWN, DAC_OVERRIDE, FOWNER, SETGID, SETUID | yes               | 256        | yes (+tmpfs /tmp, /var/run/postgresql; data volume) | image-managed | app           |
| minio (local SeaweedFS fixture) | ALL      | (none)                                      | yes               | 128        | no (data volume; config is mounted read-only)       | 1000:1000     | app           |
| smtp (local)                    | ALL      | (none)                                      | yes               | 128        | no (+tmpfs /tmp; in-memory)                         | image-managed | app           |

Rationale notes:

- `cap_drop: ["ALL"]` removes the Docker default capability set from every
  service. The only `cap_add` entries are the minimum each image verifiably
  needs: Caddy (UID 1000) binds the privileged `:80` port and therefore needs
  `NET_BIND_SERVICE`; PostgreSQL's official entrypoint chowns and re-stats the
  data directory and drops to the `postgres` user, so it keeps the ownership and
  drop-privilege set (`CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `SETGID`, `SETUID`). No
  other service binds a privileged port or elevates privileges.
- `security_opt: ["no-new-privileges:true"]` blocks setuid/gainful exec in the
  container (the backend's media-processing self-trampoline re-executes the same
  binary as the same user, so it is unaffected).
- `pids_limit` bounds fork-bombs and runaway worker processes per service.
- `read_only: true` with explicit `tmpfs` for scratch paths; writable state
  lives in named volumes (`database`, `geoguessme_prod_*`) and tmpfs. The
  local/disposable `minio` service now runs official, digest-pinned and
  signature-verified SeaweedFS 4.48 as UID/GID 1000, with writable fixture data
  and a read-only credential mount. `minio` and `local-minio` remain
  compatibility names, not MinIO software. Hosted R2 is unchanged; preserve the
  legacy MinIO volume and use the separately authorized
  [local S3 fixture migration](s3-fixture-migration.md), never an implicit
  import.
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

Two complementary checks verify the deployed host matches the revision:

1. `make deployment-hash-check ENVIRONMENT=dev|production` runs the host-side
   hash check over the Cloudflare Access SSH path (requires
   `TUNNEL_SERVICE_TOKEN_ID`, `TUNNEL_SERVICE_TOKEN_SECRET`,
   `DEPLOY_SSH_PRIVATE_KEY`, `DEPLOY_SSH_KNOWN_HOSTS`). The operator route is
   available through `make ops-ssh HOST=dev|production`; it retrieves the
   Cloudflare API token from Secret Service, creates a scoped temporary token,
   and waits for Access policy propagation. The check compares the installed
   root-owned `/opt/geoguessme/bin` scripts, `/opt/geoguessme/config` compose
   files, and GeoGuessMe systemd units against the root-owned
   `/opt/geoguessme/config/runtime-hashes` manifest and exits non-zero on any
   mismatch. It also reports the selected environment's current application
   revision and the root-owned `/opt/geoguessme/config/runtime-revision` for
   context. The `verify` verb is accepted by `forced-command.sh`; provisioning
   the updated forced-command, verification script, and hash manifest onto the
   host is a live step (see below).
2. The existing `geoguessme-health@dev.timer` and
   `geoguessme-health@production.timer` run the same root-owned verifier locally
   every 15 minutes through `health-check.sh`. A mismatch fails the systemd
   oneshot and invokes the existing operator alert unit. This needs no remote
   credential and preserves the F-05 Access and GitHub-environment boundaries.

## Staging a deploy-protocol change

Both deployment jobs in `.github/workflows/deploy.yml` and
`.github/workflows/release.yml` now begin with a fail-closed check of the GitHub
**repository variable** `HOSTED_DEPENDENCY_PROTOCOL_READY`. Leave it unset or
false until the operator has installed the reviewed root runtime, checked the
canonical source hashes and runtime revision, updated host Cloudflared, and
verified both SSH contexts as described below. Set it to the literal `true` only
after those live steps. A dotenv setting or Make variable cannot satisfy this
Actions guard; the flag itself proves neither review nor green CI.

The exact SSH command order, including the `deploy` verb, is:

```text
dev (7 fields):        deploy BACKEND WEB SOPS POSTGRES RESTIC REVISION
production (8 fields): deploy BACKEND WEB KEYCLOAK SOPS POSTGRES RESTIC REVISION
```

The root-owned `common.sh`, `deploy.sh`, and `forced-command.sh` must agree on
these forms before CI sends them. Every image is an immutable digest reference;
SOPS, PostgreSQL and Restic use the matching `dev-REVISION` or
`release-REVISION` alias. The installed deploy script verifies SOPS's trusted
workflow/revision signature before its pull and before decrypting secrets. SOPS
must therefore be anonymously pullable. PostgreSQL and Restic signatures are
verified before their pulls; active refs enter release metadata, and rollback
retains the prior application and independently deployed identity database refs.

Legacy dev four/five-field and production five/six-field forms and bootstrap
pins remain only for staged host compatibility. They do not prove adoption of
the new signed dependencies. Do not activate the new forms against an old root
bundle or remove compatibility before both environments and backups are
verified. A current Git source revision that differs from the installed
`runtime-revision` requires an explicit operator cutover, not just an app
deploy.

The seven content-keyed dependencies are Caddy runtime, Cloudflared, Keycloak,
PostgreSQL, Restic, SOPS and socket-proxy. Reuse requires original signed build
provenance; revision adoption and production promotion keep the same digest. The
scan-only audit covers all 17 required images (eight pinned runtime entries,
seven dependencies and two application images), blocks unexcepted fixed
High/Critical findings, and fails closed on incomplete coverage. This change
does not weaken thresholds or add exceptions. See the
[security scanning guide](../security-scanning.md) for preparation, signatures
and retained scan evidence.

The watch image retains its separate forced `watch SOCKET_PROXY_IMAGE REVISION`
command; never add it as an app-deploy argument. It verifies the
environment-specific GitHub Actions signature before pulling, atomically records
`/var/lib/geoguessme/watch/current.env`, reconciles only `socket-proxy`, checks
full watch health, and rolls back only that proxy. The monitoring project is
shared, so **dev CI must not call `watch`**. Use the existing production
operator route only after release promotion of the exact signed proxy digest
from the complete dev gate. If monitoring is inactive, `watch` stages but does
not start it. Observe the running digest and health before retiring the
temporary upstream fallback; no such live cutover is established by repository
tests.

## Applying monitored host definitions

Terraform deliberately ignores `user_data` changes on the existing stateful
host, so merging this source does not update `/opt/geoguessme/bin` or
`/opt/geoguessme/config`. Use `make credentials-preflight` followed by the
Access-protected `make ops-ssh HOST=dev` route during a planned maintenance
window. Choose one exact reviewed source revision and stage its matching files
through the existing approved operator procedure. Normally its release directory
is already present after dev deployment; the initial protocol cutover must
precede the first new-form deploy, not depend on that deploy installing root
files. This runtime revision is deliberately independent of application
revisions: dev and production may run different application commits while
sharing one host configuration. Compare staged bytes with the canonical source
hashes for the chosen revision before privileged installation; deploy-writable
release files alone are not a trusted baseline.

Install the complete **33-member** monitored set—not only recent changes:

- members 1–12, scripts (root:root, mode 0755): `common.sh`, `deploy.sh`,
  `forced-command.sh`, `watch-deploy.sh`, `verify-deployment-hashes.sh`,
  `backup.sh`, `restore-rehearsal.sh`, `health-check.sh`, `alert.sh`,
  `watch-health.sh`, `watch-refresh-metrics-token.sh`, and `watch-capacity.sh`;
- members 13–18, configuration (root:root, mode 0444):
  `compose.production.yaml`, `compose.hosted.yaml`, `compose.watch.yaml`,
  `watch/Caddyfile`, `watch/vector.yaml`, and `watch/victoria-metrics.yaml`;
- members 19–32, systemd units (root:root, mode 0644): all 14 `geoguessme-*`
  service and timer files in `infra/cloud-init/units/`;
- member 33, `config/s3-fixture/credentials.json` (root:root, mode 0644),
  installed from `deployment/s3-fixture/credentials.json`, with its root-owned
  directory mode 0755. Its `minioadmin` access/secret keys are nonsecret,
  local-only fixture credentials, not R2 credentials.

The first 32 positions are unchanged; the JSON is appended last. The canonical
[installer](../../infra/cloud-init/install-runtime-bundle.sh) consumes the fixed
33 lengths, stages every member, and rejects a truncated or trailing stream
before **any destination member is installed**. Only then does it install
root-owned files with per-file same-filesystem renames and replace the manifest.
This is complete stream validation, not a full transactional rollback guarantee
for I/O failures during installation. For the existing host, keep using the
approved manual root cutover below rather than adding a new remote script or
piping downloaded code into a privileged shell.

`/var/lib/geoguessme/watch/current.env` is mutable deployment state, not part of
the root-owned bundle or hash manifest. The forced `watch` command creates it
atomically as `deploy:deploy` mode `0600`; the watch systemd units load that
single image reference. Do not hand-edit it during a deployment or mix it with
the root-owned runtime revision.

Stop both `geoguessme-health@*.timer` units for the short copy window. Before
installing any member, stage and validate the complete set against the chosen
revision's canonical source hashes. Use
`install --owner=root --group=root --mode=...` for each file, including the
fixture JSON and its directory. Compare **all 33 installed hashes** with that
source baseline, then create a temporary root-owned mode-0444 manifest under its
`bin/...`, `config/...`, or `units/...` paths and atomically rename it to
`/opt/geoguessme/config/runtime-hashes`. Write the chosen 40-character commit to
a temporary root-owned mode-0444 file and atomically rename it to
`/opt/geoguessme/config/runtime-revision` **last**, only after complete hash
agreement. An installed-files-only manifest does not prove source agreement. The
manifest must remain root:root, outside the deploy-writable release archive.

Install host Cloudflared from the same reviewed
[host-tool inventory](../../deployment/images/host-tools.json): Linux AMD64
version **2026.9.3**, Debian package SHA-256
`bc073ef293d504cf5ac533bd0aa1c824ef6b4f358765ccaa6628a8a95cacb4b7`. Through the
approved operator route, follow the checksum-before-`dpkg -i` steps already in
the [cloud-init template](../../infra/cloud-init/cloud-config.yaml.tftpl),
confirm the installed version, then verify the tunnel service and both Access
routes. A container scan or CI's checksum-verified client does not update or
attest to the host binary; retain separate package/version evidence.

Run `systemctl daemon-reload`, then restart the timers. Do not mix revisions.
Run `verify dev` and `verify production` over their respective Access SSH
applications using `make deployment-hash-check ENVIRONMENT=dev` and
`make deployment-hash-check ENVIRONMENT=production` with the matching scoped
credentials. The operator route remains `make ops-ssh HOST=dev|production`;
never broaden CI's restricted keys for installation. Both checks must pass, show
the chosen `runtime-revision`, and cover all 33 entries, including the JSON even
when `local-minio` or monitoring is inactive. Retain canonical source hashes,
installed hashes, revision and SSH results with the maintenance record before
setting the repository readiness variable. Rehearse alerts with a controlled
mismatch only on a disposable host, never by tampering with production.

Repeat this all-files cutover whenever a later deployed revision changes a
monitored definition. The root-owned compose files are the definitions the live
`compose` helper actually uses, so a source-only compose change is not active
until this operator step is completed. For a new host, set Terraform's required
`runtime_revision` to the exact commit checked out while rendering the plan;
cloud-init then writes the initial root-owned marker alongside those same files.
The rendered transport uses standard base64 MIME `application/gzip` around a
`#cloud-config-archive` whose cloud-config content is compact JSON (a YAML
subset), preserving native UTF-8 handling. Before provisioning, the exact
rendered headers, wrapping and payload must pass the strict Hetzner 32 KiB
user-data limit and native parser tests. Transport design alone is not evidence
that the current rendering passes; no existing host user-data transition is
performed by changing this source.

## Operator closure checklist (live steps)

Source changes and focused tests do not establish CI publication, registry
signatures, a dev runtime cutover or a host user-data transition. Complete local
`make verify` evidence is still required; it is not supplied by this runbook. No
user-data migration or live operator action is authorized merely by these
documentation changes. The following remain operator closure steps:

- [ ] Keep `HOSTED_DEPENDENCY_PROTOCOL_READY` unset/false until a separately
      authorized operator installs all 33 single-revision members, retains
      canonical source/installed hash and revision proof, installs and checks
      host Cloudflared 2026.9.3, and verifies both Access SSH contexts. Only
      then set the GitHub repository variable to literal `true`.
- [ ] After a release promotes the signed proxy digest, use the separate
      production `watch IMAGE REVISION` command, verify the running
      `socket-proxy` digest and full watch health, and confirm no other service
      was recreated. Do not perform this cutover from the dev workflow because
      the watch project is shared with production.
- [ ] Confirm both `geoguessme-health@*.timer` units run the integrity check and
      that an intentional rehearsal mismatch triggers the existing alert path.
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
