package api

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"

	"github.com/rangoDJ/meshmanager/internal/gcode"
	"github.com/rangoDJ/meshmanager/internal/library"
)

// toolpathVersion is part of the ETag; bump it when the payload or parser changes.
const toolpathVersion = 1

type toolpathHeader struct {
	Version       int           `json:"version"`
	Layers        []gcode.Layer `json:"layers"`
	Features      []string      `json:"features"`
	Extrusions    int           `json:"extrusions"`
	Travels       int           `json:"travels"`
	Min           [3]float64    `json:"min"`
	Max           [3]float64    `json:"max"`
	Slicer        string        `json:"slicer"`
	PrintSeconds  float64       `json:"print_seconds"`
	FilamentGrams float64       `json:"filament_grams"`
	FilamentMM    float64       `json:"filament_mm"`
}

// handleToolpath streams a sliced G-code file's toolpath for the layer preview:
//
//	"MMT1" | uint32 LE header length | header JSON (space-padded to 4 bytes) |
//	float32 LE × 4 per extrusion (x1 y1 x2 y2) | float32 LE × 4 per travel |
//	uint8 feature per extrusion (padded to 4 bytes)
//
// Each layer gives the Z and the ranges of its extrusions and travels.
func (s *Server) handleToolpath(w http.ResponseWriter, r *http.Request) {
	loc, ok := s.fileLocation(w, r)
	if !ok {
		return
	}
	if loc.Format != "gcode" {
		writeError(w, http.StatusNotFound, "no layer preview for this format")
		return
	}
	etag := fmt.Sprintf(`"t%d-%d-%d"`, toolpathVersion, loc.Size, loc.Mtime.UnixMicro())
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

	rc, _, err := library.Open(library.FromStore(loc))
	if err != nil {
		writeError(w, http.StatusNotFound, "cannot open this file: "+err.Error())
		return
	}
	tp, err := gcode.Parse(rc)
	rc.Close()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "cannot read this G-code: "+err.Error())
		return
	}

	h := toolpathHeader{
		Version: toolpathVersion, Layers: tp.Layers, Features: gcode.Features,
		Extrusions: len(tp.Feature), Travels: len(tp.Travels) / 4, Min: tp.Min, Max: tp.Max,
		Slicer: tp.Slicer, PrintSeconds: tp.PrintSeconds, FilamentGrams: tp.FilamentGrams, FilamentMM: tp.FilamentMM,
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	var out io.Writer = w
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gz, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
		defer gz.Close()
		out = gz
	}
	if err := writeToolpath(out, h, tp); err != nil && r.Context().Err() == nil {
		slog.Warn("write toolpath", "file", loc.ID, "err", err)
	}
}

func writeToolpath(w io.Writer, h toolpathHeader, tp *gcode.Toolpath) error {
	header, err := json.Marshal(h)
	if err != nil {
		return err
	}
	for len(header)%4 != 0 {
		header = append(header, ' ')
	}
	bw := bufio.NewWriterSize(w, 256<<10)
	var prefix [8]byte
	copy(prefix[:4], "MMT1")
	binary.LittleEndian.PutUint32(prefix[4:], uint32(len(header)))
	if _, err := bw.Write(prefix[:]); err != nil {
		return err
	}
	if _, err := bw.Write(header); err != nil {
		return err
	}
	var buf [4]byte
	for _, list := range [][]float32{tp.Extrusions, tp.Travels} {
		for _, f := range list {
			binary.LittleEndian.PutUint32(buf[:], math.Float32bits(f))
			if _, err := bw.Write(buf[:]); err != nil {
				return err
			}
		}
	}
	if _, err := bw.Write(tp.Feature); err != nil {
		return err
	}
	if pad := (4 - len(tp.Feature)%4) % 4; pad > 0 {
		if _, err := bw.Write(make([]byte, pad)); err != nil {
			return err
		}
	}
	return bw.Flush()
}
