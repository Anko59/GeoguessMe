# Authentication

## Access / refresh token flow

GeoGuessMe uses a split-token authentication scheme:

1. **Login** (`POST /api/v1/auth/login`) or **Signup**
   (`POST /api/v1/auth/signup`) returns:
    - `access_token` (JWT, short-lived) in the JSON response body
    - `refresh_token` (opaque, long-lived) as an HttpOnly cookie
2. The access token is sent on every authenticated request as
   `Authorization: Bearer <token>`.
3. When the access token expires, the client calls `POST /api/v1/auth/refresh` —
   the backend reads the refresh cookie, rotates the session, and issues new
   credentials.

### Access token (JWT)

- Algorithm: `HS256`
- Expiry: configured via `ACCESS_TOKEN_TTL` (default 15 minutes)
- Claims: `user_id`, `auth_version`, `token_type: "access"`, `iss`, `aud`
- Issuer: `geoguessme`
- Audience: `geoguessme-web`
- The frontend stores this in memory only (never `localStorage` or cookies).

### Refresh cookie

- Name: `refresh_token`
- Path: `/api/v1/auth`
- `HttpOnly`: true
- `SameSite`: `Lax`
- `Secure`: true in production (`APP_ENV=production`), false in development
- Expiry: configured via `REFRESH_TOKEN_TTL` (default 30 days)
- The cookie is cleared on logout and on refresh failure.

### Refresh rotation

Each use of a refresh token creates a new session and retires the old one in a
single database transaction. If a refresh token has already been revoked or
consumed, the rotation fails and the cookie is cleared.

The browser coordinates startup restoration and 401 recovery through one
single-flight refresh request. This prevents React development checks or
concurrent API failures from consuming the same one-time refresh cookie twice.
Hard reloads therefore restore the session in Chromium and Firefox as long as
the refresh cookie is present and valid.

## Verification

- `POST /api/v1/auth/verify/request` (authenticated) sends a verification email.
- `POST /api/v1/auth/verify {token}` consumes a single-use opaque token.
- Token TTL: `VERIFICATION_TOKEN_TTL` (default 24 hours).
- Tokens are stored hashed (SHA-256); older unverified tokens for the same user
  are invalidated on each new request.
- Before sending, any existing unverified token for the user is deleted.

Token URL format: `{PUBLIC_URL}/verify-email?token={raw}`.

## Password reset

- `POST /api/v1/auth/password/forgot {email}` sends a reset link (always returns
  202 to prevent email enumeration).
- `POST /api/v1/auth/password/reset {token, password}` atomically consumes the
  token, updates the password hash, bumps `auth_version`, and revokes all
  refresh sessions.
- Token TTL: `RESET_TOKEN_TTL` (default 1 hour).

Authenticated users can update their username, email address, or selected
profile avatar through `PATCH /api/v1/auth/profile`; the current password is
required and changing the email clears its verification state. Password changes
use `POST /api/v1/auth/password/change`, require the current password, and
revoke all sessions so the user must sign in again.

## Logout

- `POST /api/v1/auth/logout` — revokes the current refresh session and clears
  the cookie. Returns 204.
- `POST /api/v1/auth/logout?all=1` — revokes all refresh sessions for the user
  **and** bumps `auth_version`, invalidating every outstanding access token.

## Auth version and immediate revocation

Every user has an `auth_version` column (integer, starts at 0). The JWT access
token records the user's `auth_version` at issuance. On every authenticated
request, `AuthMiddleware` compares the claim against the stored value from the
database. If they differ, the request is rejected with 401.

Auth version is bumped by:

- Password reset
- Password change
- `logout?all=1`

This means password reset and global logout invalidate all outstanding access
tokens immediately, even before the short-lived JWT would have expired.

## Account deletion

`DELETE /api/v1/auth/account {password}` (authenticated, password confirmation):

1. Verifies the password.
2. Calls `DeleteUserCascade` which removes:
    - All owned media (queues durable deletion jobs for S3 objects)
    - All refresh sessions, verification tokens, password-reset tokens,
      WebSocket tickets
    - Cascade-deletes memberships, messages, guesses, challenge_views
3. Deletes the user row entirely (not a soft-delete), releasing the username and
   email for reuse.

Returns 204 on success.

## Threat model

| Threat                        | Mitigation                                                         |
| ----------------------------- | ------------------------------------------------------------------ |
| Access token theft (XSS)      | Token in memory only; server checks `auth_version` per request     |
| Refresh token theft           | HttpOnly cookie, path-restricted to `/api/v1/auth`, rotated on use |
| CSRF on refresh               | SameSite=Lax, cookie not sent on cross-site POST                   |
| Replay of refresh token       | One-time rotation: consumed token is revoked atomically            |
| Brute-force login             | Rate-limited by identity (`RateLimitByIdentity` on signup/login)   |
| Session after password change | Auth version bump revokes all tokens                               |
| Email enumeration             | Forgot password always returns 202                                 |
| Stale leak via logs           | Backend never logs tokens, signed URLs, passwords, or coordinates  |
