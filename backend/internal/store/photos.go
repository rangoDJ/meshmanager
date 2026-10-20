package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Photo is a picture attached to a file, such as a shot of the finished print.
type Photo struct {
	ID          int64     `json:"id"`
	FileID      int64     `json:"file_id"`
	ContentType string    `json:"content_type"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	Size        int64     `json:"size"`
	IsCover     bool      `json:"is_cover"`
	CreatedAt   time.Time `json:"created_at"`
	// Paths are relative to DATA_DIR: the original, and a smaller copy for tiles.
	Path      string `json:"-"`
	ThumbPath string `json:"-"`
	ThumbType string `json:"-"`
}

const photoColumns = "id, file_id, content_type, width, height, size, is_cover, created_at, path, thumb_path, thumb_type"

func scanPhoto(r pgx.Row) (Photo, error) {
	var p Photo
	err := r.Scan(&p.ID, &p.FileID, &p.ContentType, &p.Width, &p.Height, &p.Size, &p.IsCover, &p.CreatedAt,
		&p.Path, &p.ThumbPath, &p.ThumbType)
	return p, err
}

func (s *Store) AddPhoto(ctx context.Context, p Photo) (Photo, error) {
	err := s.InTx(ctx, func(q *Store, _ pgx.Tx) error {
		var exists bool
		if err := q.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM files WHERE id = $1)", p.FileID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		// The first photo of a file becomes its thumbnail; later ones are just photos.
		return q.db.QueryRow(ctx, `
			INSERT INTO file_photos (file_id, path, content_type, thumb_path, thumb_type, width, height, size, is_cover)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOT EXISTS (SELECT 1 FROM file_photos WHERE file_id = $1))
			RETURNING id`, p.FileID, p.Path, p.ContentType, p.ThumbPath, p.ThumbType, p.Width, p.Height, p.Size).Scan(&p.ID)
	})
	if err != nil {
		return Photo{}, err
	}
	return s.GetPhoto(ctx, p.ID)
}

func (s *Store) GetPhoto(ctx context.Context, id int64) (Photo, error) {
	p, err := scanPhoto(s.db.QueryRow(ctx, "SELECT "+photoColumns+" FROM file_photos WHERE id = $1", id))
	return p, notFound(err)
}

func (s *Store) ListPhotos(ctx context.Context, fileID int64) ([]Photo, error) {
	rows, err := s.db.Query(ctx, "SELECT "+photoColumns+" FROM file_photos WHERE file_id = $1 ORDER BY created_at, id", fileID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Photo, error) { return scanPhoto(r) })
	if out == nil {
		out = []Photo{}
	}
	return out, err
}

// SetPhotoCover makes a photo its file's thumbnail, or clears that so the generated
// thumbnail shows again.
func (s *Store) SetPhotoCover(ctx context.Context, id int64, cover bool) (Photo, error) {
	err := s.InTx(ctx, func(q *Store, _ pgx.Tx) error {
		p, err := q.GetPhoto(ctx, id)
		if err != nil {
			return err
		}
		if _, err := q.db.Exec(ctx, "UPDATE file_photos SET is_cover = false WHERE file_id = $1 AND is_cover", p.FileID); err != nil {
			return err
		}
		_, err = q.db.Exec(ctx, "UPDATE file_photos SET is_cover = $2 WHERE id = $1", id, cover)
		return err
	})
	if err != nil {
		return Photo{}, err
	}
	return s.GetPhoto(ctx, id)
}

// DeletePhoto removes the row and returns the stored paths so the caller can delete the files.
func (s *Store) DeletePhoto(ctx context.Context, id int64) ([]string, error) {
	var path, thumb string
	err := s.db.QueryRow(ctx, "DELETE FROM file_photos WHERE id = $1 RETURNING path, thumb_path", id).Scan(&path, &thumb)
	return []string{path, thumb}, notFound(err)
}

// PhotoPathsForSource lists the stored photo files of a source's files, for cleanup
// when the source is removed.
func (s *Store) PhotoPathsForSource(ctx context.Context, sourceID int64) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT unnest(ARRAY[p.path, p.thumb_path]) FROM file_photos p JOIN files f ON f.id = p.file_id
		WHERE f.source_id = $1`, sourceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// --- folders ------------------------------------------------------------------

// Folder is a subfolder card: its size and the file whose thumbnail covers it.
type Folder struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Files int64  `json:"files"`
	// CoverID is the covering file; Custom is true when the user chose it.
	CoverID *int64 `json:"cover_id"`
	Custom  bool   `json:"custom"`
}

// ListFolders returns the folders directly inside dir, with file counts and a cover each.
// Without a chosen cover, a folder is covered by its first model (or image).
func (s *Store) ListFolders(ctx context.Context, sourceID int64, dir string, kinds []string) ([]Folder, error) {
	q := &queryBuilder{}
	src := q.arg(sourceID)
	d := q.arg(dir)
	conds := []string{"f.source_id = " + src, "f.status = 'present'"}
	if dir == "" {
		conds = append(conds, "f.dir <> ''")
	} else {
		conds = append(conds, "f.dir LIKE "+q.arg(escapeLike(dir)+"/%"))
	}
	if len(kinds) > 0 {
		conds = append(conds, "f.kind = ANY("+q.arg(kinds)+")")
	}
	rows, err := s.db.Query(ctx, fmt.Sprintf(`
		WITH sub AS (
			SELECT f.id, f.dir, f.kind, f.name,
			       CASE WHEN %[1]s::text = '' THEN split_part(f.dir, '/', 1)
			            ELSE %[1]s::text || '/' || split_part(substr(f.dir, length(%[1]s::text) + 2), '/', 1) END AS child
			FROM files f WHERE %[2]s
		)
		SELECT child, count(*),
		       (array_agg(id ORDER BY (dir = child) DESC, (kind = 'model') DESC, (kind = 'image') DESC, lower(name), id))[1]
		FROM sub GROUP BY child ORDER BY lower(child)`, d, strings.Join(conds, " AND ")), q.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Folder
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.Path, &f.Files, &f.CoverID); err != nil {
			return nil, err
		}
		f.Name = f.Path
		if i := strings.LastIndex(f.Path, "/"); i >= 0 {
			f.Name = f.Path[i+1:]
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	chosen, err := s.db.Query(ctx, "SELECT dir, file_id FROM folder_covers WHERE source_id = $1", sourceID)
	if err != nil {
		return nil, err
	}
	covers := map[string]int64{}
	for chosen.Next() {
		var d string
		var id int64
		if err := chosen.Scan(&d, &id); err != nil {
			chosen.Close()
			return nil, err
		}
		covers[d] = id
	}
	chosen.Close()
	if err := chosen.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if id, ok := covers[out[i].Path]; ok {
			out[i].CoverID, out[i].Custom = &id, true
		}
	}
	return out, nil
}

// SetFolderCover picks the file whose thumbnail covers a folder.
func (s *Store) SetFolderCover(ctx context.Context, sourceID int64, dir string, fileID int64) error {
	tag, err := s.db.Exec(ctx, `
		INSERT INTO folder_covers (source_id, dir, file_id)
		SELECT $1, $2, f.id FROM files f WHERE f.id = $3 AND f.source_id = $1
		ON CONFLICT (source_id, dir) DO UPDATE SET file_id = EXCLUDED.file_id`, sourceID, dir, fileID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClearFolderCover(ctx context.Context, sourceID int64, dir string) error {
	_, err := s.db.Exec(ctx, "DELETE FROM folder_covers WHERE source_id = $1 AND dir = $2", sourceID, dir)
	return err
}
