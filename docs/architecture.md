# Architecture

## Overview

GeoGuessMe is a client-server web application with a Go HTTP/WebSocket backend
and a React/TypeScript/Vite frontend served by Caddy. All media is stored in a
private S3-compatible bucket and proxied through the authenticated backend —
browsers never see S3 URLs directly. Backend wiring is centralized in the
composition root `backend/app.go`: every handler reaches its dependencies
through the `App` type, and `main.go` owns process lifecycle.

## Components

```text
┌──────────────┐       ┌──────────────┐
│   Browser    │◄─────►│    Caddy     │  Same-origin game gateway
│  (React SPA) │       │  :80/:443    │
└──────────────┘       └───┬──────┬───┘
                           │      │ /oauth2/* + exact OIDC exchange
                    /api/* │      ▼
                           │  ┌──────────────┐       ┌──────────────┐
                           │  │ OAuth2 Proxy │◄─────►│   Keycloak   │◄── Google
                           │  │    (BFF)     │       │ auth.* realm │   (optional)
                           │  └──────┬───────┘       └──────┬───────┘
                           ▼         │                      ▼
                     ┌──────────────┐│              ┌──────────────┐
                     │   Backend    │◄┘              │ Keycloak DB  │
                     │  :8080       │── verifies ───►│ PostgreSQL   │
                     └──┬───┬───┬───┘ issuer/JWKS    └──────────────┘
                        │   │   │
             ┌──────────┘   │   └──────────┐
             ▼              ▼               ▼
     ┌────────────┐ ┌────────────┐ ┌──────────────┐
     │ App DB     │ │  S3/MinIO  │ │    SMTP      │
     │ PostgreSQL │ │  :9000     │ │  :1025/587   │
     └────────────┘ └────────────┘ └──────────────┘
```

## Trust boundaries

1. **Browser ↔ Caddy**: TLS-terminated (or plain HTTP in dev). Caddy adds
   security headers and a Content Security Policy. The SPA is served from `/`
   and the API is reverse-proxied from `/api/*`.

2. **Caddy ↔ Backend**: Loopback (same Compose network). The backend trusts the
   gateway only when `TRUSTED_PROXY_CIDRS` is configured.

3. **Caddy ↔ OAuth2 Proxy ↔ Keycloak**: Caddy sends only `/oauth2/*` and the
   exact OIDC session-exchange route through the BFF. OAuth2 Proxy owns the
   encrypted provider session and forwards its Keycloak token only to that
   backend exchange; browser JavaScript never receives it.

4. **Backend ↔ Keycloak**: The backend independently validates issuer,
   signature, expiry, and application audience. Keycloak has a separate
   PostgreSQL database and lifecycle from both game environments.

5. **Backend ↔ PostgreSQL**: Configurable connection string (`DATABASE_URL`).
   SSL mode can be required in production.

6. **Backend ↔ S3**: Private HTTPS endpoint. Object keys never reach browsers.

7. **Backend ↔ SMTP**: TLS modes `off`, `starttls`, or `tls`. Production
   requires `starttls` or `tls`.

## Request flows

### HTTP API

All public endpoints are under `/api/v1`. The middleware chain is applied
outermost to innermost in `App.routes()` (`backend/app.go`):

- `SecurityHeaders` — sets `X-Frame-Options`, `CSP`, `Referrer-Policy`
- `CORS` — checks `Origin` against `ALLOWED_ORIGINS`
- `RequestLog` — logs each request with status, method, path, duration
- `Recover` — catches panics, logs, returns 500
- `RequestID` — assigns or propagates `X-Request-ID`

At registration time, selected handlers are additionally wrapped by `RateLimit`
(per-identity rate limiting on auth routes) and `AuthMiddleware` (protected
routes — validates the Bearer token, then checks account activity and
`auth_version` against the database).

### Keycloak login, signup, and migration

Normal login and signup both redirect through OAuth2 Proxy to the shared
Keycloak realm, where native email/password or optional Google can authenticate
the player. Apple and GitHub are deferred. The exact callback exchange resolves
the durable `(issuer, subject)` mapping and issues the application's existing
access and refresh session. A first Keycloak identity either reuses an exact
verified-email user or atomically creates a passwordless app user.

If only a pending/unverified legacy claim matches, the callback exposes the
otherwise hidden migration route. That legacy password session is backend-
enforced read-only: GET/HEAD, account deletion/recovery, and starting the OIDC
link remain available; other writes return `403 migration_required`. Linking
adds the identity to the same `users.id`, bumps `auth_version`, revokes old
sessions/tickets, and restores normal writes through the new Keycloak-backed
session.

### WebSocket chat

1. Client requests a one-time ticket: `POST /api/v1/ws/ticket?group_id=...`
2. Server returns an opaque 32-byte token (60-second TTL).
3. Client upgrades via `GET /api/v1/ws?group_id=...&ticket=...`
4. Origin is checked **before** the ticket is consumed (one-time tickets cannot
   be burned by a prohibited origin).
5. Messages are persisted, broadcast to all group members, and include `id`,
   `group_id`, `user_id`, `username`, `avatar`, `kind`, `photo_id?`, `content`,
   `created_at`.

### Media access

Media is stored with private S3 keys under `photos/<uuid>`. The backend streams
it through authenticated endpoints:

- `GET /api/v1/challenges/{photoID}/media` — view window only
- `GET /api/v1/challenges/{photoID}/media?result=1` — after results visible

The frontend fetches media as an authenticated blob and renders it via a
short-lived `blob:` object URL.

## Storage

| Data                                     | Store                        | Notes                                 |
| ---------------------------------------- | ---------------------------- | ------------------------------------- |
| Users, groups, photos, guesses, messages | App PostgreSQL               | Relational, embedded migrations       |
| App identity mappings                    | App PostgreSQL               | Durable issuer/subject → `users.id`   |
| Keycloak users, clients, provider config | Separate Keycloak PostgreSQL | Shared realm, independent backups     |
| Media images                             | S3-compatible (MinIO in dev) | Private bucket, no public URLs        |
| Refresh sessions, tokens                 | PostgreSQL                   | Hashed tokens, one-time use           |
| Media deletion queue                     | PostgreSQL                   | Durable jobs for async object removal |

## Email

The backend sends verification and password-reset emails via SMTP. SMTP can be
disabled (`off`), use `starttls`, or use implicit `tls`. Delivery is optional;
account creation and gameplay do not depend on SMTP availability. Development
uses [Mailpit](http://localhost:8025).

## Cleanup worker

A background goroutine runs immediately on startup and then every hour:

1. **Token cleanup**: deletes expired refresh sessions, verification tokens,
   password-reset tokens, and WebSocket tickets.
2. **Challenge-view expiry**: removes expired challenge_views rows older than
   one day.
3. **Retention sweep**: finds photos past their `retention_at`, marks them
   `removed`, and enqueues durable deletion jobs.
4. **Deletion queue drain**: claims up to 25 pending deletion jobs (with a
   15-minute back-off), deletes the object from S3, and marks jobs complete.
   Failures are logged and retried.

The backlog of pending deletion jobs is exposed via the
`geoguessme_storage_cleanup_backlog` metric.
