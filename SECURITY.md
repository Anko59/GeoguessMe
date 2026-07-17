# Security policy

## Threat model summary

- **Private media:** Photo media is never browser-facing. Uploads go to a
  private S3 bucket. Serving is same-origin through the authenticated backend
  (`/api/v1/challenges/{id}/media`) only during the active view window or with
  `?result=1` for authorized result viewing.
- **Auth version revocation:** Every access token embeds the account's
  `auth_version` as a claim. Password reset, account deletion, and "logout all"
  bump the version, immediately invalidating all outstanding access tokens for
  that account.
- **SMTP TLS policy:** Authenticated SMTP credentials are never transmitted
  over a plaintext link. The backend refuses `SMTP_TLS=off` when a username is
  present, and production enforces `SMTP_TLS=starttls` or `SMTP_TLS=tls`.
- **Read-only rootfs:** Production backend runs non-root with a read-only root
  filesystem and only `/tmp` writable.

## Reporting vulnerabilities

Report vulnerabilities **privately** to the project maintainers — do not open a
public issue. Include reproduction steps, affected endpoint and version, and
observed impact. Do not include real credentials, tokens, signed URLs, or
personal data in the report.

Supported deployments receive fixes on a reasonable best-effort basis. Rotate
any credential included in a report immediately after receiving a fix.
