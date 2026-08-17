-- Expand the named custom reaction set while preserving legacy emoji rows.
-- The API allowlist and OpenAPI ReactionKey enum are updated in the same
-- release. Existing reaction rows remain valid during the constraint swap.
ALTER TABLE message_reactions DROP CONSTRAINT IF EXISTS message_reactions_reaction_check;
ALTER TABLE message_reactions ADD CONSTRAINT message_reactions_reaction_check CHECK (
    reaction IN (
        'like', 'love', 'laugh', 'wow', 'sad', 'spot-on', 'lost',
        'mind-blown', 'wrong-way', 'vacation', 'dislike', 'cry', 'kiss',
        'wink', 'grin', 'heart-eyes', 'sunglasses', 'angry', 'confused',
        'sleepy', 'clap', 'pray', 'fire', 'party',
        '👍', '❤️', '😂', '😮', '😢', '🙏'
    )
);
