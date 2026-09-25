# GeoGuessMe privacy policy

Last updated: 17 September 2026

This policy applies to the GeoGuessMe website and Android app. The public
version is available at <https://geoguessme.com/privacy>. It explains what we
collect, why we use it, who can receive it, how long we keep it, and how to
exercise your rights. The app is GeoGuessMe, and privacy questions can be sent
to `privacy@geoguessme.com`.

## Who is responsible

The data controller is the operator of GeoGuessMe, the service published at
[geoguessme.com](https://geoguessme.com). The operator identity, publication
details, and hosting providers are listed on the legal notice page at
<https://geoguessme.com/legal>.

For any privacy question or request — access, correction, deletion, restriction,
objection, or portability — contact `privacy@geoguessme.com`. You can also
delete your account yourself from **Settings** in the app.

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
asks for these permissions, and you can deny them.

- Location is read once, at the moment you create a challenge, and attached to
  that challenge only. The app does not track your position in the background.
- Camera frames and selected files stay on your device until you press **Send**.
- Visual effects, including optional face-tracking lenses, are processed on your
  device. Face geometry is never uploaded or stored by GeoGuessMe.
- Uploaded images are normalized and EXIF metadata, including embedded GPS
  coordinates, is removed before storage.

### Technical and notification data

We process session cookies, authentication and one-time-token hashes, WebSocket
ticket data, and security-relevant request metadata. If you enable browser
notifications, we store the encrypted Web Push endpoint and its subscription
keys so notifications can be delivered.

Our hosting and security systems may process an IP address, browser or device
type, timestamps, and error information to deliver the service and prevent
abuse. GeoGuessMe does not use this information to build advertising profiles.

## Why we use information, and on what legal basis

For people in the European Union, we rely on the following legal bases under the
GDPR:

| Purpose                                                                                                                                     | Legal basis                                             |
| ------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------- |
| Creating and operating your account, keeping you signed in, verifying your email, and recovering your account                               | Performance of the service (Art. 6(1)(b))               |
| Providing the game: groups, chat, challenges, scoring, maps, and showing results to the right members                                       | Performance of the service (Art. 6(1)(b))               |
| Camera, microphone, and location capture for a challenge you choose to create                                                               | Performance of the service (Art. 6(1)(b)), kept minimal |
| Optional face-tracking lenses                                                                                                               | Your consent, on-device only (Art. 6(1)(a))             |
| Delivering Web Push notifications you have enabled                                                                                          | Your consent (Art. 6(1)(a))                             |
| Securing the service: rate limiting, abuse and fraud prevention, debugging, and keeping unauthorized people out of private groups and media | Legitimate interests (Art. 6(1)(f))                     |
| Complying with legal requests and defending legal claims                                                                                    | Legal obligation or legitimate interests                |

Where we rely on consent, you can withdraw it at any time: turn off
notifications in the app or your device settings, and deny or revoke camera,
microphone, or location permission in your device settings. Withdrawing consent
does not affect processing that already happened.

We do not sell or rent personal data to data brokers, use it for targeted
advertising, or use camera frames for facial-recognition identification.

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

## International transfers

The application and database servers run in the European Union (Hetzner Cloud).
Some providers operate global networks that may involve transfers outside the
European Economic Area:

- Cloudflare delivers network protection and object storage through a global
  network and is certified under the EU-U.S. Data Privacy Framework where that
  framework applies; otherwise transfers rely on the European Commission's
  Standard Contractual Clauses.
- Brevo processes transactional email in the European Union.
- If you enable Web Push, your browser's push service delivers encrypted
  notification payloads; the notification content is end-to-end encrypted, so
  the push service does not read it.

Where a provider is established outside the EEA and is not covered by an
adequacy decision, we rely on Standard Contractual Clauses or an equivalent
safeguard with that provider. Contact us if you want details about the
safeguards that apply to your data.

## How long we keep information

| Information                                                             | Retention                                                                                            |
| ----------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| Account and group data, messages, guesses, and scores                   | Until you delete your account, or until the data is no longer needed to provide the service          |
| Challenge photos and videos                                             | The configured media-retention period, 30 days by default, then removed from active object storage   |
| Scores and limited challenge metadata                                   | May remain after the original media is removed                                                       |
| Encrypted database backups                                              | Restore points kept for 24 hours, 14 days, 8 weeks, and up to 6 months, then deleted on the rotation |
| Refresh sessions                                                        | Deleted 30 days after expiry or revocation                                                           |
| One-time tokens (email verification, password reset, WebSocket tickets) | Deleted 1 day after use, or at token expiry otherwise                                                |
| Web Push subscriptions                                                  | Until you disable notifications or delete your account                                               |

Deleting your account removes the account, sessions, authentication material,
group memberships, messages, guesses, challenge views, and associated database
records. Uploaded media is queued for deletion from object storage; that final
storage step can complete shortly after the account record is removed. Backup
copies age out on the rotation above.

## Security

We use encrypted transport, secure session cookies, password hashing,
authorization checks for group and media access, upload validation, metadata
stripping, rate limits, and controlled access to operational systems. No
internet service can promise absolute security. Use a unique password and
contact us promptly if you suspect unauthorized access.

If a personal-data breach is likely to put your rights at risk, we will notify
the French data-protection authority (CNIL) within 72 hours where required, and
inform you directly when the risk is high.

## Your rights

If you are in the European Union, you have the right to:

- **Access** the personal data we process about you;
- **Rectify** inaccurate data — you can update your username, recovery email,
  and avatar in Settings;
- **Erase** your data — you can delete your account from Settings, or ask us to
  do it;
- **Restrict** processing while a dispute about the data is resolved;
- **Object** to processing based on legitimate interests;
- **Portability** — receive the data you provided in a structured,
  machine-readable format;
- **Withdraw consent** at any time for consent-based processing;
- **Complain** to the CNIL (Commission Nationale de l'Informatique et des
  Libertés, [cnil.fr](https://www.cnil.fr)) or your local data-protection
  authority.

To exercise a right, email `privacy@geoguessme.com` with enough information to
locate the account safely. Do not send passwords, authentication codes, or other
sensitive credentials by email. We answer within one month; complex or numerous
requests can extend that by two further months, and we will tell you if so.

## Permissions and children

Users can deny camera, microphone, location, or notification permission in
device or browser settings. Features that need a denied permission may not work.

GeoGuessMe is intended for people aged 15 and over. We do not knowingly collect
personal information from children under 15, and signup requires confirming the
minimum age. If a higher minimum age applies where you live, follow that
requirement. If you are a parent or guardian and believe a child under 15 has
provided personal information, contact us so we can investigate and remove it
where appropriate.

## Changes

We may update this policy when GeoGuessMe changes or privacy requirements
evolve. We will update the date at the top of this page and, when a change is
material, provide a clearer notice in the app or through an appropriate service
message.

For the designed public presentation of this policy, see
<https://geoguessme.com/privacy>.
