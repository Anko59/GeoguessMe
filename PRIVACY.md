# GeoGuessMe privacy policy

Last updated: 11 September 2026

This policy applies to the GeoGuessMe website and Android app. The public
version is available at <https://geoguessme.com/privacy>. It explains what we
collect, why we use it, who can receive it, and how to request deletion. The app
is GeoGuessMe, and privacy questions can be sent to `privacy@geoguessme.com`.

## What we collect

We collect information you provide, information created when you play, and
limited technical information needed to secure and operate the service.

### Account and sign-in

- Username, email address, password hash, email-verification status, avatar
  choice, and account timestamps.
- If you use an enabled third-party sign-in option, the provider identifier and
  basic profile or contact information needed to create or link your account.
  GeoGuessMe does not receive your provider password.
- Session cookies and authentication material needed to keep you signed in,
  verify your email, recover your account, and revoke access.

### Groups and gameplay

- Group memberships, invite and participation records, chat messages, and
  reactions.
- Challenge photos or videos, challenge metadata, guesses, scores, and game
  timing information.
- Group members see content according to the game state and the uploader's
  choices, including whether a challenge location is hidden until reveal.

### Camera, location, and media

When you choose to create a challenge, GeoGuessMe can use your camera,
microphone for a recording, and device location. The operating system or browser
asks for these permissions, and you can deny them. Camera frames and selected
files stay on your device until you press **Send**. Visual effects are processed
on the device. Uploaded images are normalized and EXIF metadata, including
embedded GPS coordinates, is removed.

### Technical and notification data

We process session cookies, authentication and one-time-token hashes, WebSocket
ticket data, and security-relevant request metadata. If you enable browser
notifications, we store the encrypted Web Push endpoint and its subscription
keys so notifications can be delivered.

Our hosting and security systems may process an IP address, browser or device
type, timestamps, and error information to deliver the service and prevent
abuse. GeoGuessMe does not use this information to build advertising profiles.

## How we use information

We use information to:

- authenticate users, maintain sessions, verify email, and recover accounts;
- create groups, deliver invitations, provide chat, and show the game to the
  right members;
- process challenges, protect uploaded media, calculate scores, and show maps
  and results;
- deliver a notification when a user has enabled notifications for a group;
- detect abuse, rate-limit requests, troubleshoot failures, and secure the
  service; and
- respond to privacy, support, account-access, and deletion requests.

GeoGuessMe does not sell or rent personal data to data brokers, use it for
targeted advertising, or use camera frames for facial-recognition
identification.

## Sharing and service providers

We share information only when it is needed to provide GeoGuessMe, when a user
directs us to share it through a game, or when disclosure is required to protect
people, the service, or the law.

- **Other players:** Group members can receive usernames, avatars, messages,
  challenge media, guesses, and scores that the game makes available.
- **Hosting and storage providers:** Current deployments use Hetzner for server
  hosting and Cloudflare for network protection, DNS, email routing, and object
  storage. These providers process application data only as needed to operate
  the service.
- **Transactional email provider:** Brevo processes email addresses and message
  contents for verification, account recovery, and essential service mail.
- **Maps and push delivery:** OpenStreetMap tile infrastructure receives
  ordinary map-tile requests. If Web Push is enabled, the browser's push service
  receives encrypted notification delivery requests.
- **Optional sign-in providers:** Keycloak brokers an enabled provider such as
  Google. Those providers may process sign-in according to their own privacy
  policies. Users choose whether to use that sign-in method.

We may disclose information to comply with a valid legal request, enforce our
terms, investigate fraud or abuse, or protect the rights and safety of users and
the service. We do not sell personal information.

## Retention

- Account and group data is kept until the account is deleted or the data is no
  longer needed to provide the service.
- Challenge media is kept for the configured media-retention period, currently
  30 days by default, and then removed from active object storage.
- Scores and limited challenge metadata may remain after the original media is
  removed.
- Expired sessions, verification tokens, password-reset tokens, and WebSocket
  tickets are cleaned up automatically.

## Security

We use encrypted transport, secure session cookies, password hashing,
authorization checks for group and media access, upload validation, metadata
stripping, rate limits, and controlled access to operational systems. No
internet service can promise absolute security. Use a unique password and
contact us promptly if you suspect unauthorized access.

## Account deletion and data requests

Users can delete their account from **Settings**. Account deletion removes the
account, sessions, authentication material, group memberships, messages,
guesses, challenge views, and associated database records. Uploaded media is
queued for deletion from object storage; that final storage step can complete
shortly after the account record is removed.

Users can update their username, recovery email, and avatar in Settings. To
request access, correction, or deletion support, email `privacy@geoguessme.com`.
Do not send passwords, authentication codes, or other sensitive credentials by
email.

## Permissions and children

Users can deny camera, microphone, location, or notification permission in
device or browser settings. Features that need a denied permission may not work.

GeoGuessMe is not directed to children under 13, and we do not knowingly collect
personal information from children under 13. If a higher minimum age applies
where you live, follow that requirement. Contact us if you believe a child has
provided personal information so we can investigate and remove it where
appropriate.

## Changes

We may update this policy when GeoGuessMe changes or privacy requirements
evolve. We will update the date at the top of this page and, when a change is
material, provide a clearer notice in the app or through an appropriate service
message.

For the designed public presentation of this policy, see
<https://geoguessme.com/privacy>.
