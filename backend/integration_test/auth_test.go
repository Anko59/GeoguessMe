package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignupValidation(t *testing.T) {
	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"username": "ab", "email": "bad", "password": "weak"}, "", nil)
	require.Equalf(t, http.StatusBadRequest, resp.StatusCode, "body: %s", data)
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(data, &envelope))
	require.NotEmpty(t, envelope.Error.Code)
}

func TestProtectedRouteRequiresAuth(t *testing.T) {
	resp, _ := doJSON(t, http.MethodGet, "/api/v1/user/groups", nil, "", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestSignupLoginAndDuplicate(t *testing.T) {
	resetRateLimiter(t)
	user := unique("alice")
	email := user + "@example.test"
	first := signup(t, user, email, "StrongPassword123")

	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"username": user, "email": "other" + email, "password": "StrongPassword123"}, "", nil)
	require.Equalf(t, http.StatusConflict, resp.StatusCode, "duplicate username: %s", data)

	resp, data = doJSON(t, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"username": user + "2", "email": email, "password": "StrongPassword123"}, "", nil)
	require.Equalf(t, http.StatusConflict, resp.StatusCode, "duplicate email: %s", data)

	// Reset rate limiter so login requests aren't throttled by the preceding
	// signup calls that shared the same identity key.
	resetRateLimiter(t)

	// Login with correct credentials succeeds; wrong password does not.
	resp, data = doJSON(t, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"username": user, "password": "StrongPassword123"}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "login: %s", data)

	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"username": user, "password": "WrongPassword123"}, "", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Authenticated protected route works.
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/user/groups", nil, first.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPasswordResetRevokesSessions(t *testing.T) {
	resetRateLimiter(t)
	user := unique("resetter")
	email := user + "@example.test"
	const pass = "StrongPassword123"
	session := signup(t, user, email, pass)

	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/password/forgot", map[string]string{"email": email}, "", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	token := tokenFromMailpit(t, email, "/reset-password")
	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/password/reset",
		map[string]string{"token": token, "password": "BrandNewPassword123"}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "reset: %s", data)

	// The pre-reset refresh session is revoked.
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", []*http.Cookie{session.refresh})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Login with the new password works; the old one does not.
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login", map[string]string{"username": user, "password": "BrandNewPassword123"}, "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login", map[string]string{"username": user, "password": pass}, "", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAccountDeletionImmediateLossAndReuse(t *testing.T) {
	resetRateLimiter(t)
	user := unique("deleter")
	email := user + "@example.test"
	const pass = "StrongPassword123"
	session := signup(t, user, email, pass)

	resp, _ := doJSON(t, http.MethodDelete, "/api/v1/auth/account",
		map[string]string{"password": pass}, session.access, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// The access token is rejected immediately even though it has not expired.
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/user/groups", nil, session.access, nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Identity can be reused.
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"username": user, "email": email, "password": pass}, "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRefreshRotationSingleUse(t *testing.T) {
	resetRateLimiter(t)
	user := unique("rotate")
	email := user + "@example.test"
	session := signup(t, user, email, "StrongPassword123")

	// First refresh consumes the old session, issues a new one.
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", []*http.Cookie{session.refresh})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// The old refresh token is now consumed. Second attempt fails.
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", []*http.Cookie{session.refresh})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
