package api_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func cubeSTL(size float32) []byte {
	v := [8][3]float32{{0, 0, 0}, {1, 0, 0}, {1, 1, 0}, {0, 1, 0}, {0, 0, 1}, {1, 0, 1}, {1, 1, 1}, {0, 1, 1}}
	quads := [6][4]int{{0, 3, 2, 1}, {4, 5, 6, 7}, {0, 1, 5, 4}, {2, 3, 7, 6}, {1, 2, 6, 5}, {0, 4, 7, 3}}
	var b bytes.Buffer
	b.Write(make([]byte, 80))
	_ = binary.Write(&b, binary.LittleEndian, uint32(12))
	for _, q := range quads {
		for _, tri := range [2][3]int{{q[0], q[1], q[2]}, {q[0], q[2], q[3]}} {
			_ = binary.Write(&b, binary.LittleEndian, [3]float32{})
			for _, i := range tri {
				_ = binary.Write(&b, binary.LittleEndian, [3]float32{v[i][0] * size, v[i][1] * size, v[i][2] * size})
			}
			b.Write([]byte{0, 0})
		}
	}
	return b.Bytes()
}

const cubeOBJ = `v 0 0 0
v 10 0 0
v 10 10 0
v 0 10 0
v 0 0 10
v 10 0 10
v 10 10 10
v 0 10 10
f 1 4 3 2
f 5 6 7 8
f 1 2 6 5
f 3 4 8 7
f 2 3 7 6
f 1 5 8 4
`

func pngBytes(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range w {
		img.Set(i, i%h, color.NRGBA{255, 0, 0, 255})
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func threeMFWithThumbnail() []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("3D/3dmodel.model")
	_, _ = w.Write([]byte(`<?xml version="1.0"?><model unit="millimeter" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02">
<resources><object id="1"><mesh><vertices><vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="0" y="1" z="0"/></vertices>
<triangles><triangle v1="0" v2="1" v3="2"/></triangles></mesh></object></resources><build><item objectid="1"/></build></model>`))
	w, _ = zw.Create("Metadata/plate_1.png")
	_, _ = w.Write(pngBytes(32, 32))
	_ = zw.Close()
	return buf.Bytes()
}

func writeLibFile(t *testing.T, root, rel string, data []byte) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func (c *client) raw(path string) (int, http.Header, []byte) {
	c.t.Helper()
	resp, err := c.http.Get(c.base + path)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, body
}

// waitForScan polls the source until its latest scan has finished.
func waitForScan(t *testing.T, c *client, sourceID any) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_, list := c.do("GET", "/api/sources", nil)
		for _, it := range list["items"].([]any) {
			src := it.(map[string]any)
			if src["id"] != sourceID {
				continue
			}
			if scan, _ := src["last_scan"].(map[string]any); scan != nil {
				switch scan["state"] {
				case "done":
					return src
				case "failed":
					t.Fatalf("scan failed: %v", scan)
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatal("scan did not finish")
	return nil
}

func listFiles(t *testing.T, c *client, query string) ([]map[string]any, float64) {
	t.Helper()
	status, body := c.do("GET", "/api/files?"+query, nil)
	if status != 200 {
		t.Fatalf("list files %q: %d %v", query, status, body)
	}
	var items []map[string]any
	for _, it := range body["items"].([]any) {
		items = append(items, it.(map[string]any))
	}
	total, _ := body["total"].(float64)
	return items, total
}

func names(items []map[string]any) string {
	var n []string
	for _, it := range items {
		n = append(n, it["name"].(string))
	}
	return strings.Join(n, ",")
}

func TestLibraryScanThumbnailsAndSearch(t *testing.T) {
	cfg := testConfig(t)
	base, st := newServerWithConfig(t, cfg)
	c := newClient(t, base)
	setupAdmin(t, c)
	startWorker(t, st, cfg)

	lib := filepath.Join(cfg.LibraryRoot, "prints")
	writeLibFile(t, lib, "dragon.stl", cubeSTL(10))
	writeLibFile(t, lib, "Benchy (1).stl", cubeSTL(20))
	writeLibFile(t, lib, "parts/bracket.obj", []byte(cubeOBJ))
	writeLibFile(t, lib, "parts/plates/project.3mf", threeMFWithThumbnail())
	writeLibFile(t, lib, "parts/photo.png", pngBytes(800, 400))
	writeLibFile(t, lib, "readme.txt", []byte("hello"))
	writeLibFile(t, lib, "ignored.xyz", []byte("unknown format"))
	writeLibFile(t, lib, ".hidden/secret.stl", cubeSTL(1))
	writeLibFile(t, lib, "@eaDir/thumb.stl", cubeSTL(1))
	writeLibFile(t, lib, "parts/broken.stl", []byte("solid broken\nnot really\n"))

	// Folder browser and path validation.
	status, dirs := c.do("GET", "/api/fs/dirs", nil)
	if status != 200 || !strings.Contains(fmt.Sprint(dirs["dirs"]), "prints") {
		t.Fatalf("browse: %d %v", status, dirs)
	}
	if status, _ := c.do("POST", "/api/sources", map[string]any{"path": "../"}); status != 400 {
		t.Errorf("source outside library root: %d, want 400", status)
	}
	if status, _ := c.do("POST", "/api/sources", map[string]any{"path": "nope"}); status != 400 {
		t.Errorf("missing folder: %d, want 400", status)
	}

	status, src := c.do("POST", "/api/sources", map[string]any{"path": "prints", "thumbnail_mode": "lazy"})
	if status != 201 || src["name"] != "prints" {
		t.Fatalf("create source: %d %v", status, src)
	}
	if status, _ := c.do("POST", "/api/sources", map[string]any{"path": "prints/parts"}); status != 409 {
		t.Errorf("overlapping source: %d, want 409", status)
	}
	srcID := src["id"]
	src = waitForScan(t, c, srcID)
	if src["file_count"] != 7.0 || src["model_count"] != 5.0 {
		t.Errorf("counts: files=%v models=%v, want 7 and 5", src["file_count"], src["model_count"])
	}

	models, total := listFiles(t, c, "kind=model&sort=name")
	if total != 5 || names(models) != "Benchy (1).stl,bracket.obj,broken.stl,dragon.stl,project.3mf" {
		t.Fatalf("models: total=%v names=%s", total, names(models))
	}
	for _, m := range models {
		if h, _ := m["content_hash"].(string); len(h) != 64 {
			t.Errorf("%s: content hash %q", m["name"], h)
		}
	}

	// Search matches path segments, case-insensitively, across terms.
	if items, _ := listFiles(t, c, "q=PLATES+proj"); names(items) != "project.3mf" {
		t.Errorf("search: %s", names(items))
	}
	// Folder filter includes subfolders.
	sid := fmt.Sprint(srcID)
	if items, _ := listFiles(t, c, "source="+sid+"&dir=parts&kind=model&sort=name"); names(items) != "bracket.obj,broken.stl,project.3mf" {
		t.Errorf("dir filter: %s", names(items))
	}
	// Cursor pagination walks every item exactly once.
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 10; page++ {
		status, body := c.do("GET", "/api/files?sort=-size&limit=2&cursor="+url.QueryEscape(cursor), nil)
		if status != 200 {
			t.Fatalf("page: %d %v", status, body)
		}
		for _, it := range body["items"].([]any) {
			n := it.(map[string]any)["name"].(string)
			if seen[n] {
				t.Fatalf("%s returned twice", n)
			}
			seen[n] = true
		}
		next, _ := body["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != 7 {
		t.Errorf("pagination saw %d files, want 7", len(seen))
	}

	// Folder tree with recursive counts.
	_, tree := c.do("GET", "/api/sources/"+sid+"/tree", nil)
	if tree["count"] != 7.0 {
		t.Errorf("tree root count: %v", tree["count"])
	}
	parts := tree["children"].([]any)[0].(map[string]any)
	if parts["name"] != "parts" || parts["count"] != 4.0 {
		t.Errorf("parts folder: %v", parts)
	}

	// Facets count kinds and formats.
	status, facets := c.do("GET", "/api/files/facets", nil)
	if status != 200 || !strings.Contains(fmt.Sprint(facets["items"]), "format:stl kind:model") {
		t.Errorf("facets: %d %v", status, facets)
	}

	// Lazy thumbnails: 202 until the worker renders it, then a PNG.
	byName := map[string]map[string]any{}
	for _, m := range models {
		byName[m["name"].(string)] = m
	}
	thumbOf := func(name string) (int, http.Header, []byte) {
		path := fmt.Sprintf("/api/files/%v/thumbnail", byName[name]["id"])
		deadline := time.Now().Add(20 * time.Second)
		for {
			status, h, body := c.raw(path)
			if status != http.StatusAccepted || time.Now().After(deadline) {
				return status, h, body
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	for _, name := range []string{"dragon.stl", "bracket.obj", "project.3mf"} {
		status, h, body := thumbOf(name)
		if status != 200 || h.Get("Content-Type") != "image/png" {
			t.Fatalf("%s thumbnail: %d %s", name, status, body)
		}
		img, err := png.Decode(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("%s thumbnail is not a PNG: %v", name, err)
		}
		if name == "project.3mf" && img.Bounds().Dx() != 32 {
			t.Errorf("3mf should use its embedded 32px preview, got %v", img.Bounds())
		}
	}
	if status, _, _ := thumbOf("broken.stl"); status != 404 {
		t.Errorf("broken model thumbnail: %d, want 404", status)
	}

	// After rendering, the listing reports dimensions and a cacheable thumb URL.
	_, dragon := c.do("GET", fmt.Sprintf("/api/files/%v", byName["dragon.stl"]["id"]), nil)
	if dragon["thumb"] != "ready" || !strings.Contains(fmt.Sprint(dragon["thumb_url"]), "?v=") {
		t.Errorf("dragon thumb state: %v %v", dragon["thumb"], dragon["thumb_url"])
	}
	if dims := fmt.Sprint(dragon["dimensions"]); dims != "[10 10 10]" || dragon["triangles"] != 12.0 {
		t.Errorf("dragon stats: dims=%s triangles=%v", dims, dragon["triangles"])
	}
	_, broken := c.do("GET", fmt.Sprintf("/api/files/%v", byName["broken.stl"]["id"]), nil)
	if broken["thumb"] != "failed" {
		t.Errorf("broken thumb state: %v", broken["thumb"])
	}
	_, readme := listFiles(t, c, "kind=document")
	if readme != 1 {
		t.Errorf("documents: %v", readme)
	}

	// Download returns the original bytes as an attachment.
	status, h, body := c.raw(fmt.Sprintf("/api/files/%v/download", byName["bracket.obj"]["id"]))
	if status != 200 || string(body) != cubeOBJ || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") {
		t.Errorf("download: %d %q %q", status, h.Get("Content-Disposition"), body)
	}

	// Rescan: a modified file loses its hash-derived data; a deleted one goes missing.
	oldHash := byName["dragon.stl"]["content_hash"]
	future := time.Now().Add(time.Hour)
	p := writeLibFile(t, lib, "dragon.stl", cubeSTL(30))
	_ = os.Chtimes(p, future, future)
	_ = os.Remove(filepath.Join(lib, "Benchy (1).stl"))
	if status, job := c.do("POST", "/api/sources/"+sid+"/scan", nil); status != 202 {
		t.Fatalf("rescan: %d %v", status, job)
	}
	src = waitForScan(t, c, srcID)
	if msg := fmt.Sprint(src["last_scan"].(map[string]any)["message"]); !strings.Contains(msg, "1 missing") {
		t.Errorf("rescan summary: %s", msg)
	}
	models, _ = listFiles(t, c, "kind=model&sort=name")
	if names(models) != "bracket.obj,broken.stl,dragon.stl,project.3mf" {
		t.Errorf("after rescan: %s", names(models))
	}
	for _, m := range models {
		if m["name"] == "dragon.stl" {
			if m["content_hash"] == oldHash || m["thumb"] != "pending" || m["dimensions"] != nil {
				t.Errorf("modified file kept stale data: hash=%v thumb=%v dims=%v", m["content_hash"], m["thumb"], m["dimensions"])
			}
		}
	}

	// Deleting the source clears the index but leaves files on disk.
	if status, _ := c.do("DELETE", "/api/sources/"+sid, nil); status != 204 {
		t.Fatalf("delete source: %d", status)
	}
	if _, total := listFiles(t, c, ""); total != 0 {
		t.Errorf("files after delete: %v", total)
	}
	if _, err := os.Stat(filepath.Join(lib, "dragon.stl")); err != nil {
		t.Errorf("library file touched: %v", err)
	}
}

func TestEagerThumbnails(t *testing.T) {
	cfg := testConfig(t)
	base, st := newServerWithConfig(t, cfg)
	c := newClient(t, base)
	setupAdmin(t, c)
	startWorker(t, st, cfg)

	lib := filepath.Join(cfg.LibraryRoot, "eager")
	for i := range 5 {
		writeLibFile(t, lib, fmt.Sprintf("cube%d.stl", i), cubeSTL(float32(i+1)))
	}
	_, src := c.do("POST", "/api/sources", map[string]any{"path": "eager", "thumbnail_mode": "eager"})
	waitForScan(t, c, src["id"])

	deadline := time.Now().Add(20 * time.Second)
	for {
		items, _ := listFiles(t, c, "kind=model")
		ready := 0
		for _, it := range items {
			if it["thumb"] == "ready" {
				ready++
			}
		}
		if ready == 5 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of 5 thumbnails rendered eagerly", ready)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func TestScanDeduplicationAndStaleRecovery(t *testing.T) {
	cfg := testConfig(t)
	base, _ := newServerWithConfig(t, cfg)
	c := newClient(t, base)
	setupAdmin(t, c)
	writeLibFile(t, filepath.Join(cfg.LibraryRoot, "lib"), "cube.stl", cubeSTL(10))

	// No worker running: the first scan stays queued.
	_, src := c.do("POST", "/api/sources", map[string]any{"path": "lib"})
	scanPath := fmt.Sprintf("/api/sources/%v/scan", src["id"])
	if status, _ := c.do("POST", scanPath, nil); status != 409 {
		t.Fatalf("second scan while one is queued: %d, want 409", status)
	}

	// Simulate a worker that died mid-scan: River gave up on the job, but the tracking
	// row still says running. A new scan must be allowed and the stale row closed.
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "UPDATE jobs SET state = 'running' WHERE kind = 'scan'"); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, "UPDATE river_job SET state = 'discarded', finalized_at = now() WHERE kind = 'scan'"); err != nil {
		t.Fatal(err)
	}
	if status, body := c.do("POST", scanPath, nil); status != 202 {
		t.Fatalf("scan after a dead worker: %d %v", status, body)
	}
	var stale string
	if err := testPool.QueryRow(ctx, "SELECT error FROM jobs WHERE kind = 'scan' ORDER BY id LIMIT 1").Scan(&stale); err != nil || stale != "interrupted" {
		t.Errorf("stale scan row: error=%q (%v)", stale, err)
	}
}
