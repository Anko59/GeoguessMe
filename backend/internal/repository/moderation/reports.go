package moderation

import (
	"context"
	"database/sql"
	"errors"

	"geoguessme/internal/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrReportTargetUnavailable deliberately does not distinguish a missing target
// from one outside the requester's groups.
var ErrReportTargetUnavailable = errors.New("report target unavailable")

type ContentReport struct {
	ID             string `json:"id"`
	ReportedUserID string `json:"-"`
	Created        bool   `json:"-"`
}

type Repository struct{ pool database.Pool }

func NewRepository(pool database.Pool) *Repository { return &Repository{pool: pool} }

// CreateContentReport checks current group membership and persists a notice in
// one statement. A concurrent duplicate returns the original ID and never
// sends a second notification. Target IDs remain immutable even if source
// content is later deleted.
func (r *Repository) CreateContentReport(ctx context.Context, reporterID, kind, targetID, reason, details string) (ContentReport, error) {
	if kind != "user" && kind != "message" {
		return ContentReport{}, ErrReportTargetUnavailable
	}
	var report ContentReport
	query := `WITH eligible AS (
		SELECT u.id AS reported_user_id, NULL::text AS message_id
		FROM users u
		WHERE $2 = 'user' AND u.id = $3 AND u.id <> $1 AND u.deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM group_members mine JOIN group_members theirs
		              ON theirs.group_id = mine.group_id
		              WHERE mine.user_id = $1 AND theirs.user_id = u.id)
		UNION ALL
		SELECT m.user_id, m.id FROM messages m
		JOIN users u ON u.id = m.user_id AND u.deleted_at IS NULL
		JOIN group_members gm ON gm.group_id = m.group_id AND gm.user_id = $1
		WHERE $2 = 'message' AND m.id = $3 AND m.user_id <> $1
	), inserted AS (
		INSERT INTO content_reports (id, reporter_id, reported_user_id, message_id, target_kind, target_id, reason, details)
		SELECT $6, $1, reported_user_id, message_id, $2, $3, $4, $5 FROM eligible
		ON CONFLICT (reporter_id, target_kind, target_id) DO NOTHING
		RETURNING id
	)
	SELECT COALESCE((SELECT id FROM inserted), existing.id), eligible.reported_user_id,
	       EXISTS (SELECT 1 FROM inserted)
	FROM eligible LEFT JOIN content_reports existing
	  ON existing.reporter_id = $1 AND existing.target_kind = $2 AND existing.target_id = $3`
	var id sql.NullString
	err := r.pool.QueryRow(ctx, query, reporterID, kind, targetID, reason, details, uuid.NewString()).Scan(&id, &report.ReportedUserID, &report.Created)
	if errors.Is(err, pgx.ErrNoRows) {
		return ContentReport{}, ErrReportTargetUnavailable
	}
	if err != nil {
		return ContentReport{}, err
	}
	if id.Valid {
		report.ID = id.String
		return report, nil
	}
	// A concurrent transaction can commit its conflict after our statement's
	// snapshot was taken. Resolve the now-visible winning row in a new snapshot.
	err = r.pool.QueryRow(ctx, `SELECT id FROM content_reports WHERE reporter_id = $1 AND target_kind = $2 AND target_id = $3`, reporterID, kind, targetID).Scan(&report.ID)
	return report, err
}
