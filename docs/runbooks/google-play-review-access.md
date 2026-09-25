---
status: primary
---

# Google Play review access

Google Play must be able to reach every restricted part of GeoGuessMe during
each review. The app therefore has a dedicated reviewer account. It is not a
personal account, and its password must never be committed to Git, written in an
issue or pull request, sent to CI, or included in public documentation.

## Account policy

The reviewer account must remain:

- active for the lifetime of the published app and any pending review;
- usable with the ordinary GeoGuessMe username/password login;
- free of MFA, CAPTCHA, invitation, subscription, device, or email-verification
  prerequisites;
- separate from every maintainer's personal account; and
- free of production data that is not needed for review.

At the time of this runbook's creation, production exposes the legacy
username/password flow (`OIDC_ENABLED=false`). Verify the live capability before
each major submission because the authentication rollout may change this path.

## Secret storage

The reviewer credential bundle is stored in the release operator's local GNOME
Keyring. The repository contains only these lookup coordinates:

```text
service: geoguessme-google-play-review
username item: account=reviewer-username
password item: account=reviewer-password
```

Future agents may load the values into shell variables for a verification or
Play Console handoff, but must not print them or include them in command-line
arguments, logs, screenshots, chat messages, GitHub Actions, or tracked files:

```sh
review_username="$(secret-tool lookup service geoguessme-google-play-review account reviewer-username)"
review_password="$(secret-tool lookup service geoguessme-google-play-review account reviewer-password)"
```

The keyring is intentionally local. If the operator changes computers, the
credential must be transferred through an approved private secret channel and
the old keyring entry must not be copied into the repository.

## Safe verification

Before submitting an update, verify the account against production without
printing the session token or password. Use the API only to confirm login,
authenticated profile access, and access to the groups screen; do not create or
modify production groups as part of a routine check.

The expected result is a successful login and HTTP 200 responses from the
authenticated profile and groups requests. If verification fails, stop the
submission, repair the account, and update the Play Console entry before
resuming review.

When the password is rotated, update the keyring and Play Console in the same
maintenance window. Do not rotate it while a review is active unless the old
credential has already failed.

## Google Play Console entry

In the app's **App access** or **Get ready to publish your app** section, add a
sign-in-details set with:

| Field    | Value                                                 |
| -------- | ----------------------------------------------------- |
| Name     | `Google Play reviewer account`                        |
| Username | The value from the keyring's `reviewer-username` item |
| Password | The value from the keyring's `reviewer-password` item |

Use this English explanation in **Any other information required for access**:

```text
Open GeoGuessMe and select Login. Enter the username and password above. No email verification, invitation, subscription, one-time code, or two-factor authentication is required. After signing in, open Groups.
```

Do not select Google sign-in for this entry: the reviewer account is a normal
GeoGuessMe account and must not depend on a personal Google session. Keep the
account and these details available for every subsequent update review.

## Account creation or recovery

If the keyring entry is missing or the account no longer works:

1. Stop the Play Console submission.
2. Create or repair a dedicated account through the production signup/login
   flow, using no recovery email unless an operator-owned mailbox is required
   for a documented recovery plan.
3. Verify login and the protected Groups screen.
4. Replace the keyring values without printing them.
5. Update the Play Console sign-in details before submitting again.

Never solve a missing reviewer credential by putting a password in a PR,
repository secret, deployment environment, browser URL, or public support
request.
