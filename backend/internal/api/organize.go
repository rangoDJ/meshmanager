package api

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rangoDJ/meshmanager/internal/store"
)

// maxBulk caps how many files one bulk action touches.
const maxBulk = 10000

func parseIDs(vals []string) ([]int64, error) {
	var out []int64
	for _, v := range vals {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id < 1 {
			return nil, errors.New("invalid id")
		}
		out = append(out, id)
	}
	return out, nil
}

// filterBody is a file filter in a request body, mirroring the query parameters of GET /api/files.
type filterBody struct {
	Source     int64    `json:"source"`
	Dir        string   `json:"dir"`
	Kinds      []string `json:"kinds"`
	Formats    []string `json:"formats"`
	Q          string   `json:"q"`
	Tags       []int64  `json:"tags"`
	TagAny     bool     `json:"tag_any"`
	NotTags    []int64  `json:"not_tags"`
	Collection int64    `json:"collection"`
	Unfiled    bool     `json:"unfiled"`
}

func (b filterBody) filter() store.FileFilter {
	f := store.FileFilter{
		SourceID: b.Source, Kinds: b.Kinds, Formats: b.Formats, Query: b.Q,
		Tags: b.Tags, TagAny: b.TagAny, NotTags: b.NotTags, Collection: b.Collection, Unfiled: b.Unfiled,
	}
	if b.Source > 0 {
		f.Dir = b.Dir
	}
	return f
}

// selection names the files an action applies to: some ids, or everything a filter matches.
type selection struct {
	IDs    []int64     `json:"ids"`
	Filter *filterBody `json:"filter"`
}

// resolveSelection returns the file ids, or writes the error response and returns false.
func (s *Server) resolveSelection(w http.ResponseWriter, r *http.Request, sel selection) ([]int64, bool) {
	var ids []int64
	switch {
	case sel.Filter != nil:
		var err error
		ids, err = s.store.FileIDs(r.Context(), sel.Filter.filter(), maxBulk)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "collection not found")
			return nil, false
		}
		if err != nil {
			internalError(w, r, err)
			return nil, false
		}
	default:
		ids = sel.IDs
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "no files selected")
		return nil, false
	}
	if len(ids) > maxBulk {
		writeError(w, http.StatusBadRequest, "too many files at once (the limit is "+strconv.Itoa(maxBulk)+")")
		return nil, false
	}
	return ids, true
}

// --- tags ----------------------------------------------------------------------

type tagRequest struct {
	Name   *string `json:"name"`
	Color  *string `json:"color"`
	Pinned *bool   `json:"pinned"`
}

func validTagName(w http.ResponseWriter, name string) bool {
	n := strings.TrimSpace(name)
	if n == "" || len(n) > 60 {
		writeError(w, http.StatusBadRequest, "a tag name needs 1 to 60 characters")
		return false
	}
	return true
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListTags(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	if tags == nil {
		tags = []store.Tag{}
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var req tagRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == nil {
		writeError(w, http.StatusBadRequest, "a tag needs a name")
		return
	}
	if !validTagName(w, *req.Name) {
		return
	}
	color := ""
	if req.Color != nil {
		color = *req.Color
		if !store.ValidTagColor(color) {
			writeError(w, http.StatusBadRequest, "unknown colour")
			return
		}
	}
	tag, err := s.store.CreateTag(r.Context(), *req.Name, color, req.Pinned != nil && *req.Pinned)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "a tag with that name already exists")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, tag)
}

func (s *Server) handleUpdateTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req tagRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != nil && !validTagName(w, *req.Name) {
		return
	}
	if req.Color != nil && !store.ValidTagColor(*req.Color) {
		writeError(w, http.StatusBadRequest, "unknown colour")
		return
	}
	tag, err := s.store.UpdateTag(r.Context(), id, store.TagUpdate{Name: req.Name, Color: req.Color, Pinned: req.Pinned})
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "tag not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "a tag with that name already exists")
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, tag)
	}
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteTag(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "tag not found")
	case err != nil:
		internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleMergeTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Into int64 `json:"into"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	err := s.store.MergeTags(r.Context(), id, req.Into)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "tag not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusBadRequest, "pick a different tag to merge into")
	case err != nil:
		internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleBulkTags adds and removes tags on the selected files. Tags named in add_names
// are created when they don't exist yet.
func (s *Server) handleBulkTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		selection
		AddIDs    []int64  `json:"add_ids"`
		AddNames  []string `json:"add_names"`
		RemoveIDs []int64  `json:"remove_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.AddIDs)+len(req.AddNames)+len(req.RemoveIDs) == 0 {
		writeError(w, http.StatusBadRequest, "nothing to change")
		return
	}
	for _, n := range req.AddNames {
		if !validTagName(w, n) {
			return
		}
	}
	ids, ok := s.resolveSelection(w, r, req.selection)
	if !ok {
		return
	}
	added, err := s.store.ApplyTags(r.Context(), ids, req.AddIDs, req.AddNames, req.RemoveIDs)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": len(ids), "added": added})
}

// --- notes and provenance --------------------------------------------------------

func (s *Server) handleUpdateMeta(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Notes     *string `json:"notes"`
		SourceURL *string `json:"source_url"`
		Designer  *string `json:"designer"`
		License   *string `json:"license"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	limits := []struct {
		name string
		v    *string
		max  int
	}{{"notes", req.Notes, 10000}, {"source URL", req.SourceURL, 2000}, {"designer", req.Designer, 200}, {"licence", req.License, 200}}
	for _, l := range limits {
		if l.v != nil && len(*l.v) > l.max {
			writeError(w, http.StatusBadRequest, "the "+l.name+" is too long")
			return
		}
	}
	if req.SourceURL != nil {
		u := strings.TrimSpace(*req.SourceURL)
		req.SourceURL = &u
		// The address is shown as a link, so only web addresses are allowed.
		if parsed, err := url.Parse(u); u != "" && (err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "") {
			writeError(w, http.StatusBadRequest, "the source URL must be a web address starting with http:// or https://")
			return
		}
	}
	trim := func(p *string) *string {
		if p == nil {
			return nil
		}
		v := strings.TrimSpace(*p)
		return &v
	}
	meta, err := s.store.SetFileMeta(r.Context(), id, store.MetaUpdate{
		Notes: req.Notes, SourceURL: req.SourceURL, Designer: trim(req.Designer), License: trim(req.License),
	})
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "file not found")
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, meta)
	}
}
