package groups

import (
	"geoguessme/internal/database"
)

// Repository is the gameplay persistence collection: group membership and
// notification preferences, group and challenge data, and leaderboard/ranking
// queries. PR 6 moves the group, challenge, guess, delivery, and leaderboard
// slices off the database.DB package global onto methods bound to an injected
// pool, mirroring the internal/repository/chat package from PR 5.
//
// The three responsibilities from the refactor roadmap live in separate files:
// membership/preferences (membership.go), group/challenge data (groups.go and
// challenges.go), and ranking/leaderboard queries (leaderboard.go). Row
// scanning and column definitions have one owner: scan.go.
//
// Instances are independent: two Repositories built on different pools never
// share state.
type Repository struct {
	pool database.Pool
}

// NewRepository returns a Repository bound to the given pool.
func NewRepository(pool database.Pool) *Repository {
	return &Repository{pool: pool}
}
