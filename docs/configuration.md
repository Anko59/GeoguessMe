# Configuration

All backend configuration is read once at startup via environment variables and
validated. Never commit a real `.env` or production secret.

## Variables

| Variable                 | Type     | Default                                       | Applies to | Validation                                                                                 |
| ------------------------ | -------- | --------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------ |
| `APP_ENV`                | string   | `development`                                 | All        | `production` enables additional security checks                                            |
| `PORT`                   | string   | `8080`                                        | All        | Must be integer 1–65535                                                                    |
| `PUBLIC_URL`             | string   | `http://localhost:5173`                       | All        | Used for email token URLs                                                                  |
| `DATABASE_URL`           | string   | — (required)                                  | All        | Must be set                                                                                |
| `DB_MIN_CONNS`           | int      | `2`                                           | All        | Must be ≥ 0                                                                                |
| `DB_MAX_CONNS`           | int      | `10`                                          | All        | Must be ≥ 1 and ≥ DB_MIN_CONNS                                                             |
| `JWT_SECRET`             | string   | — (required)                                  | All        | Must be ≥ 32 characters                                                                    |
| `ACCESS_TOKEN_TTL`       | duration | `15m`                                         | All        | Must be positive, shorter than REFRESH_TOKEN_TTL                                           |
| `REFRESH_TOKEN_TTL`      | duration | `720h` (30d)                                  | All        | Must be positive, longer than ACCESS_TOKEN_TTL                                             |
| `VERIFICATION_TOKEN_TTL` | duration | `24h`                                         | All        | Must be positive                                                                           |
| `RESET_TOKEN_TTL`        | duration | `1h`                                          | All        | Must be positive                                                                           |
| `BCRYPT_COST`            | int      | `12`                                          | All        | Must be 4–31                                                                               |
| `SMTP_HOST`              | string   | — (empty)                                     | All        | Required in production                                                                     |
| `SMTP_PORT`              | int      | `1025`                                        | All        | Must be 1–65535 if host is set                                                             |
| `SMTP_USERNAME`          | string   | —                                             | All        | Authenticated SMTP requires TLS                                                            |
| `SMTP_PASSWORD`          | string   | —                                             | All        |                                                                                            |
| `SMTP_FROM`              | string   | `no-reply@localhost`                          | All        | Required in production                                                                     |
| `SMTP_TLS`               | string   | `off`                                         | All        | Must be `off`, `starttls`, or `tls`. Cannot be `off` in production                         |
| `SMTP_DIAL_TIMEOUT`      | duration | `10s`                                         | All        | Must be positive                                                                           |
| `SMTP_TIMEOUT`           | duration | `30s`                                         | All        | Must be positive                                                                           |
| `S3_ENDPOINT`            | string   | `http://localhost:9000`                       | All        | Must be valid http(s) URL                                                                  |
| `S3_REGION`              | string   | `us-east-1`                                   | All        |                                                                                            |
| `S3_BUCKET`              | string   | `geoguessme-media`                            | All        | Must be non-empty                                                                          |
| `S3_ACCESS_KEY`          | string   | `minioadmin`                                  | All        | Must be non-empty                                                                          |
| `S3_SECRET_KEY`          | string   | `minioadmin`                                  | All        | Must be non-empty                                                                          |
| `S3_USE_PATH_STYLE`      | bool     | `true`                                        | All        |                                                                                            |
| `ALLOWED_ORIGINS`        | list     | `http://localhost:5173,http://localhost:3000` | All        | Must contain explicit origins, no wildcards. Each must be a valid URL with scheme and host |
| `TRUSTED_PROXY_CIDRS`    | list     | — (empty)                                     | All        | Used for rate-limit client IP resolution                                                   |
| `UPLOAD_MAX_BYTES`       | int64    | `5242880` (5 MiB)                             | All        | Must be > 0                                                                                |
| `UPLOAD_MAX_PIXELS`      | uint64   | `25000000` (25 MP)                            | All        | Must be > 0                                                                                |
| `CHALLENGE_TTL`          | duration | `24h`                                         | All        | Must be positive, > PHOTO_VIEW_WINDOW                                                      |
| `PHOTO_VIEW_WINDOW`      | duration | `10s`                                         | All        | Must be positive, < CHALLENGE_TTL                                                          |
| `PHOTO_RETENTION`        | duration | `720h` (30d)                                  | All        | Must be ≥ CHALLENGE_TTL                                                                    |
| `UPLOAD_DIR`             | string   | `./uploads`                                   | All        | Only used when `STORAGE_DRIVER=local`                                                      |
| `RATE_LIMIT_REQUESTS`    | int      | `10`                                          | All        | Must be > 0                                                                                |
| `RATE_LIMIT_WINDOW`      | duration | `1m`                                          | All        | Must be > 0                                                                                |
| `LOG_LEVEL`              | string   | `info`                                        | All        | `debug`, `info`, `warn`, `error`                                                           |
| `STORAGE_DRIVER`         | string   | — (uses S3)                                   | All        | Set to `local` to use local filesystem storage                                             |
| `METRICS_TOKEN`          | string   | —                                             | All        | Required in production. Bearer token required to access the `/metrics` endpoint.           |

## Production validation

When `APP_ENV=production`, the following additional checks apply:

- `SMTP_HOST` and `SMTP_FROM` are required
- `SMTP_TLS` cannot be `off`
- Authenticated SMTP (`SMTP_USERNAME` set) requires `starttls` or `tls`
- `METRICS_TOKEN` is required
- S3 endpoint must not be `http://localhost:*`

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
