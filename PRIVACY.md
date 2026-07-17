# Privacy and data retention

## Data inventory

GeoGuessMe stores:

- **Account data:** username, email (normalized lowercase), bcrypt password
  hash, avatar URL, email verification status.
- **Group data:** memberships, typed messages (WebSocket chat).
- **Challenge data:** photo metadata, guesses (latitude/longitude), scores.
- **Session data:** refresh session hashes, one-time token hashes
  (verification, password reset), WebSocket ticket hashes.
- **Uploaded media:** JPEG/PNG/WebP photos. EXIF metadata (including GPS) is
  stripped during upload normalization.

## Retention

| Data | Retention | Mechanism |
|---|---|---|
| Uploaded media | `PHOTO_RETENTION` (default 720h / 30 days) | Background cleanup worker marks `lifecycle_status = 'removed'`, nulls `storage_key`, enqueues durable deletion job |
| Auth tokens | Expired + 30 days (refresh sessions) / expired + 1 day (one-time tokens, tickets) | Periodic `CleanupAuthTokens` sweep |
| Expired challenge views | 1 day after `view_expires_at` | `ExpireChallengeViews` sweep |
| Aggregate scores and challenge metadata | Indefinite | Remain after media cleanup |

## Account deletion

When a user deletes their account (`DELETE /auth/account`), the
`DeleteUserCascade` transaction:

1. Collects every `storage_key` from photos authored by the user and enqueues
   them as durable deletion jobs (`media_deletion_jobs`).
2. Deletes all refresh sessions, email verification tokens, password reset
   tokens, and WebSocket tickets for the account.
3. Deletes the user row. Database `ON DELETE CASCADE` removes photos,
   messages, guesses, challenge views, and group memberships.
4. Bumps `auth_version` (already handled by the cascade path).

Media is deleted asynchronously: the deletion jobs are drained by the
background cleanup worker against object storage. Account identity (username,
email) is released immediately.

## Data requests

Operators should configure `PHOTO_RETENTION` and backup retention to match
applicable privacy obligations. Publish a contact address for data access,
correction, or deletion requests in your instance's documentation.

For detailed technical description, see
[docs/security-and-privacy.md](docs/security-and-privacy.md).
