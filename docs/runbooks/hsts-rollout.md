---
status: primary
---

# HSTS rollout runbook

The zone's single authoritative HSTS policy is managed by
`cloudflare_zone_setting.security_header` in `infra/terraform/main.tf`:

```hcl
strict_transport_security = {
  enabled            = true
  include_subdomains = false
  max_age            = 86400
  nosniff            = true
  preload            = false
}
```

This program rolls HSTS out gradually. **Preload is never enabled.**

## Phase 1 (initial, deployed by this change)

- `max_age = 86400` (one day)
- `include_subdomains = false`
- `preload = false`

This lets browsers cache the HSTS policy for a short window so operators and
users can react if anything breaks.

## Phase 2 (after seven green days)

After seven consecutive days of green production health checks (`make smoke` /
the hosted-health workflow) with Phase 1, raise the policy:

```hcl
max_age            = 15552000
include_subdomains = true
```

Apply through the normal reviewed Terraform flow:

```sh
make terraform-fmt-check
make terraform-plan
CONFIRM=apply make terraform-apply
```

## Never preload

Preload submission is intentionally out of scope for this program. If the
operator later considers preload, they must first ensure the zone serves HTTPS
for every subdomain, `include_subdomains = true` has been live for at least 180
days, and the HSTS header is present on every response; then submit through
<https://hstspreload.org> and keep `preload = true` set in the header policy. Do
not enable preload as part of this remediation program.

## Verification

- Confirm the response header after apply:
  `curl -sI https://geoguessme.com | grep -i strict-transport-security`
- Confirm `include_subdomains` is absent during Phase 1 and present after
  Phase 2.
- Confirm dev and production health checks remain green for the full window.
