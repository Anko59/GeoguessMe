---
status: primary
---

# Personal-data breach response runbook

This runbook covers suspected or confirmed personal-data breaches on the
GeoGuessMe production environment. It implements the operational GDPR duties
recorded in [docs/data-protection.md](../data-protection.md) and complements the
general service incident flow in [docs/operations.md](../operations.md).

## Roles and contacts

| Role                     | Who                          | How                             |
| ------------------------ | ---------------------------- | ------------------------------- |
| Incident lead            | On-call operator             | Pager/alert channel             |
| Privacy lead             | GeoGuessMe operator          | `privacy@geoguessme.com`        |
| Authority notification   | Privacy lead                 | CNIL breach notification portal |
| Hosting security contact | Hetzner / Cloudflare support | Provider console                |

## Definitions

A personal-data breach is a security incident leading to accidental or unlawful
destruction, loss, alteration, unauthorized disclosure of, or access to personal
data. Examples for GeoGuessMe: a database dump leaked, private challenge media
exposed, an admin account compromised, a processor reports a breach affecting
our users.

## Procedure

### 1. Detect and contain (immediately)

- Revoke compromised credentials, rotate secrets per
  [SECURITY.md](../../SECURITY.md) and the hosted-deployment runbook, and stop
  ongoing exfiltration (disable the affected service, block the source).
- Preserve evidence: snapshot logs before they rotate; do not wipe systems
  before the assessment below.

### 2. Assess (target: within 24 hours)

- What data categories were affected (see the register in
  [data-protection.md](../data-protection.md))? Encrypted backups with keys
  intact are usually not a breach; a leaked database is.
- How many users, and which groups or media?
- Is the risk likely to affect rights and freedoms (identity theft, private
  photos of identifiable people, location data)? Challenge photos with people or
  home locations are high-risk by default.
- Record the assessment — including "no personal data affected" outcomes — in
  the breach register.

### 3. Notify the CNIL (within 72 hours when required)

- If the breach is likely to result in a risk to rights and freedoms, the
  privacy lead files the CNIL notification within 72 hours of awareness,
  covering: nature of the breach, categories and approximate number of data
  subjects and records, likely consequences, measures taken or proposed.
- If it cannot be filed in 72 hours, submit the available facts within the
  deadline and complete the notification in phases.
- Documentation for the reasoning (even when not notifying) stays in the breach
  register: the CNIL can request it.

### 4. Inform affected users (without undue delay when high risk)

- When the risk is high, inform affected users in clear language: what happened,
  which data, what we did, what they should do (unique password, device checks),
  and how to contact us.
- Do not disclose details that would help an attacker while containment is
  ongoing unless public disclosure reduces harm.

### 5. Recover and review

- Complete service recovery per [docs/operations.md](../operations.md) and, for
  host-level compromise, the hosted-deployment replacement procedure.
- Within one week, hold a post-incident review: root cause, what slowed
  detection, whether safeguards (rate limits, media access checks, backup
  encryption) need changes, and file the follow-up issues.
- Archive the incident record under `docs/runbooks/` with `status: archival`
  once the review lands; the breach register entry is kept by the operator and
  is not committed to Git.

## Rollback / false alarm

If assessment shows no personal data was affected, document the assessment and
close the incident; no authority or user notification is required. Revisit if
new evidence appears within the retention window of the relevant logs.
