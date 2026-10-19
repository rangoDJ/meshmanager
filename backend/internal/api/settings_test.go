package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rangoDJ/meshmanager/internal/settings"
)

func TestSettingsAPI(t *testing.T) {
	cfg := testConfig(t)
	base, _ := newServerWithConfig(t, cfg)
	c := newClient(t, base)
	setupAdmin(t, c)

	status, body := c.do("GET", "/api/settings", nil)
	cur := body["settings"].(map[string]any)
	if status != 200 || cur["thumb_size"] != 384.0 || cur["scan_interval_hours"] != 12.0 {
		t.Fatalf("defaults: %d %v", status, body)
	}
	if !strings.HasSuffix(fmt.Sprint(body["file"]), settings.FileName) {
		t.Errorf("file path: %v", body["file"])
	}

	if status, _ := c.do("PUT", "/api/settings", map[string]any{"thumb_size": 50}); status != 400 {
		t.Errorf("invalid thumb_size: %d, want 400", status)
	}
	if status, _ := c.do("PUT", "/api/settings", map[string]any{"nope": 1}); status != 400 {
		t.Errorf("unknown field: %d, want 400", status)
	}

	// Partial update: other fields keep their values.
	status, body = c.do("PUT", "/api/settings", map[string]any{"thumb_size": 256, "index_videos": true})
	cur = body["settings"].(map[string]any)
	if status != 200 || cur["thumb_size"] != 256.0 || cur["index_videos"] != true || cur["hash_concurrency"] != 4.0 {
		t.Fatalf("save: %d %v", status, body)
	}

	raw, err := os.ReadFile(filepath.Join(cfg.DataDir, settings.FileName))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk settings.Settings
	if err := json.Unmarshal(raw, &onDisk); err != nil || onDisk.ThumbSize != 256 || !onDisk.IndexVideos {
		t.Fatalf("config.json = %s (%v)", raw, err)
	}
}

// Saved settings reach the worker without a restart.
func TestSettingsApplyToJobs(t *testing.T) {
	cfg := testConfig(t)
	base, st := newServerWithConfig(t, cfg)
	c := newClient(t, base)
	setupAdmin(t, c)
	startWorker(t, st, cfg)

	lib := filepath.Join(cfg.LibraryRoot, "lib")
	writeLibFile(t, lib, "cube.stl", cubeSTL(10))
	writeLibFile(t, lib, "timelapse.mp4", []byte("not really a video"))

	_, src := c.do("POST", "/api/sources", map[string]any{"path": "lib"})
	waitForScan(t, c, src["id"])
	if _, n := listFiles(t, c, "kind=video"); n != 0 {
		t.Fatalf("videos indexed while index_videos is off: %v", n)
	}

	if status, _ := c.do("PUT", "/api/settings", map[string]any{"index_videos": true, "thumb_size": 160}); status != 200 {
		t.Fatal("save settings failed")
	}
	if status, _ := c.do("POST", fmt.Sprintf("/api/sources/%v/scan", src["id"]), nil); status != 202 {
		t.Fatal("rescan failed")
	}
	waitForScan(t, c, src["id"])
	if _, n := listFiles(t, c, "kind=video"); n != 1 {
		t.Errorf("videos after enabling index_videos: %v, want 1", n)
	}

	models, _ := listFiles(t, c, "kind=model")
	path := fmt.Sprintf("/api/files/%v/thumbnail", models[0]["id"])
	deadline := time.Now().Add(20 * time.Second)
	for {
		status, _, body := c.raw(path)
		if status == http.StatusOK {
			img, err := png.Decode(bytes.NewReader(body))
			if err != nil || img.Bounds().Dx() != 160 {
				t.Fatalf("thumbnail should be 160px from settings, got %v (%v)", img.Bounds(), err)
			}
			return
		}
		if status != http.StatusAccepted || time.Now().After(deadline) {
			t.Fatalf("thumbnail: %d", status)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
