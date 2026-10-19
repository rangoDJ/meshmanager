// Package render draws thumbnail images of meshes with a small software rasterizer:
// orthographic three-quarter view, z-buffer, two-sided Lambert lighting, supersampled.
// It works on the flat triangle list directly, so memory stays proportional to the mesh.
package render

import (
	"image"
	"image/color"
	"math"

	"github.com/rangoDJ/meshmanager/internal/mesh"
)

// Version changes whenever output changes, so stale thumbnails can be re-rendered.
const Version = 1

type Options struct {
	Size        int // output width and height in pixels
	Supersample int // samples per pixel along each axis
	Color       color.NRGBA
}

var DefaultColor = color.NRGBA{R: 0xa3, G: 0xb1, B: 0xd6, A: 0xff}

type vec [3]float64

func (a vec) dot(b vec) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func (a vec) sub(b vec) vec     { return vec{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func (a vec) scale(s float64) vec {
	return vec{a[0] * s, a[1] * s, a[2] * s}
}
func (a vec) add(b vec) vec { return vec{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a vec) cross(b vec) vec {
	return vec{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func (a vec) norm() vec {
	l := math.Sqrt(a.dot(a))
	if l == 0 {
		return a
	}
	return a.scale(1 / l)
}

// Mesh renders m (Z-up) onto a transparent square image, framed to fit.
func Mesh(m *mesh.Mesh, opt Options) *image.NRGBA {
	if opt.Supersample < 1 {
		opt.Supersample = 2
	}
	if opt.Color.A == 0 {
		opt.Color = DefaultColor
	}
	ss := opt.Supersample
	S := opt.Size * ss

	// Camera from the front-right, above; "front" is -Y as in slicers.
	view := vec{1, -1.6, 1.15}.norm()
	up := vec{0, 0, 1}
	forward := view.scale(-1)
	right := forward.cross(up).norm()
	camUp := right.cross(forward)

	key := right.scale(-0.35).add(camUp.scale(0.55)).add(view.scale(0.75)).norm()
	fill := right.scale(0.6).add(camUp.scale(-0.2)).add(view.scale(0.4)).norm()

	// Project every vertex once to frame the model tightly.
	t := m.Tris
	n := len(t) / 3
	proj := make([]float32, n*3) // screen x, screen y, depth
	minX, minY, minD := math.Inf(1), math.Inf(1), math.Inf(1)
	maxX, maxY, maxD := math.Inf(-1), math.Inf(-1), math.Inf(-1)
	for i := range n {
		p := vec{float64(t[i*3]), float64(t[i*3+1]), float64(t[i*3+2])}
		x, y, d := p.dot(right), p.dot(camUp), p.dot(view)
		proj[i*3], proj[i*3+1], proj[i*3+2] = float32(x), float32(y), float32(d)
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
		minD, maxD = math.Min(minD, d), math.Max(maxD, d)
	}
	w, h := maxX-minX, maxY-minY
	extent := math.Max(w, h)
	if extent <= 0 || math.IsInf(extent, 0) || math.IsNaN(extent) {
		return image.NewNRGBA(image.Rect(0, 0, opt.Size, opt.Size))
	}
	const margin = 0.06
	scale := float64(S) * (1 - 2*margin) / extent
	offX := (float64(S)-w*scale)/2 - minX*scale
	offY := (float64(S)-h*scale)/2 - minY*scale
	depthRange := math.Max(maxD-minD, 1e-9)

	zbuf := make([]float32, S*S)
	for i := range zbuf {
		zbuf[i] = float32(math.Inf(-1))
	}
	shade := make([]float32, S*S)

	for tri := 0; tri+2 < n; tri += 3 {
		a := vec{float64(t[tri*3]), float64(t[tri*3+1]), float64(t[tri*3+2])}
		b := vec{float64(t[tri*3+3]), float64(t[tri*3+4]), float64(t[tri*3+5])}
		c := vec{float64(t[tri*3+6]), float64(t[tri*3+7]), float64(t[tri*3+8])}
		normal := b.sub(a).cross(c.sub(a)).norm()
		if normal.dot(view) < 0 {
			normal = normal.scale(-1) // two-sided: inverted or open meshes still shade
		}
		intensity := 0.3 + 0.62*math.Max(0, normal.dot(key)) + 0.2*math.Max(0, normal.dot(fill))

		var sx, sy, sz [3]float64
		for k := range 3 {
			o := (tri + k) * 3
			sx[k] = float64(proj[o])*scale + offX
			sy[k] = float64(S) - (float64(proj[o+1])*scale + offY)
			sz[k] = float64(proj[o+2])
		}
		area := edge(sx[0], sy[0], sx[1], sy[1], sx[2], sy[2])
		if math.Abs(area) < 1e-12 {
			continue
		}
		x0 := max(0, int(math.Floor(min(sx[0], sx[1], sx[2]))))
		x1 := min(S-1, int(math.Ceil(max(sx[0], sx[1], sx[2]))))
		y0 := max(0, int(math.Floor(min(sy[0], sy[1], sy[2]))))
		y1 := min(S-1, int(math.Ceil(max(sy[0], sy[1], sy[2]))))
		for py := y0; py <= y1; py++ {
			fy := float64(py) + 0.5
			for px := x0; px <= x1; px++ {
				fx := float64(px) + 0.5
				w0 := edge(sx[1], sy[1], sx[2], sy[2], fx, fy)
				w1 := edge(sx[2], sy[2], sx[0], sy[0], fx, fy)
				w2 := edge(sx[0], sy[0], sx[1], sy[1], fx, fy)
				if area > 0 {
					if w0 < 0 || w1 < 0 || w2 < 0 {
						continue
					}
				} else if w0 > 0 || w1 > 0 || w2 > 0 {
					continue
				}
				z := float32((w0*sz[0] + w1*sz[1] + w2*sz[2]) / area)
				i := py*S + px
				if z <= zbuf[i] {
					continue
				}
				zbuf[i] = z
				// Subtle depth cue: farther surfaces slightly darker.
				depth := (float64(z) - minD) / depthRange
				shade[i] = float32(intensity * (0.8 + 0.2*depth))
			}
		}
	}

	return resolve(zbuf, shade, opt.Size, ss, opt.Color)
}

func edge(ax, ay, bx, by, px, py float64) float64 {
	return (bx-ax)*(py-ay) - (by-ay)*(px-ax)
}

// resolve box-filters the supersampled buffers into the output image.
func resolve(zbuf, shade []float32, size, ss int, base color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	S := size * ss
	samples := float64(ss * ss)
	negInf := float32(math.Inf(-1))
	for y := range size {
		for x := range size {
			covered := 0
			var sum float64
			for dy := range ss {
				row := (y*ss + dy) * S
				for dx := range ss {
					i := row + x*ss + dx
					if zbuf[i] != negInf {
						covered++
						sum += float64(shade[i])
					}
				}
			}
			if covered == 0 {
				continue
			}
			k := sum / float64(covered)
			o := img.PixOffset(x, y)
			img.Pix[o] = clamp(float64(base.R) * k)
			img.Pix[o+1] = clamp(float64(base.G) * k)
			img.Pix[o+2] = clamp(float64(base.B) * k)
			img.Pix[o+3] = clamp(255 * float64(covered) / samples)
		}
	}
	return img
}

func clamp(v float64) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	}
	return uint8(v + 0.5)
}
