package api_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rangoDJ/meshmanager/internal/testutil"
)

func tgzBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write(data)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

// RAR, 7Z and TGZ archives browse like folders; later parts of a split RAR are not
// opened on their own.
func TestOtherArchiveFormats(t *testing.T) {
	cfg := testConfig(t)
	base, st := newServerWithConfig(t, cfg)
	c := newClient(t, base)
	setupAdmin(t, c)
	startWorker(t, st, cfg)

	lib := filepath.Join(cfg.LibraryRoot, "lib")
	writeLibFile(t, lib, "Packs/dragon.rar", []byte(testutil.RAR5Stored(map[string]string{
		"Dragon/dragon.stl": string(cubeSTL(10)),
		"readme.txt":        "from a rar",
	})))
	writeLibFile(t, lib, "Packs/split.part1.rar", []byte(testutil.RAR5Stored(map[string]string{"part.stl": string(cubeSTL(4))})))
	writeLibFile(t, lib, "Packs/split.part2.rar", []byte("not a real volume; must not be opened"))
	writeLibFile(t, lib, "Packs/bundle.tar.gz", tgzBytes(t, map[string][]byte{"Boat/boat.obj": []byte(cubeOBJ)}))
	sevenZip, err := os.ReadFile("../library/testdata/pack.7z")
	if err != nil {
		t.Fatal(err)
	}
	writeLibFile(t, lib, "Packs/cube.7z", sevenZip)

	_, src := c.do("POST", "/api/sources", map[string]any{"path": "lib"})
	done := waitForScan(t, c, src["id"])
	if msg := fmt.Sprint(done["last_scan"].(map[string]any)["message"]); strings.Contains(msg, "unreadable") {
		t.Errorf("scan reported unreadable files: %s", msg)
	}

	models, _ := listFiles(t, c, "kind=model&sort=name")
	paths := map[string]map[string]any{}
	for _, m := range models {
		paths[m["rel_path"].(string)] = m
	}
	for _, want := range []string{
		"Packs/dragon.rar/Dragon/dragon.stl",
		"Packs/split.part1.rar/part.stl",
		"Packs/bundle.tar.gz/Boat/boat.obj",
		"Packs/cube.7z/Pack/cube.stl",
	} {
		m := paths[want]
		if m == nil {
			t.Errorf("missing %s (have %d models)", want, len(models))
			continue
		}
		if h, _ := m["content_hash"].(string); len(h) != 64 {
			t.Errorf("%s not hashed", want)
		}
	}
	if len(models) != 4 {
		t.Errorf("models: %d, want 4", len(models))
	}

	// Content reads straight from each archive type.
	for _, want := range []string{"Packs/dragon.rar/Dragon/dragon.stl", "Packs/cube.7z/Pack/cube.stl", "Packs/bundle.tar.gz/Boat/boat.obj"} {
		m := paths[want]
		if m == nil {
			continue
		}
		status, _, body := c.raw(fmt.Sprintf("/api/files/%v/geometry", m["id"]))
		if status != 200 || parseGeometry(t, body).Header.Triangles != 12 {
			t.Errorf("%s geometry: %d", want, status)
		}
	}
	docs, _ := listFiles(t, c, "kind=document")
	if len(docs) != 2 { // readme.txt from the rar, notes.txt from the 7z
		t.Errorf("documents: %s", names(docs))
	}
	for _, d := range docs {
		if d["name"] == "readme.txt" {
			status, _, body := c.raw(fmt.Sprintf("/api/files/%v/download", d["id"]))
			if status != 200 || string(body) != "from a rar" {
				t.Errorf("rar member download: %d %q", status, body)
			}
		}
	}
}
