// Package mesh loads 3D model files into a flat triangle list, Z-up, in millimetres.
package mesh

import (
	"errors"
	"fmt"
	"math"
)

// MaxTriangles caps memory per model (~36 bytes per triangle).
const MaxTriangles = 5_000_000

var (
	ErrTooLarge    = fmt.Errorf("model has more than %d triangles", MaxTriangles)
	ErrEmpty       = errors.New("model contains no triangles")
	ErrUnsupported = errors.New("unsupported format")
)

// Mesh is a triangle soup: 9 float32 per triangle (three xyz vertices).
type Mesh struct {
	Tris []float32
}

func (m *Mesh) Triangles() int { return len(m.Tris) / 9 }

// addTri appends a triangle, enforcing MaxTriangles.
func (m *Mesh) addTri(a, b, c [3]float32) error {
	if len(m.Tris) >= MaxTriangles*9 {
		return ErrTooLarge
	}
	m.Tris = append(m.Tris, a[0], a[1], a[2], b[0], b[1], b[2], c[0], c[1], c[2])
	return nil
}

// yUpToZUp rotates a Y-up model (OBJ, glTF convention) so Z points up.
func (m *Mesh) yUpToZUp() {
	for i := 0; i < len(m.Tris); i += 3 {
		y, z := m.Tris[i+1], m.Tris[i+2]
		m.Tris[i+1], m.Tris[i+2] = -z, y
	}
}

type Stats struct {
	Triangles int
	Min, Max  [3]float64
	// Volume is only meaningful for closed meshes.
	Volume float64
	Area   float64
}

func (s Stats) Size() [3]float64 {
	return [3]float64{s.Max[0] - s.Min[0], s.Max[1] - s.Min[1], s.Max[2] - s.Min[2]}
}

func (m *Mesh) Stats() Stats {
	s := Stats{Triangles: m.Triangles()}
	if s.Triangles == 0 {
		return s
	}
	for k := range 3 {
		s.Min[k], s.Max[k] = math.Inf(1), math.Inf(-1)
	}
	var vol float64
	t := m.Tris
	for i := 0; i+8 < len(t); i += 9 {
		a := [3]float64{float64(t[i]), float64(t[i+1]), float64(t[i+2])}
		b := [3]float64{float64(t[i+3]), float64(t[i+4]), float64(t[i+5])}
		c := [3]float64{float64(t[i+6]), float64(t[i+7]), float64(t[i+8])}
		for _, p := range [3][3]float64{a, b, c} {
			for k := range 3 {
				s.Min[k] = math.Min(s.Min[k], p[k])
				s.Max[k] = math.Max(s.Max[k], p[k])
			}
		}
		ab := sub(b, a)
		ac := sub(c, a)
		n := cross(ab, ac)
		s.Area += math.Sqrt(dot(n, n)) / 2
		vol += dot(a, cross(b, c)) / 6
	}
	s.Volume = math.Abs(vol)
	return s
}

func sub(a, b [3]float64) [3]float64 { return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func dot(a, b [3]float64) float64    { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

// Load reads just the geometry of a model file (see LoadModel for parts and colours).
func Load(path, format string) (*Mesh, error) {
	m, err := LoadModel(path, format)
	if err != nil {
		return nil, err
	}
	return &m.Mesh, nil
}

// maxWatertightCheck bounds the memory of the edge map (~100 bytes per triangle).
const maxWatertightCheck = 3_000_000

// Watertight reports whether the mesh is closed: after welding vertices with identical
// coordinates, every edge is shared by exactly two triangles. checked is false when the
// mesh is too large to check.
func (m *Mesh) Watertight() (closed, checked bool) {
	n := m.Triangles()
	if n == 0 || n > maxWatertightCheck {
		return false, false
	}
	t := m.Tris
	ids := make(map[[3]float32]uint32, n/2+1)
	vertex := func(i int) uint32 {
		k := [3]float32{t[i], t[i+1], t[i+2]}
		if id, ok := ids[k]; ok {
			return id
		}
		id := uint32(len(ids))
		ids[k] = id
		return id
	}
	edges := make(map[uint64]uint8, n*3/2+1)
	addEdge := func(a, b uint32) {
		if a > b {
			a, b = b, a
		}
		k := uint64(a)<<32 | uint64(b)
		if edges[k] < 255 {
			edges[k]++
		}
	}
	for i := 0; i+8 < len(t); i += 9 {
		a, b, c := vertex(i), vertex(i+3), vertex(i+6)
		if a == b || b == c || a == c {
			continue // degenerate (e.g. at a sphere's poles); not part of the surface
		}
		addEdge(a, b)
		addEdge(b, c)
		addEdge(c, a)
	}
	for _, count := range edges {
		if count != 2 {
			return false, true
		}
	}
	return true, true
}
