-- 007_chat_media.sql
-- Keep ordinary chat attachments independent from challenge photos: they have
-- different authorization and retention semantics and must never expose map
-- locations or challenge lifecycle data.
CREATE TABLE IF NOT EXISTS chat_media (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storage_key TEXT NOT NULL UNIQUE,
    mime_type TEXT NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'video/mp4', 'video/webm')),
    byte_size BIGINT NOT NULL CHECK (byte_size > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_kind_check;
ALTER TABLE messages
    ADD CONSTRAINT messages_kind_check CHECK (kind IN ('text', 'challenge', 'media', 'system'));
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS media_id TEXT REFERENCES chat_media(id) ON DELETE SET NULL;
CREATE UNIQUE INDEX IF NOT EXISTS messages_media_id_key ON messages(media_id) WHERE media_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS chat_media_group_created_idx ON chat_media(group_id, created_at);
