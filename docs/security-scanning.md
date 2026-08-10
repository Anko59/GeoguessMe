# Image security scanning

Every final/runtime image that GeoGuessMe ships is scanned for known
vulnerabilities by [Trivy](https://trivy.dev) through `make audit-images`. The
gate is deterministic: it fails on **fixed** High/Critical findings (a fix is
available in a newer release) and merely reports unfixed High/Critical findings,
which cannot be remediated by bumping a version.

## Scope of `make audit-images`

The target scans the following images (see `AUDIT_IMAGES` in
`tools/make/deployment.mk`):

- **Database** — `postgres:15-alpine` (digest-pinned).
- **Web server** — `caddy:2.11.4-alpine` (digest-pinned; the frontend final
  image is Caddy-based).
- **Deployment utilities** — `cloudflare/cloudflared`, `hashicorp/terraform`,
  `ghcr.io/getsops/sops`, and `restic/restic` (all digest-pinned).
- **Application images** — appended automatically:
    - from the `BACKEND_IMAGE` / `WEB_IMAGE` environment variables when set (CI
      publishes and release promotion scan the exact `name@sha256` digests);
    - otherwise the locally built `geoguessme-backend:local` /
      `geoguessme-web:local` images when they exist (produced by
      `make build-images`);
    - otherwise a warning is printed and application images are skipped.

Override the image list with `AUDIT_IMAGES="img1@sha256:... img2@sha256:..."`.
Images are always referenced by pinned digest, never by a floating tag. Images
already available in the host Docker daemon are exported with `docker save` and
scanned from a tarball. This includes private application digests pulled by the
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

Committed exceptions live in `tools/quality/image-scan-exceptions.yaml` and are
validated by `tools/quality/image-scan-exceptions-check.sh`. Each entry requires
all of the following fields:

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
4. If a newer version is not yet available, record a committed exception with an
   owner and a 30-day expiry, then revisit it before it expires.
