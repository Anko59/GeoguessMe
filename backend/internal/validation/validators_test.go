package validation

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
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

func TestEmailGroupAndNameValidation(t *testing.T) {
	for _, value := range []string{"alice@example.test", "a+b@example.test"} {
		if err := ValidateEmail(value); err != nil {
			t.Errorf("valid email %q: %v", value, err)
		}
	}
	for _, value := range []string{"", "missing-at", "alice@example.test ", string(bytes.Repeat([]byte{'a'}, 255))} {
		if err := ValidateEmail(value); err == nil {
			t.Errorf("invalid email %q accepted", value)
		}
	}
	for _, value := range []string{"ABC123", "000000"} {
		if err := ValidateGroupCode(value); err != nil {
			t.Errorf("valid group code %q: %v", value, err)
		}
	}
	for _, value := range []string{"abc123", "ABC12", "ABC-12"} {
		if err := ValidateGroupCode(value); err == nil {
			t.Errorf("invalid group code %q accepted", value)
		}
	}
	if ValidateGroupName("  Paris  ") != nil || ValidateGroupName(string(bytes.Repeat([]byte{'x'}, 101))) == nil {
		t.Fatal("group name validation failed")
	}
	var validationError *ValidationError
	if err := ValidateEmail("bad"); err == nil || !errors.As(err, &validationError) || validationError.Field != "email" {
		t.Fatal("validation error did not expose its field")
	}
}

func TestUploadedFileValidationAndExtensions(t *testing.T) {
	file := &testMultipartFile{Reader: bytes.NewReader([]byte{0x89, 0x50, 0x4e, 0x47, 1})}
	if err := ValidateUploadedFile(file, &multipart.FileHeader{Size: 5, Filename: "photo.png"}); err != nil {
		t.Fatal(err)
	}
	for _, header := range []*multipart.FileHeader{{Size: 0}, {Size: MaxFileSize + 1}} {
		if err := ValidateUploadedFile(&testMultipartFile{Reader: bytes.NewReader([]byte("bad"))}, header); err == nil {
			t.Fatal("invalid file size accepted")
		}
	}
	if err := ValidateUploadedFile(&testMultipartFile{Reader: bytes.NewReader([]byte("bad"))}, &multipart.FileHeader{Size: 3}); err == nil {
		t.Fatal("invalid magic bytes accepted")
	}
	if GetSafeExtension("image.JPEG") != ".jpeg" || GetSafeExtension("image.unknown") != ".jpg" {
		t.Fatal("safe extension mapping failed")
	}
}

type testMultipartFile struct{ *bytes.Reader }

func (f *testMultipartFile) Close() error               { return nil }
func (f *testMultipartFile) Read(p []byte) (int, error) { return f.Reader.Read(p) }
func (f *testMultipartFile) Seek(offset int64, whence int) (int64, error) {
	return f.Reader.Seek(offset, whence)
}

var _ io.ReadSeeker = (*testMultipartFile)(nil)
