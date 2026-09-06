---
status: primary
---

# Social-auth rollout with existing-account continuity

This is the canonical rollout plan for introducing Keycloak at
`https://auth.geoguessme.com` without replacing GeoGuessMe's existing user
records. Production currently has a small, known legacy population (about 11
players), but every step treats the mapping as durable production data.

## Invariants

- `users.id` remains the application identity before, during, and after
  migration.
- A Keycloak identity adds a `user_identities` row; it never creates a second
  user for a known legacy player.
- Groups, memberships, scores, guesses, messages, media, and timestamps are not
  copied or rewritten during account linking.
- Only a verified Keycloak email may auto-link an existing verified recovery
  email. Pending/unverified matches require a legacy-authenticated link intent.
- The `(issuer, subject)` pair is authoritative after linking. Email is only a
  bootstrap/linking signal and may change later.
- Existing application username-or-email/password login remains visible and
  fully functional before and after Keycloak is enabled or linked.
- Linking Keycloak is optional. Native Keycloak email/password and Google are
  additional methods; Apple and GitHub are outside this rollout.

## Phase 0: inventory and dark deployment

1. Take and verify an application-database backup and a separate Keycloak
   database backup.
2. Record every current user ID and ownership count before applying the additive
   identity migration. Keep this export with restricted release evidence.
3. Apply migration `023_oidc_identities.sql` with `OIDC_ENABLED=false`.
4. Deploy Keycloak, OAuth2 Proxy, and the compatible application build. Confirm
   that legacy signup, login, and gameplay are unchanged while OIDC is off.
5. Configure the production and dev clients separately. Production callbacks and
   origins use `https://geoguessme.com`; the issuer uses
   `https://auth.geoguessme.com/realms/geoguessme`.
6. Before deploying this configuration over an existing hosted installation,
   update both encrypted application environments without rotating their stable
   database, JWT, cookie, backup, or Web Push secrets. Preserve the existing
   Keycloak application-client secret, rename its key from
   `OAUTH2_PROXY_CLIENT_SECRET` to `OIDC_CLIENT_SECRET`, and remove the old
   `OAUTH2_PROXY_CLIENT_ID` and `OAUTH2_PROXY_OIDC_ISSUER_URL` duplicates. The
   shared encrypted identity environment is generated separately as documented
   in the hosted-deployment runbook.

Suggested inventory queries (do not publish email or subject values in release
logs):

```sql
SELECT id, username, created_at
FROM users
WHERE deleted_at IS NULL
ORDER BY created_at, id;

SELECT u.id,
       u.username,
       COUNT(DISTINCT gm.group_id) AS group_count,
       COUNT(DISTINCT g.id) AS guess_count,
       COUNT(DISTINCT p.id) AS photo_count
FROM users AS u
LEFT JOIN group_members AS gm ON gm.user_id = u.id
LEFT JOIN guesses AS g ON g.user_id = u.id
LEFT JOIN photos AS p ON p.user_id = u.id
WHERE u.deleted_at IS NULL
GROUP BY u.id, u.username
ORDER BY u.created_at, u.id;
```

If the deployed schema uses a renamed membership table, adapt only that
read-only evidence query; never change production schema to fit the example.

Run the aggregate application inventory after migration 023:

```bash
make prod-legacy-identity-plan
```

The output contains counts only: total legacy accounts, already linked,
verified-email candidates, pending email, and missing email. It must total the
known production population without printing addresses.

## Phase 1: optional pre-provisioning

Stock Keycloak does not support the application's bcrypt hash format. Never copy
hashes into Keycloak or write its database directly. Pre-provisioning verified
legacy emails is optional and must not be required for rollout: users who ignore
an invitation continue signing in with their existing username or email and
application password with full access.

If operators choose to invite verified-email users to add Keycloak credentials,
use `make prod-legacy-identity-provision CONFIRM=provision`. The command is
idempotent and does not insert `user_identities`; linking occurs only after the
player authenticates. Pending-email and missing-email accounts are skipped.

## Phase 2: coexistence launch

Enable OIDC only after the local and dev flows are green. During this phase:

- Normal login visibly offers the existing username-or-email/password form,
  optional Google, and native Keycloak email/password.
- Existing password accounts have full read and write access. They are not
  required to link, verify an email, reset a password, or take any rollout
  action.
- A brand-new verified native or social identity creates a new GeoGuessMe user
  and its identity mapping atomically.
- An exact verified recovery-email match may link automatically. A pending or
  unverified match returns `account_link_required`; the player signs in to the
  existing account normally and may connect Google from Settings.
- Linking preserves the same `users.id`, history, and application password. It
  revokes old sessions and WebSocket tickets, then issues a fresh session.
- Apple and GitHub stay disabled. TOTP, recovery codes, passkeys, and linking
  remain optional.

Before enabling OIDC, require regression coverage proving username and email
password login, full write access for unlinked users, password login after
linking, identity-conflict handling, and the OIDC-disable rollback. Compare the
post-launch user IDs and ownership counts with the Phase 0 export; any changed
ID or missing history blocks the rollout.

## Legacy-authentication retention

Do not schedule removal of existing password login as part of the social-login
rollout. The password hash, reset/change-password endpoints, normal login UI,
and full-access backend behavior are supported compatibility features. Any
future retirement would require a separate product decision, explicit user
communication and consent, complete adoption evidence, a rollback plan, and a
separately reviewed release. Merely linking Google is not consent to disable the
existing login method.

## Provider registration and callback contract

Provider credentials are not interchangeable with the GeoGuessMe application
client secret. Create one Google credential for the hosted Keycloak broker and
register this exact HTTPS value:

| Provider | Provider-side identifier | Callback / return URL                                                  |
| -------- | ------------------------ | ---------------------------------------------------------------------- |
| Google   | OAuth 2.0 web client     | `https://auth.geoguessme.com/realms/geoguessme/broker/google/endpoint` |

For Google, also configure the consent screen and use the web client ID and
secret. The callback URL must include the broker alias exactly as shown. Apple
and GitHub remain disabled even if credentials are accidentally present; enable
them only in a separately reviewed provider rollout.

Put those values in the shared encrypted identity environment, run
`make identity-up` so the realm reconciler applies configuration to an existing
realm, and then test the Google button in hosted dev. A correct provider
callback returns first to `auth.geoguessme.com`; Keycloak then returns to
`https://dev.geoguessme.com/oauth2/callback` or
`https://geoguessme.com/oauth2/callback` through its separate application
client. Do not register either application callback in a social-provider
console.

The local stack intentionally uses placeholder provider credentials. Keycloak
keeps those brokers disabled and the application omits their buttons instead of
sending a player into an `invalid_client` response. Google can be activated
locally with dedicated credentials and the HTTPS callback
`https://auth-dev.geoguessme.com/realms/geoguessme/broker/google/endpoint`.
Native email/password, verification mail, callback exchange, and the branded
Keycloak pages run entirely locally.

An expired Keycloak page after returning from a provider means its one-time
browser state is stale or was already consumed. Restart from the corresponding
GeoGuessMe provider button; never bookmark or retry a broker callback URL.

## Account deletion with a social provider

Deleting a linked GeoGuessMe account deletes the Keycloak user first, including
its Google federation link and Keycloak sessions, and only then deletes the
application row and gameplay data. If Keycloak cannot confirm deletion, the
operation fails closed and retains the application data for a safe retry.

The upstream Google account and its provider-side authorization remain outside
GeoGuessMe's control. The player may therefore choose Google again later; that
must create a blank Keycloak and GeoGuessMe account, require a new GeoGuessMe
username, and never restore the deleted ID, profile, groups, scores, or history.
A permanent provider-identity denylist would retain a tombstone after account
deletion and is deliberately not part of this rollout.

The local Keycloak E2E test verifies that the deleted user returns `404` from
the Keycloak Admin API and that the old credentials no longer authenticate.
Because an automated suite must not store a personal Google session, the hosted
dev release check must additionally delete a disposable Google-created player,
start Google sign-in again, and confirm that empty username onboarding appears
with none of the deleted account's data.

## Identity-conflict handling

Never merge accounts automatically when the social subject already belongs to
another user or when only an unverified/pending email matches. Preserve both
rows, return the generic linking error to the player, and investigate using
restricted server-side IDs. Any manual correction requires a backup, explicit
confirmation of both account owners, and before/after ownership-count evidence.

## Rollback

For an identity incident, roll back the application configuration to the dark
deployment by setting `OIDC_ENABLED=false` and removing OAuth2 Proxy from the
public route. Keep issuer/client settings available. That rollback restores the
OIDC-off UI while the same legacy-password access remains available for linked
and unlinked accounts, without unlinking anyone. Accounts created only in
Keycloak after launch require Keycloak to return. Account deletion still
attempts upstream-first Keycloak deletion; if Keycloak is unavailable it fails
closed and keeps local data. Do not roll back migration 023 or delete identity
mappings. Restore a database backup only for demonstrated data corruption;
ordinary identity or provider outages require an application or configuration
rollback, not a schema rollback.

## Completion evidence

The release record must include:

- backup identifiers and successful restore-check evidence;
- the pre-rollout user-ID/ownership snapshot and post-launch comparison;
- counts of active and optionally linked users (without emails/subjects);
- desktop and mobile screenshots of login, signup, Keycloak providers,
  authenticated groups, and optional security settings;
- the tested OIDC-disable rollback result;
- username/email password-login tests before and after optional linking, plus a
  full-write-access test for an unlinked account.
