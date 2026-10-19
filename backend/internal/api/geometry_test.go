package api_test

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"testing"
)

type geometry struct {
	Header struct {
		Version   int `json:"version"`
		Triangles int `json:"triangles"`
		Parts     []struct {
			Name  string `json:"name"`
			Start int    `json:"start"`
			Count int    `json:"count"`
		} `json:"parts"`
		Plates     []any      `json:"plates"`
		Min        [3]float64 `json:"min"`
		Max        [3]float64 `json:"max"`
		VolumeMM3  float64    `json:"volume_mm3"`
		Watertight *bool      `json:"watertight"`
	}
	Floats []float32
}

func parseGeometry(t *testing.T, b []byte) geometry {
	t.Helper()
	if len(b) < 8 || string(b[:4]) != "MMG1" {
		t.Fatalf("bad magic: %q", b[:min(len(b), 8)])
	}
	n := int(binary.LittleEndian.Uint32(b[4:8]))
	if n%4 != 0 {
		t.Fatalf("header length %d not 4-byte aligned", n)
	}
	var g geometry
	if err := json.Unmarshal(b[8:8+n], &g.Header); err != nil {
		t.Fatalf("header: %v", err)
	}
	rest := b[8+n:]
	for i := 0; i+4 <= len(rest); i += 4 {
		g.Floats = append(g.Floats, math.Float32frombits(binary.LittleEndian.Uint32(rest[i:])))
	}
	return g
}

func TestGeometryEndpoint(t *testing.T) {
	cfg := testConfig(t)
	base, st := newServerWithConfig(t, cfg)
	c := newClient(t, base)
	setupAdmin(t, c)
	startWorker(t, st, cfg)

	lib := filepath.Join(cfg.LibraryRoot, "lib")
	writeLibFile(t, lib, "cube.stl", cubeSTL(10))
	writeLibFile(t, lib, "photo.png", pngBytes(4, 4))
	_, src := c.do("POST", "/api/sources", map[string]any{"path": "lib"})
	waitForScan(t, c, src["id"])

	items, _ := listFiles(t, c, "sort=name")
	byName := map[string]map[string]any{}
	for _, it := range items {
		byName[it["name"].(string)] = it
	}
	if byName["cube.stl"]["viewable"] != true || byName["photo.png"]["viewable"] != false {
		t.Fatalf("viewable flags: stl=%v png=%v", byName["cube.stl"]["viewable"], byName["photo.png"]["viewable"])
	}
	path := fmt.Sprintf("/api/files/%v/geometry", byName["cube.stl"]["id"])

	req, _ := http.NewRequest("GET", base+path, nil)
	req.Header.Set("Accept-Encoding", "gzip") // set by hand so Go doesn't decompress for us
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("geometry: %d encoding=%q", resp.StatusCode, resp.Header.Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(zr)
	g := parseGeometry(t, body)

	h := g.Header
	if h.Triangles != 12 || len(g.Floats) != 12*9 {
		t.Fatalf("triangles=%d floats=%d", h.Triangles, len(g.Floats))
	}
	if len(h.Parts) != 1 || h.Parts[0].Count != 12 || h.Plates == nil {
		t.Errorf("parts=%+v plates=%v", h.Parts, h.Plates)
	}
	if h.Max[2] != 10 || h.Min[0] != 0 || math.Abs(h.VolumeMM3-1000) > 1e-3 {
		t.Errorf("bounds %v..%v volume %v", h.Min, h.Max, h.VolumeMM3)
	}
	if h.Watertight == nil || !*h.Watertight {
		t.Errorf("watertight = %v", h.Watertight)
	}

	// Revalidation with the ETag skips the conversion.
	etag := resp.Header.Get("ETag")
	req, _ = http.NewRequest("GET", base+path, nil)
	req.Header.Set("If-None-Match", etag)
	resp, err = c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if etag == "" || resp.StatusCode != http.StatusNotModified {
		t.Errorf("revalidate: etag=%q status=%d", etag, resp.StatusCode)
	}

	// Stats are stored for the drawer and grid.
	_, file := c.do("GET", fmt.Sprintf("/api/files/%v", byName["cube.stl"]["id"]), nil)
	if file["watertight"] != true || fmt.Sprint(file["dimensions"]) != "[10 10 10]" {
		t.Errorf("stored stats: watertight=%v dims=%v", file["watertight"], file["dimensions"])
	}

	if status, _ := c.do("GET", fmt.Sprintf("/api/files/%v/geometry", byName["photo.png"]["id"]), nil); status != 404 {
		t.Errorf("geometry of a PNG: %d, want 404", status)
	}
}
