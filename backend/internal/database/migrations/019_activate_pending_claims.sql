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
