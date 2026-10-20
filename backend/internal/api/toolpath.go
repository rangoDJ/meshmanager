package api

import (
	"archive/zip"
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rangoDJ/meshmanager/internal/formats"
	"github.com/rangoDJ/meshmanager/internal/gcode"
	"github.com/rangoDJ/meshmanager/internal/library"
	"github.com/rangoDJ/meshmanager/internal/store"
)

// toolpathVersion is part of the ETag; bump it when the payload or parser changes.
const toolpathVersion = 2

// maxPlateGCode bounds one plate's G-code after decompression.
const maxPlateGCode = 2 << 30

type toolpathHeader struct {
	// Plates lists the sliced plates of a .gcode.3mf (empty for plain G-code); Plate is
	// the one these toolpaths belong to.
	Plates        []int         `json:"plates"`
	Plate         int           `json:"plate"`
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
	if loc.Format != "gcode" && loc.Format != formats.GCode3MF {
		writeError(w, http.StatusNotFound, "no layer preview for this format")
		return
	}
	wantPlate := 0 // 0: the first plate
	if v := r.URL.Query().Get("plate"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "plate must be a positive number")
			return
		}
		wantPlate = n
	}
	etag := fmt.Sprintf(`"t%d-%d-%d-p%d"`, toolpathVersion, loc.Size, loc.Mtime.UnixMicro(), wantPlate)
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

	var tp *gcode.Toolpath
	plates, plate := []int{}, 0
	if loc.Format == formats.GCode3MF {
		var status int
		var err error
		if tp, plates, plate, status, err = s.plateToolpath(loc, wantPlate); err != nil {
			writeError(w, status, err.Error())
			return
		}
	} else {
		rc, _, err := library.Open(library.FromStore(loc))
		if err != nil {
			writeError(w, http.StatusNotFound, "cannot open this file: "+err.Error())
			return
		}
		tp, err = gcode.Parse(rc)
		rc.Close()
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "cannot read this G-code: "+err.Error())
			return
		}
	}

	h := toolpathHeader{
		Version: toolpathVersion, Layers: tp.Layers, Features: gcode.Features,
		Extrusions: len(tp.Feature), Travels: len(tp.Travels) / 4, Min: tp.Min, Max: tp.Max,
		Slicer: tp.Slicer, PrintSeconds: tp.PrintSeconds, FilamentGrams: tp.FilamentGrams, FilamentMM: tp.FilamentMM,
		Plates: plates, Plate: plate,
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

var plateGCodeRe = regexp.MustCompile(`(?i)^metadata/plate_(\d+)\.gcode$`)

// plateToolpath parses one plate of a Bambu Studio / OrcaSlicer `.gcode.3mf`: a ZIP with
// Metadata/plate_N.gcode per sliced plate and Metadata/slice_info.config holding the
// slicer's time and filament estimates. want is the plate number, or 0 for the first.
// On failure it returns the HTTP status to answer with.
func (s *Server) plateToolpath(loc store.FileLocation, want int) (tp *gcode.Toolpath, plates []int, plate, status int, err error) {
	full, cleanup, err := library.Materialize(library.FromStore(loc), library.TempDir(s.cfg.DataDir))
	if err != nil {
		return nil, nil, 0, http.StatusNotFound, errors.New("cannot open this file: " + err.Error())
	}
	defer cleanup()
	zr, err := zip.OpenReader(full)
	if err != nil {
		return nil, nil, 0, http.StatusUnprocessableEntity, errors.New("cannot read this file as a 3MF: " + err.Error())
	}
	defer zr.Close()

	members := map[int]*zip.File{}
	var info *zip.File
	for _, f := range zr.File {
		if m := plateGCodeRe.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			members[n] = f
		} else if strings.EqualFold(f.Name, "Metadata/slice_info.config") {
			info = f
		}
	}
	for n := range members {
		plates = append(plates, n)
	}
	slices.Sort(plates)
	if len(plates) == 0 {
		return nil, nil, 0, http.StatusUnprocessableEntity, errors.New("this 3MF holds no sliced plates")
	}
	plate = want
	if plate == 0 {
		plate = plates[0]
	}
	f := members[plate]
	if f == nil {
		return nil, nil, 0, http.StatusNotFound, fmt.Errorf("plate %d is not in this file", plate)
	}
	if f.UncompressedSize64 > maxPlateGCode {
		return nil, nil, 0, http.StatusUnprocessableEntity, errors.New("this plate's G-code is too large to preview")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, nil, 0, http.StatusUnprocessableEntity, errors.New("cannot read this plate: " + err.Error())
	}
	tp, err = gcode.Parse(rc)
	rc.Close()
	if err != nil {
		return nil, nil, 0, http.StatusUnprocessableEntity, errors.New("cannot read this G-code: " + err.Error())
	}

	if info != nil {
		est := readSliceInfo(info)[plate]
		if tp.PrintSeconds == 0 {
			tp.PrintSeconds = est.seconds
		}
		if tp.FilamentGrams == 0 {
			tp.FilamentGrams = est.grams
		}
		if tp.FilamentMM == 0 {
			tp.FilamentMM = est.mm
		}
	}
	return tp, plates, plate, 0, nil
}

type plateEstimate struct{ seconds, grams, mm float64 }

// readSliceInfo reads the per-plate estimates from slice_info.config:
//
//	<plate><metadata key="index" value="1"/><metadata key="prediction" value="3600"/>
//	<metadata key="weight" value="12.3"/><filament used_m="4.1" used_g="12.3"/></plate>
func readSliceInfo(f *zip.File) map[int]plateEstimate {
	out := map[int]plateEstimate{}
	if f.UncompressedSize64 > 8<<20 {
		return out
	}
	rc, err := f.Open()
	if err != nil {
		return out
	}
	defer rc.Close()
	var doc struct {
		Plates []struct {
			Metadata []struct {
				Key   string `xml:"key,attr"`
				Value string `xml:"value,attr"`
			} `xml:"metadata"`
			Filaments []struct {
				UsedM string `xml:"used_m,attr"`
				UsedG string `xml:"used_g,attr"`
			} `xml:"filament"`
		} `xml:"plate"`
	}
	if err := xml.NewDecoder(rc).Decode(&doc); err != nil {
		return out
	}
	num := func(s string) float64 {
		v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return max(v, 0)
	}
	for i, p := range doc.Plates {
		idx := i + 1
		var e plateEstimate
		for _, m := range p.Metadata {
			switch m.Key {
			case "index":
				if n, err := strconv.Atoi(strings.TrimSpace(m.Value)); err == nil && n > 0 {
					idx = n
				}
			case "prediction":
				e.seconds = num(m.Value)
			case "weight":
				e.grams = num(m.Value)
			}
		}
		var grams, metres float64
		for _, fil := range p.Filaments {
			grams += num(fil.UsedG)
			metres += num(fil.UsedM)
		}
		if e.grams == 0 {
			e.grams = grams
		}
		e.mm = metres * 1000
		out[idx] = e
	}
	return out
}
