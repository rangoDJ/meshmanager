package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// hasOrganizeData is true for files someone has tagged, noted, filed, queued,
// photographed or chosen as a folder cover. Those are the files worth following.
const hasOrganizeData = `(
	EXISTS (SELECT 1 FROM file_tags WHERE file_id = f.id) OR
	EXISTS (SELECT 1 FROM file_meta WHERE file_id = f.id) OR
	EXISTS (SELECT 1 FROM collection_files WHERE file_id = f.id) OR
	EXISTS (SELECT 1 FROM print_items WHERE file_id = f.id) OR
	EXISTS (SELECT 1 FROM file_photos WHERE file_id = f.id) OR
	EXISTS (SELECT 1 FROM folder_covers WHERE file_id = f.id))`

type moveCandidate struct {
	ID       int64
	SourceID int64
	Dir      string
	Name     string
	Hash     *string
	Size     int64
	Mtime    time.Time
	Format   string
}

// ReconcileMoves hands what was recorded about files that vanished (tags, notes,
// collections, queue entries, photos, folder covers) to the files that appeared with
// the same content. That is how a rename or move on disk is followed without a rescan
// losing anything. It returns how many files were followed.
//
// Files are matched by content hash. Kinds that aren't hashed (images, documents, G-code)
// are matched by size, modification time and format, and only when that picks out exactly
// one new file.
func (s *Store) ReconcileMoves(ctx context.Context) (int, error) {
	// Members before their archives: deleting an archive row deletes its members.
	rows, err := s.db.Query(ctx, `
		SELECT f.id, f.source_id, f.dir, f.name, f.content_hash, f.size, f.mtime, f.format
		FROM files f
		WHERE f.status = 'missing' AND `+hasOrganizeData+`
		ORDER BY (f.archive_id IS NULL), f.id`)
	if err != nil {
		return 0, err
	}
	gone, err := pgx.CollectRows(rows, pgx.RowToStructByPos[moveCandidate])
	if err != nil {
		return 0, err
	}

	moved := 0
	for _, g := range gone {
		target, ok, err := s.findMoveTarget(ctx, g)
		if err != nil {
			return moved, err
		}
		if !ok {
			continue
		}
		if err := s.InTx(ctx, func(q *Store, _ pgx.Tx) error { return q.moveOrganizeData(ctx, g, target) }); err != nil {
			return moved, err
		}
		moved++
	}
	return moved, nil
}

func (s *Store) findMoveTarget(ctx context.Context, g moveCandidate) (moveCandidate, bool, error) {
	var rows pgx.Rows
	var err error
	hashed := g.Hash != nil
	if hashed {
		rows, err = s.db.Query(ctx, `
			SELECT id, source_id, dir, name, content_hash, size, mtime, format FROM files
			WHERE status = 'present' AND content_hash = $1 AND size = $2`, *g.Hash, g.Size)
	} else {
		rows, err = s.db.Query(ctx, `
			SELECT id, source_id, dir, name, content_hash, size, mtime, format FROM files
			WHERE status = 'present' AND content_hash IS NULL AND size = $1 AND mtime = $2 AND format = $3`,
			g.Size, g.Mtime, g.Format)
	}
	if err != nil {
		return moveCandidate{}, false, err
	}
	cands, err := pgx.CollectRows(rows, pgx.RowToStructByPos[moveCandidate])
	if err != nil || len(cands) == 0 {
		return moveCandidate{}, false, err
	}
	if !hashed {
		if len(cands) != 1 {
			return moveCandidate{}, false, nil // too ambiguous to guess
		}
		return cands[0], true, nil
	}
	// Several identical files: prefer the one that kept its name, then the same source.
	best, bestScore := cands[0], -1
	for _, c := range cands {
		score := 0
		if c.Name == g.Name {
			score += 2
		}
		if c.SourceID == g.SourceID {
			score++
		}
		if score > bestScore {
			best, bestScore = c, score
		}
	}
	return best, true, nil
}

// moveOrganizeData copies everything recorded on file `from` onto `to`, then drops `from`.
func (s *Store) moveOrganizeData(ctx context.Context, from, to moveCandidate) error {
	steps := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO file_tags (file_id, tag_id) SELECT $2, tag_id FROM file_tags WHERE file_id = $1 ON CONFLICT DO NOTHING`,
			[]any{from.ID, to.ID}},
		// Notes and provenance: keep what the new file already has, fill the gaps from the old one.
		{`INSERT INTO file_meta (file_id, notes, source_url, designer, license)
		  SELECT $2, notes, source_url, designer, license FROM file_meta WHERE file_id = $1
		  ON CONFLICT (file_id) DO UPDATE SET
			notes = CASE WHEN file_meta.notes = '' THEN EXCLUDED.notes ELSE file_meta.notes END,
			source_url = CASE WHEN file_meta.source_url = '' THEN EXCLUDED.source_url ELSE file_meta.source_url END,
			designer = CASE WHEN file_meta.designer = '' THEN EXCLUDED.designer ELSE file_meta.designer END,
			license = CASE WHEN file_meta.license = '' THEN EXCLUDED.license ELSE file_meta.license END,
			updated_at = now()`,
			[]any{from.ID, to.ID}},
		{`INSERT INTO collection_files (collection_id, file_id, added_at)
		  SELECT collection_id, $2, added_at FROM collection_files WHERE file_id = $1 ON CONFLICT DO NOTHING`,
			[]any{from.ID, to.ID}},
		{`UPDATE print_items SET file_id = $2
		  WHERE file_id = $1 AND NOT EXISTS (
			SELECT 1 FROM print_items o WHERE o.file_id = $2 AND o.list_id = print_items.list_id)`,
			[]any{from.ID, to.ID}},
		// One cover per file: if the new file already has one, the old one is just a photo.
		{`UPDATE file_photos SET is_cover = false
		  WHERE file_id = $1 AND is_cover AND EXISTS (SELECT 1 FROM file_photos WHERE file_id = $2 AND is_cover)`,
			[]any{from.ID, to.ID}},
		{`UPDATE file_photos SET file_id = $2 WHERE file_id = $1`, []any{from.ID, to.ID}},
		// A folder cover follows its file, and the folder with it when the file sat directly inside.
		{`DELETE FROM folder_covers c WHERE c.file_id = $1 AND c.dir = $2 AND EXISTS (
			SELECT 1 FROM folder_covers o WHERE o.source_id = $3 AND o.dir = $4
			  AND (o.source_id, o.dir) <> (c.source_id, c.dir))`,
			[]any{from.ID, from.Dir, to.SourceID, to.Dir}},
		{`UPDATE folder_covers SET file_id = $2, source_id = $4, dir = $5 WHERE file_id = $1 AND dir = $3`,
			[]any{from.ID, to.ID, from.Dir, to.SourceID, to.Dir}},
		{`UPDATE folder_covers SET file_id = $2 WHERE file_id = $1`, []any{from.ID, to.ID}},
		{`DELETE FROM files WHERE id = $1`, []any{from.ID}},
	}
	for _, st := range steps {
		if _, err := s.db.Exec(ctx, st.sql, st.args...); err != nil {
			return err
		}
	}
	return nil
}
