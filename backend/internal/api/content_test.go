package api_test

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentAndHostPath(t *testing.T) {
	cfg := testConfig(t)
	base, st := newServerWithConfig(t, cfg)
	c := newClient(t, base)
	setupAdmin(t, c)
	startWorker(t, st, cfg)
	if status, _ := c.do("PUT", "/api/settings", map[string]any{"index_videos": true}); status != 200 {
		t.Fatal("enable videos")
	}

	lib := filepath.Join(cfg.LibraryRoot, "lib")
	writeLibFile(t, lib, "Docs/readme.md", []byte("# Print notes\n<script>alert(1)</script>\n"))
	writeLibFile(t, lib, "Docs/manual.pdf", []byte("%PDF-1.4 fake"))
	writeLibFile(t, lib, "Docs/cut.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`))
	writeLibFile(t, lib, "Photos/print.png", pngBytes(120, 60))
	writeLibFile(t, lib, "Video/timelapse.mp4", []byte(strings.Repeat("v", 1000)))
	writeLibFile(t, lib, "cube.stl", cubeSTL(10))
	_, src := c.do("POST", "/api/sources", map[string]any{"path": "lib"})
	waitForScan(t, c, src["id"])

	items, _ := listFiles(t, c, "kind=document&kind=image&kind=video&kind=model&sort=name")
	byName := map[string]map[string]any{}
	for _, it := range items {
		byName[it["name"].(string)] = it
	}
	for name, want := range map[string]bool{"readme.md": true, "manual.pdf": true, "cut.svg": true, "print.png": true, "timelapse.mp4": true, "cube.stl": false} {
		if byName[name]["inline"] != want {
			t.Errorf("%s inline = %v, want %v", name, byName[name]["inline"], want)
		}
	}
	content := func(name string, headers map[string]string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/files/%v/content", base, byName[name]["id"]), nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}

	resp, body := content("cut.svg", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/svg+xml" ||
		!strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("svg: %d %q csp=%q", resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Security-Policy"))
	}
	resp, body = content("readme.md", nil)
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") || !strings.Contains(body, "# Print notes") ||
		!strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("markdown served as %q, csp %q", resp.Header.Get("Content-Type"), resp.Header.Get("Content-Security-Policy"))
	}
	resp, _ = content("manual.pdf", nil)
	if resp.Header.Get("Content-Type") != "application/pdf" || resp.Header.Get("X-Frame-Options") != "SAMEORIGIN" ||
		resp.Header.Get("Content-Security-Policy") != "frame-ancestors 'self'" {
		t.Errorf("pdf headers: %q %q %q", resp.Header.Get("Content-Type"), resp.Header.Get("X-Frame-Options"), resp.Header.Get("Content-Security-Policy"))
	}
	resp, body = content("timelapse.mp4", map[string]string{"Range": "bytes=10-19"})
	if resp.StatusCode != http.StatusPartialContent || len(body) != 10 || resp.Header.Get("Content-Type") != "video/mp4" {
		t.Errorf("video range: %d len=%d", resp.StatusCode, len(body))
	}
	if resp, _ := content("cube.stl", nil); resp.StatusCode != 404 {
		t.Errorf("stl content: %d, want 404", resp.StatusCode)
	}

	status, info := c.do("GET", fmt.Sprintf("/api/files/%v/image-info", byName["print.png"]["id"]), nil)
	if status != 200 || info["width"] != 120.0 || info["height"] != 60.0 {
		t.Errorf("image info: %d %v", status, info)
	}

	// Host paths: POSIX, Windows/UNC style, and clearing.
	sid := fmt.Sprint(src["id"])
	readme := fmt.Sprintf("/api/files/%v", byName["readme.md"]["id"])
	if _, f := c.do("GET", readme, nil); f["host_path"] != nil {
		t.Errorf("host path without a source host path: %v", f["host_path"])
	}
	for set, want := range map[string]string{
		"/mnt/nas/prints/": "/mnt/nas/prints/Docs/readme.md",
		`\\nas\prints`:     `\\nas\prints\Docs\readme.md`,
		`D:\3D Prints\`:    `D:\3D Prints\Docs\readme.md`,
	} {
		if status, s := c.do("PATCH", "/api/sources/"+sid, map[string]any{"host_path": set}); status != 200 || s["host_path"] != strings.TrimSpace(set) {
			t.Fatalf("set host path %q: %d %v", set, status, s["host_path"])
		}
		if _, f := c.do("GET", readme, nil); f["host_path"] != want {
			t.Errorf("host path for %q: %v, want %q", set, f["host_path"], want)
		}
	}
	if _, s := c.do("PATCH", "/api/sources/"+sid, map[string]any{"host_path": ""}); s["host_path"] != nil {
		t.Errorf("clearing host path: %v", s["host_path"])
	}
	if _, s := c.do("PATCH", "/api/sources/"+sid, map[string]any{"name": "Renamed"}); s["name"] != "Renamed" || s["host_path"] != nil {
		t.Errorf("rename kept host path cleared? %v %v", s["name"], s["host_path"])
	}
}
