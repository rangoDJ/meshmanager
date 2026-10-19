// Package gcode turns sliced G-code into a toolpath for the layer preview: extrusion
// and travel segments grouped by layer, tagged with the feature type the slicer wrote
// in its comments (walls, infill, support…), plus the print time and filament use the
// slicer recorded.
package gcode

import (
	"bufio"
	"errors"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// MaxSegments caps memory per file (~17 bytes per extrusion segment).
const MaxSegments = 8_000_000

var ErrTooLarge = errors.New("G-code has too many moves to preview")

// Feature types, in the order the viewer colours them.
var Features = []string{"Other", "Outer wall", "Inner wall", "Infill", "Solid infill", "Top/bottom", "Bridge", "Support", "Skirt/brim", "Overhang"}

const (
	featOther uint8 = iota
	featOuterWall
	featInnerWall
	featInfill
	featSolidInfill
	featTopBottom
	featBridge
	featSupport
	featSkirt
	featOverhang
)

// Layer is a run of segments printed at one height.
type Layer struct {
	Z              float64 `json:"z"`
	Height         float64 `json:"height"`
	ExtrusionStart int     `json:"extrusion_start"`
	ExtrusionCount int     `json:"extrusion_count"`
	TravelStart    int     `json:"travel_start"`
	TravelCount    int     `json:"travel_count"`
}

type Toolpath struct {
	Layers []Layer
	// Extrusions and Travels hold x1, y1, x2, y2 per segment, in mm; the layer gives Z.
	Extrusions []float32
	Travels    []float32
	// Feature is the feature type of each extrusion segment (index into Features).
	Feature []uint8

	Min, Max [3]float64 // bounds of the extrusions

	// What the slicer recorded, when it did.
	Slicer        string
	PrintSeconds  float64
	FilamentGrams float64
	FilamentMM    float64
}

func (t *Toolpath) segments() int { return len(t.Feature) + len(t.Travels)/4 }

type parser struct {
	tp        *Toolpath
	x, y, z   float64
	e         float64
	absXYZ    bool
	absE      bool
	inches    bool
	feature   uint8
	layerZ    float64
	haveLayer bool
	// Travels at a new height wait here and join the layer they lead into.
	pending []float32
	err     error
}

// Parse reads G-code. Unknown commands are ignored; only geometry and metadata matter.
func Parse(r io.Reader) (*Toolpath, error) {
	p := &parser{tp: &Toolpath{}, absXYZ: true, absE: true}
	for k := range 3 {
		p.tp.Min[k], p.tp.Max[k] = math.Inf(1), math.Inf(-1)
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		p.line(sc.Text())
		if p.err != nil {
			return nil, p.err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	p.closeLayer()
	tp := p.tp
	// Drop empty layers (travel-only Z hops) and compute heights.
	kept := tp.Layers[:0]
	for _, l := range tp.Layers {
		if l.ExtrusionCount > 0 {
			kept = append(kept, l)
		}
	}
	tp.Layers = kept
	prev := 0.0
	for i := range tp.Layers {
		tp.Layers[i].Height = tp.Layers[i].Z - prev
		prev = tp.Layers[i].Z
	}
	if len(tp.Feature) == 0 {
		return nil, errors.New("no extrusion moves found")
	}
	return tp, nil
}

func (p *parser) line(raw string) {
	code, comment, _ := strings.Cut(raw, ";")
	if comment != "" {
		p.comment(strings.TrimSpace(comment))
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return
	}
	fields := strings.Fields(strings.ToUpper(code))
	if strings.HasPrefix(fields[0], "N") && len(fields) > 1 { // line numbers
		fields = fields[1:]
	}
	switch fields[0] {
	case "G0", "G00", "G1", "G01":
		p.move(fields[1:], false, false)
	case "G2", "G02":
		p.move(fields[1:], true, true)
	case "G3", "G03":
		p.move(fields[1:], true, false)
	case "G90":
		p.absXYZ, p.absE = true, true
	case "G91":
		p.absXYZ, p.absE = false, false
	case "M82":
		p.absE = true
	case "M83":
		p.absE = false
	case "G20":
		p.inches = true
	case "G21":
		p.inches = false
	case "G92":
		for _, f := range fields[1:] {
			if v, ok := num(f, 'E'); ok {
				p.e = v
			}
		}
	}
}

func num(field string, axis byte) (float64, bool) {
	if len(field) < 2 || field[0] != axis {
		return 0, false
	}
	v, err := strconv.ParseFloat(field[1:], 64)
	return v, err == nil
}

func (p *parser) move(args []string, arc, clockwise bool) {
	nx, ny, nz := p.x, p.y, p.z
	var e, i, j float64
	hasE := false
	scale := 1.0
	if p.inches {
		scale = 25.4
	}
	for _, f := range args {
		if f == "" {
			continue
		}
		v, err := strconv.ParseFloat(f[1:], 64)
		if err != nil {
			continue
		}
		switch f[0] {
		case 'X':
			if p.absXYZ {
				nx = v * scale
			} else {
				nx += v * scale
			}
		case 'Y':
			if p.absXYZ {
				ny = v * scale
			} else {
				ny += v * scale
			}
		case 'Z':
			if p.absXYZ {
				nz = v * scale
			} else {
				nz += v * scale
			}
		case 'E':
			e, hasE = v, true
		case 'I':
			i = v * scale
		case 'J':
			j = v * scale
		}
	}

	extruding := false
	if hasE {
		delta := e
		if p.absE {
			delta = e - p.e
			p.e = e
		}
		extruding = delta > 1e-9
	}
	if nz != p.z {
		p.z = nz
	}
	moved := nx != p.x || ny != p.y
	if !moved {
		p.x, p.y = nx, ny
		return
	}
	if extruding {
		p.ensureLayer()
	}

	add := func(x1, y1, x2, y2 float64) {
		if p.tp.segments() >= MaxSegments {
			p.err = ErrTooLarge
			return
		}
		seg := []float32{float32(x1), float32(y1), float32(x2), float32(y2)}
		if extruding {
			p.tp.Extrusions = append(p.tp.Extrusions, seg...)
			p.tp.Feature = append(p.tp.Feature, p.feature)
			for _, pt := range [2][2]float64{{x1, y1}, {x2, y2}} {
				p.bound(pt[0], pt[1], p.z)
			}
		} else if p.haveLayer && math.Abs(p.z-p.layerZ) < 1e-6 {
			p.tp.Travels = append(p.tp.Travels, seg...)
		} else if p.haveLayer {
			p.pending = append(p.pending, seg...)
		}
		// Travels before the first layer (homing, purge moves) aren't drawn.
	}

	if arc {
		p.arc(nx, ny, i, j, clockwise, add)
	} else {
		add(p.x, p.y, nx, ny)
	}
	p.x, p.y = nx, ny
}

// arc approximates G2/G3 (centre given by I/J offsets) with short segments.
func (p *parser) arc(nx, ny, i, j float64, clockwise bool, add func(x1, y1, x2, y2 float64)) {
	cx, cy := p.x+i, p.y+j
	r := math.Hypot(i, j)
	a0 := math.Atan2(p.y-cy, p.x-cx)
	a1 := math.Atan2(ny-cy, nx-cx)
	sweep := a1 - a0
	if clockwise && sweep >= 0 {
		sweep -= 2 * math.Pi
	} else if !clockwise && sweep <= 0 {
		sweep += 2 * math.Pi
	}
	steps := max(2, min(128, int(math.Abs(sweep)*r/0.5)+1)) // ~0.5 mm chords
	px, py := p.x, p.y
	for s := 1; s <= steps; s++ {
		a := a0 + sweep*float64(s)/float64(steps)
		x, y := cx+r*math.Cos(a), cy+r*math.Sin(a)
		if s == steps {
			x, y = nx, ny
		}
		add(px, py, x, y)
		px, py = x, y
	}
}

func (p *parser) bound(x, y, z float64) {
	for k, v := range [3]float64{x, y, z} {
		p.tp.Min[k] = math.Min(p.tp.Min[k], v)
		p.tp.Max[k] = math.Max(p.tp.Max[k], v)
	}
}

// ensureLayer starts a new layer when extrusion happens at a new height.
func (p *parser) ensureLayer() {
	if p.haveLayer && math.Abs(p.z-p.layerZ) < 1e-6 {
		return
	}
	p.closeLayer()
	p.tp.Layers = append(p.tp.Layers, Layer{
		Z:              p.z,
		ExtrusionStart: len(p.tp.Feature),
		TravelStart:    len(p.tp.Travels) / 4,
	})
	p.tp.Travels = append(p.tp.Travels, p.pending...)
	p.pending = p.pending[:0]
	p.layerZ, p.haveLayer = p.z, true
}

func (p *parser) closeLayer() {
	if n := len(p.tp.Layers); n > 0 {
		l := &p.tp.Layers[n-1]
		l.ExtrusionCount = len(p.tp.Feature) - l.ExtrusionStart
		l.TravelCount = len(p.tp.Travels)/4 - l.TravelStart
	}
}

var (
	durationRe = regexp.MustCompile(`(?:(\d+)d\s*)?(?:(\d+)h\s*)?(?:(\d+)m\s*)?(?:(\d+)s)?`)
	numberRe   = regexp.MustCompile(`[-+]?\d*\.?\d+`)
)

// comment picks up feature types and the slicer's estimates.
func (p *parser) comment(c string) {
	lc := strings.ToLower(c)
	switch {
	case strings.HasPrefix(lc, "type:"):
		p.feature = featureOf(lc[5:])
		return
	case strings.HasPrefix(lc, "feature:"):
		p.feature = featureOf(lc[8:])
		return
	case strings.HasPrefix(lc, "generated by "):
		if p.tp.Slicer == "" {
			p.tp.Slicer = strings.TrimSpace(c[len("generated by "):])
		}
	case strings.HasPrefix(lc, "flavor:") || strings.HasPrefix(lc, "generated with cura"):
		if p.tp.Slicer == "" {
			p.tp.Slicer = "Cura"
		}
	}

	// Print time.
	switch {
	case strings.HasPrefix(lc, "time:"): // Cura: ;TIME:3723
		if v, err := strconv.ParseFloat(strings.TrimSpace(c[5:]), 64); err == nil && p.tp.PrintSeconds == 0 {
			p.tp.PrintSeconds = v
		}
	case strings.HasPrefix(lc, "estimated printing time (normal mode)"),
		strings.Contains(lc, "total estimated time:"):
		text := lc
		if k := strings.LastIndex(text, "total estimated time:"); k >= 0 {
			text = text[k+len("total estimated time:"):]
		} else if k := strings.Index(text, "="); k >= 0 {
			text = text[k+1:]
		}
		if s := parseDuration(text); s > 0 {
			p.tp.PrintSeconds = s
		}
	}

	// Filament.
	switch {
	case strings.HasPrefix(lc, "filament used [g]"), strings.HasPrefix(lc, "total filament weight [g]"):
		if v := firstNumber(lc); v > 0 && p.tp.FilamentGrams == 0 {
			p.tp.FilamentGrams = v
		}
	case strings.HasPrefix(lc, "filament used [mm]"), strings.HasPrefix(lc, "total filament length [mm]"):
		if v := firstNumber(lc); v > 0 && p.tp.FilamentMM == 0 {
			p.tp.FilamentMM = v
		}
	case strings.HasPrefix(lc, "filament used:"): // Cura: ;Filament used: 1.234m
		if v := firstNumber(lc); v > 0 && p.tp.FilamentMM == 0 {
			p.tp.FilamentMM = v * 1000
		}
	}
}

func firstNumber(s string) float64 {
	_, rest, ok := strings.Cut(s, "]")
	if !ok {
		rest = s
	}
	m := numberRe.FindString(rest)
	v, _ := strconv.ParseFloat(m, 64)
	return v
}

func parseDuration(s string) float64 {
	m := durationRe.FindStringSubmatch(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if m == nil {
		return 0
	}
	total := 0.0
	for k, mult := range []float64{86400, 3600, 60, 1} {
		if m[k+1] != "" {
			v, _ := strconv.ParseFloat(m[k+1], 64)
			total += v * mult
		}
	}
	return total
}

// featureOf maps the feature names PrusaSlicer, Bambu Studio, OrcaSlicer and Cura write.
func featureOf(name string) uint8 {
	n := strings.TrimSpace(name)
	switch {
	case strings.Contains(n, "skirt"), strings.Contains(n, "brim"):
		return featSkirt
	case strings.Contains(n, "support"):
		return featSupport
	case strings.Contains(n, "bridge"):
		return featBridge
	case strings.Contains(n, "overhang"):
		return featOverhang
	case strings.Contains(n, "external perimeter"), strings.Contains(n, "outer wall"), n == "wall-outer":
		return featOuterWall
	case strings.Contains(n, "perimeter"), strings.Contains(n, "inner wall"), n == "wall-inner":
		return featInnerWall
	case strings.Contains(n, "top"), strings.Contains(n, "bottom"), n == "skin", strings.Contains(n, "ironing"):
		return featTopBottom
	case strings.Contains(n, "solid infill"), strings.Contains(n, "internal solid"):
		return featSolidInfill
	case strings.Contains(n, "infill"), n == "fill":
		return featInfill
	}
	return featOther
}
