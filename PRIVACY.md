# Privacy and data retention

This is the public-facing summary of the data GeoGuessMe stores and how long it
is retained. Implementation details and operator controls are documented in
[Security and privacy](docs/security-and-privacy.md).

## Data inventory

GeoGuessMe stores:

- **Account data:** username, email (normalized lowercase), bcrypt password
  hash, avatar URL, email verification status.
- **Group data:** memberships, typed messages (WebSocket chat).
- **Challenge data:** photo metadata, guesses (latitude/longitude), scores.
- **Session data:** refresh session hashes, one-time token hashes (verification,
  password reset), WebSocket ticket hashes.
- **Uploaded media:** JPEG/PNG/WebP photos. EXIF metadata (including GPS) is
  stripped during upload normalization.

## Retention

| Data                                    | Retention                                                                         | Mechanism                                                                                                          |
| --------------------------------------- | --------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Uploaded media                          | `PHOTO_RETENTION` (default 720h / 30 days)                                        | Background cleanup worker marks `lifecycle_status = 'removed'`, nulls `storage_key`, enqueues durable deletion job |
| Auth tokens                             | Expired + 30 days (refresh sessions) / expired + 1 day (one-time tokens, tickets) | Periodic `CleanupAuthTokens` sweep                                                                                 |
| Expired challenge views                 | 1 day after `view_expires_at`                                                     | `ExpireChallengeViews` sweep                                                                                       |
| Aggregate scores and challenge metadata | Indefinite                                                                        | Remain after media cleanup                                                                                         |

## Account deletion

When a user deletes their account:

1. Their sessions and authentication tokens are deleted.
2. Their photos, messages, guesses, challenge views, and group memberships are
   deleted from the database.
3. Their uploaded media is queued for asynchronous deletion from object storage.

Their username and email address are released immediately. Media deletion can
finish shortly after the database deletion because the storage cleanup is
asynchronous.

## Data requests

Operators should configure `PHOTO_RETENTION` and backup retention to match
applicable privacy obligations. Publish a contact address for data access,
correction, or deletion requests in your instance's documentation.

For technical retention mechanisms, authorization behavior, and operator
responsibilities, see [Security and privacy](docs/security-and-privacy.md).
