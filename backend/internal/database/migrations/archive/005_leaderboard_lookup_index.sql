-- 005 leaderboard_lookup_index
CREATE INDEX IF NOT EXISTS guesses_group_user_created_idx ON guesses(group_id, user_id, created_at);
