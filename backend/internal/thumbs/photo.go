package thumbs

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
)

// Photo reads an uploaded picture (JPEG, PNG, GIF, WebP or BMP): it returns the picture's
// size, and a copy scaled to fit size×size for tiles. The copy is a PNG when the picture
// has transparency and a JPEG otherwise.
func Photo(data []byte, size int) (width, height int, thumb []byte, contentType string, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, nil, "", fmt.Errorf("not a picture this app can read: %w", err)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width*cfg.Height > maxImagePixels {
		return 0, 0, nil, "", fmt.Errorf("picture is too large (%dx%d)", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, 0, nil, "", fmt.Errorf("cannot read the picture: %w", err)
	}
	small := fit(img, size)
	var buf bytes.Buffer
	if o, ok := small.(interface{ Opaque() bool }); ok && !o.Opaque() {
		contentType = "image/png"
		err = png.Encode(&buf, small)
	} else {
		contentType = "image/jpeg"
		err = jpeg.Encode(&buf, small, &jpeg.Options{Quality: 82})
	}
	if err != nil {
		return 0, 0, nil, "", err
	}
	return cfg.Width, cfg.Height, buf.Bytes(), contentType, nil
}
