# Image security scanning

Every final/runtime image that GeoGuessMe ships is scanned for known
vulnerabilities by [Trivy](https://trivy.dev) through `make audit-images`. The
gate is deterministic: it fails on **fixed** High/Critical findings (a fix is
available in a newer release) and merely reports unfixed High/Critical findings,
which cannot be remediated by bumping a version.

## Scope of `make audit-images`

The target scans the following images (see `AUDIT_IMAGES` in
`tools/make/deployment.mk`):

- **Database** —
  `geoguessme/postgres-openssl:15.19-openssl-3.5.8-libuuid-2.42.3`, a locally
  rebuilt `postgres:15-alpine` (digest-pinned base) that layers the OpenSSL and
  libuuid packages refreshed to fixed releases (CVE-2026-14456 and the
  CVE-2026-53612 family). The upstream postgres image still ships the vulnerable
  package versions.
- **Web server** — the digest-pinned Caddy 2.11.4 frontend image, which also
  refreshes curl/libcurl to 8.22.0-r0 and OpenSSL to the fixed releases at build
  time. Until the official image includes the `golang.org/x/net` v0.56.0 fix,
  the Dockerfile rebuilds the released Caddy binary from the pinned official
  builder with that dependency override; the resulting application image is
  scanned directly. The watch gateway deliberately reuses this exact released
  web artifact through `WEB_IMAGE`, so it does not introduce a second unpatched
  Caddy runtime.
- **Deployment utilities** — the project-owned `geoguessme-sops` image derives
  from the digest-pinned upstream SOPS v3.13.3 image and refreshes `libexpat1`
  to Debian's fixed `2.5.0-1+deb12u4`. The upstream SOPS image is only a build
  input; CI scans and signs the exact published derivative, verifies anonymous
  access to its digest, and production promotes that same development digest
  without rebuilding. When a project SOPS digest is supplied, the host verifies
  its workflow identity and source revision before pulling or invoking it. The
  first monitored runtime cutover temporarily retains a digest-pinned upstream
  bootstrap for legacy command arities; it is not verified with this project's
  workflow signature and is removed by the final runtime update. The package is
  public because host-side decryption precedes GHCR login; it contains only the
  SOPS utility, not application secrets. GHCR's initial private-package default
  requires a one-time visibility change after first push. All libexpat
  exceptions were removed after the package fix; remaining SOPS exceptions are
  time-boxed and limited to unrelated upstream base/runtime findings.

    `geoguessme/cloudflared-tools:2026.9.1-openssl-3.5.7` is a locally rebuilt
    cloudflared with the OpenSSL libraries refreshed to the fixed Debian
    release; the upstream distroless-based image cannot run package tools, so
    only the two OpenSSL libraries and their dpkg metadata are layered on top.
    Its unpatched upstream digest is only a build input, so the audit scans the
    patched final image rather than failing on the vulnerable intermediate. The
    pinned Restic release is rebuilt on an alpine runtime whose OpenSSL is
    refreshed in the same way plus the fixed `golang.org/x/net` module, and the
    remediation images are scanned by their exact image-ID digest — refreshing a
    remediation layer changes that ID and requires the committed exceptions to
    be reviewed and repointed, failing closed otherwise. The Restic image is
    published and signed with the exact development revision because hosted
    backup and restore operations run it on deployment hosts. Terraform is
    rebuilt with the same dependency fix and covered by `make terraform-test`;
    it is a local planning tool, not a shipped runtime.

- **Application images** — appended automatically:
    - from the `BACKEND_IMAGE` / `WEB_IMAGE` environment variables when set (CI
      publishes and release promotion scan the exact `name@sha256` digests);
    - otherwise the locally built `geoguessme-backend:local` /
      `geoguessme-web:local` images when they exist (produced by
      `make build-images`);
    - otherwise a warning is printed and application images are skipped.
- **Identity image** — the locally built `geoguessme-keycloak:local` image, or
  the exact `KEYCLOAK_IMAGE` digest supplied by CI. It derives from the
  digest-pinned
  [Keycloak 26.7.5 release](https://www.keycloak.org/2026/09/keycloak-2675-released),
  which includes FreeMarker 2.3.35 and Quarkus 3.33.4 (managing Jackson Databind
  2.21.7). Publication and production promotion scan the exact signed digest;
  production rollback retains the previous identity image reference in hosted
  deployment metadata.

Override the third-party image list with
`AUDIT_IMAGES="img1@sha256:... img2@sha256:..."`. `SOPS_IMAGE` is always
appended separately; local audits use the patched local tag and CI/release jobs
override it with the exact published digest. Third-party and published images
use pinned digests, never floating tags. Local images use explicit `:local`
build names and are scanned by their Docker image ID. Images already available
in the host Docker daemon are exported with `docker save` and scanned from a
tarball. This includes private application and Keycloak digests pulled by
authenticated publication and promotion workflows, so registry credentials never
enter the Trivy container. Other registry images are scanned directly by their
digest-pinned reference.

## Blocking semantics

For every image the target runs three Trivy passes:

| Pass          | Command shape                                                                        | Result                                                                                                  |
| ------------- | ------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- |
| JSON report   | `trivy image --severity HIGH,CRITICAL --exit-code 0 --format json`                   | Full High/Critical findings (fixed **and** unfixed) written to `report.json`; never fails the gate      |
| SBOM          | `trivy image --skip-db-update --format spdx-json`                                    | Software bill of materials written to `sbom.spdx.json`                                                  |
| Blocking gate | `trivy image --severity HIGH,CRITICAL --ignore-unfixed --exit-code 1 --format table` | Fails the gate when any High/Critical finding **has a fix** and is not covered by a committed exception |

`--ignore-unfixed` makes Trivy report only findings with an available fix, so
the gate fails exclusively on **fixed** High/Critical findings. Unfixed findings
never block — they are captured in `report.json` for triage.

All passes run through the Dockerized Trivy tool service
(`deployment/compose.tools.yaml`); nothing runs on the host directly.

## Exceptions

Committed exceptions live in `tools/quality/image-scan-exceptions.yaml` and the
component-specific `tools/quality/image-scan-exceptions-keycloak.yaml`,
`tools/quality/image-scan-exceptions-oauth2-proxy.yaml`,
`tools/quality/image-scan-exceptions-cloudflared.yaml`, and
`tools/quality/image-scan-exceptions-sops.yaml`. They are validated together by
`tools/quality/image-scan-exceptions-check.sh`. Each entry requires all of the
following fields:

- `id` — the CVE/GHSA identifier;
- `image` — the exact scanned image reference (the same string passed to
  `AUDIT_IMAGES`);
- `digest` — the exact `sha256:<64 hex>` digest the exception applies to;
- `owner` — the accountable operator or team;
- `reachable` — a one-line reachability rationale;
- `approved: true` — explicit approval;
- `expires` — an ISO date (`YYYY-MM-DD`) that is today or later and at most 30
  days after the exception is recorded.

The validator fails the gate on any missing field, a malformed or unpinned image
digest, disagreement between the image reference and digest field, `approved`
other than `true`, an unknown key, or an expired or over-30-day expiry. In
`--emit` mode it writes the per-image Trivy ignorefile (used only by the
blocking pass) containing only the exceptions that match the scanned reference
or its name plus digest. The JSON report remains complete and includes excepted
findings.

Application images also carry the standard OCI `base.name` and `base.digest`
labels. When an image is available in the Docker daemon, the gate validates both
labels and appends exceptions belonging to that exact digest-pinned base image.
This lets a reviewed upstream exception follow unchanged base layers into the
backend or web image without creating impossible pre-build exceptions for a
publication digest that does not exist yet. A missing half, malformed digest, or
digest embedded in `base.name` fails closed. Application layers must not install
or replace operating-system packages after the labelled base stage.

Rules:

- Only **fixed** High/Critical findings may be excepted. Unfixed findings are
  never blocked, so they need no exception and must not be silenced.
- An exception expires after at most 30 days; when it expires the pin must be
  refreshed or the exception renewed through an approved remediation review.

## Reports, SBOMs, and retention

Per-image output lands in `security/image-reports/<sanitized-ref>/`:

- `report.json` — full High/Critical findings (including unfixed);
- `sbom.spdx.json` — SPDX software bill of materials;
- `ignore.trivy` — the derived Trivy ignorefile for that image.

The `security/image-reports/` directory is gitignored and is never committed. CI
uploads the reports and SBOMs as build artifacts with bounded retention
(approximately seven days) and no secrets.

## Where the gate runs

`make audit-images` runs:

- inside `make verify` (the complete release gate), and therefore in the nightly
  and post-merge development pipeline;
- at image publication, scanning the exact built digests before signing;
- at release promotion, scanning the exact promoted digest again before
  production;
- in the weekly security workflow, keeping the pinned set continuously reviewed.

## Refreshing pins

When the blocking gate reports a fixed High/Critical finding:

1. Bump the image pin to the newest compatible stable release.
2. Resolve and record its content digest (the image must remain pinned as
   `name@sha256:...`; never downgrade a digest pin to a floating tag).
3. Re-run `make audit-images` and repeat until the fixed finding is gone.
4. If a newer upstream version is not yet available, apply the fix in the
   shipped image when the dependency can be safely rebuilt and verified; do not
   bypass the blocking gate with an exception merely to release.
