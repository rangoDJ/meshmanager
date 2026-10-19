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
	"github.com/riverqueue/river/rivertype"

	"github.com/rangoDJ/meshmanager/internal/config"
	"github.com/rangoDJ/meshmanager/internal/scan"
	"github.com/rangoDJ/meshmanager/internal/settings"
	"github.com/rangoDJ/meshmanager/internal/store"
)

const (
	KindPing  = "ping"
	KindScan  = "scan"
	QueueScan = "scan"

	// River priorities: 1 runs first. Thumbnails requested by someone looking at the
	// grid jump ahead of bulk (eager) thumbnail work.
	PriorityInteractive = 1
	PriorityBulk        = 3
)

// activeStates makes a job unique only while one is waiting or running, so the same
// work can be queued again once it has finished.
var activeStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRetryable,
	rivertype.JobStateRunning,
	rivertype.JobStateScheduled,
}

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

// --- scanning ----------------------------------------------------------------

type ScanArgs struct {
	SourceID int64 `json:"source_id"`
	JobID    int64 `json:"job_id"`
}

func (ScanArgs) Kind() string { return KindScan }

func (ScanArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueScan, MaxAttempts: 1}
}

// ErrScanInProgress means the source already has a scan queued or running.
var ErrScanInProgress = fmt.Errorf("a scan of this source is already queued or running")

// EnqueueScan queues a scan of a source together with its progress-tracking row.
//
// At most one scan per source is pending at a time. That is decided by the source's
// tracking row rather than River's unique jobs: River marks a job completed slightly
// after it returns, which would reject a "Scan now" right after a scan finished.
func EnqueueScan(ctx context.Context, st *store.Store, client *river.Client[pgx.Tx], src store.Source) (store.Job, error) {
	var job store.Job
	err := st.InTx(ctx, func(q *store.Store, tx pgx.Tx) error {
		last, err := q.LockSourceLastScan(ctx, src.ID) // serializes enqueues per source
		if err != nil {
			return err
		}
		if last != nil && (last.State == store.JobQueued || last.State == store.JobRunning) {
			alive, err := q.ScanJobAlive(ctx, last.ID)
			if err != nil {
				return err
			}
			if alive {
				return ErrScanInProgress
			}
			// Its worker died mid-scan (container killed): close the stale row and rescan.
			if err := q.FinishJob(ctx, last.ID, store.JobFailed, "", "interrupted"); err != nil {
				return err
			}
		}
		if job, err = q.CreateJob(ctx, KindScan, map[string]any{"source_id": src.ID, "source_name": src.Name}); err != nil {
			return err
		}
		if _, err := client.InsertTx(ctx, tx, ScanArgs{SourceID: src.ID, JobID: job.ID}, nil); err != nil {
			return err
		}
		return q.SetSourceScanJob(ctx, src.ID, job.ID)
	})
	return job, err
}

type ScanWorker struct {
	river.WorkerDefaults[ScanArgs]
	Store    *store.Store
	Settings *settings.Store
}

func (w *ScanWorker) Timeout(*river.Job[ScanArgs]) time.Duration { return 24 * time.Hour }

func (w *ScanWorker) Work(ctx context.Context, job *river.Job[ScanArgs]) error {
	id := job.Args.JobID
	src, err := w.Store.GetSource(ctx, job.Args.SourceID)
	if err == store.ErrNotFound {
		return w.Store.FinishJob(ctx, id, store.JobFailed, "", "source was removed")
	}
	if err != nil {
		return err
	}
	if err := w.Store.StartJob(ctx, id); err != nil {
		return err
	}

	// Progress writes are best-effort; a slow DB shouldn't fail the scan.
	progress := func(frac float64, msg string) {
		if err := w.Store.SetJobProgress(ctx, id, frac, msg); err != nil && ctx.Err() == nil {
			slog.Warn("scan progress", "err", err)
		}
	}
	started := time.Now()
	cur := w.Settings.Get()
	scanner := &scan.Scanner{Store: w.Store, HashConcurrency: cur.HashConcurrency, IndexVideos: cur.IndexVideos}
	res, err := scanner.Scan(ctx, src, progress)
	if err != nil {
		_ = w.Store.FinishJob(context.WithoutCancel(ctx), id, store.JobFailed, res.Summary(), err.Error())
		slog.Error("scan failed", "source", src.Name, "err", err)
		return nil
	}
	if err := w.Store.FinishSourceScan(ctx, src.ID); err != nil {
		return err
	}

	summary := res.Summary()
	if src.ThumbnailMode == "eager" {
		client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
		if err != nil {
			return err
		}
		n, err := EnqueueMissingThumbnails(ctx, w.Store, client, src.ID)
		if err != nil {
			return err
		}
		summary += fmt.Sprintf(", %d thumbnails queued", n)
	}
	slog.Info("scan finished", "source", src.Name, "duration", time.Since(started).Round(time.Millisecond), "result", summary)
	return w.Store.FinishJob(ctx, id, store.JobDone, summary, "")
}

// ScheduledScansArgs is the periodic job that rescans sources once their interval
// (from settings) has passed. It runs often and checks, rather than being scheduled at
// the interval itself, so a changed interval applies without a restart.
type ScheduledScansArgs struct{}

func (ScheduledScansArgs) Kind() string { return "scheduled_scans" }

// scheduledScanCheck is how often sources are checked for a due rescan.
const scheduledScanCheck = 15 * time.Minute

type ScheduledScansWorker struct {
	river.WorkerDefaults[ScheduledScansArgs]
	Store    *store.Store
	Settings *settings.Store
}

func (w *ScheduledScansWorker) Work(ctx context.Context, _ *river.Job[ScheduledScansArgs]) error {
	interval := w.Settings.Get().ScanInterval()
	if interval <= 0 {
		return nil
	}
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return err
	}
	sources, err := w.Store.ListSources(ctx)
	if err != nil {
		return err
	}
	for _, src := range sources {
		if !scanDue(src, interval, time.Now()) {
			continue
		}
		if _, err := EnqueueScan(ctx, w.Store, client, src); err != nil && err != ErrScanInProgress {
			return err
		}
	}
	return nil
}

// scanDue reports whether an enabled source's last scan attempt is older than interval.
// The attempt time (not the finish time) counts, so a failing source, such as an
// unmounted NAS share, is retried once per interval rather than every check.
func scanDue(src store.Source, interval time.Duration, now time.Time) bool {
	if !src.Enabled {
		return false
	}
	if src.LastScan == nil {
		return true
	}
	return now.Sub(src.LastScan.CreatedAt) >= interval
}

// --- thumbnails ---------------------------------------------------------------

type ThumbnailArgs struct {
	FileID int64 `json:"file_id" river:"unique"`
}

func (ThumbnailArgs) Kind() string { return "thumbnail" }

func (ThumbnailArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Priority:    PriorityBulk,
		MaxAttempts: 2,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeStates},
	}
}

// RequestThumbnail queues a thumbnail for someone waiting on it. If the file is already
// queued as bulk work, its priority is raised instead.
func RequestThumbnail(ctx context.Context, st *store.Store, client *river.Client[pgx.Tx], fileID int64) error {
	res, err := client.Insert(ctx, ThumbnailArgs{FileID: fileID}, &river.InsertOpts{Priority: PriorityInteractive})
	if err != nil {
		return err
	}
	if res.UniqueSkippedAsDuplicate {
		// River has no API to change priority, so update its job row directly.
		_, err = st.Pool().Exec(ctx, `
			UPDATE river_job SET priority = $2
			WHERE kind = 'thumbnail' AND state IN ('available', 'scheduled', 'retryable')
			  AND (args->>'file_id')::bigint = $1 AND priority > $2`, fileID, PriorityInteractive)
	}
	return err
}

// EnqueueMissingThumbnails queues bulk thumbnail jobs for every file of a source that
// can have a thumbnail but doesn't yet.
func EnqueueMissingThumbnails(ctx context.Context, st *store.Store, client *river.Client[pgx.Tx], sourceID int64) (int, error) {
	formatsList := thumbnailFormats()
	queued := 0
	var after int64
	for {
		ids, err := st.FilesNeedingThumbnails(ctx, sourceID, formatsList, after, 1000)
		if err != nil || len(ids) == 0 {
			return queued, err
		}
		params := make([]river.InsertManyParams, len(ids))
		for i, id := range ids {
			params[i] = river.InsertManyParams{Args: ThumbnailArgs{FileID: id}}
		}
		if _, err := client.InsertMany(ctx, params); err != nil {
			return queued, err
		}
		queued += len(ids)
		after = ids[len(ids)-1]
	}
}

// PurgeThumbnailsArgs deletes a removed source's thumbnail folder.
type PurgeThumbnailsArgs struct {
	SourceID int64 `json:"source_id"`
}

func (PurgeThumbnailsArgs) Kind() string { return "purge_thumbnails" }

type PurgeThumbnailsWorker struct {
	river.WorkerDefaults[PurgeThumbnailsArgs]
	DataDir string
}

func (w *PurgeThumbnailsWorker) Work(_ context.Context, job *river.Job[PurgeThumbnailsArgs]) error {
	return os.RemoveAll(SourceThumbDir(w.DataDir, job.Args.SourceID))
}

// --- clients -------------------------------------------------------------------

// NewInsertClient returns a client that can only enqueue jobs (used by the API).
func NewInsertClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), &river.Config{})
}

// NewWorkerClient returns a client that runs jobs (used by the worker process).
// It can also enqueue, so a process running both the API and jobs needs only this one.
func NewWorkerClient(pool *pgxpool.Pool, st *store.Store, cfg config.Config, set *settings.Store, logger *slog.Logger) (*river.Client[pgx.Tx], error) {
	hostname, _ := os.Hostname()

	workers := river.NewWorkers()
	river.AddWorker(workers, &PingWorker{Store: st, Hostname: hostname, Step: 400 * time.Millisecond})
	river.AddWorker(workers, &CleanupSessionsWorker{Store: st})
	river.AddWorker(workers, &ScanWorker{Store: st, Settings: set})
	river.AddWorker(workers, &ScheduledScansWorker{Store: st, Settings: set})
	river.AddWorker(workers, &ThumbnailWorker{Store: st, DataDir: cfg.DataDir, Settings: set})
	river.AddWorker(workers, &PurgeThumbnailsWorker{DataDir: cfg.DataDir})

	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return CleanupSessionsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(scheduledScanCheck),
			func() (river.JobArgs, *river.InsertOpts) { return ScheduledScansArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	}

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger:  logger,
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: max(1, cfg.WorkerConcurrency)},
			QueueScan:          {MaxWorkers: 2},
		},
		PeriodicJobs: periodic,
	})
}
