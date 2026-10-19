package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	JobQueued  = "queued"
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
)

type Job struct {
	ID         int64           `json:"id"`
	Kind       string          `json:"kind"`
	State      string          `json:"state"`
	Payload    json.RawMessage `json:"payload"`
	Progress   float64         `json:"progress"`
	Message    *string         `json:"message"`
	Error      *string         `json:"error"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  *time.Time      `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at"`
}

const jobCols = "id, kind, state, payload, progress, message, error, created_at, started_at, finished_at"

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.Kind, &j.State, &j.Payload, &j.Progress, &j.Message, &j.Error,
		&j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	return j, notFound(err)
}

func (s *Store) CreateJob(ctx context.Context, kind string, payload any) (Job, error) {
	raw := json.RawMessage("{}")
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return Job{}, err
		}
		raw = b
	}
	return scanJob(s.db.QueryRow(ctx,
		"INSERT INTO jobs (kind, payload) VALUES ($1, $2) RETURNING "+jobCols, kind, raw))
}

func (s *Store) GetJob(ctx context.Context, id int64) (Job, error) {
	return scanJob(s.db.QueryRow(ctx, "SELECT "+jobCols+" FROM jobs WHERE id = $1", id))
}

func (s *Store) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	rows, err := s.db.Query(ctx, "SELECT "+jobCols+" FROM jobs ORDER BY created_at DESC, id DESC LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *Store) StartJob(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE jobs SET state = 'running', started_at = now(), progress = 0, error = NULL
		WHERE id = $1`, id)
	return err
}

func (s *Store) SetJobProgress(ctx context.Context, id int64, progress float64, message string) error {
	_, err := s.db.Exec(ctx, "UPDATE jobs SET progress = $2, message = $3 WHERE id = $1", id, progress, message)
	return err
}

func (s *Store) FinishJob(ctx context.Context, id int64, state, message, errMsg string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE jobs SET state = $2, message = NULLIF($3, ''), error = NULLIF($4, ''),
		       progress = CASE WHEN $2 = 'done' THEN 1 ELSE progress END,
		       finished_at = now()
		WHERE id = $1`, id, state, message, errMsg)
	return err
}
