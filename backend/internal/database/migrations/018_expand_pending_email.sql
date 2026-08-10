-- Expand: introduce nullable pending contact-claim columns. A submitted but
-- unverified address lives in pending_email; only a successfully verified
-- address occupies the verified email columns. Making email/email_normalized
-- nullable lets accounts exist without a verified address while the UNIQUE
-- index on email_normalized still guarantees a given verified address belongs
-- to at most one account (PostgreSQL allows multiple NULLs in a unique index).
ALTER TABLE users ADD COLUMN IF NOT EXISTS pending_email TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS pending_email_normalized TEXT;
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users ALTER COLUMN email_normalized DROP NOT NULL;
CREATE INDEX IF NOT EXISTS users_pending_email_normalized_idx ON users(pending_email_normalized);
