package thumbs

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/draw"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// errNoPreview means the file format can carry a preview but this file has none.
var errNoPreview = errors.New("the file has no saved preview")

// gcodePreview returns the largest thumbnail a slicer embedded in G-code comments:
//
//	; thumbnail begin 300x300 12345      (also thumbnail_PNG / thumbnail_JPG)
//	; iVBORw0KGgo...
//	; thumbnail end
//
// Slicers put them at the top of the file, so only the first few MB are read.
func gcodePreview(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(io.LimitReader(f, 8<<20))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	var best image.Image
	bestArea := 0
	var b64 strings.Builder
	in := false
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sc.Text()), ";"))
		switch {
		case strings.HasPrefix(line, "thumbnail") && strings.Contains(line, " begin"):
			if strings.HasPrefix(line, "thumbnail_QOI") {
				continue // not decodable here
			}
			in = true
			b64.Reset()
		case in && strings.HasPrefix(line, "thumbnail") && strings.Contains(line, " end"):
			in = false
			data, err := base64.StdEncoding.DecodeString(b64.String())
			if err != nil {
				continue
			}
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				continue
			}
			if a := img.Bounds().Dx() * img.Bounds().Dy(); a > bestArea {
				best, bestArea = img, a
			}
		case in:
			b64.WriteString(line)
		}
	}
	if best == nil {
		return nil, errNoPreview
	}
	return best, nil
}

var platePNGRe = regexp.MustCompile(`(?i)^metadata/plate_(\d+)\.png$`)

// gcode3mfPreview returns the render Bambu Studio / OrcaSlicer save for the first
// sliced plate of a .gcode.3mf (Metadata/plate_1.png).
func gcode3mfPreview(path string) (image.Image, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var best *zip.File
	bestN := 0
	for _, f := range zr.File {
		m := platePNGRe.FindStringSubmatch(f.Name)
		if m == nil || f.UncompressedSize64 > 20<<20 {
			continue
		}
		if n, _ := strconv.Atoi(m[1]); best == nil || n < bestN {
			best, bestN = f, n
		}
	}
	if best == nil {
		return nil, errNoPreview
	}
	rc, err := best.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	img, _, err := image.Decode(rc)
	return img, err
}

// freecadPreview returns the thumbnail FreeCAD saves inside an .FCStd (a ZIP).
func freecadPreview(path string) (image.Image, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if !strings.EqualFold(f.Name, "thumbnails/Thumbnail.png") || f.UncompressedSize64 > 20<<20 {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		img, _, err := image.Decode(rc)
		return img, err
	}
	return nil, errNoPreview
}

// lightburnPreview returns the preview LightBurn stores in a project file as
// <Thumbnail Source="base64 PNG"/>.
func lightburnPreview(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64<<20))
	if err != nil {
		return nil, err
	}
	const marker = `<Thumbnail Source="`
	i := bytes.Index(data, []byte(marker))
	if i < 0 {
		return nil, errNoPreview
	}
	rest := data[i+len(marker):]
	end := bytes.IndexByte(rest, '"')
	if end < 0 {
		return nil, errNoPreview
	}
	png, err := base64.StdEncoding.DecodeString(string(rest[:end]))
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(png))
	return img, err
}

// maxSVGBytes bounds SVGs we rasterize.
const maxSVGBytes = 16 << 20

// rasterizeSVG draws an SVG to fit in size×size on a white background. Scripts and
// external references are never evaluated.
func rasterizeSVG(path string, size int) (image.Image, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > maxSVGBytes {
		return nil, errors.New("SVG is too large to preview (" + strconv.FormatInt(st.Size(), 10) + " bytes)")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	icon, err := oksvg.ReadIconStream(f, oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, err
	}
	vw, vh := icon.ViewBox.W, icon.ViewBox.H
	if vw <= 0 || vh <= 0 {
		vw, vh = float64(size), float64(size)
	}
	w, h := size, size
	if vw >= vh {
		h = max(1, int(float64(size)*vh/vw))
	} else {
		w = max(1, int(float64(size)*vw/vh))
	}
	icon.SetTarget(0, 0, float64(w), float64(h))
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	// Cut files are usually dark lines on nothing; give them paper so they read on a
	// dark grid as well as a light one.
	draw.Draw(rgba, rgba.Bounds(), image.White, image.Point{}, draw.Src)
	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	icon.Draw(rasterx.NewDasher(w, h, scanner), 1)
	return rgba, nil
}
