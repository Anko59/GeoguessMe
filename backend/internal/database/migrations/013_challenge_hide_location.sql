-- The poster may hide the challenge location: guessers then only see their
-- distance, and the exact location is revealed on the results page once the
-- configured hide duration (default 48 hours) has passed since creation.
ALTER TABLE photos ADD COLUMN IF NOT EXISTS hide_location BOOLEAN NOT NULL DEFAULT FALSE;
