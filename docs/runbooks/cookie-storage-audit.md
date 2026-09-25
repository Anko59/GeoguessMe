---
status: primary
---

# Cookie and client-storage audit runbook

GeoGuessMe does not use analytics, advertising, or tracking technologies, so no
cookie-consent banner is required today: every cookie is strictly necessary for
the service the user requested. This runbook is the recurring audit that keeps
that statement true before each release and at least quarterly. It implements
review item 6 of the legal-gap review.

## When to run

- Before each production release promotion.
- After adding or changing any frontend dependency, PWA behavior, edge
  configuration (Cloudflare, OAuth2 Proxy), or authentication flow.
- At least quarterly.

## Audit steps

### 1. Enumerate cookies in production

In a clean browser profile, sign in at geoguessme.com and list every cookie for
the origin and its parent domain (browser DevTools → Application → Cookies).
Repeat for the Keycloak realm host when social or hosted sign-in is enabled. For
each cookie record: name, purpose, set-by, expiry, HttpOnly, Secure, SameSite.

Expected set (no others are acceptable without a policy review):

| Cookie                                         | Set by       | Purpose                 | Notes                           |
| ---------------------------------------------- | ------------ | ----------------------- | ------------------------------- |
| `refresh_token`                                | Backend      | Session rotation        | HttpOnly, path-scoped, SameSite |
| OAuth2 Proxy session                           | oauth2-proxy | Hosted OIDC session     | HttpOnly, SameSite              |
| Keycloak `AUTH_SESSION_ID`, `KC_RESTART`       | Keycloak     | Brokered sign-in flow   | Realm host only                 |
| Cloudflare cookies (`__cf_bm`, `cf_clearance`) | Cloudflare   | Bot protection / Access | Edge-managed, necessary         |

### 2. Enumerate client storage

- Local storage: PWA session cache keys (`geoguessme_*`), latest-200 message
  hints; verify `clearCachedSession` on logout removes them.
- Session storage: invite-token and OIDC return-to keys; verify they are
  consumed or cleared after the flow.
- IndexedDB / caches: PWA runtime caches; verify no personal data beyond the
  documented message hints and no media blobs persist.

### 3. Verify no third-party trackers

- DevTools → Network: confirm requests go only to geoguessme.com, the configured
  API origin, Keycloak host, Cloudflare, OpenStreetMap tiles, and the browser
  push endpoint.
- Confirm the built bundle contains no analytics/ads SDKs: run
  `make build-frontend` and search `frontend/dist/assets` for known tracker
  domains.
- Re-run `make deps-npm-security-update` cadence checks so a transitive tracker
  cannot slip in unnoticed.

### 4. Decide on consent UI

If any non-essential cookie or tracker appears (analytics, ads, A/B testing,
third-party embedded content), stop the release that introduces it. Consent
requires a prior-opt-in banner with equal ease of acceptance and refusal, a
consent log, and a policy update — plan it as its own PR before launch.

### 5. Record evidence

File the audit result (date, revision, cookie table, storage table, tracker
search result) in the operator's compliance log. Findings that contradict
[PRIVACY.md](../../PRIVACY.md) are release blockers.
