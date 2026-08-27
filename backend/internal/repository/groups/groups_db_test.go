package groups

import (
	"context"
	"errors"
	"testing"
	"time"

	"geoguessme/internal/elo"
	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func TestGroupPhotoAndNotificationPersistence(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	photo := &models.GroupPhoto{
		GroupID: "group-1", StorageKey: "groups/group-1/photo/new",
		MIMEType: "image/webp", ByteSize: 1234, CreatedAt: now,
	}

	mock.ExpectQuery("SELECT group_id, storage_key, mime_type").
		WithArgs(photo.GroupID).
		WillReturnRows(pgxmock.NewRows([]string{"group_id", "storage_key", "mime_type", "byte_size", "created_at"}).
			AddRow(photo.GroupID, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.CreatedAt))
	got, err := repo.GroupPhoto(ctx, photo.GroupID)
	if err != nil || got == nil || *got != *photo {
		t.Fatalf("group photo = %+v, %v", got, err)
	}

	mock.ExpectQuery("SELECT group_id, storage_key, mime_type").
		WithArgs("missing").
		WillReturnError(pgx.ErrNoRows)
	got, err = repo.GroupPhoto(ctx, "missing")
	if err != nil || got != nil {
		t.Fatalf("missing group photo = %+v, %v", got, err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT storage_key FROM group_photos").
		WithArgs(photo.GroupID).
		WillReturnRows(pgxmock.NewRows([]string{"storage_key"}).AddRow("groups/group-1/photo/old"))
	mock.ExpectExec("INSERT INTO group_photos").
		WithArgs(photo.GroupID, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.CreatedAt).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	previous, err := repo.SetGroupPhoto(ctx, photo)
	if err != nil || previous != "groups/group-1/photo/old" {
		t.Fatalf("set group photo = %q, %v", previous, err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT storage_key FROM group_photos").
		WithArgs(photo.GroupID).
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec("INSERT INTO group_photos").
		WithArgs(photo.GroupID, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.CreatedAt).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	previous, err = repo.SetGroupPhoto(ctx, photo)
	if err != nil || previous != "" {
		t.Fatalf("create group photo = %q, %v", previous, err)
	}

	mock.ExpectQuery("SELECT COALESCE").
		WithArgs(photo.GroupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"enabled"}).AddRow(false))
	enabled, err := repo.NotificationPreference(ctx, photo.GroupID, "user-1")
	if err != nil || enabled {
		t.Fatalf("notification preference = %v, %v", enabled, err)
	}
	mock.ExpectExec("INSERT INTO group_notification_preferences").
		WithArgs(photo.GroupID, "user-1", true).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.SetNotificationPreference(ctx, photo.GroupID, "user-1", true); err != nil {
		t.Fatalf("set notification preference: %v", err)
	}
}

func TestSetGroupPhotoRollsBackOnPersistenceFailure(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	photo := &models.GroupPhoto{GroupID: "group-1", StorageKey: "new", MIMEType: "image/webp", ByteSize: 12, CreatedAt: time.Now()}
	persistErr := errors.New("write failed")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT storage_key FROM group_photos").
		WithArgs(photo.GroupID).
		WillReturnRows(pgxmock.NewRows([]string{"storage_key"}).AddRow("old"))
	mock.ExpectExec("INSERT INTO group_photos").
		WithArgs(photo.GroupID, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.CreatedAt).
		WillReturnError(persistErr)
	mock.ExpectRollback()
	if _, err := repo.SetGroupPhoto(context.Background(), photo); !errors.Is(err, persistErr) {
		t.Fatalf("set group photo error = %v", err)
	}
}

func TestGroupQueriesAndMembership(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	group := &models.Group{ID: "group-1", Name: "Paris", Code: "ABC123", CreatedAt: now}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO groups").WithArgs(group.ID, group.Name, group.Code, group.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO group_members").WithArgs(group.ID, "user-1", group.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if err := repo.Create(context.Background(), group, "user-1"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO groups").WithArgs(group.ID, group.Name, group.Code, group.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if err := repo.Create(context.Background(), group, ""); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, name, code, created_at FROM groups WHERE code").WithArgs(group.Code).WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow(group.ID, group.Name, group.Code, group.CreatedAt))
	got, err := repo.ByCode(context.Background(), group.Code)
	if err != nil || got == nil || got.ID != group.ID {
		t.Fatalf("group by code = %+v, %v", got, err)
	}
	mock.ExpectQuery("SELECT id, name, code, created_at FROM groups WHERE code").WithArgs("missing").WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}))
	got, err = repo.ByCode(context.Background(), "missing")
	if err != nil || got != nil {
		t.Fatalf("missing group = %+v, %v", got, err)
	}
	mock.ExpectExec("INSERT INTO group_members").WithArgs(group.ID, "user-2", now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.AddMember(context.Background(), &models.GroupMember{GroupID: group.ID, UserID: "user-2", JoinedAt: now}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("INSERT INTO group_members").WithArgs(group.ID, "user-3", now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.AddMember(context.Background(), &models.GroupMember{GroupID: group.ID, UserID: "user-3", JoinedAt: now}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(group.ID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	member, err := repo.IsMember(context.Background(), group.ID, "user-1")
	if err != nil || !member {
		t.Fatalf("membership = %v, %v", member, err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(group.ID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	member, err = repo.IsMember(context.Background(), group.ID, "user-2")
	if err != nil || member {
		t.Fatalf("non-membership = %v, %v", member, err)
	}
	mock.ExpectQuery("SELECT id, name, code, created_at FROM groups WHERE id").WithArgs(group.ID).WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow(group.ID, group.Name, group.Code, group.CreatedAt))
	got, err = repo.ByID(context.Background(), group.ID)
	if err != nil || got == nil {
		t.Fatalf("group by id = %+v, %v", got, err)
	}
}

func TestSharesGroup(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	// A user always shares a group with themself, without a query.
	shared, err := repo.SharesGroup(context.Background(), "user-1", "user-1")
	if err != nil || !shared {
		t.Fatalf("self shared = %v, %v", shared, err)
	}
	// Two members of a common group share it.
	mock.ExpectQuery("SELECT EXISTS").WithArgs("user-1", "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	shared, err = repo.SharesGroup(context.Background(), "user-1", "user-2")
	if err != nil || !shared {
		t.Fatalf("shared group = %v, %v", shared, err)
	}
	// Players in disjoint groups do not share one.
	mock.ExpectQuery("SELECT EXISTS").WithArgs("user-1", "user-3").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	shared, err = repo.SharesGroup(context.Background(), "user-1", "user-3")
	if err != nil || shared {
		t.Fatalf("disjoint groups = %v, %v", shared, err)
	}
}

func TestGroupListsMembersAndLeaderboard(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT g.id, g.name, g.code").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow("g1", "One", "AAA111", now))
	groups, err := repo.UserGroups(context.Background(), "user-1")
	if err != nil || len(groups) != 1 || groups[0].ID != "g1" || groups[0].Name != "One" {
		t.Fatalf("groups = %+v, %v", groups, err)
	}
	mock.ExpectQuery("SELECT u.id, u.username, u.avatar").WithArgs("g1").WillReturnRows(pgxmock.NewRows([]string{"id", "username", "avatar"}).AddRow("u1", "alice", "a.png"))
	members, err := repo.Members(context.Background(), "g1")
	if err != nil || len(members) != 1 || members[0].Username != "alice" {
		t.Fatalf("members = %+v, %v", members, err)
	}
	leaderboardQuery := `(?s)SELECT u\.id, u\.username, u\.avatar.*SUM\(g\.score\).*ORDER BY COALESCE\(SUM\(g\.score\), 0\) DESC`
	challengesQuery := `(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE AND NOT g\.timed_out AND g\.group_id = \$1`
	emptyChallenges := func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"})
	}
	mock.ExpectQuery(leaderboardQuery).WithArgs("g1").WillReturnRows(pgxmock.NewRows([]string{"id", "username", "avatar", "score", "count", "average", "total_points"}).AddRow("u1", "alice", "avatar4.png", 160, 2, 80.0, 7600))
	mock.ExpectQuery(challengesQuery).WithArgs("g1").WillReturnRows(emptyChallenges())
	entries, err := repo.LeaderboardForPeriod(context.Background(), "g1", LeaderboardAllTime)
	if err != nil || len(entries) != 1 || entries[0].Score != 160 || entries[0].Average != 80.0 || entries[0].Avatar != "avatar4.png" || entries[0].TotalPoints != 7600 || entries[0].Elo != 0 || entries[0].Rank.Name != "Lost Tourist" {
		t.Fatalf("leaderboard = %+v, %v", entries, err)
	}
	mock.ExpectQuery(leaderboardQuery).WithArgs("g1").WillReturnRows(pgxmock.NewRows([]string{"id", "username", "avatar", "score", "count", "average", "total_points"}).AddRow("u1", "alice", "avatar4.png", 160, 2, 80.0, 7600))
	mock.ExpectQuery(challengesQuery).WithArgs("g1").WillReturnRows(emptyChallenges())
	if entries, err := repo.LeaderboardForPeriod(context.Background(), "g1", LeaderboardAllTime); err != nil || len(entries) != 1 || entries[0].Elo != 0 {
		t.Fatalf("LeaderboardForPeriod = %+v, %v", entries, err)
	}
}

func TestLeaderboardPeriodStart(t *testing.T) {
	now := time.Date(2026, time.July, 26, 18, 45, 0, 0, time.FixedZone("CEST", 2*60*60))
	week := leaderboardPeriodStart(LeaderboardWeek, now)
	month := leaderboardPeriodStart(LeaderboardMonth, now)
	if week == nil || !week.Equal(time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("week start = %v", week)
	}
	if month == nil || !month.Equal(time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("month start = %v", month)
	}
	if start := leaderboardPeriodStart(LeaderboardAllTime, now); start != nil {
		t.Fatalf("all-time start = %v", start)
	}
}

func TestLoadChallengesDropsSingleGuesserPhotos(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE AND NOT g\.timed_out AND g\.group_id = \$1 ORDER BY`).
		WithArgs("g1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}).
			AddRow("p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "u1", 4000).
			AddRow("p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "u2", 1000).
			AddRow("p2", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "u1", 2000))
	challenges, err := repo.LoadChallenges(context.Background(), "g1", nil)
	if err != nil {
		t.Fatalf("LoadChallenges = %v", err)
	}
	if len(challenges) != 1 {
		t.Fatalf("challenges = %d, want 1 (single-guesser photo dropped)", len(challenges))
	}
	if challenges[0].ID != "p1" || len(challenges[0].Guesses) != 2 {
		t.Fatalf("challenge = %+v", challenges[0])
	}
}

func TestGlobalEloRanksByComputedRating(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE AND NOT g\.timed_out ORDER BY`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}).
			AddRow("p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "alice", 5000).
			AddRow("p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "bob", 0))
	stats, err := repo.GlobalElo(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GlobalElo = %v", err)
	}
	if stats.Elo <= 1000 || stats.Rank != 1 || stats.TotalPlayers != 2 {
		t.Fatalf("elo stats = %+v", stats)
	}
}

func TestGlobalEloUnratedPlayer(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE AND NOT g\.timed_out ORDER BY`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}).
			AddRow("p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "alice", 5000).
			AddRow("p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "bob", 0))
	stats, err := repo.GlobalElo(context.Background(), "carol")
	if err != nil {
		t.Fatalf("GlobalElo = %v", err)
	}
	if stats.Elo != 0 || stats.Rank != 0 || stats.TotalPlayers != 2 {
		t.Fatalf("unrated stats = %+v", stats)
	}
}

func TestGlobalAverageRank(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectQuery(`(?s)WITH scores AS.*ranked`).
		WithArgs("alice").
		WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(int64(2), int64(5)))
	stats, err := repo.GlobalAverageRank(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GlobalAverageRank = %v", err)
	}
	if stats.Rank != 2 || stats.TotalPlayers != 5 {
		t.Fatalf("average rank stats = %+v", stats)
	}
}

func TestSortLeaderboardByMetric(t *testing.T) {
	entries := []LeaderboardEntry{
		{UserID: "a", Username: "alice", Score: 100, Average: 90, Elo: 900, TotalPoints: 100},
		{UserID: "b", Username: "bob", Score: 200, Average: 95, Elo: 1100, TotalPoints: 200},
		{UserID: "c", Username: "carol", Score: 150, Average: 60, Elo: 1200, TotalPoints: 150},
	}
	SortLeaderboard(entries, LeaderboardMetricTotal)
	if entries[0].UserID != "b" {
		t.Fatalf("total order = %+v, want bob first", entries)
	}
	SortLeaderboard(entries, LeaderboardMetricAverage)
	if entries[0].UserID != "b" || entries[1].UserID != "a" {
		t.Fatalf("average order = %+v", entries)
	}
	SortLeaderboard(entries, LeaderboardMetricElo)
	if entries[0].UserID != "c" || entries[1].UserID != "b" {
		t.Fatalf("elo order = %+v", entries)
	}
}

func TestLeaderboardFactorMapping(t *testing.T) {
	cases := map[LeaderboardPeriod]elo.Factor{
		LeaderboardWeek:    elo.FactorWeekly,
		LeaderboardMonth:   elo.FactorMonthly,
		LeaderboardAllTime: elo.FactorAllTime,
	}
	for period, want := range cases {
		if got := leaderboardFactor(period); got != want {
			t.Fatalf("leaderboardFactor(%s) = %v, want %v", period, got, want)
		}
	}
}

func TestWeeklyChallengeEloDeltas(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE AND NOT g\.timed_out.*AND p\.created_at >= \$1 ORDER BY`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}).
			AddRow("p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "alice", 5000).
			AddRow("p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "bob", 0))
	deltas, err := repo.WeeklyChallengeEloDeltas(context.Background(), "p1")
	if err != nil {
		t.Fatalf("WeeklyChallengeEloDeltas = %v", err)
	}
	if deltas["alice"] != 20 || deltas["bob"] != -20 {
		t.Fatalf("deltas = %+v, want alice: 20, bob: -20 (weekly factor)", deltas)
	}
}

func TestWeeklyChallengeEloDeltasOutsideWeek(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	// The SQL window only returns this week's challenges, so a photo created
	// before the week started is absent from the replay.
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*AND p\.created_at >= \$1 ORDER BY`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}).
			AddRow("p2", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "alice", 5000).
			AddRow("p2", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "bob", 0))
	deltas, err := repo.WeeklyChallengeEloDeltas(context.Background(), "p1")
	if err != nil {
		t.Fatalf("WeeklyChallengeEloDeltas = %v", err)
	}
	if deltas != nil {
		t.Fatalf("deltas = %+v, want nil for a challenge outside the weekly window", deltas)
	}
}
