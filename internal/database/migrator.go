// Package database provides database connectivity and management for the literature review service.
package database

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog"
)

// Migrator handles database migrations.
type Migrator struct {
	migrate *migrate.Migrate
	sqlDB   *sql.DB // sql.DB wrapper around pgx pool, must be closed
	logger  zerolog.Logger
}

// NewMigrator creates a new migrator instance.
// It requires a valid database connection and a path to the migrations directory.
func NewMigrator(db *DB, migrationsPath string, logger zerolog.Logger) (*Migrator, error) {
	// Validate inputs
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	if db.pool == nil {
		return nil, fmt.Errorf("database pool not initialized")
	}
	if migrationsPath == "" {
		return nil, fmt.Errorf("migrations path is required")
	}

	// Validate migrations path exists before creating database connections
	if _, err := os.Stat(migrationsPath); err != nil {
		return nil, fmt.Errorf("migrations path validation failed: %w", err)
	}

	// Get a standard database/sql connection from pgx pool
	sqlDB := stdlib.OpenDBFromPool(db.pool)

	// Create the postgres driver
	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{
		MigrationsTable: "schema_migrations",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres driver: %w", err)
	}

	// Create the migrate instance
	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"postgres",
		driver,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrator: %w", err)
	}

	return &Migrator{
		migrate: m,
		sqlDB:   sqlDB,
		logger:  logger,
	}, nil
}

// Up runs all pending migrations.
func (m *Migrator) Up() error {
	m.logger.Info().Msg("running database migrations...")

	if err := m.migrate.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			m.logger.Info().Msg("no migrations to apply")
			return nil
		}
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	m.logger.Info().Msg("migrations completed successfully")
	return nil
}

// Down rolls back all migrations.
func (m *Migrator) Down() error {
	m.logger.Warn().Msg("rolling back all migrations...")

	if err := m.migrate.Down(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			m.logger.Info().Msg("no migrations to roll back")
			return nil
		}
		return fmt.Errorf("failed to rollback migrations: %w", err)
	}

	m.logger.Info().Msg("migrations rolled back successfully")
	return nil
}

// Steps runs n migrations (positive = up, negative = down).
func (m *Migrator) Steps(n int) error {
	m.logger.Info().Int("steps", n).Msg("running migration steps...")

	if err := m.migrate.Steps(n); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			m.logger.Info().Msg("no migrations to apply")
			return nil
		}
		// Handle "file does not exist" which occurs when at latest version
		if errors.Is(err, os.ErrNotExist) {
			m.logger.Info().Msg("no more migrations available")
			return nil
		}
		return fmt.Errorf("failed to run migration steps: %w", err)
	}

	m.logger.Info().Int("steps", n).Msg("migration steps completed successfully")
	return nil
}

// Version returns the current migration version.
func (m *Migrator) Version() (uint, bool, error) {
	return m.migrate.Version()
}

// Force sets the migration version without running migrations.
// This is useful for recovering from failed migrations.
func (m *Migrator) Force(version int) error {
	m.logger.Warn().Int("version", version).Msg("forcing migration version...")
	return m.migrate.Force(version)
}

// Close closes the migrator and releases resources.
// If both source and database close operations fail, both errors are combined.
func (m *Migrator) Close() error {
	sourceErr, dbErr := m.migrate.Close()

	// Close the sql.DB wrapper to release connections back to the pool
	if m.sqlDB != nil {
		if err := m.sqlDB.Close(); err != nil && dbErr == nil {
			dbErr = err
		}
	}

	if sourceErr != nil && dbErr != nil {
		return fmt.Errorf("failed to close migrator: source error: %v, database error: %w", sourceErr, dbErr)
	}
	if sourceErr != nil {
		return fmt.Errorf("failed to close source: %w", sourceErr)
	}
	if dbErr != nil {
		return fmt.Errorf("failed to close database: %w", dbErr)
	}
	return nil
}

// DropAll drops all tables in the database.
// WARNING: This is destructive and should only be used in testing.
func (m *Migrator) DropAll() error {
	m.logger.Warn().Msg("dropping all database objects...")
	return m.migrate.Drop()
}
