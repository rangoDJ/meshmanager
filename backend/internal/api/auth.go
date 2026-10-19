package api

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/rangoDJ/meshmanager/internal/auth"
	"github.com/rangoDJ/meshmanager/internal/store"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authStatus struct {
	SetupRequired bool        `json:"setup_required"`
	User          *store.User `json:"user"`
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	st := authStatus{SetupRequired: n == 0}
	if _, u, err := s.sessionFromRequest(r); err == nil {
		st.User = &u
	}
	writeJSON(w, http.StatusOK, st)
}

// handleSetup creates the admin account on first run, while no users exist.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !decodeJSON(w, r, &c) {
		return
	}
	if msg := auth.ValidateUsername(c.Username); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if msg := auth.ValidatePassword(c.Password); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	hash, err := auth.HashPassword(c.Password)
	if err != nil {
		internalError(w, r, err)
		return
	}
	u, err := s.store.CreateFirstAdmin(r.Context(), c.Username, hash)
	if errors.Is(err, store.ErrSetupDone) {
		writeError(w, http.StatusConflict, "setup has already been completed")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	slog.Info("admin account created", "username", u.Username)
	if err := s.startSession(w, r, u); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if limited, wait := s.limiter.Limited(ip); limited {
		w.Header().Set("Retry-After", fmt.Sprint(int(math.Ceil(wait.Seconds()))))
		writeError(w, http.StatusTooManyRequests, "too many failed sign-in attempts, try again later")
		return
	}
	var c credentials
	if !decodeJSON(w, r, &c) {
		return
	}
	u, err := s.store.GetUserByUsername(r.Context(), c.Username)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		internalError(w, r, err)
		return
	}
	// u.PasswordHash is empty for an unknown user; VerifyPassword still does the work.
	if !auth.VerifyPassword(u.PasswordHash, c.Password) {
		s.limiter.Hit(ip)
		slog.Warn("failed sign-in", "username", c.Username, "ip", ip)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	s.limiter.Reset(ip)
	if err := s.store.TouchLogin(r.Context(), u.ID); err != nil {
		internalError(w, r, err)
		return
	}
	if err := s.startSession(w, r, u); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u store.User) error {
	token := auth.NewToken()
	expires := time.Now().Add(s.cfg.SessionTTL)
	if _, err := s.store.CreateSession(r.Context(), u.ID, auth.HashToken(token), expires, r.UserAgent(), s.clientIP(r)); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteSession(r.Context(), currentSession(r).ID); err != nil {
		internalError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleChangePassword updates the password and signs out every other session.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user := currentUser(r)
	if !auth.VerifyPassword(user.PasswordHash, req.CurrentPassword) {
		writeError(w, http.StatusBadRequest, "current password is incorrect")
		return
	}
	if msg := auth.ValidatePassword(req.NewPassword); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if err := s.store.SetPassword(r.Context(), user.ID, hash); err != nil {
		internalError(w, r, err)
		return
	}
	if err := s.store.DeleteOtherSessions(r.Context(), user.ID, currentSession(r).ID); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
