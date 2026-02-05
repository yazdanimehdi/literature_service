// Package database provides database connectivity and management for the literature review service.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/helixir/literature-review-service/internal/config"
)

// Database operational constants.
const (
	// HealthCheckTimeout is the maximum time to wait for a health check ping.
	HealthCheckTimeout = 5 * time.Second
)

// HealthStatus contains database health information.
type HealthStatus struct {
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
	TotalConns       int32  `json:"total_conns"`
	AcquiredConns    int32  `json:"acquired_conns"`
	IdleConns        int32  `json:"idle_conns"`
	ConstructingConns int32 `json:"constructing_conns"`
	MaxConns         int32  `json:"max_conns"`
}

// DB represents the database connection pool.
type DB struct {
	pool   *pgxpool.Pool
	config *config.DatabaseConfig
	logger zerolog.Logger
}

// DBTX is an interface that both *pgxpool.Pool and pgx.Tx satisfy.
// This allows repositories to work with both direct pool connections and transactions.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// Compile-time check that *DB implements DBTX.
var _ DBTX = (*DB)(nil)

// New creates a new database connection pool.
func New(ctx context.Context, cfg *config.DatabaseConfig, logger zerolog.Logger) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Configure pool settings
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = cfg.HealthCheckPeriod

	// Configure connection settings
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	// Add logging hooks
	poolConfig.BeforeAcquire = func(ctx context.Context, conn *pgx.Conn) bool {
		logger.Trace().Msg("acquiring connection from pool")
		return true
	}

	poolConfig.AfterRelease = func(conn *pgx.Conn) bool {
		logger.Trace().Msg("releasing connection to pool")
		return true
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Str("database", cfg.Name).
		Int32("max_conns", cfg.MaxConns).
		Int32("min_conns", cfg.MinConns).
		Msg("database connection pool established")

	return &DB{
		pool:   pool,
		config: cfg,
		logger: logger,
	}, nil
}

// Pool returns the underlying connection pool.
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// Close closes the database connection pool.
func (db *DB) Close() {
	if db.pool != nil {
		db.pool.Close()
		db.logger.Info().Msg("database connection pool closed")
	}
}

// Ping verifies the database connection is alive.
func (db *DB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Stats returns pool statistics.
func (db *DB) Stats() *pgxpool.Stat {
	return db.pool.Stat()
}

// Health returns database health information as a typed struct.
func (db *DB) Health(ctx context.Context) HealthStatus {
	stat := db.pool.Stat()
	health := HealthStatus{
		TotalConns:        stat.TotalConns(),
		AcquiredConns:     stat.AcquiredConns(),
		IdleConns:         stat.IdleConns(),
		ConstructingConns: stat.ConstructingConns(),
		MaxConns:          stat.MaxConns(),
	}

	// Check if we can ping
	pingCtx, cancel := context.WithTimeout(ctx, HealthCheckTimeout)
	defer cancel()
	if err := db.pool.Ping(pingCtx); err != nil {
		health.Status = "unhealthy"
		health.Error = err.Error()
	} else {
		health.Status = "healthy"
	}

	return health
}

// WithTransaction executes a function within a database transaction.
// If the function returns an error, the transaction is rolled back.
// If the function completes successfully, the transaction is committed.
func (db *DB) WithTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return db.WithTransactionOptions(ctx, pgx.TxOptions{}, fn)
}

// WithSerializableTransaction executes a function within a serializable transaction.
func (db *DB) WithSerializableTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return db.WithTransactionOptions(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	}, fn)
}

// WithRepeatableReadTransaction executes a function within a repeatable read transaction.
func (db *DB) WithRepeatableReadTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return db.WithTransactionOptions(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead,
	}, fn)
}

// WithReadOnlyTransaction executes a function within a read-only transaction.
func (db *DB) WithReadOnlyTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return db.WithTransactionOptions(ctx, pgx.TxOptions{
		AccessMode: pgx.ReadOnly,
	}, fn)
}

// WithTransactionOptions executes a function within a transaction with custom options.
func (db *DB) WithTransactionOptions(ctx context.Context, opts pgx.TxOptions, fn func(tx pgx.Tx) error) error {
	tx, err := db.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			// Attempt rollback on panic
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				db.logger.Error().
					Err(rbErr).
					Interface("panic", p).
					Msg("failed to rollback transaction after panic")
			}
			panic(p) // Re-throw panic after rollback
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			db.logger.Error().
				Err(rbErr).
				AnErr("original_error", err).
				Msg("failed to rollback transaction")
			return fmt.Errorf("transaction error: %w (rollback error: %v)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Exec executes a query without returning any rows.
// This method implements the DBTX interface.
func (db *DB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, sql, args...)
}

// QueryRow executes a query that is expected to return at most one row.
// This method implements the DBTX interface.
func (db *DB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.pool.QueryRow(ctx, sql, args...)
}

// Query executes a query that returns rows.
// This method implements the DBTX interface.
func (db *DB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.pool.Query(ctx, sql, args...)
}

// SendBatch sends a batch of queries to the database.
// This method implements the DBTX interface.
func (db *DB) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	return db.pool.SendBatch(ctx, batch)
}

// AcquireAdvisoryLock acquires an advisory lock with the given key.
// Returns true if the lock was acquired, false if it was already held.
func (db *DB) AcquireAdvisoryLock(ctx context.Context, key int64) (bool, error) {
	var acquired bool
	err := db.pool.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired)
	return acquired, err
}

// ReleaseAdvisoryLock releases an advisory lock with the given key.
func (db *DB) ReleaseAdvisoryLock(ctx context.Context, key int64) error {
	_, err := db.pool.Exec(ctx, "SELECT pg_advisory_unlock($1)", key)
	return err
}

// AcquireAdvisoryLockTx acquires a transaction-scoped advisory lock.
// The lock is automatically released when the transaction ends.
func (db *DB) AcquireAdvisoryLockTx(ctx context.Context, tx pgx.Tx, key int64) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", key)
	return err
}

// TryAcquireAdvisoryLockTx tries to acquire a transaction-scoped advisory lock.
// Returns true if the lock was acquired, false if it was already held.
func (db *DB) TryAcquireAdvisoryLockTx(ctx context.Context, tx pgx.Tx, key int64) (bool, error) {
	var acquired bool
	err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock($1)", key).Scan(&acquired)
	return acquired, err
}
