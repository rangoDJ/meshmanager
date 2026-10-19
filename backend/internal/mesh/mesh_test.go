package mesh

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
)

// Unit cube corners and quad faces (outward winding), scaled to 10 mm in tests.
var cubeVerts = [8][3]float32{
	{0, 0, 0}, {1, 0, 0}, {1, 1, 0}, {0, 1, 0},
	{0, 0, 1}, {1, 0, 1}, {1, 1, 1}, {0, 1, 1},
}
var cubeQuads = [6][4]int{
	{0, 3, 2, 1}, {4, 5, 6, 7}, {0, 1, 5, 4}, {2, 3, 7, 6}, {1, 2, 6, 5}, {0, 4, 7, 3},
}

func cubeTris(size float32) [][3][3]float32 {
	var tris [][3][3]float32
	v := func(i int) [3]float32 {
		c := cubeVerts[i]
		return [3]float32{c[0] * size, c[1] * size, c[2] * size}
	}
	for _, q := range cubeQuads {
		tris = append(tris, [3][3]float32{v(q[0]), v(q[1]), v(q[2])}, [3][3]float32{v(q[0]), v(q[2]), v(q[3])})
	}
	return tris
}

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-3 }

func checkCube(t *testing.T, m *Mesh, size float64) {
	t.Helper()
	st := m.Stats()
	if st.Triangles != 12 {
		t.Errorf("triangles = %d, want 12", st.Triangles)
	}
	for k, s := range st.Size() {
		if !near(s, size) {
			t.Errorf("size[%d] = %v, want %v", k, s, size)
		}
	}
	if !near(st.Volume, size*size*size) {
		t.Errorf("volume = %v, want %v", st.Volume, size*size*size)
	}
	if !near(st.Area, 6*size*size) {
		t.Errorf("area = %v, want %v", st.Area, 6*size*size)
	}
}

func binarySTL(tris [][3][3]float32, header string) []byte {
	var b bytes.Buffer
	h := make([]byte, 80)
	copy(h, header)
	b.Write(h)
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(tris)))
	for _, tri := range tris {
		_ = binary.Write(&b, binary.LittleEndian, [3]float32{}) // normal
		for _, v := range tri {
			_ = binary.Write(&b, binary.LittleEndian, v)
		}
		b.Write([]byte{0, 0})
	}
	return b.Bytes()
}

func TestSTLBinary(t *testing.T) {
	// Header starts with "solid" on purpose: size must win over the prefix.
	p := writeFile(t, "cube.stl", binarySTL(cubeTris(10), "solid but actually binary"))
	m, err := Load(p, "stl")
	if err != nil {
		t.Fatal(err)
	}
	checkCube(t, m, 10)
}

func TestSTLASCII(t *testing.T) {
	var b strings.Builder
	b.WriteString("solid cube\n")
	for _, tri := range cubeTris(10) {
		b.WriteString("  facet normal 0 0 0\n    outer loop\n")
		for _, v := range tri {
			fmt.Fprintf(&b, "      vertex %g %g %g\n", v[0], v[1], v[2])
		}
		b.WriteString("    endloop\n  endfacet\n")
	}
	b.WriteString("endsolid cube\n")
	m, err := Load(writeFile(t, "cube.stl", []byte(b.String())), "stl")
	if err != nil {
		t.Fatal(err)
	}
	checkCube(t, m, 10)
}

func TestOBJ(t *testing.T) {
	var b strings.Builder
	b.WriteString("# cube\no Cube\n")
	for _, v := range cubeVerts {
		fmt.Fprintf(&b, "v %g %g %g\n", v[0]*10, v[1]*10, v[2]*10)
	}
	b.WriteString("vn 0 0 1\nvt 0 0\n")
	for i, q := range cubeQuads {
		if i == 0 {
			// Negative (relative) indices with v/vt/vn syntax.
			fmt.Fprintf(&b, "f %d/1/1 %d/1/1 %d/1/1 %d/1/1\n", q[0]-8, q[1]-8, q[2]-8, q[3]-8)
			continue
		}
		fmt.Fprintf(&b, "f %d %d %d %d\n", q[0]+1, q[1]+1, q[2]+1, q[3]+1)
	}
	m, err := Load(writeFile(t, "cube.obj", []byte(b.String())), "obj")
	if err != nil {
		t.Fatal(err)
	}
	checkCube(t, m, 10)
}

func TestPLY(t *testing.T) {
	header := "ply\nformat %s 1.0\ncomment test\nelement vertex 8\nproperty float x\nproperty float y\nproperty float z\n" +
		"property uchar red\nelement face 6\nproperty list uchar int vertex_indices\nend_header\n"

	var ascii strings.Builder
	fmt.Fprintf(&ascii, header, "ascii")
	for _, v := range cubeVerts {
		fmt.Fprintf(&ascii, "%g %g %g 255\n", v[0]*10, v[1]*10, v[2]*10)
	}
	for _, q := range cubeQuads {
		fmt.Fprintf(&ascii, "4 %d %d %d %d\n", q[0], q[1], q[2], q[3])
	}

	var bin bytes.Buffer
	fmt.Fprintf(&bin, header, "binary_little_endian")
	for _, v := range cubeVerts {
		_ = binary.Write(&bin, binary.LittleEndian, [3]float32{v[0] * 10, v[1] * 10, v[2] * 10})
		bin.WriteByte(255)
	}
	for _, q := range cubeQuads {
		bin.WriteByte(4)
		for _, i := range q {
			_ = binary.Write(&bin, binary.LittleEndian, int32(i))
		}
	}

	for name, data := range map[string][]byte{"ascii": []byte(ascii.String()), "binary": bin.Bytes()} {
		t.Run(name, func(t *testing.T) {
			m, err := Load(writeFile(t, "cube.ply", data), "ply")
			if err != nil {
				t.Fatal(err)
			}
			checkCube(t, m, 10)
		})
	}
}

func cubeMeshXML(scale float32) string {
	var b strings.Builder
	b.WriteString("<mesh><vertices>")
	for _, v := range cubeVerts {
		fmt.Fprintf(&b, `<vertex x="%g" y="%g" z="%g"/>`, v[0]*scale, v[1]*scale, v[2]*scale)
	}
	b.WriteString("</vertices><triangles>")
	for _, q := range cubeQuads {
		fmt.Fprintf(&b, `<triangle v1="%d" v2="%d" v3="%d"/><triangle v1="%d" v2="%d" v3="%d"/>`,
			q[0], q[1], q[2], q[0], q[2], q[3])
	}
	b.WriteString("</triangles></mesh>")
	return b.String()
}

func zipFile(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, content := range files {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return writeFile(t, name, buf.Bytes())
}

func Test3MFSimple(t *testing.T) {
	model := `<?xml version="1.0" encoding="UTF-8"?>
<model unit="centimeter" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02">
 <resources><object id="1" type="model">` + cubeMeshXML(1) + `</object></resources>
 <build><item objectid="1"/></build>
</model>`
	p := zipFile(t, "cube.3mf", map[string]string{"3D/3dmodel.model": model})
	m, err := Load(p, "3mf")
	if err != nil {
		t.Fatal(err)
	}
	checkCube(t, m, 10) // 1 cm cube in centimetre units = 10 mm
}

// Bambu-style: root model references a mesh in another part via p:path, with
// component and build transforms that must compose.
func Test3MFProductionComponents(t *testing.T) {
	rels := `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
 <Relationship Target="/3D/3dmodel.model" Id="rel0" Type="http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel"/>
 <Relationship Target="/Metadata/thumbnail.png" Id="rel1" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/thumbnail"/>
</Relationships>`
	root := `<?xml version="1.0"?>
<model unit="millimeter" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02"
       xmlns:p="http://schemas.microsoft.com/3dmanufacturing/production/2015/06">
 <resources>
  <object id="2" type="model"><components>
   <component p:path="/3D/Objects/object_1.model" objectid="1" transform="2 0 0 0 2 0 0 0 2 0 0 0"/>
  </components></object>
 </resources>
 <build><item objectid="2" transform="1 0 0 0 1 0 0 0 1 100 50 0"/></build>
</model>`
	part := `<?xml version="1.0"?><model unit="millimeter" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02">
 <resources><object id="1" type="model">` + cubeMeshXML(5) + `</object></resources></model>`

	p := zipFile(t, "plate.3mf", map[string]string{
		"_rels/.rels":               rels,
		"3D/3dmodel.model":          root,
		"3D/Objects/object_1.model": part,
		"Metadata/thumbnail.png":    "PNGDATA",
	})
	m, err := Load(p, "3mf")
	if err != nil {
		t.Fatal(err)
	}
	checkCube(t, m, 10) // 5 mm cube scaled by 2
	st := m.Stats()
	if !near(st.Min[0], 100) || !near(st.Min[1], 50) {
		t.Errorf("build transform not applied: min = %v", st.Min)
	}

	thumb, err := ThreeMFThumbnail(p)
	if err != nil || string(thumb) != "PNGDATA" {
		t.Errorf("embedded thumbnail = %q, %v", thumb, err)
	}
}

func TestAffineThen(t *testing.T) {
	scale2 := affine{2, 0, 0, 0, 2, 0, 0, 0, 2, 0, 0, 0}
	move := affine{1, 0, 0, 0, 1, 0, 0, 0, 1, 10, 0, 0}
	x, _, _ := scale2.then(move).apply(1, 0, 0)
	if x != 12 { // scale first, then move
		t.Fatalf("scale then move: got %v, want 12", x)
	}
	x, _, _ = move.then(scale2).apply(1, 0, 0)
	if x != 22 { // move first, then scale
		t.Fatalf("move then scale: got %v, want 22", x)
	}
}

func TestGLB(t *testing.T) {
	doc := gltf.NewDocument()
	var positions [][3]float32
	var indices []uint32
	for _, tri := range cubeTris(0.01) { // 10 mm, in metres
		for _, v := range tri {
			// Store Y-up: glTF (x, y, z) = our (x, z, -y)
			positions = append(positions, [3]float32{v[0], v[2], -v[1]})
			indices = append(indices, uint32(len(indices)))
		}
	}
	posAcc := modeler.WritePosition(doc, positions)
	idxAcc := modeler.WriteIndices(doc, indices)
	doc.Meshes = []*gltf.Mesh{{Primitives: []*gltf.Primitive{{
		Attributes: gltf.PrimitiveAttributes{gltf.POSITION: posAcc},
		Indices:    gltf.Index(idxAcc),
	}}}}
	doc.Nodes = []*gltf.Node{{Mesh: gltf.Index(0), Translation: [3]float64{0.1, 0, 0}}}
	doc.Scenes[0].Nodes = []int{0}

	p := filepath.Join(t.TempDir(), "cube.glb")
	if err := gltf.SaveBinary(doc, p); err != nil {
		t.Fatal(err)
	}
	m, err := Load(p, "glb")
	if err != nil {
		t.Fatal(err)
	}
	checkCube(t, m, 10)
	if st := m.Stats(); !near(st.Min[0], 100) || !near(st.Min[2], 0) {
		t.Errorf("node translation or axis conversion wrong: min = %v", st.Min)
	}
}

func TestRejectsGarbage(t *testing.T) {
	for _, f := range []string{"stl", "obj", "ply", "3mf", "glb"} {
		p := writeFile(t, "junk."+f, []byte("this is not a model"))
		if _, err := Load(p, f); err == nil {
			t.Errorf("%s: expected error for garbage input", f)
		}
	}
}
