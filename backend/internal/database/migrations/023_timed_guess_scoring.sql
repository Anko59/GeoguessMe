-- Scoring now includes a time penalty and a server-recorded timeout.
-- Guesses submitted after the guess deadline are recorded as timed-out rows
-- (score 0, timed_out) so they still appear in results.
ALTER TABLE guesses ADD COLUMN IF NOT EXISTS timed_out BOOLEAN NOT NULL DEFAULT FALSE;

-- Migration 021 was released with the previous 2-minute default and already
-- backfilled legacy view rows with it. Recompute exactly those rows with the
-- current 5-minute default (identical expression, only the interval differs);
-- rows capped at the challenge expiry are unchanged by either default and are
-- therefore left alone.
UPDATE challenge_views v
SET guess_expires_at = LEAST(v.view_expires_at + INTERVAL '5 minutes', p.expires_at)
FROM photos p
WHERE p.id = v.photo_id
  AND v.guess_expires_at IS NOT NULL
  AND v.guess_expires_at <> v.view_expires_at + INTERVAL '5 minutes'
  AND v.guess_expires_at = LEAST(v.view_expires_at + INTERVAL '2 minutes', p.expires_at);
