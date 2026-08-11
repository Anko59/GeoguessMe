package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

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

	// Duplicate username collides with the generic conflict.
	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"username": user, "email": "other" + email, "password": "StrongPassword123"}, "", nil)
	require.Equalf(t, http.StatusConflict, resp.StatusCode, "duplicate username: %s", data)
	usernameConflict := data

	// The same address is claimable by another account while it is still an
	// unverified pending claim: signup must not reveal registration status.
	resp, data = doJSON(t, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"username": user + "2", "email": email, "password": "StrongPassword123"}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "second pending claim: %s", data)

	// Once one account verifies the address, signup still accepts it as a
	// pending claim. Verification—not registration—decides ownership, so this
	// response cannot enumerate verified recovery addresses.
	verifyToken := tokensFromMailpitTo(t, "Verify your GeoGuessMe email", "/verify-email", email, 1)[0]
	resp, data = doJSON(t, http.MethodPost, "/api/v1/auth/verify", map[string]string{"token": verifyToken}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "verify: %s", data)
	resp, data = doJSON(t, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"username": user + "3", "email": email, "password": "StrongPassword123"}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "verified email pending claim: %s", data)
	require.NotEqual(t, usernameConflict, data)
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

	// Recovery only ever targets verified addresses, so the account must be
	// verified before a reset link can be issued for its address.
	verifyToken := tokenFromMailpit(t, "Verify your GeoGuessMe email", "/verify-email")
	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/verify", map[string]string{"token": verifyToken}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "verify: %s", data)

	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/password/forgot", map[string]string{"email": email}, "", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	token := tokenFromMailpit(t, "Reset your GeoGuessMe password", "/reset-password")
	resp, data = doJSON(t, http.MethodPost, "/api/v1/auth/password/reset",
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

func TestProfileUpdateAndPasswordChange(t *testing.T) {
	resetRateLimiter(t)
	user := unique("profile")
	email := user + "@example.test"
	const pass = "StrongPassword123"
	session := signup(t, user, email, pass)

	updatedUsername := user + "new"
	updatedEmail := updatedUsername + "@example.test"
	resp, data := doJSON(t, http.MethodPatch, "/api/v1/auth/profile", map[string]string{
		"username":         updatedUsername,
		"email":            updatedEmail,
		"avatar":           "avatar2.png",
		"current_password": pass,
	}, session.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "profile update: %s", data)
	var profile struct {
		Username     string `json:"username"`
		Email        string `json:"email"`
		PendingEmail string `json:"pending_email"`
		Avatar       string `json:"avatar"`
	}
	require.NoError(t, json.Unmarshal(data, &profile))
	require.Equal(t, updatedUsername, profile.Username)
	// The submitted email becomes a pending claim; the account is still
	// unverified so no verified email exists yet.
	require.Equal(t, "", profile.Email)
	require.Equal(t, updatedEmail, profile.PendingEmail)
	require.Equal(t, "avatar2.png", profile.Avatar)

	resp, data = doJSON(t, http.MethodGet, "/api/v1/auth/profile", nil, session.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "profile read: %s", data)
	var progression struct {
		TotalPoints int `json:"total_points"`
		GuessCount  int `json:"guess_count"`
		Rank        struct {
			Level int    `json:"level"`
			Name  string `json:"name"`
		} `json:"rank"`
		GlobalRank struct {
			Rank         int `json:"rank"`
			TotalPlayers int `json:"total_players"`
		} `json:"global_rank"`
	}
	require.NoError(t, json.Unmarshal(data, &progression))
	require.Equal(t, 0, progression.TotalPoints)
	require.Equal(t, 0, progression.GuessCount)
	require.Equal(t, 1, progression.Rank.Level)
	require.Equal(t, "Completely Lost", progression.Rank.Name)
	// A player who never guessed is not part of the ranked population.
	require.Equal(t, 0, progression.GlobalRank.Rank)

	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/password/change", map[string]string{
		"current_password": pass,
		"new_password":     "NewStrongPassword123",
	}, session.access, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/user/groups", nil, session.access, nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", []*http.Cookie{session.refresh})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": updatedUsername,
		"password": "NewStrongPassword123",
	}, "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
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

// TestPendingEmailClaimsPromoteFirstAndFailSecondGenerically proves multiple
// accounts may hold the same pending address, the first claim to verify wins,
// and the losing claim gets a generic verification failure that reveals
// neither the conflict nor the owning account.
func TestPendingEmailClaimsPromoteFirstAndFailSecondGenerically(t *testing.T) {
	resetRateLimiter(t)
	sharedEmail := unique("claim") + "@example.test"
	signup(t, unique("claimant-a"), sharedEmail, "StrongPassword123")
	signup(t, unique("claimant-b"), sharedEmail, "StrongPassword123")

	// Both signups succeeded: a pending claim never collides with another
	// pending claim, only with a verified address. Collect the two distinct
	// verification tokens addressed to the shared claim (one per claimant).
	tokens := tokensFromMailpitTo(t, "Verify your GeoGuessMe email", "/verify-email", sharedEmail, 2)
	require.Len(t, tokens, 2)

	// Whichever claimant verifies first promotes the shared address; the other
	// claimant's verification must then fail generically.
	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/verify", map[string]string{"token": tokens[0]}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "first verify: %s", data)

	resp, data = doJSON(t, http.MethodPost, "/api/v1/auth/verify", map[string]string{"token": tokens[1]}, "", nil)
	require.Equalf(t, http.StatusBadRequest, resp.StatusCode, "second verify: %s", data)
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(data, &envelope))
	require.Equal(t, "verification_failed", envelope.Error.Code)
	require.NotContains(t, string(data), sharedEmail)
}

// TestForgotPasswordRequiresVerifiedEmail proves password recovery only acts
// on confirmed addresses: an unverified (pending) address receives the uniform
// 202 but never a reset link, while the verified address receives one.
func TestForgotPasswordRequiresVerifiedEmail(t *testing.T) {
	resetRateLimiter(t)
	user := unique("recover")
	email := user + "@example.test"
	const pass = "StrongPassword123"
	signup(t, user, email, pass)

	// Pending-only address: uniform 202, no reset mail is ever addressed to it.
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/password/forgot", map[string]string{"email": email}, "", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	require.False(t, mailpitHasResetTo(t, email), "reset mail must not be sent to an unverified address")

	// Verify the address, then recovery issues a reset link for it.
	verifyToken := tokensFromMailpitTo(t, "Verify your GeoGuessMe email", "/verify-email", email, 1)[0]
	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/verify", map[string]string{"token": verifyToken}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "verify: %s", data)

	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/password/forgot", map[string]string{"email": email}, "", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	tokenFromMailpit(t, "Reset your GeoGuessMe password", "/reset-password")

	// An unknown address also gets the uniform 202.
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/password/forgot", map[string]string{"email": unique("nobody") + "@example.test"}, "", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
}

// TestEmailChangeKeepsVerifiedRecovery proves a verified recovery address stays
// active while a replacement is pending, and the replacement only takes effect
// after it verifies.
func TestEmailChangeKeepsVerifiedRecovery(t *testing.T) {
	resetRateLimiter(t)
	user := unique("email-switch")
	oldEmail := user + "@example.test"
	newEmail := user + "-new@example.test"
	const pass = "StrongPassword123"
	session := signup(t, user, oldEmail, pass)

	// Verify the original address.
	verifyToken := tokensFromMailpitTo(t, "Verify your GeoGuessMe email", "/verify-email", oldEmail, 1)[0]
	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/verify", map[string]string{"token": verifyToken}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "verify: %s", data)

	// Change the profile email: it becomes a pending claim and the verified
	// address stays active, so the owner DTO still carries the old email.
	resp, data = doJSON(t, http.MethodPatch, "/api/v1/auth/profile", map[string]string{
		"username":         user,
		"email":            newEmail,
		"avatar":           "avatar.png",
		"current_password": pass,
	}, session.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "profile update: %s", data)
	var profile struct {
		Email        string `json:"email"`
		PendingEmail string `json:"pending_email"`
	}
	require.NoError(t, json.Unmarshal(data, &profile))
	require.Equal(t, oldEmail, profile.Email)
	require.Equal(t, newEmail, profile.PendingEmail)

	// Recovery for the old (still verified) address still works.
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/password/forgot", map[string]string{"email": oldEmail}, "", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	tokenFromMailpit(t, "Reset your GeoGuessMe password", "/reset-password")
	resetCountBefore := mailpitResetCountTo(t, oldEmail)

	// Request verification for the pending replacement, then verify it: the
	// new address becomes the verified email and the old one stops being
	// recoverable.
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/verify/request", nil, session.access, nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	verifyToken = tokensFromMailpitTo(t, "Verify your GeoGuessMe email", "/verify-email", newEmail, 1)[0]
	resp, data = doJSON(t, http.MethodPost, "/api/v1/auth/verify", map[string]string{"token": verifyToken}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "replacement verify: %s", data)
	resp, data = doJSON(t, http.MethodGet, "/api/v1/auth/profile", nil, session.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "profile read: %s", data)
	// Unmarshal into a fresh struct: a cleared pending_email is omitted from
	// the JSON (omitempty), and reusing the earlier struct would retain the
	// stale value from the profile-update response.
	var replaced struct {
		Email        string `json:"email"`
		PendingEmail string `json:"pending_email"`
	}
	require.NoError(t, json.Unmarshal(data, &replaced))
	require.Equal(t, newEmail, replaced.Email)
	require.Equal(t, "", replaced.PendingEmail)

	// The replaced address must no longer be recoverable: recovery answers
	// with the uniform 202 but never addresses a new reset message to it.
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/password/forgot", map[string]string{"email": oldEmail}, "", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	require.Equal(t, resetCountBefore, mailpitResetCountTo(t, oldEmail), "replaced address must no longer be recoverable")

	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/password/forgot", map[string]string{"email": newEmail}, "", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	tokenFromMailpit(t, "Reset your GeoGuessMe password", "/reset-password")
}

// --- Mailpit helpers for contact-claim assertions -------------------------

// mailpitMessageDetail fetches a message body with its recipients. The
// transactional sender puts the recipient in Bcc, so all three fields are
// checked when matching an address.
type mailpitMessageDetail struct {
	Text string `json:"Text"`
	To   []struct {
		Address string `json:"Address"`
	} `json:"To"`
	Cc []struct {
		Address string `json:"Address"`
	} `json:"Cc"`
	Bcc []struct {
		Address string `json:"Address"`
	} `json:"Bcc"`
}

// mailpitGet performs a context-aware GET against Mailpit (noctx-compliant).
func mailpitGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func fetchMailpitMessage(t *testing.T, id string) mailpitMessageDetail {
	t.Helper()
	resp, err := mailpitGet(context.Background(), mailpitBase()+"/api/v1/message/"+id)
	if err != nil {
		t.Fatalf("mailpit message fetch: %v", err)
	}
	defer resp.Body.Close()
	var detail mailpitMessageDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("mailpit message decode: %v", err)
	}
	return detail
}

func mailpitMessageAddresses(detail mailpitMessageDetail, address string) bool {
	for _, recipient := range append(append(detail.To, detail.Cc...), detail.Bcc...) {
		if strings.EqualFold(recipient.Address, address) {
			return true
		}
	}
	return false
}

// tokensFromMailpitTo returns every distinct token extracted from messages
// with the subject that are addressed to the given recipient, waiting until at
// least want tokens arrive. Recipients are unique per test, so filtering by
// recipient keeps assertions deterministic even though mailpit is a shared
// inbox across the suite.
func tokensFromMailpitTo(t *testing.T, subject, linkPath, toEmail string, want int) []string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	queryURL, _ := url.Parse(mailpitBase() + "/api/v1/search")
	query := queryURL.Query()
	query.Set("query", `subject:"`+subject+`"`)
	queryURL.RawQuery = query.Encode()
	var tokens []string
	for time.Now().Before(deadline) {
		resp, err := mailpitGet(context.Background(), queryURL.String())
		if err == nil {
			var summary struct {
				Messages []struct {
					ID string `json:"ID"`
				} `json:"messages"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&summary)
			_ = resp.Body.Close()
			tokens = tokens[:0]
			for _, message := range summary.Messages {
				detail := fetchMailpitMessage(t, message.ID)
				if !mailpitMessageAddresses(detail, toEmail) {
					continue
				}
				if token := extractToken(detail.Text, linkPath); token != "" && !tokenListContains(tokens, token) {
					tokens = append(tokens, token)
				}
			}
			if len(tokens) >= want {
				return tokens
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return tokens
}

// mailpitHasResetTo reports whether any reset email is addressed to the given
// address. Recipients are unique per test, so this is deterministic.
func mailpitHasResetTo(t *testing.T, email string) bool {
	t.Helper()
	return mailpitResetCountTo(t, email) > 0
}

// mailpitResetCountTo counts reset messages currently addressed to the given
// address.
func mailpitResetCountTo(t *testing.T, email string) int {
	t.Helper()
	queryURL, _ := url.Parse(mailpitBase() + "/api/v1/search")
	query := queryURL.Query()
	query.Set("query", `subject:"Reset your GeoGuessMe password"`)
	queryURL.RawQuery = query.Encode()
	resp, err := mailpitGet(context.Background(), queryURL.String())
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var summary struct {
		Messages []struct {
			ID string `json:"ID"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return 0
	}
	count := 0
	for _, message := range summary.Messages {
		if mailpitMessageAddresses(fetchMailpitMessage(t, message.ID), email) {
			count++
		}
	}
	return count
}

func tokenListContains(tokens []string, token string) bool {
	for _, existing := range tokens {
		if existing == token {
			return true
		}
	}
	return false
}
