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
	{name: "Completely Lost", minPoints: 0, trophyKey: "completely-lost"},
	{name: "Lost Tourist", minPoints: 5000, trophyKey: "lost-tourist"},
	{name: "Clueless Wanderer", minPoints: 15000, trophyKey: "clueless-wanderer"},
	{name: "Rookie Guesser", minPoints: 30000, trophyKey: "rookie-guesser"},
	{name: "Geography Beginner", minPoints: 50000, trophyKey: "geography-beginner"},
	{name: "Geography Student", minPoints: 75000, trophyKey: "geography-student"},
	{name: "Map Reader", minPoints: 105000, trophyKey: "map-reader"},
	{name: "Landmark Spotter", minPoints: 140000, trophyKey: "landmark-spotter"},
	{name: "Local Guide", minPoints: 180000, trophyKey: "local-guide"},
	{name: "Seasoned Traveler", minPoints: 225000, trophyKey: "seasoned-traveler"},
	{name: "Explorer", minPoints: 275000, trophyKey: "explorer"},
	{name: "Scout", minPoints: 335000, trophyKey: "scout"},
	{name: "Wayfinder", minPoints: 405000, trophyKey: "wayfinder"},
	{name: "Road Reader", minPoints: 485000, trophyKey: "road-reader"},
	{name: "Navigator", minPoints: 575000, trophyKey: "navigator"},
	{name: "Surveyor", minPoints: 675000, trophyKey: "surveyor"},
	{name: "Cartographer", minPoints: 800000, trophyKey: "cartographer"},
	{name: "Geo Detective", minPoints: 950000, trophyKey: "geo-detective"},
	{name: "Geo Analyst", minPoints: 1125000, trophyKey: "geo-analyst"},
	{name: "Expert Navigator", minPoints: 1325000, trophyKey: "expert-navigator"},
	{name: "Master Cartographer", minPoints: 1550000, trophyKey: "master-cartographer"},
	{name: "Master Wayfinder", minPoints: 1800000, trophyKey: "master-wayfinder"},
	{name: "World Expert", minPoints: 2100000, trophyKey: "world-expert"},
	{name: "Geo Savant", minPoints: 2450000, trophyKey: "geo-savant"},
	{name: "Globe Master", minPoints: 2850000, trophyKey: "globe-master"},
	{name: "Earth Master", minPoints: 3300000, trophyKey: "earth-master"},
	{name: "Human Compass", minPoints: 3800000, trophyKey: "human-compass"},
	{name: "Human GPS", minPoints: 4400000, trophyKey: "human-gps"},
	{name: "World Sage", minPoints: 5100000, trophyKey: "world-sage"},
	{name: "Living Atlas", minPoints: 6000000, trophyKey: "living-atlas"},
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
