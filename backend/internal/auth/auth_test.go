package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func TestPasswordHashing(t *testing.T) {
	password := "SecretPass123"

	// Test Hash
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	assert.NoError(t, err)
	hash := string(hashBytes)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)

	// Test Check Correct
	match := CheckPasswordHash(password, hash)
	assert.True(t, match)

	// Test Check Incorrect
	match = CheckPasswordHash("WrongPass", hash)
	assert.False(t, match)
}

func TestTokenGeneration(t *testing.T) {
	userID := "user123"
	service := NewService("a-test-secret-that-is-longer-than-32-bytes", "issuer", "audience", 15*time.Minute)

	// Generate Token
	token, err := service.GenerateToken(userID)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Validate Token
	claims, err := service.ValidateToken(token)
	assert.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)

	// Check Expiration (roughly)
	assert.WithinDuration(t, time.Now().Add(24*time.Hour), claims.ExpiresAt.Time, time.Minute)
}
