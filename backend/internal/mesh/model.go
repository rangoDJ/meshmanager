package mesh

import (
	"fmt"
	"os"
	"strings"
)

// Part is a contiguous run of triangles belonging to one object, part or volume.
type Part struct {
	Name  string `json:"name"`
	Plate int    `json:"plate"`           // 1-based plate number; 0 when the file has no plates
	Color string `json:"color,omitempty"` // "#rrggbb" when the file assigns one
	Start int    `json:"start"`           // index of the first triangle
	Count int    `json:"count"`
}

type Plate struct {
	Index int    `json:"index"`
	Name  string `json:"name,omitempty"`
}

// Model is a mesh plus its structure: named, coloured parts and slicer plates.
type Model struct {
	Mesh
	Parts  []Part
	Plates []Plate
}

// beginPart starts a new part at the current end of the triangle list.
func (m *Model) beginPart(name string, plate int, color string) {
	m.Parts = append(m.Parts, Part{Name: name, Plate: plate, Color: color, Start: m.Triangles()})
}

// finish computes part sizes and drops empty parts.
func (m *Model) finish() {
	total := m.Triangles()
	if len(m.Parts) == 0 {
		m.Parts = []Part{{Count: total}}
		return
	}
	kept := m.Parts[:0]
	for i, p := range m.Parts {
		end := total
		if i+1 < len(m.Parts) {
			end = m.Parts[i+1].Start
		}
		p.Count = end - p.Start
		if p.Count > 0 {
			kept = append(kept, p)
		}
	}
	m.Parts = kept
}

// OnPlate returns a model with only the parts on the given plate, or m itself when
// the model has no such plate.
func (m *Model) OnPlate(plate int) *Model {
	if plate == 0 || len(m.Plates) == 0 {
		return m
	}
	out := &Model{Plates: m.Plates}
	for _, p := range m.Parts {
		if p.Plate != plate {
			continue
		}
		np := p
		np.Start = out.Triangles()
		out.Tris = append(out.Tris, m.Tris[p.Start*9:(p.Start+p.Count)*9]...)
		out.Parts = append(out.Parts, np)
	}
	if out.Triangles() == 0 {
		return m
	}
	return out
}

// FirstPlate is the lowest plate number that has geometry, or 0.
func (m *Model) FirstPlate() int {
	first := 0
	for _, p := range m.Parts {
		if p.Plate > 0 && (first == 0 || p.Plate < first) {
			first = p.Plate
		}
	}
	return first
}

// normColor turns "#RGB", "#RRGGBB" or "#RRGGBBAA" (with or without '#') into "#rrggbb".
func normColor(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	switch len(s) {
	case 3:
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	case 6, 8:
		s = s[:6]
	default:
		return ""
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return ""
		}
	}
	return "#" + strings.ToLower(s)
}

func rgbaToHex(c [4]float64) string {
	b := func(v float64) int {
		v = max(0, min(1, v))
		return int(v*255 + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", b(c[0]), b(c[1]), b(c[2]))
}

// LoadModel reads a model file of the given format name (see package formats),
// including parts, colours and plates where the format has them.
func LoadModel(path, format string) (*Model, error) {
	var (
		m   *Model
		err error
	)
	switch format {
	case "3mf":
		m, err = Read3MFModel(path)
	case "glb", "gltf":
		m, err = ReadGLBModel(path)
	case "stl", "obj", "ply":
		var f *os.File
		if f, err = os.Open(path); err != nil {
			return nil, err
		}
		defer f.Close()
		var mesh *Mesh
		switch format {
		case "stl":
			var st os.FileInfo
			if st, err = f.Stat(); err != nil {
				return nil, err
			}
			mesh, err = ReadSTL(f, st.Size())
		case "obj":
			mesh, err = ReadOBJ(f)
		case "ply":
			mesh, err = ReadPLY(f)
		}
		if mesh != nil {
			m = &Model{Mesh: *mesh}
		}
	default:
		return nil, ErrUnsupported
	}
	if err != nil {
		return nil, err
	}
	m.finish()
	if m.Triangles() == 0 {
		return nil, ErrEmpty
	}
	return m, nil
}
