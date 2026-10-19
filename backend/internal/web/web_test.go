package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerFS(t *testing.T) {
	h := HandlerFS(fstest.MapFS{
		"index.html":      {Data: []byte("<html>app</html>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
		"favicon.svg":     {Data: []byte("<svg/>")},
	})

	cases := []struct {
		method, path string
		status       int
		body         string
		cache        string
	}{
		{"GET", "/", 200, "app", "no-cache"},
		{"GET", "/settings/account", 200, "app", "no-cache"},
		{"GET", "/assets/app-1.js", 200, "console.log", "immutable"},
		{"GET", "/assets/missing.js", 404, "", ""},
		{"GET", "/favicon.svg", 200, "<svg/>", ""},
		{"GET", "/../../etc/passwd", 200, "app", "no-cache"},
		{"POST", "/", 405, "", ""},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.status {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, rec.Code, c.status)
		}
		if !strings.Contains(rec.Body.String(), c.body) {
			t.Errorf("%s %s: body %q missing %q", c.method, c.path, rec.Body.String(), c.body)
		}
		if c.cache != "" && !strings.Contains(rec.Header().Get("Cache-Control"), c.cache) {
			t.Errorf("%s %s: Cache-Control %q, want %q", c.method, c.path, rec.Header().Get("Cache-Control"), c.cache)
		}
	}
}

func TestHandlerFSWithoutBuild(t *testing.T) {
	rec := httptest.NewRecorder()
	HandlerFS(fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", rec.Code)
	}
}
