package store

import (
	"context"
	"time"
)

type WorkerInfo struct {
	ID         string    `json:"id"`
	Hostname   string    `json:"hostname"`
	Version    string    `json:"version"`
	StartedAt  time.Time `json:"started_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

func (s *Store) WorkerHeartbeat(ctx context.Context, w WorkerInfo) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO workers (id, hostname, version, started_at, last_seen_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (id) DO UPDATE SET last_seen_at = now()`,
		w.ID, w.Hostname, w.Version, w.StartedAt)
	return err
}

func (s *Store) RemoveWorker(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, "DELETE FROM workers WHERE id = $1", id)
	return err
}

// ListWorkers returns workers that sent a heartbeat within the window, and prunes older rows.
func (s *Store) ListWorkers(ctx context.Context, window time.Duration) ([]WorkerInfo, error) {
	secs := window.Seconds()
	if _, err := s.db.Exec(ctx,
		"DELETE FROM workers WHERE last_seen_at < now() - make_interval(secs => $1 * 10)", secs); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, hostname, version, started_at, last_seen_at FROM workers
		WHERE last_seen_at > now() - make_interval(secs => $1)
		ORDER BY started_at`, secs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	workers := []WorkerInfo{}
	for rows.Next() {
		var w WorkerInfo
		if err := rows.Scan(&w.ID, &w.Hostname, &w.Version, &w.StartedAt, &w.LastSeenAt); err != nil {
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, rows.Err()
}
