package chat

import (
	"context"
)

// SetMessageReaction applies a reaction for a user. The message existence is
// verified inside the same statement (the insert targets the messages row via
// a WHERE EXISTS guard), so a stale call for an unknown message is a harmless
// no-op instead of an independent existence query followed by a separate
// mutation. The HTTP handler has already loaded and authorized the message
// before this point; the guard only protects against FK violations for direct
// callers. Reacting twice is idempotent (ON CONFLICT DO NOTHING).
func (r *Repository) SetMessageReaction(ctx context.Context, messageID, userID, reaction string) error {
	if reaction == "" {
		return ErrInvalidReaction
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO message_reactions(message_id, user_id, reaction)
		SELECT $1, $2, $3
		WHERE EXISTS (SELECT 1 FROM messages WHERE id = $1)
		ON CONFLICT (message_id, user_id, reaction) DO NOTHING`, messageID, userID, reaction)
	return err
}

// DeleteMessageReaction removes a reaction for a user. Deleting a reaction for
// an unknown message or an absent reaction is a no-op, so a single DELETE
// statement provides the full result without any independent existence query.
func (r *Repository) DeleteMessageReaction(ctx context.Context, messageID, userID, reaction string) error {
	if reaction == "" {
		return ErrInvalidReaction
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM message_reactions WHERE message_id = $1 AND user_id = $2 AND reaction = $3`, messageID, userID, reaction)
	return err
}
