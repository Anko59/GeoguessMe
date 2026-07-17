# Security and privacy

## Authorization model

### Authentication

- Access tokens are short-lived JWTs (default 15 minutes) stored in browser
  memory only. Refresh tokens are HttpOnly cookies path-restricted to
  `/api/v1/auth` with SameSite=Lax.
- Every authenticated request checks the JWT's embedded `auth_version` against
  the database. This provides immediate revocation on password reset or
  `logout?all=1`.
- WebSocket connections use a separate one-time ticket mechanism (60-second TTL)
  — access tokens are never passed in WS URLs.

### Group membership

Most group-scoped operations (details, members, messages, leaderboard, photo
upload, challenge accept) verify the requesting user is a current member of the
target group. Non-members receive a 403 `forbidden` response.

Membership checks are also enforced at the repository layer inside transactions
(e.g., `AcceptChallenge` and `SubmitGuess` verify membership while holding a row
lock on the photo).

### Media access

Media is never served from a public URL. The backend proxies all image requests
through authenticated handlers:

- During the view window: `GET /api/v1/challenges/{photoID}/media`
- After results are visible: `GET /api/v1/challenges/{photoID}/media?result=1`
- The backend checks the exact view deadline from the database on every media
  request — a re-acceptance cannot extend access beyond the original window.
- Object keys are UUIDs with no predictable naming.

## Private media architecture

- S3 endpoints and object keys never reach the browser.
- The frontend fetches media as an authenticated blob and renders it via a
  short-lived `blob:` object URL (not a direct S3 URL).
- `Cache-Control: private, no-store` is set on media responses to prevent
  caching by shared proxies.

## Data inventory

| Data                           | Storage            | Retention                                              | Notes                                                    |
| ------------------------------ | ------------------ | ------------------------------------------------------ | -------------------------------------------------------- |
| Username, email, password hash | PostgreSQL (users) | Until account deletion                                 | Password hashed with bcrypt (`BCRYPT_COST=12` default)   |
| Photos and locations           | PostgreSQL + S3    | Until `PHOTO_RETENTION` (default 30 days) after upload | Actual coordinates stored only in DB; media stored in S3 |
| Guesses and scores             | PostgreSQL         | Indefinitely                                           | Aggregate leaderboard rows remain after media cleanup    |
| Messages                       | PostgreSQL         | Indefinitely                                           |                                                          |
| Refresh sessions               | PostgreSQL         | 30 days after expiry/revocation                        | Hashed (SHA-256)                                         |
| One-time tokens                | PostgreSQL         | 1 day after use; at TTL otherwise                      | Hashed (SHA-256)                                         |
| WebSocket tickets              | PostgreSQL         | 1 day after use; 60s TTL otherwise                     | Hashed (SHA-256)                                         |

## Account deletion

`DELETE /api/v1/auth/account` requires password confirmation and runs
`DeleteUserCascade` which:

1. Enqueues all authored media for S3 deletion via `media_deletion_jobs`
2. Deletes all tokens and sessions (refresh, verification, password reset, WS)
3. Cascade-deletes memberships, messages, guesses, challenge_views via foreign
   key constraints
4. Deletes the user row entirely (releases username/email for reuse)

## Operator obligations

- Run with TLS termination at the edge (managed ingress or reverse proxy).
- Restrict `/metrics` to internal monitoring.
- Never commit `.env` files or production secrets to version control.
- Configure `TRUSTED_PROXY_CIDRS` to the actual proxy network so client IP
  resolution is accurate for rate limiting.
- Set `LOG_LEVEL` to `info` or `warn` in production (`debug` may leak request
  details).
- Monitor the `geoguessme_storage_cleanup_backlog` metric — a growing backlog
  indicates object-store connectivity issues.

## Security reporting

Vulnerabilities should be reported privately to the project maintainers. See
[SECURITY.md](../SECURITY.md) for details. There is no bug bounty program.
