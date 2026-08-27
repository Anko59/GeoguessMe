---
status: primary
---

# Social-auth rollout and legacy-account migration

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
- The read-only migration policy is enforced by the backend, not only by hidden
  frontend controls.
- Native email/password remains fully available through Keycloak. Google is
  optional; Apple and GitHub are outside this rollout.

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

## Phase 1: pre-provision verified legacy accounts

Stock Keycloak 26.7.1 does not support the application's bcrypt hash format. Do
not copy hashes into the Keycloak database and never write that database
directly. After backups and the aggregate plan are reviewed, create only the
eligible Keycloak users and send each one a Keycloak `UPDATE_PASSWORD` action:

```bash
make prod-legacy-identity-provision CONFIRM=provision
```

The command is idempotent. It creates a passwordless Keycloak user only for an
active, unlinked application account whose recovery email was already verified
by GeoGuessMe. It marks that address verified in Keycloak, requires a password
choice, and sends the action email. A retry reuses the exact verified Keycloak
user and resends only while `UPDATE_PASSWORD` is still outstanding. After the
password action, Keycloak returns the player to the GeoGuessMe login page.

Pre-provisioning does not insert `user_identities` and does not unlock the old
application session. The first successful Keycloak login supplies the verified
email, atomically links it to the existing `users.id`, revokes old sessions, and
restores writes. This prevents an operator-side partial failure from binding an
account before its owner authenticates.

Pending-email and missing-email accounts are deliberately skipped. They use the
hidden legacy-authenticated linking path in Phase 2; an unverified claim is
never enough to control an account.

## Phase 2: Keycloak launch and read-only legacy accounts

Enable OIDC only after the local and dev flows are green. During this phase:

- Normal login and signup use Keycloak exclusively. Both pages offer optional
  Google plus native Keycloak email/password. The application does not display
  legacy credential forms, and passwords are entered only on the branded
  Keycloak origin.
- A brand-new verified native or social identity creates a new GeoGuessMe user
  and its identity mapping atomically.
- Apple and GitHub are disabled in Keycloak and absent from the application UI.
- An exact verified recovery-email match may link automatically. A pending or
  unverified match returns `account_link_required`, which is the only normal UI
  path that reveals `/migrate-account`.
- The migration page accepts old credentials once and issues a read-only
  session. Settings then starts the explicit Keycloak link.
- Linking preserves the same `users.id`. The account remains fully usable and
  all old refresh sessions and WebSocket tickets are revoked. Password login is
  rejected for the linked account.
- TOTP, recovery codes, and passkeys are offered in Keycloak account settings
  but are never mandatory.

At this point every unmigrated legacy account becomes read-only under the matrix
below. It is not silently converted, and its password remains available only
through the hidden migration route.

Monitor migration progress without exposing identity subjects:

```sql
SELECT u.id,
       u.username,
       EXISTS (
           SELECT 1
           FROM user_identities AS ui
           WHERE ui.user_id = u.id
       ) AS migrated
FROM users AS u
WHERE u.deleted_at IS NULL
ORDER BY u.created_at, u.id;
```

For each of the known legacy players, compare the post-link ID and ownership
counts with the Phase 0 export. Any changed ID or missing history blocks the
rollout.

## Migration completion during Phase 2

Each existing player must connect a Keycloak email/password or Google identity
before normal write access is restored. Contact the approximately 11 known
players directly, keep the migration support path available, and track only
aggregate migrated/unmigrated counts in routine operations. A player may still
read existing history before migrating; do not delete, duplicate, or reassign
that player's application row.

Close the phase only after every active legacy player is linked or has been
handled through the documented support process, and after IDs and ownership
counts match the Phase 0 evidence. Retiring legacy password material is a later,
separately reviewed cleanup after the rollback observation window; it is not
part of this rollout.

## Phase 3: later PR to retire legacy authentication

This repository intentionally retains the hidden legacy login, bcrypt hashes,
reset endpoints, `legacy_password_enabled`, and the read-only migration support
path in the current PR. Remove them only in a second, separately reviewed PR
after all active accounts are linked, the support queue is empty, ID/ownership
evidence matches Phase 0, and the rollback observation window has closed.

That later PR must proceed in this order:

1. take fresh application and Keycloak backups and archive aggregate completion
   evidence;
2. remove the hidden migration UI/routes and legacy reset/change-password code;
3. deploy and observe the Keycloak-only application while the schema remains
   backward compatible;
4. in a subsequent forward-only migration, clear/drop password material and
   legacy-only tables or columns;
5. update the rollback plan because revisions older than that cleanup will then
   require the pre-cleanup backup.

Do not combine steps 2 and 4 in one deployment. The compatible application must
be proven before schema cleanup makes an old binary unusable.

### Read-only legacy-session matrix

With OIDC enabled, a legacy account with no `user_identities` row may inspect
existing history, but it cannot change gameplay state until it completes the
Settings linking flow. This is server-enforced in the same release as the
Keycloak-only login/signup UI. The migration attaches Keycloak to that same user
row; after successful linking, normal write access is restored immediately.

The backend policy must allow:

- authentication, refresh, logout, and account recovery;
- GET/HEAD access to the player's existing groups, history, messages, results,
  rankings, and settings;
- starting and completing the OIDC link flow;
- account deletion and any legally required privacy operation.

It must reject other state-changing operations for an unmigrated legacy user,
including profile/avatar edits, group create/join/leave, invite management, chat
messages/reactions, media or challenge creation, challenge acceptance and
guesses, and push-subscription changes. Use one stable API error:
`migration_required`, HTTP 403) so the frontend can show a migration banner and
link directly to Settings.

Do not delete legacy password hashes or identity columns in this phase.
Passwords remain rollback material until the observation window has closed and a
later, separately reviewed cleanup is approved.

Before enabling OIDC, require all of the following:

- a server-side write-policy implementation with route-level tests proving the
  read-only matrix and its linking/privacy exceptions;
- UI coverage for the banner, Settings migration action, successful unlock,
  expired link intent, and identity conflict;
- an operator-visible count of migrated and unmigrated active users;
- direct notice to the small known legacy population and a tested support path;
- a fresh backup and a rehearsed rollback that disables only the policy.

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
OIDC-off compatibility UI and retained legacy-password access for linked and
unlinked legacy accounts whose hash still exists, without unlinking anyone.
Accounts created only in Keycloak after launch require Keycloak to return.
Account deletion still attempts upstream-first Keycloak deletion; if Keycloak is
unavailable it fails closed and keeps local data. Do not roll back migration 023
or delete identity mappings. Restore a database backup only for demonstrated
data corruption; ordinary identity or provider outages require an application or
configuration rollback, not a schema rollback.

## Completion evidence

The release record must include:

- backup identifiers and successful restore-check evidence;
- the pre-rollout user-ID/ownership snapshot and post-link comparison;
- counts of active, migrated, and unmigrated users (without emails/subjects);
- desktop and mobile screenshots of login, signup, Keycloak providers,
  authenticated groups, and optional security settings;
- the tested OIDC-disable rollback result;
- the complete read-only route matrix and migration-unlock test.
