package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSignup(t *testing.T) {
	// Setup
	// Note: In a real app we'd mock the database.
	// For now we assume the DB is running or we skip if not.
	// Actually, unit tests shouldn't depend on external DB.
	// But since I haven't set up dependency injection for the DB,
	// I will write a test that mocks the request structure but might fail if DB isn't there.
	// Ideally we refactor to use interfaces.

	// For this task, I'll focus on the handler signature and bad request validation
	// which doesn't need DB.

	t.Run("Invalid Payload", func(t *testing.T) {
		reqBody := []byte(`{"username": ""}`) // Missing password
		req, _ := http.NewRequest("POST", "/signup", bytes.NewBuffer(reqBody))
		rr := httptest.NewRecorder()

		handler := http.HandlerFunc(Signup)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}
