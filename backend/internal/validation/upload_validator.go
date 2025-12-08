package validation

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"strings"
)

const (
	MaxFileSize = 5 * 1024 * 1024 // 5MB
)

// FileType represents allowed file types
type FileType struct {
	Extension  string
	MimeType   string
	MagicBytes [][]byte
}

var AllowedImageTypes = []FileType{
	{
		Extension: ".jpg",
		MimeType:  "image/jpeg",
		MagicBytes: [][]byte{
			{0xFF, 0xD8, 0xFF}, // JPEG
		},
	},
	{
		Extension: ".jpeg",
		MimeType:  "image/jpeg",
		MagicBytes: [][]byte{
			{0xFF, 0xD8, 0xFF}, // JPEG
		},
	},
	{
		Extension: ".png",
		MimeType:  "image/png",
		MagicBytes: [][]byte{
			{0x89, 0x50, 0x4E, 0x47}, // PNG
		},
	},
	{
		Extension: ".webp",
		MimeType:  "image/webp",
		MagicBytes: [][]byte{
			{0x52, 0x49, 0x46, 0x46}, // RIFF (at offset 0)
			// WEBP signature is at offset 8, but we'll check RIFF first
		},
	},
}

// ValidateUploadedFile validates file type, size, and content
func ValidateUploadedFile(file multipart.File, header *multipart.FileHeader) error {
	// Validate file size
	if header.Size > MaxFileSize {
		return fmt.Errorf("file size exceeds maximum allowed size of %d bytes", MaxFileSize)
	}

	if header.Size == 0 {
		return fmt.Errorf("file is empty")
	}

	// Read first 512 bytes for magic byte detection
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read file: %v", err)
	}
	buffer = buffer[:n]

	// Reset file pointer
	_, err = file.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to reset file pointer: %v", err)
	}

	// Validate file type by magic bytes
	isValid := false
	for _, fileType := range AllowedImageTypes {
		for _, magic := range fileType.MagicBytes {
			if len(buffer) >= len(magic) && bytes.Equal(buffer[:len(magic)], magic) {
				isValid = true
				break
			}
		}
		if isValid {
			break
		}
	}

	if !isValid {
		return fmt.Errorf("invalid file type: only JPEG, PNG, and WebP images are allowed")
	}

	return nil
}

// GetSafeExtension returns a safe file extension
func GetSafeExtension(filename string) string {
	ext := strings.ToLower(filename)
	for _, fileType := range AllowedImageTypes {
		if strings.HasSuffix(ext, fileType.Extension) {
			return fileType.Extension
		}
	}
	return ".jpg" // Default to jpg if unknown
}
