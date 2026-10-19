// Package testutil holds helpers shared by tests.
package testutil

import (
	"encoding/binary"
	"hash/crc32"
	"sort"
)

// RAR5Stored writes a minimal RAR5 archive with stored (uncompressed) files, following
// the RAR 5.0 technical note, since Go has no RAR writer.
func RAR5Stored(files map[string]string) []byte {
	vint := func(v uint64) []byte {
		var b []byte
		for {
			c := byte(v & 0x7f)
			v >>= 7
			if v != 0 {
				c |= 0x80
			}
			b = append(b, c)
			if v == 0 {
				return b
			}
		}
	}
	block := func(body []byte) []byte {
		// CRC32 | header size | header (type, flags, ...)
		size := vint(uint64(len(body)))
		crc := crc32.ChecksumIEEE(append(append([]byte{}, size...), body...))
		out := binary.LittleEndian.AppendUint32(nil, crc)
		return append(append(out, size...), body...)
	}

	out := []byte("Rar!\x1a\x07\x01\x00")
	main := append(append(vint(1), vint(0)...), vint(0)...) // type main, flags, archive flags
	out = append(out, block(main)...)

	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		data := []byte(files[name])
		var h []byte
		h = append(h, vint(2)...)                 // type: file
		h = append(h, vint(0x02)...)              // flags: data area present
		h = append(h, vint(uint64(len(data)))...) // data size
		h = append(h, vint(0x04)...)              // file flags: CRC32 present
		h = append(h, vint(uint64(len(data)))...) // unpacked size
		h = append(h, vint(0x20)...)              // attributes
		h = binary.LittleEndian.AppendUint32(h, crc32.ChecksumIEEE(data))
		h = append(h, vint(0)...) // compression: version 0, stored
		h = append(h, vint(1)...) // host OS: Unix
		h = append(h, vint(uint64(len(name)))...)
		h = append(h, name...)
		out = append(out, block(h)...)
		out = append(out, data...)
	}
	end := append(append(vint(5), vint(0)...), vint(0)...) // end of archive
	return append(out, block(end)...)
}
