-- Activate: migrate pre-existing unverified addresses into pending claims.
-- Accounts with email_verified_at set keep their verified address untouched.
-- Unverified accounts move their address from the verified columns into the
-- pending columns so recovery and uniqueness only ever act on confirmed
-- addresses. The statement is deterministic and idempotent: a second run
-- matches no rows because email is already NULL for the affected accounts.
UPDATE users
SET pending_email = email,
    pending_email_normalized = email_normalized,
    email = NULL,
    email_normalized = NULL
WHERE email_verified_at IS NULL AND email IS NOT NULL;

-- Bind verification tokens created by the previous release to the contact
-- claim they were issued for. The column stays nullable during the rolling
-- compatibility window so an older replica can still insert a token; the new
-- binary rejects an unbound token and a resend replaces it with a bound one.
UPDATE email_verification_tokens AS token
SET target_email_normalized = COALESCE(users.pending_email_normalized, users.email_normalized)
FROM users
WHERE token.user_id = users.id AND token.target_email_normalized IS NULL;
