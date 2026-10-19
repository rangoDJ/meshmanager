// Package imageinfo reads an image's pixel size and resolution (DPI), so the UI can
// say how large it prints.
package imageinfo

import (
	"bytes"
	"encoding/binary"
	"image"
	_ "image/gif" // register decoders for DecodeConfig
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

type Info struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	DPIX   float64 `json:"dpi_x"` // 0 when the file doesn't say
	DPIY   float64 `json:"dpi_y"`
}

// Read parses the start of an image file (a few MB is plenty).
func Read(head []byte) (Info, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(head))
	if err != nil {
		return Info{}, err
	}
	info := Info{Width: cfg.Width, Height: cfg.Height}
	switch {
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		info.DPIX, info.DPIY = pngDPI(head)
	case bytes.HasPrefix(head, []byte{0xff, 0xd8}):
		info.DPIX, info.DPIY = jpegDPI(head)
	}
	return info, nil
}

// pngDPI reads the pHYs chunk (pixels per metre).
func pngDPI(b []byte) (float64, float64) {
	for i := 8; i+12 <= len(b); {
		n := int(binary.BigEndian.Uint32(b[i:]))
		typ := string(b[i+4 : i+8])
		if typ == "pHYs" && n >= 9 && i+8+9 <= len(b) {
			d := b[i+8:]
			if d[8] == 1 { // unit: metre
				return float64(binary.BigEndian.Uint32(d)) * 0.0254, float64(binary.BigEndian.Uint32(d[4:])) * 0.0254
			}
			return 0, 0
		}
		if typ == "IDAT" || n < 0 {
			return 0, 0
		}
		i += 12 + n
	}
	return 0, 0
}

// jpegDPI reads the JFIF APP0 density, falling back to EXIF XResolution/YResolution.
func jpegDPI(b []byte) (float64, float64) {
	for i := 2; i+4 <= len(b); {
		if b[i] != 0xff {
			return 0, 0
		}
		marker := b[i+1]
		if marker == 0xda || marker == 0xd9 { // start of scan / end of image
			return 0, 0
		}
		n := int(binary.BigEndian.Uint16(b[i+2:]))
		if n < 2 || i+2+n > len(b) {
			return 0, 0
		}
		seg := b[i+4 : i+2+n]
		switch {
		case marker == 0xe0 && bytes.HasPrefix(seg, []byte("JFIF\x00")) && len(seg) >= 12:
			unit := seg[7]
			x, y := float64(binary.BigEndian.Uint16(seg[8:])), float64(binary.BigEndian.Uint16(seg[10:]))
			switch unit {
			case 1:
				return x, y
			case 2:
				return x * 2.54, y * 2.54
			}
		case marker == 0xe1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")):
			if x, y := exifDPI(seg[6:]); x > 0 {
				return x, y
			}
		}
		i += 2 + n
	}
	return 0, 0
}

// exifDPI reads XResolution (0x011A), YResolution (0x011B) and ResolutionUnit (0x0128)
// from IFD0 of a TIFF/EXIF block.
func exifDPI(t []byte) (float64, float64) {
	if len(t) < 8 {
		return 0, 0
	}
	var order binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, 0
	}
	ifd := int(order.Uint32(t[4:]))
	if ifd+2 > len(t) {
		return 0, 0
	}
	rational := func(off int) float64 {
		if off+8 > len(t) {
			return 0
		}
		num, den := order.Uint32(t[off:]), order.Uint32(t[off+4:])
		if den == 0 {
			return 0
		}
		return float64(num) / float64(den)
	}
	var x, y float64
	unit := 2 // inches by default
	count := int(order.Uint16(t[ifd:]))
	for k := range count {
		e := ifd + 2 + k*12
		if e+12 > len(t) {
			break
		}
		switch order.Uint16(t[e:]) {
		case 0x011a:
			x = rational(int(order.Uint32(t[e+8:])))
		case 0x011b:
			y = rational(int(order.Uint32(t[e+8:])))
		case 0x0128:
			unit = int(order.Uint16(t[e+8:]))
		}
	}
	if unit == 3 { // centimetres
		x, y = x*2.54, y*2.54
	}
	return x, y
}
