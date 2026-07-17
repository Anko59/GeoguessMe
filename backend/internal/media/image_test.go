package media

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
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
