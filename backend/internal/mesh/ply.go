package mesh

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

type plyProp struct {
	name      string
	typ       string
	list      bool
	countType string
}

type plyElem struct {
	name  string
	count int
	props []plyProp
}

var plySizes = map[string]int{
	"char": 1, "int8": 1, "uchar": 1, "uint8": 1,
	"short": 2, "int16": 2, "ushort": 2, "uint16": 2,
	"int": 4, "int32": 4, "uint": 4, "uint32": 4,
	"float": 4, "float32": 4, "double": 8, "float64": 8,
}

// ReadPLY reads ASCII and binary (little or big endian) PLY files.
func ReadPLY(r io.Reader) (*Mesh, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	format, elems, err := readPLYHeader(br)
	if err != nil {
		return nil, err
	}

	var vals plyValues
	switch format {
	case "ascii":
		vals = &plyASCII{r: br}
	case "binary_little_endian":
		vals = &plyBinary{r: br, order: binary.LittleEndian}
	case "binary_big_endian":
		vals = &plyBinary{r: br, order: binary.BigEndian}
	default:
		return nil, fmt.Errorf("unknown PLY format %q", format)
	}

	var verts [][3]float32
	var faces [][]int
	for _, el := range elems {
		xyz := [3]int{-1, -1, -1}
		faceProp := -1
		for i, p := range el.props {
			switch {
			case p.name == "x":
				xyz[0] = i
			case p.name == "y":
				xyz[1] = i
			case p.name == "z":
				xyz[2] = i
			case p.list && (p.name == "vertex_indices" || p.name == "vertex_index"):
				faceProp = i
			}
		}
		isVertex := el.name == "vertex" && xyz[0] >= 0 && xyz[1] >= 0 && xyz[2] >= 0
		isFace := el.name == "face" && faceProp >= 0
		if isVertex && el.count > MaxTriangles*3 {
			return nil, ErrTooLarge
		}

		for range el.count {
			var v [3]float32
			for i, p := range el.props {
				if p.list {
					n, err := vals.next(p.countType)
					if err != nil {
						return nil, err
					}
					list := make([]int, int(n))
					for k := range list {
						x, err := vals.next(p.typ)
						if err != nil {
							return nil, err
						}
						list[k] = int(x)
					}
					if isFace && i == faceProp {
						faces = append(faces, list)
					}
					continue
				}
				x, err := vals.next(p.typ)
				if err != nil {
					return nil, err
				}
				if isVertex {
					for k := range 3 {
						if xyz[k] == i {
							v[k] = float32(x)
						}
					}
				}
			}
			if isVertex {
				verts = append(verts, v)
			}
		}
	}

	m := &Mesh{}
	for _, f := range faces {
		for k := 1; k+1 < len(f); k++ {
			a, b, c := f[0], f[k], f[k+1]
			if a < 0 || b < 0 || c < 0 || a >= len(verts) || b >= len(verts) || c >= len(verts) {
				continue
			}
			if err := m.addTri(verts[a], verts[b], verts[c]); err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

func readPLYHeader(r *bufio.Reader) (string, []plyElem, error) {
	line, err := r.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ply" {
		return "", nil, errors.New("not a PLY file")
	}
	var format string
	var elems []plyElem
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", nil, fmt.Errorf("PLY header: %w", err)
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "format":
			if len(f) < 2 {
				return "", nil, errors.New("PLY header: bad format line")
			}
			format = f[1]
		case "element":
			if len(f) < 3 {
				return "", nil, errors.New("PLY header: bad element line")
			}
			n, err := strconv.Atoi(f[2])
			if err != nil || n < 0 {
				return "", nil, fmt.Errorf("PLY header: bad element count %q", f[2])
			}
			elems = append(elems, plyElem{name: f[1], count: n})
		case "property":
			if len(elems) == 0 {
				return "", nil, errors.New("PLY header: property before element")
			}
			el := &elems[len(elems)-1]
			if len(f) >= 5 && f[1] == "list" {
				if plySizes[f[2]] == 0 || plySizes[f[3]] == 0 {
					return "", nil, fmt.Errorf("PLY header: unknown type in %q", strings.TrimSpace(line))
				}
				el.props = append(el.props, plyProp{name: f[4], typ: f[3], list: true, countType: f[2]})
			} else if len(f) >= 3 {
				if plySizes[f[1]] == 0 {
					return "", nil, fmt.Errorf("PLY header: unknown type %q", f[1])
				}
				el.props = append(el.props, plyProp{name: f[2], typ: f[1]})
			}
		case "end_header":
			return format, elems, nil
		}
	}
}

type plyValues interface {
	next(typ string) (float64, error)
}

type plyASCII struct {
	r      *bufio.Reader
	fields []string
}

func (p *plyASCII) next(string) (float64, error) {
	for len(p.fields) == 0 {
		line, err := p.r.ReadString('\n')
		if err != nil && line == "" {
			return 0, fmt.Errorf("PLY data: %w", io.ErrUnexpectedEOF)
		}
		p.fields = strings.Fields(line)
	}
	tok := p.fields[0]
	p.fields = p.fields[1:]
	return strconv.ParseFloat(tok, 64)
}

type plyBinary struct {
	r     *bufio.Reader
	order binary.ByteOrder
	buf   [8]byte
}

func (p *plyBinary) next(typ string) (float64, error) {
	n := plySizes[typ]
	if _, err := io.ReadFull(p.r, p.buf[:n]); err != nil {
		return 0, fmt.Errorf("PLY data: %w", err)
	}
	b := p.buf[:n]
	switch typ {
	case "char", "int8":
		return float64(int8(b[0])), nil
	case "uchar", "uint8":
		return float64(b[0]), nil
	case "short", "int16":
		return float64(int16(p.order.Uint16(b))), nil
	case "ushort", "uint16":
		return float64(p.order.Uint16(b)), nil
	case "int", "int32":
		return float64(int32(p.order.Uint32(b))), nil
	case "uint", "uint32":
		return float64(p.order.Uint32(b)), nil
	case "float", "float32":
		return float64(math.Float32frombits(p.order.Uint32(b))), nil
	default: // double
		return math.Float64frombits(p.order.Uint64(b)), nil
	}
}
