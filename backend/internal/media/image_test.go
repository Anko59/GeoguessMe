package media

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"testing"
)

type uploadFile struct{ *bytes.Reader }

func (f uploadFile) Close() error                               { return nil }
func (f uploadFile) Read(p []byte) (int, error)                 { return f.Reader.Read(p) }
func (f uploadFile) ReadAt(p []byte, offset int64) (int, error) { return f.Reader.ReadAt(p, offset) }
func (f uploadFile) Seek(offset int64, whence int) (int64, error) {
	return f.Reader.Seek(offset, whence)
}

var _ multipart.File = uploadFile{}

func TestNormalizeUploadStripsMetadataAndReencodes(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	image, err := NormalizeUpload(uploadFile{bytes.NewReader(data)}, int64(len(data)), 5*1024*1024, 25_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if image.MIMEType != "image/png" || len(image.Data) == 0 || image.PixelWidth != 1 || image.PixelHeight != 1 {
		t.Fatalf("unexpected normalized image: %+v", image)
	}
}

func TestNormalizeUploadRejectsLimitsAndMalformedData(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeUpload(uploadFile{bytes.NewReader(data)}, 0, 5*1024*1024, 25_000_000); err == nil {
		t.Fatal("zero declared size accepted")
	}
	if _, err := NormalizeUpload(uploadFile{bytes.NewReader(data)}, int64(len(data)), 5*1024*1024, 0); err == nil {
		t.Fatal("zero pixel budget accepted")
	}
	if _, err := NormalizeUpload(uploadFile{bytes.NewReader([]byte("not image"))}, 9, 5*1024*1024, 25_000_000); err == nil {
		t.Fatal("expected malformed image rejection")
	}
	if _, err := NormalizeUpload(uploadFile{bytes.NewReader([]byte{1, 2, 3})}, 3, 2, 25_000_000); err == nil {
		t.Fatal("expected byte limit rejection")
	}
}

func TestNormalizeUploadReencodesJPEG(t *testing.T) {
	var source bytes.Buffer
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 1))
	canvas.Set(0, 0, color.RGBA{R: 255, A: 255})
	canvas.Set(1, 0, color.RGBA{B: 255, A: 255})
	if err := jpeg.Encode(&source, canvas, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}

	result, err := NormalizeUpload(uploadFile{bytes.NewReader(source.Bytes())}, int64(source.Len()), 5*1024*1024, 25_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if result.MIMEType != "image/jpeg" || result.PixelWidth != 2 || result.PixelHeight != 1 || len(result.Data) == 0 {
		t.Fatalf("unexpected normalized JPEG: %+v", result)
	}
}

func TestNormalizeChallengeUploadAcceptsRecordedVideoContainers(t *testing.T) {
	webm := []byte{0x1a, 0x45, 0xdf, 0xa3, 0x9f, 0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'}
	result, err := NormalizeChallengeUpload(uploadFile{bytes.NewReader(webm)}, int64(len(webm)), 1024, 25_000_000)
	if err != nil {
		t.Fatalf("normalize WebM: %v", err)
	}
	if result.MIMEType != "video/webm" || !bytes.Equal(result.Data, webm) {
		t.Fatalf("WebM result = %#v", result)
	}

	mp4 := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, make([]byte, 12)...)
	result, err = NormalizeChallengeUpload(uploadFile{bytes.NewReader(mp4)}, int64(len(mp4)), 1024, 25_000_000)
	if err != nil {
		t.Fatalf("normalize MP4: %v", err)
	}
	if result.MIMEType != "video/mp4" || !bytes.Equal(result.Data, mp4) {
		t.Fatalf("MP4 result = %#v", result)
	}
}

func TestNormalizeAvatarResizesLargePNG(t *testing.T) {
	// Generate a 400×300 PNG in-memory.
	canvas := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for x := 0; x < 400; x++ {
		for y := 0; y < 300; y++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var source bytes.Buffer
	if err := png.Encode(&source, canvas); err != nil {
		t.Fatal(err)
	}
	result, err := NormalizeAvatar(uploadFile{bytes.NewReader(source.Bytes())}, int64(source.Len()), 5*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if result.MIMEType != "image/jpeg" || result.PixelWidth > AvatarMaxDimension || result.PixelHeight > AvatarMaxDimension {
		t.Fatalf("unexpected avatar: %+v", result)
	}
	if result.PixelWidth != 256 || result.PixelHeight != 192 {
		t.Fatalf("avatar dimensions = %d×%d, want 256×192", result.PixelWidth, result.PixelHeight)
	}
}

func TestNormalizeAvatarAcceptsJPEG(t *testing.T) {
	var source bytes.Buffer
	canvas := image.NewRGBA(image.Rect(0, 0, 100, 50))
	canvas.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := jpeg.Encode(&source, canvas, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	result, err := NormalizeAvatar(uploadFile{bytes.NewReader(source.Bytes())}, int64(source.Len()), 5*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if result.MIMEType != "image/jpeg" || result.PixelWidth != 100 || result.PixelHeight != 50 || len(result.Data) == 0 {
		t.Fatalf("unexpected avatar: %+v", result)
	}
}

func TestNormalizeAvatarKeepsTinyImage(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	canvas.Set(0, 0, color.RGBA{R: 255, A: 255})
	var source bytes.Buffer
	if err := png.Encode(&source, canvas); err != nil {
		t.Fatal(err)
	}
	result, err := NormalizeAvatar(uploadFile{bytes.NewReader(source.Bytes())}, int64(source.Len()), 5*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if result.MIMEType != "image/jpeg" || result.PixelWidth != 1 || result.PixelHeight != 1 {
		t.Fatalf("tiny avatar: %+v", result)
	}
}

func TestNormalizeAvatarRejectsNonImage(t *testing.T) {
	data := []byte("not an image or video")
	if _, err := NormalizeAvatar(uploadFile{bytes.NewReader(data)}, int64(len(data)), 5*1024*1024); err == nil {
		t.Fatal("expected non-image to be rejected")
	}
}

func TestNormalizeAvatarRejectsOversizedDeclaredSize(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeAvatar(uploadFile{bytes.NewReader(data)}, 101, 100); err == nil {
		t.Fatal("oversized declared size accepted")
	}
	if _, err := NormalizeAvatar(uploadFile{bytes.NewReader(data)}, 0, 100); err == nil {
		t.Fatal("zero declared size accepted")
	}
}

func TestNormalizeChallengeUploadRejectsUnsupportedMedia(t *testing.T) {
	data := []byte("not a supported media file")
	if _, err := NormalizeChallengeUpload(uploadFile{bytes.NewReader(data)}, int64(len(data)), 1024, 25_000_000); err == nil {
		t.Fatal("expected unsupported media to be rejected")
	}
}
