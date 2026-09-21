-- Public feed timed games have their own view lifecycle.  Keeping this table
-- separate from challenge_views prevents a feed challenge from inheriting a
-- group's membership or photo-expiry rules.
CREATE TABLE IF NOT EXISTS public_challenge_views (
    challenge_id TEXT NOT NULL REFERENCES public_challenges(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    accepted_at TIMESTAMPTZ NOT NULL,
    media_delivered_at TIMESTAMPTZ,
    view_expires_at TIMESTAMPTZ NOT NULL,
    guess_expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (challenge_id, user_id)
);
CREATE INDEX IF NOT EXISTS public_challenge_views_deadline_idx
    ON public_challenge_views(user_id, guess_expires_at);

-- Existing public guesses remain valid.  The identifier is useful to the
-- group-style timed-game response, while the timeout marker distinguishes an
-- authoritative expiry from a real coordinate guess.
ALTER TABLE public_guesses ADD COLUMN IF NOT EXISTS id TEXT;
UPDATE public_guesses SET id = gen_random_uuid()::text WHERE id IS NULL;
ALTER TABLE public_guesses ALTER COLUMN id SET DEFAULT gen_random_uuid()::text;
ALTER TABLE public_guesses ALTER COLUMN id SET NOT NULL;
ALTER TABLE public_guesses ADD COLUMN IF NOT EXISTS timed_out BOOLEAN NOT NULL DEFAULT FALSE;
CREATE UNIQUE INDEX IF NOT EXISTS public_guesses_id_idx ON public_guesses(id);
