package api

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/rangoDJ/meshmanager/internal/imageinfo"
	"github.com/rangoDJ/meshmanager/internal/library"
	"github.com/rangoDJ/meshmanager/internal/store"
)

// inlineTypes are the formats the app shows in place, with the type they're served as.
var inlineTypes = map[string]string{
	"jpg":  "image/jpeg",
	"png":  "image/png",
	"gif":  "image/gif",
	"webp": "image/webp",
	"bmp":  "image/bmp",
	"avif": "image/avif",
	"svg":  "image/svg+xml",
	"pdf":  "application/pdf",
	"txt":  "text/plain; charset=utf-8",
	"md":   "text/plain; charset=utf-8",
	"mp4":  "video/mp4",
	"mov":  "video/quicktime",
	"webm": "video/webm",
}

// InlineViewable reports whether a format can be opened in the app's file viewer.
func InlineViewable(format string) bool {
	_, ok := inlineTypes[format]
	return ok
}

// handleContent serves a companion file (image, PDF, text, video) for viewing in the
// app. Library files are untrusted, so every response is locked down: SVG and text are
// sandboxed (no scripts, no same-origin access), and only PDFs may be framed, by the
// app itself.
func (s *Server) handleContent(w http.ResponseWriter, r *http.Request) {
	loc, ok := s.fileLocation(w, r)
	if !ok {
		return
	}
	ct, ok := inlineTypes[loc.Format]
	if !ok {
		writeError(w, http.StatusNotFound, "this file type can't be shown in the app")
		return
	}

	h := w.Header()
	switch loc.Format {
	case "pdf":
		// The browser's PDF viewer refuses sandboxed documents; allow framing by us only.
		h.Set("Content-Security-Policy", "frame-ancestors 'self'")
		h.Set("X-Frame-Options", "SAMEORIGIN")
	default:
		h.Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src data:; style-src 'unsafe-inline'; frame-ancestors 'none'")
	}
	h.Set("Content-Type", ct)
	h.Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": loc.Name}))
	h.Set("Cache-Control", "private, no-cache")

	lib := library.FromStore(loc)
	if !lib.IsMember() {
		f, err := os.Open(lib.DiskPath())
		if err != nil {
			writeError(w, http.StatusNotFound, "file is no longer on disk")
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			writeError(w, http.StatusNotFound, "file is no longer on disk")
			return
		}
		http.ServeContent(w, r, "", st.ModTime(), f) // supports Range, so videos can seek
		return
	}
	rc, size, err := library.Open(lib)
	if err != nil {
		writeError(w, http.StatusNotFound, "cannot open this file: "+err.Error())
		return
	}
	defer rc.Close()
	h.Set("Content-Length", strconv.FormatInt(size, 10))
	if _, err := io.Copy(w, rc); err != nil && r.Context().Err() == nil {
		slog.Warn("serve archive member", "file", loc.ID, "err", err)
	}
}

// handleImageInfo returns an image's pixel size and resolution (DPI) when the file
// records one, so the UI can show its printed size.
func (s *Server) handleImageInfo(w http.ResponseWriter, r *http.Request) {
	loc, ok := s.fileLocation(w, r)
	if !ok {
		return
	}
	switch loc.Format {
	case "jpg", "png", "gif", "webp", "bmp":
	default:
		writeError(w, http.StatusNotFound, "no image information for this format")
		return
	}
	rc, _, err := library.Open(library.FromStore(loc))
	if err != nil {
		writeError(w, http.StatusNotFound, "cannot open this file: "+err.Error())
		return
	}
	defer rc.Close()
	head, err := io.ReadAll(io.LimitReader(rc, 4<<20))
	if err != nil {
		internalError(w, r, err)
		return
	}
	info, err := imageinfo.Read(head)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "cannot read this image: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// fileLocation looks up the {id} file, writing a 404 when it doesn't exist.
func (s *Server) fileLocation(w http.ResponseWriter, r *http.Request) (store.FileLocation, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return store.FileLocation{}, false
	}
	loc, err := s.store.GetFileLocation(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "file not found")
		return loc, false
	}
	if err != nil {
		internalError(w, r, err)
		return loc, false
	}
	return loc, true
}

// hostPath joins a source's host path with a file's path inside the source, using
// backslashes when the host path looks like Windows or a network share.
func hostPath(base, rel string) string {
	windows := strings.Contains(base, `\`) || (len(base) >= 2 && base[1] == ':')
	base = strings.TrimRight(base, `/\`)
	if windows {
		return base + `\` + strings.ReplaceAll(rel, "/", `\`)
	}
	return base + "/" + rel
}
