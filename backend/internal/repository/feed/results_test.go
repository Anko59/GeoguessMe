package feed

import (
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v5"
)

func TestResultsRanksGuessesAndReturnsSignedAllTimeDeltas(t *testing.T) {
	r, mock := mockRepository(t)
	created := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("viewer", "post").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT g.user_id,u.username,u.avatar,g.score,g.distance").WithArgs("viewer", "post").WillReturnRows(
		pgxmock.NewRows([]string{"user_id", "username", "avatar", "score", "distance"}).
			AddRow("winner", "Navigator", "avatar-a.png", 4800, 100.0).
			AddRow("viewer", "Explorer", "avatar-b.png", 4200, 300.0),
	)
	mock.ExpectQuery("SELECT challenge_id,created_at,user_id,score FROM").WillReturnRows(
		pgxmock.NewRows([]string{"challenge_id", "created_at", "user_id", "score"}).
			AddRow("public:post", created, "winner", 4800).
			AddRow("public:post", created, "viewer", 4200),
	)
	results, err := r.Results(t.Context(), "post", "viewer")
	if err != nil {
		t.Fatalf("Results = %v", err)
	}
	if len(results) != 2 || results[0].Rank != 1 || results[0].UserID != "winner" || results[0].EloDelta != 4 {
		t.Fatalf("winner result = %+v", results)
	}
	if results[1].Rank != 2 || !results[1].IsViewer || results[1].EloDelta != -4 {
		t.Fatalf("viewer result = %+v", results)
	}
}

func TestResultsRejectsInvisibleChallenge(t *testing.T) {
	r, mock := mockRepository(t)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("viewer", "post").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	if _, err := r.Results(t.Context(), "post", "viewer"); err != ErrNotFound {
		t.Fatalf("invisible challenge error = %v", err)
	}
}
