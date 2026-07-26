# Configuration

All backend configuration is read once at startup via environment variables and
validated. Never commit a real `.env` or production secret.

## Variables

| Variable                 | Type     | Default                                       | Applies to | Validation                                                                                                                                                      |
| ------------------------ | -------- | --------------------------------------------- | ---------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `APP_ENV`                | string   | `development`                                 | All        | Must be one of `development`, `production`, `test`. `production` enables additional security checks                                                             |
| `PORT`                   | string   | `8080`                                        | All        | Must be integer 1–65535                                                                                                                                         |
| `PUBLIC_URL`             | string   | `http://localhost:5173`                       | All        | Used for email token URLs; must use HTTPS in production                                                                                                         |
| `DATABASE_URL`           | string   | — (required)                                  | All        | Must be set                                                                                                                                                     |
| `DB_MIN_CONNS`           | int      | `2`                                           | All        | Must be ≥ 0                                                                                                                                                     |
| `DB_MAX_CONNS`           | int      | `10`                                          | All        | Must be ≥ 1 and ≥ DB_MIN_CONNS                                                                                                                                  |
| `JWT_SECRET`             | string   | — (required)                                  | All        | Must be ≥ 32 characters                                                                                                                                         |
| `ACCESS_TOKEN_TTL`       | duration | `15m`                                         | All        | Must be positive, shorter than REFRESH_TOKEN_TTL                                                                                                                |
| `REFRESH_TOKEN_TTL`      | duration | `720h` (30d)                                  | All        | Must be positive, longer than ACCESS_TOKEN_TTL                                                                                                                  |
| `VERIFICATION_TOKEN_TTL` | duration | `24h`                                         | All        | Must be positive                                                                                                                                                |
| `RESET_TOKEN_TTL`        | duration | `1h`                                          | All        | Must be positive                                                                                                                                                |
| `BCRYPT_COST`            | int      | `12`                                          | All        | Must be 4–31                                                                                                                                                    |
| `SMTP_HOST`              | string   | — (empty)                                     | All        | Required in production                                                                                                                                          |
| `SMTP_PORT`              | int      | `1025`                                        | All        | Must be 1–65535 if host is set                                                                                                                                  |
| `SMTP_USERNAME`          | string   | —                                             | All        | Optional, but must be supplied together with `SMTP_PASSWORD`; authenticated SMTP requires TLS                                                                   |
| `SMTP_PASSWORD`          | string   | —                                             | All        | Optional, but must be supplied together with `SMTP_USERNAME`                                                                                                    |
| `SMTP_FROM`              | string   | `no-reply@localhost`                          | All        | Required in production                                                                                                                                          |
| `SMTP_TLS`               | string   | `off`                                         | All        | Must be `off`, `starttls`, or `tls`. Cannot be `off` in production                                                                                              |
| `SMTP_DIAL_TIMEOUT`      | duration | `10s`                                         | All        | Must be positive                                                                                                                                                |
| `SMTP_TIMEOUT`           | duration | `30s`                                         | All        | Must be positive                                                                                                                                                |
| `S3_ENDPOINT`            | string   | `http://localhost:9000`                       | All        | Must be valid http(s) URL; must use HTTPS in production                                                                                                         |
| `S3_REGION`              | string   | `us-east-1`                                   | All        |                                                                                                                                                                 |
| `S3_BUCKET`              | string   | `geoguessme-media`                            | All        | Must be non-empty                                                                                                                                               |
| `S3_ACCESS_KEY`          | string   | `minioadmin`                                  | All        | Must be non-empty                                                                                                                                               |
| `S3_SECRET_KEY`          | string   | `minioadmin`                                  | All        | Must be non-empty                                                                                                                                               |
| `S3_USE_PATH_STYLE`      | bool     | `true`                                        | All        |                                                                                                                                                                 |
| `ALLOWED_ORIGINS`        | list     | `http://localhost:5173,http://localhost:3000` | All        | Must contain explicit origins, no wildcards. Each must be a valid URL with scheme and host                                                                      |
| `TRUSTED_PROXY_CIDRS`    | list     | — (empty)                                     | All        | Used for rate-limit client IP resolution                                                                                                                        |
| `UPLOAD_MAX_BYTES`       | int64    | `5242880` (5 MiB)                             | All        | Must be > 0                                                                                                                                                     |
| `UPLOAD_MAX_PIXELS`      | uint64   | `25000000` (25 MP)                            | All        | Must be > 0                                                                                                                                                     |
| `CHALLENGE_TTL`          | duration | `24h`                                         | All        | Must be positive, > PHOTO_VIEW_WINDOW                                                                                                                           |
| `PHOTO_VIEW_WINDOW`      | duration | `10s`                                         | All        | Must be positive, < CHALLENGE_TTL                                                                                                                               |
| `PHOTO_RETENTION`        | duration | `720h` (30d)                                  | All        | Must be ≥ CHALLENGE_TTL                                                                                                                                         |
| `UPLOAD_DIR`             | string   | `./uploads`                                   | All        | Only used when `STORAGE_DRIVER=local`                                                                                                                           |
| `RATE_LIMIT_REQUESTS`    | int      | `10`                                          | All        | Must be > 0                                                                                                                                                     |
| `RATE_LIMIT_WINDOW`      | duration | `1m`                                          | All        | Must be > 0                                                                                                                                                     |
| `LOG_LEVEL`              | string   | `info`                                        | All        | `debug`, `info`, `warn`, `error`                                                                                                                                |
| `STORAGE_DRIVER`         | string   | — (uses S3)                                   | All        | Set to `local` to use local filesystem storage                                                                                                                  |
| `METRICS_TOKEN`          | string   | —                                             | All        | Required in production. Must be ≥ 32 bytes (use `openssl rand -hex 32`). Bearer token for `/metrics`; compared in constant time. Whitespace is trimmed on load. |
| `VAPID_PUBLIC_KEY`       | string   | —                                             | All        | Required in production. 65-byte P-256 uncompressed point (base64url). Generate with `geoguessme vapid-keys`.                                                    |
| `VAPID_PRIVATE_KEY`      | string   | —                                             | All        | Required in production with PUBLIC_KEY. 32-byte P-256 scalar (base64url).                                                                                       |
| `VAPID_SUBJECT`          | string   | —                                             | All        | Required in production. Push service contact, either `mailto:` or `https:` (e.g. `mailto:ops@example.com`).                                                     |

## Production validation

When `APP_ENV=production`, the following additional checks apply:

- `SMTP_HOST` and `SMTP_FROM` are required
- `SMTP_TLS` cannot be `off`
- Authenticated SMTP (`SMTP_USERNAME` set) requires `starttls` or `tls`
- `METRICS_TOKEN` is required and must be at least 32 bytes (after trimming)
- `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, and `VAPID_SUBJECT` are required for
  Web Push notifications (generate keys with `geoguessme vapid-keys`, set
  `VAPID_SUBJECT=mailto:operator@example.com`)
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

## Testing

Tests auto-detect the test environment and use safe defaults when variables are
not set. Override by setting environment variables before running the relevant
Dockerized Make target.

## `.env` file lookup

Docker Compose reads environment from `deployment/env/*.env` files. The
production stack requires `deployment/env/production.env` (git-ignored). See
`deployment/env/production.env.example` for the template.
