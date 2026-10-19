package api

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"

	"github.com/rangoDJ/meshmanager/internal/formats"
	"github.com/rangoDJ/meshmanager/internal/library"
	"github.com/rangoDJ/meshmanager/internal/mesh"
	"github.com/rangoDJ/meshmanager/internal/store"
)

// geometryVersion is part of the ETag; bump it when the payload format or loaders change.
const geometryVersion = 1

// geometryHeader describes the triangles that follow it in a geometry response.
type geometryHeader struct {
	Version    int          `json:"version"`
	Triangles  int          `json:"triangles"`
	Parts      []mesh.Part  `json:"parts"`
	Plates     []mesh.Plate `json:"plates"`
	Min        [3]float64   `json:"min"`
	Max        [3]float64   `json:"max"`
	VolumeMM3  float64      `json:"volume_mm3"`
	AreaMM2    float64      `json:"area_mm2"`
	Watertight *bool        `json:"watertight"`
}

// handleGeometry streams a model for the 3D viewer:
//
//	"MMG1" | uint32 LE header length | header JSON (space-padded to 4 bytes) |
//	float32 LE × 9 per triangle (Z-up, mm)
//
// Parts are contiguous triangle ranges. Conversion happens on demand; browsers cache the
// result and revalidate with the ETag.
func (s *Server) handleGeometry(w http.ResponseWriter, r *http.Request) {
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
	if !formats.CanView(loc.Format) {
		writeError(w, http.StatusNotFound, "no 3D view for this format")
		return
	}

	etag := fmt.Sprintf(`"g%d-%d-%d"`, geometryVersion, loc.Size, loc.Mtime.UnixMicro())
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("Vary", "Accept-Encoding")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	select {
	case s.geometrySlots <- struct{}{}:
		defer func() { <-s.geometrySlots }()
	case <-r.Context().Done():
		return
	}

	full, cleanup, err := library.Materialize(library.FromStore(loc), library.TempDir(s.cfg.DataDir))
	if err != nil {
		writeError(w, http.StatusNotFound, "cannot open this file: "+err.Error())
		return
	}
	defer cleanup()
	m, err := mesh.LoadModel(full, loc.Format)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "cannot read this model: "+err.Error())
		return
	}

	st := m.Stats()
	h := geometryHeader{
		Version: geometryVersion, Triangles: m.Triangles(), Parts: m.Parts, Plates: m.Plates,
		Min: st.Min, Max: st.Max, VolumeMM3: st.Volume, AreaMM2: st.Area,
	}
	if h.Plates == nil {
		h.Plates = []mesh.Plate{}
	}
	if closed, checked := m.Watertight(); checked {
		h.Watertight = &closed
	}
	// Keep the stored stats in step with what the viewer shows.
	if err := s.store.SaveModelInfo(r.Context(), store.ModelInfo{
		FileID: loc.ID, Triangles: int64(st.Triangles), Size: st.Size(), Volume: st.Volume,
		Area: st.Area, Watertight: h.Watertight,
	}); err != nil && r.Context().Err() == nil {
		slog.Warn("save model info", "file", loc.ID, "err", err)
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	var out io.Writer = w
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gz, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
		defer gz.Close()
		out = gz
	}
	if err := writeGeometry(out, h, m.Tris); err != nil && r.Context().Err() == nil {
		slog.Warn("write geometry", "file", loc.ID, "err", err)
	}
}

func writeGeometry(w io.Writer, h geometryHeader, tris []float32) error {
	header, err := json.Marshal(h)
	if err != nil {
		return err
	}
	for len(header)%4 != 0 {
		header = append(header, ' ')
	}
	bw := bufio.NewWriterSize(w, 256<<10)
	var prefix [8]byte
	copy(prefix[:4], "MMG1")
	binary.LittleEndian.PutUint32(prefix[4:], uint32(len(header)))
	if _, err := bw.Write(prefix[:]); err != nil {
		return err
	}
	if _, err := bw.Write(header); err != nil {
		return err
	}
	var buf [4]byte
	for _, f := range tris {
		binary.LittleEndian.PutUint32(buf[:], math.Float32bits(f))
		if _, err := bw.Write(buf[:]); err != nil {
			return err
		}
	}
	return bw.Flush()
}
