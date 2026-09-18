package feed

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	page := models.PublicFeedLeaderboardPage{Items: []models.PublicFeedLeaderboardEntry{}}
	query := `WITH totals AS (
		SELECT g.user_id,u.username,SUM(g.score)::bigint AS total_score
		FROM public_guesses g
		JOIN users u ON u.id=g.user_id AND u.deleted_at IS NULL
		GROUP BY g.user_id,u.username
	), ranked AS (
		SELECT user_id,username,total_score,RANK() OVER (ORDER BY total_score DESC) AS rank
		FROM totals
	)
	SELECT user_id,username,total_score,rank FROM ranked `
	args := []any{limit + 1}
	if cursor.UserID != "" {
		query += `WHERE total_score < $1 OR (total_score=$1 AND (username > $2 OR (username=$2 AND user_id > $3))) `
		args = []any{cursor.Score, cursor.Username, cursor.UserID, limit + 1}
	}
	query += `ORDER BY total_score DESC,username ASC,user_id ASC LIMIT $` + strconv.Itoa(len(args))
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry models.PublicFeedLeaderboardEntry
		if err := rows.Scan(&entry.UserID, &entry.Username, &entry.TotalScore, &entry.Rank); err != nil {
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
