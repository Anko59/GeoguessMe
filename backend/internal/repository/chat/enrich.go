package chat

import (
	"context"
	"time"

	"geoguessme/internal/models"
)

// EnrichMessagesPageForViewer decorates a messages page with the
// viewer-specific challenge and reaction state shared by every direction
// (latest, forward catch-up, and backward history).
//
// Enrichment is linear in the number of messages plus linked photos: reactions
// and photo states are fetched with one query each and attached through
// indexed maps instead of a nested message scan. Benchmark
// EnrichMessagesPageForViewer in messages_test.go demonstrates the linear
// scaling.
func (r *Repository) EnrichMessagesPageForViewer(ctx context.Context, page MessagesPage, viewerID string) (MessagesPage, error) {
	if len(page.Items) == 0 {
		return page, nil
	}
	if err := r.enrichMessageReactions(ctx, page.Items, viewerID); err != nil {
		return MessagesPage{}, err
	}
	if viewerID == "" {
		return page, nil
	}
	// Index each challenge message by its photo id once, so the per-photo
	// state rows can be attached in constant time instead of rescanning the
	// page for every photo.
	photoIndex := make(map[string]int, len(page.Items))
	photoIDs := make([]string, 0, len(page.Items))
	for index := range page.Items {
		if page.Items[index].Kind == "challenge" && page.Items[index].PhotoID != nil {
			photoIndex[*page.Items[index].PhotoID] = index
			photoIDs = append(photoIDs, *page.Items[index].PhotoID)
		}
	}
	if len(photoIDs) == 0 {
		return page, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT p.id,
			p.expires_at,
			EXTRACT(EPOCH FROM (p.expires_at - p.created_at))::INTEGER,
			CASE
				WHEN p.user_id = $2 THEN 'results'
				WHEN EXISTS (SELECT 1 FROM guesses g WHERE g.photo_id = p.id AND g.user_id = $2) THEN 'guessed'
				WHEN p.expires_at <= NOW() THEN 'expired'
				WHEN EXISTS (SELECT 1 FROM challenge_views v WHERE v.photo_id = p.id AND v.user_id = $2) THEN 'accepted'
				ELSE 'available'
			END AS challenge_status,
			EXISTS (SELECT 1 FROM guesses g WHERE g.photo_id = p.id) AS challenge_resolved
		FROM photos p
		WHERE p.id = ANY($1)`, photoIDs, viewerID)
	if err != nil {
		return MessagesPage{}, err
	}
	defer rows.Close()
	statuses := make(map[string]string, len(photoIDs))
	for rows.Next() {
		var photoID, status string
		var expiresAt time.Time
		var ttlSeconds int64
		var resolved bool
		if err := rows.Scan(&photoID, &expiresAt, &ttlSeconds, &status, &resolved); err != nil {
			return MessagesPage{}, err
		}
		statuses[photoID] = status
		if index, ok := photoIndex[photoID]; ok {
			page.Items[index].ChallengeResolved = resolved
			page.Items[index].ChallengeExpiresAt = &expiresAt
			page.Items[index].ChallengeTTLSeconds = int(ttlSeconds)
		}
	}
	if err := rows.Err(); err != nil {
		return MessagesPage{}, err
	}
	for index := range page.Items {
		if page.Items[index].PhotoID != nil {
			page.Items[index].ChallengeStatus = statuses[*page.Items[index].PhotoID]
		}
	}
	return page, nil
}

// GetMessageForViewer loads a single message with its viewer-specific reaction
// state, or nil when the message does not exist.
func (r *Repository) GetMessageForViewer(ctx context.Context, messageID, viewerID string) (*models.Message, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+messageColumns+` FROM messages m LEFT JOIN users u ON m.user_id = u.id LEFT JOIN chat_media cm ON m.media_id = cm.id WHERE m.id = $1`, messageID)
	if err != nil {
		return nil, err
	}
	messages, err := scanMessageRows(rows)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	if err := r.enrichMessageReactions(ctx, messages, viewerID); err != nil {
		return nil, err
	}
	return &messages[0], nil
}

// GetChallengeMessageForViewer returns the persisted chat message associated
// with a challenge. Guess submission uses it to publish a live resolved-state
// update without creating another message or push notification.
func (r *Repository) GetChallengeMessageForViewer(ctx context.Context, photoID, viewerID string) (*models.Message, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+messageColumns+` FROM messages m LEFT JOIN users u ON m.user_id = u.id LEFT JOIN chat_media cm ON m.media_id = cm.id WHERE m.photo_id = $1 AND m.kind = 'challenge'`, photoID)
	if err != nil {
		return nil, err
	}
	messages, err := scanMessageRows(rows)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	if err := r.enrichMessageReactions(ctx, messages, viewerID); err != nil {
		return nil, err
	}
	return &messages[0], nil
}

// enrichMessageReactions attaches the viewer-specific reaction aggregates to a
// message slice in a single grouped query. The assignment is linear: one pass
// over the result rows and one pass over the messages.
func (r *Repository) enrichMessageReactions(ctx context.Context, messages []models.Message, viewerID string) error {
	messageIDs := make([]string, 0, len(messages))
	for index, message := range messages {
		messages[index].Reactions = []models.Reaction{}
		messageIDs = append(messageIDs, message.ID)
	}
	if len(messageIDs) == 0 {
		return nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT message_id, reaction, COUNT(*)::INTEGER,
			COALESCE(BOOL_OR(user_id = $2), FALSE),
			ARRAY_AGG(u.username ORDER BY u.username)
		FROM message_reactions
		JOIN users u ON u.id = message_reactions.user_id
		WHERE message_id = ANY($1)
		GROUP BY message_id, reaction
		ORDER BY message_id, reaction`, messageIDs, viewerID)
	if err != nil {
		return err
	}
	defer rows.Close()
	reactions := make(map[string][]models.Reaction)
	for rows.Next() {
		var messageID string
		var reaction models.Reaction
		if err := rows.Scan(&messageID, &reaction.Reaction, &reaction.Count, &reaction.Reacted, &reaction.Usernames); err != nil {
			return err
		}
		reaction.Emoji = reaction.Reaction
		reactions[messageID] = append(reactions[messageID], reaction)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for index := range messages {
		if items := reactions[messages[index].ID]; len(items) > 0 {
			messages[index].Reactions = items
		}
	}
	return nil
}
