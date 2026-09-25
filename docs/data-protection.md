# Data protection operations

This is the canonical operational record for GDPR compliance: processing
activities, processors and safeguards, data-subject request (DSR) handling, and
breach response. The public-facing policy is [PRIVACY.md](../PRIVACY.md); the
technical security controls are in
[security-and-privacy](security-and-privacy.md).

## Accountability snapshot

| Item            | Owner / location                                                      |
| --------------- | --------------------------------------------------------------------- |
| Controller      | The GeoGuessMe operator (identity published on the legal-notice page) |
| Privacy contact | `privacy@geoguessme.com` (Cloudflare Email Routing)                   |
| Public policy   | [PRIVACY.md](../PRIVACY.md) → geoguessme.com/privacy                  |
| Terms of use    | geoguessme.com/terms                                                  |
| Legal notice    | geoguessme.com/legal                                                  |
| Breach runbook  | [runbooks/data-breach-response](runbooks/data-breach-response.md)     |

The operator must keep signed data-processing agreements (DPAs) with every
processor below. Each provider publishes its DPA under the account's legal
settings; the operator downloads, reviews, and countersigns it during account
setup and re-checks it at least annually.

## Register of processing activities

| #   | Activity                       | Purpose                                        | Data categories                                                                            | Data subjects                  | Legal basis (Art. 6)          | Retention                                    | Recipients                           |
| --- | ------------------------------ | ---------------------------------------------- | ------------------------------------------------------------------------------------------ | ------------------------------ | ----------------------------- | -------------------------------------------- | ------------------------------------ |
| 1   | Account and session management | Operate accounts, authentication, recovery     | Username, email, password hash, session/token hashes, avatar                               | Players                        | (b) contract                  | Until account deletion; tokens per inventory | Hosting, email provider              |
| 2   | Gameplay                       | Provide the game                               | Group membership, invites, chat, reactions, challenge media, guesses, scores, timing       | Players                        | (b) contract                  | Media 30 days default; scores until deletion | Group members, hosting, storage      |
| 3   | Challenge capture              | Create challenges                              | Camera frames (device only until send), microphone, one-shot location, EXIF-stripped media | Players, bystanders in uploads | (b) contract, kept minimal    | Media 30 days default                        | Group members, hosting, storage      |
| 4   | Web Push notifications         | Deliver opted-in group notifications           | Encrypted endpoint and subscription keys                                                   | Players                        | (a) consent                   | Until disabled or account deletion           | Browser push service                 |
| 5   | Security and abuse prevention  | Rate limiting, fraud/abuse defense, debugging  | IP, user agent, timestamps, request metadata, error data                                   | Visitors, players              | (f) legitimate interests      | Security logs per hosting configuration      | Hosting, network provider            |
| 6   | Transactional email            | Verification, recovery, essential service mail | Email address, message contents                                                            | Players                        | (b) contract                  | Per email provider retention                 | Email provider                       |
| 7   | Optional social sign-in        | Third-party account linking                    | Provider identifier, basic profile claim                                                   | Players                        | (b) contract (user-initiated) | Until unlinking or account deletion          | Keycloak, upstream identity provider |
| 8   | Legal compliance               | Respond to valid requests, defend claims       | Request-specific data                                                                      | Any                            | (c) legal obligation / (f)    | Per legal requirement                        | Authorities on valid request         |

## Processor and vendor matrix

| Vendor                                       | Role                                                | Data location                              | Safeguard                                                       | DPA status expected              |
| -------------------------------------------- | --------------------------------------------------- | ------------------------------------------ | --------------------------------------------------------------- | -------------------------------- |
| Hetzner Cloud                                | Application and database hosting                    | EU (Nuremberg/Falkenstein/Helsinki)        | GDPR applies intra-EU; DPA signed                               | Signed, on file                  |
| Cloudflare                                   | Network edge, DNS, email routing, R2 object storage | Global; R2 location set at bucket creation | EU-U.S. Data Privacy Framework where applicable, otherwise SCCs | Signed, on file                  |
| Brevo                                        | Transactional email                                 | EU (France)                                | GDPR applies intra-EU; DPA signed                               | Signed, on file                  |
| Browser push services (Mozilla/Apple/Google) | Push delivery                                       | Global                                     | Payload end-to-end encrypted; provider cannot read content      | n/a (no readable data)           |
| OpenStreetMap                                | Map tiles                                           | Global CDN                                 | IP address processed for delivery; no account data              | n/a (no personal data beyond IP) |
| Apple / Google                               | App distribution, store compliance                  | Vendor-controlled                          | Store program terms                                             | n/a                              |

Sub-processors change; re-verify this table when a deployment adds or replaces a
vendor and update [PRIVACY.md](../PRIVACY.md) in the same PR.

## International transfers

The application and database run in the EU. Transfers outside the EEA rely on:

1. an adequacy decision (for example the EU-U.S. Data Privacy Framework, when
   the importer is certified and the certification covers the service), or
2. the European Commission's Standard Contractual Clauses in the provider's DPA,
   with the transfer-impact assessment noted in the vendor's account
   documentation.

## Data-subject requests (DSR)

1. **Intake.** Requests arrive at `privacy@geoguessme.com`. Acknowledge within
   72 hours. Record the request (date, type, identity details, response
   deadline) in the DSR log.
2. **Identity verification.** Verify by matching account knowledge (username,
   verified email, recent activity). Never ask for passwords or codes by email.
   If verification fails, refuse with an explanation and note it.
3. **Fulfilment.**
    - Erasure: account self-service deletion is the primary path; verify the
      cascade completed (`media_deletion_jobs`, session cleanup) and confirm to
      the requester. Backups age out on the published rotation.
    - Access/portability: export the account data from the database and media
      store; provide it in a structured, machine-readable format.
    - Rectification: point the user to Settings, or apply the correction
      server-side when the field is not self-service.
    - Restriction/objection: assess against the register's legal bases; document
      the decision.
4. **Deadline.** Respond within one month; a complex or numerous request can
   extend by two further months — inform the requester within the first month.
5. **Closure.** Record the outcome; for refusals, state the reason and the right
   to complain to the CNIL.

## Breach response

Follow [runbooks/data-breach-response](runbooks/data-breach-response.md). In
short: assess within 24 hours; notify the CNIL within 72 hours where the breach
is likely to risk rights and freedoms; inform affected users without undue delay
when the risk is high; record every incident (including false alarms) in the
breach register.
