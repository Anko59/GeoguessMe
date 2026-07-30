# Troubleshooting

## PostgreSQL volume version mismatch

**Symptom**: Containers fail to start after switching PostgreSQL images.
Database volume created by `postgres:15-alpine` and then started with
`postgres:16-alpine` (or vice versa) will produce a version mismatch error.

**Solution**: Dump and restore.

Use the Dockerized `make db-backup` target before changing the image, provision
the new development volume with `make dev`, then use `make db-restore FILE=...`
and `make migrate-status` to verify the restored schema.

## MinIO

**Symptom**: Uploads fail with `storage_error`.

- Check MinIO is healthy: `make status`
- Verify credentials match between Compose env and backend config
- On a freshly created bucket, the backend calls `EnsureBucket` automatically

**Symptom**: Media requests return 404.

- Verify the object key in `photos.storage_key` exists in the MinIO bucket
- Check `media_deletion_jobs` table — the object may have been cleaned up

## Mailpit

**Symptom**: Verification/reset emails not received.

- Open Mailpit UI: <http://localhost:8025>
- Check backend logs: `make logs-backend` — verify SMTP connection succeeded
- In development, `SMTP_TLS=off` and `SMTP_HOST=mailpit` should work

## Cookies / CORS

**Symptom**: Refresh endpoint returns 401 despite being logged in.

- Check the `refresh_token` cookie is present (DevTools → Application → Cookies)
- The cookie path is `/api/v1/auth` — ensure the refresh request targets that
  exact prefix
- SameSite=Lax means the cookie is not sent on cross-origin requests from
  external sites
- In development without HTTPS, `Secure` is automatically `false`
- Check `ALLOWED_ORIGINS` includes the frontend origin

**Symptom**: CORS errors in browser console.

- Verify the `Origin` header is in `ALLOWED_ORIGINS`
- Check that CORS preflight (OPTIONS) returns 200

## Profile photo uploads

**Symptom**: A profile photo upload returns `Authentication required`, or the
page appears to do nothing.

- Protected profile routes such as `/api/v1/auth/profile/avatar` still require
  the bearer access token; only login, signup, verification, and password
  recovery routes are public.
- Upload through the shared API client so its request interceptor can restore a
  session after a PWA reload. Do not set `Content-Type: multipart/form-data`
  manually: the browser must add the multipart boundary.
- Avatar and group-photo uploads use the browser's native `image/*` file flow;
  the server validates the image bytes and resizes the photo. If the same file
  works for a group photo but not an avatar, check the response and backend logs
  rather than renaming or re-encoding the file.
- The account settings page displays the server's structured error message in an
  alert. Check the browser Network panel for the response status and the backend
  logs with `make logs-backend` if storage fails.

The authenticated browser regression is covered by the profile-photo scenario in
`frontend/e2e/auth.spec.ts`; run it with
`GEOGUESSME_E2E_SPEC=auth.spec.ts make test-e2e-pr`.

## WebSockets

**Symptom**: WebSocket connection fails.

- Check the ticket was obtained from `POST /api/v1/ws/ticket?group_id=...`
  (authentication required)
- Verify the ticket is passed as a query parameter and is not expired
- Origin is checked BEFORE the ticket is consumed. Ensure `Origin` matches an
  allowed origin.
- Check WebSocket upgrade logs: `make logs-backend`

## Camera / geolocation

**Symptom**: Camera not available in the browser.

- HTTPS is required for camera access (except localhost)
- Check browser permissions for the site
- The frontend uses `navigator.mediaDevices.getUserMedia`

**Symptom**: Geolocation not available.

- HTTPS required (except localhost)
- Check browser permissions
- The app uses `navigator.geolocation.getCurrentPosition`

## Recorded video playback

Camera recordings and challenge media use browser-created `blob:` URLs before
and during playback. The production gateway must allow `blob:` in `media-src`;
allowing it only in `img-src` makes Chromium reject a valid WebM recording with
“No videos with supported format and MIME type found”. The camera preview now
reports a readable error if media playback still fails. Verify the complete path
with `GEOGUESSME_E2E_SPEC=challenge.spec.ts make test-e2e-pr`.

## Message actions

Reply and emoji actions are intentionally revealed on hover, keyboard focus,
hold, or horizontal swipe. Hidden actions use no layout space, so adjacent
messages keep the normal chat gap. The browser regression is covered by the chat
E2E suite.

## Group photos in the groups list

Group photos are private authenticated media. The group view and each card on
`/groups` resolve them through the same cached photo hook; the list must not
render the static group icon for a card. If a photo is unset or unavailable, the
hook falls back to `/logo.png`. Verify upload followed by a fresh groups
navigation with `GEOGUESSME_E2E_SPEC=groups.spec.ts make test-e2e-pr`.

## Migrations

**Symptom**: `make migrate-up` fails.

- Check `DATABASE_URL` is set correctly
- Verify the database exists and is reachable
- Check `schema_migrations` table for applied migrations
- Each migration runs in a transaction; fix the error and retry
- Advisory lock prevents concurrent runs — if a previous run was interrupted,
  the lock may still be held. Wait or terminate the stuck connection:

    Re-run `make migrate-up` after the interrupted transaction has released its
    session; the migration job owns the advisory-lock lifecycle.

## Playwright test failures

**Symptom**: E2E tests fail consistently.

- Make sure the test stack builds: `make test-e2e` (it tears down on failure)
- Run `make test-e2e-ui` for the Playwright UI mode with traces
- Check `frontend/test-results/` for screenshots
- Common issues: `PHOTO_VIEW_WINDOW` too short, timing-dependent race
  conditions, or missing test data
- The test stack uses `PHOTO_VIEW_WINDOW=1s` — if tests expect real-time timing,
  check assertions allow for clock skew

## Docker disk exhaustion during tests

**Symptom**: A local E2E run fails with `ENOSPC`, especially while writing a
trace, screenshot, or other diagnostic artifact.

- Check available space with `df -h /home` and inspect Docker usage with
  `docker system df`
- Remove only repository-generated artifacts and dangling BuildKit layers with
  `make artifacts-clean build-cache-prune`
- Rerun the focused Make target after cleanup; do not remove the named database
  or object-storage volumes unless the test data itself is intentionally being
  reset

The full local run is not a valid product result when the host reports `ENOSPC`;
record the infrastructure failure separately from application test failures.

## Rate limiting

**Symptom**: Requests return 429 with `Retry-After` header.

- Default rate limit is 10 requests per minute per identity
- The `Retry-After` value equals `RATE_LIMIT_WINDOW` in seconds
- Increase `RATE_LIMIT_REQUESTS` and `RATE_LIMIT_WINDOW` for testing
- Rate limiting is only applied to auth endpoints (signup, login, refresh,
  verify, forgot/reset password)
