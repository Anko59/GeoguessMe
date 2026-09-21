package models

import "time"

// PublicChallenge contains only feed-safe fields. Coordinates and storage keys
// are deliberately absent; only the guess result can reveal the answer.
type PublicChallenge struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Username      string    `json:"username"`
	Avatar        string    `json:"avatar"`
	Caption       string    `json:"caption"`
	Audience      string    `json:"audience"`
	CreatedAt     time.Time `json:"created_at"`
	IsOwner       bool      `json:"is_owner"`
	Resolved      bool      `json:"resolved"`
	ReactionCount int       `json:"reaction_count"`
	Reacted       bool      `json:"reacted"`
	CommentCount  int       `json:"comment_count"`
}

type PublicFeedPage struct {
	Items      []PublicChallenge `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

type PublicFeedLeaderboardEntry struct {
	Rank       int    `json:"rank"`
	UserID     string `json:"user_id"`
	Username   string `json:"username"`
	Avatar     string `json:"avatar"`
	TotalScore int    `json:"total_score"`
}

type PublicFeedLeaderboardPage struct {
	Items      []PublicFeedLeaderboardEntry `json:"items"`
	NextCursor string                       `json:"next_cursor"`
}

type PublicComment struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	CanDelete bool      `json:"can_delete"`
}

type PublicCommentsPage struct {
	Items      []PublicComment `json:"items"`
	NextCursor string          `json:"next_cursor"`
}

type PublicGuessResult struct {
	Score      int     `json:"score"`
	Distance   float64 `json:"distance"`
	Lat        float64 `json:"lat"`
	Long       float64 `json:"long"`
	ActualLat  float64 `json:"actual_lat"`
	ActualLong float64 `json:"actual_long"`
}

// PublicChallengeAccepted and PublicChallengeMediaDelivered mirror the
// group-game timing contract while keeping feed challenges independent from
// group photos and membership.
type PublicChallengeAccepted struct {
	ChallengeID       string    `json:"challenge_id"`
	MediaURL          string    `json:"media_url"`
	MediaType         string    `json:"media_type"`
	AcceptedAt        time.Time `json:"accepted_at"`
	ViewExpiresAt     time.Time `json:"view_expires_at"`
	GuessAfter        time.Time `json:"guess_after"`
	GuessExpiresAt    time.Time `json:"guess_expires_at"`
	ScoreGraceSeconds int       `json:"score_grace_seconds"`
	ServerTime        time.Time `json:"server_time"`
}

type PublicChallengeMediaDelivered struct {
	ViewExpiresAt     time.Time `json:"view_expires_at"`
	GuessAfter        time.Time `json:"guess_after"`
	GuessExpiresAt    time.Time `json:"guess_expires_at"`
	ScoreGraceSeconds int       `json:"score_grace_seconds"`
	ServerTime        time.Time `json:"server_time"`
}

type PublicTimedGuessResponse struct {
	GuessID     string    `json:"guess_id"`
	ChallengeID string    `json:"challenge_id"`
	Score       int       `json:"score"`
	Distance    *float64  `json:"distance,omitempty"`
	TimedOut    bool      `json:"timed_out"`
	CreatedAt   time.Time `json:"created_at"`
	Duplicate   bool      `json:"duplicate"`
	ServerTime  time.Time `json:"server_time"`
}

type PublicTimedResultGuess struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar"`
	Lat       *float64  `json:"lat,omitempty"`
	Long      *float64  `json:"long,omitempty"`
	Score     int       `json:"score"`
	Distance  *float64  `json:"distance,omitempty"`
	TimedOut  bool      `json:"timed_out"`
	CreatedAt time.Time `json:"created_at"`
}

type PublicTimedResults struct {
	ChallengeID string                   `json:"challenge_id"`
	ActualLat   float64                  `json:"actual_lat"`
	ActualLong  float64                  `json:"actual_long"`
	Guesses     []PublicTimedResultGuess `json:"guesses"`
	ServerTime  time.Time                `json:"server_time"`
}

type PublicFeedResult struct {
	Rank     int     `json:"rank"`
	UserID   string  `json:"user_id"`
	Username string  `json:"username"`
	Avatar   string  `json:"avatar"`
	Score    int     `json:"score"`
	Distance float64 `json:"distance"`
	EloDelta int     `json:"elo_delta"`
	IsViewer bool    `json:"is_viewer"`
}
