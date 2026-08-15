-- Reactions are scoped to a group message and one reaction per emoji/user.
-- The fixed emoji set is validated at the API boundary; this constraint keeps
-- direct database writers from creating malformed reaction records.
CREATE TABLE IF NOT EXISTS message_reactions (
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji TEXT NOT NULL CHECK (emoji IN ('👍', '❤️', '😂', '😮', '😢', '🙏')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (message_id, user_id, emoji)
);

CREATE INDEX IF NOT EXISTS message_reactions_message_idx ON message_reactions(message_id);
