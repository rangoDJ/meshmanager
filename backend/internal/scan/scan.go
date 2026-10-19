// Package scan indexes a source folder: it walks the tree, records every supported
// file, marks vanished files missing, and hashes models and archives with BLAKE3.
// It only ever reads from the library.
package scan

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zeebo/blake3"

	"github.com/rangoDJ/meshmanager/internal/formats"
	"github.com/rangoDJ/meshmanager/internal/library"
	"github.com/rangoDJ/meshmanager/internal/store"
)

const batchSize = 1000

// ProgressFunc receives overall progress (0..1) and a human-readable status line.
type ProgressFunc func(fraction float64, message string)

type Scanner struct {
	Store           *store.Store
	HashConcurrency int
	IndexVideos     bool
	// ProgressEvery throttles progress callbacks during hashing.
	ProgressEvery time.Duration
}

type Result struct {
	Files      int   // supported files found
	Changed    int   // new or modified since the last scan
	Missing    int64 // files that disappeared
	Hashed     int
	HashErrors int
	WalkErrors int
}

func (r Result) Summary() string {
	s := fmt.Sprintf("%d files indexed, %d new or changed, %d missing, %d hashed",
		r.Files, r.Changed, r.Missing, r.Hashed)
	if r.WalkErrors+r.HashErrors > 0 {
		s += fmt.Sprintf(", %d unreadable", r.WalkErrors+r.HashErrors)
	}
	return s
}

// Folders that are never worth indexing (NAS metadata, recycle bins, VCS).
var skipDirs = map[string]bool{
	"@eadir": true, "#recycle": true, "#snapshot": true, "$recycle.bin": true,
	"system volume information": true, "lost+found": true, "node_modules": true,
}

func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || skipDirs[strings.ToLower(name)]
}

func skipFile(name string) bool {
	// macOS resource forks and Office lock files.
	return strings.HasPrefix(name, "._") || strings.HasPrefix(name, "~$")
}

func (s *Scanner) Scan(ctx context.Context, src store.Source, progress ProgressFunc) (Result, error) {
	if progress == nil {
		progress = func(float64, string) {}
	}
	var res Result
	root := src.Path
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return res, fmt.Errorf("source folder %s is not accessible: %w", root, errOr(err, "not a directory"))
	}

	gen, err := s.Store.NextScanGeneration(ctx, src.ID)
	if err != nil {
		return res, err
	}

	progress(0, "Scanning folders…")
	batch := make([]store.FileEntry, 0, batchSize)
	flush := func() error {
		n, err := s.Store.UpsertFiles(ctx, src.ID, gen, batch)
		res.Changed += n
		batch = batch[:0]
		progress(0, fmt.Sprintf("Scanning folders… %d files found", res.Files))
		return err
	}

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			// Unreadable folder or file: note it and keep going.
			res.WalkErrors++
			slog.Warn("scan: cannot read", "path", p, "err", err)
			if d != nil && d.IsDir() && p != root {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && skipDir(name) {
				return fs.SkipDir
			}
			return nil
		}
		if skipFile(name) {
			return nil
		}
		f, ok := formats.Detect(name)
		if !ok || (f.Kind == formats.Video && !s.IndexVideos) {
			return nil
		}
		info, err := os.Stat(p) // follows symlinked files
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		dir := path.Dir(rel)
		if dir == "." {
			dir = ""
		}
		batch = append(batch, store.FileEntry{
			RelPath: rel,
			Dir:     dir,
			Name:    name,
			Kind:    string(f.Kind),
			Format:  f.Name,
			Size:    info.Size(),
			Mtime:   info.ModTime().UTC().Truncate(time.Microsecond),
		})
		res.Files++
		if len(batch) >= batchSize {
			return flush()
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	if err := flush(); err != nil {
		return res, err
	}
	// Carry members of unchanged archives forward first: archives that changed were just
	// reset to unindexed by the walk, so they're excluded here and re-listed below.
	if err := s.Store.CarryArchiveMembers(ctx, src.ID, gen); err != nil {
		return res, err
	}
	if err := s.expandArchives(ctx, src, gen, &res, progress); err != nil {
		return res, err
	}
	if res.Missing, err = s.Store.MarkMissing(ctx, src.ID, gen); err != nil {
		return res, err
	}

	hashed, failed, err := s.hashAll(ctx, src, progress)
	res.Hashed, res.HashErrors = hashed, failed
	return res, err
}

// archiveFormats are the archive formats whose contents are indexed.
var archiveFormats = []string{"zip"}

// expandArchives indexes the supported files inside new or changed archives, as if
// each archive were a folder. Unchanged archives keep their members (see
// CarryArchiveMembers); archives inside archives are listed but not opened.
func (s *Scanner) expandArchives(ctx context.Context, src store.Source, gen int64, res *Result, progress ProgressFunc) error {
	archives, err := s.Store.ArchivesToList(ctx, src.ID, archiveFormats)
	if err != nil {
		return err
	}
	for i, a := range archives {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		progress(0, fmt.Sprintf("Reading archives… %d of %d", i+1, len(archives)))
		members, err := library.ListZip(filepath.Join(src.Path, filepath.FromSlash(a.RelPath)))
		if err != nil {
			// Corrupt or encrypted: note it once; it's retried when the file changes.
			res.WalkErrors++
			slog.Warn("scan: cannot read archive", "path", a.RelPath, "err", err)
			if err := s.Store.MarkMembersIndexed(ctx, a.ID); err != nil {
				return err
			}
			continue
		}

		batch := make([]store.FileEntry, 0, batchSize)
		flush := func() error {
			n, err := s.Store.UpsertFiles(ctx, src.ID, gen, batch)
			res.Changed += n
			batch = batch[:0]
			return err
		}
		for _, m := range members {
			clean := strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(m.Name, "\\", "/")), "/")
			name := path.Base(clean)
			if clean == "" || skipFile(name) {
				continue
			}
			f, ok := formats.Detect(name)
			if !ok || (f.Kind == formats.Video && !s.IndexVideos) {
				continue
			}
			rel := a.RelPath + "/" + clean
			batch = append(batch, store.FileEntry{
				RelPath:    rel,
				Dir:        path.Dir(rel),
				Name:       name,
				Kind:       string(f.Kind),
				Format:     f.Name,
				Size:       m.Size,
				Mtime:      time.UnixMicro(m.Time).UTC(),
				ArchiveID:  a.ID,
				MemberPath: m.Name,
			})
			res.Files++
			if len(batch) >= batchSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
		if err := flush(); err != nil {
			return err
		}
		if err := s.Store.MarkMembersIndexed(ctx, a.ID); err != nil {
			return err
		}
	}
	return nil
}

// hashAll hashes every unhashed model and archive with a bounded worker pool.
// Progress is reported by bytes, since file sizes vary by orders of magnitude.
func (s *Scanner) hashAll(ctx context.Context, src store.Source, progress ProgressFunc) (int, int, error) {
	kinds := []string{string(formats.Model), string(formats.Archive)}
	total, totalBytes, err := s.Store.UnhashedTotals(ctx, src.ID, kinds)
	if err != nil || total == 0 {
		return 0, 0, err
	}

	workers := max(1, s.HashConcurrency)
	every := s.ProgressEvery
	if every <= 0 {
		every = time.Second
	}

	var doneFiles, doneBytes, failed atomic.Int64
	report := func() {
		frac := 1.0
		if totalBytes > 0 {
			frac = float64(doneBytes.Load()) / float64(totalBytes)
		}
		progress(frac, fmt.Sprintf("Hashing %d of %d files", doneFiles.Load(), total))
	}
	report()
	stopReport := make(chan struct{})
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				report()
			case <-stopReport:
				return
			}
		}
	}()
	defer close(stopReport)

	targets := make(chan store.HashTarget)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for t := range targets {
				loc := library.Location{SourcePath: src.Path, RelPath: t.RelPath, ArchiveRelPath: t.ArchiveRelPath, MemberPath: t.MemberPath}
				sum, err := hashFile(ctx, loc)
				if err == nil {
					err = s.Store.SetContentHash(ctx, t.ID, sum)
				}
				if err != nil && ctx.Err() == nil {
					failed.Add(1)
					slog.Warn("scan: hash failed", "path", t.RelPath, "err", err)
				}
				doneFiles.Add(1)
				doneBytes.Add(t.Size)
			}
		})
	}

	var afterID int64
	var pageErr error
	for ctx.Err() == nil {
		page, err := s.Store.UnhashedFiles(ctx, src.ID, kinds, afterID, 500)
		if err != nil {
			pageErr = err
			break
		}
		if len(page) == 0 {
			break
		}
		for _, t := range page {
			select {
			case targets <- t:
			case <-ctx.Done():
			}
		}
		afterID = page[len(page)-1].ID
	}
	close(targets)
	wg.Wait()
	report()

	if pageErr != nil {
		return int(doneFiles.Load() - failed.Load()), int(failed.Load()), pageErr
	}
	return int(doneFiles.Load() - failed.Load()), int(failed.Load()), ctx.Err()
}

func hashFile(ctx context.Context, loc library.Location) (string, error) {
	f, _, err := library.Open(loc)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := blake3.New()
	buf := make([]byte, 1<<20)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		n, err := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func errOr(err error, msg string) error {
	if err != nil {
		return err
	}
	return errors.New(msg)
}
