package feed

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pashagolub/pgxmock/v5"
)

func TestResultsRanksGuessesAndReturnsSignedAllTimeDeltas(t *testing.T) {
	r, mock := mockRepository(t)
	created := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT p.user_id,p.hide_location,p.created_at").WithArgs("viewer", "post").WillReturnRows(pgxmock.NewRows([]string{"user_id", "hide_location", "created_at"}).AddRow("author", false, created))
	mock.ExpectQuery("SELECT g.user_id,u.username,u.avatar,g.score,g.distance").WithArgs("viewer", "post").WillReturnRows(
		pgxmock.NewRows([]string{"user_id", "username", "avatar", "score", "distance", "pin_key", "pin_name", "pin_image"}).
			AddRow("winner", "Navigator", "avatar-a.png", 4800, 100.0, "north-star", "North Star", "/map-pins/north-star.svg").
			AddRow("viewer", "Explorer", "avatar-b.png", 4200, 300.0, "", "", ""),
	)
	mock.ExpectQuery("SELECT challenge_id,created_at,user_id,score FROM").WillReturnRows(
		pgxmock.NewRows([]string{"challenge_id", "created_at", "user_id", "score"}).
			AddRow("public:post", created, "winner", 4800).
			AddRow("public:post", created, "viewer", 4200),
	)
	results, err := r.Results(t.Context(), "post", "viewer", created, 48*time.Hour)
	if err != nil {
		t.Fatalf("Results = %v", err)
	}
	if len(results) != 2 || results[0].Rank != 1 || results[0].UserID != "winner" || results[0].EloDelta != 4 || results[0].MapPin == nil || results[0].MapPin.Key != "north-star" {
		t.Fatalf("winner result = %+v", results)
	}
	if results[1].Rank != 2 || !results[1].IsViewer || results[1].EloDelta != -4 {
		t.Fatalf("viewer result = %+v", results)
	}
}

func TestResultsRejectsInvisibleChallenge(t *testing.T) {
	r, mock := mockRepository(t)
	mock.ExpectQuery("SELECT p.user_id,p.hide_location,p.created_at").WithArgs("viewer", "post").WillReturnError(pgx.ErrNoRows)
	if _, err := r.Results(t.Context(), "post", "viewer", time.Now(), 48*time.Hour); err != ErrNotFound {
		t.Fatalf("invisible challenge error = %v", err)
	}
}

func TestLegacyResultsHidePeerDistancesWithoutChangingRanking(t *testing.T) {
	r, mock := mockRepository(t)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT p.user_id,p.hide_location,p.created_at").WithArgs("viewer", "post").WillReturnRows(pgxmock.NewRows([]string{"owner", "hide_location", "created_at"}).AddRow("author", true, now))
	mock.ExpectQuery("SELECT g.user_id,u.username,u.avatar,g.score,g.distance").WithArgs("viewer", "post").WillReturnRows(
		pgxmock.NewRows([]string{"user_id", "username", "avatar", "score", "distance", "pin_key", "pin_name", "pin_image"}).AddRow("peer", "Peer", "", 4500, 10.0, "", "", "").AddRow("viewer", "Viewer", "", 4000, 20.0, "", "", ""),
	)
	mock.ExpectQuery("SELECT challenge_id,created_at,user_id,score FROM").WillReturnRows(pgxmock.NewRows([]string{"challenge_id", "created_at", "user_id", "score"}))
	results, err := r.Results(t.Context(), "post", "viewer", now, 48*time.Hour)
	if err != nil || len(results) != 2 {
		t.Fatalf("results = %+v, %v", results, err)
	}
	if results[0].Distance != nil || results[0].Score != 4500 || results[0].Rank != 1 || results[1].Distance == nil || *results[1].Distance != 20 || !results[1].IsViewer {
		t.Fatalf("privacy changed score/own distance or leaked peer distance: %+v", results)
	}
}
