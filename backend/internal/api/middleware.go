package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/rangoDJ/meshmanager/internal/auth"
	"github.com/rangoDJ/meshmanager/internal/store"
)

const sessionCookie = "mm_session"

type ctxKey int

const (
	userKey ctxKey = iota
	sessionKey
)

func currentUser(r *http.Request) store.User {
	return r.Context().Value(userKey).(store.User)
}

func currentSession(r *http.Request) store.Session {
	return r.Context().Value(sessionKey).(store.Session)
}

// sessionFromRequest looks up the session named by the request's cookie.
func (s *Server) sessionFromRequest(r *http.Request) (store.Session, store.User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return store.Session{}, store.User{}, store.ErrNotFound
	}
	return s.store.GetSessionUser(r.Context(), auth.HashToken(c.Value))
}

func (s *Server) requireAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, user, err := s.sessionFromRequest(r)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "not signed in")
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		if time.Since(sess.LastSeenAt) > 5*time.Minute {
			if err := s.store.TouchSession(r.Context(), sess.ID); err != nil {
				slog.Warn("touch session", "err", err)
			}
		}
		ctx := context.WithValue(r.Context(), userKey, user)
		ctx = context.WithValue(ctx, sessionKey, sess)
		next(w, r.WithContext(ctx))
	})
}

// csrfGuard requires a custom header on state-changing API calls. Browsers can't send
// one cross-site without a CORS preflight, which this API never grants.
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("X-Requested-With") == "" {
				writeError(w, http.StatusForbidden, "missing X-Requested-With header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; "+
				"worker-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		level := slog.LevelInfo
		if r.URL.Path == "/api/health" || !strings.HasPrefix(r.URL.Path, "/api/") {
			level = slog.LevelDebug
		}
		slog.Log(r.Context(), level, "http",
			"method", r.Method, "path", r.URL.Path, "status", rec.status,
			"duration", time.Since(start).Round(time.Microsecond))
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.Error("panic", "method", r.Method, "path", r.URL.Path, "panic", v)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
