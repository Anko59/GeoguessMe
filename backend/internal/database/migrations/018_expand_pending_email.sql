-- Expand: introduce nullable pending contact-claim columns. A submitted but
-- unverified address lives in pending_email; only a successfully verified
-- address occupies the verified email columns. Making email/email_normalized
-- nullable lets accounts exist without a verified address while the UNIQUE
-- index on email_normalized still guarantees a given verified address belongs
-- to at most one account (PostgreSQL allows multiple NULLs in a unique index).
ALTER TABLE users ADD COLUMN IF NOT EXISTS pending_email TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS pending_email_normalized TEXT;
ALTER TABLE email_verification_tokens ADD COLUMN IF NOT EXISTS target_email_normalized TEXT;
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users ALTER COLUMN email_normalized DROP NOT NULL;
CREATE INDEX IF NOT EXISTS users_pending_email_normalized_idx ON users(pending_email_normalized);

-- Enforce the replacement semantics used by verification and recovery token
-- issuance even when two requests race. Preserve only the newest pre-existing
-- unused token before adding the partial unique indexes.
WITH ranked AS (
    SELECT
id,
        ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY created_at DESC, id DESC) AS token_rank
    FROM email_verification_tokens
    WHERE used_at IS NULL
)

DELETE FROM email_verification_tokens AS token
USING ranked
WHERE token.id = ranked.id AND ranked.token_rank > 1;

WITH ranked AS (
    SELECT
id,
        ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY created_at DESC, id DESC) AS token_rank
    FROM password_reset_tokens
    WHERE used_at IS NULL
)

DELETE FROM password_reset_tokens AS token
USING ranked
WHERE token.id = ranked.id AND ranked.token_rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS email_verification_one_unused_per_user_idx
ON email_verification_tokens(user_id) WHERE used_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS password_reset_one_unused_per_user_idx
ON password_reset_tokens(user_id) WHERE used_at IS NULL;
