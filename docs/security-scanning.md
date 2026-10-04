# Image security scanning

Every final/runtime image that GeoGuessMe ships is scanned for known
vulnerabilities by [Trivy](https://trivy.dev) through `make audit-images`. The
gate is deterministic: it fails on **fixed** High/Critical findings (a fix is
available in a newer release) and merely reports unfixed High/Critical findings,
which cannot be remediated by bumping a version.

## JavaScript dependency backports

`make audit` retains the high-severity npm audit gate and also runs
`make test-braces-security` against the dependency actually used by micromatch.
For [GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm),
there is no published fixed upstream braces release. The project therefore
maintains a private, scoped **code backport**, not an advisory exception: the
published 3.0.3 implementation plus the reviewed depth, fractional-limit and
parent-cycle guards from
[upstream PR 72](https://github.com/micromatch/braces/pull/72). Unrelated
upstream-master parser changes are deliberately excluded.

The [backport provenance](../frontend/vendor/braces-security/README.md) records
source and patch identities. npm installs a copy through `install-links=true`,
so clean container installs use the same reviewed files rather than broken
transitive symlinks. The security gate verifies installed executable bytes
before loading them, rejects excessive/cyclic nesting, and preserves ordinary
brace/micromatch behavior. Registry version matching cannot verify a downstream
patch; changing a package name alone is not remediation. No audit threshold,
advisory suppression or postinstall rewrite is used. Replace this temporary
backport with a verified upstream release when it passes the same regressions;
[issue 392](https://github.com/Anko59/GeoguessMe/issues/392) owns that
follow-up.

## Immutable artifact lifecycle

The image audit is **scan-only**. `make audit-images` never builds an image or
silently substitutes a missing artifact. Separate explicit preparation targets
own builds:

- `make build-security-tool-images` prepares local dependency images. It reuses
  validated content-keyed images when their reviewed inputs have not changed.
- `make publish-security-images` publishes only missing dependency input keys,
  scans the exact output, and signs it after successful verification.
- `make resolve-security-images` resolves and verifies published digests without
  building. It fails closed on missing, unsigned, or conflicting content.
- `make build-images` builds application artifacts using the prepared Caddy
  runtime; a frontend change does not recompile Caddy.

The [dependency manifest](../deployment/images/dependencies.tsv) records build
contexts and inputs. Identity includes platform, Dockerfile bytes, reviewed
context inputs, and applicable Docker ignore files, **not the application Git
revision**. External bases must have immutable digests. Upstream package
repositories remain external build inputs: provenance and exact output digests
are retained; a digest-pinned base alone is not a claim of bit-reproducibility.
Local state is written to `.local/security-images.env`, containing immutable
Docker image IDs; published selections contain registry digests.

CI verifies the original dependency-input signature with `dependency-build=true`
and BuildKit provenance before reuse. Adoption signatures omit that
build-purpose annotation, so approval cannot impersonate original build
evidence. Development adoption adds a revision approval and alias without
changing the manifest digest or rewriting original build provenance. Production
promotes the identical approved digest and freshly scans it before signing the
release approval. Application signatures remain tied to the protected deployment
workflow; the narrowly scoped dependency identity also trusts the protected
Security workflow for dependency publication.

An existing unsigned or invalidly signed input tag is **not permission to
rebuild or sign registry content**. Investigate interrupted publication first.
Remediate vulnerable inputs to create a new reviewed key; any recovery of an
interrupted clean publication requires explicit review of its exact digest,
provenance and scan. Registry failures never cause automatic replacement.

## Required audit inventory

The [runtime inventory](../deployment/images/runtime.tsv) retains monitoring,
logging and OAuth deployment images and includes optional S3/Mailpit fixtures.
The S3 fixture is the maintained official SeaweedFS 4.48 image, verified against
its exact release signing identity with `make verify-s3-upstream`; Mailpit uses
the compatible official 1.31.4 digest. Retired MinIO is not a runtime fallback:
preserve development data with the explicit
[local migration runbook](runbooks/s3-fixture-migration.md). Hosted R2 is
unchanged. The dependency manifest adds PostgreSQL, Cloudflared, SOPS,
socket-proxy, Keycloak, Restic and the shared Caddy runtime. Backend and web
images are mandatory in normal `make audit-images`; missing local images fail
rather than warning and skipping them.

The standalone Security workflow explicitly uses
`AUDIT_APPLICATION_IMAGES=false`: application digests are scanned at publication
and promotion, avoiding a race with publishing the current revision. It still
scans the complete deployment dependency set on every `dev` push and weekly.
Nightly verification resolves existing dependency artifacts, then builds/tests
application images and scans the complete set.

All application and identity database topologies use the selected
`POSTGRES_IMAGE`, not an unpatched upstream substitute. The shared identity
stack can retain its independently deployed `IDENTITY_POSTGRES_IMAGE` until
production adopts the new digest. Hosted metadata records database and Restic
refs; backups and restore rehearsals select the active environment's artifacts.
The watch gateway continues to reuse the exact production web digest.

The [host-tool pin](../deployment/images/host-tools.json) separately identifies
the checksum-verified Cloudflared Debian package used by CI and provisioning. A
container scan does not attest to an existing host's installed binary or OS
packages. Existing hosts require the operator cutover described below; changing
cloud-init does not update them because Terraform intentionally ignores
`user_data` drift.

## Temporary patches and removal conditions

| Component    | Temporary change                                                                          | Removal condition                                                                         |
| ------------ | ----------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Caddy        | Released 2.11.7 build and OS package refresh in a reusable runtime                        | A compatible official digest passes the same final scan and configuration/rehearsal tests |
| Terraform    | Official 1.16.5 binary with PCRE2 10.49-r0 only; no source rebuild                        | A verified upstream digest includes the package fix                                       |
| Cloudflared  | Official 2026.9.3 binary with Debian OpenSSL deb13u3 payload and genuine package metadata | A verified upstream image carries the fixes                                               |
| SOPS         | Official 3.13.3 binary with pinned libexpat deb12u4 and PCRE2 deb12u2                     | A verified upstream digest carries the package fix                                        |
| Socket-proxy | Pinned upstream entrypoint/binary with PCRE2 10.49-r0                                     | A verified upstream digest passes the same scan and proxy contracts                       |
| Restic       | Asserted 0.19.1 source commit and explicitly upgraded module graph                        | A compatible official artifact passes scans and backup/restore rehearsals                 |
| PostgreSQL   | Compatible 15.x base with OpenSSL/libuuid refresh                                         | A verified compatible official digest plus unchanged entrypoint/data contracts            |
| Keycloak     | Upstream 26.7.5 provenance wrapper, no JAR surgery                                        | No security patch remains; wrapper preserves the existing signed deployment contract      |

Terraform remains an operator tool outside the runtime default inventory;
`make terraform-test` proves functionality, not security. Its small derivative
addresses the separately observed fixed PCRE2 finding rather than adopting the
known-vulnerable raw replacement image.

Scoped npm overrides preserve YAML v4/v5 consumer contracts and CommonJS UUID
compatibility. `make test-npm-security-overrides` checks the installed
consumers; `make audit` includes it and the braces regressions. Go updates use
compatible upstream upgrades rather than replaying historical version requests
that could downgrade the current graph. Review module changes before committing.

## Blocking semantics and operational errors

One audit prepares a single vulnerability/Java database snapshot, freezes image
identity and platform, and scans **every required image before failing**. Raw
High/Critical JSON includes fixed, unfixed and approved findings. Native Trivy
conversion generates SPDX; the blocking pass uses `--ignore-unfixed` and the
native reviewed exception policy. Content and gate results are deduplicated
without allowing one reference's exception to authorize another reference.

- Exit **0**: every required image completed and no unexcepted fixed
  High/Critical finding remains.
- Exit **1**: genuine fixed High/Critical findings.
- Exit **2**: policy failure or incomplete scan. Findings already collected
  remain visible; incomplete coverage is never success.

HTTP 429, selected 5xx and transport failures receive at most four attempts,
exponential backoff and jitter. Logged `Retry-After` is honored within the
bounded budget; longer or malformed requests fail conservatively.
Authentication, missing manifests, invalid signatures/digests and vulnerability
findings are not retried as transient errors. Registry/database outages are
reported separately from vulnerability failures. Cached data is not a substitute
for a failed fresh snapshot update. Concurrent audits of one checkout fail
closed to protect the snapshot; use separate checkouts or serialized calls.

Authenticated pulls and image exports occur on the host. The pinned Trivy
container receives archives, never Docker credentials or the Docker socket. The
reports include `summary.tsv`, `db-snapshot.json`, and per-reference
`report.json`, `sbom.spdx.json`, `gate.txt`, `policy.rego`, and resolved
identity. Generated reports are gitignored. CI uploads evidence even on failure,
with seven-day retention.

## Exceptions

Existing reviewed exceptions retain owners, rationale, approval and expiry; this
lifecycle change adds no advisory suppression or expiry extension. The
[validator](../tools/quality/image-scan-exceptions-check.sh) requires exact
image/digest agreement and rejects unknown/duplicate fields or invalid approvals
and dates. Local image names no longer bypass digest matching.

An inherited exception requires **both** `package` and `installed_version` in
addition to its exact pinned base, identifier, owner, rationale, approval and
expiry. Native Trivy Rego matches all three finding fields. Upgraded packages,
changed versions, missing metadata, and unrelated packages cannot inherit the
exception. Unscoped inheritance fails closed. Full reports retain every excepted
finding. Exceptions are not added just to make an update pass.

## Operator cutover and rollback

Install the reviewed root-owned runtime bundle before activating the new forced
command protocol. Both deployment jobs fail closed unless the operator sets the
GitHub repository variable `HOSTED_DEPENDENCY_PROTOCOL_READY=true` after
verifying the reviewed bundle in the respective SSH context. Leave it unset
during installation and do not treat the flag as host-hash evidence. CI is not
authorized to replace root-owned definitions:

```text
dev:        deploy BACKEND WEB SOPS POSTGRES RESTIC REVISION
production: deploy BACKEND WEB KEYCLOAK SOPS POSTGRES RESTIC REVISION
```

All utility refs are immutable development/release aliases with matching trusted
revision signatures. SOPS must remain anonymously pullable because decryption
precedes GHCR login. Verify package visibility after first publication.
PostgreSQL/Restic are verified before their pulls; active refs are recorded in
host metadata. Rollback preserves prior database and identity refs rather than
assuming application and identity databases used the same old image.

Legacy command arities and bootstrap pins remain only for staged compatibility;
they are not evidence that a live host has adopted the patched dependencies.
Confirm both environments, the independent watch stack, scheduled backups and
host Cloudflared installation before removing that compatibility. Do not deploy
this protocol to an old root bundle or describe repository-only tests as live
cutover evidence.

## Validation

Run `make test-image-audit`, `make test-image-audit-native`,
`make test-image-scan-exceptions-regression`, `make test-dependency-images`, and
`make hosted-contract-test`. Native Trivy fixtures prove fixed High/Critical
blocking and package/version-scoped exceptions without relying on a registry
outage. Behavioral transport fixtures prove retries, complete reporting,
missing-artifact failure, digest verification and no build calls during
scanning. Deployment/gate changes also require `make preflight` and complete
`make verify` on the exact revision, including operational rehearsals. No
vulnerability gate is disabled to accept a new dependency artifact.
