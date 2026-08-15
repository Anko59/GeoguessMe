package auth

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authsvc "geoguessme/internal/auth"
	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

type fakeIdentityVerifier struct {
	identity authsvc.ExternalIdentity
	err      error
}

func (f fakeIdentityVerifier) VerifyIdentity(context.Context, string) (authsvc.ExternalIdentity, error) {
	return f.identity, f.err
}

func oidcFixture() authsvc.ExternalIdentity {
	return authsvc.ExternalIdentity{
		Issuer: "https://login.example.test/realms/geoguessme", Subject: "subject-1",
		Email: "alice@example.test", EmailVerified: true, PreferredUsername: "Alice.Example",
	}
}

func newOIDCTestAPI(t *testing.T, mock pgxmock.PgxPoolIface) *AuthAPI {
	t.Helper()
	api := newAuthAPI(t, mock, nil)
	api.cfg.OIDCEnabled = true
	api.cfg.OIDCIssuerURL = oidcFixture().Issuer
	api.oidc = fakeIdentityVerifier{identity: oidcFixture()}
	return api
}

func TestOIDCConfigAndExistingIdentityExchange(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newOIDCTestAPI(t, mock)
	api.cfg.OIDCSocialProviders = []string{"google", "apple", "github"}

	configRecorder := httptest.NewRecorder()
	api.OIDCConfig(configRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if configRecorder.Code != http.StatusOK || configRecorder.Body.String() != "{\"account_url\":\"https://login.example.test/realms/geoguessme/account/\",\"enabled\":true,\"login_path\":\"/oauth2/start\",\"social_providers\":[\"google\",\"apple\",\"github\"]}\n" {
		t.Fatalf("OIDC config = %d %q", configRecorder.Code, configRecorder.Body.String())
	}

	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Password: "!", Avatar: "avatar.png", OIDCLinked: true, CreatedAt: now, UpdatedAt: now}
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT user_id FROM user_identities").WithArgs(oidcFixture().Issuer, oidcFixture().Subject).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectExec("UPDATE user_identities SET last_login_at").WithArgs(pgxmock.AnyArg(), oidcFixture().Issuer, oidcFixture().Subject).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header.Set("Authorization", "Bearer forwarded-keycloak-token")
	api.ExchangeOIDCSession(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Set-Cookie") == "" {
		t.Fatalf("exchange = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestOIDCExchangeRequiresAndAcceptsChosenUsername(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newOIDCTestAPI(t, mock)
	expectNewOIDCIdentityLookup(mock, false)
	mock.ExpectRollback()

	recorder := httptest.NewRecorder()
	api.ExchangeOIDCSession(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"username_required"`) {
		t.Fatalf("missing username exchange = %d %q", recorder.Code, recorder.Body.String())
	}

	now := time.Now().UTC()
	user := &models.User{ID: "new-user", Username: "map-master", Email: oidcFixture().Email, Password: "!", Avatar: "avatar.png", OIDCLinked: true, CreatedAt: now, UpdatedAt: now}
	expectNewOIDCIdentityLookup(mock, false)
	mock.ExpectExec("INSERT INTO users").WithArgs(pgxmock.AnyArg(), user.Username, user.Email, user.Email, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO user_identities").WithArgs(oidcFixture().Issuer, oidcFixture().Subject, pgxmock.AnyArg(), oidcFixture().Email, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(pgxmock.AnyArg()).WillReturnRows(handlerUserRows(user))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":" map-master "}`))
	api.ExchangeOIDCSession(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"username":"map-master"`) {
		t.Fatalf("chosen username exchange = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestOIDCExchangeRejectsInvalidUsernameBeforeIdentityLookup(t *testing.T) {
	api := newOIDCTestAPI(t, newAuthMockPool(t))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":"not valid"}`))
	api.ExchangeOIDCSession(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid_username"`) {
		t.Fatalf("invalid username exchange = %d %q", recorder.Code, recorder.Body.String())
	}
}

func expectNewOIDCIdentityLookup(mock pgxmock.PgxPoolIface, pending bool) {
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT user_id FROM user_identities").WithArgs(oidcFixture().Issuer, oidcFixture().Subject).WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("email:alice@example.test").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT id FROM users WHERE email_normalized").WithArgs("alice@example.test").WillReturnError(pgx.ErrNoRows)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("alice@example.test").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(pending))
}

func TestOIDCExchangeRequiresExplicitLinkForPendingEmail(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newOIDCTestAPI(t, mock)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT user_id FROM user_identities").WithArgs(oidcFixture().Issuer, oidcFixture().Subject).WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("email:alice@example.test").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT id FROM users WHERE email_normalized").WithArgs("alice@example.test").WillReturnError(pgx.ErrNoRows)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("alice@example.test").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	recorder := httptest.NewRecorder()
	api.ExchangeOIDCSession(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
	if recorder.Code != http.StatusConflict || recorder.Body.String() == "" {
		t.Fatalf("pending-email exchange = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestOIDCExchangeAutoLinksExactVerifiedEmailWithoutChangingUserID(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newOIDCTestAPI(t, mock)
	now := time.Now().UTC()
	user := &models.User{
		ID: "legacy-user-1", Username: "alice", Email: "alice@example.test",
		Password: "hash", PasswordEnabled: true, Avatar: "avatar.png", OIDCLinked: true,
		AuthVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT user_id FROM user_identities").WithArgs(oidcFixture().Issuer, oidcFixture().Subject).WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("email:alice@example.test").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT id FROM users WHERE email_normalized").WithArgs("alice@example.test").WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(user.ID))
	mock.ExpectExec("INSERT INTO user_identities").WithArgs(oidcFixture().Issuer, oidcFixture().Subject, user.ID, oidcFixture().Email, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE users SET auth_version").WithArgs(user.ID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs(user.ID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	recorder := httptest.NewRecorder()
	api.ExchangeOIDCSession(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("verified-email exchange = %d %q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"id":"legacy-user-1"`) {
		t.Fatalf("verified-email exchange changed canonical user: %s", recorder.Body.String())
	}
}

func TestOIDCExplicitLinkIntentIsConsumed(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newOIDCTestAPI(t, mock)
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM oidc_link_intents").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("DELETE", 0))
	mock.ExpectExec("INSERT INTO oidc_link_intents").WithArgs(pgxmock.AnyArg(), "user-1", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	startRecorder := httptest.NewRecorder()
	api.StartOIDCLink(startRecorder, requestWithUser(http.MethodPost, "/", "", "user-1"))
	if startRecorder.Code != http.StatusNoContent {
		t.Fatalf("start link = %d %q", startRecorder.Code, startRecorder.Body.String())
	}
	linkCookie := startRecorder.Result().Cookies()[0]

	now := time.Now().UTC()
	user := &models.User{ID: "user-1", Username: "alice", Password: "hash", PasswordEnabled: true, Avatar: "avatar.png", OIDCLinked: true, AuthVersion: 1, CreatedAt: now, UpdatedAt: now}
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("UPDATE oidc_link_intents SET used_at").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(user.ID))
	mock.ExpectQuery("SELECT user_id FROM user_identities").WithArgs(oidcFixture().Issuer, oidcFixture().Subject).WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec("INSERT INTO user_identities").WithArgs(oidcFixture().Issuer, oidcFixture().Subject, user.ID, oidcFixture().Email, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE users SET auth_version").WithArgs(user.ID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE refresh_sessions SET revoked_at").WithArgs(user.ID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM websocket_tickets").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	exchangeRecorder := httptest.NewRecorder()
	exchangeRequest := httptest.NewRequest(http.MethodPost, "/", nil)
	exchangeRequest.AddCookie(linkCookie)
	api.ExchangeOIDCSession(exchangeRecorder, exchangeRequest)
	if exchangeRecorder.Code != http.StatusOK || len(exchangeRecorder.Result().Cookies()) < 2 {
		t.Fatalf("linked exchange = %d %q", exchangeRecorder.Code, exchangeRecorder.Body.String())
	}
}
