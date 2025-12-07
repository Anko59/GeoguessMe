package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{"Valid", "valid_user_123", false},
		{"Too Short", "ab", true},
		{"Too Long", "this_username_is_way_too_long_for_our_system_limit", true},
		{"Invalid Char", "user@name", true},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"Valid", "StrongPass123", false},
		{"Too Short", "Short1", true},
		{"No Number", "NoNumberPass", true},
		{"No Upper", "noupper123", true},
		{"No Lower", "NOLOWER123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCoordinates(t *testing.T) {
	tests := []struct {
		name    string
		lat     float64
		lng     float64
		wantErr bool
	}{
		{"Valid", 45.0, 90.0, false},
		{"Invalid Lat High", 91.0, 0.0, true},
		{"Invalid Lat Low", -91.0, 0.0, true},
		{"Invalid Lng High", 0.0, 181.0, true},
		{"Invalid Lng Low", 0.0, -181.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCoordinates(tt.lat, tt.lng)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
