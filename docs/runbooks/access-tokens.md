---
status: primary
---

# Cloudflare Access service-token runbook

Three Cloudflare Access applications protect the hosted environment, each with
its own 90-day service token so a leaked token cannot cross environments:

| Application            | Hostname                     | Used by       | Token variable         |
| ---------------------- | ---------------------------- | ------------- | ---------------------- |
| GeoGuessMe dev health  | `dev.geoguessme.com`         | `health.yml`  | `dev_health_token_id`  |
| GeoGuessMe dev deploy  | `deploy.geoguessme.com`      | `deploy.yml`  | `dev_deploy_token_id`  |
| GeoGuessMe prod deploy | `deploy-prod.geoguessme.com` | `release.yml` | `prod_deploy_token_id` |

## Tokens never enter Terraform

Terraform references each token only by its **client ID** through the variables
`dev_health_token_id`, `dev_deploy_token_id`, and `prod_deploy_token_id`. The
corresponding **secrets** (client secrets) are created and stored entirely
outside Terraform: in the matching GitHub environment secrets. Terraform has no
`cloudflare_zero_trust_access_service_token` resource and exposes no
service-token outputs, so secrets cannot leak through state or output.

## Creating a token (90 days)

Create each token through the Cloudflare dashboard (Zero Trust → Access →
Service Auth → Service tokens) or the API:

```sh
cloudflared access token create \
  --account-id "$CLOUDFLARE_ACCOUNT_ID" \
  --name "geoguessme-dev-health" \
  --duration 2160h   # 90 days
```

Record the returned client ID and secret. The secret is shown only once: treat a
lost secret as unrecoverable and rotate.

## Piping into GitHub environment secrets

Each token's secret goes directly into its GitHub environment secret, never into
Terraform, Git, a command line, or a ticket:

| Token secret             | GitHub environment | Secret names                                      |
| ------------------------ | ------------------ | ------------------------------------------------- |
| dev health secret        | `monitoring`       | `CF_ACCESS_CLIENT_ID` / `CF_ACCESS_CLIENT_SECRET` |
| dev deployment secret    | `development`      | `CF_ACCESS_CLIENT_ID` / `CF_ACCESS_CLIENT_SECRET` |
| production deploy secret | `production`       | `CF_ACCESS_CLIENT_ID` / `CF_ACCESS_CLIENT_SECRET` |

The client **IDs** are non-secret and go into `infra/terraform/terraform.tfvars`
(which is gitignored) as `dev_health_token_id`, `dev_deploy_token_id`, and
`prod_deploy_token_id`.

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

The former single `github` service token
(`cloudflare_zero_trust_access_service_token.github`, 8760h) is removed from
Terraform in this change. After all three new tokens are created, piped into
GitHub, and the intended/denial tests pass, revoke the legacy token in the
Cloudflare dashboard so no live credential depends on it.

## Expiry metadata, alerts, and rotation

- Record non-secret expiry metadata (token name, client ID, created/expiry
  dates) in the operator's calendar or secrets manager.
- Alert 14 days before each expiry. Rotation is required every 60 days with
  overlap: create the replacement, pipe its secret into the GitHub environment,
  update `terraform.tfvars` with the new client ID, `make terraform-plan`,
  apply, verify a deployment, then revoke the old token.
