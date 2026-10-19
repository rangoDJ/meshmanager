package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/riverqueue/river"

	"github.com/rangoDJ/meshmanager/internal/formats"
	"github.com/rangoDJ/meshmanager/internal/library"
	"github.com/rangoDJ/meshmanager/internal/settings"
	"github.com/rangoDJ/meshmanager/internal/store"
	"github.com/rangoDJ/meshmanager/internal/thumbs"
)

// SourceThumbDir is where a source's thumbnails live under DATA_DIR.
func SourceThumbDir(dataDir string, sourceID int64) string {
	return filepath.Join(dataDir, "thumbs", strconv.FormatInt(sourceID, 10))
}

// thumbRelPath fans thumbnails out to at most 1000 files per folder.
func thumbRelPath(sourceID, fileID int64, ext string) string {
	return filepath.ToSlash(filepath.Join("thumbs",
		strconv.FormatInt(sourceID, 10),
		strconv.FormatInt(fileID/1000, 10),
		strconv.FormatInt(fileID, 10)+ext))
}

func thumbnailFormats() []string { return formats.ThumbnailFormats() }

type ThumbnailWorker struct {
	river.WorkerDefaults[ThumbnailArgs]
	Store    *store.Store
	DataDir  string
	Settings *settings.Store
}

func (w *ThumbnailWorker) Work(ctx context.Context, job *river.Job[ThumbnailArgs]) error {
	loc, err := w.Store.GetFileLocation(ctx, job.Args.FileID)
	if errors.Is(err, store.ErrNotFound) {
		return nil // file or source was removed
	}
	if err != nil {
		return err
	}
	if loc.Status != "present" {
		return nil
	}
	if t, err := w.Store.GetThumbnail(ctx, loc.ID); err == nil && t.RendererVersion == thumbs.Version {
		return nil // already done, e.g. a lazy and an eager request raced
	}

	full, cleanup, err := library.Materialize(library.FromStore(loc), library.TempDir(w.DataDir))
	if err == nil {
		defer cleanup()
	}
	var res thumbs.Result
	if err == nil {
		res, err = thumbs.Generate(full, loc.Format, w.Settings.Get().ThumbSize)
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Bad or unsupported files fail the same way every time; record it, don't retry.
		slog.Info("thumbnail failed", "path", loc.RelPath, "err", err)
		return w.Store.SaveThumbnail(ctx, store.Thumbnail{
			FileID: loc.ID, Status: store.ThumbFailed, Error: err.Error(), RendererVersion: thumbs.Version,
		})
	}

	rel := thumbRelPath(loc.SourceID, loc.ID, thumbs.Ext(res.ContentType))
	if err := writeAtomic(filepath.Join(w.DataDir, filepath.FromSlash(rel)), res); err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}
	b := res.Image.Bounds()
	if err := w.Store.SaveThumbnail(ctx, store.Thumbnail{
		FileID: loc.ID, Status: store.ThumbReady, Path: rel, ContentType: res.ContentType,
		Width: b.Dx(), Height: b.Dy(), Origin: res.Origin, RendererVersion: thumbs.Version,
	}); err != nil {
		return err
	}
	if st := res.Stats; st != nil {
		return w.Store.SaveModelInfo(ctx, store.ModelInfo{
			FileID: loc.ID, Triangles: int64(st.Triangles), Size: st.Size(), Volume: st.Volume, Area: st.Area,
			Watertight: res.Watertight,
		})
	}
	return nil
}

// writeAtomic writes via a temp file and rename so readers never see a partial image.
func writeAtomic(dst string, res thumbs.Result) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".thumb-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := thumbs.Encode(tmp, res); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}
