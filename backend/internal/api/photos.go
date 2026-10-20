package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/rangoDJ/meshmanager/internal/store"
	"github.com/rangoDJ/meshmanager/internal/thumbs"
)

const (
	maxPhotoBytes = 20 << 20
	photoThumbPx  = 512
)

// photoTypes are the picture formats accepted for upload, by sniffed content type.
var photoTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

type photoView struct {
	store.Photo
	URL      string `json:"url"`
	ThumbURL string `json:"thumb_url"`
}

// photoURL addresses a stored photo: the full picture, or the smaller copy used on tiles.
func photoURL(id int64, thumb bool) string {
	size := "full"
	if thumb {
		size = "thumb"
	}
	return "/api/photos/" + strconv.FormatInt(id, 10) + "/image?size=" + size
}

func toPhotoView(p store.Photo) photoView {
	v := "&v=" + strconv.FormatInt(p.CreatedAt.UnixMilli(), 10)
	return photoView{Photo: p, URL: photoURL(p.ID, false) + v, ThumbURL: photoURL(p.ID, true) + v}
}

func randomName() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) removeStored(rel ...string) {
	for _, p := range rel {
		if p == "" {
			continue
		}
		full := filepath.Join(s.cfg.DataDir, filepath.FromSlash(p))
		if !within(filepath.Clean(s.cfg.DataDir), full) {
			continue
		}
		if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("remove photo file", "path", p, "err", err)
		}
	}
}

// handleUploadPhoto attaches a picture to a file.
func (s *Server) handleUploadPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := s.store.GetFile(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "file not found")
		return
	} else if err != nil {
		internalError(w, r, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes+(1<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "that picture is too big (the limit is 20 MB)")
		} else {
			writeError(w, http.StatusBadRequest, "send the picture as a form upload named photo")
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, _, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "send the picture as a form upload named photo")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPhotoBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read the upload")
		return
	}
	if len(data) > maxPhotoBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "that picture is too big (the limit is 20 MB)")
		return
	}
	ext, allowed := photoTypes[http.DetectContentType(data)]
	if !allowed {
		writeError(w, http.StatusUnsupportedMediaType, "use a JPEG, PNG, GIF or WebP picture")
		return
	}
	width, height, thumb, thumbType, err := thumbs.Photo(data, photoThumbPx)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	name := randomName()
	rel := path.Join("photos", name+ext)
	thumbRel := path.Join("photos", name+".t"+thumbs.Ext(thumbType))
	if err := os.MkdirAll(filepath.Join(s.cfg.DataDir, "photos"), 0o755); err != nil {
		internalError(w, r, err)
		return
	}
	if err := os.WriteFile(filepath.Join(s.cfg.DataDir, filepath.FromSlash(rel)), data, 0o644); err != nil {
		internalError(w, r, err)
		return
	}
	if err := os.WriteFile(filepath.Join(s.cfg.DataDir, filepath.FromSlash(thumbRel)), thumb, 0o644); err != nil {
		s.removeStored(rel)
		internalError(w, r, err)
		return
	}
	photo, err := s.store.AddPhoto(r.Context(), store.Photo{
		FileID: id, Path: rel, ThumbPath: thumbRel, ThumbType: thumbType,
		ContentType: http.DetectContentType(data), Width: width, Height: height, Size: int64(len(data)),
	})
	if err != nil {
		s.removeStored(rel, thumbRel)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toPhotoView(photo))
}

// handleGetPhoto serves a stored picture; size=thumb gives the smaller copy.
func (s *Server) handleGetPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := s.store.GetPhoto(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "photo not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	rel, ct := p.Path, p.ContentType
	if r.URL.Query().Get("size") == "thumb" {
		rel, ct = p.ThumbPath, p.ThumbType
	}
	full := filepath.Join(s.cfg.DataDir, filepath.FromSlash(rel))
	if !within(filepath.Clean(s.cfg.DataDir), full) {
		writeError(w, http.StatusNotFound, "photo not found")
		return
	}
	f, err := os.Open(full)
	if err != nil {
		writeError(w, http.StatusNotFound, "the photo file is missing")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", ct)
	if r.URL.Query().Has("v") {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
	}
	http.ServeContent(w, r, "", p.CreatedAt, f)
}

func (s *Server) handleUpdatePhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Cover *bool `json:"cover"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Cover == nil {
		writeError(w, http.StatusBadRequest, "nothing to change")
		return
	}
	p, err := s.store.SetPhotoCover(r.Context(), id, *req.Cover)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "photo not found")
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, toPhotoView(p))
	}
}

func (s *Server) handleDeletePhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	paths, err := s.store.DeletePhoto(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "photo not found")
	case err != nil:
		internalError(w, r, err)
	default:
		s.removeStored(paths...)
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- folder cards ---------------------------------------------------------------

type folderView struct {
	store.Folder
	Cover *fileView `json:"cover"`
}

// handleListFolders returns the folders directly inside a folder, each with its cover.
func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := s.store.GetSource(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "source not found")
		return
	} else if err != nil {
		internalError(w, r, err)
		return
	}
	folders, err := s.store.ListFolders(r.Context(), id, r.URL.Query().Get("dir"), r.URL.Query()["kind"])
	if err != nil {
		internalError(w, r, err)
		return
	}
	var coverIDs []int64
	for _, f := range folders {
		if f.CoverID != nil {
			coverIDs = append(coverIDs, *f.CoverID)
		}
	}
	covers, err := s.store.FilesByIDs(r.Context(), coverIDs)
	if err != nil {
		internalError(w, r, err)
		return
	}
	byID := make(map[int64]fileView, len(covers))
	for _, c := range covers {
		byID[c.ID] = toView(c)
	}
	out := make([]folderView, len(folders))
	for i, f := range folders {
		out[i] = folderView{Folder: f}
		if f.CoverID != nil {
			if v, ok := byID[*f.CoverID]; ok {
				out[i].Cover = &v
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSetFolderCover(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Dir    string `json:"dir"`
		FileID int64  `json:"file_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Dir == "" || req.FileID < 1 {
		writeError(w, http.StatusBadRequest, "a folder and a file are needed")
		return
	}
	err := s.store.SetFolderCover(r.Context(), id, req.Dir, req.FileID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "that file isn't in this source")
	case err != nil:
		internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleClearFolderCover(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		writeError(w, http.StatusBadRequest, "a folder is needed")
		return
	}
	if err := s.store.ClearFolderCover(r.Context(), id, dir); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
