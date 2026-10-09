# Image security scanning

Every required image is scanned by [Trivy](https://trivy.dev), including all
severities and findings without a published fix. Scanning remains mandatory;
prioritization decides which findings block delivery.

## Security decisions and ownership

The operator authorized this policy on October 9, 2026 after repeated upstream
compiler advisories consumed the development/release workflow. A scanner's
“fixed version” identifies a package fix, not an available compatible vendor
image. Automatically rebuilding vendors turned release lag into project
maintenance without establishing deployment exploitability.

- `make audit` keeps the strict application source/dependency gate: called Go
  vulnerabilities and high-severity npm findings block, with reviewed backport
  regressions retained.
- `make audit-images` blocks any finding in the fresh
  [CISA Known Exploited Vulnerabilities catalogue](https://www.cisa.gov/known-exploited-vulnerabilities-catalog),
  regardless of severity or whether a fix is available. Confirmed deployment
  exposures outside CISA are additive blockers in
  [blocking-cves.tsv](../tools/quality/image-audit/blocking-cves.tsv), with an
  accountable owner and evidence. No exclusion can suppress these blockers.
- Other runtime findings remain `ADVISORY`, with complete JSON and SPDX
  evidence. This explicitly accepts residual vendor risk; absence from CISA is
  not evidence of safety, non-reachability, or a completed exploitability
  review. A passing gate means the defined release criteria passed, not zero
  CVEs.
- Unknown image identity, missing artifacts, incomplete scans, invalid policy,
  and unavailable/stale exploit data still block with exit 2.

The catalogue is fetched from CISA's fixed HTTPS endpoint for each audit,
checked for valid/unique CVEs, matching count, and a release date within 30
days, then frozen and hashed alongside the Trivy database snapshot. No cached
feed or caller-selected endpoint can substitute for a failed refresh. CISA can
lag new exploitation; the additive exposure blocklist covers confirmed
actionable findings before catalogue inclusion.

Upstream owns third-party binaries and OS packages. Adopt compatible upstream
releases through the ordinary tested PR flow. The Docker Dependabot group checks
all image recipe directories weekly and opens one compatible-update PR; majors
require separate review. The runtime TSV inventory and host-tool pins remain
explicit reviewed inputs because Dependabot does not update arbitrary TSV/JSON.
No vulnerability-only source rebuild or OS package transplant is part of the
normal response. If an actionable finding has no vendor fix, mitigate exposure
or disable the affected optional service; a custom fork requires an explicit
product decision with an owner and retirement plan.

The seven existing content-keyed publications are signed deployment envelopes,
not maintained source forks. Caddy/Restic now use official released binaries;
SOPS, Cloudflared, PostgreSQL and socket-proxy retain upstream package contents.
Keycloak retains its runtime-user setting; Restic clears its upstream entrypoint
to preserve the existing explicit-command interface used by backups. No new
envelope is required merely because another vendor ships an older Go compiler.
Original artifact identity, signatures, and exact-digest promotion remain
unchanged.

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

### Reviewed one-time Keycloak publication recovery

The manually dispatched **Security** workflow on protected `dev` has one finite
recovery operation, `make recover-reviewed-keycloak`. It accepts no artifact
inputs and cannot run locally or on another branch/workflow. It recovers only:

- Input key: `cb1dd9163b630bbda840ba2906310e4b402f55e69bc81b4bc4c8f95c26511689`.
- Original index:
  `sha256:f3b02924f109607058d1238eab1c06ec7fdc9f6151beb5252ec765f3fe5060f3`.
- Original producer revision: `a228769dbaf98ce8a5aff7fb0946456f34f8e21b`.
- Original
  [Security publisher run and job](https://github.com/Anko59/GeoguessMe/actions/runs/37242418190/job/111553605968).

That publisher pushed the original bytes, then rejected BuildKit's current
SLSAv1 build-type identifier before scanning/signing. After explicit operator
approval, dispatch Security on `dev`; no deployment or readiness step is run.
The original log's ANSI bytes are captured in the private review file, never
rendered in a terminal. The operation authenticates the original run/job/log,
verifies the exact input/index/runtime, original VCS, embedded recipe, upstream
and local platform, and freshly runs the unchanged native audit on that original
index. Only then may it add the original `dependency-build=true` input signature
under the same trusted protected-workflow identity. Already-valid signatures are
verified and freshly scanned without re-signing. It never rebuilds, aliases or
replaces bytes.

Network/authentication errors, unavailable original evidence, changed inputs or
digest, invalid signatures, and failed scans stop recovery. Ordinary publication
still refuses all unsigned existing tags. Review and retire this dedicated job,
script and target after this incident; do not repurpose its constants for other
artifacts. `make test-reviewed-keycloak-recovery` tests the bounded exception
without credentials or registry writes.

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
`POSTGRES_IMAGE`, not an unreviewed mutable substitute. The shared identity
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

## Application tooling and temporary source patches

Application and Dockerized Go tooling use the pinned Go 1.26.9 compiler; keep
backend toolchain directives, tool tags, and fixture helpers aligned. The
backend requires `golang.org/x/net` 0.60.0 for the October HTTP fixes. Upstream
image compilers are vendor inputs, not reasons to add project-owned source
builds.

Scoped npm overrides preserve YAML v4/v5 and CommonJS UUID compatibility.
`make test-npm-security-overrides` verifies installed consumers; `make audit`
includes those and the braces regressions. These are application dependency
contracts, independent of runtime image prioritization. The Terraform operator
image's existing PCRE2 refresh is outside the shipped runtime inventory; retire
it on a compatible upstream refresh, without expanding that patch mechanism.

## Blocking semantics and operational errors

One audit prepares a single vulnerability/Java database snapshot, freezes image
identity and platform, and scans **every required image before failing**. Raw
JSON includes every severity, fixed and unfixed findings. Native Trivy
conversion generates SPDX and applies the reviewed exploitation policy to the
exact frozen report, without rescanning or silently changing databases. Content
and gate results are deduplicated by immutable identity and policy hash.

- Exit **0**: all required images completed; no known-exploited or confirmed
  deployment-exposure blocker remains. Advisory findings can still be present.
- Exit **1**: blocking exploitation/exposure findings, including unfixed ones.
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

## Retired exception mechanism

The severity-based image CVE exceptions and their expiry-renewal machinery are
removed. Known-exploited and confirmed-exposure findings cannot be excepted. The
old `make test-image-scan-exceptions-regression` name remains a compatibility
alias for the replacement risk-policy tests, rather than an expiration gate.

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
they are not evidence that a live host has adopted the selected dependencies.
Confirm both environments, the independent watch stack, scheduled backups and
host Cloudflared installation before removing that compatibility. Do not deploy
this protocol to an old root bundle or describe repository-only tests as live
cutover evidence.

## Validation

Run `make test-image-audit`, `make test-image-audit-native`,
`make test-dependency-images`, and `make hosted-contract-test`. Native Trivy
fixtures prove exploitation blocking, advisory retention, and
unfixed/low-severity blockers without relying on a registry outage. Behavioral
transport fixtures prove retries, complete reporting, missing-artifact failure,
digest verification and no build calls during scanning. Deployment/gate changes
also require `make preflight` and complete `make verify` on the exact revision,
including operational rehearsals. Source security gates remain strict; image
release decisions follow the explicit exploitation policy above.
