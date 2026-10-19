package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/rangoDJ/meshmanager/internal/jobs"
	"github.com/rangoDJ/meshmanager/internal/store"
)

// within reports whether p is root or inside it. Both must be cleaned absolute paths.
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// resolveLibraryPath turns user input into a real directory path inside LIBRARY_ROOT.
func (s *Server) resolveLibraryPath(input string) (string, error) {
	root, err := filepath.EvalSymlinks(s.cfg.LibraryRoot)
	if err != nil {
		return "", fmt.Errorf("LIBRARY_ROOT %s does not exist; mount your model folders there", s.cfg.LibraryRoot)
	}
	p := strings.TrimSpace(input)
	if p == "" {
		return root, nil
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(p))
	if err != nil {
		return "", fmt.Errorf("folder %s does not exist", input)
	}
	if !within(root, real) {
		return "", fmt.Errorf("folder must be inside %s", s.cfg.LibraryRoot)
	}
	st, err := os.Stat(real)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("%s is not a folder", input)
	}
	return real, nil
}

type dirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type dirListing struct {
	Root   string     `json:"root"`
	Path   string     `json:"path"`
	Parent *string    `json:"parent"`
	Dirs   []dirEntry `json:"dirs"`
}

// handleBrowseDirs lists subfolders, for picking a source folder in the UI.
func (s *Server) handleBrowseDirs(w http.ResponseWriter, r *http.Request) {
	p, err := s.resolveLibraryPath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	root, _ := filepath.EvalSymlinks(s.cfg.LibraryRoot)
	entries, err := os.ReadDir(p)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read folder: "+err.Error())
		return
	}
	out := dirListing{Root: root, Path: p, Dirs: []dirEntry{}}
	if p != root {
		parent := filepath.Dir(p)
		out.Parent = &parent
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(p, e.Name())
		if st, err := os.Stat(full); err == nil && st.IsDir() {
			out.Dirs = append(out.Dirs, dirEntry{Name: e.Name(), Path: full})
		}
		if len(out.Dirs) >= 2000 {
			break
		}
	}
	sort.Slice(out.Dirs, func(i, j int) bool {
		return strings.ToLower(out.Dirs[i].Name) < strings.ToLower(out.Dirs[j].Name)
	})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListSources(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type sourceRequest struct {
	Name          *string `json:"name"`
	HostPath      *string `json:"host_path"`
	Path          string  `json:"path"`
	ThumbnailMode *string `json:"thumbnail_mode"`
	Enabled       *bool   `json:"enabled"`
}

func validMode(m *string) bool {
	return m == nil || *m == "lazy" || *m == "eager"
}

func (s *Server) handleCreateSource(w http.ResponseWriter, r *http.Request) {
	var req sourceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validMode(req.ThumbnailMode) {
		writeError(w, http.StatusBadRequest, "thumbnail_mode must be lazy or eager")
		return
	}
	path, err := s.resolveLibraryPath(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing, err := s.store.ListSources(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	for _, e := range existing {
		if within(e.Path, path) || within(path, e.Path) {
			writeError(w, http.StatusConflict, fmt.Sprintf("this folder overlaps with source %q (%s)", e.Name, e.Path))
			return
		}
	}

	name := filepath.Base(path)
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		name = strings.TrimSpace(*req.Name)
	}
	mode := "lazy"
	if req.ThumbnailMode != nil {
		mode = *req.ThumbnailMode
	}
	src, err := s.store.CreateSource(r.Context(), name, path, mode)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "this folder is already a source")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if _, err := jobs.EnqueueScan(r.Context(), s.store, s.queue, src); err != nil {
		internalError(w, r, err)
		return
	}
	if src, err = s.store.GetSource(r.Context(), src.ID); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, src)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

func (s *Server) handleUpdateSource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req sourceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Path != "" {
		writeError(w, http.StatusBadRequest, "a source's folder can't be changed; add a new source instead")
		return
	}
	if !validMode(req.ThumbnailMode) {
		writeError(w, http.StatusBadRequest, "thumbnail_mode must be lazy or eager")
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "name can't be empty")
			return
		}
		req.Name = &trimmed
	}
	if req.HostPath != nil {
		trimmed := strings.TrimSpace(*req.HostPath)
		if len(trimmed) > 1024 {
			writeError(w, http.StatusBadRequest, "host path is too long")
			return
		}
		req.HostPath = &trimmed
	}
	src, err := s.store.UpdateSource(r.Context(), id, store.SourceUpdate{
		Name: req.Name, HostPath: req.HostPath, ThumbnailMode: req.ThumbnailMode, Enabled: req.Enabled,
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "source not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, src)
}

// handleDeleteSource removes a source from the index. Files on disk are never touched;
// the source's thumbnails are deleted in the background.
func (s *Server) handleDeleteSource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := s.store.InTx(r.Context(), func(q *store.Store, tx pgx.Tx) error {
		if err := q.DeleteSource(r.Context(), id); err != nil {
			return err
		}
		_, err := s.queue.InsertTx(r.Context(), tx, jobs.PurgeThumbnailsArgs{SourceID: id}, nil)
		return err
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "source not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleScanSource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	src, err := s.store.GetSource(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "source not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	job, err := jobs.EnqueueScan(r.Context(), s.store, s.queue, src)
	if errors.Is(err, jobs.ErrScanInProgress) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

type folderNode struct {
	Name     string        `json:"name"`
	Path     string        `json:"path"`
	Count    int64         `json:"count"` // files in this folder and below
	Children []*folderNode `json:"children"`
}

// handleSourceTree returns the folder tree of a source with recursive file counts.
func (s *Server) handleSourceTree(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	src, err := s.store.GetSource(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "source not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	counts, err := s.store.DirCounts(r.Context(), id, r.URL.Query()["kind"])
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, buildTree(src.Name, counts))
}

func buildTree(rootName string, counts []store.DirCount) *folderNode {
	root := &folderNode{Name: rootName, Path: "", Children: []*folderNode{}}
	nodes := map[string]*folderNode{"": root}
	var get func(p string) *folderNode
	get = func(p string) *folderNode {
		if n, ok := nodes[p]; ok {
			return n
		}
		parentPath := ""
		name := p
		if i := strings.LastIndexByte(p, '/'); i >= 0 {
			parentPath, name = p[:i], p[i+1:]
		}
		n := &folderNode{Name: name, Path: p, Children: []*folderNode{}}
		nodes[p] = n
		parent := get(parentPath)
		parent.Children = append(parent.Children, n)
		return n
	}
	for _, c := range counts {
		// Add the count to the folder and every ancestor.
		for p := c.Dir; ; {
			get(p).Count += c.Count
			if p == "" {
				break
			}
			if i := strings.LastIndexByte(p, '/'); i >= 0 {
				p = p[:i]
			} else {
				p = ""
			}
		}
	}
	var sortAll func(n *folderNode)
	sortAll = func(n *folderNode) {
		sort.Slice(n.Children, func(i, j int) bool {
			return strings.ToLower(n.Children[i].Name) < strings.ToLower(n.Children[j].Name)
		})
		for _, c := range n.Children {
			sortAll(c)
		}
	}
	sortAll(root)
	return root
}
