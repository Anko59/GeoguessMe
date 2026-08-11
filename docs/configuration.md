# Configuration

All backend configuration is read once at startup via environment variables and
validated. Never commit a real `.env` or production secret.

## Variables

| Variable                          | Type     | Default                                        | Applies to | Validation                                                                                                                                                                                                                                                                                         |
| --------------------------------- | -------- | ---------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `APP_ENV`                         | string   | `development`                                  | All        | Must be one of `development`, `production`, `test`. `production` enables additional security checks                                                                                                                                                                                                |
| `PORT`                            | string   | `8080`                                         | All        | Must be integer 1–65535                                                                                                                                                                                                                                                                            |
| `PUBLIC_URL`                      | string   | `http://localhost:5173`                        | All        | Used for email token URLs; must use HTTPS in production                                                                                                                                                                                                                                            |
| `DATABASE_URL`                    | string   | — (required)                                   | All        | Must be set                                                                                                                                                                                                                                                                                        |
| `DB_MIN_CONNS`                    | int      | `2`                                            | All        | Must be ≥ 0                                                                                                                                                                                                                                                                                        |
| `DB_MAX_CONNS`                    | int      | `10`                                           | All        | Must be ≥ 1 and ≥ DB_MIN_CONNS                                                                                                                                                                                                                                                                     |
| `JWT_SECRET`                      | string   | — (required)                                   | All        | Must be ≥ 32 characters                                                                                                                                                                                                                                                                            |
| `ACCESS_TOKEN_TTL`                | duration | `15m`                                          | All        | Must be positive, shorter than REFRESH_TOKEN_TTL                                                                                                                                                                                                                                                   |
| `REFRESH_TOKEN_TTL`               | duration | `720h` (30d)                                   | All        | Must be positive, longer than ACCESS_TOKEN_TTL                                                                                                                                                                                                                                                     |
| `VERIFICATION_TOKEN_TTL`          | duration | `24h`                                          | All        | Must be positive                                                                                                                                                                                                                                                                                   |
| `RESET_TOKEN_TTL`                 | duration | `1h`                                           | All        | Must be positive                                                                                                                                                                                                                                                                                   |
| `BCRYPT_COST`                     | int      | `12`                                           | All        | Must be 4–31                                                                                                                                                                                                                                                                                       |
| `SMTP_HOST`                       | string   | — (empty)                                      | All        | Required in production                                                                                                                                                                                                                                                                             |
| `SMTP_PORT`                       | int      | `1025`                                         | All        | Must be 1–65535 if host is set                                                                                                                                                                                                                                                                     |
| `SMTP_USERNAME`                   | string   | —                                              | All        | Optional, but must be supplied together with `SMTP_PASSWORD`; authenticated SMTP requires TLS                                                                                                                                                                                                      |
| `SMTP_PASSWORD`                   | string   | —                                              | All        | Optional, but must be supplied together with `SMTP_USERNAME`                                                                                                                                                                                                                                       |
| `SMTP_FROM`                       | string   | `no-reply@localhost`                           | All        | Required in production                                                                                                                                                                                                                                                                             |
| `SMTP_TLS`                        | string   | `off`                                          | All        | Must be `off`, `starttls`, or `tls`. Cannot be `off` in production                                                                                                                                                                                                                                 |
| `SMTP_DIAL_TIMEOUT`               | duration | `10s`                                          | All        | Must be positive                                                                                                                                                                                                                                                                                   |
| `SMTP_TIMEOUT`                    | duration | `30s`                                          | All        | Must be positive                                                                                                                                                                                                                                                                                   |
| `S3_ENDPOINT`                     | string   | `http://localhost:9000`                        | All        | Must be valid http(s) URL; must use HTTPS in production                                                                                                                                                                                                                                            |
| `S3_REGION`                       | string   | `us-east-1`                                    | All        |                                                                                                                                                                                                                                                                                                    |
| `S3_BUCKET`                       | string   | `geoguessme-media`                             | All        | Must be non-empty                                                                                                                                                                                                                                                                                  |
| `S3_ACCESS_KEY`                   | string   | `minioadmin`                                   | All        | Must be non-empty                                                                                                                                                                                                                                                                                  |
| `S3_SECRET_KEY`                   | string   | `minioadmin`                                   | All        | Must be non-empty                                                                                                                                                                                                                                                                                  |
| `S3_USE_PATH_STYLE`               | bool     | `true`                                         | All        |                                                                                                                                                                                                                                                                                                    |
| `ALLOWED_ORIGINS`                 | list     | `http://localhost:5173,http://localhost:3000`  | All        | Must contain explicit origins, no wildcards. Each must be a valid URL with scheme and host                                                                                                                                                                                                         |
| `TRUSTED_PROXY_CIDRS`             | list     | — (empty)                                      | All        | Used for rate-limit client IP resolution                                                                                                                                                                                                                                                           |
| `UPLOAD_MAX_BYTES`                | int64    | `10485760` (10 MiB)                            | All        | Must be > 0; caps each normalized image or camera-recorded video clip                                                                                                                                                                                                                              |
| `AVATAR_MAX_BYTES`                | int64    | `26214400` (25 MiB)                            | All        | Must be > 0; caps the original profile photo before it is normalized to the stored avatar thumbnail                                                                                                                                                                                                |
| `UPLOAD_MAX_PIXELS`               | uint64   | `25000000` (25 MP)                             | All        | Must be > 0                                                                                                                                                                                                                                                                                        |
| `CHALLENGE_TTL`                   | duration | `24h`                                          | All        | Must be positive, > PHOTO_VIEW_WINDOW                                                                                                                                                                                                                                                              |
| `LOCATION_HIDE_DURATION`          | duration | `48h`                                          | All        | Must be positive; how long a hidden challenge location stays hidden                                                                                                                                                                                                                                |
| `PHOTO_VIEW_WINDOW`               | duration | `10s`                                          | All        | Must be positive, < CHALLENGE_TTL                                                                                                                                                                                                                                                                  |
| `PHOTO_RETENTION`                 | duration | `720h` (30d)                                   | All        | Must be ≥ CHALLENGE_TTL                                                                                                                                                                                                                                                                            |
| `UPLOAD_DIR`                      | string   | `./uploads`                                    | All        | Only used when `STORAGE_DRIVER=local`                                                                                                                                                                                                                                                              |
| `RATE_LIMIT_REQUESTS`             | int      | `10`                                           | All        | Must be > 0                                                                                                                                                                                                                                                                                        |
| `RATE_LIMIT_WINDOW`               | duration | `1m`                                           | All        | Must be > 0                                                                                                                                                                                                                                                                                        |
| `RATE_LIMIT_LOGIN`                | buckets  | `identity:10/1m,trustedIP:30/1m,global:300/1m` | All        | Login policy; comma-separated `type:limit/window` buckets.                                                                                                                                                                                                                                         |
| `RATE_LIMIT_SIGNUP`               | buckets  | `identity:3/1h,trustedIP:5/1h,global:60/1m`    | All        | Signup policy; bucket types must be unique.                                                                                                                                                                                                                                                        |
| `RATE_LIMIT_EMAIL`                | buckets  | `identity:3/1h,trustedIP:5/1h,global:30/1m`    | All        | Verification and recovery email policy.                                                                                                                                                                                                                                                            |
| `RATE_LIMIT_RESET`                | buckets  | `trustedIP:10/1h`                              | All        | Password-reset policy.                                                                                                                                                                                                                                                                             |
| `RATE_LIMIT_PUSH`                 | buckets  | `user:10/1h,trustedIP:20/1h`                   | All        | Push subscription policy.                                                                                                                                                                                                                                                                          |
| `RATE_LIMIT_DEFAULT`              | buckets  | `identity:10/1m,trustedIP:60/1m`               | All        | Default authenticated-route policy.                                                                                                                                                                                                                                                                |
| `RATE_LIMIT_FAIL_CLOSED`          | list     | `login,signup,email,reset`                     | All        | Known policies that reject new keys when the bounded counter store is full.                                                                                                                                                                                                                        |
| `RATE_LIMIT_STORE_CAP`            | int      | `50000`                                        | All        | Must be at least 1; process-wide maximum number of live rate-limit counters.                                                                                                                                                                                                                       |
| `LOG_LEVEL`                       | string   | `info`                                         | All        | `debug`, `info`, `warn`, `error`                                                                                                                                                                                                                                                                   |
| `STORAGE_DRIVER`                  | string   | — (uses S3)                                    | All        | Set to `local` to use local filesystem storage                                                                                                                                                                                                                                                     |
| `METRICS_TOKEN`                   | string   | —                                              | All        | Required in production. Must be ≥ 32 bytes (use `openssl rand -hex 32`). Bearer token for `/metrics`; compared in constant time. Whitespace is trimmed on load.                                                                                                                                    |
| `VAPID_PUBLIC_KEY`                | string   | —                                              | All        | Optional complete Web Push keypair. 65-byte P-256 uncompressed point (base64url). Generate with `geoguessme vapid-keys`.                                                                                                                                                                           |
| `VAPID_PRIVATE_KEY`               | string   | —                                              | All        | Required when `VAPID_PUBLIC_KEY` is set. 32-byte P-256 scalar (base64url).                                                                                                                                                                                                                         |
| `VAPID_SUBJECT`                   | string   | —                                              | All        | Required with a VAPID keypair. Push service contact, either `mailto:` or `https:` (e.g. `mailto:ops@example.com`).                                                                                                                                                                                 |
| `PUSH_ENDPOINT_ALLOWLIST`         | list     | four major providers                           | All        | Comma-separated DNS allowlist of push delivery hosts; an endpoint is accepted only when its host matches an entry exactly or as a subdomain. Default: `fcm.googleapis.com,push.services.mozilla.com,web-push.apple.com,wns.windows.com`. Must be non-empty in production when Web Push is enabled. |
| `PUSH_MAX_SUBSCRIPTIONS_PER_USER` | int      | `5`                                            | All        | Maximum distinct Web Push subscriptions one account may hold. Re-subscribing an existing endpoint never counts against the cap; a sixth new endpoint is rejected with `409 subscription_limit`. Enforced transactionally against races. Must be ≥ 1.                                               |
| `PUSH_SUBSCRIPTION_EXPIRY`        | duration | `2160h` (90d)                                  | All        | Idle window after which the cleanup worker deletes an unused subscription, measured from the last successful delivery (`last_used_at`) or creation when never used. Must be positive.                                                                                                              |
| `PUSH_DELIVERY_WORKERS`           | int      | `4`                                            | All        | Global Web Push delivery concurrency: at most this many sends run at once across all hosts. Must be ≥ 1.                                                                                                                                                                                           |
| `PUSH_DELIVERY_PER_HOST`          | int      | `2`                                            | All        | Concurrent sends permitted to a single push-service host. Must be ≥ 1.                                                                                                                                                                                                                             |
| `PUSH_DELIVERY_TIMEOUT`           | duration | `5s`                                           | All        | Per-send deadline for one push HTTP POST. Must be positive.                                                                                                                                                                                                                                        |
| `PUSH_QUEUE_DEPTH`                | int      | `256`                                          | All        | Bounded fan-out job queue. When full, new notifications are dropped (counted in `push_drops_total`) rather than blocking the triggering request. Must be ≥ 1.                                                                                                                                      |

## Production validation

When `APP_ENV=production`, the following additional checks apply:

- `SMTP_HOST` and `SMTP_FROM` are required
- `SMTP_TLS` cannot be `off`
- Authenticated SMTP (`SMTP_USERNAME` set) requires `starttls` or `tls`
- `METRICS_TOKEN` is required and must be at least 32 bytes (after trimming)
- Web Push is enabled only when `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, and
  `VAPID_SUBJECT` are set together. Leaving all three unset explicitly disables
  Push in production; a partial configuration is rejected.
- `PUSH_ENDPOINT_ALLOWLIST` must be non-empty when Web Push is enabled; an
  explicit empty value rejects startup so delivery can never target an arbitrary
  host. Push delivery also refuses redirects and dials only globally routable
  addresses (never loopback, private, link-local, metadata, multicast,
  documentation, or unspecified ranges).
- The Web Push lifecycle and delivery bounds (`PUSH_MAX_SUBSCRIPTIONS_PER_USER`,
  `PUSH_SUBSCRIPTION_EXPIRY`, `PUSH_DELIVERY_WORKERS`, `PUSH_DELIVERY_PER_HOST`,
  `PUSH_DELIVERY_TIMEOUT`, `PUSH_QUEUE_DEPTH`) must all be positive; zero or
  negative values reject startup so a misconfiguration can never disable the
  subscription cap or remove the delivery deadlines.
- S3 endpoint must use HTTPS and must not be local MinIO

`APP_ENV` itself must be one of `development`, `production`, or `test` in every
environment; any other value is rejected at startup so the metrics
authentication decision is unambiguous.

## Hosted operator variables

The hosted dotenv also contains values consumed by PostgreSQL and backup
containers rather than the backend: `POSTGRES_USER`, `POSTGRES_PASSWORD`,
`POSTGRES_DB`, `RESTIC_REPOSITORY`, `RESTIC_PASSWORD`, `AWS_ACCESS_KEY_ID`, and
`AWS_SECRET_ACCESS_KEY`. Dev and production must use different PostgreSQL,
application, age, and R2 media credentials. Backup credentials are scoped only
to the private backup bucket. Both remote environments deliberately use
`APP_ENV=production`; dev is distinguished by its URL, project, port, bucket,
credentials, and tighter resource limits.

## Example `.env` for development

```bash
APP_ENV=development
PORT=8080
DATABASE_URL=postgres://user:password@localhost:5432/geoguessme?sslmode=disable
JWT_SECRET=dev_secret_key_change_me_please_32_characters
ALLOWED_ORIGINS=http://localhost:5173
S3_ENDPOINT=http://localhost:9000
S3_BUCKET=geoguessme-media
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
SMTP_HOST=localhost
SMTP_PORT=1025
SMTP_FROM=no-reply@localhost
```

## Startup validation

The backend fails to start when a configured variable is present but malformed
(for example an integer variable set to `abc`). `APP_ENV`, `PORT`, `PUBLIC_URL`,
`S3_ENDPOINT`, `TRUSTED_PROXY_CIDRS`, database/rate-limit/SMTP integers,
timings, and upload byte limits are all strictly parsed at startup, and every
malformed variable is reported in a single error so an operator can fix the
whole configuration in one attempt. Defaults apply only when a variable is
absent, never when it is present but invalid. Empty values remain valid for
optional string settings (for example `STORAGE_DRIVER=` falls back to S3 and
`TRUSTED_PROXY_CIDRS=` means no trusted proxies).

## Testing

Tests supply explicit configuration (`APP_ENV=test` and explicit secrets via the
Dockerized test stack). Override by setting environment variables before running
the relevant Dockerized Make target.

## `.env` file lookup

Docker Compose reads environment from `deployment/env/*.env` files. The
production stack requires `deployment/env/production.env` (git-ignored). See
`deployment/env/production.env.example` for the template.
