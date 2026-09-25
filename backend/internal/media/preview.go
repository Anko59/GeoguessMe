package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"golang.org/x/image/draw"
)

// FeedPreview irreversibly removes detail before the image reaches the feed.
// It consumes an already validated, metadata-free image from NormalizeUpload.
// Upscaling and the client blur make the tiny preview pleasant to display;
// removing CSS can never recover the original pixels.
func FeedPreview(normalized []byte) ([]byte, error) {
	source, _, err := image.Decode(bytes.NewReader(normalized))
	if err != nil {
		return nil, fmt.Errorf("decode preview: %w", err)
	}
	w, h := source.Bounds().Dx(), source.Bounds().Dy()
	if w >= h {
		h = max(1, h*32/w)
		w = 32
	} else {
		w = max(1, w*32/h)
		h = 32
	}
	preview := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(preview, preview.Bounds(), source, source.Bounds(), draw.Src, nil)
	var output bytes.Buffer
	if err := jpeg.Encode(&output, preview, &jpeg.Options{Quality: 45}); err != nil {
		return nil, fmt.Errorf("encode preview: %w", err)
	}
	return output.Bytes(), nil
}
