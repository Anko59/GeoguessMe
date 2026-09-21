package feed

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"geoguessme/internal/models"
)

type LeaderboardCursor struct {
	Score    int    `json:"score"`
	Username string `json:"username"`
	UserID   string `json:"user_id"`
}

func ParseLeaderboardCursor(value string) (LeaderboardCursor, error) {
	var cursor LeaderboardCursor
	if value == "" {
		return cursor, nil
	}
	if len(value) > 256 {
		return cursor, ErrInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Username == "" || cursor.UserID == "" {
		return LeaderboardCursor{}, ErrInvalidCursor
	}
	return cursor, nil
}

func encodeLeaderboardCursor(entry models.PublicFeedLeaderboardEntry) string {
	data, _ := json.Marshal(LeaderboardCursor{Score: entry.TotalScore, Username: entry.Username, UserID: entry.UserID})
	return base64.RawURLEncoding.EncodeToString(data)
}

func (r *Repository) Leaderboard(ctx context.Context, cursor LeaderboardCursor, limit int) (models.PublicFeedLeaderboardPage, error) {
	return r.loadLeaderboard(ctx, cursor, limit, `WITH totals AS (
		SELECT g.user_id,u.username,u.avatar,SUM(g.score)::bigint AS total_score
		FROM public_guesses g
		JOIN users u ON u.id=g.user_id AND u.deleted_at IS NULL
		GROUP BY g.user_id,u.username,u.avatar
	), ranked AS (
		SELECT user_id,username,avatar,total_score,RANK() OVER (ORDER BY total_score DESC) AS rank
		FROM totals
	)
	SELECT user_id,username,avatar,total_score,rank FROM ranked `, nil)
}

// ProfileLeaderboard ranks players by their guesses on a target player's feed
// challenges. The target's visible posts are filtered with challengeVisibility
// using the authenticated viewer, and the target itself is excluded even if a
// legacy row ever exists in public_guesses.
func (r *Repository) ProfileLeaderboard(ctx context.Context, target, viewer string, cursor LeaderboardCursor, limit int) (models.PublicFeedLeaderboardPage, error) {
	if err := r.authorizeProfileViewer(ctx, target, viewer); err != nil {
		return models.PublicFeedLeaderboardPage{Items: []models.PublicFeedLeaderboardEntry{}}, err
	}
	return r.loadLeaderboard(ctx, cursor, limit, `WITH totals AS (
		SELECT g.user_id,u.username,u.avatar,SUM(g.score)::bigint AS total_score
		FROM public_guesses g
		JOIN public_challenges p ON p.id=g.challenge_id
		JOIN users u ON u.id=g.user_id AND u.deleted_at IS NULL
		WHERE p.user_id=$2 AND g.user_id<>$2 AND `+challengeVisibility+`
		GROUP BY g.user_id,u.username,u.avatar
	), ranked AS (
		SELECT user_id,username,avatar,total_score,RANK() OVER (ORDER BY total_score DESC) AS rank
		FROM totals
	)
	SELECT user_id,username,avatar,total_score,rank FROM ranked `, []any{viewer, target})
}

func (r *Repository) loadLeaderboard(ctx context.Context, cursor LeaderboardCursor, limit int, query string, args []any) (models.PublicFeedLeaderboardPage, error) {
	page := models.PublicFeedLeaderboardPage{Items: []models.PublicFeedLeaderboardEntry{}}
	if cursor.UserID != "" {
		first := len(args) + 1
		query += fmt.Sprintf(`WHERE total_score < $%d OR (total_score=$%d AND (username > $%d OR (username=$%d AND user_id > $%d))) `, first, first, first+1, first+1, first+2)
		args = append(args, cursor.Score, cursor.Username, cursor.UserID)
	}
	limitPosition := len(args) + 1
	args = append(args, limit+1)
	query += `ORDER BY total_score DESC,username ASC,user_id ASC LIMIT $` + strconv.Itoa(limitPosition)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry models.PublicFeedLeaderboardEntry
		if err := rows.Scan(&entry.UserID, &entry.Username, &entry.Avatar, &entry.TotalScore, &entry.Rank); err != nil {
			return page, err
		}
		page.Items = append(page.Items, entry)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = encodeLeaderboardCursor(page.Items[limit-1])
	}
	return page, nil
}
