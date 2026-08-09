-- Retire the compatibility objects introduced by migration 011. The
-- application revision that stopped reading and writing `emoji` was deployed
-- and verified before this forward-only cleanup was created.
DROP TRIGGER IF EXISTS sync_message_reaction_columns ON message_reactions;
DROP FUNCTION IF EXISTS sync_message_reaction_columns();

ALTER TABLE message_reactions DROP CONSTRAINT IF EXISTS message_reactions_legacy_emoji_key;
ALTER TABLE message_reactions DROP CONSTRAINT IF EXISTS message_reactions_reaction_matches_emoji;
ALTER TABLE message_reactions DROP COLUMN IF EXISTS emoji;
