package mesh

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
)

// ReadOBJ reads Wavefront OBJ geometry (v and f lines). Polygons are fan-triangulated,
// and the model is treated as Y-up.
func ReadOBJ(r io.Reader) (*Mesh, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16<<20)
	var verts [][3]float32
	var idx []int
	m := &Mesh{}

	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) < 2 || (line[1] != ' ' && line[1] != '\t') {
			continue
		}
		switch line[0] {
		case 'v':
			f := bytes.Fields(line[2:])
			if len(f) < 3 {
				continue
			}
			var v [3]float32
			for k := range 3 {
				x, err := strconv.ParseFloat(string(f[k]), 32)
				if err != nil {
					return nil, err
				}
				v[k] = float32(x)
			}
			verts = append(verts, v)
		case 'f':
			idx = idx[:0]
			valid := true
			for _, tok := range bytes.Fields(line[2:]) {
				if slash := bytes.IndexByte(tok, '/'); slash >= 0 {
					tok = tok[:slash]
				}
				n, err := strconv.Atoi(string(tok))
				if err != nil || n == 0 {
					valid = false
					break
				}
				if n < 0 {
					n += len(verts)
				} else {
					n--
				}
				if n < 0 || n >= len(verts) {
					valid = false
					break
				}
				idx = append(idx, n)
			}
			if !valid {
				continue
			}
			for k := 1; k+1 < len(idx); k++ {
				if err := m.addTri(verts[idx[0]], verts[idx[k]], verts[idx[k+1]]); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	m.yUpToZUp()
	return m, nil
}
