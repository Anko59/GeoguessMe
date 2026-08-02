// Package progression contains the server-owned rules for lifetime player
// progression. Keeping the thresholds here makes every client consume the
// same rank calculation instead of recreating it from display data.
package progression

type rankDefinition struct {
	name      string
	minPoints int
	trophyKey string
}

// Rank is the public progression state for a player. Points in a rank are
// lifetime guess points earned after reaching the rank's threshold.
type Rank struct {
	Level           int    `json:"level"`
	Name            string `json:"name"`
	MinPoints       int    `json:"min_points"`
	NextPoints      *int   `json:"next_points,omitempty"`
	PointsInRank    int    `json:"points_in_rank"`
	PointsToNext    int    `json:"points_to_next"`
	ProgressPercent int    `json:"progress_percent"`
	TrophyKey       string `json:"trophy_key"`
	// Next is the following rank, or nil at the highest rank. It lets clients
	// render the next rank's name and badge without duplicating the table.
	Next *Rank `json:"next_rank,omitempty"`
}

var rankDefinitions = [...]rankDefinition{
	{name: "Page", minPoints: 0, trophyKey: "page"},
	{name: "Squire", minPoints: 500, trophyKey: "squire"},
	{name: "Yeoman", minPoints: 1500, trophyKey: "yeoman"},
	{name: "Herald", minPoints: 3000, trophyKey: "herald"},
	{name: "Knight Errant", minPoints: 5000, trophyKey: "knight-errant"},
	{name: "Knight", minPoints: 7500, trophyKey: "knight"},
	{name: "Banneret", minPoints: 10000, trophyKey: "banneret"},
	{name: "Castellan", minPoints: 15000, trophyKey: "castellan"},
	{name: "Baron", minPoints: 20000, trophyKey: "baron"},
	{name: "Viscount", minPoints: 30000, trophyKey: "viscount"},
	{name: "Count", minPoints: 45000, trophyKey: "count"},
	{name: "Earl", minPoints: 65000, trophyKey: "earl"},
	{name: "Marquess", minPoints: 90000, trophyKey: "marquess"},
	{name: "Duke", minPoints: 120000, trophyKey: "duke"},
	{name: "Grand Duke", minPoints: 160000, trophyKey: "grand-duke"},
	{name: "Prince", minPoints: 220000, trophyKey: "prince"},
	{name: "Regent", minPoints: 300000, trophyKey: "regent"},
	{name: "Sovereign", minPoints: 400000, trophyKey: "sovereign"},
	{name: "High King", minPoints: 550000, trophyKey: "high-king"},
	{name: "Emperor", minPoints: 750000, trophyKey: "emperor"},
}

// RankCount is useful to clients that want to validate a rank level without
// duplicating the rank table.
const RankCount = len(rankDefinitions)

// RankForPoints returns the rank and progress for a lifetime point total.
// Negative totals are treated as zero so malformed historical data cannot
// produce a negative progress bar.
func RankForPoints(totalPoints int) Rank {
	if totalPoints < 0 {
		totalPoints = 0
	}
	index := 0
	for next := 1; next < len(rankDefinitions); next++ {
		if totalPoints < rankDefinitions[next].minPoints {
			break
		}
		index = next
	}
	definition := rankDefinitions[index]
	rank := Rank{Level: index + 1, Name: definition.name, MinPoints: definition.minPoints, TrophyKey: definition.trophyKey}
	rank.PointsInRank = totalPoints - definition.minPoints
	if index == len(rankDefinitions)-1 {
		rank.ProgressPercent = 100
		return rank
	}
	next := rankDefinitions[index+1]
	rank.Next = &Rank{Level: index + 2, Name: next.name, MinPoints: next.minPoints, TrophyKey: next.trophyKey}
	nextPoints := next.minPoints
	rank.NextPoints = &nextPoints
	rank.PointsToNext = nextPoints - definition.minPoints
	rank.ProgressPercent = rank.PointsInRank * 100 / rank.PointsToNext
	if rank.ProgressPercent > 100 {
		rank.ProgressPercent = 100
	}
	return rank
}
