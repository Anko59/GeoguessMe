-- 026 group_challenge_globe_index
CREATE INDEX IF NOT EXISTS photos_group_created_id_idx
ON photos (group_id, created_at DESC, id DESC);
