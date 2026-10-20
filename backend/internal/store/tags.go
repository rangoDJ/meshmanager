package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// TagColors are the colours a tag can have. The frontend maps each to a shade that
// reads well in both themes.
var TagColors = []string{"slate", "red", "orange", "amber", "green", "teal", "blue", "indigo", "violet", "pink"}

func ValidTagColor(c string) bool {
	for _, v := range TagColors {
		if v == c {
			return true
		}
	}
	return false
}

// TagRef is a tag as shown on a file.
type TagRef struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Color  string `json:"color"`
	Pinned bool   `json:"pinned"`
}

type Tag struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Pinned    bool      `json:"pinned"`
	Files     int64     `json:"files"` // present files carrying the tag
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id, t.name, t.color, t.pinned, count(f.id), t.created_at
		FROM tags t
		LEFT JOIN file_tags ft ON ft.tag_id = t.id
		LEFT JOIN files f ON f.id = ft.file_id AND f.status = 'present'
		GROUP BY t.id
		ORDER BY t.pinned DESC, lower(t.name)`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Tag])
}

func (s *Store) GetTag(ctx context.Context, id int64) (Tag, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id, t.name, t.color, t.pinned,
		       (SELECT count(*) FROM file_tags ft JOIN files f ON f.id = ft.file_id
		        WHERE ft.tag_id = t.id AND f.status = 'present'),
		       t.created_at
		FROM tags t WHERE t.id = $1`, id)
	if err != nil {
		return Tag{}, err
	}
	t, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Tag])
	return t, notFound(err)
}

// CreateTag adds a tag; an empty colour picks the next one in the palette.
func (s *Store) CreateTag(ctx context.Context, name, color string, pinned bool) (Tag, error) {
	if color == "" {
		var n int
		if err := s.db.QueryRow(ctx, "SELECT count(*) FROM tags").Scan(&n); err != nil {
			return Tag{}, err
		}
		auto := TagColors[1:] // slate is for tags the user greys out on purpose
		color = auto[n%len(auto)]
	}
	var id int64
	err := s.db.QueryRow(ctx,
		"INSERT INTO tags (name, color, pinned) VALUES ($1, $2, $3) RETURNING id",
		strings.TrimSpace(name), color, pinned).Scan(&id)
	if isUniqueViolation(err) {
		return Tag{}, ErrConflict
	}
	if err != nil {
		return Tag{}, err
	}
	return s.GetTag(ctx, id)
}

type TagUpdate struct {
	Name   *string
	Color  *string
	Pinned *bool
}

func (s *Store) UpdateTag(ctx context.Context, id int64, u TagUpdate) (Tag, error) {
	if u.Name != nil {
		n := strings.TrimSpace(*u.Name)
		u.Name = &n
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE tags SET name = coalesce($2, name), color = coalesce($3, color), pinned = coalesce($4, pinned)
		WHERE id = $1`, id, u.Name, u.Color, u.Pinned)
	if isUniqueViolation(err) {
		return Tag{}, ErrConflict
	}
	if err != nil {
		return Tag{}, err
	}
	if tag.RowsAffected() == 0 {
		return Tag{}, ErrNotFound
	}
	return s.GetTag(ctx, id)
}

func (s *Store) DeleteTag(ctx context.Context, id int64) error {
	return s.InTx(ctx, func(q *Store, _ pgx.Tx) error {
		if err := q.replaceTagInRules(ctx, id, 0); err != nil {
			return err
		}
		tag, err := q.db.Exec(ctx, "DELETE FROM tags WHERE id = $1", id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// MergeTags moves every file from one tag to another and deletes the first.
func (s *Store) MergeTags(ctx context.Context, from, into int64) error {
	if from == into {
		return ErrConflict
	}
	return s.InTx(ctx, func(q *Store, _ pgx.Tx) error {
		if _, err := q.GetTag(ctx, into); err != nil {
			return err
		}
		if _, err := q.db.Exec(ctx, `
			INSERT INTO file_tags (file_id, tag_id)
			SELECT file_id, $2 FROM file_tags WHERE tag_id = $1
			ON CONFLICT DO NOTHING`, from, into); err != nil {
			return err
		}
		if err := q.replaceTagInRules(ctx, from, into); err != nil {
			return err
		}
		tag, err := q.db.Exec(ctx, "DELETE FROM tags WHERE id = $1", from)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ApplyTags adds and removes tags on a set of files. Names that don't exist yet are
// created. It returns the number of file/tag pairs added.
func (s *Store) ApplyTags(ctx context.Context, fileIDs, addIDs []int64, addNames []string, removeIDs []int64) (int64, error) {
	var added int64
	err := s.InTx(ctx, func(q *Store, _ pgx.Tx) error {
		for _, raw := range addNames {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			var id int64
			err := q.db.QueryRow(ctx, "SELECT id FROM tags WHERE lower(name) = lower($1)", name).Scan(&id)
			if err == pgx.ErrNoRows {
				t, err := q.CreateTag(ctx, name, "", false)
				if err != nil {
					return err
				}
				id = t.ID
			} else if err != nil {
				return err
			}
			addIDs = append(addIDs, id)
		}
		if len(addIDs) > 0 {
			tag, err := q.db.Exec(ctx, `
				INSERT INTO file_tags (file_id, tag_id)
				SELECT f.id, t.id FROM files f, tags t
				WHERE f.id = ANY($1) AND t.id = ANY($2)
				ON CONFLICT DO NOTHING`, fileIDs, addIDs)
			if err != nil {
				return err
			}
			added = tag.RowsAffected()
		}
		if len(removeIDs) > 0 {
			if _, err := q.db.Exec(ctx,
				"DELETE FROM file_tags WHERE file_id = ANY($1) AND tag_id = ANY($2)", fileIDs, removeIDs); err != nil {
				return err
			}
		}
		return nil
	})
	return added, err
}

// FileMeta is a file's notes and provenance.
type FileMeta struct {
	Notes     string `json:"notes"`
	SourceURL string `json:"source_url"`
	Designer  string `json:"designer"`
	License   string `json:"license"`
}

func (m FileMeta) empty() bool {
	return m.Notes == "" && m.SourceURL == "" && m.Designer == "" && m.License == ""
}

func (s *Store) GetFileMeta(ctx context.Context, fileID int64) (FileMeta, error) {
	var m FileMeta
	err := s.db.QueryRow(ctx,
		"SELECT notes, source_url, designer, license FROM file_meta WHERE file_id = $1", fileID,
	).Scan(&m.Notes, &m.SourceURL, &m.Designer, &m.License)
	if err == pgx.ErrNoRows {
		return FileMeta{}, nil
	}
	return m, err
}

type MetaUpdate struct {
	Notes     *string
	SourceURL *string
	Designer  *string
	License   *string
}

// SetFileMeta changes the given fields; the row goes away once everything is empty.
func (s *Store) SetFileMeta(ctx context.Context, fileID int64, u MetaUpdate) (FileMeta, error) {
	var out FileMeta
	err := s.InTx(ctx, func(q *Store, _ pgx.Tx) error {
		var exists bool
		if err := q.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM files WHERE id = $1)", fileID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		m, err := q.GetFileMeta(ctx, fileID)
		if err != nil {
			return err
		}
		if u.Notes != nil {
			m.Notes = *u.Notes
		}
		if u.SourceURL != nil {
			m.SourceURL = *u.SourceURL
		}
		if u.Designer != nil {
			m.Designer = *u.Designer
		}
		if u.License != nil {
			m.License = *u.License
		}
		out = m
		if m.empty() {
			_, err := q.db.Exec(ctx, "DELETE FROM file_meta WHERE file_id = $1", fileID)
			return err
		}
		_, err = q.db.Exec(ctx, `
			INSERT INTO file_meta (file_id, notes, source_url, designer, license)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (file_id) DO UPDATE SET
				notes = EXCLUDED.notes, source_url = EXCLUDED.source_url,
				designer = EXCLUDED.designer, license = EXCLUDED.license, updated_at = now()`,
			fileID, m.Notes, m.SourceURL, m.Designer, m.License)
		return err
	})
	return out, err
}
