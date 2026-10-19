package mesh

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"sort"
	"strconv"
	"strings"
)

// slicerInfo is what slicers store next to the 3MF geometry: which object instance sits
// on which plate, which parts are modifiers rather than printed geometry, and which
// filament (colour) each object or part uses.
//
//   - Bambu Studio / OrcaSlicer: Metadata/model_settings.config (objects, parts, plates)
//     and Metadata/project_settings.config (JSON, "filament_colour").
//   - PrusaSlicer: Metadata/Slic3r_PE_model.config (volumes as triangle ranges) and
//     Metadata/Slic3r_PE.config ("; filament_colour = #..;#..").
type slicerInfo struct {
	filamentColors []string // index 0 is extruder 1
	bambuObjects   map[string]*bambuObject
	plateOf        map[instanceKey]int
	plates         []Plate
	prusaVolumes   map[string][]prusaVolume
}

type instanceKey struct {
	objectID string
	instance int
}

type bambuObject struct {
	name     string
	extruder int
	parts    map[string]bambuPart
}

type bambuPart struct {
	name     string
	extruder int
	subtype  string
}

type prusaVolume struct {
	first, last int
	name        string
	extruder    int
	modelPart   bool
}

type kv struct {
	Type  string `xml:"type,attr"`
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}

func metaValue(meta []kv, key string) string {
	for _, m := range meta {
		if m.Key == key {
			return m.Value
		}
	}
	return ""
}

func metaInt(meta []kv, key string) int {
	n, _ := strconv.Atoi(metaValue(meta, key))
	return n
}

func readSlicerInfo(files map[string]*zip.File) *slicerInfo {
	s := &slicerInfo{
		bambuObjects: map[string]*bambuObject{},
		plateOf:      map[instanceKey]int{},
		prusaVolumes: map[string][]prusaVolume{},
	}
	s.readBambu(files)
	s.readPrusa(files)
	return s
}

func (s *slicerInfo) readBambu(files map[string]*zip.File) {
	if b := readZipFile(files["metadata/project_settings.config"], 16<<20); b != nil {
		var settings struct {
			FilamentColour []string `json:"filament_colour"`
		}
		if json.Unmarshal(b, &settings) == nil {
			for _, c := range settings.FilamentColour {
				s.filamentColors = append(s.filamentColors, normColor(c))
			}
		}
	}

	b := readZipFile(files["metadata/model_settings.config"], 64<<20)
	if b == nil {
		return
	}
	var cfg struct {
		Objects []struct {
			ID    string `xml:"id,attr"`
			Meta  []kv   `xml:"metadata"`
			Parts []struct {
				ID      string `xml:"id,attr"`
				Subtype string `xml:"subtype,attr"`
				Meta    []kv   `xml:"metadata"`
			} `xml:"part"`
		} `xml:"object"`
		Plates []struct {
			Meta      []kv `xml:"metadata"`
			Instances []struct {
				Meta []kv `xml:"metadata"`
			} `xml:"model_instance"`
		} `xml:"plate"`
	}
	if xml.Unmarshal(b, &cfg) != nil {
		return
	}
	for _, o := range cfg.Objects {
		bo := &bambuObject{name: metaValue(o.Meta, "name"), extruder: metaInt(o.Meta, "extruder"), parts: map[string]bambuPart{}}
		for _, p := range o.Parts {
			bo.parts[p.ID] = bambuPart{
				name:     metaValue(p.Meta, "name"),
				extruder: metaInt(p.Meta, "extruder"),
				subtype:  p.Subtype,
			}
		}
		s.bambuObjects[o.ID] = bo
	}
	for i, p := range cfg.Plates {
		index := metaInt(p.Meta, "plater_id")
		if index == 0 {
			index = i + 1
		}
		s.plates = append(s.plates, Plate{Index: index, Name: metaValue(p.Meta, "plater_name")})
		for _, inst := range p.Instances {
			key := instanceKey{metaValue(inst.Meta, "object_id"), metaInt(inst.Meta, "instance_id")}
			s.plateOf[key] = index
		}
	}
	sort.Slice(s.plates, func(i, j int) bool { return s.plates[i].Index < s.plates[j].Index })
}

func (s *slicerInfo) readPrusa(files map[string]*zip.File) {
	if b := readZipFile(files["metadata/slic3r_pe.config"], 16<<20); b != nil && len(s.filamentColors) == 0 {
		sc := bufio.NewScanner(bytes.NewReader(b))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if rest, ok := strings.CutPrefix(line, "; filament_colour = "); ok {
				for _, c := range strings.Split(rest, ";") {
					s.filamentColors = append(s.filamentColors, normColor(c))
				}
				break
			}
		}
	}

	b := readZipFile(files["metadata/slic3r_pe_model.config"], 64<<20)
	if b == nil {
		return
	}
	var cfg struct {
		Objects []struct {
			ID      string `xml:"id,attr"`
			Meta    []kv   `xml:"metadata"`
			Volumes []struct {
				First int  `xml:"firstid,attr"`
				Last  int  `xml:"lastid,attr"`
				Meta  []kv `xml:"metadata"`
			} `xml:"volume"`
		} `xml:"object"`
	}
	if xml.Unmarshal(b, &cfg) != nil {
		return
	}
	for _, o := range cfg.Objects {
		objExtruder := metaInt(o.Meta, "extruder")
		for _, v := range o.Volumes {
			extruder := metaInt(v.Meta, "extruder")
			if extruder == 0 {
				extruder = objExtruder
			}
			vt := metaValue(v.Meta, "volume_type")
			s.prusaVolumes[o.ID] = append(s.prusaVolumes[o.ID], prusaVolume{
				first:     v.First,
				last:      v.Last,
				name:      firstNonEmpty(metaValue(v.Meta, "name"), metaValue(o.Meta, "name")),
				extruder:  extruder,
				modelPart: vt == "" || vt == "ModelPart",
			})
		}
	}
}

// describe returns the display name and extruder of a mesh object, and whether it is
// a modifier or support volume that isn't printed (and so isn't shown).
func (s *slicerInfo) describe(root, objectID, objName string) (name string, extruder int, skip bool) {
	name = objName
	bo := s.bambuObjects[root]
	if bo == nil {
		return name, 0, false
	}
	if name == "" {
		name = bo.name
	}
	extruder = bo.extruder
	if bp, ok := bo.parts[objectID]; ok {
		if bp.subtype != "" && bp.subtype != "normal_part" {
			return "", 0, true
		}
		name = firstNonEmpty(bp.name, name)
		if bp.extruder > 0 {
			extruder = bp.extruder
		}
	}
	return name, extruder, false
}

// color is the filament colour of a 1-based extruder, or "" if unknown.
func (s *slicerInfo) color(extruder int) string {
	if len(s.filamentColors) == 0 {
		return ""
	}
	if extruder < 1 {
		extruder = 1
	}
	if extruder > len(s.filamentColors) {
		return ""
	}
	return s.filamentColors[extruder-1]
}
