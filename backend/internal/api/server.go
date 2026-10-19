// Package api implements the HTTP JSON API under /api and mounts the frontend.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/rangoDJ/meshmanager/internal/auth"
	"github.com/rangoDJ/meshmanager/internal/config"
	"github.com/rangoDJ/meshmanager/internal/settings"
	"github.com/rangoDJ/meshmanager/internal/store"
)

const maxBodyBytes = 1 << 20

type Server struct {
	cfg      config.Config
	store    *store.Store
	queue    *river.Client[pgx.Tx]
	settings *settings.Store
	web      http.Handler
	limiter  *auth.RateLimiter
	// geometrySlots bounds concurrent model conversions for the viewer (memory heavy).
	geometrySlots chan struct{}
}

type Deps struct {
	Config config.Config
	Store  *store.Store
	Queue  *river.Client[pgx.Tx]
	// Settings holds the user-editable app settings (DATA_DIR/config.json).
	Settings *settings.Store
	// Web serves the frontend; nil disables it.
	Web http.Handler
}

func New(d Deps) *Server {
	return &Server{
		cfg:      d.Config,
		store:    d.Store,
		queue:    d.Queue,
		settings: d.Settings,
		web:      d.Web,
		limiter:  auth.NewRateLimiter(10, 5*time.Minute),

		geometrySlots: make(chan struct{}, 2),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)

	mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/setup", s.handleSetup)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.Handle("POST /api/auth/logout", s.requireAuth(s.handleLogout))
	mux.Handle("GET /api/auth/me", s.requireAuth(s.handleMe))
	mux.Handle("POST /api/auth/password", s.requireAuth(s.handleChangePassword))

	mux.Handle("GET /api/system", s.requireAuth(s.handleSystem))
	mux.Handle("GET /api/settings", s.requireAuth(s.handleGetSettings))
	mux.Handle("PUT /api/settings", s.requireAuth(s.handleSaveSettings))

	mux.Handle("GET /api/jobs", s.requireAuth(s.handleListJobs))
	mux.Handle("GET /api/jobs/{id}", s.requireAuth(s.handleGetJob))
	mux.Handle("POST /api/jobs/ping", s.requireAuth(s.handlePingJob))

	mux.Handle("GET /api/fs/dirs", s.requireAuth(s.handleBrowseDirs))
	mux.Handle("GET /api/sources", s.requireAuth(s.handleListSources))
	mux.Handle("POST /api/sources", s.requireAuth(s.handleCreateSource))
	mux.Handle("PATCH /api/sources/{id}", s.requireAuth(s.handleUpdateSource))
	mux.Handle("DELETE /api/sources/{id}", s.requireAuth(s.handleDeleteSource))
	mux.Handle("POST /api/sources/{id}/scan", s.requireAuth(s.handleScanSource))
	mux.Handle("GET /api/sources/{id}/tree", s.requireAuth(s.handleSourceTree))

	mux.Handle("GET /api/files", s.requireAuth(s.handleListFiles))
	mux.Handle("GET /api/files/facets", s.requireAuth(s.handleFacets))
	mux.Handle("GET /api/files/{id}", s.requireAuth(s.handleGetFile))
	mux.Handle("GET /api/files/{id}/thumbnail", s.requireAuth(s.handleThumbnail))
	mux.Handle("GET /api/files/{id}/download", s.requireAuth(s.handleDownload))
	mux.Handle("GET /api/files/{id}/geometry", s.requireAuth(s.handleGeometry))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	if s.web != nil {
		mux.Handle("/", s.web)
	}

	return recoverer(logRequests(securityHeaders(csrfGuard(mux))))
}

// clientIP returns the caller's address. Behind a trusted proxy it takes the last
// X-Forwarded-For hop, which is the one the proxy itself appended.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) isHTTPS(r *http.Request) bool {
	switch s.cfg.CookieSecure {
	case "true":
		return true
	case "false":
		return false
	}
	return r.TLS != nil || (s.cfg.TrustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func internalError(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil && errors.Is(err, context.Canceled) {
		// The client went away (e.g. a thumbnail tile scrolled out of view); nothing to report.
		return
	}
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

// decodeJSON reads a JSON body into dst and writes a 400 on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return false
	}
	return true
}
