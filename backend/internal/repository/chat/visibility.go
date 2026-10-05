package chat

import (
	"context"
	"fmt"
	"strings"

	"geoguessme/internal/models"
)

// messageVisibility is applied before ordering and LIMIT, never after paging.
func messageVisibility(viewerParam string) string {
	return `NOT EXISTS (SELECT 1 FROM user_blocks b WHERE
		(b.blocker_id = ` + viewerParam + ` AND b.blocked_id = m.user_id)
		OR (b.blocker_id = m.user_id AND b.blocked_id = ` + viewerParam + `))`
}

// viewerMessageQuery preserves trusted internal reads without a viewer while
// adding both sender visibility and reply-link redaction to authenticated reads.
func viewerMessageQuery(query string, args []any, viewers ...string) (string, []any) {
	if len(viewers) == 0 || viewers[0] == "" {
		return query, args
	}
	param := fmt.Sprintf("$%d", len(args)+1)
	query = strings.Replace(query, "m.reply_to_id,", `CASE WHEN EXISTS (
		SELECT 1 FROM messages reply JOIN user_blocks b ON
		(b.blocker_id = `+param+` AND b.blocked_id = reply.user_id)
		OR (b.blocker_id = reply.user_id AND b.blocked_id = `+param+`)
		WHERE reply.id = m.reply_to_id) THEN NULL ELSE m.reply_to_id END,`, 1)
	boundary := len(query)
	for _, suffix := range []string{" GROUP BY", " ORDER BY", " LIMIT"} {
		if index := strings.Index(query, suffix); index >= 0 && index < boundary {
			boundary = index
		}
	}
	query = query[:boundary] + " AND " + messageVisibility(param) + query[boundary:]
	return query, append(args, viewers[0])
}

// GetGroupMessagesPageBeforeForViewer is the authenticated history path.
func (r *Repository) GetGroupMessagesPageBeforeForViewer(ctx context.Context, groupID, beforeID string, limit int, viewerID string) (MessagesPage, error) {
	page, err := r.GetGroupMessagesPageBefore(ctx, groupID, beforeID, limit, viewerID)
	if err != nil {
		return page, err
	}
	return r.EnrichMessagesPageForViewer(ctx, page, viewerID)
}

// GetChatMediaForViewer hides attachments whose message author is blocked in
// either direction. The caller must additionally authorize group membership.
func (r *Repository) GetChatMediaForViewer(ctx context.Context, mediaID, viewerID string) (*models.ChatMedia, error) {
	return r.getChatMedia(ctx, mediaID, viewerID)
}

// ReactionUsageForGroupForViewer excludes blocked messages and contributors.
func (r *Repository) ReactionUsageForGroupForViewer(ctx context.Context, groupID, viewerID string) ([]ReactionUsage, error) {
	return r.ReactionUsageForGroup(ctx, groupID, viewerID)
}
