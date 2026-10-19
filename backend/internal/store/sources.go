package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type Source struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Path               string     `json:"path"`
	ThumbnailMode      string     `json:"thumbnail_mode"`
	Enabled            bool       `json:"enabled"`
	LastScanJobID      *int64     `json:"last_scan_job_id"`
	LastScanFinishedAt *time.Time `json:"last_scan_finished_at"`
	CreatedAt          time.Time  `json:"created_at"`

	FileCount  int64 `json:"file_count"`
	ModelCount int64 `json:"model_count"`
	TotalSize  int64 `json:"total_size"`
	LastScan   *Job  `json:"last_scan"`
}

const sourceQuery = `
	SELECT s.id, s.name, s.path, s.thumbnail_mode, s.enabled, s.last_scan_job_id,
	       s.last_scan_finished_at, s.created_at,
	       coalesce(c.files, 0), coalesce(c.models, 0), coalesce(c.bytes, 0)
	FROM sources s
	LEFT JOIN LATERAL (
		SELECT count(*) AS files, count(*) FILTER (WHERE kind = 'model') AS models, sum(size) AS bytes
		FROM files WHERE source_id = s.id AND status = 'present'
	) c ON true`

func scanSource(row pgx.Row) (Source, error) {
	var s Source
	err := row.Scan(&s.ID, &s.Name, &s.Path, &s.ThumbnailMode, &s.Enabled, &s.LastScanJobID,
		&s.LastScanFinishedAt, &s.CreatedAt, &s.FileCount, &s.ModelCount, &s.TotalSize)
	return s, notFound(err)
}

func (s *Store) ListSources(ctx context.Context) ([]Source, error) {
	rows, err := s.db.Query(ctx, sourceQuery+" ORDER BY lower(s.name), s.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Source{}
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, src)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range list {
		if err := s.attachLastScan(ctx, &list[i]); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func (s *Store) GetSource(ctx context.Context, id int64) (Source, error) {
	src, err := scanSource(s.db.QueryRow(ctx, sourceQuery+" WHERE s.id = $1", id))
	if err != nil {
		return src, err
	}
	return src, s.attachLastScan(ctx, &src)
}

func (s *Store) attachLastScan(ctx context.Context, src *Source) error {
	if src.LastScanJobID == nil {
		return nil
	}
	j, err := s.GetJob(ctx, *src.LastScanJobID)
	if err == ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	src.LastScan = &j
	return nil
}

func (s *Store) CreateSource(ctx context.Context, name, path, thumbnailMode string) (Source, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		"INSERT INTO sources (name, path, thumbnail_mode) VALUES ($1, $2, $3) RETURNING id",
		name, path, thumbnailMode).Scan(&id)
	if isUniqueViolation(err) {
		return Source{}, ErrConflict
	}
	if err != nil {
		return Source{}, err
	}
	return s.GetSource(ctx, id)
}

type SourceUpdate struct {
	Name          *string
	ThumbnailMode *string
	Enabled       *bool
}

func (s *Store) UpdateSource(ctx context.Context, id int64, u SourceUpdate) (Source, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE sources SET
			name = coalesce($2, name),
			thumbnail_mode = coalesce($3, thumbnail_mode),
			enabled = coalesce($4, enabled)
		WHERE id = $1`, id, u.Name, u.ThumbnailMode, u.Enabled)
	if err != nil {
		return Source{}, err
	}
	if tag.RowsAffected() == 0 {
		return Source{}, ErrNotFound
	}
	return s.GetSource(ctx, id)
}

func (s *Store) DeleteSource(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM sources WHERE id = $1", id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// NextScanGeneration starts a new scan generation for the source.
func (s *Store) NextScanGeneration(ctx context.Context, id int64) (int64, error) {
	var gen int64
	err := s.db.QueryRow(ctx,
		"UPDATE sources SET scan_generation = scan_generation + 1 WHERE id = $1 RETURNING scan_generation",
		id).Scan(&gen)
	return gen, notFound(err)
}

// LockSourceLastScan locks the source row for the rest of the transaction and returns
// its most recent scan's tracking job, if any.
func (s *Store) LockSourceLastScan(ctx context.Context, sourceID int64) (*Job, error) {
	var jobID *int64
	err := s.db.QueryRow(ctx, "SELECT last_scan_job_id FROM sources WHERE id = $1 FOR UPDATE", sourceID).Scan(&jobID)
	if err != nil {
		return nil, notFound(err)
	}
	if jobID == nil {
		return nil, nil
	}
	j, err := s.GetJob(ctx, *jobID)
	if err == ErrNotFound {
		return nil, nil
	}
	return &j, err
}

// ScanJobAlive reports whether River still has a pending or running scan job for the
// tracking row trackingID.
func (s *Store) ScanJobAlive(ctx context.Context, trackingID int64) (bool, error) {
	var alive bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM river_job
			WHERE kind = 'scan' AND (args->>'job_id')::bigint = $1
			  AND state IN ('available', 'pending', 'retryable', 'running', 'scheduled')
		)`, trackingID).Scan(&alive)
	return alive, err
}

func (s *Store) SetSourceScanJob(ctx context.Context, id, jobID int64) error {
	_, err := s.db.Exec(ctx, "UPDATE sources SET last_scan_job_id = $2 WHERE id = $1", id, jobID)
	return err
}

func (s *Store) FinishSourceScan(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, "UPDATE sources SET last_scan_finished_at = now() WHERE id = $1", id)
	return err
}
