package progression

import "testing"

func TestRankForPointsCoversEveryRank(t *testing.T) {
	if RankCount < 20 {
		t.Fatalf("rank count = %d, want at least 20", RankCount)
	}
	for index, definition := range rankDefinitions {
		rank := RankForPoints(definition.minPoints)
		if rank.Level != index+1 || rank.Name != definition.name || rank.TrophyKey != definition.trophyKey {
			t.Fatalf("rank at %d points = %+v, want level %d %q", definition.minPoints, rank, index+1, definition.name)
		}
	}
}

func TestRankProgressIsBoundedBetweenThresholds(t *testing.T) {
	rank := RankForPoints(600)
	if rank.Name != "Squire" || rank.PointsInRank != 100 || rank.PointsToNext != 1000 || rank.ProgressPercent != 10 {
		t.Fatalf("rank progress = %+v", rank)
	}
	if got := RankForPoints(-10); got.Level != 1 || got.PointsInRank != 0 {
		t.Fatalf("negative points = %+v", got)
	}
	last := RankForPoints(rankDefinitions[len(rankDefinitions)-1].minPoints + 999999)
	if last.Level != RankCount || last.NextPoints != nil || last.ProgressPercent != 100 {
		t.Fatalf("maximum rank = %+v", last)
	}
}
