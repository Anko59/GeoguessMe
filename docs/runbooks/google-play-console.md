---
status: primary
---

# Google Play developer account runbook

This is the canonical operating guide for the GeoGuessMe Google Play developer
account and Android distribution. The app identity is:

| Item                | Value                                                                                 |
| ------------------- | ------------------------------------------------------------------------------------- |
| App name            | GeoGuessMe                                                                            |
| Package name        | `com.geoguessme.app`                                                                  |
| Android project     | `frontend/android/`                                                                   |
| Privacy policy      | <https://geoguessme.com/privacy>                                                      |
| Account and app IDs | Read from the Play Console or API at run time; do not hardcode them in the repository |

The website and Android app share the same frontend, but a successful hosted web
deployment does not publish an Android artifact. Android distribution is a
separate release surface and must use the process below.

## Operating principle

The agent owns the ordinary technical work:

- inspect the current Play state and the app's package, version, signing, and
  policy evidence;
- build and test the Android App Bundle from the validated repository revision;
- use the Google Play Developer API to upload bundles, manage testing tracks and
  tester lists, update store listings, validate edits, and commit changes;
- use the Play Console through the authenticated browser only where the API has
  no supported operation, then verify the resulting state; and
- keep private release evidence and report the exact Play state after each
  mutation.

The agent does not stop for a routine **Save**, **Submit**, or API edit commit.
It performs the operation, verifies the resulting state, and continues through
the release checklist. A pause is justified only for a human-only boundary:
interactive Google sign-in or 2FA, a legal or financial attestation, a fact the
repository cannot establish (for example the developer's legal identity or
target audience), or people who must personally opt in as testers.

Never use the owner's personal password, recovery codes, payment information, or
2FA device as an automation credential. A Play Console reviewer account is also
separate from the service account used by the API.

## Access model

Google Cloud and Google Play are related but have different permissions:

1. A Google Cloud project hosts the **Google Play Developer API** enablement and
   the service account. The existing GeoGuessMe Cloud project may be used when
   it is the approved project for this integration; a second project is not
   needed merely because the Play app exists.
2. The service account must separately be invited in **Play Console → Users and
   permissions**. IAM Owner or Editor on the Cloud project does not grant it
   access to the Play developer account.
3. The service account's Play permissions are granted in Play Console, ideally
   at app scope. Grant only the permissions required for the current task. Start
   with read access and testing-track access; add production release,
   store-presence, or app-content permissions only when that work is needed.
4. Permission changes can take time to propagate. Re-read the account and app
   state after a change rather than retrying a failed mutation blindly.

The one-time account-owner setup is:

1. Select or create the approved Google Cloud project.
2. Enable the Google Play Developer API.
3. Create a dedicated service account whose name identifies this repository and
   purpose, for example `geoguessme-play-publisher`.
4. Invite its exact `...iam.gserviceaccount.com` email in Play Console. Select
   the GeoGuessMe app and grant the least-privilege permissions needed for the
   planned operations.
5. Verify access with a read-only app/track query before attempting an upload.
6. Record the service-account email and key ID, but never record the private key
   in GitHub issues, pull requests, release notes, or this repository.

The Google documentation describes the setup in
[Getting started with the Google Play Developer API](https://developers.google.com/android-publisher/getting_started)
and the available account permissions in
[Add developer account users and manage permissions](https://support.google.com/googleplay/android-developer/answer/9844686).

## Credential handling

Play API credentials are secrets. They must be kept outside the repository and
outside ordinary shell history:

- Local commands use the GNOME Keyring-backed agent environment. A future Play
  API client may receive `GOOGLE_APPLICATION_CREDENTIALS` pointing to a mode
  `0600` file outside the repository, or may materialize a short-lived file from
  the approved keyring/vault entry. The agent must use
  `direnv exec /home/anko/Work/projects/GeoguessMe <command>` for credentialed
  commands.
- CI should use a secret-managed file or workload identity. If a JSON key is
  unavoidable, inject it only for the job, restrict the GitHub environment, and
  delete the temporary file in cleanup. Do not put the JSON in a repository
  secret if the platform offers a safer federated identity path.
- The repository's **Play API access check** workflow uses the preferred
  federated path. Set these as non-secret variables on the GitHub `production`
  environment: `PLAY_GCP_WORKLOAD_IDENTITY_PROVIDER` (the full Google WIF
  provider resource) and `PLAY_GCP_SERVICE_ACCOUNT` (the exact service-account
  email invited in Play Console). The workflow exchanges GitHub's OIDC identity
  for a short-lived access token and passes it to the Dockerized client through
  `PLAY_ACCESS_TOKEN`.
- Do not commit service-account JSON, `google-services.json`, release keystores,
  passwords, access tokens, or generated AAB/APK files. Do not print them,
  include them in diagnostic artifacts, or pass them as command-line arguments.
- Keep a restricted recovery copy in the team's password manager. Rotate or
  revoke a key immediately if it may have been exposed, then verify a read-only
  API call before restoring write access.

If the credential is not available, the agent must first inspect the approved
keyring/vault path and the Play Console service-account invitation. It must not
ask the user to upload a secret into chat or silently create a second account.

## What the API can and cannot do

Use the API first. The Google Play Developer API uses an edit transaction for
most publishing changes: create an edit, make all changes in that edit, validate
it, and commit it once. Changes are not live before commit, and making manual
Console changes while an edit is in progress can discard that edit.

| Task                                                              | Preferred operation                   | Notes                                                                          |
| ----------------------------------------------------------------- | ------------------------------------- | ------------------------------------------------------------------------------ |
| Create the first Play app                                         | Play Console                          | One-time Console action; the package name becomes fixed after the first upload |
| Read app, tracks, bundles, listings                               | Android Publisher API                 | Confirm package identity before every write                                    |
| Upload an Android App Bundle                                      | `edits.bundles.upload`                | Upload the signed `.aab`, never the debug APK                                  |
| Create/update a test release                                      | `edits.tracks.update`                 | Use `internal` first, then the approved closed-test track                      |
| Manage tester email lists                                         | `edits.testers.update`                | Store only the minimum tester data needed for the list                         |
| Update localized listing text                                     | `edits.listings.update`               | Keep English (`en-US`) complete before adding translations                     |
| Validate and publish an edit                                      | `edits.validate`, then `edits.commit` | Record edit ID, version code, and final status privately                       |
| Reply to reviews or inspect review data                           | Reviews API                           | Use only when the task requires it and avoid exposing user data                |
| Data safety, content rating, target audience, ads, app access     | Play Console                          | These policy forms are not replaced by an Android bundle upload                |
| Production-access questionnaire and policy appeals                | Play Console                          | Requires accurate product/business facts and may require the owner             |
| Developer identity, payments, tax, legal profile, account closure | Play Console                          | Owner-only or account-level actions; never automate with personal credentials  |

The API authorization scope for publishing is
`https://www.googleapis.com/auth/androidpublisher`. A service account having
that OAuth scope is not enough by itself: it still needs the corresponding Play
Console permission.

The canonical API reference is
[Google Play Android Publisher API v3](https://developers.google.com/android-publisher/api-ref/rest).
Its edit workflow is described in
[Edits](https://developers.google.com/android-publisher/edits).

## Read-only integration check

Before adding write access to a release workflow, run the repository's manual
**Play API access check** workflow. It validates the WIF configuration, obtains
an Android Publisher access token with the narrow API scope, and reads the app
identity. A successful result must report package `com.geoguessme.app`; a
different package means the configuration points at the wrong app and must be
fixed before any edit is created.

This check intentionally has no Play mutation capability. It does not create an
edit, upload an AAB, change a track, or commit a release. Those operations must
remain in the production release workflow and execute only after the exact
signed bundle has passed the Android release contract.

## Android release preparation

Every Play release starts from a repository revision that has passed the normal
release process:

1. Merge the feature PR into `dev` through the normal signed, verified squash
   flow.
2. Wait for the complete `dev` verification gate and successful development
   deployment. Run the source-blind hosted QA and retain its revision-bound
   report when the release checklist requires it.
3. Perform the Android checks from that exact validated tree:

    ```text
    make test-mobile
    make mobile-build-release
    ```

    `make test-mobile` covers the disposable backend, Android emulator, and
    Maestro journey. Physical Android acceptance is still required for camera,
    microphone, location, file selection, video recording, links, and device
    differences that an emulator cannot prove.

4. For a production release, use the repository's short-lived `release/*` branch
   and normal `main` promotion flow. Build the AAB from the exact promoted
   revision; do not rebuild from a different checkout after the release gate.
5. Confirm all of the following before uploading:

    - package name is exactly `com.geoguessme.app`;
    - version name comes from `.release-version`;
    - version code is greater than every version already present on the target
      Play track or app;
    - the AAB is signed with the configured upload key, not the debug key;
    - the AAB SHA-256, Git revision, package, version code, and version name are
      recorded in restricted release evidence; and
    - privacy policy, Data safety, app-access instructions, content rating,
      target audience, permissions, store listing, and country availability are
      consistent with the artifact.

The Android build currently derives `versionCode` as
`major * 1,000,000 + minor * 1,000 + patch`. Treat the Gradle file as the source
of truth if this formula changes. Play version codes are immutable once
uploaded, so the agent must query Play before choosing or building a release.

## Signing and Play App Signing

New Play apps use Play App Signing. Google holds the app-signing key; GeoGuessMe
uses a separate upload key to authenticate bundles uploaded to Play.

- Keep the upload keystore and its two passwords in separate secure backups.
- Use the Dockerized targets in [the mobile guide](../mobile.md) to create or
  use the keystore. Release builds fail closed when signing variables are
  missing; never fall back to a debug build.
- Verify the upload certificate fingerprint against the certificate registered
  in Play Console before the first upload and after any key rotation.
- If the upload key is lost, use Play Console's documented upload-key reset
  process. Do not create a new Play app and do not alter the package name.
- If Play rejects a bundle because its version code was already used, increment
  `.release-version`, rebuild from the validated revision, and keep the rejected
  artifact out of all release evidence.

See
[Upload your app to the Play Console](https://developer.android.com/studio/publish/upload-bundle)
for the current Play App Signing and Android App Bundle requirements.

## Testing-track workflow

### Internal testing

Use internal testing for the first distribution of a new bundle and for fast
smoke testing. The agent should:

1. create an API edit for `com.geoguessme.app`;
2. upload the signed AAB;
3. update the internal track with the new version code and English release
   notes;
4. validate and commit the edit;
5. read back the track and bundle, then obtain the tester opt-in link; and
6. verify installation, sign-in, refresh, camera/location permissions, chat,
   challenge upload, guess submission, logout, and account deletion on a real
   Android device when possible.

Internal testing is distributed by URL and is not a public production release.
It is also the right place to detect a bad upload key, an incorrect API origin,
or a bundle that cannot start before recruiting the larger closed-test group.

### Closed testing and production access

Check the developer account type and creation date in Play Console. A personal
developer account created after 13 November 2023 must complete a closed test
with at least 12 testers continuously opted in for at least 14 days before
requesting production access. The agent can create the tester list, publish the
closed-test release, monitor opt-in state, and produce the tester instructions.
People must opt in themselves and remain opted in for the full continuous
period; the agent cannot manufacture that requirement.

Before requesting production access, the agent verifies:

- the minimum tester count and uninterrupted opt-in period;
- closed-test feedback and any reproducible defects;
- the exact version code tested by the group;
- completed store listing and app-content declarations;
- reviewer sign-in instructions and a working reviewer account; and
- privacy policy and Data safety consistency with the current artifact.

The agent then submits the production-access request and production release
through the available API or Console path, keeps the resulting review status,
and continues monitoring without asking the user to press a routine button.

Google's current
[app testing requirements](https://support.google.com/googleplay/android-developer/answer/14151465)
and
[testing-track guide](https://support.google.com/googleplay/android-developer/answer/9845334)
are the source of truth if these thresholds or track names change.

## Store listing and policy evidence

The Play listing must match the app that is actually in the bundle. The agent
maintains English listing text and assets through the API where supported and
uses the Console for fields the API does not expose.

The policy evidence starts with these repository sources:

- [PRIVACY.md](../../PRIVACY.md) and the public
  [privacy page](https://geoguessme.com/privacy);
- [security and privacy](../security-and-privacy.md);
- `frontend/android/app/src/main/AndroidManifest.xml` for Android permissions;
- `frontend/src/` and `frontend/android/` for runtime behavior; and
- the pinned dependency manifests and SDK documentation for third-party data
  handling.

The current Android artifact requests internet, camera, microphone, coarse
location, and fine location permissions. The location used for a challenge is
sent with the challenge only when the user chooses to create/send it; uploaded
challenge coordinates are stored as game data and are revealed according to the
game's location-hiding rules. Camera and microphone data is used for the chosen
capture flow. The current native build does not register browser Web Push
subscriptions, and no advertising profile is part of the product contract.

This inventory is evidence for the form, not a pre-filled answer that can be
copied forever. Before each material app update, the agent audits the actual
artifact and SDKs and updates the Play declarations when behavior changes. In
particular, Google's **Shared** field means transfer to a third party under
Google's definition; it must not be guessed from whether another GeoGuessMe
player can see game content. Document the provider, purpose, retention, and user
choice for each declared data type.

For every app that is active outside an internal-only test, keep the Data safety
form complete and consistent with the privacy policy. The privacy policy must be
a public, non-geofenced web page and must also be reachable from inside the app.
Account creation requires an accessible account-deletion path; GeoGuessMe
provides it from **Settings**.

### Reviewer access

If signed-in content is required, maintain a dedicated reviewer account with a
small seeded group and safe sample content. The account must:

- use a stable username/email and password stored only in the approved secure
  vault;
- not require the owner's 2FA device, email OTP, or a personal mailbox;
- have only the access needed to demonstrate the app's core functionality; and
- be updated in Play Console's App access instructions whenever auth, required
  verification, or navigation changes.

The agent keeps the reviewer instructions in English, tests them against the
submitted version, and never puts the credentials in source, a PR, screenshots,
logs, or release notes. If a reviewer account is unavailable, the agent may
create or repair the dedicated account using the normal product flow, but must
not expose its password while doing so.

## Release evidence and rollback

Keep a restricted record for every upload and production change containing:

- source commit and whether it passed the dev deployment and Android checks;
- package name, version name, version code, AAB SHA-256, and signing certificate
  fingerprint;
- Play track, edit ID, release status, countries, and rollout fraction;
- the exact privacy-policy URL and policy-form status;
- reviewer-access account label, without the password; and
- tester-track link and opt-in count when a closed-test gate applies.

Do not store raw tester email lists, service-account JSON, reviewer passwords,
or owner identity/payment documents in the repository. If a Play upload or
review fails, preserve the status and error, fix the underlying bundle or
declaration, and create a new edit. Do not blindly retry a stale edit or
silently upload a different artifact.

If a release must be stopped, halt or roll back the Play track using the version
code and track recorded in the release evidence, then verify the resulting
serving version. An Android rollback cannot undo a forward database migration;
coordinate it with the hosted deployment rollback rules in the
[hosted deployment runbook](hosted-deployment.md).

## Routine account maintenance

At least quarterly, and after every material release, the agent checks:

- Play Console inbox, policy deadlines, app status, review status, Android
  vitals, pre-launch reports, crashes, ANRs, and tester feedback;
- service-account access, app-scoped permissions, key age, and unused users;
- the upload certificate and Play App Signing state;
- country availability, device exclusions, staged-rollout state, and the
  currently served version code;
- privacy policy, Data safety, app-access instructions, content rating, target
  audience, ads declaration, and permissions declarations; and
- whether the API still reads and writes the expected app and track state.

Remove stale Play users and tester lists when they are no longer needed, but
preserve the restricted release record. Do not delete the app, close the
developer account, change the legal/payment profile, or rotate the app-signing
key as routine maintenance; those are deliberate owner-level operations.

## Common failures

| Symptom                         | First checks                                                                                                                                                  |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| API `401`                       | Key validity, token scope, clock skew, and whether the credential belongs to the intended service account                                                     |
| API `403`                       | Android Publisher API enabled, service account invited in Play Console, app-scoped permission, and propagation time; Cloud IAM alone is insufficient          |
| API `409` or edit conflict      | Another edit or manual Console change is active; read the current state, discard the stale edit, and start one fresh edit                                     |
| Bundle rejected                 | Package name, monotonically increasing version code, upload certificate, target SDK, manifest permissions, and AAB integrity                                  |
| Testers cannot install          | Correct opt-in URL, tester Google account, track country/device state, release status, and whether the tester is still opted in to another track              |
| “Not available in your country” | Track country targeting and release status first; internal testing uses its own URL-based availability and is not the same as production country distribution |
| Review requests access          | Test the English reviewer instructions and dedicated account against the exact submitted version; never provide owner credentials                             |
| Policy rejection                | Preserve the rejection text, map it to the app behavior and declaration, fix the source or form, and submit a new reviewed edit                               |

The Play Console Help pages for
[publishing an app](https://support.google.com/googleplay/android-developer/answer/9859751),
[Data safety](https://support.google.com/googleplay/android-developer/answer/10787469),
and
[app content](https://support.google.com/googleplay/android-developer/answer/9859455)
are the current policy references. Re-check them when Google changes the Console
workflow.
