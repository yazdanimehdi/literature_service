// Package main provides a CLI tool for database migrations.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/helixir/literature-review-service/internal/config"
	"github.com/helixir/literature-review-service/internal/database"
	"github.com/helixir/literature-review-service/internal/observability"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Define CLI flags.
	up := flag.Bool("up", false, "Run all pending migrations")
	down := flag.Bool("down", false, "Roll back all migrations")
	steps := flag.Int("steps", 0, "Run N migration steps (positive=up, negative=down)")
	version := flag.Bool("version", false, "Print the current migration version")
	force := flag.Int("force", -1, "Force set migration version (use to recover from failed migrations)")
	migrationsPath := flag.String("path", "", "Override the migrations directory path")
	flag.Parse()

	// Validate that exactly one action is specified.
	actionCount := 0
	if *up {
		actionCount++
	}
	if *down {
		actionCount++
	}
	if *steps != 0 {
		actionCount++
	}
	if *version {
		actionCount++
	}
	if *force >= 0 {
		actionCount++
	}

	if actionCount == 0 {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\nPlease specify one of: -up, -down, -steps N, -version, -force V")
		return fmt.Errorf("no action specified")
	}

	if actionCount > 1 {
		return fmt.Errorf("specify only one action at a time")
	}

	// Load configuration (database settings from env/config file).
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Set up structured logging with console output for the CLI tool.
	logger := observability.NewLogger(observability.LoggingConfig{
		Level:      "info",
		Format:     "console",
		Output:     "stdout",
		AddSource:  false,
		TimeFormat: time.RFC3339,
	})
	logger = logger.With().Str("component", "migrate").Logger()

	// Allow CLI flag to override migration path.
	migrationDir := cfg.Database.MigrationPath
	if *migrationsPath != "" {
		migrationDir = *migrationsPath
	}

	// Connect to the database.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := database.New(ctx, &cfg.Database, logger)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()
	logger.Info().Msg("database connection established")

	// Create migrator.
	migrator, err := database.NewMigrator(db, migrationDir, logger)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		if closeErr := migrator.Close(); closeErr != nil {
			logger.Error().Err(closeErr).Msg("failed to close migrator")
		}
	}()

	// Execute the requested action.
	switch {
	case *up:
		logger.Info().Msg("running all pending migrations")
		if err := migrator.Up(); err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
		printVersion(migrator, logger)
		return nil

	case *down:
		logger.Warn().Msg("rolling back all migrations")
		if err := migrator.Down(); err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		printVersion(migrator, logger)
		return nil

	case *steps != 0:
		logger.Info().Int("steps", *steps).Msg("running migration steps")
		if err := migrator.Steps(*steps); err != nil {
			return fmt.Errorf("migrate steps: %w", err)
		}
		printVersion(migrator, logger)
		return nil

	case *version:
		printVersion(migrator, logger)
		return nil

	case *force >= 0:
		logger.Warn().Int("version", *force).Msg("forcing migration version")
		if err := migrator.Force(*force); err != nil {
			return fmt.Errorf("force version: %w", err)
		}
		printVersion(migrator, logger)
		return nil

	default:
		return fmt.Errorf("no action specified")
	}
}

// printVersion prints the current migration version to stdout.
func printVersion(migrator *database.Migrator, logger zerolog.Logger) {
	v, dirty, err := migrator.Version()
	if err != nil {
		logger.Warn().Err(err).Msg("could not determine migration version")
		return
	}
	logger.Info().
		Uint("version", v).
		Bool("dirty", dirty).
		Msg("current migration version")
}
