package mesh

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// A 3MF file is a zip of XML "model" parts. Objects hold a mesh or components that
// reference other objects, possibly in other parts (Bambu/Orca put meshes in
// 3D/Objects/*.model via the production extension's p:path). Build items place objects.
// Slicers add their own metadata for plates, part types and filament colours; see
// slicer.go.

const maxComponentDepth = 16

// affine is a 3MF 3x4 transform in row-vector form: p' = p·M + t.
type affine [12]float64

var identity = affine{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}

func (m affine) apply(x, y, z float64) (float64, float64, float64) {
	return x*m[0] + y*m[3] + z*m[6] + m[9],
		x*m[1] + y*m[4] + z*m[7] + m[10],
		x*m[2] + y*m[5] + z*m[8] + m[11]
}

// then returns the transform that applies m first, then n.
func (m affine) then(n affine) affine {
	var r affine
	tx, ty, tz := n.apply(m.apply(0, 0, 0))
	r[9], r[10], r[11] = tx, ty, tz
	for i, e := range [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}} {
		x, y, z := n.apply(m.apply(e[0], e[1], e[2]))
		r[i*3], r[i*3+1], r[i*3+2] = x-tx, y-ty, z-tz
	}
	return r
}

func parseAffine(s string) (affine, error) {
	if s == "" {
		return identity, nil
	}
	f := strings.Fields(s)
	if len(f) != 12 {
		return identity, fmt.Errorf("bad transform %q", s)
	}
	var m affine
	for i, v := range f {
		x, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return identity, fmt.Errorf("bad transform %q", s)
		}
		m[i] = x
	}
	return m, nil
}

type tmfComponent struct {
	objectID string
	part     string
	xf       affine
}

type tmfObject struct {
	name       string
	pid        string
	pindex     int
	verts      [][3]float32
	tris       [][3]int32
	components []tmfComponent
}

type tmfPart struct {
	objects map[string]*tmfObject
	build   []tmfComponent
	// colours of basematerials and colorgroups, by property group id
	materials map[string][]string
}

type threeMF struct {
	files  map[string]*zip.File
	parts  map[string]*tmfPart
	slicer *slicerInfo
	model  *Model
}

// Read3MF loads the geometry of every build item of a 3MF, with transforms applied.
func Read3MF(filename string) (*Mesh, error) {
	m, err := Read3MFModel(filename)
	if err != nil {
		return nil, err
	}
	return &m.Mesh, nil
}

// Read3MFModel loads a 3MF with one part per object or volume, plate assignments and
// colours from the slicer project data when present.
func Read3MFModel(filename string) (*Model, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	files := zipIndex(&zr.Reader)
	t := &threeMF{files: files, parts: map[string]*tmfPart{}, slicer: readSlicerInfo(files), model: &Model{}}
	t.model.Plates = t.slicer.plates

	root := t.rootPart()
	rp, err := t.part(root)
	if err != nil {
		return nil, err
	}
	if len(rp.build) == 0 {
		// No build section: show every object that has a mesh.
		for id := range rp.objects {
			rp.build = append(rp.build, tmfComponent{objectID: id, part: root, xf: identity})
		}
	}
	instances := map[string]int{}
	for _, item := range rp.build {
		inst := instances[item.objectID]
		instances[item.objectID]++
		plate := t.slicer.plateOf[instanceKey{item.objectID, inst}]
		if err := t.emit(item.part, item.objectID, item.xf, plate, item.objectID, 0); err != nil {
			return nil, err
		}
	}
	t.model.finish()
	return t.model, nil
}

func zipIndex(zr *zip.Reader) map[string]*zip.File {
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[strings.ToLower(strings.TrimPrefix(f.Name, "/"))] = f
	}
	return files
}

type relationships struct {
	Rels []struct {
		Type   string `xml:"Type,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

func readRels(files map[string]*zip.File) relationships {
	var rels relationships
	if f := files["_rels/.rels"]; f != nil {
		if rc, err := f.Open(); err == nil {
			_ = xml.NewDecoder(rc).Decode(&rels)
			rc.Close()
		}
	}
	return rels
}

func (t *threeMF) rootPart() string {
	for _, r := range readRels(t.files).Rels {
		if strings.HasSuffix(r.Type, "/3dmodel") {
			return normPart(r.Target)
		}
	}
	return "3d/3dmodel.model"
}

func normPart(p string) string {
	return strings.ToLower(strings.TrimPrefix(path.Clean("/"+p), "/"))
}

// emit adds an object (and its components) to the model. root is the id of the
// build-level object, which slicer metadata is keyed by.
func (t *threeMF) emit(partFile, objectID string, xf affine, plate int, root string, depth int) error {
	if depth > maxComponentDepth {
		return errors.New("3MF components nested too deeply")
	}
	p, err := t.part(partFile)
	if err != nil {
		return err
	}
	obj := p.objects[objectID]
	if obj == nil {
		return nil
	}

	if len(obj.tris) > 0 {
		if vols := t.slicer.prusaVolumes[objectID]; depth == 0 && len(vols) > 0 {
			// PrusaSlicer: one object, split into volumes by triangle range.
			for _, v := range vols {
				if !v.modelPart {
					continue
				}
				name := firstNonEmpty(v.name, obj.name, "Object "+objectID)
				t.model.beginPart(name, plate, t.slicer.color(v.extruder))
				last := min(v.last, len(obj.tris)-1)
				if err := t.addTris(obj, obj.tris[min(v.first, last+1):last+1], xf); err != nil {
					return err
				}
			}
		} else if name, extruder, skip := t.slicer.describe(root, objectID, obj.name); !skip {
			color := t.slicer.color(extruder)
			if color == "" {
				color = p.materialColor(obj)
			}
			t.model.beginPart(firstNonEmpty(name, "Object "+objectID), plate, color)
			if err := t.addTris(obj, obj.tris, xf); err != nil {
				return err
			}
		}
	}
	for _, c := range obj.components {
		if err := t.emit(c.part, c.objectID, c.xf.then(xf), plate, root, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (t *threeMF) addTris(obj *tmfObject, tris [][3]int32, xf affine) error {
	for _, tri := range tris {
		var v [3][3]float32
		for k, i := range tri {
			if int(i) >= len(obj.verts) || i < 0 {
				continue
			}
			src := obj.verts[i]
			x, y, z := xf.apply(float64(src[0]), float64(src[1]), float64(src[2]))
			v[k] = [3]float32{float32(x), float32(y), float32(z)}
		}
		if err := t.model.addTri(v[0], v[1], v[2]); err != nil {
			return err
		}
	}
	return nil
}

func (p *tmfPart) materialColor(obj *tmfObject) string {
	colors := p.materials[obj.pid]
	if obj.pindex >= 0 && obj.pindex < len(colors) {
		return colors[obj.pindex]
	}
	return ""
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func (t *threeMF) part(name string) (*tmfPart, error) {
	if p, ok := t.parts[name]; ok {
		return p, nil
	}
	f := t.files[name]
	if f == nil {
		return nil, fmt.Errorf("3MF part %q not found", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	p, err := parseModelPart(rc, name)
	if err != nil {
		return nil, fmt.Errorf("3MF part %q: %w", name, err)
	}
	t.parts[name] = p
	return p, nil
}

var unitScale = map[string]float64{
	"micron": 0.001, "millimeter": 1, "centimeter": 10, "inch": 25.4, "foot": 304.8, "meter": 1000,
}

// parseModelPart streams a model XML part. Vertex coordinates are converted to mm.
func parseModelPart(r io.Reader, name string) (*tmfPart, error) {
	p := &tmfPart{objects: map[string]*tmfObject{}, materials: map[string][]string{}}
	dec := xml.NewDecoder(r)
	scale := 1.0
	var cur *tmfObject
	inBuild := false
	group := "" // current basematerials/colorgroup id
	vertexCount := 0

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return p, nil
		}
		if err != nil {
			return nil, err
		}
		switch el := tok.(type) {
		case xml.StartElement:
			a := attrs(el)
			switch el.Name.Local {
			case "model":
				if s, ok := unitScale[a["unit"]]; ok {
					scale = s
				}
			case "basematerials", "colorgroup":
				group = a["id"]
			case "base":
				if group != "" {
					p.materials[group] = append(p.materials[group], normColor(a["displaycolor"]))
				}
			case "color":
				if group != "" {
					p.materials[group] = append(p.materials[group], normColor(a["color"]))
				}
			case "object":
				cur = &tmfObject{name: a["name"], pid: a["pid"], pindex: -1}
				if v, err := strconv.Atoi(a["pindex"]); err == nil {
					cur.pindex = v
				} else if a["pid"] != "" {
					cur.pindex = 0
				}
				p.objects[a["id"]] = cur
			case "vertex":
				if cur == nil {
					continue
				}
				if vertexCount++; vertexCount > MaxTriangles*3 {
					return nil, ErrTooLarge
				}
				var v [3]float32
				for k, key := range [3]string{"x", "y", "z"} {
					x, err := strconv.ParseFloat(a[key], 64)
					if err != nil {
						return nil, fmt.Errorf("bad vertex %s=%q", key, a[key])
					}
					v[k] = float32(x * scale)
				}
				cur.verts = append(cur.verts, v)
			case "triangle":
				if cur == nil {
					continue
				}
				var tri [3]int32
				for k, key := range [3]string{"v1", "v2", "v3"} {
					n, err := strconv.ParseInt(a[key], 10, 32)
					if err != nil {
						return nil, fmt.Errorf("bad triangle %s=%q", key, a[key])
					}
					tri[k] = int32(n)
				}
				cur.tris = append(cur.tris, tri)
			case "component", "item":
				xf, err := parseAffine(a["transform"])
				if err != nil {
					return nil, err
				}
				c := tmfComponent{objectID: a["objectid"], part: name, xf: xf}
				if a["path"] != "" {
					c.part = normPart(a["path"])
				}
				if el.Name.Local == "item" && inBuild {
					p.build = append(p.build, c)
				} else if el.Name.Local == "component" && cur != nil {
					cur.components = append(cur.components, c)
				}
			case "build":
				inBuild = true
			}
		case xml.EndElement:
			switch el.Name.Local {
			case "object":
				cur = nil
			case "build":
				inBuild = false
			case "basematerials", "colorgroup":
				group = ""
			}
		}
	}
}

// attrs maps attribute local names to values; the production extension's p:path
// becomes "path".
func attrs(el xml.StartElement) map[string]string {
	m := make(map[string]string, len(el.Attr))
	for _, a := range el.Attr {
		m[a.Name.Local] = a.Value
	}
	return m
}

// ThreeMFThumbnail returns the preview image a slicer embedded in a 3MF, if any.
// Bambu/Orca store per-plate renders; most others store Metadata/thumbnail.png.
func ThreeMFThumbnail(filename string) ([]byte, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	files := zipIndex(&zr.Reader)

	candidates := []string{"metadata/plate_1.png"}
	for _, r := range readRels(files).Rels {
		if strings.HasSuffix(strings.ToLower(r.Type), "/metadata/thumbnail") {
			candidates = append(candidates, normPart(r.Target))
		}
	}
	candidates = append(candidates, "metadata/thumbnail.png")

	for _, name := range candidates {
		b := readZipFile(files[name], 20<<20)
		if b != nil {
			return b, nil
		}
	}
	return nil, nil
}

// readZipFile returns a member's content, or nil if it's missing, empty or too big.
func readZipFile(f *zip.File, limit uint64) []byte {
	if f == nil || f.UncompressedSize64 == 0 || f.UncompressedSize64 > limit {
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil
	}
	return b
}
