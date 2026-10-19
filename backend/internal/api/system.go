package api

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rangoDJ/meshmanager/internal/jobs"
	"github.com/rangoDJ/meshmanager/internal/store"
	"github.com/rangoDJ/meshmanager/internal/version"
)

// handleHealth is public and used by the Docker healthcheck.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	resp := map[string]string{"status": "ok", "version": version.Version, "database": "ok"}
	if err := s.store.Ping(ctx); err != nil {
		resp["status"], resp["database"] = "unavailable", "unreachable"
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type systemInfo struct {
	Version   string             `json:"version"`
	GoVersion string             `json:"go_version"`
	Platform  string             `json:"platform"`
	Workers   []store.WorkerInfo `json:"workers"`
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	workers, err := s.store.ListWorkers(r.Context(), 30*time.Second)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, systemInfo{
		Version:   version.Version,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		Workers:   workers,
	})
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = n
	}
	list, err := s.store.ListJobs(r.Context(), limit)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	job, err := s.store.GetJob(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// handlePingJob enqueues a test job. The tracking row and the River job are inserted
// in one transaction, so neither exists without the other.
func (s *Server) handlePingJob(w http.ResponseWriter, r *http.Request) {
	var job store.Job
	err := s.store.InTx(r.Context(), func(q *store.Store, tx pgx.Tx) error {
		var err error
		if job, err = q.CreateJob(r.Context(), jobs.KindPing, nil); err != nil {
			return err
		}
		_, err = s.queue.InsertTx(r.Context(), tx, jobs.PingArgs{JobID: job.ID}, nil)
		return err
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}
