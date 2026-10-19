package api_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func zipBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(2025, 1, 2, 3, 4, 6, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestArchiveBrowsing(t *testing.T) {
	cfg := testConfig(t)
	base, st := newServerWithConfig(t, cfg)
	c := newClient(t, base)
	setupAdmin(t, c)
	startWorker(t, st, cfg)

	lib := filepath.Join(cfg.LibraryRoot, "lib")
	bundle := zipBytes(t, map[string][]byte{
		"Dragon/dragon.stl":        cubeSTL(10),
		"Dragon/supports/base.obj": []byte(cubeOBJ),
		"render.png":               pngBytes(64, 32),
		"readme.txt":               []byte("print at 0.2mm"),
		"__MACOSX/Dragon/._dragon": []byte("junk"),
		"inner.zip":                zipBytes(t, map[string][]byte{"deep.stl": cubeSTL(1)}),
		"unknown.xyz":              []byte("skip"),
	})
	zipPath := writeLibFile(t, lib, "Packs/bundle.zip", bundle)
	writeLibFile(t, lib, "loose.stl", cubeSTL(5))

	_, src := c.do("POST", "/api/sources", map[string]any{"path": "lib"})
	sid := fmt.Sprint(src["id"])
	waitForScan(t, c, src["id"])

	// Members appear under the archive like a folder; junk and nested contents don't.
	members, total := listFiles(t, c, "source="+sid+"&dir=Packs/bundle.zip&sort=name&kind=model&kind=image&kind=document&kind=archive")
	if total != 5 || names(members) != "base.obj,dragon.stl,inner.zip,readme.txt,render.png" {
		t.Fatalf("members: total=%v %s", total, names(members))
	}
	byName := map[string]map[string]any{}
	for _, m := range members {
		byName[m["name"].(string)] = m
		if m["archive_id"] == nil {
			t.Errorf("%s: archive_id missing", m["name"])
		}
	}
	dragon := byName["dragon.stl"]
	if dragon["rel_path"] != "Packs/bundle.zip/Dragon/dragon.stl" || dragon["dir"] != "Packs/bundle.zip/Dragon" {
		t.Errorf("dragon path: %v / %v", dragon["rel_path"], dragon["dir"])
	}
	if h, _ := dragon["content_hash"].(string); len(h) != 64 {
		t.Errorf("member not hashed: %q", h)
	}
	if items, _ := listFiles(t, c, "q=supports"); names(items) != "base.obj" {
		t.Errorf("search inside archive: %s", names(items))
	}

	// The folder tree shows the archive as a folder.
	_, tree := c.do("GET", "/api/sources/"+sid+"/tree", nil)
	if !strings.Contains(fmt.Sprint(tree), "name:bundle.zip") {
		t.Errorf("tree has no bundle.zip node: %v", tree)
	}

	// Thumbnail, geometry and download read straight from the archive.
	thumbPath := fmt.Sprintf("/api/files/%v/thumbnail", dragon["id"])
	deadline := time.Now().Add(20 * time.Second)
	for {
		status, h, _ := c.raw(thumbPath)
		if status == 200 && h.Get("Content-Type") == "image/png" {
			break
		}
		if status != http.StatusAccepted || time.Now().After(deadline) {
			t.Fatalf("member thumbnail: %d", status)
		}
		time.Sleep(100 * time.Millisecond)
	}
	status, _, body := c.raw(fmt.Sprintf("/api/files/%v/geometry", dragon["id"]))
	if status != 200 || parseGeometry(t, body).Header.Triangles != 12 {
		t.Fatalf("member geometry: %d", status)
	}
	status, h, body := c.raw(fmt.Sprintf("/api/files/%v/download", byName["readme.txt"]["id"]))
	if status != 200 || string(body) != "print at 0.2mm" || !strings.Contains(h.Get("Content-Disposition"), "readme.txt") {
		t.Errorf("member download: %d %q %q", status, h.Get("Content-Disposition"), body)
	}
	entries, _ := os.ReadDir(filepath.Join(cfg.DataDir, "tmp"))
	if len(entries) != 0 {
		t.Errorf("extracted members left behind: %d", len(entries))
	}

	rescan := func() map[string]any {
		t.Helper()
		if status, body := c.do("POST", "/api/sources/"+sid+"/scan", nil); status != 202 {
			t.Fatalf("rescan: %d %v", status, body)
		}
		return waitForScan(t, c, src["id"])
	}

	// Unchanged archive: members carry over, nothing is re-read.
	rescan()
	if _, n := listFiles(t, c, "source="+sid+"&dir=Packs/bundle.zip&kind=model"); n != 2 {
		t.Errorf("members after an unchanged rescan: %v, want 2", n)
	}

	// Changed archive: removed members go missing, new ones appear.
	updated := zipBytes(t, map[string][]byte{"Dragon/dragon.stl": cubeSTL(10), "new_part.3mf": nil})
	if err := os.WriteFile(zipPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(zipPath, future, future)
	rescan()
	items, _ := listFiles(t, c, "source="+sid+"&dir=Packs/bundle.zip&sort=name&kind=model&kind=image&kind=document")
	if names(items) != "dragon.stl,new_part.3mf" {
		t.Errorf("after changing the archive: %s", names(items))
	}

	// Deleted archive: everything inside goes missing.
	_ = os.Remove(zipPath)
	rescan()
	if _, n := listFiles(t, c, "source="+sid+"&dir=Packs/bundle.zip&kind=model"); n != 0 {
		t.Errorf("members after deleting the archive: %v", n)
	}
	if items, _ := listFiles(t, c, "kind=model"); names(items) != "loose.stl" {
		t.Errorf("remaining models: %s", names(items))
	}
}
