# PWA and Web Push

GeoGuessMe is a fully installable Progressive Web Application with end-to-end
Web Push notifications. No third-party push provider or native app wrapper is
required.

## Installable PWA

The app includes a [Web App Manifest](/manifest.webmanifest) at the root scope,
themed icons in `/icons/`, iOS `<meta>` tags, and a service worker (`/sw.js`)
registered on every page load.

### Install flow

- **Android / desktop Chromium**: a `beforeinstallprompt` banner appears when
  the user is authenticated, letting them install the app in one tap.
- **iOS Safari**: the app displays step-by-step **Add to Home Screen**
  instructions (Share → Add to Home Screen → Add). Safari does not support the
  `beforeinstallprompt` event, so the guidance is unconditional until dismissed
  or the app is installed.
- The banner is dismissible; the choice is stored in `localStorage`.

Once installed, the app launches in standalone mode with a navy theme and no
browser chrome.

### Service worker

The worker (`/sw.js`) serves a single purpose: receiving push events and showing
notifications. It does **not** cache the SPA, so asset freshness is unaffected.
`skipWaiting` + `clients.claim` keep push handling up to date without a manual
reload.

## Web Push

### Architecture

```text
Browser               Backend               Push Service
   │                     │                      │
   │ Subscribe (after     │                      │
   │  permission +        │                      │
   │  VAPID key fetch)    │                      │
   ├─────────────────────►│                      │
   │  POST /push/subscribe│                      │
   │                     │ Stores subscription │
   │                     │ (p256dh + auth)     │
   │                     │                      │
   │                     │ Event (challenge or  │
   │                     │  chat message)       │
   │                     │                      │
   │                     │ Encrypt + POST       │
   │                     ├─────────────────────►│
   │  Show notification  │                      │
   │◄────────────────────┤                      │
   │                     │                      │
```

1. The browser fetches the VAPID public key and calls
   `PushManager.subscribe({applicationServerKey, userVisibleOnly: true})`.
2. The resulting `PushSubscription` JSON is POSTed to `/api/v1/push/subscribe`
   and persisted in PostgreSQL.
3. When a challenge or chat event fires, the backend resolves group members,
   fetches their subscriptions, encrypts each payload with the subscription's
   `p256dh` key and `auth` secret (RFC 8291 + RFC 8188 aes128gcm), and POSTs to
   the push service endpoint with a VAPID JWT (RFC 8292) authorization header.
4. The push service delivers the encrypted message; the service worker decrypts
   it (the browser handles this transparently) and fires `push` →
   `showNotification`.

### VAPID keys

Generate a stable keypair once per deployment:

```bash
make migrate-up  # start the stack first, then:
docker compose -f deployment/compose.prod.yaml exec backend geoguessme vapid-keys
```

Or run locally:

```bash
go run . vapid-keys
```

The output is three `env` lines (`VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, and a
suggested `VAPID_SUBJECT`). Set them in the production environment:

```bash
VAPID_PUBLIC_KEY=<base64url output>
VAPID_PRIVATE_KEY=<base64url output>
VAPID_SUBJECT=mailto:operator@example.com
```

In development and test environments the backend mints ephemeral keys at
startup. Browser subscriptions created under ephemeral keys are invalidated on
the next restart, so operators must set stable keys in any environment where
push delivery matters.

### Endpoints

| Method   | Path                            | Auth     | Description                                                                                   |
| -------- | ------------------------------- | -------- | --------------------------------------------------------------------------------------------- |
| `GET`    | `/api/v1/push/vapid-public-key` | required | Returns `{"public_key":"<base64url>"}` so the frontend can subscribe without a hardcoded key. |
| `POST`   | `/api/v1/push/subscribe`        | required | Stores `{endpoint, keys: {p256dh, auth}}`. Re-Upserts on matching `user_id` + `endpoint`.     |
| `DELETE` | `/api/v1/push/unsubscribe`      | required | Removes the subscription for the given endpoint, or all of them when `endpoint` is omitted.   |

### Notification triggers

- **New challenge**: when a group member uploads a photo, other members receive
  "_Username_ posted a new challenge in _Group_".
- **New chat message**: when someone sends a chat message, group members receive
  "_Username_: _message content_" (truncated to 140 characters).

Notifications are best-effort: if the push service is unreachable or the buffer
is full, the message is dropped and logged. Permanently invalid subscriptions
(endpoint 404/410) are removed from the database and never retried.

### Subscription lifecycle

- **Permission**: the browser requests notification permission on the first
  subscription. If denied, the enable button shows "Blocked in settings".
- **Expiration / rotation**: the browser can silently replace a subscriber's
  `p256dh` key. The service worker reports `pushsubscriptionchange`, and every
  authenticated page load syncs the active subscription via
  `GET /push/vapid-public-key` + `POST /push/subscribe`.
- **Unsubscribe**: the app calls `PushSubscription.unsubscribe()` locally and
  DELETEs the endpoint from the backend.
- **Stale cleanup**: the first failed delivery to a dead endpoint removes it
  from the database, so no further attempts are made.

## iOS-specific notes

- iOS 16.4+ supports Web Push, but **only from an installed PWA**. The app shows
  clear Add to Home Screen instructions on iOS Safari.
- The `apple-mobile-web-app-capable` and `apple-touch-icon` meta tags provide
  the native-like launch experience.
- The status bar uses `black-translucent` to blend with the navy theme.
