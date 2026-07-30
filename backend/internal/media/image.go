package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const AvatarMaxDimension = 256

type Image struct {
	Data        []byte
	MIMEType    string
	PixelWidth  int
	PixelHeight int
}

// NormalizeAvatar decodes an uploaded image, shrinks it to fit within
// AvatarMaxDimension preserving aspect ratio, re-encodes as JPEG q85, and
// strips all metadata. Videos and non-images are rejected.
func NormalizeAvatar(file multipart.File, declaredSize, maxBytes int64, maxPixels uint64) (*Image, error) {
	if declaredSize <= 0 || declaredSize > maxBytes {
		return nil, fmt.Errorf("photo must be between 1 byte and %d bytes", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read avatar: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("photo exceeds %d bytes", maxBytes)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid image: choose a JPG, PNG, or WebP photo")
	}
	if config.Width <= 0 || config.Height <= 0 || uint64(config.Width)*uint64(config.Height) > maxPixels {
		return nil, fmt.Errorf("photo has too many pixels; choose a smaller-resolution image")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid image: choose a JPG, PNG, or WebP photo")
	}
	scaledW, scaledH := avatarDimensions(config.Width, config.Height)
	var result image.Image
	if scaledW == config.Width && scaledH == config.Height {
		result = decoded
	} else {
		dst := image.NewRGBA(image.Rect(0, 0, scaledW, scaledH))
		draw.CatmullRom.Scale(dst, dst.Bounds(), decoded, decoded.Bounds(), draw.Over, nil)
		result = dst
	}
	var output bytes.Buffer
	if err := jpeg.Encode(&output, result, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode avatar: %w", err)
	}
	_ = format
	return &Image{Data: output.Bytes(), MIMEType: "image/jpeg", PixelWidth: scaledW, PixelHeight: scaledH}, nil
}

func avatarDimensions(w, h int) (int, int) {
	if w <= AvatarMaxDimension && h <= AvatarMaxDimension {
		return w, h
	}
	if w >= h {
		return AvatarMaxDimension, max(1, h*AvatarMaxDimension/w)
	}
	return max(1, w*AvatarMaxDimension/h), AvatarMaxDimension
}

// Upload is a validated challenge asset. Images are decoded and re-encoded to
// remove metadata; browser-recorded videos are kept as their validated source
// bytes because this service deliberately has no transcoding runtime.
type Upload struct {
	Data     []byte
	MIMEType string
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
	return normalizeImageData(data, maxPixels)
}

// NormalizeChallengeUpload accepts normalized images and camera-recorded MP4
// or WebM clips. The server determines the media type from container bytes,
// rather than relying on the multipart content type supplied by a client.
func NormalizeChallengeUpload(file multipart.File, declaredSize, maxBytes int64, maxPixels uint64) (*Upload, error) {
	if declaredSize <= 0 || declaredSize > maxBytes {
		return nil, fmt.Errorf("media must be between 1 byte and %d bytes", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read media: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("media exceeds %d bytes", maxBytes)
	}
	if mimeType := recordedVideoMIMEType(data); mimeType != "" {
		return &Upload{Data: data, MIMEType: mimeType}, nil
	}
	image, err := normalizeImageData(data, maxPixels)
	if err != nil {
		return nil, err
	}
	return &Upload{Data: image.Data, MIMEType: image.MIMEType}, nil
}

func normalizeImageData(data []byte, maxPixels uint64) (*Image, error) {
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

func recordedVideoMIMEType(data []byte) string {
	if isMP4(data) {
		return "video/mp4"
	}
	if isWebM(data) {
		return "video/webm"
	}
	return ""
}

func isMP4(data []byte) bool {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return false
	}
	// ISO base media file brands used by current browser MediaRecorder
	// implementations. Do not accept a generic ftyp box as video.
	switch string(data[8:12]) {
	case "isom", "iso2", "mp41", "mp42", "avc1", "M4V ":
		return true
	default:
		return false
	}
}

func isWebM(data []byte) bool {
	if len(data) < 8 || !bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		return false
	}
	limit := min(len(data), 4096)
	return bytes.Contains(data[:limit], []byte("webm"))
}
