// Contact-claim tests (F-09): pending email claims, verified-only recovery,
// generic collision handling, and the owner-only contact DTO.

package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
)

// TestSignupDoesNotRevealVerifiedEmail proves an address already verified by
// another account remains claimable as pending at signup. Ownership is decided
// only by verification, so account creation cannot enumerate recovery emails.
func TestSignupDoesNotRevealVerifiedEmail(t *testing.T) {
	now := time.Now().UTC()
	existing := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
	signupBody := `{"username":"alice","email":"alice@example.test","password":"StrongPassword123"}`

	// Duplicate username remains a conflict because usernames are public identity.
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs("alice").WillReturnRows(handlerUserRows(existing))
	usernameRecorder := httptest.NewRecorder()
	api.Signup(usernameRecorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(signupBody)))
	if usernameRecorder.Code != http.StatusConflict {
		t.Fatalf("duplicate username status = %d (%s)", usernameRecorder.Code, usernameRecorder.Body.String())
	}

	// The same verified address with a fresh username succeeds as a pending
	// claim and sends a verification link without looking up its owner.
	mock = newAuthMockPool(t)
	api = newAuthAPI(t, mock, nil)
	mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs("bob").WillReturnRows(pgxmock.NewRows(userColumnsForQuery()))
	mock.ExpectExec("INSERT INTO users").WithArgs(pgxmock.AnyArg(), "bob", "alice@example.test", "alice@example.test", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM email_verification_tokens").WithArgs(pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("DELETE", 0))
	mock.ExpectExec("INSERT INTO email_verification_tokens").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "alice@example.test", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	emailRecorder := httptest.NewRecorder()
	api.Signup(emailRecorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":"bob","email":"alice@example.test","password":"StrongPassword123"}`)))
	if emailRecorder.Code != http.StatusOK {
		t.Fatalf("verified email claim status = %d (%s)", emailRecorder.Code, emailRecorder.Body.String())
	}

	if !strings.Contains(usernameRecorder.Body.String(), "signup_unavailable") || strings.Contains(usernameRecorder.Body.String(), "taken") {
		t.Fatalf("username collision response is not generic: %s", usernameRecorder.Body.String())
	}
}

func TestSignupAllowsNoRecoveryEmail(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs("emailfree").WillReturnRows(pgxmock.NewRows(userColumnsForQuery()))
	mock.ExpectExec("INSERT INTO users").WithArgs(pgxmock.AnyArg(), "emailfree", nil, nil, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	// No verification token is issued without a contact claim.
	mock.ExpectExec("INSERT INTO refresh_sessions").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	recorder := httptest.NewRecorder()
	api.Signup(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":"emailfree","password":"StrongPassword123"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("email-free signup status = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestSignupMapsInsertFailures(t *testing.T) {
	tests := []struct {
		name       string
		insertErr  error
		wantStatus int
	}{
		{name: "unique race", insertErr: &pgconn.PgError{Code: "23505"}, wantStatus: http.StatusConflict},
		{name: "database outage", insertErr: &pgconn.PgError{Code: "08006"}, wantStatus: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mock := newAuthMockPool(t)
			api := newAuthAPI(t, mock, nil)
			mock.ExpectQuery("SELECT .*FROM users WHERE username").WithArgs("alice").WillReturnRows(pgxmock.NewRows(userColumnsForQuery()))
			mock.ExpectExec("INSERT INTO users").
				WithArgs(pgxmock.AnyArg(), "alice", "alice@example.test", "alice@example.test", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
				WillReturnError(test.insertErr)
			recorder := httptest.NewRecorder()
			api.Signup(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"username":"alice","email":"alice@example.test","password":"StrongPassword123"}`)))
			if recorder.Code != test.wantStatus {
				t.Fatalf("signup status = %d (%s), want %d", recorder.Code, recorder.Body.String(), test.wantStatus)
			}
		})
	}
}

// TestVerifyEmailClaimConflictIsGeneric proves a claim conflict surfaces as a
// generic verification error that reveals neither the conflict nor the owning
// account.
func TestVerifyEmailClaimConflictIsGeneric(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"user_id", "target_email_normalized"}).AddRow("user-1", "taken@example.test"))
	mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"pending_email", "pending_email_normalized"}).AddRow("taken@example.test", "taken@example.test"))
	mock.ExpectQuery("SELECT EXISTS").WithArgs("taken@example.test", "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	recorder := httptest.NewRecorder()
	api.VerifyEmail(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"token":"conflict-token"}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("claim conflict status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "verification_failed") || strings.Contains(recorder.Body.String(), "taken") {
		t.Fatalf("claim conflict must be a generic verification error: %s", recorder.Body.String())
	}
}

// TestVerifyEmailNothingToPromoteIsIdempotent proves verifying an
// already-verified account without a pending claim succeeds.
func TestVerifyEmailNothingToPromoteIsIdempotent(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"user_id", "target_email_normalized"}).AddRow("user-1", "alice@example.test"))
	mock.ExpectQuery("SELECT pending_email, pending_email_normalized FROM users").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"pending_email", "pending_email_normalized"}).AddRow(nil, nil))
	mock.ExpectQuery("SELECT email_normalized FROM users").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"email_normalized"}).AddRow("alice@example.test"))
	mock.ExpectCommit()
	recorder := httptest.NewRecorder()
	api.VerifyEmail(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"token":"already-verified-token"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("nothing-to-promote verify status = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestVerifyEmailDatabaseFailureIsInternal(t *testing.T) {
	mock := newAuthMockPool(t)
	api := newAuthAPI(t, mock, nil)
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE email_verification_tokens").WithArgs(pgxmock.AnyArg()).
		WillReturnError(&pgconn.PgError{Code: "08006"})
	mock.ExpectRollback()
	recorder := httptest.NewRecorder()
	api.VerifyEmail(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"token":"verification-token"}`)))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("verification outage status = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

// TestForgotPasswordRecoveryByEmailState proves verified addresses receive a
// reset link while pending addresses receive a verification link first. Both
// cases retain the uniform 202 response used to prevent email enumeration.
func TestForgotPasswordRecoveryByEmailState(t *testing.T) {
	now := time.Now().UTC()

	t.Run("Verified Address", func(t *testing.T) {
		mock := newAuthMockPool(t)
		api := newAuthAPI(t, mock, nil)
		verified := now.Add(-time.Hour)
		user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", EmailVerifiedAt: &verified, Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
		mock.ExpectQuery("SELECT .*FROM users WHERE email_normalized").WithArgs("alice@example.test").WillReturnRows(handlerUserRows(user))
		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM password_reset_tokens").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectExec("INSERT INTO password_reset_tokens").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		recorder := httptest.NewRecorder()
		api.ForgotPassword(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"email":"alice@example.test"}`)))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("verified recovery status = %d", recorder.Code)
		}
	})

	t.Run("Pending Address", func(t *testing.T) {
		mock := newAuthMockPool(t)
		api := newAuthAPI(t, mock, nil)
		mock.ExpectQuery("SELECT .*FROM users WHERE email_normalized").WithArgs("alice@example.test").WillReturnRows(pgxmock.NewRows(userColumnsForQuery()))
		pending := &models.User{ID: "user-1", Username: "alice", PendingEmail: "alice@example.test", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
		mock.ExpectQuery("pending_email_normalized").WithArgs("alice@example.test").WillReturnRows(handlerUserRows(pending))
		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM email_verification_tokens").WithArgs(pending.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectExec("INSERT INTO email_verification_tokens").WithArgs(pgxmock.AnyArg(), pending.ID, pgxmock.AnyArg(), "alice@example.test", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		recorder := httptest.NewRecorder()
		api.ForgotPassword(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"email":"alice@example.test"}`)))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("pending recovery status = %d", recorder.Code)
		}
	})

	t.Run("Unknown Address", func(t *testing.T) {
		mock := newAuthMockPool(t)
		api := newAuthAPI(t, mock, nil)
		mock.ExpectQuery("SELECT .*FROM users WHERE email_normalized").WithArgs("nobody@example.test").WillReturnRows(pgxmock.NewRows(userColumnsForQuery()))
		mock.ExpectQuery("pending_email_normalized").WithArgs("nobody@example.test").WillReturnRows(pgxmock.NewRows(userColumnsForQuery()))
		recorder := httptest.NewRecorder()
		api.ForgotPassword(recorder, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"email":"nobody@example.test"}`)))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("unknown recovery status = %d", recorder.Code)
		}
	})
}

// TestRequestVerificationTargetsPending proves resend only targets the current
// pending claim and is a silent no-op for a fully verified account.
func TestRequestVerificationTargetsPending(t *testing.T) {
	now := time.Now().UTC()
	verified := now.Add(-time.Hour)

	t.Run("Unverified With Pending", func(t *testing.T) {
		mock := newAuthMockPool(t)
		api := newAuthAPI(t, mock, nil)
		user := &models.User{ID: "user-1", Username: "alice", PendingEmail: "alice@example.test", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
		mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM email_verification_tokens").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectExec("INSERT INTO email_verification_tokens").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), "alice@example.test", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		recorder := httptest.NewRecorder()
		api.RequestVerification(recorder, requestWithUser(http.MethodPost, "/", "", user.ID))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("unverified resend status = %d", recorder.Code)
		}
	})

	t.Run("Verified With Replacement Pending", func(t *testing.T) {
		mock := newAuthMockPool(t)
		api := newAuthAPI(t, mock, nil)
		user := &models.User{ID: "user-1", Username: "alice", Email: "old@example.test", EmailVerifiedAt: &verified, PendingEmail: "new@example.test", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
		mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM email_verification_tokens").WithArgs(user.ID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectExec("INSERT INTO email_verification_tokens").WithArgs(pgxmock.AnyArg(), user.ID, pgxmock.AnyArg(), "new@example.test", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		recorder := httptest.NewRecorder()
		api.RequestVerification(recorder, requestWithUser(http.MethodPost, "/", "", user.ID))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("replacement resend status = %d", recorder.Code)
		}
	})

	t.Run("Verified Without Pending Is A Noop", func(t *testing.T) {
		mock := newAuthMockPool(t)
		api := newAuthAPI(t, mock, nil)
		user := &models.User{ID: "user-1", Username: "alice", Email: "alice@example.test", EmailVerifiedAt: &verified, Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
		mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(user.ID).WillReturnRows(handlerUserRows(user))
		recorder := httptest.NewRecorder()
		api.RequestVerification(recorder, requestWithUser(http.MethodPost, "/", "", user.ID))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("noop resend status = %d", recorder.Code)
		}
	})
}

// TestAuthUserResponseExposesOnlyOwnerContactFields proves the owner-only DTO
// carries a nullable verified email, the optional pending claim, and the
// verification state, while the public profile shape never carries either
// address.
func TestAuthUserResponseExposesOnlyOwnerContactFields(t *testing.T) {
	now := time.Now().UTC()
	verified := now.Add(-time.Hour)

	verifiedEmail := "old@example.test"
	pending := "new@example.test"
	user := &models.User{ID: "user-1", Username: "alice", Email: verifiedEmail, EmailVerifiedAt: &verified, PendingEmail: pending, Avatar: "avatar.png"}
	raw, err := json.Marshal(userResponse(user))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `"email":"old@example.test"`) || !strings.Contains(body, `"pending_email":"new@example.test"`) || !strings.Contains(body, "email_verified_at") {
		t.Fatalf("owner DTO missing contact fields: %s", body)
	}

	// An unverified account has no email key at all; only the pending claim.
	pendingOnly := &models.User{ID: "user-2", Username: "bob", PendingEmail: "bob@example.test", Avatar: "avatar.png"}
	raw, err = json.Marshal(userResponse(pendingOnly))
	if err != nil {
		t.Fatal(err)
	}
	body = string(raw)
	if strings.Contains(body, `"email":`) || !strings.Contains(body, `"pending_email":"bob@example.test"`) {
		t.Fatalf("pending-only DTO shape = %s", body)
	}

	// The public profile response never exposes either address.
	public := PublicProfileResponse{ID: "user-3", Username: "carol", Avatar: "avatar.png"}
	raw, err = json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	body = string(raw)
	if strings.Contains(body, "email") {
		t.Fatalf("public profile leaks contact fields: %s", body)
	}
}
