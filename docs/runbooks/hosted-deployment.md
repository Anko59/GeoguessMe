---
status: primary
---

# Hosted deployment runbook

This runbook is the production change checklist for the shared Hetzner host. The
target recovery objectives are a one-hour RPO, about two hours RTO, and at most
two minutes of planned deployment interruption.

## One-time prerequisites

1. Rotate any Cloudflare or Hetzner credential shared during setup and enable
   2FA on GitHub, Cloudflare, Hetzner, and Brevo.
2. Activate Cloudflare R2 and Zero Trust. Create a private
   `geoguessme-terraform-state` bucket and a bucket-scoped S3 credential for
   Terraform state. Create separate bucket-scoped credentials for dev media,
   production media, and database backups after Terraform creates those buckets.
3. Create a Brevo free account, authenticate `geoguessme.com`, and copy its DKIM
   records into `brevo_dns_records`; set the single combined SPF value (Brevo
   include plus `_spf.mx.cloudflare.net`) in `spf_record`. Publish DMARC as
   `p=none` during validation, then tighten per
   [docs/runbooks/dmarc-rollout.md](dmarc-rollout.md) after reviewing reports.
4. Create one operator SSH key and separate CI keys for dev and production. Put
   only public keys in `terraform.tfvars`; private CI keys go into their
   matching GitHub environment.
5. Copy `backend.hcl.example` to ignored `backend.hcl`, fill the R2 endpoint,
   and export its S3 credentials plus rotated `HCLOUD_TOKEN` and
   `CLOUDFLARE_API_TOKEN`.

The Cloudflare Terraform token needs zone DNS/settings, Email Routing, R2
bucket, Tunnel, identity-provider, and Access-application write permissions
scoped to this account/zone. Service tokens are created outside Terraform, so
this token does not need Access service-token write permission. The Hetzner
token should be scoped to the dedicated project. Do not reuse either token in
application or deployment jobs.

## Provision

Run `make terraform-validate`, inspect `make terraform-plan`, and apply only the
saved plan with `CONFIRM=apply make terraform-apply`. Terraform state locking
uses R2's S3 lockfile. The server has Hetzner backups, delete/rebuild
protection, unattended security updates, a 2 GB swap file, bounded Docker logs,
and no public inbound firewall rule.

Terraform ignores post-creation `user_data` drift because Hetzner cannot update
cloud-init in place and replacing a stateful host is unsafe. Apply bootstrap
changes explicitly to the running host and verify them, or use the documented
backup/restore replacement procedure; a newly created host always receives the
current template. Runtime scripts and compose definitions must be updated as one
exact-revision set using the procedure in
[docs/runbooks/runtime-hardening.md](runtime-hardening.md#applying-monitored-host-definitions);
copying only the latest changed file creates an unverifiable host state.

The deployment and backup locks live below `/run`, which is cleared at boot.
Cloud-init installs `/etc/tmpfiles.d/geoguessme.conf` so systemd recreates
`/run/lock/geoguessme` as `deploy:deploy` with mode `0750`. When applying this
bootstrap improvement to an existing host, install that same tmpfiles rule and
run `systemd-tmpfiles --create /etc/tmpfiles.d/geoguessme.conf` through the
Access-protected operator route before rerunning a deployment. Do not broaden
the restricted CI deployment key into shell access for this repair.

Use the Access-protected operator SSH route. Store the corresponding private
operator key in the team's password manager (not this repository) and retain a
documented recovery copy. The server-only age identity is deliberately not an
operator credential and must never be copied off-host:

```text
ssh -i /path/to/operator-key \
  -o ProxyCommand='make -s cloudflared-access-ssh HOST=%h' \
  ops@deploy.geoguessme.com
```

Export a Cloudflare Access service-token client ID and client secret as
`TUNNEL_SERVICE_TOKEN_ID` and `TUNNEL_SERVICE_TOKEN_SECRET` before using this
non-interactive operator route. Create that token outside Terraform (see
[docs/runbooks/access-tokens.md](access-tokens.md)); Terraform never holds
service-token secrets and exposes no service-token outputs. Do not place either
value on the command line. Read the two generated public age recipients from
`/etc/geoguessme/age/*-recipient.txt`, fill each environment example with unique
database/JWT/metrics/Restic credentials and its dedicated R2/Brevo values. Web
Push is optional: leave all three VAPID variables absent to disable it, or mint
one stable keypair per environment and export all three alongside the other
credentials:

```text
make vapid-keys
export VAPID_PUBLIC_KEY=... VAPID_PRIVATE_KEY=... VAPID_SUBJECT=mailto:...
make secrets-generate ENV=dev RECIPIENT=age1...
make secrets-generate ENV=production RECIPIENT=age1...
```

Both environments set `APP_ENV=production`. The backend disables Push when all
three VAPID variables are absent, but rejects a partial keypair or invalid
contact subject. Store each configured environment's pair outside the
repository: rotating it invalidates every browser subscription, so the same
values must be reused whenever that secret file is regenerated.

### Credential and Push recovery

- Cloudflare only reveals an Access service-token secret at creation or
  rotation. Tokens are created outside Terraform and rotate every 60 days with
  overlap (see [docs/runbooks/access-tokens.md](access-tokens.md)). If a secret
  is unavailable locally, rotate that environment's token, update
  `CF_ACCESS_CLIENT_ID` and `CF_ACCESS_CLIENT_SECRET` in the matching GitHub
  environment (`development`, `production`, or `monitoring`), and verify a
  deployment before the overlap expires. Alert 14 days before each token's
  expiry.
- If the operator SSH private key is missing, use a planned Hetzner rescue-mode
  reboot to install a newly generated public key in
  `/home/ops/.ssh/authorized_keys`. Restrict temporary SSH ingress to the
  operator's `/32`, verify Access-routed SSH and public readiness after normal
  boot, then remove that firewall rule.
- To enable Push on an already-deployed host, generate one stable VAPID pair,
  add all three values to its existing production dotenv, re-encrypt that exact
  dotenv with `/etc/geoguessme/age/production-recipient.txt`, commit only the
  encrypted file, and restart the production application stack. Never rerun
  `secrets-generate` for this operation: it rotates unrelated database, JWT, and
  backup values.

Review the encrypted files, commit them, and never commit plaintext dotenv or
age private keys. Unless both GHCR packages are public, add a read-only
`read:packages` token as `GHCR_TOKEN` in each encrypted host environment; it
must have no repository-write scope. Set the Terraform Access service-token
outputs, SSH private key, and SSH known-host line as environment-scoped GitHub
secrets named `CF_ACCESS_CLIENT_ID`, `CF_ACCESS_CLIENT_SECRET`,
`DEPLOY_SSH_PRIVATE_KEY`, and `DEPLOY_SSH_KNOWN_HOSTS`. Put the same two
read-only Access values in the `monitoring` environment; it contains no SSH key
and is restricted to scheduled health checks. Its deployment-branch policy must
allow `dev`, because GitHub runs scheduled workflows from the default branch and
`dev` is the default branch. A policy that allows only `main` makes every
`Hosted health` run fail immediately with no steps executed.

Dev and production use independent Restic repositories at the `/dev` and
`/production` prefixes of the shared private backup bucket. Keep their
`RESTIC_PASSWORD` values distinct; a restore must use the matching environment
secret file.

## GitHub and release flow

Features merge by verified squash PR into `dev`; pull-request CI runs a fast
gate plus path-selected backend integration or Chromium E2E. Every `dev` push
runs the complete operational gate once, publishes signed digest-only images,
and deploys development. Release PRs target `main` from a short-lived repository
`release/*` branch; CI verifies that its tree exactly equals the current `dev`
tree and that revision deployed successfully. Create that branch from `main` and
materialize the `dev` tree on it; this preserves linear squash history without
rewriting protected branches. The release workflow checks tree equality,
verifies the dev signatures, promotes the same manifests without rebuilding,
adds the production signature, selects the next semantic patch version (with
reads the committed `.release-version` manifest, validates that it is newer than
the latest semantic release tag, creates the GitHub release/tag, and deploys
production. Pull-request jobs never receive deployment secrets.

Both branches require signed commits, the aggregate Dockerized verification
check, PRs, linear history, resolved conversations, admin enforcement, and
prohibit force-push/deletion. The `development` environment accepts only `dev`;
`production` accepts only `main`. The `monitoring` environment accepts only
`main`. Keep approvals at zero while the repository has one maintainer.

## Dev acceptance and production launch

Verify Access OTP for `jeancollette138@gmail.com`, signup/verification/reset
email, uploads and reads, WebSockets, client-IP rate limiting, TLS/security
headers, backup creation, and isolated restore on dev. No fixed soak or
quarantine delay is required; record this acceptance evidence for the exact
deployed revision before merging the release PR. Resolve or supersede every
failing Dependabot PR before merging the release PR.

For production, confirm a fresh pre-deploy backup and complete a real isolated
restore rehearsal. Verify public health, account email, R2, WebSockets, backup
age, and active-release metadata after deployment. Configure a €10 monthly
Hetzner budget notification in the console; Hetzner does not expose that account
billing control in this Terraform module.

## Recovery

An application deployment failure automatically restarts the previous signed
image digests. Database migrations are forward-only. For an incompatible
migration, stop the affected stack, preserve logs and the failed database, get
explicit operator confirmation, restore the matching pre-deploy Restic snapshot
into a separate database, validate it, and only then replace the affected
volume. Never invoke the disposable smoke suite against production.
