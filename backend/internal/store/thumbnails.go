package store

import (
	"context"
	"time"
)

const (
	ThumbReady  = "ready"
	ThumbFailed = "failed"
)

type Thumbnail struct {
	FileID          int64
	Status          string
	Path            string // relative to DATA_DIR
	ContentType     string
	Width, Height   int
	Origin          string
	Error           string
	RendererVersion int
	CreatedAt       time.Time
}

func (s *Store) GetThumbnail(ctx context.Context, fileID int64) (Thumbnail, error) {
	var t Thumbnail
	var path, ct, origin, errMsg *string
	var w, h *int
	err := s.db.QueryRow(ctx, `
		SELECT file_id, status, path, content_type, width, height, origin, error, renderer_version, created_at
		FROM thumbnails WHERE file_id = $1`, fileID,
	).Scan(&t.FileID, &t.Status, &path, &ct, &w, &h, &origin, &errMsg, &t.RendererVersion, &t.CreatedAt)
	if err != nil {
		return t, notFound(err)
	}
	t.Path, t.ContentType, t.Origin, t.Error = deref(path), deref(ct), deref(origin), deref(errMsg)
	if w != nil && h != nil {
		t.Width, t.Height = *w, *h
	}
	return t, nil
}

func (s *Store) SaveThumbnail(ctx context.Context, t Thumbnail) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO thumbnails (file_id, status, path, content_type, width, height, origin, error, renderer_version)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, 0), NULLIF($6, 0), NULLIF($7, ''), NULLIF($8, ''), $9)
		ON CONFLICT (file_id) DO UPDATE SET
			status = EXCLUDED.status, path = EXCLUDED.path, content_type = EXCLUDED.content_type,
			width = EXCLUDED.width, height = EXCLUDED.height, origin = EXCLUDED.origin,
			error = EXCLUDED.error, renderer_version = EXCLUDED.renderer_version, created_at = now()`,
		t.FileID, t.Status, t.Path, t.ContentType, t.Width, t.Height, t.Origin, truncate(t.Error, 500), t.RendererVersion)
	return err
}

type ModelInfo struct {
	FileID    int64
	Triangles int64
	Size      [3]float64
	Volume    float64
	Area      float64
}

func (s *Store) SaveModelInfo(ctx context.Context, m ModelInfo) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO model_info (file_id, triangles, size_x, size_y, size_z, volume_mm3, area_mm2)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (file_id) DO UPDATE SET
			triangles = EXCLUDED.triangles, size_x = EXCLUDED.size_x, size_y = EXCLUDED.size_y,
			size_z = EXCLUDED.size_z, volume_mm3 = EXCLUDED.volume_mm3, area_mm2 = EXCLUDED.area_mm2,
			analyzed_at = now()`,
		m.FileID, m.Triangles, m.Size[0], m.Size[1], m.Size[2], m.Volume, m.Area)
	return err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
