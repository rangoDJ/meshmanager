package mesh

import (
	"errors"
	"fmt"
	"math"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
)

const maxNodeDepth = 64

// mat4 is a column-major 4x4 matrix, as glTF stores it.
type mat4 [16]float64

var ident4 = mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}

func (a mat4) mul(b mat4) mat4 {
	var r mat4
	for c := range 4 {
		for row := range 4 {
			var s float64
			for k := range 4 {
				s += a[k*4+row] * b[c*4+k]
			}
			r[c*4+row] = s
		}
	}
	return r
}

func (a mat4) apply(p [3]float32) [3]float32 {
	x, y, z := float64(p[0]), float64(p[1]), float64(p[2])
	return [3]float32{
		float32(a[0]*x + a[4]*y + a[8]*z + a[12]),
		float32(a[1]*x + a[5]*y + a[9]*z + a[13]),
		float32(a[2]*x + a[6]*y + a[10]*z + a[14]),
	}
}

func nodeMatrix(n *gltf.Node) mat4 {
	if m := mat4(n.MatrixOrDefault()); m != ident4 {
		return m
	}
	t := n.TranslationOrDefault()
	q := n.RotationOrDefault() // x, y, z, w
	s := n.ScaleOrDefault()
	x, y, z, w := q[0], q[1], q[2], q[3]
	rot := mat4{
		1 - 2*(y*y+z*z), 2 * (x*y + z*w), 2 * (x*z - y*w), 0,
		2 * (x*y - z*w), 1 - 2*(x*x+z*z), 2 * (y*z + x*w), 0,
		2 * (x*z + y*w), 2 * (y*z - x*w), 1 - 2*(x*x+y*y), 0,
		0, 0, 0, 1,
	}
	scale := mat4{s[0], 0, 0, 0, 0, s[1], 0, 0, 0, 0, s[2], 0, 0, 0, 0, 1}
	trans := ident4
	trans[12], trans[13], trans[14] = t[0], t[1], t[2]
	return trans.mul(rot).mul(scale)
}

// ReadGLB loads the geometry of the default scene of a binary glTF.
func ReadGLB(path string) (*Mesh, error) {
	m, err := ReadGLBModel(path)
	if err != nil {
		return nil, err
	}
	return &m.Mesh, nil
}

// ReadGLBModel loads the default scene of a binary glTF with node transforms applied,
// one part per primitive coloured by its material. glTF is Y-up; the result is Z-up
// millimetres.
func ReadGLBModel(path string) (*Model, error) {
	doc, err := gltf.Open(path)
	if err != nil {
		return nil, err
	}
	g := &glbLoader{doc: doc, mesh: &Model{}}

	var roots []int
	switch {
	case doc.Scene != nil && *doc.Scene < len(doc.Scenes):
		roots = doc.Scenes[*doc.Scene].Nodes
	case len(doc.Scenes) > 0:
		roots = doc.Scenes[0].Nodes
	}
	if len(roots) == 0 {
		for i := range doc.Meshes {
			if err := g.addMesh(i, ident4, ""); err != nil {
				return nil, err
			}
		}
	}
	for _, n := range roots {
		if err := g.walk(n, ident4, 0); err != nil {
			return nil, err
		}
	}
	// glTF is specified in metres, but many exporters write millimetre values anyway.
	// A model under 5 units across is almost certainly in metres.
	if st := g.mesh.Stats(); st.Triangles > 0 {
		size := st.Size()
		if math.Max(size[0], math.Max(size[1], size[2])) < 5 {
			for i := range g.mesh.Tris {
				g.mesh.Tris[i] *= 1000
			}
		}
	}
	g.mesh.yUpToZUp()
	return g.mesh, nil
}

type glbLoader struct {
	doc  *gltf.Document
	mesh *Model
}

func (g *glbLoader) walk(idx int, parent mat4, depth int) error {
	if idx < 0 || idx >= len(g.doc.Nodes) || depth > maxNodeDepth {
		return nil
	}
	n := g.doc.Nodes[idx]
	world := parent.mul(nodeMatrix(n))
	if n.Mesh != nil {
		if err := g.addMesh(*n.Mesh, world, n.Name); err != nil {
			return err
		}
	}
	for _, c := range n.Children {
		if err := g.walk(c, world, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (g *glbLoader) addMesh(idx int, xf mat4, nodeName string) error {
	if idx < 0 || idx >= len(g.doc.Meshes) {
		return nil
	}
	m := g.doc.Meshes[idx]
	for _, prim := range m.Primitives {
		if _, draco := prim.Extensions["KHR_draco_mesh_compression"]; draco {
			return errors.New("Draco-compressed glTF is not supported yet")
		}
		if prim.Mode != gltf.PrimitiveTriangles {
			continue
		}
		posIdx, ok := prim.Attributes[gltf.POSITION]
		if !ok || posIdx >= len(g.doc.Accessors) {
			continue
		}
		pos, err := modeler.ReadPosition(g.doc, g.doc.Accessors[posIdx], nil)
		if err != nil {
			return err
		}
		var indices []uint32
		if prim.Indices != nil && *prim.Indices < len(g.doc.Accessors) {
			if indices, err = modeler.ReadIndices(g.doc, g.doc.Accessors[*prim.Indices], nil); err != nil {
				return err
			}
		} else {
			indices = make([]uint32, len(pos))
			for i := range indices {
				indices[i] = uint32(i)
			}
		}
		g.mesh.beginPart(firstNonEmpty(m.Name, nodeName, fmt.Sprintf("Mesh %d", idx+1)), 0, g.materialColor(prim))
		world := make([][3]float32, len(pos))
		for i, p := range pos {
			world[i] = xf.apply(p)
		}
		for i := 0; i+2 < len(indices); i += 3 {
			a, b, c := indices[i], indices[i+1], indices[i+2]
			if int(a) >= len(world) || int(b) >= len(world) || int(c) >= len(world) {
				continue
			}
			if err := g.mesh.addTri(world[a], world[b], world[c]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *glbLoader) materialColor(prim *gltf.Primitive) string {
	if prim.Material == nil || *prim.Material >= len(g.doc.Materials) {
		return ""
	}
	pbr := g.doc.Materials[*prim.Material].PBRMetallicRoughness
	if pbr == nil || pbr.BaseColorFactor == nil {
		return ""
	}
	return rgbaToHex(*pbr.BaseColorFactor)
}
