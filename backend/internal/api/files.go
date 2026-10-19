package api

import (
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rangoDJ/meshmanager/internal/formats"
	"github.com/rangoDJ/meshmanager/internal/jobs"
	"github.com/rangoDJ/meshmanager/internal/store"
	"github.com/rangoDJ/meshmanager/internal/thumbs"
)

// fileView is a file as the frontend sees it.
type fileView struct {
	store.FileItem
	// Thumb is ready, pending (not rendered yet), failed, or none (format has no thumbnails).
	Thumb string `json:"thumb"`
	// ThumbURL includes a version so browsers can cache the image forever.
	ThumbURL *string `json:"thumb_url"`
}

func toView(it store.FileItem) fileView {
	v := fileView{FileItem: it}
	base := "/api/files/" + strconv.FormatInt(it.ID, 10) + "/thumbnail"
	switch {
	case it.ThumbStatus != nil && *it.ThumbStatus == store.ThumbReady:
		v.Thumb = "ready"
		u := base + "?v=" + strconv.FormatInt(it.ThumbAt.UnixMilli(), 10)
		v.ThumbURL = &u
	case it.ThumbStatus != nil && *it.ThumbStatus == store.ThumbFailed:
		v.Thumb = "failed"
	case formats.CanThumbnail(it.Format):
		v.Thumb = "pending"
		v.ThumbURL = &base
	default:
		v.Thumb = "none"
	}
	return v
}

func fileFilterFrom(r *http.Request) (store.FileFilter, string) {
	q := r.URL.Query()
	f := store.FileFilter{
		Kinds:   q["kind"],
		Formats: q["format"],
		Query:   q.Get("q"),
		Sort:    q.Get("sort"),
		Cursor:  q.Get("cursor"),
		Dir:     q.Get("dir"),
	}
	if v := q.Get("source"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return f, "source must be a number"
		}
		f.SourceID = id
	}
	if f.Dir != "" && f.SourceID == 0 {
		return f, "dir requires source"
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			return f, "limit must be between 1 and 500"
		}
		f.Limit = n
	}
	return f, ""
}

type fileList struct {
	Items      []fileView `json:"items"`
	NextCursor *string    `json:"next_cursor"`
	// Total is only computed for the first page.
	Total *int64 `json:"total,omitempty"`
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	f, msg := fileFilterFrom(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	items, next, err := s.store.ListFiles(r.Context(), f)
	if errors.Is(err, store.ErrBadCursor) {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := fileList{Items: make([]fileView, len(items))}
	for i, it := range items {
		out.Items[i] = toView(it)
	}
	if next != "" {
		out.NextCursor = &next
	}
	if f.Cursor == "" {
		total, err := s.store.CountFiles(r.Context(), f)
		if err != nil {
			internalError(w, r, err)
			return
		}
		out.Total = &total
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleFacets(w http.ResponseWriter, r *http.Request) {
	f, msg := fileFilterFrom(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	facets, err := s.store.Facets(r.Context(), f)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, facets)
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	it, err := s.store.GetFile(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toView(it))
}

// handleThumbnail serves a ready thumbnail. A missing one is queued at high priority
// and answered with 202, so the client retries shortly.
func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := s.store.GetThumbnail(r.Context(), id)
	switch {
	case err == nil && t.Status == store.ThumbReady && t.RendererVersion == thumbs.Version:
		f, err := os.Open(filepath.Join(s.cfg.DataDir, filepath.FromSlash(t.Path)))
		if err != nil {
			break // file vanished from /data: render again
		}
		defer f.Close()
		w.Header().Set("Content-Type", t.ContentType)
		if r.URL.Query().Has("v") {
			w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "private, no-cache")
		}
		http.ServeContent(w, r, "", t.CreatedAt, f)
		return
	case err == nil && t.Status == store.ThumbFailed:
		writeError(w, http.StatusNotFound, "thumbnail could not be generated: "+t.Error)
		return
	case err != nil && !errors.Is(err, store.ErrNotFound):
		internalError(w, r, err)
		return
	}

	loc, err := s.store.GetFileLocation(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !formats.CanThumbnail(loc.Format) {
		writeError(w, http.StatusNotFound, "no thumbnail for this format")
		return
	}
	if err := jobs.RequestThumbnail(r.Context(), s.store, s.queue, id); err != nil {
		internalError(w, r, err)
		return
	}
	w.Header().Set("Retry-After", "1")
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
}

// handleDownload streams the original file as an attachment.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	loc, err := s.store.GetFileLocation(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	full := filepath.Join(loc.SourcePath, filepath.FromSlash(loc.RelPath))
	if !within(filepath.Clean(loc.SourcePath), full) {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	f, err := os.Open(full)
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
	// Always an attachment with a neutral type, so nothing from the library (SVG, HTML
	// inside a model pack) ever renders in the app's origin.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": loc.Name}))
	http.ServeContent(w, r, "", st.ModTime(), f)
}
