-- Result pages seek by challenge and then rank by the immutable score.
CREATE INDEX IF NOT EXISTS public_guesses_results_idx
    ON public_guesses(challenge_id, score DESC, created_at ASC, user_id ASC);
