package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"

	_ "golang.org/x/image/webp"
)

type Image struct {
	Data        []byte
	MIMEType    string
	PixelWidth  int
	PixelHeight int
}

func NormalizeUpload(file multipart.File, declaredSize, maxBytes int64, maxPixels uint64) (*Image, error) {
	if declaredSize <= 0 || declaredSize > maxBytes {
		return nil, fmt.Errorf("image must be between 1 byte and %d bytes", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("image exceeds %d bytes", maxBytes)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid image: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || uint64(config.Width)*uint64(config.Height) > maxPixels {
		return nil, fmt.Errorf("image exceeds pixel limit")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	var output bytes.Buffer
	mimeType := "image/jpeg"
	switch format {
	case "png":
		mimeType = "image/png"
		if err := png.Encode(&output, decoded); err != nil {
			return nil, fmt.Errorf("encode PNG: %w", err)
		}
	case "jpeg":
		if err := jpeg.Encode(&output, decoded, &jpeg.Options{Quality: 90}); err != nil {
			return nil, fmt.Errorf("encode JPEG: %w", err)
		}
	case "webp":
		if err := jpeg.Encode(&output, decoded, &jpeg.Options{Quality: 90}); err != nil {
			return nil, fmt.Errorf("encode JPEG: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported image format")
	}
	return &Image{Data: output.Bytes(), MIMEType: mimeType, PixelWidth: config.Width, PixelHeight: config.Height}, nil
}
