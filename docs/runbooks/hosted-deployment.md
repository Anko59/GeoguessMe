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
3. Create a Brevo free account, authenticate `geoguessme.com`, and copy its
   SPF/DKIM records into Terraform-managed DNS before launch. Publish DMARC as
   `p=none` during validation, then tighten after reviewing reports.
4. Create one operator SSH key and separate CI keys for dev and production. Put
   only public keys in `terraform.tfvars`; private CI keys go into their
   matching GitHub environment.
5. Copy `backend.hcl.example` to ignored `backend.hcl`, fill the R2 endpoint,
   and export its S3 credentials plus rotated `HCLOUD_TOKEN` and
   `CLOUDFLARE_API_TOKEN`.

The Cloudflare Terraform token needs zone DNS, R2 bucket, Tunnel, Access apps,
and Access service-token write permissions scoped to this account/zone. The
Hetzner token should be scoped to the dedicated project. Do not reuse either
token in application or deployment jobs.

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
current template.

Use the Access-protected operator SSH route:

```text
ssh -i /path/to/operator-key \
  -o ProxyCommand='make -s cloudflared-access-ssh HOST=%h' \
  ops@deploy.geoguessme.com
```

Export the Terraform Access service-token outputs as `TUNNEL_SERVICE_TOKEN_ID`
and `TUNNEL_SERVICE_TOKEN_SECRET` before using this non-interactive operator
route. Do not place either value on the command line. Read the two generated
public age recipients from `/etc/geoguessme/age/*-recipient.txt`, fill each
environment example with unique database/JWT/metrics/Restic credentials and its
dedicated R2/Brevo values, then run:

```text
make secrets-generate ENV=dev RECIPIENT=age1...
make secrets-generate ENV=production RECIPIENT=age1...
```

Review the encrypted files, commit them, and never commit plaintext dotenv or
age private keys. Unless both GHCR packages are public, add a read-only
`read:packages` token as `GHCR_TOKEN` in each encrypted host environment; it
must have no repository-write scope. Set the Terraform Access service-token
outputs, SSH private key, and SSH known-host line as environment-scoped GitHub
secrets named `CF_ACCESS_CLIENT_ID`, `CF_ACCESS_CLIENT_SECRET`,
`DEPLOY_SSH_PRIVATE_KEY`, and `DEPLOY_SSH_KNOWN_HOSTS`. Put the same two
read-only Access values in the `monitoring` environment; it contains no SSH key
and is restricted to scheduled health checks from `main`.

Dev and production use independent Restic repositories at the `/dev` and
`/production` prefixes of the shared private backup bucket. Keep their
`RESTIC_PASSWORD` values distinct; a restore must use the matching environment
secret file.

## GitHub and release flow

Features merge by verified squash PR into `dev`; pull-request CI runs a fast
gate plus path-selected backend integration or Chromium E2E. Every `dev` push
runs the complete operational gate once, publishes signed digest-only images,
and deploys development. Release PRs merge only `dev` into `main`; CI verifies
that the exact dev revision deployed successfully. The release workflow checks
tree equality, verifies the dev signatures, promotes the same manifests without
rebuilding, adds the production signature, selects the next semantic patch
version (with `v0.2.0` as the launch floor), creates the GitHub release/tag, and
deploys production. Pull-request jobs never receive deployment secrets.

Both branches require signed commits, the aggregate Dockerized verification
check, PRs, linear history, resolved conversations, admin enforcement, and
prohibit force-push/deletion. The `development` environment accepts only `dev`;
`production` accepts only `main`. The `monitoring` environment accepts only
`main`. Keep approvals at zero while the repository has one maintainer.

## Dev acceptance and production launch

Verify Access OTP for `jeancollette138@gmail.com`, signup/verification/reset
email, uploads and reads, WebSockets, client-IP rate limiting, TLS/security
headers, backup creation, and isolated restore on dev. Soak for at least 24
hours. Resolve or supersede every failing Dependabot PR before merging the
release PR.

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
