-- Pin unlock evaluation reads a player's scored, weekly, group, and streak history.
CREATE INDEX IF NOT EXISTS guesses_map_pin_progress_user_score_idx
    ON guesses(user_id, score) WHERE NOT timed_out;
CREATE INDEX IF NOT EXISTS guesses_map_pin_progress_user_created_idx
    ON guesses(user_id, created_at) WHERE NOT timed_out;
CREATE INDEX IF NOT EXISTS public_guesses_map_pin_progress_user_created_idx
    ON public_guesses(user_id, created_at, score) WHERE NOT timed_out;
CREATE INDEX IF NOT EXISTS public_guesses_map_pin_progress_user_score_idx
    ON public_guesses(user_id, score) WHERE NOT timed_out;
CREATE INDEX IF NOT EXISTS guesses_map_pin_group_history_idx
    ON guesses(user_id, group_id) WHERE NOT timed_out;

-- Streak evaluation includes timeout rows because a timeout breaks a streak.
CREATE INDEX IF NOT EXISTS guesses_map_pin_attempt_history_idx
    ON guesses(user_id, created_at, photo_id);
CREATE INDEX IF NOT EXISTS public_guesses_map_pin_attempt_history_idx
    ON public_guesses(user_id, created_at, challenge_id);
