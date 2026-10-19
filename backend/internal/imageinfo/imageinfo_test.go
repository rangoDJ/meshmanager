package imageinfo

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.5 }

// withPHYs inserts a pHYs chunk right after IHDR.
func withPHYs(pngData []byte, ppm uint32) []byte {
	ihdrEnd := 8 + 12 + int(binary.BigEndian.Uint32(pngData[8:]))
	data := binary.BigEndian.AppendUint32(nil, ppm)
	data = binary.BigEndian.AppendUint32(data, ppm)
	data = append(data, 1)
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	body := append([]byte("pHYs"), data...)
	chunk = append(chunk, body...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(body))
	out := append([]byte{}, pngData[:ihdrEnd]...)
	out = append(out, chunk...)
	return append(out, pngData[ihdrEnd:]...)
}

func TestPNG(t *testing.T) {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewGray(image.Rect(0, 0, 600, 300)))
	info, err := Read(withPHYs(b.Bytes(), 11811)) // 11811 px/m ≈ 300 dpi
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 600 || info.Height != 300 || !near(info.DPIX, 300) || !near(info.DPIY, 300) {
		t.Fatalf("%+v", info)
	}

	plain, _ := Read(b.Bytes())
	if plain.DPIX != 0 {
		t.Errorf("PNG without pHYs reported %v dpi", plain.DPIX)
	}
}

// withJFIF rewrites the JFIF APP0 density fields Go's encoder doesn't set.
func withJFIF(jpg []byte, unit byte, x, y uint16) []byte {
	app0 := []byte{0xff, 0xe0, 0, 16, 'J', 'F', 'I', 'F', 0, 1, 1, unit, byte(x >> 8), byte(x), byte(y >> 8), byte(y), 0, 0}
	return append(append([]byte{0xff, 0xd8}, app0...), jpg[2:]...)
}

func TestJPEG(t *testing.T) {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewGray(image.Rect(0, 0, 80, 40)), nil)

	dpi, err := Read(withJFIF(b.Bytes(), 1, 150, 150))
	if err != nil || dpi.Width != 80 || !near(dpi.DPIX, 150) {
		t.Fatalf("JFIF dpi: %+v %v", dpi, err)
	}
	cm, _ := Read(withJFIF(b.Bytes(), 2, 100, 100)) // dots per cm
	if !near(cm.DPIX, 254) {
		t.Errorf("JFIF dpcm: %+v", cm)
	}
}

func TestEXIF(t *testing.T) {
	// Little-endian TIFF with IFD0: XResolution, YResolution = 72/1, unit = inch.
	tiff := []byte("II\x2a\x00\x08\x00\x00\x00")
	tiff = binary.LittleEndian.AppendUint16(tiff, 3)
	entry := func(tag, typ uint16, count, value uint32) {
		tiff = binary.LittleEndian.AppendUint16(tiff, tag)
		tiff = binary.LittleEndian.AppendUint16(tiff, typ)
		tiff = binary.LittleEndian.AppendUint32(tiff, count)
		tiff = binary.LittleEndian.AppendUint32(tiff, value)
	}
	rationalAt := uint32(8 + 2 + 3*12 + 4)
	entry(0x011a, 5, 1, rationalAt)
	entry(0x011b, 5, 1, rationalAt)
	entry(0x0128, 3, 1, 2)
	tiff = binary.LittleEndian.AppendUint32(tiff, 0) // next IFD
	tiff = binary.LittleEndian.AppendUint32(tiff, 72)
	tiff = binary.LittleEndian.AppendUint32(tiff, 1)

	if x, y := exifDPI(tiff); !near(x, 72) || !near(y, 72) {
		t.Fatalf("exif dpi %v %v", x, y)
	}
}

func TestGarbage(t *testing.T) {
	if _, err := Read([]byte("not an image")); err == nil {
		t.Error("expected an error")
	}
}
