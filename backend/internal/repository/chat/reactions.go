package chat

import "context"

// ReactionUsage is the number of times a reaction has been selected in a
// group. It is used to order the reaction picker by the group's actual usage.
type ReactionUsage struct {
	Reaction string `json:"reaction"`
	Count    int    `json:"count"`
}

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

// ReactionUsageForGroup returns reaction counts across the complete group
// history, ordered by popularity and then key for deterministic ties.
func (r *Repository) ReactionUsageForGroup(ctx context.Context, groupID string) ([]ReactionUsage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT mr.reaction, COUNT(*)::INTEGER
		FROM message_reactions mr
		JOIN messages m ON m.id = mr.message_id
		WHERE m.group_id = $1
		GROUP BY mr.reaction
		ORDER BY COUNT(*) DESC, mr.reaction ASC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	usage := make([]ReactionUsage, 0)
	for rows.Next() {
		var item ReactionUsage
		if err := rows.Scan(&item.Reaction, &item.Count); err != nil {
			return nil, err
		}
		usage = append(usage, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return usage, nil
}
