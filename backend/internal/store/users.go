package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	IsAdmin      bool       `json:"is_admin"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at"`
}

const userCols = "id, username, password_hash, is_admin, created_at, last_login_at"

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt, &u.LastLoginAt)
	return u, notFound(err)
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string, isAdmin bool) (User, error) {
	u, err := scanUser(s.db.QueryRow(ctx,
		"INSERT INTO users (username, password_hash, is_admin) VALUES ($1, $2, $3) RETURNING "+userCols,
		username, passwordHash, isAdmin))
	if isUniqueViolation(err) {
		return u, ErrConflict
	}
	return u, err
}

// CreateFirstAdmin creates the admin account only while the users table is empty.
// The table lock makes concurrent first-run setups safe.
func (s *Store) CreateFirstAdmin(ctx context.Context, username, passwordHash string) (User, error) {
	var u User
	err := s.InTx(ctx, func(q *Store, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "LOCK TABLE users IN EXCLUSIVE MODE"); err != nil {
			return err
		}
		n, err := q.CountUsers(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrSetupDone
		}
		u, err = q.CreateUser(ctx, username, passwordHash, true)
		return err
	})
	return u, err
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	return scanUser(s.db.QueryRow(ctx,
		"SELECT "+userCols+" FROM users WHERE lower(username) = lower($1)", username))
}

func (s *Store) SetPassword(ctx context.Context, userID int64, passwordHash string) error {
	_, err := s.db.Exec(ctx, "UPDATE users SET password_hash = $2 WHERE id = $1", userID, passwordHash)
	return err
}

func (s *Store) TouchLogin(ctx context.Context, userID int64) error {
	_, err := s.db.Exec(ctx, "UPDATE users SET last_login_at = now() WHERE id = $1", userID)
	return err
}
