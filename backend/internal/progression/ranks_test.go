package progression

import "testing"

func TestRankForPointsCoversEveryRank(t *testing.T) {
	if RankCount != 30 {
		t.Fatalf("rank count = %d, want 30", RankCount)
	}
	for index, definition := range rankDefinitions {
		rank := RankForPoints(definition.minPoints)
		if rank.Level != index+1 || rank.Name != definition.name || rank.TrophyKey != definition.trophyKey {
			t.Fatalf("rank at %d points = %+v, want level %d %q", definition.minPoints, rank, index+1, definition.name)
		}
		if index == len(rankDefinitions)-1 {
			if rank.Next != nil || rank.NextPoints != nil {
				t.Fatalf("highest rank must have no next rank: %+v", rank)
			}
			continue
		}
		next := rankDefinitions[index+1]
		if rank.Next == nil || rank.Next.Level != index+2 || rank.Next.Name != next.name || rank.Next.TrophyKey != next.trophyKey {
			t.Fatalf("rank %d next = %+v, want level %d %q", index+1, rank.Next, index+2, next.name)
		}
	}
}

func TestRankProgressIsBoundedBetweenThresholds(t *testing.T) {
	rank := RankForPoints(6000)
	if rank.Name != "Lost Tourist" || rank.PointsInRank != 1000 || rank.PointsToNext != 10000 || rank.ProgressPercent != 10 {
		t.Fatalf("rank progress = %+v", rank)
	}
	if got := RankForPoints(-10); got.Level != 1 || got.PointsInRank != 0 {
		t.Fatalf("negative points = %+v", got)
	}
	last := RankForPoints(rankDefinitions[len(rankDefinitions)-1].minPoints + 999999)
	if last.Level != RankCount || last.NextPoints != nil || last.Next != nil || last.ProgressPercent != 100 {
		t.Fatalf("maximum rank = %+v", last)
	}
}
