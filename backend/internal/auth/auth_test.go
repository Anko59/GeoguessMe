package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPasswordHashing(t *testing.T) {
	password := "SecretPass123"

	// Test Hash
	hash, err := HashPassword(password)
	assert.NoError(t, err)
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

	// Generate Token
	token, err := GenerateToken(userID)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Validate Token
	claims, err := ValidateToken(token)
	assert.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)

	// Check Expiration (roughly)
	assert.WithinDuration(t, time.Now().Add(24*time.Hour), claims.ExpiresAt.Time, time.Minute)
}
