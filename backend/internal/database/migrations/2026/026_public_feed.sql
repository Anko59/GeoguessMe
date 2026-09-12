-- Public posts are opt-in and independent of private group challenges.
CREATE TABLE IF NOT EXISTS public_challenges (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    caption TEXT NOT NULL CHECK (char_length(caption) <= 500),
    storage_key TEXT NOT NULL UNIQUE,
    mime_type TEXT NOT NULL,
    preview BYTEA NOT NULL,
    lat DOUBLE PRECISION NOT NULL CHECK (lat BETWEEN -90 AND 90),
    long DOUBLE PRECISION NOT NULL CHECK (long BETWEEN -180 AND 180),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS public_challenges_feed_idx ON public_challenges(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS public_challenges_author_idx ON public_challenges(user_id);

CREATE TABLE IF NOT EXISTS public_guesses (
    challenge_id TEXT NOT NULL REFERENCES public_challenges(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lat DOUBLE PRECISION NOT NULL CHECK (lat BETWEEN -90 AND 90),
    long DOUBLE PRECISION NOT NULL CHECK (long BETWEEN -180 AND 180),
    score INTEGER NOT NULL CHECK (score BETWEEN 0 AND 5000),
    distance DOUBLE PRECISION NOT NULL CHECK (distance >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (challenge_id, user_id)
);
CREATE INDEX IF NOT EXISTS public_guesses_user_idx ON public_guesses(user_id);

CREATE TABLE IF NOT EXISTS public_reactions (
    challenge_id TEXT NOT NULL REFERENCES public_challenges(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (challenge_id, user_id)
);
CREATE INDEX IF NOT EXISTS public_reactions_user_idx ON public_reactions(user_id);

CREATE TABLE IF NOT EXISTS public_comments (
    id TEXT PRIMARY KEY,
    challenge_id TEXT NOT NULL REFERENCES public_challenges(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL CHECK (char_length(btrim(content)) BETWEEN 1 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS public_comments_page_idx ON public_comments(challenge_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS public_comments_author_idx ON public_comments(user_id);

-- This also runs for account cascades, in the same transaction as the delete.
-- The existing cleanup worker owns retries and physical object removal.
CREATE OR REPLACE FUNCTION enqueue_public_challenge_media() RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO media_deletion_jobs(id, storage_key, source)
    VALUES (gen_random_uuid()::text, OLD.storage_key, 'manual')
    ON CONFLICT (storage_key) WHERE completed_at IS NULL DO NOTHING;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS public_challenge_media_deletion ON public_challenges;
CREATE TRIGGER public_challenge_media_deletion
BEFORE DELETE ON public_challenges
FOR EACH ROW EXECUTE FUNCTION enqueue_public_challenge_media();
