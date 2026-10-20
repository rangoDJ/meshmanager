package api

import (
	"encoding/csv"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/rangoDJ/meshmanager/internal/store"
)

func validListName(w http.ResponseWriter, name string) bool {
	n := strings.TrimSpace(name)
	if n == "" || len(n) > 100 {
		writeError(w, http.StatusBadRequest, "a list name needs 1 to 100 characters")
		return false
	}
	return true
}

func (s *Server) handleListPrintLists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.store.ListPrintLists(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	if lists == nil {
		lists = []store.PrintList{}
	}
	writeJSON(w, http.StatusOK, lists)
}

func (s *Server) handleCreatePrintList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Printer string `json:"printer"`
	}
	if !decodeJSON(w, r, &req) || !validListName(w, req.Name) {
		return
	}
	if len(req.Printer) > 100 {
		writeError(w, http.StatusBadRequest, "the printer name is too long")
		return
	}
	l, err := s.store.CreatePrintList(r.Context(), req.Name, req.Printer)
	switch {
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "there is already a list with that name")
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusCreated, l)
	}
}

func (s *Server) handleUpdatePrintList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name    *string `json:"name"`
		Printer *string `json:"printer"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != nil && !validListName(w, *req.Name) {
		return
	}
	if req.Printer != nil && len(*req.Printer) > 100 {
		writeError(w, http.StatusBadRequest, "the printer name is too long")
		return
	}
	l, err := s.store.UpdatePrintList(r.Context(), id, req.Name, req.Printer)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "list not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "there is already a list with that name")
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, l)
	}
}

func (s *Server) handleDeletePrintList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := s.store.DeletePrintList(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "list not found")
	case err != nil:
		internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

type printItemView struct {
	store.PrintItem
	File fileView `json:"file"`
}

func (s *Server) loadPrintItems(w http.ResponseWriter, r *http.Request, listID int64) (store.PrintList, []store.PrintItem, bool) {
	list, err := s.store.GetPrintList(r.Context(), listID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "list not found")
		return list, nil, false
	}
	if err != nil {
		internalError(w, r, err)
		return list, nil, false
	}
	items, err := s.store.ListPrintItems(r.Context(), listID)
	if err != nil {
		internalError(w, r, err)
		return list, nil, false
	}
	return list, items, true
}

func (s *Server) handleListPrintItems(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, items, ok := s.loadPrintItems(w, r, id)
	if !ok {
		return
	}
	out := make([]printItemView, len(items))
	for i, it := range items {
		out[i] = printItemView{PrintItem: it, File: toView(it.File)}
	}
	writeJSON(w, http.StatusOK, map[string]any{"list": list, "items": out})
}

func (s *Server) handleAddPrintItems(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		selection
		Quantity int `json:"quantity"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Quantity < 0 || req.Quantity > 999 {
		writeError(w, http.StatusBadRequest, "quantity must be between 1 and 999")
		return
	}
	ids, ok := s.resolveSelection(w, r, req.selection)
	if !ok {
		return
	}
	added, err := s.store.AddPrintItems(r.Context(), id, ids, req.Quantity)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "list not found")
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"files": len(ids), "added": added})
	}
}

func (s *Server) handleUpdatePrintItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Status   *string  `json:"status"`
		Quantity *int     `json:"quantity"`
		Note     *string  `json:"note"`
		Position *float64 `json:"position"`
		ListID   *int64   `json:"list_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	switch {
	case req.Status != nil && !store.ValidPrintStatus(*req.Status):
		writeError(w, http.StatusBadRequest, "status must be todo, printing or done")
		return
	case req.Quantity != nil && (*req.Quantity < 1 || *req.Quantity > 999):
		writeError(w, http.StatusBadRequest, "quantity must be between 1 and 999")
		return
	case req.Note != nil && len(*req.Note) > 1000:
		writeError(w, http.StatusBadRequest, "the note is too long")
		return
	}
	err := s.store.UpdatePrintItem(r.Context(), id, store.PrintItemUpdate{
		Status: req.Status, Quantity: req.Quantity, Note: req.Note, Position: req.Position, ListID: req.ListID,
	})
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "item not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "that model is already on the other list")
	case err != nil:
		internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleDeletePrintItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := s.store.DeletePrintItem(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "item not found")
	case err != nil:
		internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleClearDonePrintItems(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	n, err := s.store.ClearDonePrintItems(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": n})
}

// handleExportPrintList sends a list as a spreadsheet-friendly CSV.
func (s *Server) handleExportPrintList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, items, ok := s.loadPrintItems(w, r, id)
	if !ok {
		return
	}
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, list.Name)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name + ".csv"}))

	out := csv.NewWriter(w)
	_, _ = w.Write([]byte("\xEF\xBB\xBF")) // so Excel reads it as UTF-8
	_ = out.Write([]string{"Order", "File", "Quantity", "Status", "Format", "Size (mm)", "Location", "Note"})
	for i, it := range items {
		f := it.File
		size := ""
		if f.Dimensions != nil {
			size = fmt.Sprintf("%.1f × %.1f × %.1f", f.Dimensions[0], f.Dimensions[1], f.Dimensions[2])
		}
		loc := f.SourceName + " / " + f.RelPath
		if v := toView(f); v.HostPath != nil {
			loc = *v.HostPath
		}
		_ = out.Write([]string{
			strconv.Itoa(i + 1), csvSafe(f.Name), strconv.Itoa(it.Quantity), it.Status, strings.ToUpper(f.Format),
			size, csvSafe(loc), csvSafe(it.Note),
		})
	}
	out.Flush()
}

// csvSafe stops a cell from being read as a formula when the CSV is opened in a spreadsheet.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
