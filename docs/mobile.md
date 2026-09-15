# Mobile application

GeoGuessMe uses Capacitor to distribute the existing React/TypeScript
application as a native app. There is one frontend and one Go API: Vite builds
the browser/PWA, while Capacitor copies the same build into the Android native
project. Android is the validated reference platform.

## Architecture and boundaries

Platform differences live in `frontend/src/platform`. The shared screens,
authentication, groups, chat, challenges, Leaflet map, camera UI, MediaPipe,
Three.js composition, uploads, and WebSocket protocol are unchanged. The
adapters provide runtime detection, API and public URL construction,
geolocation, sharing, haptics, app/deep-link lifecycle, and the Web Push
transport boundary.

The production native build uses bundled assets at `https://app.geoguessme.com`
and talks to the normal production origin. Native HTTP, WebSocket,
refresh-cookie, and OIDC URLs are derived from `VITE_API_ORIGIN`; invite links
are derived from `VITE_WEB_ORIGIN`. Both default to `https://geoguessme.com` in
the Make workflow. The hosted backend must allow `https://app.geoguessme.com` in
`ALLOWED_ORIGINS` before distributing the app. Do not weaken cookie flags or use
a development server URL in a distributable build.

Android uses native geolocation, share sheets, haptics, status-bar styling, deep
links, and system-back handling. Camera capture deliberately retains the WebView
`getUserMedia`/video/canvas pipeline. PWA installation UI and Web Push
registration remain browser-only.

## Android prerequisites and commands

The host needs Git, Make, Docker, Docker Compose, and enough disk for the
Android SDK and emulator (allow roughly 12 GiB while preparing from an empty
cache). Android Studio, a host JDK, Node, Gradle, Maestro, and a host Android
SDK are not required. `/dev/kvm` is selected automatically when available; the
emulator falls back to software acceleration otherwise.

From the repository root:

```text
make mobile-prepare  # pinned SDK packages and the API 36 AVD
make mobile-sync     # production web build and Capacitor sync
make mobile-build    # debug APK
make test-mobile     # isolated stack + seed + build + emulator + Maestro
```

`make mobile-build` writes
`frontend/android/app/build/outputs/apk/debug/app-debug.apk`. `make test-mobile`
is unattended: it builds an APK with a test-only localhost server URL, boots a
clean headless emulator, uses `adb reverse` to reach the isolated Compose
gateway, grants declared test permissions, installs the APK, and runs Maestro.
Its cleanup removes the disposable database and media volumes.

The local-server override exists only through `CAPACITOR_SERVER_URL` during the
test build. Normal mobile builds leave it empty and embed production assets. To
point a bundled development build at a different API without changing source,
pass HTTPS origins explicitly:

```text
make mobile-build \
  MOBILE_API_ORIGIN=https://dev.geoguessme.com \
  MOBILE_WEB_ORIGIN=https://dev.geoguessme.com
```

The corresponding backend origin allowlist must include
`https://app.geoguessme.com`. Cleartext traffic is enabled only for the Android
debug build type; release builds prohibit it.

## Automated journey and diagnostics

The Maestro journey launches and authenticates the installed app, opens a seeded
group, sends a chat message, accepts a challenge, places and submits a map
guess, reviews the result, opens the camera, captures the emulator's
deterministic image input, previews/retakes it, and uploads a new challenge. The
emulator location is fixed as well, so attachment does not depend on host
hardware or an external account.

On failure, inspect `.local/mobile/artifacts/`. It contains Maestro JUnit output
and logs plus a screenshot, UI hierarchy, full logcat, top activity, installed
package/permission state, emulator log, and acceleration report where available.
Credentials generated for the isolated fixture stay under ignored
`.local/mobile/` paths.

The pull-request workflow selects this journey for Android, backend, frontend,
shared, deployment, and mobile-tooling changes. The post-merge development
workflow runs it for every push to `dev`, before publishing development images
or deploying the hosted development stack. A failed mobile gate blocks that
publication and retains only the bounded diagnostic directory for seven days.
This development gate validates the app/runtime integration; it does not create
or upload a signed Play bundle. The production release workflow must build and
verify the release-specific signed AAB after the release version and upload-key
configuration are available, then retain its provenance with the artifact.

Camera switching, held video recording, file-picker import, and advanced lenses
remain covered by the existing browser tests and shared implementation but are
not automated in the first native Maestro journey. They need device-matrix
acceptance on representative physical Android hardware before a store release.

## Distribution and signing

The checked-in Android project uses the standard debug key for local testing.
Never commit a release keystore, passwords, `google-services.json`, APK/AAB
outputs, SDK files, AVD data, or generated test credentials. A release owner
must configure Gradle signing from the deployment secret store, increment the
native version code/name, build an AAB from the validated revision, and retain
the signed artifact digest with release evidence. Store submission and signing
are outside the automated debug workflow. Account access, Play API operations,
testing tracks, policy declarations, and store maintenance are documented in the
[Google Play developer account runbook](runbooks/google-play-console.md).

The Dockerized release flow keeps the upload key under ignored `.local/` and
passes passwords only as environment variables. In a private shell, export two
passwords without putting them in the command history, then create the key once:

```text
read -r -s -p 'Keystore password: ' MOBILE_KEYSTORE_PASSWORD
export MOBILE_KEYSTORE_PASSWORD
read -r -s -p 'Key password: ' MOBILE_KEY_PASSWORD
export MOBILE_KEY_PASSWORD
make mobile-keystore
unset MOBILE_KEYSTORE_PASSWORD MOBILE_KEY_PASSWORD
```

Keep the keystore and both passwords in separate secure backups. To build a
release bundle, export the passwords again and run the build from the validated
revision:

```text
read -r -s -p 'Keystore password: ' MOBILE_KEYSTORE_PASSWORD
export MOBILE_KEYSTORE_PASSWORD
read -r -s -p 'Key password: ' MOBILE_KEY_PASSWORD
export MOBILE_KEY_PASSWORD
make mobile-build-release
unset MOBILE_KEYSTORE_PASSWORD MOBILE_KEY_PASSWORD
```

The resulting signed bundle is
`frontend/android/app/build/outputs/bundle/release/app-release.aab`. The release
task fails closed unless all signing values are present; no debug key fallback
is allowed. The default package is `com.geoguessme.app` and the version comes
from `.release-version`.

Before a bundle is handed to a distribution workflow, verify it and generate the
non-secret provenance manifest through the Dockerized Make targets:

```text
make mobile-verify-release \
  MOBILE_EXPECTED_UPLOAD_CERT_SHA256=... \
  MOBILE_REQUIRE_EXPECTED_CERT=true
make mobile-release-manifest
```

The verifier reads the packaged manifest with the pinned Bundletool image,
checks the fixed package name, compares the version name and calculated version
code with `.release-version`, verifies the JAR signature, and records the AAB
and upload-certificate SHA-256 values. The manifest additionally binds those
values to the source commit and Git tree supplied by Make. It contains no
passwords or private-key material.

The production release workflow now performs this build before image promotion.
It materializes the keystore only on the ephemeral runner from the
`MOBILE_UPLOAD_KEYSTORE_BASE64` production secret, passes the two signing
passwords through `MOBILE_KEYSTORE_PASSWORD` and `MOBILE_KEY_PASSWORD`, and
requires the non-secret `MOBILE_UPLOAD_CERT_SHA256` production variable. It then
verifies the source SHA/tree, package, version, signing certificate, and AAB
SHA-256, retains the exact AAB plus manifest as a workflow artifact, and
attaches both files to the GitHub release. The image-promotion job consumes that
artifact; it never rebuilds the bundle. No Play edit is created or committed by
this artifact job.

Configure the production environment once, without committing any signing
material. The keystore secret is binary data encoded as one base64 line; the two
passwords remain separate secrets:

```text
base64 -w0 .local/mobile/upload-keystore.jks | \
  gh secret set MOBILE_UPLOAD_KEYSTORE_BASE64 --env production
gh secret set MOBILE_KEYSTORE_PASSWORD --env production
gh secret set MOBILE_KEY_PASSWORD --env production
gh variable set MOBILE_UPLOAD_CERT_SHA256 --env production
```

Run the `gh secret set` commands from a private shell and provide each password
interactively when prompted. The certificate variable is the SHA-256 fingerprint
of the Play upload certificate, with or without colons. The separate Play OIDC
variables documented below are used for API access and are not a replacement for
the Android upload key. The Play publication job consumes the exact retained
artifact after the production deployment succeeds.

### Play API release automation

The production release workflow now owns the complete Android distribution
boundary. It builds and verifies the signed AAB, binds it to the release commit
and Git tree, retains the AAB and manifest as one artifact, and waits for the
production deployment to succeed. It then authenticates through GitHub OIDC with
a short-lived Android Publisher token and performs one Play edit:

1. confirm the app package identity;
2. create an edit and upload the retained AAB;
3. update the configured track with the manifest version code and release
   status;
4. validate the edit;
5. commit the edit with changes sent for review; and
6. read the track back and fail if Play does not report the submitted version
   and status.

The workflow does not rebuild or select a different bundle after the release
artifact job. The API client verifies the local AAB SHA-256 against the manifest
before it creates an edit, and verifies the Play-reported version code before
changing the track.

Configure these values in the GitHub `production` environment before running a
production release:

- `MOBILE_UPLOAD_KEYSTORE_BASE64` secret: the base64-encoded upload keystore;
- `MOBILE_KEYSTORE_PASSWORD` and `MOBILE_KEY_PASSWORD` secrets: signing
  passwords;
- `MOBILE_UPLOAD_CERT_SHA256` variable: the expected upload certificate
  fingerprint;
- `PLAY_GCP_WORKLOAD_IDENTITY_PROVIDER` variable: the Google WIF provider
  resource;
- `PLAY_GCP_SERVICE_ACCOUNT` variable: the Play-authorized service-account
  email; and
- `PLAY_RELEASE_TRACK` variable: the target Play track, normally `internal`, a
  closed-test track, or `production` after the account is eligible.

`PLAY_RELEASE_STATUS` is an optional variable and defaults to `completed`; use
`inProgress` only when the release process explicitly requires a staged rollout.
The Google service account must separately have the required app-scoped Play
permissions. The workflow accepts no JSON key and does not create a credential
file.

The repository also includes a read-only **Play API access check** workflow. Use
it to validate OIDC and app identity without creating an edit or changing Play
state. It is a diagnostic, not an alternative publication path.

If the access preflight, artifact verification, production deployment, or
post-commit track readback fails, the release stops and retains the failing
workflow evidence. A successful commit is reported with the package, edit ID,
track, version, digest, and source provenance; tokens and signing material are
never printed or stored.

App links recognize `https://geoguessme.com`, `https://www.geoguessme.com`, and
the `geoguessme:` custom scheme. HTTPS app links become verified only after the
production site serves an Android Digital Asset Links file for the actual
release signing certificate.

## Push transport

Web Push remains unchanged for browsers and installed PWAs; the native runtime
does not register that service-worker subscription. Native push requires an
explicit backend device-token model (platform, installation owner, token
rotation, logout/account-deletion cleanup), FCM/APNs credentials, delivery
handling, and notification deep links. Those credentials and server contract do
not exist in this iteration, so native push is intentionally absent rather than
emulating Web Push insecurely.

## iOS enablement path

iOS has not been compiled or tested because the reference environment is Ubuntu.
The shared platform adapters already distinguish `ios`, use Capacitor APIs
supported on iOS, and avoid Android addresses in application code.

On a macOS development branch, keep all Capacitor packages on the repository's
same major and patch line, add `@capacitor/ios`, generate and commit
`frontend/ios`, and add Dockerized Make targets equivalent to `mobile-sync` for
`cap add ios`/`cap sync ios`. Then use the generated Xcode workspace to:

1. select the application team and bundle identifier;
2. add camera, microphone, photo-library, and location usage descriptions;
3. configure Associated Domains for the production app links;
4. configure release signing without committing credentials;
5. validate password/OIDC login, refresh cookies, WebSockets, camera switching,
   photo/video/file selection, lenses, uploads, Leaflet gestures, deep links,
   keyboard/safe areas, background/resume, and logout on simulator and device;
6. add an iOS Maestro journey only after the native project passes that device
   acceptance.

APNs/native push should be implemented only with the shared server-side token
lifecycle described above. No iOS readiness or App Store compatibility is
claimed by the Android result.
