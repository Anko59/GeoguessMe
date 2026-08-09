package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"geoguessme/internal/chat"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"

	"github.com/jackc/pgx/v5"
)

func TestGameHandlersRejectUnsupportedMethods(t *testing.T) {
	mock := newMockPool(t)
	groupsAPI := NewGroupAPI(stubGroupReader{})
	chatAPI := newChatAPI(t, mock, mustTestStore(t), nil)
	gameAPI := newGameAPI(t, mock)
	tests := []struct {
		name string
		hand http.HandlerFunc
	}{
		{"create group", gameAPI.CreateGroup}, {"join group", gameAPI.JoinGroup}, {"leaderboard", gameAPI.GetLeaderboard},
		{"ticket", chatAPI.CreateWebSocketTicket}, {"guess", gameAPI.SubmitChallengeGuess}, {"results", gameAPI.GetChallengeResults},
		{"messages", chatAPI.GetGroupMessages}, {"group details", gameAPI.GetGroupDetails}, {"group members", gameAPI.GetGroupMembers},
		{"user groups", groupsAPI.GetUserGroups}, {"upload", gameAPI.UploadPhoto}, {"accept", gameAPI.AcceptChallenge}, {"media", gameAPI.ServeChallengeMedia},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			requireStatus(t, testCase.hand, requestWithUser(http.MethodPatch, "/", `{}`, "user-1"), http.StatusMethodNotAllowed)
		})
	}
}

func TestGameAndChatValidationBranches(t *testing.T) {
	mock := newMockPool(t)
	nilHubAPI := newChatAPI(t, mock, mustTestStore(t), nil)
	requireStatus(t, nilHubAPI.HandleChat, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusServiceUnavailable)
	hubAPI := newChatAPI(t, mock, mustTestStore(t), chat.NewHub(nil, nil))
	requireStatus(t, hubAPI.HandleChat, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusUnauthorized)

	gameAPI := newGameAPI(t, mock)
	requireStatus(t, nilHubAPI.CreateWebSocketTicket, requestWithUser(http.MethodPost, "/", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.GetLeaderboard, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.GetLeaderboard, requestWithUser(http.MethodGet, "/?group_id=group-1&period=year", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, nilHubAPI.GetGroupMessages, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.GetGroupDetails, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.GetGroupMembers, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.SubmitChallengeGuess, requestWithUser(http.MethodPost, "/", `{}`, "user-1"), http.StatusBadRequest)
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs("").WillReturnError(pgx.ErrNoRows)
	requireStatus(t, gameAPI.GetChallengeResults, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusNotFound)
	requireStatus(t, gameAPI.AcceptChallenge, requestWithUser(http.MethodPost, "/", `{}`, "user-1"), http.StatusBadRequest)
	repos := repository.NewRepository(mock)
	nilStoreGame := NewGameAPI(repos.Groups, repos.Chat, repos, nil, handlerConfig(), nil, nil, time.Now)
	requireStatus(t, nilStoreGame.ServeChallengeMedia, requestWithUser(http.MethodGet, "/", "", "user-1"), http.StatusServiceUnavailable)
}

func TestGroupAndUploadValidation(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	requireStatus(t, gameAPI.CreateGroup, requestWithUser(http.MethodPost, "/", `{"name":""}`, "user-1"), http.StatusBadRequest)
	requireStatus(t, gameAPI.JoinGroup, requestWithUser(http.MethodPost, "/", `{"code":"bad"}`, "user-1"), http.StatusBadRequest)
	repos := repository.NewRepository(mock)
	nilStoreGame := NewGameAPI(repos.Groups, repos.Chat, repos, nil, handlerConfig(), nil, nil, time.Now)
	requireStatus(t, nilStoreGame.UploadPhoto, requestWithUser(http.MethodPost, "/", "", "user-1"), http.StatusServiceUnavailable)

	validationGame := NewGameAPI(repos.Groups, repos.Chat, repos, &validationStore{}, handlerConfig(), nil, nil, time.Now)
	requireStatus(t, validationGame.UploadPhoto, requestWithUser(http.MethodPost, "/", "not-multipart", "user-1"), http.StatusBadRequest)
	if err := ValidateID("", "id"); err == nil {
		t.Fatal("empty identifier accepted")
	}
	if err := ValidateID("not-a-uuid", "id"); err == nil {
		t.Fatal("invalid identifier accepted")
	}
	if err := ValidateID("00000000-0000-0000-0000-000000000001", "id"); err != nil {
		t.Fatal(err)
	}
	if mediaURL(&models.Photo{ID: "photo-1"}, false) != "/api/v1/challenges/photo-1/media" || mediaURL(&models.Photo{ID: "photo-1"}, true) == mediaURL(&models.Photo{ID: "photo-1"}, false) {
		t.Fatal("media URLs are not distinct")
	}
}
