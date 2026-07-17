# Architecture

## Overview

GeoGuessMe is a client-server web application with a Go HTTP/WebSocket backend
and a React/TypeScript/Vite frontend served by Caddy. All media is stored in a
private S3-compatible bucket and proxied through the authenticated backend —
browsers never see S3 URLs directly.

## Components

```text
┌──────────────┐       ┌──────────────┐
│   Browser    │◄─────►│    Caddy     │  Production gateway (same-origin)
│  (React SPA) │       │  :80/:443    │
└──────────────┘       └──────┬───────┘
                              │ /api/*, /api/v1/ws
                              ▼
                      ┌──────────────┐
                      │   Backend    │  Go binary, serves /api/v1/*
                      │  :8080       │  Migrations via `geoguessme migrate`
                      └──┬───┬───┬───┘
                         │   │   │
              ┌──────────┘   │   └──────────┐
              ▼              ▼               ▼
      ┌────────────┐ ┌────────────┐ ┌──────────────┐
      │ PostgreSQL │ │  S3/MinIO  │ │    SMTP      │
      │  :5432     │ │  :9000     │ │  :1025/587   │
      └────────────┘ └────────────┘ └──────────────┘
```

## Trust boundaries

1. **Browser ↔ Caddy**: TLS-terminated (or plain HTTP in dev). Caddy adds
   security headers and a Content Security Policy. The SPA is served from `/`
   and the API is reverse-proxied from `/api/*`.

2. **Caddy ↔ Backend**: Loopback (same Compose network). The backend trusts the
   gateway only when `TRUSTED_PROXY_CIDRS` is configured.

3. **Backend ↔ PostgreSQL**: Configurable connection string (`DATABASE_URL`).
   SSL mode can be required in production.

4. **Backend ↔ S3**: Private HTTPS endpoint. Object keys never reach browsers.

5. **Backend ↔ SMTP**: TLS modes `off`, `starttls`, or `tls`. Production
   requires `starttls` or `tls`.

## Request flows

### HTTP API

All public endpoints are under `/api/v1`. Middleware stack (outer to inner):

- `RequestID` — assigns or propagates `X-Request-ID`
- `Recover` — catches panics, logs, returns 500
- `RequestLog` — logs each request with status, method, path, duration
- `CORS` — checks `Origin` against `ALLOWED_ORIGINS`
- `SecurityHeaders` — sets `X-Frame-Options`, `CSP`, `Referrer-Policy`
- `RateLimit` (select auth routes) — per-identity rate limiting
- `AuthMiddleware` (protected routes) — validates Bearer token, checks
  `auth_version` and account activity against the database

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
| Users, groups, photos, guesses, messages | PostgreSQL                   | Relational, embedded migrations       |
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
