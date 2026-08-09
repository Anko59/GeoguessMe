-- 014 retire the legacy reaction emoji compatibility mechanism.
-- The emoji column, its synchronization trigger, and the constraints that
-- kept emoji and reaction consistent existed only so older application images
-- could read and write reactions during the compatibility window. The
-- two-deployment rollout removed those writers first: the previous revision is
-- no longer running and rollback no longer requires the emoji column.
--
-- The reaction column (message_reactions_pkey, reaction_check) is the stable
-- storage and is untouched. The six legacy emoji values remain valid reaction
-- KEYS in reaction_check because historical reaction rows may still hold them;
-- the API boundary rejects new keys outside the ten canonical ones.
DROP TRIGGER IF EXISTS sync_message_reaction_columns ON message_reactions;

DROP FUNCTION IF EXISTS sync_message_reaction_columns();

ALTER TABLE message_reactions DROP CONSTRAINT IF EXISTS message_reactions_legacy_emoji_key;

ALTER TABLE message_reactions DROP CONSTRAINT IF EXISTS message_reactions_reaction_matches_emoji;

ALTER TABLE message_reactions DROP COLUMN IF EXISTS emoji;
