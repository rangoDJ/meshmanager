package render

import (
	"testing"
	"time"

	"github.com/rangoDJ/meshmanager/internal/mesh"
)

// cube returns a 10 mm cube as a triangle soup.
func cube() *mesh.Mesh {
	v := [8][3]float32{{0, 0, 0}, {10, 0, 0}, {10, 10, 0}, {0, 10, 0}, {0, 0, 10}, {10, 0, 10}, {10, 10, 10}, {0, 10, 10}}
	quads := [6][4]int{{0, 3, 2, 1}, {4, 5, 6, 7}, {0, 1, 5, 4}, {2, 3, 7, 6}, {1, 2, 6, 5}, {0, 4, 7, 3}}
	m := &mesh.Mesh{}
	for _, q := range quads {
		for _, tri := range [2][3]int{{q[0], q[1], q[2]}, {q[0], q[2], q[3]}} {
			for _, i := range tri {
				m.Tris = append(m.Tris, v[i][0], v[i][1], v[i][2])
			}
		}
	}
	return m
}

func TestRenderCube(t *testing.T) {
	img := Mesh(cube(), Options{Size: 128, Supersample: 2})
	if img.Bounds().Dx() != 128 || img.Bounds().Dy() != 128 {
		t.Fatalf("size = %v", img.Bounds())
	}
	if a := img.NRGBAAt(0, 0).A; a != 0 {
		t.Errorf("corner should be transparent, alpha = %d", a)
	}
	center := img.NRGBAAt(64, 64)
	if center.A != 255 {
		t.Errorf("center should be opaque, got %v", center)
	}

	// Three visible faces get different shading.
	shades := map[uint8]bool{}
	for _, p := range [][2]int{{64, 30}, {40, 80}, {90, 80}} {
		c := img.NRGBAAt(p[0], p[1])
		if c.A == 255 {
			shades[c.R] = true
		}
	}
	if len(shades) < 2 {
		t.Errorf("expected distinct face shading, got %v", shades)
	}

	// The model should fill most of the frame (tight framing).
	covered := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 0 {
			covered++
		}
	}
	if frac := float64(covered) / (128 * 128); frac < 0.4 {
		t.Errorf("model covers only %.0f%% of the image", frac*100)
	}
}

func TestRenderDegenerate(t *testing.T) {
	m := &mesh.Mesh{Tris: []float32{1, 1, 1, 1, 1, 1, 1, 1, 1}}
	img := Mesh(m, Options{Size: 32})
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			t.Fatal("degenerate mesh should render transparent")
		}
	}
}

func BenchmarkRenderMillionTriangles(b *testing.B) {
	base := cube()
	m := &mesh.Mesh{}
	// ~1M small triangles spread over a grid.
	for i := range 83_334 {
		dx, dy := float32(i%300)*12, float32(i/300)*12
		for k := 0; k < len(base.Tris); k += 3 {
			m.Tris = append(m.Tris, base.Tris[k]+dx, base.Tris[k+1]+dy, base.Tris[k+2])
		}
	}
	b.ResetTimer()
	start := time.Now()
	for range b.N {
		Mesh(m, Options{Size: 384, Supersample: 2})
	}
	b.ReportMetric(float64(time.Since(start).Milliseconds())/float64(b.N), "ms/render")
}

func TestRenderPartColors(t *testing.T) {
	m := cube()
	parts := []mesh.Part{{Start: 0, Count: 12, Color: "#ff0000"}}
	img := Mesh(m, Options{Size: 64, Supersample: 2, Parts: parts})
	c := img.NRGBAAt(32, 32)
	if c.A != 255 || c.R <= c.G || c.R <= c.B || c.G > 10 {
		t.Fatalf("expected a red-shaded pixel, got %v", c)
	}
}
