package media

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestFeedPreviewRemovesImageDetail(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			if (x+y)%2 == 0 {
				source.Set(x, y, color.White)
			} else {
				source.Set(x, y, color.Black)
			}
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, source); err != nil {
		t.Fatal(err)
	}
	data, err := FeedPreview(raw.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	preview, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Bounds().Dx() != 32 || preview.Bounds().Dy() != 32 {
		t.Fatalf("unexpected preview dimensions: %v", preview.Bounds())
	}
	r, _, _, _ := preview.At(16, 16).RGBA()
	if r < 20000 || r > 45000 {
		t.Fatalf("high frequency detail survived: %d", r)
	}
	if _, err := FeedPreview([]byte("invalid")); err == nil {
		t.Fatal("accepted invalid image")
	}
}
