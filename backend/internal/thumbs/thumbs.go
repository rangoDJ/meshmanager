// Package thumbs produces a thumbnail image for any file whose format supports one.
package thumbs

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // register decoders
	"image/jpeg"
	"image/png"
	"io"
	"os"

	"golang.org/x/image/draw"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/rangoDJ/meshmanager/internal/mesh"
	"github.com/rangoDJ/meshmanager/internal/render"
)

// Version is stored with each thumbnail; bump it (or render.Version) to re-render.
const Version = render.Version

// maxImagePixels guards against decompression bombs and huge photos.
const maxImagePixels = 100_000_000

const (
	OriginRendered = "rendered"
	OriginEmbedded = "embedded"
	OriginImage    = "image"
)

type Result struct {
	Image       image.Image
	Origin      string
	ContentType string
	// Stats and Watertight are set when the mesh was loaded (not for embedded previews
	// or images). Watertight is nil when the mesh was too large to check.
	Stats      *mesh.Stats
	Watertight *bool
}

var ErrUnsupported = errors.New("no thumbnail support for this format")

// Generate makes a thumbnail that fits in size×size for the file at path.
func Generate(path, format string, size int) (Result, error) {
	switch format {
	case "3mf":
		// Slicers embed their own (coloured, per-plate) preview; prefer it.
		if b, err := mesh.ThreeMFThumbnail(path); err == nil && b != nil {
			if img, _, err := image.Decode(bytes.NewReader(b)); err == nil {
				return Result{Image: fit(img, size), Origin: OriginEmbedded, ContentType: "image/png"}, nil
			}
		}
		return renderModel(path, format, size)
	case "stl", "obj", "ply", "glb", "gltf":
		return renderModel(path, format, size)
	case "svg":
		img, err := rasterizeSVG(path, size)
		if err != nil {
			return Result{}, err
		}
		return Result{Image: img, Origin: OriginImage, ContentType: "image/png"}, nil
	case "gcode", "fcstd", "lbrn":
		var img image.Image
		var err error
		switch format {
		case "gcode":
			img, err = gcodePreview(path)
		case "fcstd":
			img, err = freecadPreview(path)
		default:
			img, err = lightburnPreview(path)
		}
		if err != nil {
			return Result{}, err
		}
		return Result{Image: fit(img, size), Origin: OriginEmbedded, ContentType: "image/png"}, nil
	case "jpg", "png", "gif", "webp", "bmp":
		img, err := decodeImage(path)
		if err != nil {
			return Result{}, err
		}
		ct := "image/png"
		if format == "jpg" {
			ct = "image/jpeg"
		}
		return Result{Image: fit(img, size), Origin: OriginImage, ContentType: ct}, nil
	}
	return Result{}, ErrUnsupported
}

func renderModel(path, format string, size int) (Result, error) {
	m, err := mesh.LoadModel(path, format)
	if err != nil {
		return Result{}, err
	}
	st := m.Stats()
	res := Result{Origin: OriginRendered, ContentType: "image/png", Stats: &st}
	if closed, checked := m.Watertight(); checked {
		res.Watertight = &closed
	}
	// A multi-plate project shows its first plate, not every plate spread out.
	shown := m.OnPlate(m.FirstPlate())
	res.Image = render.Mesh(&shown.Mesh, render.Options{Size: size, Supersample: 2, Parts: shown.Parts})
	return res, nil
}

func decodeImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width*cfg.Height > maxImagePixels {
		return nil, fmt.Errorf("image is too large (%dx%d)", cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	return img, err
}

// fit scales img down to fit in size×size, keeping its aspect ratio. It never upscales.
func fit(img image.Image, size int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= size && h <= size {
		return img
	}
	if w >= h {
		h = max(1, h*size/w)
		w = size
	} else {
		w = max(1, w*size/h)
		h = size
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// Encode writes the thumbnail in its content type.
func Encode(w io.Writer, r Result) error {
	if r.ContentType == "image/jpeg" {
		return jpeg.Encode(w, r.Image, &jpeg.Options{Quality: 82})
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	return enc.Encode(w, r.Image)
}

// Ext returns the file extension for a content type.
func Ext(contentType string) string {
	if contentType == "image/jpeg" {
		return ".jpg"
	}
	return ".png"
}
