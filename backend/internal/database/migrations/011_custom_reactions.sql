-- Custom reaction artwork replaces the fixed emoji set. The reaction column
-- stores a stable reaction key; the six legacy emoji values are preserved so
-- existing reactions keep working and remain toggleable.
ALTER TABLE message_reactions ADD COLUMN IF NOT EXISTS reaction TEXT;
UPDATE message_reactions SET reaction = emoji WHERE reaction IS NULL;
ALTER TABLE message_reactions ALTER COLUMN reaction SET NOT NULL;
ALTER TABLE message_reactions DROP CONSTRAINT IF EXISTS message_reactions_emoji_check;
ALTER TABLE message_reactions DROP CONSTRAINT IF EXISTS message_reactions_pkey;
ALTER TABLE message_reactions ADD CONSTRAINT message_reactions_pkey PRIMARY KEY (message_id, user_id, reaction);
ALTER TABLE message_reactions ADD CONSTRAINT message_reactions_reaction_check CHECK (reaction IN ('like', 'love', 'laugh', 'wow', 'sad', 'spot-on', 'lost', 'mind-blown', 'wrong-way', 'vacation', '👍', '❤️', '😂', '😮', '😢', '🙏'));
ALTER TABLE message_reactions DROP COLUMN IF EXISTS emoji;
CREATE INDEX IF NOT EXISTS message_reactions_message_idx ON message_reactions(message_id);
