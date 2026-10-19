// Package store holds all SQL access. Methods run against the pool, or against a
// transaction when called on a Store returned by WithTx / InTx.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("already exists")
	ErrSetupDone = errors.New("setup already completed")
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Store struct {
	db   DBTX
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{db: pool, pool: pool}
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func (s *Store) WithTx(tx pgx.Tx) *Store {
	return &Store{db: tx, pool: s.pool}
}

// InTx runs fn in a new transaction, committing if it returns nil.
func (s *Store) InTx(ctx context.Context, fn func(q *Store, tx pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(s.WithTx(tx), tx)
	})
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
