package library

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"
)

// ArchiveFormats are the archive formats whose contents are indexed.
var ArchiveFormats = []string{"zip", "rar", "7z", "tar", "tgz"}

// Member is one regular file inside an archive.
type Member struct {
	Name string // path inside the archive, exactly as stored
	Size int64
	Time int64 // modification time, Unix microseconds
}

// maxMembers bounds how many files one archive may contribute to the index.
const maxMembers = 20000

var rarPart = regexp.MustCompile(`(?i)\.part0*(\d+)\.rar$`)

// RARPart returns the volume number of a "name.partN.rar" file, or 0 when the name has
// no part number. Only part 1 of a split set is opened; it reads the others itself.
func RARPart(name string) int {
	m := rarPart.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// junk reports archive entries that are never worth indexing (macOS metadata).
func junk(name string) bool {
	clean := strings.ReplaceAll(name, "\\", "/")
	base := path.Base(clean)
	return strings.HasPrefix(clean, "__MACOSX/") || strings.HasPrefix(base, "._") || base == ".DS_Store"
}

// ListArchive returns the regular files inside an archive of the given format.
func ListArchive(diskPath, format string) ([]Member, error) {
	var out []Member
	add := func(name string, size int64, micros int64) bool {
		if junk(name) {
			return true
		}
		out = append(out, Member{Name: name, Size: size, Time: micros})
		return len(out) < maxMembers
	}

	switch format {
	case "zip":
		zr, err := zip.OpenReader(diskPath)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if f.FileInfo().IsDir() || !f.Mode().IsRegular() {
				continue
			}
			if !add(f.Name, int64(f.UncompressedSize64), f.Modified.UnixMicro()) {
				break
			}
		}

	case "7z":
		r, err := sevenzip.OpenReader(diskPath)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		for _, f := range r.File {
			if f.FileInfo().IsDir() || !f.Mode().IsRegular() {
				continue
			}
			if !add(f.Name, int64(f.UncompressedSize), f.Modified.UnixMicro()) {
				break
			}
		}

	case "rar":
		r, err := rardecode.OpenReader(diskPath)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		for {
			h, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if h.IsDir || !h.Mode().IsRegular() {
				continue
			}
			if !add(h.Name, h.UnPackedSize, h.ModificationTime.UnixMicro()) {
				break
			}
		}

	case "tar", "tgz":
		f, tr, err := openTar(diskPath, format)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if h.Typeflag != tar.TypeReg {
				continue
			}
			if !add(h.Name, h.Size, h.ModTime.UnixMicro()) {
				break
			}
		}

	default:
		return nil, fmt.Errorf("archive format %q is not supported", format)
	}
	return out, nil
}

// openTar opens a plain or gzipped tar. The returned closer closes everything.
func openTar(diskPath, format string) (io.Closer, *tar.Reader, error) {
	f, err := os.Open(diskPath)
	if err != nil {
		return nil, nil, err
	}
	if format != "tgz" {
		return f, tar.NewReader(f), nil
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return &multiCloser{closers: []io.Closer{gz, f}}, tar.NewReader(gz), nil
}

// openMember returns one member's content and size. Formats without an index (tar,
// rar) and solid 7z archives are read from the start until the member is reached.
func openMember(diskPath, format, member string) (io.ReadCloser, int64, error) {
	tooBig := func(n int64) error {
		return fmt.Errorf("archive member is too large (%d bytes)", n)
	}

	switch format {
	case "zip":
		zr, err := zip.OpenReader(diskPath)
		if err != nil {
			return nil, 0, fmt.Errorf("open archive: %w", err)
		}
		for _, f := range zr.File {
			if f.Name != member {
				continue
			}
			if f.UncompressedSize64 > maxMemberSize {
				zr.Close()
				return nil, 0, tooBig(int64(f.UncompressedSize64))
			}
			rc, err := f.Open()
			if err != nil {
				zr.Close()
				return nil, 0, err
			}
			return &multiCloser{Reader: rc, closers: []io.Closer{rc, zr}}, int64(f.UncompressedSize64), nil
		}
		zr.Close()

	case "7z":
		r, err := sevenzip.OpenReader(diskPath)
		if err != nil {
			return nil, 0, fmt.Errorf("open archive: %w", err)
		}
		for _, f := range r.File {
			if f.Name != member {
				continue
			}
			if f.UncompressedSize > maxMemberSize {
				r.Close()
				return nil, 0, tooBig(int64(f.UncompressedSize))
			}
			rc, err := f.Open()
			if err != nil {
				r.Close()
				return nil, 0, err
			}
			return &multiCloser{Reader: rc, closers: []io.Closer{rc, r}}, int64(f.UncompressedSize), nil
		}
		r.Close()

	case "rar":
		r, err := rardecode.OpenReader(diskPath)
		if err != nil {
			return nil, 0, fmt.Errorf("open archive: %w", err)
		}
		for {
			h, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				r.Close()
				return nil, 0, err
			}
			if h.Name != member || h.IsDir {
				continue
			}
			if h.UnPackedSize > maxMemberSize {
				r.Close()
				return nil, 0, tooBig(h.UnPackedSize)
			}
			return &multiCloser{Reader: r, closers: []io.Closer{r}}, h.UnPackedSize, nil
		}
		r.Close()

	case "tar", "tgz":
		c, tr, err := openTar(diskPath, format)
		if err != nil {
			return nil, 0, fmt.Errorf("open archive: %w", err)
		}
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				c.Close()
				return nil, 0, err
			}
			if h.Name != member || h.Typeflag != tar.TypeReg {
				continue
			}
			if h.Size > maxMemberSize {
				c.Close()
				return nil, 0, tooBig(h.Size)
			}
			return &multiCloser{Reader: tr, closers: []io.Closer{c}}, h.Size, nil
		}
		c.Close()

	default:
		return nil, 0, fmt.Errorf("archive format %q is not supported", format)
	}
	return nil, 0, ErrMemberNotFound
}
