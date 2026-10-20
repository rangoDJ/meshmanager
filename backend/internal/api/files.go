package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rangoDJ/meshmanager/internal/formats"
	"github.com/rangoDJ/meshmanager/internal/jobs"
	"github.com/rangoDJ/meshmanager/internal/library"
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
	// Viewable is true when the file opens in the 3D viewer.
	Viewable bool `json:"viewable"`
	// Inline is true when the file opens in the app's file viewer (images, PDF, text, video).
	Inline bool `json:"inline"`
	// HostPath is the file's location on the host, when the source has a host path set.
	HostPath *string `json:"host_path"`
	// CustomThumb is true when the thumbnail is a photo the user chose.
	CustomThumb bool `json:"custom_thumb"`
	// Detail is only present when a single file is fetched.
	*fileDetail
}

// fileDetail is what the file panel shows beyond what the grid needs.
type fileDetail struct {
	Meta        store.FileMeta        `json:"meta"`
	Collections []store.CollectionRef `json:"collections"`
	Queue       []store.QueueRef      `json:"queue"`
	Photos      []photoView           `json:"photos"`
}

func toView(it store.FileItem) fileView {
	v := fileView{FileItem: it, Viewable: formats.CanView(it.Format), Inline: InlineViewable(it.Format)}
	if it.SourceHostPath != nil && *it.SourceHostPath != "" {
		hp := hostPath(*it.SourceHostPath, it.RelPath)
		v.HostPath = &hp
	}
	base := "/api/files/" + strconv.FormatInt(it.ID, 10) + "/thumbnail"
	// A thumbnail from an older renderer counts as not rendered yet.
	current := it.ThumbVersion != nil && *it.ThumbVersion == thumbs.Version
	switch {
	case it.CoverPhotoID != nil && it.CoverAt != nil:
		// A photo the user chose stands in for the generated thumbnail.
		v.Thumb, v.CustomThumb = "ready", true
		u := photoURL(*it.CoverPhotoID, true) + "&v=" + strconv.FormatInt(it.CoverAt.UnixMilli(), 10)
		v.ThumbURL = &u
	case current && it.ThumbStatus != nil && *it.ThumbStatus == store.ThumbReady:
		v.Thumb = "ready"
		u := base + "?v=" + strconv.FormatInt(it.ThumbAt.UnixMilli(), 10)
		v.ThumbURL = &u
	case current && it.ThumbStatus != nil && *it.ThumbStatus == store.ThumbFailed:
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
	var err error
	if f.Tags, err = parseIDs(q["tag"]); err != nil {
		return f, "tag must be a number"
	}
	if f.NotTags, err = parseIDs(q["notag"]); err != nil {
		return f, "notag must be a number"
	}
	f.TagAny = q.Get("tags") == "any"
	if v := q.Get("collection"); v != "" {
		if f.Collection, err = strconv.ParseInt(v, 10, 64); err != nil || f.Collection < 1 {
			return f, "collection must be a number"
		}
	}
	f.Unfiled = q.Get("unfiled") == "1"
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
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "collection not found")
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
	v := toView(it)
	d, err := s.loadDetail(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	v.fileDetail = d
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) loadDetail(ctx context.Context, id int64) (*fileDetail, error) {
	meta, err := s.store.GetFileMeta(ctx, id)
	if err != nil {
		return nil, err
	}
	cols, err := s.store.FileCollections(ctx, id)
	if err != nil {
		return nil, err
	}
	queue, err := s.store.FileQueue(ctx, id)
	if err != nil {
		return nil, err
	}
	photos, err := s.store.ListPhotos(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &fileDetail{Meta: meta, Collections: cols, Queue: queue, Photos: make([]photoView, len(photos))}
	for i, p := range photos {
		d.Photos[i] = toPhotoView(p)
	}
	return d, nil
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
	case err == nil && t.Status == store.ThumbFailed && t.RendererVersion == thumbs.Version:
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
	lib := library.FromStore(loc)
	if !within(filepath.Clean(loc.SourcePath), lib.DiskPath()) {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	// Always an attachment with a neutral type, so nothing from the library (SVG, HTML
	// inside a model pack) ever renders in the app's origin.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": loc.Name}))

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
		http.ServeContent(w, r, "", st.ModTime(), f)
		return
	}

	// Archive members stream straight out of the archive (no range requests).
	rc, size, err := library.Open(lib)
	if err != nil {
		w.Header().Del("Content-Disposition")
		writeError(w, http.StatusNotFound, "cannot open this file: "+err.Error())
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	if _, err := io.Copy(w, rc); err != nil && r.Context().Err() == nil {
		slog.Warn("download archive member", "file", loc.ID, "err", err)
	}
}
