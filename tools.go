//go:build tools
// +build tools

// Package tools imports dependencies that are used by this project but not directly
// imported in the main codebase. This ensures they are tracked in go.mod.
package tools

import (
	// Configuration
	_ "github.com/spf13/viper"

	// Logging
	_ "github.com/rs/zerolog"

	// Database
	_ "github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"

	// Utilities
	_ "github.com/google/uuid"
	_ "github.com/go-playground/validator/v10"

	// gRPC and protobuf
	_ "google.golang.org/grpc"
	_ "google.golang.org/protobuf/proto"

	// Temporal
	_ "go.temporal.io/sdk/client"
	_ "go.temporal.io/sdk/worker"
	_ "go.temporal.io/sdk/workflow"
	_ "go.temporal.io/api/enums/v1"

	// Kafka
	_ "github.com/segmentio/kafka-go"

	// Rate limiting
	_ "golang.org/x/time/rate"

	// Metrics
	_ "github.com/prometheus/client_golang/prometheus"

	// Testing
	_ "github.com/stretchr/testify/assert"
	_ "github.com/stretchr/testify/require"
	_ "github.com/testcontainers/testcontainers-go"
	_ "github.com/testcontainers/testcontainers-go/modules/postgres"

	// Shared packages
	_ "github.com/helixir/grpcauth"
	_ "github.com/helixir/outbox"
)
