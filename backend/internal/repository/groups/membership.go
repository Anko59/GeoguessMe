package groups

import (
	"context"
	"database/sql"
	"time"

	"geoguessme/internal/models"
)

// Member is the wire shape of one row of the group members listing.
type Member struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

// IsMember reports whether userID is a member of groupID.
func (r *Repository) IsMember(ctx context.Context, groupID, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`, groupID, userID).Scan(&exists)
	return exists, err
}

// RequireMember returns ErrNotMember unless userID belongs to groupID. It is
// the canonical membership gate every gameplay handler calls so no handler can
// implement a subtly different membership rule.
func (r *Repository) RequireMember(ctx context.Context, groupID, userID string) error {
	member, err := r.IsMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if !member {
		return ErrNotMember
	}
	return nil
}

// AddMember adds a user to a group; a duplicate membership is an idempotent
// success.
func (r *Repository) AddMember(ctx context.Context, member *models.GroupMember) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO group_members (group_id, user_id, joined_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, member.GroupID, member.UserID, member.JoinedAt)
	return err
}

// Members returns every member of groupID ordered by username.
func (r *Repository) Members(ctx context.Context, groupID string) ([]Member, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.username, u.avatar
		FROM users u
		JOIN group_members gm ON u.id = gm.user_id
		WHERE gm.group_id = $1
		ORDER BY u.username
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []Member
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.ID, &member.Username, &member.Avatar); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

// NotificationPreference reports whether a member receives group notifications
// (default true when no explicit preference row exists).
func (r *Repository) NotificationPreference(ctx context.Context, groupID, userID string) (bool, error) {
	var enabled bool
	err := r.pool.QueryRow(ctx, `SELECT COALESCE((SELECT enabled FROM group_notification_preferences WHERE group_id = $1 AND user_id = $2), TRUE)`, groupID, userID).Scan(&enabled)
	return enabled, err
}

// SetNotificationPreference upserts a member's group notification preference.
func (r *Repository) SetNotificationPreference(ctx context.Context, groupID, userID string, enabled bool) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO group_notification_preferences (group_id, user_id, enabled, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (group_id, user_id) DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = CURRENT_TIMESTAMP`, groupID, userID, enabled)
	return err
}

// UserGroups returns the groups a user belongs to, newest first.
func (r *Repository) UserGroups(ctx context.Context, userID string) ([]models.Group, error) {
	rows, err := r.pool.Query(ctx, `SELECT g.id, g.name, g.code, g.created_at FROM groups g JOIN group_members gm ON g.id = gm.group_id WHERE gm.user_id = $1 ORDER BY g.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []models.Group
	for rows.Next() {
		var group models.Group
		if err := rows.Scan(&group.ID, &group.Name, &group.Code, &group.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

// SharesGroup reports whether two users are members of at least one common
// group. A user always shares a group with themself.
func (r *Repository) SharesGroup(ctx context.Context, userA, userB string) (bool, error) {
	if userA == userB {
		return true, nil
	}
	var shared bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM group_members a
			JOIN group_members b ON b.group_id = a.group_id
			WHERE a.user_id = $1 AND b.user_id = $2
		)`, userA, userB).Scan(&shared)
	return shared, err
}

// UserInbox returns one authoritative summary per group the viewer belongs to.
// A member's joined_at is the initial read boundary; subsequent boundaries are
// persisted by MarkInboxRead. Messages authored by the viewer are not unread.
func (r *Repository) UserInbox(ctx context.Context, userID string) ([]models.GroupInbox, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT g.id, g.name,
		       COALESCE((
		           SELECT COUNT(*)
		           FROM messages m
		           LEFT JOIN group_message_reads mr
		             ON mr.group_id = m.group_id AND mr.user_id = $1
		           WHERE m.group_id = g.id
		             AND m.user_id <> $1
		             AND m.created_at > COALESCE(mr.last_read_at, gm.joined_at)
		       ), 0),
		       latest.id, latest.kind, latest.username, latest.created_at
		FROM groups g
		JOIN group_members gm ON gm.group_id = g.id AND gm.user_id = $1
		LEFT JOIN LATERAL (
		    SELECT m.id, m.kind, u.username, m.created_at
		    FROM messages m
		    JOIN users u ON u.id = m.user_id
		    WHERE m.group_id = g.id
		    ORDER BY m.created_at DESC, m.id DESC
		    LIMIT 1
		) latest ON true
		ORDER BY COALESCE(latest.created_at, g.created_at) DESC, g.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	inbox := make([]models.GroupInbox, 0)
	for rows.Next() {
		var item models.GroupInbox
		var unread int64
		var messageID, messageKind, messageUsername sql.NullString
		var messageAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &unread, &messageID, &messageKind, &messageUsername, &messageAt); err != nil {
			return nil, err
		}
		item.UnreadCount = int(unread)
		if messageID.Valid {
			item.LatestMessage = &models.InboxMessageMeta{
				ID: messageID.String, Kind: messageKind.String, Username: messageUsername.String, CreatedAt: messageAt.Time,
			}
		}
		inbox = append(inbox, item)
	}
	return inbox, rows.Err()
}

// MarkInboxRead advances the viewer's read boundary only for a group they
// belong to. The timestamp is supplied by the handler's injected clock so
// tests and deployments have deterministic state transitions.
func (r *Repository) MarkInboxRead(ctx context.Context, groupID, userID string, readAt time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO group_message_reads(group_id, user_id, last_read_at)
		SELECT $1, $2, $3
		WHERE EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)
		ON CONFLICT (group_id, user_id) DO UPDATE
		SET last_read_at = GREATEST(group_message_reads.last_read_at, EXCLUDED.last_read_at)`, groupID, userID, readAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotMember
	}
	return nil
}
