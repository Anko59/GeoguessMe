-- Aggregate feed scores by user without scanning every user's guesses.
CREATE INDEX IF NOT EXISTS public_guesses_leaderboard_user_idx
    ON public_guesses(user_id, score DESC);
