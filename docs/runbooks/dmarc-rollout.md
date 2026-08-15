---
status: primary
---

# DMARC rollout runbook

DMARC is published from `infra/terraform/main.tf` as a TXT record on `_dmarc`.
The initial record stays at `p=none` and must only advance while legitimate,
SPF/DKIM-aligned mail from Brevo continues to deliver to the operator mailbox.

Current record (initial, deployed by this change):

```text
v=DMARC1; p=none; rua=mailto:dmarc@geoguessme.com; adkim=s; aspf=s
```

DMARC reports go to `dmarc@geoguessme.com`, which Cloudflare Email Routing
forwards to the operator mailbox (`var.operator_email`). Confirm those reports
arrive before advancing the policy.

## Enable forwarding safely

Cloudflare will not forward to a destination address until its verification
email has been accepted, so provisioning is deliberately split into two reviewed
applies:

1. Set `operator_email` and leave `enable_dmarc_forwarding = false`. Review and
   apply the plan; this enables Email Routing and requests destination
   verification without creating a broken forwarding rule.
2. Accept Cloudflare's verification email and confirm the address is verified in
   the dashboard.
3. Set `enable_dmarc_forwarding = true`, create and review a fresh plan, then
   apply it to create the `dmarc@geoguessme.com` forwarding rule.
4. Send a test message to `dmarc@geoguessme.com` and confirm it reaches the
   operator mailbox before relying on the DMARC aggregate reports.

## Staged rollout

Advance the `p=` value and `pct=` only after each preceding stage has delivered
legitimate aligned mail for at least seven days. Edit the `content` of the
`cloudflare_dns_record.dmarc` resource in `infra/terraform/main.tf` for each
stage, then run the reviewed plan/apply flow.

| Stage | Policy                  | Condition to advance                                           |
| ----- | ----------------------- | -------------------------------------------------------------- |
| 1     | `p=none` (initial)      | Mail is SPF/DKIM aligned and reaches the inbox; reports arrive |
| 2     | `p=quarantine; pct=25`  | Stage 1 green for 7 days                                       |
| 3     | `p=quarantine; pct=100` | Stage 2 green for 7 days                                       |
| 4     | `p=reject; pct=100`     | Stage 3 green for 7 days                                       |

Do **not** advance while legitimate aligned mail fails delivery or lands in
spam. If any stage breaks legitimate mail, revert to the previous stage and
investigate sender alignment (SPF include in `spf_record`, DKIM in
`brevo_dns_records`, `From:` domain match) before retrying.

## Verification

- After each apply, confirm delivery of a Brevo-sent email to the operator
  mailbox with `Authentication-Results` showing `dkim=pass spf=pass`.
- Confirm aggregate reports (`rua`) keep arriving at `dmarc@geoguessme.com` and
  reach the operator mailbox.
