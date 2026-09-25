---
status: primary
---

# Cloudflare Access service-token runbook

Three Cloudflare Access applications protect the hosted environment, each with
its own long-lived workflow service token so a leaked token cannot cross
environments:

| Application            | Hostname                     | Used by       | Resource-ID variable   |
| ---------------------- | ---------------------------- | ------------- | ---------------------- |
| GeoGuessMe dev health  | `dev.geoguessme.com`         | `health.yml`  | `dev_health_token_id`  |
| GeoGuessMe dev deploy  | `deploy.geoguessme.com`      | `deploy.yml`  | `dev_deploy_token_id`  |
| GeoGuessMe prod deploy | `deploy-prod.geoguessme.com` | `release.yml` | `prod_deploy_token_id` |

## Tokens never enter Terraform

Terraform references each token only by its Cloudflare resource **ID** (the
creation response's UUID-shaped `id`) through `dev_health_token_id`,
`dev_deploy_token_id`, and `prod_deploy_token_id`. This is distinct from the
`client_id` sent in the `CF-Access-Client-Id` header. The `client_id` and
`client_secret` are created and stored outside Terraform in the matching GitHub
environment secrets. Terraform has no managed service-token resource or
credential outputs, so client credentials cannot leak through state or output.

## Creating a token (90 days)

Create each token through the Cloudflare dashboard (Zero Trust → Access controls
→ Service credentials → Service Tokens) with a 90-day duration. Record all three
returned values separately:

- `id`: the resource UUID used by the matching Terraform `*_token_id` variable;
- `client_id`: the value stored as the matching GitHub environment's
  `CF_ACCESS_CLIENT_ID`;
- `client_secret`: the value stored as `CF_ACCESS_CLIENT_SECRET`.

The client secret is shown only once: treat a lost secret as unrecoverable and
rotate. Never put either client credential in `terraform.tfvars`.

## Local operator access

Before asking for a credential, run `make credentials-preflight`. It reports
only whether the supported local sources are available; it never prints secret
values. The development and production GitHub environment secrets are readable
by their workflows, but GitHub CLI does not reveal their stored values to a
local shell. For repository operations, use
`gh auth status --hostname github.com` to check the local GitHub identity
without displaying its token.

For operator SSH, the local Secret Service keyring item `service=codex-api`,
`provider=cloudflare`, `project=geoguessme`, `name=CLOUDFLARE_API_TOKEN` can
administer Access. The helper also accepts an already-exported
`CLOUDFLARE_API_TOKEN`. The API token needs `Access: Service Tokens Write` and
`Access: Apps and Policies Write`. The operator SSH identity must be loaded in
`ssh-agent`; Docker Compose, `curl`, and `jq` must be available.

Use the supported route:

```text
make credentials-preflight
make ops-ssh HOST=dev
make ops-ssh HOST=production
```

`ops-ssh` creates a one-hour service token and a policy limited to the selected
SSH application, waits until Access accepts it, then opens an SSH session as
`ops`. It removes the policy and token when the session ends. Credentials stay
in process memory or mode-0600 temporary files and are never printed or passed
on a command line. If cleanup fails, the token expires within one hour. For a
non-interactive operation, set `OPS_SSH_COMMAND` to the remote command, for
example:

```text
make ops-ssh HOST=dev OPS_SSH_COMMAND='sudo systemd-tmpfiles --create /etc/tmpfiles.d/geoguessme.conf'
```

The permanent dev and production workflow tokens remain separate and are not
reused by this local operator route.

## Local deployed-dev QA access

The local LLM QA runner does not require a stored browser service token. When
`QA_ACCESS_CLIENT_ID` and `QA_ACCESS_CLIENT_SECRET` are absent, it uses the
existing `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` to discover the
matching Access application, create a one-hour service token and an
application-local policy, and remove both on exit. The API token consequently
needs `Access: Service Tokens Write` and `Access: Apps and Policies Write` for
this workflow. The client credentials exist only in process memory and the
browser container environment; they are not printed, prompted for, or written to
the report. A cleanup failure emits a warning; the service token remains limited
to its one-hour lifetime.

## Piping into GitHub environment secrets

Each token's secret goes directly into its GitHub environment secret, never into
Terraform, Git, a command line, or a ticket:

| Token secret             | GitHub environment | Secret names                                      |
| ------------------------ | ------------------ | ------------------------------------------------- |
| dev health secret        | `monitoring`       | `CF_ACCESS_CLIENT_ID` / `CF_ACCESS_CLIENT_SECRET` |
| dev deployment secret    | `development`      | `CF_ACCESS_CLIENT_ID` / `CF_ACCESS_CLIENT_SECRET` |
| production deploy secret | `production`       | `CF_ACCESS_CLIENT_ID` / `CF_ACCESS_CLIENT_SECRET` |

The resource **IDs** are non-secret and go into
`infra/terraform/terraform.tfvars` (which is gitignored) as
`dev_health_token_id`, `dev_deploy_token_id`, and `prod_deploy_token_id`. Do not
put the distinct `client_id` values there.

Human email-OTP access is managed separately in Terraform. `access_email` is the
operator identity shared by the protected applications; the optional
`dev_access_emails` set adds approved identities to `dev.geoguessme.com` only.
The deployment SSH application at `deploy.geoguessme.com` remains restricted to
the operator identity. Keep the configured values in the ignored
`infra/terraform/terraform.tfvars` file and review the saved Terraform plan
before applying changes.

## Testing intended and cross-application denial

After wiring each token, verify both directions before rotating the old token:

1. Intended access: run each workflow (`health.yml`, `deploy.yml`,
   `release.yml`) and confirm it authenticates through Access and completes.
2. Cross-application denial: take the dev deployment token and try to reach
   `deploy-prod.geoguessme.com`; it must be denied. Repeat with the production
   token against `deploy.geoguessme.com` and the health token against either SSH
   hostname. Only the owner email-OTP policy may reach more than one
   application.

## Revoking the legacy shared annual token

The former single `github` service token (8760h) is removed from Terraform state
with `destroy = false`, so applying this transition does not revoke credentials
that the live workflows still need. Confirm the reviewed plan says the legacy
token will be forgotten without being destroyed. After all three new tokens are
created, piped into GitHub, and the intended/denial tests pass, revoke the
legacy token in the Cloudflare dashboard so no live credential depends on it.

## Expiry metadata, alerts, and rotation

- Record non-secret expiry metadata (token name, resource ID, client ID, and
  created/expiry dates) in the operator's calendar or secrets manager.
- Alert 14 days before each expiry. Rotation is required every 60 days with
  overlap: rotate the token in place with a future expiry for the previous
  secret, pipe the new secret into the matching GitHub environment, verify the
  workflow, then expire the previous secret. An in-place rotation preserves the
  resource ID and client ID, so it requires no Terraform change.
