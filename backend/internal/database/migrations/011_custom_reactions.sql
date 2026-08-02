-- Custom reaction artwork replaces the fixed emoji set. Keep the legacy emoji
-- column synchronized during the compatibility window: an older application
-- image must remain usable during deployment and rollback, while newer images
-- read and write the stable reaction key.
ALTER TABLE message_reactions ADD COLUMN IF NOT EXISTS reaction TEXT;
UPDATE message_reactions SET reaction = emoji WHERE reaction IS NULL;
ALTER TABLE message_reactions ALTER COLUMN reaction SET NOT NULL;
ALTER TABLE message_reactions DROP CONSTRAINT IF EXISTS message_reactions_emoji_check;
ALTER TABLE message_reactions DROP CONSTRAINT IF EXISTS message_reactions_pkey;
ALTER TABLE message_reactions ADD CONSTRAINT message_reactions_pkey PRIMARY KEY (message_id, user_id, reaction);
ALTER TABLE message_reactions ADD CONSTRAINT message_reactions_reaction_check CHECK (reaction IN ('like', 'love', 'laugh', 'wow', 'sad', 'spot-on', 'lost', 'mind-blown', 'wrong-way', 'vacation', '👍', '❤️', '😂', '😮', '😢', '🙏'));
ALTER TABLE message_reactions ADD CONSTRAINT message_reactions_reaction_matches_emoji CHECK (reaction = emoji);

CREATE OR REPLACE FUNCTION sync_message_reaction_columns() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.reaction IS NULL THEN
        NEW.reaction := NEW.emoji;
    ELSIF NEW.emoji IS NULL THEN
        NEW.emoji := NEW.reaction;
    ELSIF TG_OP = 'UPDATE' AND NEW.reaction IS DISTINCT FROM OLD.reaction THEN
        NEW.emoji := NEW.reaction;
    ELSIF TG_OP = 'UPDATE' AND NEW.emoji IS DISTINCT FROM OLD.emoji THEN
        NEW.reaction := NEW.emoji;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS sync_message_reaction_columns ON message_reactions;
CREATE TRIGGER sync_message_reaction_columns
BEFORE INSERT OR UPDATE ON message_reactions
FOR EACH ROW EXECUTE FUNCTION sync_message_reaction_columns();

ALTER TABLE message_reactions ADD CONSTRAINT message_reactions_legacy_emoji_key UNIQUE (message_id, user_id, emoji);
CREATE INDEX IF NOT EXISTS message_reactions_message_idx ON message_reactions(message_id);
