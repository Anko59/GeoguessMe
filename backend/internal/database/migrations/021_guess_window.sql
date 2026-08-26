-- The guessing phase is time-boxed: after the private view window ends, a
-- member has GUESS_WINDOW (default 2 minutes) to submit their guess.
-- guess_expires_at is the authoritative server deadline (view end + window,
-- capped at the challenge expiry), so a player who closes the app still
-- misses the window and can never submit a late guess.
ALTER TABLE challenge_views ADD COLUMN IF NOT EXISTS guess_expires_at TIMESTAMPTZ;

-- Existing view rows predate the guess window. Backfill them with the default
-- window measured from their recorded view end (and capped at the challenge
-- expiry, exactly like newly created rows), so the deadline is deterministic
-- for in-flight challenges after the migration.
UPDATE challenge_views v
SET guess_expires_at = LEAST(v.view_expires_at + INTERVAL '2 minutes', p.expires_at)
FROM photos p
WHERE p.id = v.photo_id AND v.guess_expires_at IS NULL;
