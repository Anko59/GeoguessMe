# Deployment guide

Player blocking requires the forward-only `038_user_blocks` migration and the
enforcing backend before the updated frontend. Retain block data on rollback;
see [blocking rollout and rollback](user-blocking.md#api-and-rollout).

The supported deployment workflow is documented in
[deployment/README.md](../deployment/README.md). It covers first deploy,
migrations, immutable image upgrades, rollback, backup/restore, restart
behavior, health checks, secrets, outage response, and rehearsal evidence.
Nightly `make verify` builds local application and Keycloak images with Buildx
`--load` before `make audit-images` scans them, so the image gate inspects the
artifacts produced by that verification run. The Keycloak base-image pin and its
narrowly scoped audit exceptions are maintained in the
[security scanning guide](security-scanning.md). Hosted SOPS and the monitoring
socket proxy use project-owned, digest-pinned security derivatives; CI scans
their exact published digests before signing and production promotion never
rebuilds them. The proxy fixes Alpine PCRE2 without a CVE exception and is
updated through a separate `watch` command, not the app deploy. Host runtime
changes follow the staged procedure in the
[runtime hardening runbook](runbooks/runtime-hardening.md#staging-a-deploy-protocol-change).

Both frontend Dockerfiles include the reviewed local braces security backport
before npm installs dependencies. Keep the vendor source in the build context; a
manifest/lockfile-only copy is insufficient for this file dependency.
`make audit` verifies upstream integrity, reconstructed source, and depth-limit
regressions in addition to normal dependency scanning. See the
[backport compatibility ledger](agent-engineering.md#braces-security-backport)
for provenance and the upstream replacement/removal condition. Production still
promotes the exact verified image digest without rebuilding.

The gateway image explicitly installs its public Caddy configuration as
root-owned mode `0644`, so the non-root runtime can read it even when the source
checkout uses a restrictive umask. This does not change permissions on host
credentials or encrypted deployment configuration.

OAuth2 Proxy retains its pinned upstream image and read-only UID `65532`
configuration mounts. Hosted deployment, `make prod-up`, and the production
rehearsal run the
[public config preparer](../deployment/oauth2-proxy/prepare-public-configs.sh)
to set only its two tracked public templates to mode `0644`, independent of the
checkout/extraction umask. The preparer rejects missing files and symlinks;
private environment files and runtime credential values are never changed.

The concrete hosted implementation and launch checklist is in the
[hosted deployment runbook](runbooks/hosted-deployment.md). It covers the
Hetzner CX23, Cloudflare Tunnel/Access/R2, SOPS age keys, GitHub environments,
signed digest deployments, Brevo, monitoring, and recovery.

Terraform losslessly compresses the full host bootstrap, runtime installer, and
32-member runtime bundle with `gzip+base64`; the rendered cloud-init still must
fit Hetzner's unchanged 32 KiB limit. Cloud-init writes the bootstrap as a
root-owned `0700` executable after installing the required packages. The
[bootstrap script](../infra/cloud-init/bootstrap-host.sh) checks its tools
before configuring the host, retains the ordered SSH/firewall/backup setup, and
leaves monitoring disabled pending operator setup. Runtime extraction consumes
one shared file descriptor and hashes the exact installed bytes with fixed
ownership and permissions.

Android distribution is part of the production release boundary but remains a
separate artifact from the hosted services. The production workflow builds and
verifies the signed Android App Bundle before image promotion, retains its
provenance manifest, and publishes that exact artifact to the configured Play
track only after the production deployment succeeds. The Play publication job
uses GitHub OIDC and the separate `play-publishing` environment; only the host
deployment uses GitHub's `production` environment. It fails closed on access,
digest, version, and edit-validation checks. See the
[mobile release guide](mobile.md) and
[Google Play account runbook](runbooks/google-play-console.md) for the
configuration and recovery procedure.

The hosted system has three Compose projects: isolated dev and production game
stacks on loopback ports `8082` and `8081`, plus shared Keycloak and its own
PostgreSQL database on `8083` for `auth.geoguessme.com`. Each game stack keeps
its own OAuth2 Proxy beside Caddy. Cloudflare Tunnel is the only ingress and
Caddy is the same-origin application gateway, so Traefik is not part of this
topology. Keycloak owns ordinary email/password signup/login and brokers Google.
Apple and GitHub remain disabled for this rollout.

Generate `deployment/secrets/identity.env.enc` for both host age recipients with
`make identity-secrets-generate`, then follow the
[social-auth rollout runbook](runbooks/social-auth-rollout.md) for Keycloak
provisioning, existing-account continuity, optional linking, and validation.

All operational actions use Dockerized Make targets:

```text
make compose-validate
make prod-config
make prod-migrate
make prod-up
make smoke BASE_URL=https://your-domain.example
make prod-logs
make prod-container-verify
```

`make prod-container-verify` builds the pinned production images, validates
non-root users, image healthchecks, read-only filesystems, and Compose
configuration, then starts a disposable production-like local stack with
test-only credentials, polls health and readiness, verifies representative HTTP
behavior (liveness, readiness, auth enforcement, WebSocket auth), and tears down
all resources. It is safe for local/CI use because it uses the `local-db`,
`local-minio`, and `local-smtp` Compose profiles and never touches production
infrastructure. The gateway binds only `127.0.0.1:18083` and the disposable
Mailpit UI uses `18085` by default; set `GEOGUESSME_PROD_VERIFY_WEB_PORT` or
`GEOGUESSME_PROD_VERIFY_SMTP_PORT` when those ports are occupied. The rehearsal
requires Docker Compose 2.24.4+ for `!override`: the gateway binding replaces
all inherited production web ports, including `GEOGUESSME_WEB_PORT`, rather than
publishing both the production and rehearsal ports. Teardown errors are visible
and fail an otherwise successful rehearsal; an earlier verification failure
keeps its original exit status.

Development, integration/E2E, and the optional `local-minio` profile pull the
MinIO release from the public `quay.io/thanos/minio` mirror. All three pin the
same release and immutable manifest digest previously used for
`quay.io/minio/minio`; the mirror serves that exact manifest. Verify the digest
before changing registries. Registry access failures do not require a data
migration or volume reset.

Compose restart is not zero-downtime rolling deployment. Do not describe this
topology as rolling without adding an orchestrator and its corresponding failure
and rollback evidence.

## Live acceptance

The group globe uses the standard application rollout and migration job.
Migration 026 adds its history pagination index; allow for index creation time
on large photo tables. It is compatible with previous application binaries. See
[migration 026](database-migrations.md#migration-026-group-challenge-globe-index).
The Earth texture is bundled with the frontend; no imagery API key or new
environment variable is required. The production Caddy Content Security Policy
allows the exact OSM host in `img-src`, and exact OSM plus NASA GIBS in
`connect-src` for globe detail requests. If this policy change is rolled back,
the bundled Earth and group history remain available but close detail tiles do
not load; restore those origins in the Caddy policy when re-enabling globe
detail imagery.

Repository rehearsals remain disposable. Live R2, Access, Tunnel, and Brevo must
be validated on dev, and an isolated production backup restore must be
completed, before the first production promotion to `main`. There is no fixed
24-hour soak or quarantine delay; promotion may proceed once this live evidence
and every automated release gate pass for the exact deployed revision.

Hosted application deployments wait up to five minutes for an in-progress hourly
backup to release the per-environment backup lock. This avoids rejecting a valid
deployment because the timer fired during the CI gate while retaining a bounded
failure for a backup that does not complete.

## Compatibility-removal rollout

The application compatibility PR removes the pre-GA inputs — the messages
`after_id` parameter, the reaction `emoji` request/response alias, and the
singular challenge `group_id` form field — but deliberately leaves the legacy
reaction database column and synchronization objects in place. Hosted deploys
run every pending migration before replacing the backend, so bundling the
cleanup migration with that application revision would remove rollback schema
compatibility while the previous revision is still serving. The 0.3.0 release
must therefore leave migration 014 out of its ordinary migration set; the
cleanup is a later, separately reviewed release.

1. Deploy the new application revision (reads/writes only the new fields: the
   opaque `cursor`/`stable_cursor` contract, `reaction`, and repeated
   `group_ids`). The database stays backward compatible during this window, so
   the previous revision can still serve or roll back.
2. Complete the normal deployment, then confirm the previous revision is no
   longer running and is outside the rollback window.
3. Migration 014, delivered in a separate cleanup PR after the compatible
   application deployment succeeded, drops the legacy reaction column, trigger,
   function, and constraints. Its deployment applies the forward-only migration
   before restarting the already-compatible application revision.

**Rollback:** before step 3, reverting to the previous image is safe because the
schema is still backward compatible. After step 3 the migration is forward-only
by repository rule: do not attempt to re-add the emoji column; restore the
pre-migration backup instead (see `deployment/README.md`). Rehearse both
sequences with `make migration-test`, `make restart-rehearsal`, and
`make backup-rehearsal` before the live rollout.

## See also

- [deployment/README.md](../deployment/README.md) — rehearsal evidence table,
  tool architecture, first-deploy steps
- [configuration.md](configuration.md) — every environment variable, defaults,
  production validation
- [operations.md](operations.md) — health, metrics, backups, secret rotation,
  incident response
- [testing.md](testing.md) — comprehensive gate listing with expected results
