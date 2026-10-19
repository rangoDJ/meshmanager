package mesh

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
)

// cubesMeshXML returns one <mesh> holding a cube per spec {size, x, y, z}.
func cubesMeshXML(cubes ...[4]float32) string {
	var b strings.Builder
	b.WriteString("<mesh><vertices>")
	for _, c := range cubes {
		for _, v := range cubeVerts {
			fmt.Fprintf(&b, `<vertex x="%g" y="%g" z="%g"/>`, v[0]*c[0]+c[1], v[1]*c[0]+c[2], v[2]*c[0]+c[3])
		}
	}
	b.WriteString("</vertices><triangles>")
	for n := range cubes {
		o := n * 8
		for _, q := range cubeQuads {
			fmt.Fprintf(&b, `<triangle v1="%d" v2="%d" v3="%d"/><triangle v1="%d" v2="%d" v3="%d"/>`,
				q[0]+o, q[1]+o, q[2]+o, q[0]+o, q[2]+o, q[3]+o)
		}
	}
	b.WriteString("</triangles></mesh>")
	return b.String()
}

const coreNS = `xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02"`

// A Bambu Studio style project: meshes in a sub-model, two plates, a modifier part,
// a second instance of the same object, and per-part filament colours.
func TestBambuPlatesAndColors(t *testing.T) {
	root := `<?xml version="1.0"?>
<model unit="millimeter" ` + coreNS + ` xmlns:p="http://schemas.microsoft.com/3dmanufacturing/production/2015/06">
 <resources>
  <object id="2" type="model"><components>
   <component p:path="/3D/Objects/object_1.model" objectid="1"/>
   <component p:path="/3D/Objects/object_1.model" objectid="3"/>
  </components></object>
  <object id="4" type="model"><components>
   <component p:path="/3D/Objects/object_1.model" objectid="5"/>
  </components></object>
 </resources>
 <build>
  <item objectid="2" transform="1 0 0 0 1 0 0 0 1 0 0 0"/>
  <item objectid="2" transform="1 0 0 0 1 0 0 0 1 300 0 0"/>
  <item objectid="4" transform="1 0 0 0 1 0 0 0 1 350 0 0"/>
 </build>
</model>`
	objects := `<?xml version="1.0"?><model unit="millimeter" ` + coreNS + `><resources>
 <object id="1" type="model">` + cubesMeshXML([4]float32{10, 0, 0, 0}) + `</object>
 <object id="3" type="model">` + cubesMeshXML([4]float32{5, 2, 2, 2}) + `</object>
 <object id="5" type="model">` + cubesMeshXML([4]float32{20, 0, 0, 0}) + `</object>
</resources></model>`
	settings := `<?xml version="1.0" encoding="UTF-8"?>
<config>
  <object id="2">
    <metadata key="name" value="Benchy"/>
    <metadata key="extruder" value="1"/>
    <part id="1" subtype="normal_part"><metadata key="name" value="Hull"/><metadata key="extruder" value="2"/></part>
    <part id="3" subtype="modifier_part"><metadata key="name" value="Infill modifier"/></part>
  </object>
  <object id="4">
    <metadata key="name" value="Box"/>
    <metadata key="extruder" value="3"/>
    <part id="5" subtype="normal_part"><metadata key="name" value="Box"/></part>
  </object>
  <plate>
    <metadata key="plater_id" value="1"/>
    <metadata key="plater_name" value="First"/>
    <model_instance><metadata key="object_id" value="2"/><metadata key="instance_id" value="0"/></model_instance>
  </plate>
  <plate>
    <metadata key="plater_id" value="2"/>
    <model_instance><metadata key="object_id" value="2"/><metadata key="instance_id" value="1"/></model_instance>
    <model_instance><metadata key="object_id" value="4"/><metadata key="instance_id" value="0"/></model_instance>
  </plate>
</config>`
	project := `{"filament_colour": ["#FF0000", "#00FF00", "#0000FFFF"], "printer_model": "Bambu Lab X1 Carbon"}`

	p := zipFile(t, "project.3mf", map[string]string{
		"3D/3dmodel.model":                 root,
		"3D/Objects/object_1.model":        objects,
		"Metadata/model_settings.config":   settings,
		"Metadata/project_settings.config": project,
	})
	m, err := LoadModel(p, "3mf")
	if err != nil {
		t.Fatal(err)
	}

	want := []Part{
		{Name: "Hull", Plate: 1, Color: "#00ff00", Start: 0, Count: 12},
		{Name: "Hull", Plate: 2, Color: "#00ff00", Start: 12, Count: 12},
		{Name: "Box", Plate: 2, Color: "#0000ff", Start: 24, Count: 12},
	}
	if fmt.Sprint(m.Parts) != fmt.Sprint(want) {
		t.Fatalf("parts:\n got %+v\nwant %+v", m.Parts, want)
	}
	if fmt.Sprint(m.Plates) != fmt.Sprint([]Plate{{1, "First"}, {2, ""}}) {
		t.Errorf("plates: %+v", m.Plates)
	}
	if m.FirstPlate() != 1 {
		t.Errorf("first plate = %d", m.FirstPlate())
	}

	plate2 := m.OnPlate(2)
	if plate2.Triangles() != 24 || len(plate2.Parts) != 2 || plate2.Parts[1].Start != 12 {
		t.Fatalf("plate 2: %d triangles, parts %+v", plate2.Triangles(), plate2.Parts)
	}
	if st := plate2.Stats(); !near(st.Min[0], 300) || !near(st.Max[0], 370) {
		t.Errorf("plate 2 extent x: %v..%v", st.Min[0], st.Max[0])
	}
	if m.OnPlate(9) != m {
		t.Error("unknown plate should return the whole model")
	}
}

func TestPrusaVolumes(t *testing.T) {
	model := `<?xml version="1.0"?><model unit="millimeter" ` + coreNS + `><resources>
 <object id="1" type="model">` + cubesMeshXML([4]float32{10, 0, 0, 0}, [4]float32{4, 3, 3, 3}) + `</object>
</resources><build><item objectid="1"/></build></model>`
	volumes := `<?xml version="1.0"?><config>
 <object id="1" instances_count="1">
  <metadata type="object" key="name" value="Bracket"/>
  <volume firstid="0" lastid="11">
   <metadata type="volume" key="name" value="Body"/>
   <metadata type="volume" key="volume_type" value="ModelPart"/>
   <metadata type="volume" key="extruder" value="2"/>
  </volume>
  <volume firstid="12" lastid="23">
   <metadata type="volume" key="name" value="Modifier"/>
   <metadata type="volume" key="volume_type" value="ParameterModifier"/>
  </volume>
 </object>
</config>`
	p := zipFile(t, "prusa.3mf", map[string]string{
		"3D/3dmodel.model":                model,
		"Metadata/Slic3r_PE_model.config": volumes,
		"Metadata/Slic3r_PE.config":       "; generated by PrusaSlicer\n; filament_colour = #FF8000;#00FFFF\n",
	})
	m, err := LoadModel(p, "3mf")
	if err != nil {
		t.Fatal(err)
	}
	want := []Part{{Name: "Body", Color: "#00ffff", Start: 0, Count: 12}}
	if fmt.Sprint(m.Parts) != fmt.Sprint(want) {
		t.Fatalf("parts: got %+v, want %+v", m.Parts, want)
	}
}

func TestCoreMaterialColor(t *testing.T) {
	model := `<?xml version="1.0"?><model unit="millimeter" ` + coreNS + `><resources>
 <basematerials id="5"><base name="Red" displaycolor="#FF0000FF"/><base name="Blue" displaycolor="#0000FF"/></basematerials>
 <object id="1" name="Blue cube" pid="5" pindex="1">` + cubesMeshXML([4]float32{10, 0, 0, 0}) + `</object>
</resources><build><item objectid="1"/></build></model>`
	m, err := LoadModel(zipFile(t, "colors.3mf", map[string]string{"3D/3dmodel.model": model}), "3mf")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Parts) != 1 || m.Parts[0].Color != "#0000ff" || m.Parts[0].Name != "Blue cube" {
		t.Fatalf("parts: %+v", m.Parts)
	}
}

func TestGLBMaterialColor(t *testing.T) {
	doc := gltf.NewDocument()
	var positions [][3]float32
	for _, tri := range cubeTris(0.01) {
		for _, v := range tri {
			positions = append(positions, [3]float32{v[0], v[2], -v[1]})
		}
	}
	doc.Materials = []*gltf.Material{{
		Name:                 "Orange",
		PBRMetallicRoughness: &gltf.PBRMetallicRoughness{BaseColorFactor: &[4]float64{1, 0.5, 0, 1}},
	}}
	doc.Meshes = []*gltf.Mesh{{Name: "Knob", Primitives: []*gltf.Primitive{{
		Attributes: gltf.PrimitiveAttributes{gltf.POSITION: modeler.WritePosition(doc, positions)},
		Material:   gltf.Index(0),
	}}}}
	doc.Nodes = []*gltf.Node{{Mesh: gltf.Index(0)}}
	doc.Scenes[0].Nodes = []int{0}
	p := filepath.Join(t.TempDir(), "knob.glb")
	if err := gltf.SaveBinary(doc, p); err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel(p, "glb")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Parts) != 1 || m.Parts[0].Name != "Knob" || m.Parts[0].Color != "#ff8000" {
		t.Fatalf("parts: %+v", m.Parts)
	}
}

func TestSinglePartFormats(t *testing.T) {
	p := writeFile(t, "cube.stl", binarySTL(cubeTris(10), ""))
	m, err := LoadModel(p, "stl")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Parts) != 1 || m.Parts[0].Count != 12 || len(m.Plates) != 0 {
		t.Fatalf("parts %+v plates %+v", m.Parts, m.Plates)
	}
}

func TestWatertight(t *testing.T) {
	closedCube := &Mesh{}
	for _, tri := range cubeTris(10) {
		_ = closedCube.addTri(tri[0], tri[1], tri[2])
	}
	if closed, checked := closedCube.Watertight(); !closed || !checked {
		t.Errorf("cube: closed=%v checked=%v", closed, checked)
	}

	open := &Mesh{Tris: append([]float32(nil), closedCube.Tris[:len(closedCube.Tris)-9]...)}
	if closed, _ := open.Watertight(); closed {
		t.Error("cube with a missing triangle reported closed")
	}
	// Degenerate triangles (two corners at the same point) are ignored.
	withDegenerate := &Mesh{Tris: append(append([]float32(nil), closedCube.Tris...), 0, 0, 0, 0, 0, 0, 10, 0, 0)}
	if closed, _ := withDegenerate.Watertight(); !closed {
		t.Error("a degenerate triangle made a closed cube report open")
	}
	if _, checked := (&Mesh{}).Watertight(); checked {
		t.Error("empty mesh should not be checked")
	}
}
