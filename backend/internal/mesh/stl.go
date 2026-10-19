package mesh

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
)

// ReadSTL reads binary or ASCII STL. size is the file size, used to tell the two
// apart, since many binary STLs also start with "solid".
func ReadSTL(r io.Reader, size int64) (*Mesh, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	head, _ := br.Peek(84)
	if len(head) == 84 {
		n := int64(binary.LittleEndian.Uint32(head[80:84]))
		if size == 84+50*n {
			return readBinarySTL(br, n)
		}
	}
	if bytes.HasPrefix(bytes.TrimLeft(head, " \t\r\n"), []byte("solid")) {
		return readASCIISTL(br)
	}
	if len(head) == 84 {
		// Binary with trailing bytes after the last triangle.
		if n := int64(binary.LittleEndian.Uint32(head[80:84])); size > 84+50*n {
			return readBinarySTL(br, n)
		}
	}
	return nil, errors.New("not a valid STL file")
}

func readBinarySTL(r *bufio.Reader, n int64) (*Mesh, error) {
	if n > MaxTriangles {
		return nil, ErrTooLarge
	}
	if _, err := r.Discard(84); err != nil {
		return nil, err
	}
	m := &Mesh{Tris: make([]float32, 0, n*9)}
	var buf [50]byte
	for range n {
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, fmt.Errorf("truncated binary STL: %w", err)
		}
		for k := range 9 {
			bits := binary.LittleEndian.Uint32(buf[12+4*k:])
			m.Tris = append(m.Tris, math.Float32frombits(bits))
		}
	}
	return m, nil
}

func readASCIISTL(r io.Reader) (*Mesh, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	m := &Mesh{}
	var tri [3][3]float32
	n := 0
	for sc.Scan() {
		f := bytes.Fields(sc.Bytes())
		if len(f) != 4 || !bytes.Equal(f[0], []byte("vertex")) {
			continue
		}
		for k := range 3 {
			v, err := strconv.ParseFloat(string(f[k+1]), 32)
			if err != nil {
				return nil, fmt.Errorf("bad vertex: %w", err)
			}
			tri[n][k] = float32(v)
		}
		if n++; n == 3 {
			if err := m.addTri(tri[0], tri[1], tri[2]); err != nil {
				return nil, err
			}
			n = 0
		}
	}
	return m, sc.Err()
}
