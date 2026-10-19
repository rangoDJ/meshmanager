// Package library opens indexed files wherever their bytes live: on disk, or inside
// an archive (ZIP, RAR, 7Z, TAR). It only ever reads from the library.
package library

import (
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/rangoDJ/meshmanager/internal/store"
)

// maxMemberSize bounds how much a single archive member may expand to.
const maxMemberSize = 4 << 30

// ErrMemberNotFound means the archive no longer contains the member.
var ErrMemberNotFound = errors.New("file not found inside the archive")

type Location struct {
	SourcePath     string
	RelPath        string // the file's own path; for members, a virtual path through the archive
	ArchiveRelPath string // set for archive members
	ArchiveFormat  string // zip, rar, 7z, tar or tgz, for archive members
	MemberPath     string // the member's name inside the archive
}

func FromStore(l store.FileLocation) Location {
	return Location{
		SourcePath:     l.SourcePath,
		RelPath:        l.RelPath,
		ArchiveRelPath: l.ArchiveRelPath,
		ArchiveFormat:  l.ArchiveFormat,
		MemberPath:     l.MemberPath,
	}
}

func (l Location) IsMember() bool { return l.MemberPath != "" }

// DiskPath is the file on disk: the file itself, or the archive that contains it.
func (l Location) DiskPath() string {
	rel := l.RelPath
	if l.IsMember() {
		rel = l.ArchiveRelPath
	}
	return filepath.Join(l.SourcePath, filepath.FromSlash(rel))
}

type multiCloser struct {
	io.Reader
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	var first error
	for _, c := range m.closers {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Open returns the file's content and uncompressed size.
func Open(l Location) (io.ReadCloser, int64, error) {
	if !l.IsMember() {
		f, err := os.Open(l.DiskPath())
		if err != nil {
			return nil, 0, err
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, 0, err
		}
		return f, st.Size(), nil
	}
	rc, size, err := openMember(l.DiskPath(), l.ArchiveFormat, l.MemberPath)
	if err != nil {
		return nil, 0, err
	}
	return &multiCloser{Reader: io.LimitReader(rc, maxMemberSize), closers: []io.Closer{rc}}, size, nil
}

// Materialize returns a path on disk holding the file's content, for loaders that
// need a real file. Archive members are extracted to a temporary file in tmpDir;
// cleanup removes it. For plain files the path is the file itself.
func Materialize(l Location, tmpDir string) (string, func(), error) {
	if !l.IsMember() {
		return l.DiskPath(), func() {}, nil
	}
	rc, _, err := Open(l)
	if err != nil {
		return "", nil, err
	}
	defer rc.Close()
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", nil, err
	}
	// Keep the extension: some loaders (glTF) look at it.
	ext := strings.ToLower(path.Ext(l.MemberPath))
	tmp, err := os.CreateTemp(tmpDir, "member-*"+ext)
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.Remove(tmp.Name()) }
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		cleanup()
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return tmp.Name(), cleanup, nil
}

// TempDir is where extracted members go under DATA_DIR.
func TempDir(dataDir string) string { return filepath.Join(dataDir, "tmp") }
