-- 006_message_replies.sql
-- Preserve an in-group parent message for Messenger-style reply context.
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS reply_to_id TEXT REFERENCES messages(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS messages_reply_to_idx ON messages(reply_to_id) WHERE reply_to_id IS NOT NULL;
