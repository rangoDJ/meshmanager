package store

import (
	"context"
	"time"
)

type Session struct {
	ID         int64
	UserID     int64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
}

func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time, userAgent, ip string) (Session, error) {
	sess := Session{UserID: userID, ExpiresAt: expiresAt}
	err := s.db.QueryRow(ctx, `
		INSERT INTO auth_sessions (token_hash, user_id, expires_at, user_agent, ip)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, last_seen_at`,
		tokenHash, userID, expiresAt, truncate(userAgent, 255), truncate(ip, 64),
	).Scan(&sess.ID, &sess.CreatedAt, &sess.LastSeenAt)
	return sess, err
}

// GetSessionUser returns the unexpired session for a token hash, with its user.
func (s *Store) GetSessionUser(ctx context.Context, tokenHash string) (Session, User, error) {
	var sess Session
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT s.id, s.user_id, s.created_at, s.expires_at, s.last_seen_at,
		       u.id, u.username, u.password_hash, u.is_admin, u.created_at, u.last_login_at
		FROM auth_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash,
	).Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.ExpiresAt, &sess.LastSeenAt,
		&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt, &u.LastLoginAt)
	return sess, u, notFound(err)
}

func (s *Store) TouchSession(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, "UPDATE auth_sessions SET last_seen_at = now() WHERE id = $1", id)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, "DELETE FROM auth_sessions WHERE id = $1", id)
	return err
}

func (s *Store) DeleteOtherSessions(ctx context.Context, userID, keepID int64) error {
	_, err := s.db.Exec(ctx, "DELETE FROM auth_sessions WHERE user_id = $1 AND id <> $2", userID, keepID)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.Exec(ctx, "DELETE FROM auth_sessions WHERE user_id = $1", userID)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, "DELETE FROM auth_sessions WHERE expires_at <= now()")
	return tag.RowsAffected(), err
}

func truncate(v string, n int) string {
	if len(v) <= n {
		return v
	}
	// Back off to a rune boundary so Postgres doesn't reject invalid UTF-8.
	for n > 0 && v[n]&0xC0 == 0x80 {
		n--
	}
	return v[:n]
}
