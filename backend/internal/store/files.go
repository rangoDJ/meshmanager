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
	// For files inside an archive: the archive's file id and the member's path in it.
	ArchiveID  int64
	MemberPath string
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
	archives := make([]int64, len(entries))
	members := make([]string, len(entries))
	for i, e := range entries {
		paths[i], dirs[i], names[i], kinds[i], fmts[i] = e.RelPath, e.Dir, e.Name, e.Kind, e.Format
		sizes[i], mtimes[i], archives[i], members[i] = e.Size, e.Mtime, e.ArchiveID, e.MemberPath
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
			INSERT INTO files (source_id, rel_path, dir, name, kind, format, size, mtime, seen_generation,
			                   archive_id, member_path)
			SELECT $1, u.rel, u.dir, u.name, u.kind, u.fmt, u.size, u.mtime, $9, NULLIF(u.aid, 0), NULLIF(u.member, '')
			FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::bigint[],
			            $8::timestamptz[], $10::bigint[], $11::text[])
			     AS u(rel, dir, name, kind, fmt, size, mtime, aid, member)
			ON CONFLICT (source_id, rel_path) DO UPDATE SET
				kind = EXCLUDED.kind,
				format = EXCLUDED.format,
				size = EXCLUDED.size,
				mtime = EXCLUDED.mtime,
				seen_generation = EXCLUDED.seen_generation,
				archive_id = EXCLUDED.archive_id,
				member_path = EXCLUDED.member_path,
				status = 'present',
				missing_since = NULL,
				content_hash = CASE
					WHEN files.size = EXCLUDED.size AND files.mtime = EXCLUDED.mtime THEN files.content_hash
				END,
				members_indexed = files.members_indexed
					AND files.size = EXCLUDED.size AND files.mtime = EXCLUDED.mtime`,
			sourceID, paths, dirs, names, kinds, fmts, sizes, mtimes, gen, archives, members); err != nil {
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
	// Set for archive members: where to read the bytes from.
	ArchiveRelPath string
	ArchiveFormat  string
	MemberPath     string
}

// UnhashedFiles pages through present files of the given kinds that have no content hash.
func (s *Store) UnhashedFiles(ctx context.Context, sourceID int64, kinds []string, afterID int64, limit int) ([]HashTarget, error) {
	rows, err := s.db.Query(ctx, `
		SELECT f.id, f.rel_path, f.size, coalesce(a.rel_path, ''), coalesce(a.format, ''),
		       coalesce(f.member_path, '')
		FROM files f LEFT JOIN files a ON a.id = f.archive_id
		WHERE f.source_id = $1 AND f.status = 'present' AND f.content_hash IS NULL
		  AND f.kind = ANY($2) AND f.id > $3
		ORDER BY f.id LIMIT $4`, sourceID, kinds, afterID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (HashTarget, error) {
		var t HashTarget
		err := r.Scan(&t.ID, &t.RelPath, &t.Size, &t.ArchiveRelPath, &t.ArchiveFormat, &t.MemberPath)
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

// FileLocation is what's needed to open a file: a path on disk, or for archive members,
// the archive's path plus the member's path inside it.
type FileLocation struct {
	ID             int64
	SourceID       int64
	SourcePath     string
	RelPath        string
	Name           string
	Format         string
	Status         string
	Size           int64
	Mtime          time.Time
	ArchiveRelPath string
	ArchiveFormat  string
	MemberPath     string
}

func (s *Store) GetFileLocation(ctx context.Context, id int64) (FileLocation, error) {
	var l FileLocation
	err := s.db.QueryRow(ctx, `
		SELECT f.id, f.source_id, s.path, f.rel_path, f.name, f.format, f.status, f.size, f.mtime,
		       coalesce(a.rel_path, ''), coalesce(a.format, ''), coalesce(f.member_path, '')
		FROM files f JOIN sources s ON s.id = f.source_id
		LEFT JOIN files a ON a.id = f.archive_id
		WHERE f.id = $1`, id,
	).Scan(&l.ID, &l.SourceID, &l.SourcePath, &l.RelPath, &l.Name, &l.Format, &l.Status, &l.Size, &l.Mtime,
		&l.ArchiveRelPath, &l.ArchiveFormat, &l.MemberPath)
	return l, notFound(err)
}

type ArchiveRef struct {
	ID      int64
	RelPath string
	Format  string
}

// ArchivesToList returns present archives of a source whose members haven't been
// indexed since they last changed.
func (s *Store) ArchivesToList(ctx context.Context, sourceID int64, formats []string) ([]ArchiveRef, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, rel_path, format FROM files
		WHERE source_id = $1 AND status = 'present' AND kind = 'archive' AND archive_id IS NULL
		  AND format = ANY($2) AND NOT members_indexed
		ORDER BY id`, sourceID, formats)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ArchiveRef])
}

func (s *Store) MarkMembersIndexed(ctx context.Context, archiveID int64) error {
	_, err := s.db.Exec(ctx, "UPDATE files SET members_indexed = true WHERE id = $1", archiveID)
	return err
}

// CarryArchiveMembers moves the members of unchanged (still indexed) archives seen in
// generation gen into that generation, so MarkMissing keeps them. Call it before
// re-listing changed archives.
func (s *Store) CarryArchiveMembers(ctx context.Context, sourceID, gen int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE files m SET seen_generation = $2, status = 'present', missing_since = NULL
		FROM files a
		WHERE m.archive_id = a.id AND a.source_id = $1 AND a.seen_generation = $2
		  AND a.members_indexed AND m.seen_generation < $2`, sourceID, gen)
	return err
}

// --- listing -----------------------------------------------------------------

type FileItem struct {
	ID             int64       `json:"id"`
	SourceID       int64       `json:"source_id"`
	SourceName     string      `json:"source_name"`
	SourceHostPath *string     `json:"-"`
	RelPath        string      `json:"rel_path"`
	Dir            string      `json:"dir"`
	Name           string      `json:"name"`
	Kind           string      `json:"kind"`
	Format         string      `json:"format"`
	Size           int64       `json:"size"`
	Mtime          time.Time   `json:"mtime"`
	ContentHash    *string     `json:"content_hash"`
	ArchiveID      *int64      `json:"archive_id"`
	ThumbStatus    *string     `json:"-"`
	ThumbAt        *time.Time  `json:"-"`
	ThumbVersion   *int        `json:"-"`
	ThumbError     *string     `json:"thumb_error,omitempty"`
	Triangles      *int64      `json:"triangles"`
	Dimensions     *[3]float64 `json:"dimensions"`
	VolumeMM3      *float64    `json:"volume_mm3"`
	AreaMM2        *float64    `json:"area_mm2"`
	Watertight     *bool       `json:"watertight"`
	Tags           []TagRef    `json:"tags"`
	// A photo chosen as the file's thumbnail replaces the generated one.
	CoverPhotoID *int64     `json:"-"`
	CoverAt      *time.Time `json:"-"`
	// Missing is true for a file that was indexed but is no longer on disk.
	Missing bool `json:"missing"`
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

const fileColumns = `
	f.id, f.source_id, s.name, s.host_path, f.rel_path, f.dir, f.name, f.kind, f.format, f.size, f.mtime,
	f.content_hash, f.archive_id, t.status, t.created_at, t.error, t.renderer_version,
	m.triangles, m.size_x, m.size_y, m.size_z, m.volume_mm3, m.area_mm2, m.watertight,
	coalesce((SELECT json_agg(json_build_object('id', tg.id, 'name', tg.name, 'color', tg.color, 'pinned', tg.pinned)
	                          ORDER BY tg.pinned DESC, lower(tg.name))
	          FROM file_tags ft JOIN tags tg ON tg.id = ft.tag_id WHERE ft.file_id = f.id), '[]'::json),
	cp.id, cp.created_at, f.status = 'missing'`

// fileJoinsRest is everything a file listing joins to files f.
const fileJoinsRest = `
	JOIN sources s ON s.id = f.source_id
	LEFT JOIN thumbnails t ON t.file_id = f.id
	LEFT JOIN model_info m ON m.file_id = f.id
	LEFT JOIN file_photos cp ON cp.file_id = f.id AND cp.is_cover`

const fileJoins = "\n\tFROM files f" + fileJoinsRest

func (s *Store) ListFiles(ctx context.Context, f FileFilter) ([]FileItem, string, error) {
	f, err := s.resolve(ctx, f)
	if err != nil {
		return nil, "", err
	}
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

	rows, err := s.db.Query(ctx, "SELECT "+fileColumns+fileJoins+`
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

func scanFileItem(r pgx.CollectableRow) (FileItem, error) { return scanFileItemWith(r) }

// scanFileItemWith scans a row of fileColumns, after any leading columns of the caller's own.
func scanFileItemWith(r interface{ Scan(...any) error }, lead ...any) (FileItem, error) {
	var it FileItem
	var sx, sy, sz *float64
	err := r.Scan(append(lead, &it.ID, &it.SourceID, &it.SourceName, &it.SourceHostPath, &it.RelPath, &it.Dir, &it.Name, &it.Kind,
		&it.Format, &it.Size, &it.Mtime, &it.ContentHash, &it.ArchiveID, &it.ThumbStatus, &it.ThumbAt, &it.ThumbError,
		&it.ThumbVersion, &it.Triangles, &sx, &sy, &sz, &it.VolumeMM3, &it.AreaMM2, &it.Watertight,
		&it.Tags, &it.CoverPhotoID, &it.CoverAt, &it.Missing)...)
	if it.Tags == nil {
		it.Tags = []TagRef{}
	}
	if sx != nil && sy != nil && sz != nil {
		it.Dimensions = &[3]float64{*sx, *sy, *sz}
	}
	return it, err
}

func (s *Store) GetFile(ctx context.Context, id int64) (FileItem, error) {
	rows, err := s.db.Query(ctx, "SELECT "+fileColumns+fileJoins+" WHERE f.id = $1", id)
	if err != nil {
		return FileItem{}, err
	}
	it, err := pgx.CollectExactlyOneRow(rows, scanFileItem)
	return it, notFound(err)
}

func (s *Store) CountFiles(ctx context.Context, f FileFilter) (int64, error) {
	f, err := s.resolve(ctx, f)
	if err != nil {
		return 0, err
	}
	q := f.filters(true)
	var n int64
	err = s.db.QueryRow(ctx, "SELECT count(*) FROM files f WHERE "+q.sql(), q.args...).Scan(&n)
	return n, err
}

type Facet struct {
	Kind   string `json:"kind"`
	Format string `json:"format"`
	Count  int64  `json:"count"`
}

// Facets counts files by kind and format, ignoring the kind and format filters.
func (s *Store) Facets(ctx context.Context, f FileFilter) ([]Facet, error) {
	f, err := s.resolve(ctx, f)
	if err != nil {
		return nil, err
	}
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

// FilesByIDs returns the given files in the order of ids; missing ones are left out.
func (s *Store) FilesByIDs(ctx context.Context, ids []int64) ([]FileItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, "SELECT "+fileColumns+fileJoins+" WHERE f.id = ANY($1)", ids)
	if err != nil {
		return nil, err
	}
	items, err := pgx.CollectRows(rows, scanFileItem)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]FileItem, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	out := make([]FileItem, 0, len(items))
	for _, id := range ids {
		if it, ok := byID[id]; ok {
			out = append(out, it)
		}
	}
	return out, nil
}
