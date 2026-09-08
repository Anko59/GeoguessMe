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
are outside the automated debug workflow.

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
