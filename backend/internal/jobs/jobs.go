// Package jobs defines background work run by the worker process through River.
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/rangoDJ/meshmanager/internal/store"
)

const KindPing = "ping"

// PingArgs is a smoke-test job that proves the API → queue → worker path works.
type PingArgs struct {
	JobID int64 `json:"job_id"`
}

func (PingArgs) Kind() string { return KindPing }

type CleanupSessionsArgs struct{}

func (CleanupSessionsArgs) Kind() string { return "cleanup_sessions" }

type PingWorker struct {
	river.WorkerDefaults[PingArgs]
	Store    *store.Store
	Hostname string
	Step     time.Duration
}

func (w *PingWorker) Work(ctx context.Context, job *river.Job[PingArgs]) error {
	id := job.Args.JobID
	if err := w.Store.StartJob(ctx, id); err != nil {
		return err
	}
	const steps = 5
	for i := 1; i <= steps; i++ {
		select {
		case <-ctx.Done():
			_ = w.Store.FinishJob(context.WithoutCancel(ctx), id, store.JobFailed, "", ctx.Err().Error())
			return ctx.Err()
		case <-time.After(w.Step):
		}
		msg := fmt.Sprintf("step %d of %d", i, steps)
		if err := w.Store.SetJobProgress(ctx, id, float64(i)/steps, msg); err != nil {
			return err
		}
	}
	return w.Store.FinishJob(ctx, id, store.JobDone, "pong from "+w.Hostname, "")
}

type CleanupSessionsWorker struct {
	river.WorkerDefaults[CleanupSessionsArgs]
	Store *store.Store
}

func (w *CleanupSessionsWorker) Work(ctx context.Context, _ *river.Job[CleanupSessionsArgs]) error {
	n, err := w.Store.DeleteExpiredSessions(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		slog.Info("deleted expired sessions", "count", n)
	}
	return nil
}

// NewInsertClient returns a client that can only enqueue jobs (used by the API).
func NewInsertClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), &river.Config{})
}

// NewWorkerClient returns a client that runs jobs (used by the worker process).
func NewWorkerClient(pool *pgxpool.Pool, st *store.Store, concurrency int, logger *slog.Logger) (*river.Client[pgx.Tx], error) {
	hostname, _ := os.Hostname()

	workers := river.NewWorkers()
	river.AddWorker(workers, &PingWorker{Store: st, Hostname: hostname, Step: 400 * time.Millisecond})
	river.AddWorker(workers, &CleanupSessionsWorker{Store: st})

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger:  logger,
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: concurrency},
		},
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return CleanupSessionsArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true},
			),
		},
	})
}
