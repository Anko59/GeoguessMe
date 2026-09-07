package party

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/handlers"
	"geoguessme/internal/chat"
	"geoguessme/internal/config"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	partyrepo "geoguessme/internal/repository/party"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func handlerConfig() *config.Config {
	return &config.Config{PartyTimeDuration: time.Hour, PartyTimeCooldown: 48 * time.Hour}
}

// newMockPool returns an isolated mock pool for one handler test; unmet
// expectations fail the test at cleanup.
func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	return mock
}

type fakeProfiles struct {
	users map[string]*models.User
}

func (f fakeProfiles) GetUserByID(_ context.Context, userID string) (*models.User, error) {
	if user, ok := f.users[userID]; ok {
		return user, nil
	}
	return nil, errors.New("user not found")
}

type fakeNotifier struct {
	groupID       string
	excludeUserID string
	starterName   string
	calls         int
}

func (f *fakeNotifier) NotifyPartyStarted(_ context.Context, groupID, excludeUserID, starterUsername string) {
	f.calls++
	f.groupID, f.excludeUserID, f.starterName = groupID, excludeUserID, starterUsername
}

func requestWithUser(method, target, body, userID string) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	if userID != "" {
		request = request.WithContext(handlers.WithUserID(request.Context(), userID))
	}
	return request
}

func newAPI(t *testing.T, mock pgxmock.PgxPoolIface, notifier *fakeNotifier) *API {
	t.Helper()
	repos := repository.NewRepository(mock)
	return NewAPI(repos.Groups, repos.Party, repos.Chat, fakeProfiles{users: map[string]*models.User{
		"user-1": {ID: "user-1", Username: "Alice"},
	}}, notifier, chat.NewHub(nil, nil), handlerConfig(), time.Now)
}

func expectMembership(mock pgxmock.PgxPoolIface, member bool) {
	mock.ExpectQuery("SELECT EXISTS").WithArgs("00000000-0000-0000-0000-000000000001", "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(member))
}

var activeWindow = []string{"id", "group_id", "started_by", "started_at", "ends_at"}

func windowRow(id string, startedAt, endsAt time.Time) *pgxmock.Rows {
	return pgxmock.NewRows(activeWindow).AddRow(id, "00000000-0000-0000-0000-000000000001", "user-1", startedAt, endsAt)
}

func TestPartyStatusStates(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	cases := []struct {
		name           string
		rows           *pgxmock.Rows
		wantActive     bool
		wantRecharging bool
	}{
		{
			name:       "active party",
			rows:       windowRow("w1", now.Add(-30*time.Minute), now.Add(30*time.Minute)),
			wantActive: true,
		},
		{
			name:           "recharging after a past party",
			rows:           windowRow("w2", now.Add(-2*time.Hour), now.Add(-time.Hour)),
			wantRecharging: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			mock := newMockPool(t)
			api := newAPI(t, mock, &fakeNotifier{})
			expectMembership(mock, true)
			mock.ExpectQuery("SELECT id, group_id, started_by").WithArgs("00000000-0000-0000-0000-000000000001", pgxmock.AnyArg()).WillReturnRows(testCase.rows)
			recorder := httptest.NewRecorder()
			api.HandleParty(recorder, requestWithUser(http.MethodGet, "/?group_id=00000000-0000-0000-0000-000000000001", "", "user-1"))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
			}
			var response struct {
				Active          bool       `json:"active"`
				StartedAt       *time.Time `json:"started_at"`
				EndsAt          *time.Time `json:"ends_at"`
				NextAvailableAt *time.Time `json:"next_available_at"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Active != testCase.wantActive {
				t.Fatalf("active = %v, want %v: %s", response.Active, testCase.wantActive, recorder.Body.String())
			}
			if testCase.wantActive && response.EndsAt == nil {
				t.Fatalf("active state must publish ends_at: %s", recorder.Body.String())
			}
			if !testCase.wantActive && response.EndsAt != nil {
				t.Fatalf("inactive state must omit ends_at: %s", recorder.Body.String())
			}
			if (response.NextAvailableAt != nil) != (testCase.wantActive || testCase.wantRecharging) {
				t.Fatalf("next_available_at mismatch: %s", recorder.Body.String())
			}
		})
	}

	t.Run("never-started party is available immediately", func(t *testing.T) {
		mock := newMockPool(t)
		api := newAPI(t, mock, &fakeNotifier{})
		expectMembership(mock, true)
		mock.ExpectQuery("SELECT id, group_id, started_by").WithArgs("00000000-0000-0000-0000-000000000001", pgxmock.AnyArg()).WillReturnError(pgx.ErrNoRows)
		recorder := httptest.NewRecorder()
		api.HandleParty(recorder, requestWithUser(http.MethodGet, "/?group_id=00000000-0000-0000-0000-000000000001", "", "user-1"))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d", recorder.Code)
		}
		body := recorder.Body.String()
		if !strings.Contains(body, `"active":false`) || strings.Contains(body, "next_available_at") {
			t.Fatalf("fresh group body = %s", body)
		}
	})
}

func TestPartyStatusGuards(t *testing.T) {
	t.Run("missing group id is 400", func(t *testing.T) {
		mock := newMockPool(t)
		api := newAPI(t, mock, &fakeNotifier{})
		recorder := httptest.NewRecorder()
		api.HandleParty(recorder, requestWithUser(http.MethodGet, "/", "", "user-1"))
		requireErrorCode(t, recorder, http.StatusBadRequest, "missing_group_id")
	})
	t.Run("non-member is 403", func(t *testing.T) {
		mock := newMockPool(t)
		api := newAPI(t, mock, &fakeNotifier{})
		expectMembership(mock, false)
		recorder := httptest.NewRecorder()
		api.HandleParty(recorder, requestWithUser(http.MethodGet, "/?group_id=00000000-0000-0000-0000-000000000001", "", "user-1"))
		requireErrorCode(t, recorder, http.StatusForbidden, "forbidden")
	})
	t.Run("unsupported method is 405", func(t *testing.T) {
		mock := newMockPool(t)
		api := newAPI(t, mock, &fakeNotifier{})
		recorder := httptest.NewRecorder()
		api.HandleParty(recorder, requestWithUser(http.MethodDelete, "/?group_id=00000000-0000-0000-0000-000000000001", "", "user-1"))
		requireErrorCode(t, recorder, http.StatusMethodNotAllowed, "method_not_allowed")
	})
}

func TestStartPartyAnnouncesAndNotifies(t *testing.T) {
	mock := newMockPool(t)
	notifier := &fakeNotifier{}
	api := newAPI(t, mock, notifier)
	expectMembership(mock, true)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM groups").WithArgs("00000000-0000-0000-0000-000000000001").WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("00000000-0000-0000-0000-000000000001"))
	mock.ExpectQuery("SELECT id, group_id, started_by").WithArgs("00000000-0000-0000-0000-000000000001", pgxmock.AnyArg()).WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec("INSERT INTO group_party_times").
		WithArgs(pgxmock.AnyArg(), "00000000-0000-0000-0000-000000000001", "user-1", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	// SaveMessage resolves the sender profile before the insert.
	mock.ExpectQuery("SELECT username, avatar FROM users").WithArgs("user-1").
		WillReturnRows(pgxmock.NewRows([]string{"username", "avatar"}).AddRow("Alice", "a.png"))
	mock.ExpectExec("INSERT INTO messages").
		WithArgs(pgxmock.AnyArg(), "00000000-0000-0000-0000-000000000001", "user-1", "system", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	recorder := httptest.NewRecorder()
	api.HandleParty(recorder, requestWithUser(http.MethodPost, "/", `{"group_id":"00000000-0000-0000-0000-000000000001"}`, "user-1"))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("start status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Active     bool       `json:"active"`
		EndsAt     *time.Time `json:"ends_at"`
		ServerTime time.Time  `json:"server_time"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Active || response.EndsAt == nil {
		t.Fatalf("start body = %s", recorder.Body.String())
	}
	if notifier.calls != 1 || notifier.starterName != "Alice" || notifier.excludeUserID != "user-1" {
		t.Fatalf("notifier = %+v", notifier)
	}
}

func TestStartPartyConflicts(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name      string
		latest    *pgxmock.Rows
		wantCode  int
		wantError string
	}{
		{
			name:      "active party answers 409 party_active",
			latest:    windowRow("w1", now.Add(-30*time.Minute), now.Add(30*time.Minute)),
			wantCode:  http.StatusConflict,
			wantError: "party_active",
		},
		{
			name:      "recharging cooldown answers 409 party_recharging",
			latest:    windowRow("w2", now.Add(-2*time.Hour), now.Add(-time.Hour)),
			wantCode:  http.StatusConflict,
			wantError: "party_recharging",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			mock := newMockPool(t)
			notifier := &fakeNotifier{}
			api := newAPI(t, mock, notifier)
			expectMembership(mock, true)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT id FROM groups").WithArgs("00000000-0000-0000-0000-000000000001").WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("00000000-0000-0000-0000-000000000001"))
			mock.ExpectQuery("SELECT id, group_id, started_by").WithArgs("00000000-0000-0000-0000-000000000001", pgxmock.AnyArg()).WillReturnRows(testCase.latest)
			mock.ExpectRollback()
			recorder := httptest.NewRecorder()
			api.HandleParty(recorder, requestWithUser(http.MethodPost, "/", `{"group_id":"00000000-0000-0000-0000-000000000001"}`, "user-1"))
			requireErrorCode(t, recorder, testCase.wantCode, testCase.wantError)
			if notifier.calls != 0 {
				t.Fatalf("conflict must not notify, calls = %d", notifier.calls)
			}
		})
	}

	t.Run("unknown group answers 404", func(t *testing.T) {
		mock := newMockPool(t)
		api := newAPI(t, mock, &fakeNotifier{})
		expectMembership(mock, true)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id FROM groups").WithArgs("00000000-0000-0000-0000-000000000001").WillReturnError(pgx.ErrNoRows)
		mock.ExpectRollback()
		recorder := httptest.NewRecorder()
		api.HandleParty(recorder, requestWithUser(http.MethodPost, "/", `{"group_id":"00000000-0000-0000-0000-000000000001"}`, "user-1"))
		requireErrorCode(t, recorder, http.StatusNotFound, "group_not_found")
	})

	t.Run("non-member cannot start", func(t *testing.T) {
		mock := newMockPool(t)
		api := newAPI(t, mock, &fakeNotifier{})
		expectMembership(mock, false)
		recorder := httptest.NewRecorder()
		api.HandleParty(recorder, requestWithUser(http.MethodPost, "/", `{"group_id":"00000000-0000-0000-0000-000000000001"}`, "user-1"))
		requireErrorCode(t, recorder, http.StatusForbidden, "forbidden")
	})

	t.Run("missing group id in body is 400", func(t *testing.T) {
		mock := newMockPool(t)
		api := newAPI(t, mock, &fakeNotifier{})
		recorder := httptest.NewRecorder()
		api.HandleParty(recorder, requestWithUser(http.MethodPost, "/", `{}`, "user-1"))
		requireErrorCode(t, recorder, http.StatusBadRequest, "missing_group_id")
	})
}

func requireErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d (%s), want %d", recorder.Code, recorder.Body.String(), status)
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != code {
		t.Fatalf("body = %s (%v), want error code %q", recorder.Body.String(), err, code)
	}
}

func TestBuildStatusShapes(t *testing.T) {
	now := time.Date(2025, 6, 20, 20, 0, 0, 0, time.UTC)
	cooldown := 48 * time.Hour
	if got := buildStatus(now, cooldown, nil); got.Active || got.NextAvailableAt != nil {
		t.Fatalf("empty state = %+v", got)
	}
	past := &partyrepo.Window{StartedAt: now.Add(-2 * time.Hour), EndsAt: now.Add(-time.Hour)}
	recharged := buildStatus(past.EndsAt.Add(cooldown), cooldown, past)
	if recharged.Active || recharged.NextAvailableAt != nil {
		t.Fatalf("elapsed recharge state = %+v", recharged)
	}
	recharging := buildStatus(now.Add(time.Hour), cooldown, past)
	if recharging.Active || recharging.NextAvailableAt == nil || !recharging.NextAvailableAt.Equal(past.EndsAt.Add(cooldown)) {
		t.Fatalf("recharge state = %+v", recharging)
	}
	liveWindow := &partyrepo.Window{StartedAt: now.Add(-30 * time.Minute), EndsAt: now.Add(30 * time.Minute)}
	live := buildStatus(now, cooldown, liveWindow)
	if !live.Active || live.StartedAt == nil || live.EndsAt == nil || live.NextAvailableAt == nil {
		t.Fatalf("live state = %+v", live)
	}
}
