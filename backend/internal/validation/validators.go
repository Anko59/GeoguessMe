package validation

import (
	"fmt"
	"math"
	"net/mail"
	"regexp"
	"strings"
	"unicode"
)

func ValidateEmail(email string) error {
	if len(email) > 254 || strings.TrimSpace(email) == "" {
		return &ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || !strings.Contains(email, "@") {
		return &ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// UsernameValidator validates username format
func ValidateUsername(username string) error {
	if len(username) < 3 {
		return &ValidationError{Field: "username", Message: "must be at least 3 characters"}
	}
	if len(username) > 30 {
		return &ValidationError{Field: "username", Message: "must be at most 30 characters"}
	}

	// Allow alphanumeric, underscore, and hyphen
	validUsername := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validUsername.MatchString(username) {
		return &ValidationError{Field: "username", Message: "can only contain letters, numbers, underscores, and hyphens"}
	}

	return nil
}

// ValidatePassword validates password strength
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return &ValidationError{Field: "password", Message: "must be at least 8 characters"}
	}

	if len(password) > 128 {
		return &ValidationError{Field: "password", Message: "must be at most 128 characters"}
	}

	var (
		hasUpper  bool
		hasLower  bool
		hasNumber bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		}
	}

	if !hasUpper || !hasLower || !hasNumber {
		return &ValidationError{Field: "password", Message: "must contain uppercase, lowercase, and numbers"}
	}

	return nil
}

// ValidateCoordinates validates latitude and longitude
func ValidateCoordinates(lat, long float64) error {
	if math.IsNaN(lat) || math.IsInf(lat, 0) || lat < -90 || lat > 90 {
		return &ValidationError{Field: "latitude", Message: "must be between -90 and 90"}
	}

	if math.IsNaN(long) || math.IsInf(long, 0) || long < -180 || long > 180 {
		return &ValidationError{Field: "longitude", Message: "must be between -180 and 180"}
	}

	return nil
}

// ValidateGroupCode validates group code format
func ValidateGroupCode(code string) error {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return &ValidationError{Field: "code", Message: "must be exactly 6 characters"}
	}

	validCode := regexp.MustCompile(`^[A-Z0-9]+$`)
	if !validCode.MatchString(code) {
		return &ValidationError{Field: "code", Message: "must contain only uppercase letters and numbers"}
	}

	return nil
}

// ValidateGroupName validates group name
func ValidateGroupName(name string) error {
	name = strings.TrimSpace(name)
	if len(name) < 1 {
		return &ValidationError{Field: "name", Message: "is required"}
	}

	if len(name) > 100 {
		return &ValidationError{Field: "name", Message: "must be at most 100 characters"}
	}

	return nil
}
