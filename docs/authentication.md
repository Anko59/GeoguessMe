# Authentication

## Access / refresh token flow

GeoGuessMe uses a split-token authentication scheme:

1. A normal login or signup completes in Keycloak and is exchanged through
   `POST /api/v1/auth/oidc/session`. The hidden legacy migration page may call
   `POST /api/v1/auth/login` once for an unmigrated account. Either path
   returns:
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

For a faster installed-PWA startup, the frontend keeps a local hint of the last
authenticated profile. It is never an access token and does not bypass refresh:
the refresh cookie is still checked in the background before new requests are
authorized. A failed refresh or logout removes the hint and that user's cached
chat records.

## Keycloak login, signup, and social providers

When `OIDC_ENABLED=true`, both normal login and signup use Keycloak exclusively.
Native email/password is a first-class Keycloak method; Google is the optional
social method enabled for this rollout. Apple and GitHub are deliberately
deferred. A provider button starts a fresh authorization request with an
allow-listed `kc_idp_hint`. The email path carries only `login_hint`; Keycloak's
branded page collects the password, so the application never receives or relays
a Keycloak password. Signup also sends the standard `prompt=create` parameter
and requires Keycloak email verification before a session is issued.

`OIDC_SOCIAL_PROVIDERS` is the application's explicit availability contract.
Only configured providers are rendered. Keycloak independently disables
providers backed by placeholder values and keeps every broker hidden on its
native email/password screen. A player therefore chooses Google or email exactly
once; the Keycloak continuation never repeats the provider menu.

No legacy credential fields or application password-recovery links are rendered
on the normal pages. OAuth2 Proxy completes the authorization-code flow and
keeps its encrypted session out of frontend JavaScript. It forwards only
configured allow-listed provider aliases, `prompt=create`, and a validated
email-shaped `login_hint`. The backend independently verifies the Keycloak token
forwarded to the exact OIDC session-exchange route, resolves the application
account, and then issues the normal GeoGuessMe access/refresh session described
above.

`user_identities` stores the durable `(issuer, subject) -> users.id` mapping.
The existing `users.id` always remains canonical, so linking a Keycloak identity
does not copy or replace memberships, scores, guesses, messages, or media.

The first verified OIDC session resolves as follows:

1. An existing issuer/subject mapping signs in its canonical user.
2. An exact verified recovery-email match links to that existing user while
   preserving its ID.
3. A pending or unverified email match returns `account_link_required`. The
   callback reveals the dedicated migration route. The player uses the old
   username/password there once and starts the link from Settings; an email
   claim alone never controls an account.
4. With no match, the callback returns `username_required`. The verified player
   explicitly chooses an available GeoGuessMe username; no provider username is
   prefilled or silently suffixed. Native email and social signup then create
   the new application user and identity in one transaction. Any password
   belongs only to Keycloak.

An unmigrated legacy session can read its groups and history, start the OIDC
link, request recovery mail, or delete the account. Every other write is
rejected by the backend with HTTP 403 and `migration_required`. Linking adds the
Keycloak subject to the same `users.id`, revokes old sessions, and restores full
access through the newly issued Keycloak-backed session. The application legacy
password is rejected after linking while OIDC is enabled; native Keycloak
email/password remains available. Setting `OIDC_ENABLED=false` restores the
retained legacy hash for rollback without removing the identity mapping.

Keycloak offers TOTP, recovery codes, and passkeys from account settings, but
none is a default action. MFA remains opt-in for this social game. The staged
release and read-only policy are owned by the
[social-auth rollout runbook](runbooks/social-auth-rollout.md).

## Verification

- Recovery email is optional and never controls account, gameplay, or social
  authorization. Before verification it exists only as `pending_email`; only a
  confirmed claim is promoted to the verified `email` used by recovery.
- Signup never checks whether another account verified the submitted address: it
  accepts the address as a pending claim, and verification later resolves
  ownership with a generic failure if the address is already claimed.
- `POST /api/v1/auth/verify/request` (authenticated) sends a verification email.
- `POST /api/v1/auth/verify {token}` consumes a single-use opaque token.
- Token TTL: `VERIFICATION_TOKEN_TTL` (default 24 hours).
- Tokens are stored hashed (SHA-256) and bound to the exact normalized pending
  address for which they were issued. Changing the pending claim cannot
  repurpose an older link.
- Before sending, any existing unverified token for the user is deleted; a
  database constraint guarantees concurrent requests cannot leave multiple
  unused verification tokens.

Token URL format: `{PUBLIC_URL}/verify-email?token={raw}`.

## Legacy password reset

Normal email/password recovery is owned by Keycloak. The endpoints below are
retained only for the OIDC-off compatibility UI and the hidden legacy migration
support path during this rollout.

- `POST /api/v1/auth/password/forgot {email}` sends a reset link (always returns
  202 to prevent email enumeration).
- `POST /api/v1/auth/password/reset {token, password}` atomically consumes the
  token, updates the password hash, bumps `auth_version`, and revokes all
  refresh sessions.
- Token TTL: `RESET_TOKEN_TTL` (default 1 hour).

Fully migrated users can update their username, pending recovery-email claim, or
selected profile avatar through `PATCH /api/v1/auth/profile`; their Keycloak
session authorizes the change. A verified recovery address remains active until
its replacement is verified, and omitting email cancels only the pending claim.
A custom profile photo is uploaded separately through
`POST /api/v1/auth/profile/avatar`; the web client sends the original selected
file, and the backend accepts JPG, PNG, or WebP up to 25 MiB before resizing and
stripping metadata. The legacy password-change route is unavailable after
Keycloak linking.

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

`DELETE /api/v1/auth/account` is authenticated and requires the current password
when legacy authentication is active. An OIDC-linked account instead requires
exact username confirmation while OIDC is enabled.

1. Verifies the password.
2. For an OIDC-linked user, obtains a short-lived Keycloak service-account token
   and deletes the exact stored issuer/subject first. If upstream deletion
   fails, the request returns 502 and no GeoGuessMe row is removed.
3. Calls `DeleteUserCascade` which removes:
    - All owned media (queues durable deletion jobs for S3 objects)
    - All refresh sessions, verification tokens, password-reset tokens,
      WebSocket tickets
    - Cascade-deletes memberships, messages, guesses, challenge_views
4. Deletes the user row entirely (not a soft-delete), releasing the username and
   email for reuse.

The frontend also clears the OAuth2 Proxy cookie after success, so a stale
Keycloak session cannot silently recreate the just-deleted application account.

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
| Email enumeration             | Signup never checks ownership; forgot password always returns 202  |
| Stale leak via logs           | Backend never logs tokens, signed URLs, passwords, or coordinates  |
