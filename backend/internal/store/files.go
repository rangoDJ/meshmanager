package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// FileEntry is what the scanner learns about a file from the filesystem.
type FileEntry struct {
	RelPath string
	Dir     string
	Name    string
	Kind    string
	Format  string
	Size    int64
	Mtime   time.Time
}

// UpsertFiles records a batch of files seen in scan generation gen. Files that are new,
// or whose size or mtime changed, lose their hash, thumbnail and model info so they get
// recomputed. It returns how many files were new or changed.
func (s *Store) UpsertFiles(ctx context.Context, sourceID, gen int64, entries []FileEntry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	paths := make([]string, len(entries))
	dirs := make([]string, len(entries))
	names := make([]string, len(entries))
	kinds := make([]string, len(entries))
	fmts := make([]string, len(entries))
	sizes := make([]int64, len(entries))
	mtimes := make([]time.Time, len(entries))
	for i, e := range entries {
		paths[i], dirs[i], names[i], kinds[i], fmts[i] = e.RelPath, e.Dir, e.Name, e.Kind, e.Format
		sizes[i], mtimes[i] = e.Size, e.Mtime
	}

	changed := 0
	err := s.InTx(ctx, func(q *Store, tx pgx.Tx) error {
		type prev struct {
			id    int64
			size  int64
			mtime time.Time
		}
		existing := map[string]prev{}
		rows, err := tx.Query(ctx,
			"SELECT rel_path, id, size, mtime FROM files WHERE source_id = $1 AND rel_path = ANY($2)",
			sourceID, paths)
		if err != nil {
			return err
		}
		for rows.Next() {
			var p string
			var v prev
			if err := rows.Scan(&p, &v.id, &v.size, &v.mtime); err != nil {
				rows.Close()
				return err
			}
			existing[p] = v
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO files (source_id, rel_path, dir, name, kind, format, size, mtime, seen_generation)
			SELECT $1, * , $9 FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[],
			                              $7::bigint[], $8::timestamptz[])
			ON CONFLICT (source_id, rel_path) DO UPDATE SET
				kind = EXCLUDED.kind,
				format = EXCLUDED.format,
				size = EXCLUDED.size,
				mtime = EXCLUDED.mtime,
				seen_generation = EXCLUDED.seen_generation,
				status = 'present',
				missing_since = NULL,
				content_hash = CASE
					WHEN files.size = EXCLUDED.size AND files.mtime = EXCLUDED.mtime THEN files.content_hash
				END`,
			sourceID, paths, dirs, names, kinds, fmts, sizes, mtimes, gen); err != nil {
			return err
		}

		var stale []int64
		for _, e := range entries {
			p, ok := existing[e.RelPath]
			switch {
			case !ok:
				changed++
			case p.size != e.Size || p.mtime.UnixMicro() != e.Mtime.UnixMicro():
				changed++
				stale = append(stale, p.id)
			}
		}
		if len(stale) > 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM thumbnails WHERE file_id = ANY($1)", stale); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "DELETE FROM model_info WHERE file_id = ANY($1)", stale); err != nil {
				return err
			}
		}
		return nil
	})
	return changed, err
}

// MarkMissing flags files not seen in generation gen. Rows are kept so tags and notes
// can follow a file that was moved or renamed.
func (s *Store) MarkMissing(ctx context.Context, sourceID, gen int64) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE files SET status = 'missing', missing_since = now()
		WHERE source_id = $1 AND seen_generation < $2 AND status = 'present'`, sourceID, gen)
	return tag.RowsAffected(), err
}

type HashTarget struct {
	ID      int64
	RelPath string
	Size    int64
}

// UnhashedFiles pages through present files of the given kinds that have no content hash.
func (s *Store) UnhashedFiles(ctx context.Context, sourceID int64, kinds []string, afterID int64, limit int) ([]HashTarget, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, rel_path, size FROM files
		WHERE source_id = $1 AND status = 'present' AND content_hash IS NULL
		  AND kind = ANY($2) AND id > $3
		ORDER BY id LIMIT $4`, sourceID, kinds, afterID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (HashTarget, error) {
		var t HashTarget
		err := r.Scan(&t.ID, &t.RelPath, &t.Size)
		return t, err
	})
}

func (s *Store) UnhashedTotals(ctx context.Context, sourceID int64, kinds []string) (count, bytes int64, err error) {
	err = s.db.QueryRow(ctx, `
		SELECT count(*), coalesce(sum(size), 0) FROM files
		WHERE source_id = $1 AND status = 'present' AND content_hash IS NULL AND kind = ANY($2)`,
		sourceID, kinds).Scan(&count, &bytes)
	return count, bytes, err
}

func (s *Store) SetContentHash(ctx context.Context, id int64, hash string) error {
	_, err := s.db.Exec(ctx, "UPDATE files SET content_hash = $2 WHERE id = $1", id, hash)
	return err
}

// FilesNeedingThumbnails pages through present files in the given formats that have
// no thumbnail row yet.
func (s *Store) FilesNeedingThumbnails(ctx context.Context, sourceID int64, formats []string, afterID int64, limit int) ([]int64, error) {
	rows, err := s.db.Query(ctx, `
		SELECT f.id FROM files f
		LEFT JOIN thumbnails t ON t.file_id = f.id
		WHERE f.source_id = $1 AND f.status = 'present' AND f.format = ANY($2)
		  AND t.file_id IS NULL AND f.id > $3
		ORDER BY f.id LIMIT $4`, sourceID, formats, afterID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// FileLocation is what's needed to open a file on disk.
type FileLocation struct {
	ID         int64
	SourceID   int64
	SourcePath string
	RelPath    string
	Name       string
	Format     string
	Status     string
	Size       int64
	Mtime      time.Time
}

func (s *Store) GetFileLocation(ctx context.Context, id int64) (FileLocation, error) {
	var l FileLocation
	err := s.db.QueryRow(ctx, `
		SELECT f.id, f.source_id, s.path, f.rel_path, f.name, f.format, f.status, f.size, f.mtime
		FROM files f JOIN sources s ON s.id = f.source_id WHERE f.id = $1`, id,
	).Scan(&l.ID, &l.SourceID, &l.SourcePath, &l.RelPath, &l.Name, &l.Format, &l.Status, &l.Size, &l.Mtime)
	return l, notFound(err)
}

// --- listing -----------------------------------------------------------------

type FileFilter struct {
	SourceID int64
	// Dir limits results to a folder and its subfolders; requires SourceID. Empty means all.
	Dir     string
	Kinds   []string
	Formats []string
	Query   string
	Sort    string // "name", "-mtime" (default), "-size"
	Cursor  string
	Limit   int
}

type FileItem struct {
	ID          int64       `json:"id"`
	SourceID    int64       `json:"source_id"`
	SourceName  string      `json:"source_name"`
	RelPath     string      `json:"rel_path"`
	Dir         string      `json:"dir"`
	Name        string      `json:"name"`
	Kind        string      `json:"kind"`
	Format      string      `json:"format"`
	Size        int64       `json:"size"`
	Mtime       time.Time   `json:"mtime"`
	ContentHash *string     `json:"content_hash"`
	ThumbStatus *string     `json:"-"`
	ThumbAt     *time.Time  `json:"-"`
	ThumbError  *string     `json:"thumb_error,omitempty"`
	Triangles   *int64      `json:"triangles"`
	Dimensions  *[3]float64 `json:"dimensions"`
	VolumeMM3   *float64    `json:"volume_mm3"`
}

var ErrBadCursor = errors.New("invalid cursor")

type cursor struct {
	S  string `json:"s,omitempty"`
	N  int64  `json:"n,omitempty"`
	ID int64  `json:"id"`
}

func encodeCursor(c cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (cursor, error) {
	var c cursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || json.Unmarshal(b, &c) != nil {
		return c, ErrBadCursor
	}
	return c, nil
}

type queryBuilder struct {
	where []string
	args  []any
}

func (q *queryBuilder) arg(v any) string {
	q.args = append(q.args, v)
	return fmt.Sprintf("$%d", len(q.args))
}

func (q *queryBuilder) add(cond string) { q.where = append(q.where, cond) }

func (q *queryBuilder) sql() string { return strings.Join(q.where, " AND ") }

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// filters builds the WHERE clause shared by listing, counting and facets.
func (f FileFilter) filters(withKinds bool) *queryBuilder {
	q := &queryBuilder{}
	q.add("f.status = 'present'")
	if f.SourceID > 0 {
		q.add("f.source_id = " + q.arg(f.SourceID))
		if f.Dir != "" {
			q.add(fmt.Sprintf("(f.dir = %s OR f.dir LIKE %s)", q.arg(f.Dir), q.arg(escapeLike(f.Dir)+"/%")))
		}
	}
	if withKinds {
		if len(f.Kinds) > 0 {
			q.add("f.kind = ANY(" + q.arg(f.Kinds) + ")")
		}
		if len(f.Formats) > 0 {
			q.add("f.format = ANY(" + q.arg(f.Formats) + ")")
		}
	}
	for _, term := range strings.Fields(f.Query) {
		q.add("f.rel_path ILIKE " + q.arg("%"+escapeLike(term)+"%"))
	}
	return q
}

func (s *Store) ListFiles(ctx context.Context, f FileFilter) ([]FileItem, string, error) {
	q := f.filters(true)

	var order, keyExpr, cmp string
	switch f.Sort {
	case "name":
		order, keyExpr, cmp = "lower(f.name), f.id", "lower(f.name)", ">"
	case "-size":
		order, keyExpr, cmp = "f.size DESC, f.id DESC", "f.size", "<"
	default:
		f.Sort = "-mtime"
		order, keyExpr, cmp = "f.mtime DESC, f.id DESC", "f.mtime", "<"
	}
	if f.Cursor != "" {
		c, err := decodeCursor(f.Cursor)
		if err != nil {
			return nil, "", err
		}
		var key any
		switch f.Sort {
		case "name":
			key = c.S
		case "-size":
			key = c.N
		default:
			key = time.UnixMicro(c.N).UTC()
		}
		q.add(fmt.Sprintf("(%s, f.id) %s (%s, %s)", keyExpr, cmp, q.arg(key), q.arg(c.ID)))
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := s.db.Query(ctx, `
		SELECT f.id, f.source_id, s.name, f.rel_path, f.dir, f.name, f.kind, f.format, f.size, f.mtime,
		       f.content_hash, t.status, t.created_at, t.error,
		       m.triangles, m.size_x, m.size_y, m.size_z, m.volume_mm3
		FROM files f
		JOIN sources s ON s.id = f.source_id
		LEFT JOIN thumbnails t ON t.file_id = f.id
		LEFT JOIN model_info m ON m.file_id = f.id
		WHERE `+q.sql()+`
		ORDER BY `+order+`
		LIMIT `+q.arg(limit+1), q.args...)
	if err != nil {
		return nil, "", err
	}
	items, err := pgx.CollectRows(rows, scanFileItem)
	if err != nil {
		return nil, "", err
	}

	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		c := cursor{ID: last.ID}
		switch f.Sort {
		case "name":
			c.S = strings.ToLower(last.Name)
		case "-size":
			c.N = last.Size
		default:
			c.N = last.Mtime.UnixMicro()
		}
		next = encodeCursor(c)
	}
	return items, next, nil
}

func scanFileItem(r pgx.CollectableRow) (FileItem, error) {
	var it FileItem
	var sx, sy, sz *float64
	err := r.Scan(&it.ID, &it.SourceID, &it.SourceName, &it.RelPath, &it.Dir, &it.Name, &it.Kind,
		&it.Format, &it.Size, &it.Mtime, &it.ContentHash, &it.ThumbStatus, &it.ThumbAt, &it.ThumbError,
		&it.Triangles, &sx, &sy, &sz, &it.VolumeMM3)
	if sx != nil && sy != nil && sz != nil {
		it.Dimensions = &[3]float64{*sx, *sy, *sz}
	}
	return it, err
}

func (s *Store) GetFile(ctx context.Context, id int64) (FileItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT f.id, f.source_id, s.name, f.rel_path, f.dir, f.name, f.kind, f.format, f.size, f.mtime,
		       f.content_hash, t.status, t.created_at, t.error,
		       m.triangles, m.size_x, m.size_y, m.size_z, m.volume_mm3
		FROM files f
		JOIN sources s ON s.id = f.source_id
		LEFT JOIN thumbnails t ON t.file_id = f.id
		LEFT JOIN model_info m ON m.file_id = f.id
		WHERE f.id = $1`, id)
	if err != nil {
		return FileItem{}, err
	}
	it, err := pgx.CollectExactlyOneRow(rows, scanFileItem)
	return it, notFound(err)
}

func (s *Store) CountFiles(ctx context.Context, f FileFilter) (int64, error) {
	q := f.filters(true)
	var n int64
	err := s.db.QueryRow(ctx, "SELECT count(*) FROM files f WHERE "+q.sql(), q.args...).Scan(&n)
	return n, err
}

type Facet struct {
	Kind   string `json:"kind"`
	Format string `json:"format"`
	Count  int64  `json:"count"`
}

// Facets counts files by kind and format, ignoring the kind and format filters.
func (s *Store) Facets(ctx context.Context, f FileFilter) ([]Facet, error) {
	q := f.filters(false)
	rows, err := s.db.Query(ctx, `
		SELECT f.kind, f.format, count(*) FROM files f WHERE `+q.sql()+`
		GROUP BY f.kind, f.format ORDER BY count(*) DESC`, q.args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Facet])
}

type DirCount struct {
	Dir   string
	Count int64
}

// DirCounts returns the number of present files directly in each folder of a source.
func (s *Store) DirCounts(ctx context.Context, sourceID int64, kinds []string) ([]DirCount, error) {
	q := FileFilter{SourceID: sourceID, Kinds: kinds}.filters(true)
	rows, err := s.db.Query(ctx,
		"SELECT f.dir, count(*) FROM files f WHERE "+q.sql()+" GROUP BY f.dir", q.args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[DirCount])
}
