-- Scoring now includes a time penalty and a server-recorded timeout.
-- Guesses submitted after the guess deadline are recorded as timed-out rows
-- (score 0, timed_out) so they still appear in results.
ALTER TABLE guesses ADD COLUMN IF NOT EXISTS timed_out BOOLEAN NOT NULL DEFAULT FALSE;
