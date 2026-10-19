package library

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rangoDJ/meshmanager/internal/testutil"
)

// Files every generated archive holds (macOS junk must be skipped).
var fixture = map[string]string{
	"Pack/dragon.stl":        "solid dragon\nendsolid dragon\n",
	"Pack/supports/base.obj": "v 0 0 0\n",
	"readme.txt":             "print at 0.2 mm",
	"__MACOSX/Pack/._dragon": "junk",
}

func sortedNames(fixture map[string]string) []string {
	var names []string
	for n := range fixture {
		if !junk(n) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func zipArchive(t *testing.T) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range fixture {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	return buf.Bytes()
}

func tarArchive(t *testing.T, gz bool) []byte {
	var buf bytes.Buffer
	var w io.Writer = &buf
	var gzw *gzip.Writer
	if gz {
		gzw = gzip.NewWriter(&buf)
		w = gzw
	}
	tw := tar.NewWriter(w)
	_ = tw.WriteHeader(&tar.Header{Name: "Pack/", Typeflag: tar.TypeDir, Mode: 0o755})
	for name, body := range fixture {
		_ = tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body)), ModTime: time.Unix(1700000000, 0)})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	if gzw != nil {
		_ = gzw.Close()
	}
	return buf.Bytes()
}

func TestArchiveFormats(t *testing.T) {
	cases := []struct {
		format string
		path   string
	}{
		{"zip", writeTemp(t, "pack.zip", zipArchive(t))},
		{"tar", writeTemp(t, "pack.tar", tarArchive(t, false))},
		{"tgz", writeTemp(t, "pack.tar.gz", tarArchive(t, true))},
		{"rar", writeTemp(t, "pack.rar", testutil.RAR5Stored(fixture))},
	}
	for _, c := range cases {
		t.Run(c.format, func(t *testing.T) {
			members, err := ListArchive(c.path, c.format)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range members {
				got = append(got, m.Name)
				if int(m.Size) != len(fixture[m.Name]) {
					t.Errorf("%s: size %d, want %d", m.Name, m.Size, len(fixture[m.Name]))
				}
			}
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(sortedNames(fixture), ",") {
				t.Fatalf("members: %v", got)
			}

			loc := Location{SourcePath: filepath.Dir(c.path), RelPath: "x/readme.txt",
				ArchiveRelPath: filepath.Base(c.path), ArchiveFormat: c.format, MemberPath: "readme.txt"}
			rc, size, err := Open(loc)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(rc)
			rc.Close()
			if string(body) != fixture["readme.txt"] || int(size) != len(body) {
				t.Errorf("readme: %q (size %d)", body, size)
			}

			loc.MemberPath = "nope.txt"
			if _, _, err := Open(loc); err != ErrMemberNotFound {
				t.Errorf("missing member: %v", err)
			}
		})
	}
}

// pack.7z was made with 7-Zip: Pack/cube.stl (684 bytes) and notes.txt.
func TestSevenZip(t *testing.T) {
	members, err := ListArchive("testdata/pack.7z", "7z")
	if err != nil {
		t.Fatal(err)
	}
	sizes := map[string]int64{}
	for _, m := range members {
		sizes[strings.ReplaceAll(m.Name, "\\", "/")] = m.Size
	}
	if sizes["Pack/cube.stl"] != 684 || sizes["notes.txt"] != 16 || len(sizes) != 2 {
		t.Fatalf("members: %v", sizes)
	}

	abs, _ := filepath.Abs("testdata")
	loc := Location{SourcePath: abs, ArchiveRelPath: "pack.7z", ArchiveFormat: "7z", MemberPath: "notes.txt"}
	rc, _, err := Open(loc)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if string(body) != "7z fixture notes" {
		t.Errorf("notes: %q", body)
	}

	path, cleanup, err := Materialize(loc, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "7z fixture notes" || filepath.Ext(path) != ".txt" {
		t.Errorf("materialized %s: %q", path, b)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("cleanup left the temporary file")
	}
}

func TestRARPart(t *testing.T) {
	for name, want := range map[string]int{
		"pack.rar":            0,
		"pack.part1.rar":      1,
		"pack.part01.rar":     1,
		"Pack.Part2.RAR":      2,
		"pack.part10.rar":     10,
		"partial.rar":         0,
		"dragon.part3.rar.7z": 0,
	} {
		if got := RARPart(name); got != want {
			t.Errorf("RARPart(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestUnsupportedArchive(t *testing.T) {
	if _, err := ListArchive(writeTemp(t, "x.bin", []byte("x")), "lzh"); err == nil {
		t.Error("expected an error for an unsupported format")
	}
}
