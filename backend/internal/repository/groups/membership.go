package groups

import (
	"context"

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
