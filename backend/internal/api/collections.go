package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rangoDJ/meshmanager/internal/store"
)

// rulesBody is a smart collection's saved filter, as sent by the client.
type rulesBody struct {
	Tags     []int64  `json:"tags"`
	TagAny   bool     `json:"tag_any"`
	NotTags  []int64  `json:"not_tags"`
	Query    string   `json:"q"`
	Kinds    []string `json:"kinds"`
	Formats  []string `json:"formats"`
	SourceID int64    `json:"source_id"`
	Dir      string   `json:"dir"`
}

func (b rulesBody) rules() store.Rules {
	return store.Rules{
		Tags: b.Tags, TagAny: b.TagAny, NotTags: b.NotTags, Query: strings.TrimSpace(b.Query),
		Kinds: b.Kinds, Formats: b.Formats, SourceID: b.SourceID, Dir: b.Dir,
	}
}

func validCollectionName(w http.ResponseWriter, name string) bool {
	n := strings.TrimSpace(name)
	if n == "" || len(n) > 100 {
		writeError(w, http.StatusBadRequest, "a collection name needs 1 to 100 characters")
		return false
	}
	return true
}

func (s *Server) handleListCollections(w http.ResponseWriter, r *http.Request) {
	cols, err := s.store.ListCollections(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	if cols == nil {
		cols = []store.Collection{}
	}
	writeJSON(w, http.StatusOK, cols)
}

func (s *Server) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string     `json:"name"`
		ParentID *int64     `json:"parent_id"`
		Rules    *rulesBody `json:"rules"`
	}
	if !decodeJSON(w, r, &req) || !validCollectionName(w, req.Name) {
		return
	}
	var rules *store.Rules
	if req.Rules != nil {
		rr := req.Rules.rules()
		if rr.Empty() {
			writeError(w, http.StatusBadRequest, "a smart collection needs at least one rule, such as a tag or a search")
			return
		}
		rules = &rr
	}
	c, err := s.store.CreateCollection(r.Context(), req.Name, req.ParentID, rules)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "parent collection not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "there is already a collection with that name here")
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusCreated, c)
	}
}

func (s *Server) handleUpdateCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// parent_id may be null (move to the top level), so its presence has to be detected.
	var req struct {
		Name     *string         `json:"name"`
		ParentID json.RawMessage `json:"parent_id"`
		Rules    *rulesBody      `json:"rules"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	var u store.CollectionUpdate
	if req.Name != nil {
		if !validCollectionName(w, *req.Name) {
			return
		}
		u.Name = req.Name
	}
	if req.ParentID != nil {
		u.ParentSet = true
		if string(req.ParentID) != "null" {
			var pid int64
			if err := json.Unmarshal(req.ParentID, &pid); err != nil || pid < 1 {
				writeError(w, http.StatusBadRequest, "parent_id must be a number or null")
				return
			}
			u.Parent = &pid
		}
	}
	if req.Rules != nil {
		rr := req.Rules.rules()
		if rr.Empty() {
			writeError(w, http.StatusBadRequest, "a smart collection needs at least one rule, such as a tag or a search")
			return
		}
		u.Rules = &rr
	}
	c, err := s.store.UpdateCollection(r.Context(), id, u)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "collection not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "that name is taken here, a collection can't go inside itself, and a hand-filled collection can't take rules")
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, c)
	}
}

func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteCollection(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "collection not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "rename one of the collections inside it first: two would end up with the same name")
	case err != nil:
		internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleAddToCollection(w http.ResponseWriter, r *http.Request) {
	s.changeCollection(w, r, true)
}

func (s *Server) handleRemoveFromCollection(w http.ResponseWriter, r *http.Request) {
	s.changeCollection(w, r, false)
}

func (s *Server) changeCollection(w http.ResponseWriter, r *http.Request, add bool) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var sel selection
	if !decodeJSON(w, r, &sel) {
		return
	}
	ids, ok := s.resolveSelection(w, r, sel)
	if !ok {
		return
	}
	var added int64
	var err error
	if add {
		added, err = s.store.AddToCollection(r.Context(), id, ids)
	} else {
		err = s.store.RemoveFromCollection(r.Context(), id, ids)
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "collection not found")
	case errors.Is(err, store.ErrSmartCollection):
		writeError(w, http.StatusConflict, "a smart collection fills itself from its rules; change the rules or tag the files instead")
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"files": len(ids), "added": added})
	}
}
