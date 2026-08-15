package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"golang.org/x/crypto/bcrypt"
)

func profileUser(id, username string) *models.User {
	now := time.Now().UTC()
	return &models.User{ID: id, Username: username, Email: username + "@example.test", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
}

// expectProfileQueries queues every SQL expectation a profile fetch issues:
// score stats (sum, count, average), the lifetime-points global rank, the
// average global rank, and the global Elo challenge load.
func expectProfileQueries(t *testing.T, mock pgxmock.PgxPoolIface, userID string, totalPoints, guessCount int64, average float64, pointsRank, pointsPlayers, averageRank, averagePlayers int64) {
	t.Helper()
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(score\\), 0\\), COUNT\\(\\*\\), COALESCE\\(AVG\\(score\\), 0\\)").WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"total_points", "guess_count", "average_score"}).AddRow(totalPoints, guessCount, average))
	mock.ExpectQuery("WITH totals AS").WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(pointsRank, pointsPlayers))
	mock.ExpectQuery("WITH scores AS").WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(averageRank, averagePlayers))
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE ORDER BY`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}))
}

func TestGetPublicProfile(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	target := profileUser("user-2", "bob")
	viewer := profileUser("user-1", "alice")

	// Unsupported methods are rejected before any lookup.
	requireStatus(t, api.GetPublicProfile, requestWithUser(http.MethodPost, "/", "", viewer.ID), http.StatusMethodNotAllowed)

	// Unknown target player.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(target.ID).WillReturnRows(pgxmock.NewRows(userColumnsForQuery()))
	requireStatus(t, api.GetPublicProfile, getPublicProfileRequest(viewer.ID, target.ID), http.StatusNotFound)

	// Players without a shared group cannot view each other's profile.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(target.ID).WillReturnRows(handlerUserRows(target))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(target.ID, viewer.ID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	requireStatus(t, api.GetPublicProfile, getPublicProfileRequest(viewer.ID, target.ID), http.StatusForbidden)

	// Players sharing a group see the target's progression without email.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(target.ID).WillReturnRows(handlerUserRows(target))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(target.ID, viewer.ID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	expectProfileQueries(t, mock, target.ID, 7600, 3, 2533.33, 3, 1943, 7, 1943)
	recorder := httptest.NewRecorder()
	api.GetPublicProfile(recorder, getPublicProfileRequest(viewer.ID, target.ID))
	if recorder.Code != http.StatusOK {
		t.Fatalf("public profile status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{`"username":"bob"`, `"total_points":7600`, `"name":"Lost Tourist"`, `"global_rank":{"rank":3,"total_players":1943}`, `"average_score":2533.33`, `"global_average_rank":{"rank":7,"total_players":1943}`, `"global_elo_rank":{"rank":0,"total_players":0}`} {
		if !strings.Contains(body, want) {
			t.Fatalf("public profile missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, "email") {
		t.Fatalf("public profile must not expose email: %s", body)
	}

	// Viewing yourself skips the shared-group check.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(viewer.ID).WillReturnRows(handlerUserRows(viewer))
	expectProfileQueries(t, mock, viewer.ID, 0, 0, 0, 0, 0, 0, 0)
	requireStatus(t, api.GetPublicProfile, getPublicProfileRequest(viewer.ID, viewer.ID), http.StatusOK)
}

func getPublicProfileRequest(viewerID, targetID string) *http.Request {
	request := requestWithUser(http.MethodGet, "/", "", viewerID)
	request.SetPathValue("userID", targetID)
	return request
}

func TestProfileUpdateAndPasswordChange(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123"), 4)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: string(hash), Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	updated := &models.User{ID: user.ID, Username: "alice-new", Email: "alice-new@example.test", Password: string(hash), Avatar: "avatar2.png", CreatedAt: now, UpdatedAt: now}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs(updated.Username).WillReturnRows(handlerUserRows(updated))
	// The submitted email becomes a pending claim, not a replacement verified
	// address: no email-availability lookup runs and no verified address is
	// touched. Both updates and the returning read share one transaction.
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET username").WithArgs(updated.Username, updated.Avatar, user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE users").WithArgs(updated.Email, updated.Email, user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(updated))
	mock.ExpectCommit()
	recorder := httptest.NewRecorder()
	api.UpdateProfile(recorder, requestWithUser(http.MethodPatch, "/", `{"username":"alice-new","email":"alice-new@example.test","avatar":"avatar2.png","current_password":"Password123"}`, user.ID))
	if recorder.Code != http.StatusOK {
		t.Fatalf("profile update status = %d (%s)", recorder.Code, recorder.Body.String())
	}

	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(updated))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET password").WithArgs(pgxmock.AnyArg(), user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()
	recorder = httptest.NewRecorder()
	api.ChangePassword(recorder, requestWithUser(http.MethodPost, "/", `{"current_password":"Password123","new_password":"NewPassword123"}`, user.ID))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("password change status = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestProfileValidationBranches(t *testing.T) {
	api := newAuthAPI(t, newAuthMockPool(t), nil)
	recorder := httptest.NewRecorder()
	api.UpdateProfile(recorder, requestWithUser(http.MethodPost, "/", "{}", "user-1"))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("profile method status = %d", recorder.Code)
	}
	mock := newAuthMockPool(t)
	api = newAuthAPI(t, mock, nil)
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123"), 4)
	if err != nil {
		t.Fatal(err)
	}
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: string(hash)}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	requireStatus(t, api.UpdateProfile, requestWithUser(http.MethodPatch, "/", `{"username":"alice","email":"alice@example.test","avatar":"nope.png","current_password":"Password123"}`, user.ID), http.StatusBadRequest)
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	requireStatus(t, api.ChangePassword, requestWithUser(http.MethodPost, "/", `{"current_password":"Password123","new_password":"weak"}`, user.ID), http.StatusBadRequest)
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	requireStatus(t, api.UpdateProfile, requestWithUser(http.MethodPatch, "/", `{"username":"alice","email":"alice@example.test","avatar":"avatar.png","current_password":"WrongPassword123"}`, user.ID), http.StatusUnauthorized)
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	requireStatus(t, api.ChangePassword, requestWithUser(http.MethodPost, "/", `{"current_password":"WrongPassword123","new_password":"NewPassword123"}`, user.ID), http.StatusUnauthorized)
}

func TestProfileUpdateMapsPersistenceFailures(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123"), 4)
	if err != nil {
		t.Fatal(err)
	}
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: string(hash), Avatar: "avatar.png"}
	tests := []struct {
		name       string
		updateErr  error
		wantStatus int
	}{
		{name: "unique race", updateErr: &pgconn.PgError{Code: "23505"}, wantStatus: http.StatusConflict},
		{name: "database outage", updateErr: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mock := newAuthMockPool(t)
			api := newAuthAPI(t, mock, nil)
			mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
			mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs(user.Username).
				WillReturnRows(handlerUserRows(user))
			mock.ExpectBegin()
			mock.ExpectExec("UPDATE users SET username").WithArgs(user.Username, user.Avatar, user.ID).
				WillReturnError(test.updateErr)
			mock.ExpectRollback()
			requireStatus(t, api.UpdateProfile, requestWithUser(http.MethodPatch, "/", `{"username":"alice","avatar":"avatar.png","current_password":"Password123"}`, user.ID), test.wantStatus)
		})
	}
}

func TestProfileReturnsLifetimeProgression(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(score\\), 0\\), COUNT\\(\\*\\), COALESCE\\(AVG\\(score\\), 0\\)").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"total_points", "guess_count", "average_score"}).AddRow(int64(7600), int64(3), 2533.33))
	mock.ExpectQuery("WITH totals AS").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(int64(3), int64(1943)))
	mock.ExpectQuery("WITH scores AS").WithArgs(user.ID).WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(int64(7), int64(1943)))
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE ORDER BY`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}))
	recorder := httptest.NewRecorder()
	api.GetProfile(recorder, requestWithUser(http.MethodGet, "/", "", user.ID))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"name":"Lost Tourist"`) || !strings.Contains(recorder.Body.String(), `"total_points":7600`) || !strings.Contains(recorder.Body.String(), `"global_rank":{"rank":3,"total_players":1943}`) || !strings.Contains(recorder.Body.String(), `"average_score":2533.33`) || !strings.Contains(recorder.Body.String(), `"global_average_rank":{"rank":7,"total_players":1943}`) || !strings.Contains(recorder.Body.String(), `"elo":0`) {
		t.Fatalf("profile response = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

// TestEmailChangeKeepsVerifiedAddress proves changing the email records a
// pending claim while the current verified recovery address stays active: the
// response keeps the verified email and its verification state and adds the
// replacement as pending_email. No SQL path may clear email_verified_at.
func TestEmailChangeKeepsVerifiedAddress(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123"), 4)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	verified := now.Add(-24 * time.Hour)
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", EmailVerifiedAt: &verified, Password: string(hash), Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	afterUpdate := &models.User{ID: user.ID, Username: "alice", Email: "alice@example.test", EmailVerifiedAt: &verified, PendingEmail: "new@example.test", Password: string(hash), Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}

	// The same username lookup returns the user itself (not a collision), so
	// the profile update proceeds without any email-availability check.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs(user.Username).WillReturnRows(handlerUserRows(user))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE users SET username").WithArgs(user.Username, user.Avatar, user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE users").WithArgs("new@example.test", "new@example.test", user.ID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(afterUpdate))
	mock.ExpectCommit()

	recorder := httptest.NewRecorder()
	api.UpdateProfile(recorder, requestWithUser(http.MethodPatch, "/", `{"username":"alice","email":"new@example.test","avatar":"avatar.png","current_password":"Password123"}`, user.ID))
	if recorder.Code != http.StatusOK {
		t.Fatalf("email change status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"email":"alice@example.test"`) {
		t.Fatalf("verified email was not preserved: %s", body)
	}
	if !strings.Contains(body, `"pending_email":"new@example.test"`) {
		t.Fatalf("replacement claim missing from response: %s", body)
	}
	if !strings.Contains(body, "email_verified_at") {
		t.Fatalf("verification state missing from response: %s", body)
	}
}

type fakeIdentityAdmin struct {
	err     error
	issuer  string
	subject string
}

func (f *fakeIdentityAdmin) DeleteIdentity(_ context.Context, issuer, subject string) error {
	f.issuer = issuer
	f.subject = subject
	return f.err
}

func TestDeleteOIDCAccountPropagatesToKeycloakFirst(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	admin := &fakeIdentityAdmin{}
	api.oidcAdmin = admin
	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "!", Avatar: "avatar.png", OIDCLinked: true, CreatedAt: now, UpdatedAt: now}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectQuery("SELECT issuer, subject, email_at_link").WithArgs(user.ID).WillReturnRows(
		pgxmock.NewRows([]string{"issuer", "subject", "email_at_link"}).AddRow("https://login.example.test/realms/geoguessme", "subject-1", user.Email),
	)
	expectAccountCascadeDeletion(mock, user.ID)

	recorder := performDeleteAccount(api, user.ID, `{"confirmation":"alice"}`)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d %q", recorder.Code, recorder.Body.String())
	}
	if admin.issuer != "https://login.example.test/realms/geoguessme" || admin.subject != "subject-1" {
		t.Fatalf("deleted Keycloak identity = %q %q", admin.issuer, admin.subject)
	}
}

func TestDeleteOIDCAccountKeepsLocalDataWhenKeycloakFails(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	api.oidcAdmin = &fakeIdentityAdmin{err: errors.New("Keycloak unavailable")}
	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "!", Avatar: "avatar.png", OIDCLinked: true, CreatedAt: now, UpdatedAt: now}
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectQuery("SELECT issuer, subject, email_at_link").WithArgs(user.ID).WillReturnRows(
		pgxmock.NewRows([]string{"issuer", "subject", "email_at_link"}).AddRow("https://login.example.test/realms/geoguessme", "subject-1", user.Email),
	)

	recorder := performDeleteAccount(api, user.ID, `{"confirmation":"alice"}`)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("delete status = %d %q, want 502", recorder.Code, recorder.Body.String())
	}
}

func expectAccountCascadeDeletion(mock pgxmock.PgxPoolIface, userID string) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT storage_key FROM photos").WithArgs(userID).WillReturnRows(pgxmock.NewRows([]string{"storage_key"}))
	for _, table := range []string{"refresh_sessions", "email_verification_tokens", "password_reset_tokens", "websocket_tickets"} {
		mock.ExpectExec("DELETE FROM " + table).WithArgs(userID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	}
	mock.ExpectExec("DELETE FROM users").WithArgs(userID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()
}

func performDeleteAccount(api *AuthAPI, userID, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	api.DeleteAccount(recorder, requestWithUser(http.MethodDelete, "/", body, userID))
	return recorder
}
