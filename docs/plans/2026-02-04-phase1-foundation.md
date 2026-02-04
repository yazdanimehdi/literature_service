# Literature Review Service - Phase 1 Foundation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Establish the foundational infrastructure for the Literature Review Service including project scaffolding, configuration system, database schema, proto generation, and domain models.

**Architecture:** Go microservice following existing Helixir patterns - using viper for configuration, pgxpool for PostgreSQL, golang-migrate for migrations, buf for protobuf generation, and shared packages (grpcauth, outbox) via go.work.

**Tech Stack:** Go 1.25.6, PostgreSQL, buf/protobuf, viper, zerolog, pgx/v5, golang-migrate/v4

---

## Phase 1 Deliverables Summary

| ID | Deliverable | Description |
|----|-------------|-------------|
| D1.1 | Project Scaffolding | Go module, directory structure, Makefile |
| D1.2 | Configuration System | Config struct, YAML loading, env overrides, validation |
| D1.3 | Database Schema | PostgreSQL schema with migrations |
| D1.4 | Proto Generation | Literature review protos with buf |
| D1.5 | Domain Models | Go structs matching schema |

---

## Task 1: Initialize Go Module and Project Structure

**Files:**
- Create: `literature_service/go.mod`
- Create: `literature_service/cmd/server/main.go`
- Create: `literature_service/cmd/worker/main.go`
- Create: `literature_service/cmd/migrate/main.go`
- Create: `literature_service/Makefile`
- Create: `literature_service/CLAUDE.md`

**Step 1: Create go.mod**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go mod init github.com/helixir/literature-review-service
```

Expected: `go.mod` file created

**Step 2: Create directory structure**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
mkdir -p cmd/server cmd/worker cmd/migrate
mkdir -p internal/config internal/domain internal/repository internal/database
mkdir -p internal/server/grpc internal/server/http
mkdir -p internal/temporal/workflows internal/temporal/activities
mkdir -p internal/llm internal/papersources internal/ratelimit internal/ingestion
mkdir -p internal/outbox internal/pagination internal/dedup internal/observability
mkdir -p api/proto/literaturereview/v1
mkdir -p gen/proto/literaturereview/v1
mkdir -p migrations
mkdir -p config
mkdir -p pkg/testutil
mkdir -p scripts
mkdir -p tests/integration tests/e2e
```

Expected: All directories created

**Step 3: Create placeholder main.go for server**

Create `cmd/server/main.go`:

```go
// Package main provides the entry point for the literature review service server.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("literature-review-service server starting...")
	// TODO: Implement server startup
	return nil
}
```

**Step 4: Create placeholder main.go for worker**

Create `cmd/worker/main.go`:

```go
// Package main provides the entry point for the literature review Temporal worker.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("literature-review-service worker starting...")
	// TODO: Implement Temporal worker startup
	return nil
}
```

**Step 5: Create placeholder main.go for migrate**

Create `cmd/migrate/main.go`:

```go
// Package main provides a CLI tool for database migrations.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("literature-review-service migrate tool")
	// TODO: Implement migration commands
	return nil
}
```

**Step 6: Create Makefile**

Create `Makefile`:

```makefile
# Literature Review Service Makefile

.PHONY: all build test lint clean proto migrate

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary names
BINARY_SERVER=literature-review-server
BINARY_WORKER=literature-review-worker
BINARY_MIGRATE=literature-review-migrate

# Build directories
BUILD_DIR=bin

# Default target
all: lint test build

# Build binaries
build: build-server build-worker build-migrate

build-server:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_SERVER) ./cmd/server

build-worker:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_WORKER) ./cmd/worker

build-migrate:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_MIGRATE) ./cmd/migrate

# Run tests
test:
	$(GOTEST) -v -race -cover ./...

# Run tests with coverage report
test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Lint
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Generate protobuf
proto:
	buf generate

# Lint protobuf
proto-lint:
	buf lint

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Run migrations up
migrate-up:
	$(BUILD_DIR)/$(BINARY_MIGRATE) up

# Run migrations down
migrate-down:
	$(BUILD_DIR)/$(BINARY_MIGRATE) down

# Create new migration
migrate-create:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir migrations -seq $$name

# Run server locally
run-server: build-server
	./$(BUILD_DIR)/$(BINARY_SERVER)

# Run worker locally
run-worker: build-worker
	./$(BUILD_DIR)/$(BINARY_WORKER)

# Development helpers
dev-setup:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/bufbuild/buf/cmd/buf@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

.DEFAULT_GOAL := all
```

**Step 7: Create CLAUDE.md**

Create `CLAUDE.md`:

```markdown
# Literature Review Service

## Overview

The Literature Review Service performs automated literature reviews by:
- Extracting keywords from natural language queries using LLM
- Searching multiple academic databases concurrently (Semantic Scholar, OpenAlex, Scopus, PubMed, bioRxiv, arXiv)
- Recursively expanding searches based on discovered paper keywords
- Managing paper ingestion through the Ingestion Service
- Publishing events to Kafka for observability

## Build & Test Commands

```bash
# Build all binaries
make build

# Run tests
make test

# Run tests with coverage
make test-coverage

# Lint code
make lint

# Generate protobuf
make proto

# Run migrations
make migrate-up
make migrate-down
```

## Project Structure

- `cmd/server/` - gRPC + HTTP server entrypoint
- `cmd/worker/` - Temporal worker entrypoint
- `cmd/migrate/` - Database migration tool
- `internal/config/` - Configuration loading
- `internal/domain/` - Domain models and errors
- `internal/repository/` - Database repository layer
- `internal/database/` - Database connection management
- `internal/server/` - gRPC and HTTP handlers
- `internal/temporal/` - Temporal workflows and activities
- `internal/llm/` - LLM client implementations
- `internal/papersources/` - Paper source API clients
- `api/proto/` - Protobuf definitions
- `gen/proto/` - Generated protobuf code
- `migrations/` - SQL migration files

## Conventions

- Use zerolog for structured logging
- Use viper for configuration
- Use pgxpool for PostgreSQL connections
- Follow existing Helixir patterns from ingestion_service
- Use shared packages: grpcauth, outbox
```

**Step 8: Verify build succeeds**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go mod tidy
go build ./...
```

Expected: Build succeeds with no errors

**Step 9: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add .
git commit -m "feat: initialize literature-review-service project structure

- Add go.mod with module github.com/helixir/literature-review-service
- Add cmd/server, cmd/worker, cmd/migrate placeholders
- Add directory structure following ingestion_service patterns
- Add Makefile with build, test, lint, proto targets
- Add CLAUDE.md with project overview"
```

---

## Task 2: Add Core Dependencies

**Files:**
- Modify: `literature_service/go.mod`

**Step 1: Add required dependencies**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service

# Core dependencies
go get github.com/spf13/viper@v1.21.0
go get github.com/rs/zerolog@v1.34.0
go get github.com/jackc/pgx/v5@v5.8.0
go get github.com/golang-migrate/migrate/v4@v4.19.1
go get github.com/google/uuid@v1.6.0
go get github.com/go-playground/validator/v10@v10.27.0

# gRPC and protobuf
go get google.golang.org/grpc@v1.78.0
go get google.golang.org/protobuf@v1.36.11
go get google.golang.org/genproto/googleapis/rpc@v0.0.0-20260128011058-8636f8732409

# Temporal
go get go.temporal.io/sdk@v1.39.0
go get go.temporal.io/api@v1.59.0

# Kafka (for outbox)
go get github.com/segmentio/kafka-go@v0.4.50

# Rate limiting
go get golang.org/x/time@v0.14.0

# Testing
go get github.com/stretchr/testify@v1.11.1
go get github.com/testcontainers/testcontainers-go@v0.40.0
go get github.com/testcontainers/testcontainers-go/modules/postgres@v0.40.0

# Prometheus metrics
go get github.com/prometheus/client_golang@v1.23.2

# For postgres driver with migrate
go get github.com/lib/pq@v1.10.9
```

**Step 2: Add local replace directives for shared packages**

Add to `go.mod` at the end:

```
replace github.com/helixir/grpcauth => ../grpcauth

replace github.com/helixir/outbox => ../outbox
```

**Step 3: Add shared package imports**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go get github.com/helixir/grpcauth@v0.0.0
go get github.com/helixir/outbox@v0.0.0
```

**Step 4: Tidy dependencies**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go mod tidy
```

**Step 5: Verify dependencies resolve**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go build ./...
```

Expected: Build succeeds

**Step 6: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add go.mod go.sum
git commit -m "chore: add core dependencies

- viper for configuration
- zerolog for logging
- pgx/v5 for PostgreSQL
- golang-migrate for migrations
- temporal SDK for workflows
- grpc and protobuf
- shared packages: grpcauth, outbox
- testing: testify, testcontainers"
```

---

## Task 3: Implement Configuration System

**Files:**
- Create: `literature_service/internal/config/config.go`
- Create: `literature_service/internal/config/config_test.go`
- Create: `literature_service/config/config.yaml`

**Step 1: Write failing test for config loading**

Create `internal/config/config_test.go`:

```go
package config_test

import (
	"os"
	"testing"

	"github.com/helixir/literature-review-service/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear any env vars that might interfere
	os.Clearenv()

	cfg, err := config.Load()
	require.NoError(t, err)

	// Server defaults
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.HTTPPort)
	assert.Equal(t, 9090, cfg.Server.GRPCPort)

	// Database defaults
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "require", cfg.Database.SSLMode)

	// Temporal defaults
	assert.Equal(t, "localhost:7233", cfg.Temporal.HostPort)
	assert.Equal(t, "literature-review", cfg.Temporal.Namespace)

	// LLM defaults
	assert.Equal(t, "openai", cfg.LLM.Provider)
	assert.Equal(t, 5, cfg.LLM.InitialKeywordCount)
	assert.Equal(t, 3, cfg.LLM.PaperKeywordCount)
	assert.Equal(t, 2, cfg.LLM.MaxExpansionDepth)
}

func TestLoad_EnvironmentOverride(t *testing.T) {
	os.Clearenv()
	os.Setenv("LITREVIEW_SERVER_HTTP_PORT", "9000")
	os.Setenv("LITREVIEW_DATABASE_HOST", "db.example.com")
	defer os.Clearenv()

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, 9000, cfg.Server.HTTPPort)
	assert.Equal(t, "db.example.com", cfg.Database.Host)
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort: -1,
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid http port")
}

func TestDatabaseConfig_DSN(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "litreview",
		Password: "secret",
		Name:     "literature_review",
		SSLMode:  "require",
	}

	dsn := cfg.DSN()
	assert.Contains(t, dsn, "postgres://")
	assert.Contains(t, dsn, "litreview")
	assert.Contains(t, dsn, "localhost:5432")
	assert.Contains(t, dsn, "literature_review")
	assert.Contains(t, dsn, "sslmode=require")
}
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/config/... -v
```

Expected: FAIL - package not found

**Step 3: Implement configuration**

Create `internal/config/config.go`:

```go
// Package config provides configuration management for the literature review service.
package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// SSL mode constants for database connections.
const (
	SSLModeDisable    = "disable"
	SSLModeRequire    = "require"
	SSLModeVerifyCA   = "verify-ca"
	SSLModeVerifyFull = "verify-full"
)

// Config holds all configuration for the literature review service.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Temporal TemporalConfig `mapstructure:"temporal"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Metrics  MetricsConfig  `mapstructure:"metrics"`
	Tracing  TracingConfig  `mapstructure:"tracing"`
	LLM      LLMConfig      `mapstructure:"llm"`
	Kafka    KafkaConfig    `mapstructure:"kafka"`
	Outbox   OutboxConfig   `mapstructure:"outbox"`

	// Paper source configurations
	PaperSources PaperSourcesConfig `mapstructure:"paper_sources"`

	// Ingestion service client
	IngestionService IngestionServiceConfig `mapstructure:"ingestion_service"`
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	HTTPPort        int           `mapstructure:"http_port"`
	GRPCPort        int           `mapstructure:"grpc_port"`
	MetricsPort     int           `mapstructure:"metrics_port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	Host              string        `mapstructure:"host"`
	Port              int           `mapstructure:"port"`
	User              string        `mapstructure:"user"`
	Password          string        `mapstructure:"password"`
	Name              string        `mapstructure:"name"`
	SSLMode           string        `mapstructure:"ssl_mode"`
	MaxConns          int32         `mapstructure:"max_conns"`
	MinConns          int32         `mapstructure:"min_conns"`
	MaxConnLifetime   time.Duration `mapstructure:"max_conn_lifetime"`
	MaxConnIdleTime   time.Duration `mapstructure:"max_conn_idle_time"`
	HealthCheckPeriod time.Duration `mapstructure:"health_check_period"`
	ConnectTimeout    time.Duration `mapstructure:"connect_timeout"`
	MigrationPath     string        `mapstructure:"migration_path"`
	MigrationAutoRun  bool          `mapstructure:"migration_auto_run"`
}

// TemporalConfig holds Temporal workflow configuration.
type TemporalConfig struct {
	HostPort  string `mapstructure:"host_port"`
	Namespace string `mapstructure:"namespace"`
	TaskQueue string `mapstructure:"task_queue"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Output     string `mapstructure:"output"`
	AddSource  bool   `mapstructure:"add_source"`
	TimeFormat string `mapstructure:"time_format"`
}

// MetricsConfig holds metrics configuration.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

// TracingConfig holds tracing configuration.
type TracingConfig struct {
	Enabled     bool    `mapstructure:"enabled"`
	Endpoint    string  `mapstructure:"endpoint"`
	ServiceName string  `mapstructure:"service_name"`
	SampleRate  float64 `mapstructure:"sample_rate"`
}

// LLMConfig holds LLM configuration.
type LLMConfig struct {
	Provider            string        `mapstructure:"provider"` // openai, anthropic, azure_openai
	InitialKeywordCount int           `mapstructure:"initial_keyword_count"`
	PaperKeywordCount   int           `mapstructure:"paper_keyword_count"`
	MaxExpansionDepth   int           `mapstructure:"max_expansion_depth"`
	Timeout             time.Duration `mapstructure:"timeout"`
	MaxRetries          int           `mapstructure:"max_retries"`
	Temperature         float64       `mapstructure:"temperature"`

	// Provider-specific configs
	OpenAI  OpenAIConfig  `mapstructure:"openai"`
	Anthropic AnthropicConfig `mapstructure:"anthropic"`
}

// OpenAIConfig holds OpenAI-specific configuration.
type OpenAIConfig struct {
	Model     string `mapstructure:"model"`
	APIKeyEnv string `mapstructure:"api_key_env"`
	BaseURL   string `mapstructure:"base_url"`
}

// AnthropicConfig holds Anthropic-specific configuration.
type AnthropicConfig struct {
	Model     string `mapstructure:"model"`
	APIKeyEnv string `mapstructure:"api_key_env"`
	BaseURL   string `mapstructure:"base_url"`
}

// KafkaConfig holds Kafka publisher settings.
type KafkaConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	Brokers      []string      `mapstructure:"brokers"`
	Topic        string        `mapstructure:"topic"`
	BatchSize    int           `mapstructure:"batch_size"`
	BatchTimeout time.Duration `mapstructure:"batch_timeout"`
}

// OutboxConfig holds outbox processor settings.
type OutboxConfig struct {
	PollInterval  time.Duration `mapstructure:"poll_interval"`
	BatchSize     int           `mapstructure:"batch_size"`
	Workers       int           `mapstructure:"workers"`
	MaxRetries    int           `mapstructure:"max_retries"`
	LeaseDuration time.Duration `mapstructure:"lease_duration"`
}

// PaperSourcesConfig holds configurations for all paper sources.
type PaperSourcesConfig struct {
	SemanticScholar PaperSourceConfig `mapstructure:"semantic_scholar"`
	OpenAlex        PaperSourceConfig `mapstructure:"openalex"`
	Scopus          PaperSourceConfig `mapstructure:"scopus"`
	PubMed          PaperSourceConfig `mapstructure:"pubmed"`
	BioRxiv         PaperSourceConfig `mapstructure:"biorxiv"`
	ArXiv           PaperSourceConfig `mapstructure:"arxiv"`
}

// PaperSourceConfig holds configuration for a paper source.
type PaperSourceConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	BaseURL           string        `mapstructure:"base_url"`
	APIKeyEnv         string        `mapstructure:"api_key_env"`
	RequestsPerSecond float64       `mapstructure:"requests_per_second"`
	Burst             int           `mapstructure:"burst"`
	Concurrency       int           `mapstructure:"concurrency"`
	Timeout           time.Duration `mapstructure:"timeout"`
	MaxResultsPerQuery int          `mapstructure:"max_results_per_query"`
}

// IngestionServiceConfig holds ingestion service client configuration.
type IngestionServiceConfig struct {
	Address string        `mapstructure:"address"`
	Timeout time.Duration `mapstructure:"timeout"`
}

// DSN returns the PostgreSQL connection string.
func (c *DatabaseConfig) DSN() string {
	params := url.Values{}
	params.Set("sslmode", c.SSLMode)
	if c.ConnectTimeout > 0 {
		params.Set("connect_timeout", fmt.Sprintf("%d", int(c.ConnectTimeout.Seconds())))
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?%s",
		url.QueryEscape(c.User),
		url.QueryEscape(c.Password),
		c.Host,
		c.Port,
		c.Name,
		params.Encode(),
	)
}

// Address returns the HTTP server address.
func (c *ServerConfig) HTTPAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.HTTPPort)
}

// GRPCAddress returns the gRPC server address.
func (c *ServerConfig) GRPCAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.GRPCPort)
}

// Load loads configuration from environment variables and config files.
func Load() (*Config, error) {
	v := viper.New()

	setDefaults(v)

	v.SetEnvPrefix("LITREVIEW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/literature-review-service")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.http_port", 8080)
	v.SetDefault("server.grpc_port", 9090)
	v.SetDefault("server.metrics_port", 9091)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.shutdown_timeout", "30s")

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "litreview")
	v.SetDefault("database.password", "")
	v.SetDefault("database.name", "literature_review")
	v.SetDefault("database.ssl_mode", SSLModeRequire)
	v.SetDefault("database.max_conns", 50)
	v.SetDefault("database.min_conns", 10)
	v.SetDefault("database.max_conn_lifetime", "1h")
	v.SetDefault("database.max_conn_idle_time", "30m")
	v.SetDefault("database.health_check_period", "30s")
	v.SetDefault("database.connect_timeout", "10s")
	v.SetDefault("database.migration_path", "migrations")
	v.SetDefault("database.migration_auto_run", false)

	// Temporal defaults
	v.SetDefault("temporal.host_port", "localhost:7233")
	v.SetDefault("temporal.namespace", "literature-review")
	v.SetDefault("temporal.task_queue", "literature-review-tasks")

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
	v.SetDefault("logging.add_source", false)
	v.SetDefault("logging.time_format", time.RFC3339)

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.path", "/metrics")

	// Tracing defaults
	v.SetDefault("tracing.enabled", false)
	v.SetDefault("tracing.endpoint", "")
	v.SetDefault("tracing.service_name", "literature-review-service")
	v.SetDefault("tracing.sample_rate", 0.1)

	// LLM defaults
	v.SetDefault("llm.provider", "openai")
	v.SetDefault("llm.initial_keyword_count", 5)
	v.SetDefault("llm.paper_keyword_count", 3)
	v.SetDefault("llm.max_expansion_depth", 2)
	v.SetDefault("llm.timeout", "60s")
	v.SetDefault("llm.max_retries", 3)
	v.SetDefault("llm.temperature", 0.3)
	v.SetDefault("llm.openai.model", "gpt-4-turbo")
	v.SetDefault("llm.openai.api_key_env", "OPENAI_API_KEY")
	v.SetDefault("llm.openai.base_url", "https://api.openai.com/v1")
	v.SetDefault("llm.anthropic.model", "claude-3-sonnet-20240229")
	v.SetDefault("llm.anthropic.api_key_env", "ANTHROPIC_API_KEY")
	v.SetDefault("llm.anthropic.base_url", "https://api.anthropic.com")

	// Kafka defaults
	v.SetDefault("kafka.enabled", false)
	v.SetDefault("kafka.brokers", []string{"localhost:9092"})
	v.SetDefault("kafka.topic", "events.outbox.literature_review")
	v.SetDefault("kafka.batch_size", 100)
	v.SetDefault("kafka.batch_timeout", "10ms")

	// Outbox defaults
	v.SetDefault("outbox.poll_interval", "1s")
	v.SetDefault("outbox.batch_size", 100)
	v.SetDefault("outbox.workers", 4)
	v.SetDefault("outbox.max_retries", 5)
	v.SetDefault("outbox.lease_duration", "30s")

	// Paper source defaults
	setSourceDefaults(v, "semantic_scholar", "https://api.semanticscholar.org/graph/v1", 10, 20, 5, true)
	setSourceDefaults(v, "openalex", "https://api.openalex.org", 10, 30, 8, true)
	setSourceDefaults(v, "scopus", "https://api.elsevier.com/content/search/scopus", 3, 5, 3, false)
	setSourceDefaults(v, "pubmed", "https://eutils.ncbi.nlm.nih.gov/entrez/eutils", 10, 10, 5, true)
	setSourceDefaults(v, "biorxiv", "https://api.biorxiv.org", 5, 10, 3, true)
	setSourceDefaults(v, "arxiv", "http://export.arxiv.org/api", 3, 5, 3, true)

	// Ingestion service defaults
	v.SetDefault("ingestion_service.address", "localhost:9092")
	v.SetDefault("ingestion_service.timeout", "30s")
}

func setSourceDefaults(v *viper.Viper, name, baseURL string, rps float64, burst, concurrency int, enabled bool) {
	prefix := "paper_sources." + name
	v.SetDefault(prefix+".enabled", enabled)
	v.SetDefault(prefix+".base_url", baseURL)
	v.SetDefault(prefix+".requests_per_second", rps)
	v.SetDefault(prefix+".burst", burst)
	v.SetDefault(prefix+".concurrency", concurrency)
	v.SetDefault(prefix+".timeout", "30s")
	v.SetDefault(prefix+".max_results_per_query", 100)
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Server.HTTPPort <= 0 || c.Server.HTTPPort > 65535 {
		return fmt.Errorf("invalid http port: %d", c.Server.HTTPPort)
	}
	if c.Server.GRPCPort <= 0 || c.Server.GRPCPort > 65535 {
		return fmt.Errorf("invalid grpc port: %d", c.Server.GRPCPort)
	}

	if c.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Database.Port <= 0 || c.Database.Port > 65535 {
		return fmt.Errorf("invalid database port: %d", c.Database.Port)
	}
	if c.Database.Name == "" {
		return fmt.Errorf("database name is required")
	}
	if c.Database.MaxConns < c.Database.MinConns {
		return fmt.Errorf("max_conns (%d) must be >= min_conns (%d)", c.Database.MaxConns, c.Database.MinConns)
	}

	validLogLevels := map[string]bool{
		"trace": true, "debug": true, "info": true,
		"warn": true, "error": true, "fatal": true, "panic": true,
	}
	if !validLogLevels[strings.ToLower(c.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	if c.Tracing.Enabled && c.Tracing.Endpoint == "" {
		return fmt.Errorf("tracing endpoint is required when tracing is enabled")
	}
	if c.Tracing.SampleRate < 0 || c.Tracing.SampleRate > 1 {
		return fmt.Errorf("tracing sample rate must be between 0 and 1")
	}

	if c.LLM.InitialKeywordCount <= 0 {
		return fmt.Errorf("initial_keyword_count must be positive")
	}
	if c.LLM.PaperKeywordCount <= 0 {
		return fmt.Errorf("paper_keyword_count must be positive")
	}
	if c.LLM.MaxExpansionDepth < 0 {
		return fmt.Errorf("max_expansion_depth cannot be negative")
	}

	return nil
}
```

**Step 4: Run tests to verify they pass**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/config/... -v
```

Expected: All tests PASS

**Step 5: Create default config.yaml**

Create `config/config.yaml`:

```yaml
# Literature Review Service Configuration

service:
  name: "literature-review-service"
  version: "1.0.0"

server:
  host: "0.0.0.0"
  http_port: 8080
  grpc_port: 9090
  metrics_port: 9091
  read_timeout: 30s
  write_timeout: 30s
  shutdown_timeout: 30s

database:
  host: "localhost"
  port: 5432
  user: "litreview"
  # password: set via LITREVIEW_DATABASE_PASSWORD env var
  name: "literature_review"
  ssl_mode: "require"
  max_conns: 50
  min_conns: 10
  max_conn_lifetime: 1h
  max_conn_idle_time: 30m
  health_check_period: 30s
  connect_timeout: 10s
  migration_path: "migrations"
  migration_auto_run: false

temporal:
  host_port: "localhost:7233"
  namespace: "literature-review"
  task_queue: "literature-review-tasks"

logging:
  level: "info"
  format: "json"
  output: "stdout"
  add_source: false

metrics:
  enabled: true
  path: "/metrics"

tracing:
  enabled: false
  service_name: "literature-review-service"
  sample_rate: 0.1

llm:
  provider: "openai"
  initial_keyword_count: 5
  paper_keyword_count: 3
  max_expansion_depth: 2
  timeout: 60s
  max_retries: 3
  temperature: 0.3

  openai:
    model: "gpt-4-turbo"
    api_key_env: "OPENAI_API_KEY"
    base_url: "https://api.openai.com/v1"

paper_sources:
  semantic_scholar:
    enabled: true
    base_url: "https://api.semanticscholar.org/graph/v1"
    requests_per_second: 10
    burst: 20
    concurrency: 5
    timeout: 30s
    max_results_per_query: 100

  openalex:
    enabled: true
    base_url: "https://api.openalex.org"
    requests_per_second: 10
    burst: 30
    concurrency: 8
    timeout: 20s
    max_results_per_query: 100

  pubmed:
    enabled: true
    base_url: "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"
    api_key_env: "PUBMED_API_KEY"
    requests_per_second: 10
    burst: 10
    concurrency: 5
    timeout: 30s
    max_results_per_query: 100

  biorxiv:
    enabled: true
    base_url: "https://api.biorxiv.org"
    requests_per_second: 5
    burst: 10
    concurrency: 3
    timeout: 30s
    max_results_per_query: 100

  arxiv:
    enabled: true
    base_url: "http://export.arxiv.org/api"
    requests_per_second: 3
    burst: 5
    concurrency: 3
    timeout: 30s
    max_results_per_query: 100

  scopus:
    enabled: false  # Requires API key
    base_url: "https://api.elsevier.com/content/search/scopus"
    api_key_env: "SCOPUS_API_KEY"
    requests_per_second: 3
    burst: 5
    concurrency: 3
    timeout: 30s
    max_results_per_query: 100

kafka:
  enabled: false
  brokers:
    - "localhost:9092"
  topic: "events.outbox.literature_review"
  batch_size: 100
  batch_timeout: 10ms

outbox:
  poll_interval: 1s
  batch_size: 100
  workers: 4
  max_retries: 5
  lease_duration: 30s

ingestion_service:
  address: "localhost:9092"
  timeout: 30s
```

**Step 6: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/config/ config/
git commit -m "feat: implement configuration system

- Add Config struct with all service settings
- Add viper-based loading with env override support
- Add validation for all configuration values
- Add default config.yaml
- Add DSN() helper for database connection string
- Add tests for config loading and validation"
```

---

## Task 4: Implement Database Layer

**Files:**
- Create: `literature_service/internal/database/database.go`
- Create: `literature_service/internal/database/migrator.go`
- Create: `literature_service/internal/database/database_test.go`

**Step 1: Write failing test for database**

Create `internal/database/database_test.go`:

```go
package database_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/config"
	"github.com/helixir/literature-review-service/internal/database"
)

func TestDBTX_Interface(t *testing.T) {
	// Verify DBTX interface is properly defined
	var _ database.DBTX = (*database.DB)(nil)
}

func TestDatabaseConfig_DSN(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "testuser",
		Password: "testpass",
		Name:     "testdb",
		SSLMode:  "disable",
	}

	dsn := cfg.DSN()
	assert.Contains(t, dsn, "postgres://")
	assert.Contains(t, dsn, "testuser")
	assert.Contains(t, dsn, "localhost:5432")
	assert.Contains(t, dsn, "testdb")
}

// Integration test - requires database
func TestNew_ConnectionError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &config.DatabaseConfig{
		Host:           "nonexistent-host",
		Port:           5432,
		User:           "test",
		Password:       "test",
		Name:           "test",
		SSLMode:        "disable",
		ConnectTimeout: 1,
	}

	ctx := context.Background()
	_, err := database.New(ctx, cfg, nil)
	require.Error(t, err)
}
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/database/... -v -short
```

Expected: FAIL - package not found

**Step 3: Implement database package**

Create `internal/database/database.go`:

```go
// Package database provides database connectivity and management.
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

const (
	// HealthCheckTimeout is the maximum time to wait for a health check ping.
	HealthCheckTimeout = 5 * time.Second
)

// DBTX is an interface that both *pgxpool.Pool and pgx.Tx satisfy.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// DB represents the database connection pool.
type DB struct {
	pool   *pgxpool.Pool
	config *config.DatabaseConfig
	logger zerolog.Logger
}

// Ensure DB implements DBTX
var _ DBTX = (*DB)(nil)

// New creates a new database connection pool.
func New(ctx context.Context, cfg *config.DatabaseConfig, logger *zerolog.Logger) (*DB, error) {
	var log zerolog.Logger
	if logger != nil {
		log = *logger
	} else {
		log = zerolog.Nop()
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = cfg.HealthCheckPeriod
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	poolConfig.BeforeAcquire = func(ctx context.Context, conn *pgx.Conn) bool {
		log.Trace().Msg("acquiring connection from pool")
		return true
	}

	poolConfig.AfterRelease = func(conn *pgx.Conn) bool {
		log.Trace().Msg("releasing connection to pool")
		return true
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Str("database", cfg.Name).
		Int32("max_conns", cfg.MaxConns).
		Int32("min_conns", cfg.MinConns).
		Msg("database connection pool established")

	return &DB{
		pool:   pool,
		config: cfg,
		logger: log,
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

// Health returns database health information.
func (db *DB) Health(ctx context.Context) map[string]interface{} {
	stat := db.pool.Stat()
	health := map[string]interface{}{
		"total_conns":        stat.TotalConns(),
		"acquired_conns":     stat.AcquiredConns(),
		"idle_conns":         stat.IdleConns(),
		"constructing_conns": stat.ConstructingConns(),
		"max_conns":          stat.MaxConns(),
	}

	pingCtx, cancel := context.WithTimeout(ctx, HealthCheckTimeout)
	defer cancel()
	if err := db.pool.Ping(pingCtx); err != nil {
		health["status"] = "unhealthy"
		health["error"] = err.Error()
	} else {
		health["status"] = "healthy"
	}

	return health
}

// WithTransaction executes a function within a database transaction.
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

// WithTransactionOptions executes a function within a transaction with custom options.
func (db *DB) WithTransactionOptions(ctx context.Context, opts pgx.TxOptions, fn func(tx pgx.Tx) error) error {
	tx, err := db.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				db.logger.Error().
					Err(rbErr).
					Interface("panic", p).
					Msg("failed to rollback transaction after panic")
			}
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			db.logger.Error().
				Err(rbErr).
				Err(err).
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
func (db *DB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, sql, args...)
}

// Query executes a query that returns rows.
func (db *DB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.pool.Query(ctx, sql, args...)
}

// QueryRow executes a query that is expected to return at most one row.
func (db *DB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.pool.QueryRow(ctx, sql, args...)
}

// SendBatch sends a batch of queries to the database.
func (db *DB) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	return db.pool.SendBatch(ctx, batch)
}
```

**Step 4: Create migrator**

Create `internal/database/migrator.go`:

```go
package database

import (
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog"
)

// Migrator handles database migrations.
type Migrator struct {
	migrate *migrate.Migrate
	logger  zerolog.Logger
}

// NewMigrator creates a new Migrator.
func NewMigrator(pool *pgxpool.Pool, migrationsPath string, logger zerolog.Logger) (*Migrator, error) {
	db := stdlib.OpenDBFromPool(pool)

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"postgres",
		driver,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return &Migrator{
		migrate: m,
		logger:  logger,
	}, nil
}

// Up runs all pending migrations.
func (m *Migrator) Up() error {
	m.logger.Info().Msg("running database migrations")

	err := m.migrate.Up()
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	if err == migrate.ErrNoChange {
		m.logger.Info().Msg("no migrations to apply")
	} else {
		m.logger.Info().Msg("migrations completed successfully")
	}

	return nil
}

// Down rolls back all migrations.
func (m *Migrator) Down() error {
	m.logger.Info().Msg("rolling back all migrations")

	err := m.migrate.Down()
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("rollback failed: %w", err)
	}

	m.logger.Info().Msg("rollback completed")
	return nil
}

// Steps runs n migrations (positive = up, negative = down).
func (m *Migrator) Steps(n int) error {
	m.logger.Info().Int("steps", n).Msg("running migration steps")

	err := m.migrate.Steps(n)
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration steps failed: %w", err)
	}

	return nil
}

// Version returns the current migration version.
func (m *Migrator) Version() (uint, bool, error) {
	return m.migrate.Version()
}

// Close closes the migrator.
func (m *Migrator) Close() error {
	sourceErr, dbErr := m.migrate.Close()
	if sourceErr != nil {
		return fmt.Errorf("failed to close source: %w", sourceErr)
	}
	if dbErr != nil {
		return fmt.Errorf("failed to close database: %w", dbErr)
	}
	return nil
}
```

**Step 5: Run tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/database/... -v -short
```

Expected: PASS

**Step 6: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/database/
git commit -m "feat: implement database layer

- Add DB struct with pgxpool connection pool
- Add DBTX interface for repository pattern
- Add transaction helpers (WithTransaction, WithSerializableTransaction)
- Add Migrator for golang-migrate integration
- Add health check and pool statistics"
```

---

## Task 5: Create Database Migrations

**Files:**
- Create: `literature_service/migrations/000001_init_extensions.up.sql`
- Create: `literature_service/migrations/000001_init_extensions.down.sql`
- Create: `literature_service/migrations/000002_enums.up.sql`
- Create: `literature_service/migrations/000002_enums.down.sql`
- Create: `literature_service/migrations/000003_core_tables.up.sql`
- Create: `literature_service/migrations/000003_core_tables.down.sql`
- Create: `literature_service/migrations/000004_tenant_tables.up.sql`
- Create: `literature_service/migrations/000004_tenant_tables.down.sql`
- Create: `literature_service/migrations/000005_outbox.up.sql`
- Create: `literature_service/migrations/000005_outbox.down.sql`
- Create: `literature_service/migrations/000006_functions.up.sql`
- Create: `literature_service/migrations/000006_functions.down.sql`
- Create: `literature_service/migrations/000007_triggers.up.sql`
- Create: `literature_service/migrations/000007_triggers.down.sql`

**Step 1: Create extensions migration**

Create `migrations/000001_init_extensions.up.sql`:

```sql
-- Enable required PostgreSQL extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
```

Create `migrations/000001_init_extensions.down.sql`:

```sql
-- Note: Extensions are shared across the database, be careful dropping them
-- DROP EXTENSION IF EXISTS "pgcrypto";
-- DROP EXTENSION IF EXISTS "pg_trgm";
-- DROP EXTENSION IF EXISTS "uuid-ossp";
```

**Step 2: Create enums migration**

Create `migrations/000002_enums.up.sql`:

```sql
-- Identifier types for papers
CREATE TYPE identifier_type AS ENUM (
    'doi',
    'arxiv_id',
    'pubmed_id',
    'pmcid',
    'semantic_scholar_id',
    'openalex_id',
    'scopus_id'
);

-- Literature review status
CREATE TYPE review_status AS ENUM (
    'pending',
    'extracting_keywords',
    'searching',
    'expanding',
    'ingesting',
    'completed',
    'failed',
    'cancelled'
);

-- Search status
CREATE TYPE search_status AS ENUM (
    'pending',
    'in_progress',
    'completed',
    'failed'
);

-- Ingestion status
CREATE TYPE ingestion_status AS ENUM (
    'pending',
    'queued',
    'ingesting',
    'completed',
    'failed',
    'skipped'
);

-- Keyword-paper mapping type
CREATE TYPE mapping_type AS ENUM (
    'search_result',
    'extracted'
);

-- Keyword source type
CREATE TYPE source_type AS ENUM (
    'user_query',
    'paper_extraction'
);
```

Create `migrations/000002_enums.down.sql`:

```sql
DROP TYPE IF EXISTS source_type;
DROP TYPE IF EXISTS mapping_type;
DROP TYPE IF EXISTS ingestion_status;
DROP TYPE IF EXISTS search_status;
DROP TYPE IF EXISTS review_status;
DROP TYPE IF EXISTS identifier_type;
```

**Step 3: Create core tables migration**

Create `migrations/000003_core_tables.up.sql`:

```sql
-- Keywords table (global dictionary)
CREATE TABLE keywords (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    keyword TEXT NOT NULL,
    normalized_keyword TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_keywords_normalized UNIQUE (normalized_keyword)
);

CREATE INDEX idx_keywords_normalized_trgm ON keywords USING gin (normalized_keyword gin_trgm_ops);
CREATE INDEX idx_keywords_created_at ON keywords (created_at);

-- Papers table (global cache)
CREATE TABLE papers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    canonical_id TEXT NOT NULL,
    title TEXT NOT NULL,
    abstract TEXT,
    authors JSONB NOT NULL DEFAULT '[]'::jsonb,
    publication_date DATE,
    publication_year INTEGER,
    venue TEXT,
    journal TEXT,
    volume TEXT,
    issue TEXT,
    pages TEXT,
    citation_count INTEGER,
    reference_count INTEGER,
    pdf_url TEXT,
    open_access BOOLEAN DEFAULT FALSE,
    first_discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    keywords_extracted BOOLEAN NOT NULL DEFAULT FALSE,
    keywords_extracted_at TIMESTAMPTZ,
    raw_metadata JSONB,
    CONSTRAINT uq_papers_canonical_id UNIQUE (canonical_id)
);

CREATE INDEX idx_papers_canonical_id ON papers (canonical_id);
CREATE INDEX idx_papers_title_trgm ON papers USING gin (title gin_trgm_ops);
CREATE INDEX idx_papers_publication_date ON papers (publication_date DESC NULLS LAST);
CREATE INDEX idx_papers_publication_year ON papers (publication_year DESC NULLS LAST);
CREATE INDEX idx_papers_keywords_not_extracted ON papers (id) WHERE keywords_extracted = FALSE;
CREATE INDEX idx_papers_created_at ON papers (first_discovered_at DESC);

-- Paper identifiers table
CREATE TABLE paper_identifiers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    identifier_type identifier_type NOT NULL,
    identifier_value TEXT NOT NULL,
    source_api TEXT NOT NULL,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_paper_identifiers_type_value UNIQUE (identifier_type, identifier_value)
);

CREATE INDEX idx_paper_identifiers_paper_id ON paper_identifiers (paper_id);
CREATE INDEX idx_paper_identifiers_lookup ON paper_identifiers (identifier_type, identifier_value);
CREATE INDEX idx_paper_identifiers_source ON paper_identifiers (source_api);

-- Paper sources table
CREATE TABLE paper_sources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    source_api TEXT NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source_metadata JSONB,
    CONSTRAINT uq_paper_sources_paper_source UNIQUE (paper_id, source_api)
);

CREATE INDEX idx_paper_sources_paper_id ON paper_sources (paper_id);
CREATE INDEX idx_paper_sources_source_api ON paper_sources (source_api);

-- Keyword searches table
CREATE TABLE keyword_searches (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    keyword_id UUID NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    source_api TEXT NOT NULL,
    searched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    search_from_date DATE,
    search_to_date DATE NOT NULL,
    search_window_hash TEXT NOT NULL,
    papers_found INTEGER NOT NULL DEFAULT 0,
    status search_status NOT NULL DEFAULT 'completed',
    error_message TEXT,
    CONSTRAINT uq_keyword_searches_window UNIQUE (keyword_id, source_api, search_window_hash)
);

CREATE INDEX idx_keyword_searches_keyword_id ON keyword_searches (keyword_id);
CREATE INDEX idx_keyword_searches_searched_at ON keyword_searches (searched_at DESC);
CREATE INDEX idx_keyword_searches_lookup ON keyword_searches (keyword_id, source_api, search_to_date DESC);
CREATE INDEX idx_keyword_searches_status ON keyword_searches (status) WHERE status != 'completed';

-- Keyword-paper mappings table
CREATE TABLE keyword_paper_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    keyword_id UUID NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    mapping_type mapping_type NOT NULL,
    source_type source_type NOT NULL,
    confidence_score FLOAT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_keyword_paper_mappings UNIQUE (keyword_id, paper_id, mapping_type)
);

CREATE INDEX idx_kpm_keyword_id ON keyword_paper_mappings (keyword_id);
CREATE INDEX idx_kpm_paper_id ON keyword_paper_mappings (paper_id);
CREATE INDEX idx_kpm_mapping_type ON keyword_paper_mappings (mapping_type);
CREATE INDEX idx_kpm_source_type ON keyword_paper_mappings (source_type);
```

Create `migrations/000003_core_tables.down.sql`:

```sql
DROP TABLE IF EXISTS keyword_paper_mappings;
DROP TABLE IF EXISTS keyword_searches;
DROP TABLE IF EXISTS paper_sources;
DROP TABLE IF EXISTS paper_identifiers;
DROP TABLE IF EXISTS papers;
DROP TABLE IF EXISTS keywords;
```

**Step 4: Create tenant tables migration**

Create `migrations/000004_tenant_tables.up.sql`:

```sql
-- Literature review requests table (tenant-scoped)
CREATE TABLE literature_review_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id UUID NOT NULL,
    project_id UUID NOT NULL,
    user_id TEXT NOT NULL,
    original_query TEXT NOT NULL,
    temporal_workflow_id TEXT NOT NULL,
    temporal_run_id TEXT,
    status review_status NOT NULL DEFAULT 'pending',
    initial_keywords_count INTEGER,
    papers_found_count INTEGER DEFAULT 0,
    papers_ingested_count INTEGER DEFAULT 0,
    current_expansion_depth INTEGER DEFAULT 0,
    max_expansion_depth INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    config_snapshot JSONB NOT NULL,
    source_filters TEXT[] NOT NULL DEFAULT '{}',
    date_from DATE,
    date_to DATE
);

CREATE INDEX idx_lrr_tenant ON literature_review_requests (org_id, project_id);
CREATE INDEX idx_lrr_org_user ON literature_review_requests (org_id, user_id);
CREATE INDEX idx_lrr_status ON literature_review_requests (status);
CREATE INDEX idx_lrr_workflow_id ON literature_review_requests (temporal_workflow_id);
CREATE INDEX idx_lrr_created_at ON literature_review_requests (created_at DESC);
CREATE INDEX idx_lrr_active ON literature_review_requests (org_id, project_id, status)
    WHERE status NOT IN ('completed', 'failed', 'cancelled');

-- Request-keyword mappings table
CREATE TABLE request_keyword_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id UUID NOT NULL REFERENCES literature_review_requests(id) ON DELETE CASCADE,
    keyword_id UUID NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    extraction_round INTEGER NOT NULL DEFAULT 0,
    source_paper_id UUID REFERENCES papers(id) ON DELETE SET NULL,
    source_type source_type NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_request_keyword UNIQUE (request_id, keyword_id)
);

CREATE INDEX idx_rkm_request_id ON request_keyword_mappings (request_id);
CREATE INDEX idx_rkm_keyword_id ON request_keyword_mappings (keyword_id);
CREATE INDEX idx_rkm_round ON request_keyword_mappings (request_id, extraction_round);

-- Request-paper mappings table
CREATE TABLE request_paper_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id UUID NOT NULL REFERENCES literature_review_requests(id) ON DELETE CASCADE,
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    discovered_via_keyword_id UUID REFERENCES keywords(id) ON DELETE SET NULL,
    discovered_via_source TEXT NOT NULL,
    expansion_depth INTEGER NOT NULL DEFAULT 0,
    ingestion_status ingestion_status NOT NULL DEFAULT 'pending',
    ingestion_job_id TEXT,
    ingestion_error TEXT,
    ingested_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_request_paper UNIQUE (request_id, paper_id)
);

CREATE INDEX idx_rpm_request_id ON request_paper_mappings (request_id);
CREATE INDEX idx_rpm_paper_id ON request_paper_mappings (paper_id);
CREATE INDEX idx_rpm_ingestion_pending ON request_paper_mappings (request_id, ingestion_status)
    WHERE ingestion_status IN ('pending', 'queued');
CREATE INDEX idx_rpm_depth ON request_paper_mappings (request_id, expansion_depth);

-- Review progress events table
CREATE TABLE review_progress_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id UUID NOT NULL REFERENCES literature_review_requests(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    event_data JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rpe_request_id ON review_progress_events (request_id, created_at DESC);
CREATE INDEX idx_rpe_created_at ON review_progress_events (created_at DESC);
CREATE INDEX idx_rpe_cleanup ON review_progress_events (created_at)
    WHERE created_at < NOW() - INTERVAL '24 hours';
```

Create `migrations/000004_tenant_tables.down.sql`:

```sql
DROP TABLE IF EXISTS review_progress_events;
DROP TABLE IF EXISTS request_paper_mappings;
DROP TABLE IF EXISTS request_keyword_mappings;
DROP TABLE IF EXISTS literature_review_requests;
```

**Step 5: Create outbox migration**

Create `migrations/000005_outbox.up.sql`:

```sql
-- Outbox events table for transactional event publishing
CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    org_id UUID NOT NULL,
    project_id UUID,
    payload JSONB NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    publish_attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    sequence_num BIGSERIAL NOT NULL,

    -- For lease-based processing
    locked_by TEXT,
    locked_at TIMESTAMPTZ,
    lock_expires_at TIMESTAMPTZ
);

CREATE INDEX idx_outbox_unpublished ON outbox_events (sequence_num)
    WHERE published_at IS NULL;
CREATE INDEX idx_outbox_aggregate ON outbox_events (aggregate_type, aggregate_id);
CREATE INDEX idx_outbox_created_at ON outbox_events (created_at);
CREATE INDEX idx_outbox_cleanup ON outbox_events (published_at)
    WHERE published_at IS NOT NULL AND published_at < NOW() - INTERVAL '7 days';
CREATE INDEX idx_outbox_locked ON outbox_events (lock_expires_at)
    WHERE published_at IS NULL AND locked_by IS NOT NULL;

-- Dead letter table for failed events
CREATE TABLE outbox_dead_letter (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    original_event_id UUID NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    org_id UUID NOT NULL,
    project_id UUID,
    payload JSONB NOT NULL,
    metadata JSONB NOT NULL,
    original_created_at TIMESTAMPTZ NOT NULL,
    failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    failure_reason TEXT NOT NULL,
    retry_count INTEGER NOT NULL
);

CREATE INDEX idx_outbox_dl_created_at ON outbox_dead_letter (failed_at DESC);
CREATE INDEX idx_outbox_dl_aggregate ON outbox_dead_letter (aggregate_type, aggregate_id);
```

Create `migrations/000005_outbox.down.sql`:

```sql
DROP TABLE IF EXISTS outbox_dead_letter;
DROP TABLE IF EXISTS outbox_events;
```

**Step 6: Create functions migration**

Create `migrations/000006_functions.up.sql`:

```sql
-- Generate canonical ID from identifiers
CREATE OR REPLACE FUNCTION generate_canonical_id(
    p_doi TEXT DEFAULT NULL,
    p_arxiv_id TEXT DEFAULT NULL,
    p_pubmed_id TEXT DEFAULT NULL,
    p_semantic_scholar_id TEXT DEFAULT NULL,
    p_openalex_id TEXT DEFAULT NULL,
    p_scopus_id TEXT DEFAULT NULL
) RETURNS TEXT AS $$
BEGIN
    IF p_doi IS NOT NULL AND p_doi != '' THEN
        RETURN 'doi:' || lower(trim(p_doi));
    ELSIF p_arxiv_id IS NOT NULL AND p_arxiv_id != '' THEN
        RETURN 'arxiv:' || lower(trim(p_arxiv_id));
    ELSIF p_pubmed_id IS NOT NULL AND p_pubmed_id != '' THEN
        RETURN 'pmid:' || trim(p_pubmed_id);
    ELSIF p_semantic_scholar_id IS NOT NULL AND p_semantic_scholar_id != '' THEN
        RETURN 'ss:' || trim(p_semantic_scholar_id);
    ELSIF p_openalex_id IS NOT NULL AND p_openalex_id != '' THEN
        RETURN 'oalex:' || trim(p_openalex_id);
    ELSIF p_scopus_id IS NOT NULL AND p_scopus_id != '' THEN
        RETURN 'scopus:' || trim(p_scopus_id);
    ELSE
        RAISE EXCEPTION 'At least one identifier is required';
    END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE STRICT;

-- Generate search window hash for idempotency
CREATE OR REPLACE FUNCTION generate_search_window_hash(
    p_keyword_id UUID,
    p_source_api TEXT,
    p_from_date DATE,
    p_to_date DATE
) RETURNS TEXT AS $$
BEGIN
    RETURN encode(
        sha256(
            (
                p_keyword_id::text || '|' ||
                p_source_api || '|' ||
                COALESCE(p_from_date::text, 'null') || '|' ||
                p_to_date::text
            )::bytea
        ),
        'hex'
    );
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Get or create keyword (idempotent)
CREATE OR REPLACE FUNCTION get_or_create_keyword(p_keyword TEXT)
RETURNS UUID AS $$
DECLARE
    v_normalized TEXT;
    v_id UUID;
BEGIN
    v_normalized := lower(trim(regexp_replace(p_keyword, '\s+', ' ', 'g')));

    INSERT INTO keywords (keyword, normalized_keyword)
    VALUES (trim(p_keyword), v_normalized)
    ON CONFLICT (normalized_keyword)
    DO UPDATE SET keyword = keywords.keyword
    RETURNING id INTO v_id;

    RETURN v_id;
END;
$$ LANGUAGE plpgsql;

-- Find paper by any identifier
CREATE OR REPLACE FUNCTION find_paper_by_identifier(
    p_identifier_type identifier_type,
    p_identifier_value TEXT
) RETURNS UUID AS $$
DECLARE
    v_paper_id UUID;
BEGIN
    SELECT paper_id INTO v_paper_id
    FROM paper_identifiers
    WHERE identifier_type = p_identifier_type
      AND identifier_value = lower(trim(p_identifier_value))
    LIMIT 1;

    RETURN v_paper_id;
END;
$$ LANGUAGE plpgsql STABLE;
```

Create `migrations/000006_functions.down.sql`:

```sql
DROP FUNCTION IF EXISTS find_paper_by_identifier(identifier_type, TEXT);
DROP FUNCTION IF EXISTS get_or_create_keyword(TEXT);
DROP FUNCTION IF EXISTS generate_search_window_hash(UUID, TEXT, DATE, DATE);
DROP FUNCTION IF EXISTS generate_canonical_id(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);
```

**Step 7: Create triggers migration**

Create `migrations/000007_triggers.up.sql`:

```sql
-- Trigger function for progress event notifications
CREATE OR REPLACE FUNCTION notify_review_progress()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify(
        'review_progress_' || NEW.request_id::text,
        json_build_object(
            'event_id', NEW.id,
            'event_type', NEW.event_type,
            'event_data', NEW.event_data,
            'created_at', NEW.created_at
        )::text
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for progress notifications
CREATE TRIGGER trg_review_progress_notify
    AFTER INSERT ON review_progress_events
    FOR EACH ROW
    EXECUTE FUNCTION notify_review_progress();

-- Auto-update last_updated_at on papers
CREATE OR REPLACE FUNCTION update_papers_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.last_updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_papers_updated_at
    BEFORE UPDATE ON papers
    FOR EACH ROW
    EXECUTE FUNCTION update_papers_timestamp();
```

Create `migrations/000007_triggers.down.sql`:

```sql
DROP TRIGGER IF EXISTS trg_papers_updated_at ON papers;
DROP FUNCTION IF EXISTS update_papers_timestamp();
DROP TRIGGER IF EXISTS trg_review_progress_notify ON review_progress_events;
DROP FUNCTION IF EXISTS notify_review_progress();
```

**Step 8: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add migrations/
git commit -m "feat: add database migrations

- 000001: PostgreSQL extensions (uuid-ossp, pg_trgm, pgcrypto)
- 000002: Custom enum types for status tracking
- 000003: Core tables (keywords, papers, identifiers, sources, searches, mappings)
- 000004: Tenant-scoped tables (requests, request mappings, progress events)
- 000005: Outbox tables for event publishing
- 000006: Helper functions (canonical_id, keyword normalization, paper lookup)
- 000007: Triggers (progress notifications, timestamp updates)"
```

---

## Task 6: Implement Domain Models

**Files:**
- Create: `literature_service/internal/domain/models.go`
- Create: `literature_service/internal/domain/paper.go`
- Create: `literature_service/internal/domain/keyword.go`
- Create: `literature_service/internal/domain/review.go`
- Create: `literature_service/internal/domain/errors.go`
- Create: `literature_service/internal/domain/events.go`
- Create: `literature_service/internal/domain/models_test.go`

**Step 1: Write failing test for domain models**

Create `internal/domain/models_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/helixir/literature-review-service/internal/domain"
)

func TestKeyword_Normalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Machine Learning", "machine learning"},
		{"  Protein   Folding  ", "protein folding"},
		{"DEEP LEARNING", "deep learning"},
		{"alphafold2", "alphafold2"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := domain.NormalizeKeyword(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPaper_GenerateCanonicalID(t *testing.T) {
	tests := []struct {
		name     string
		ids      domain.PaperIdentifiers
		expected string
	}{
		{
			name: "DOI takes priority",
			ids: domain.PaperIdentifiers{
				DOI:               "10.1234/test",
				ArXivID:           "2301.00001",
				SemanticScholarID: "abc123",
			},
			expected: "doi:10.1234/test",
		},
		{
			name: "ArXiv when no DOI",
			ids: domain.PaperIdentifiers{
				ArXivID:           "2301.00001",
				SemanticScholarID: "abc123",
			},
			expected: "arxiv:2301.00001",
		},
		{
			name: "PubMed when no DOI or ArXiv",
			ids: domain.PaperIdentifiers{
				PubMedID:          "12345678",
				SemanticScholarID: "abc123",
			},
			expected: "pmid:12345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := domain.GenerateCanonicalID(tt.ids)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestReviewStatus_IsTerminal(t *testing.T) {
	assert.True(t, domain.ReviewStatusCompleted.IsTerminal())
	assert.True(t, domain.ReviewStatusFailed.IsTerminal())
	assert.True(t, domain.ReviewStatusCancelled.IsTerminal())
	assert.False(t, domain.ReviewStatusPending.IsTerminal())
	assert.False(t, domain.ReviewStatusSearching.IsTerminal())
}

func TestLiteratureReviewRequest_Duration(t *testing.T) {
	now := time.Now()
	request := &domain.LiteratureReviewRequest{
		StartedAt:   &now,
		CompletedAt: func() *time.Time { t := now.Add(5 * time.Minute); return &t }(),
	}

	duration := request.Duration()
	assert.Equal(t, 5*time.Minute, duration)
}

func TestAuthor_String(t *testing.T) {
	author := domain.Author{
		Name:        "John Doe",
		Affiliation: "MIT",
	}
	assert.Contains(t, author.String(), "John Doe")
}
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/domain/... -v
```

Expected: FAIL - package not found

**Step 3: Implement domain models**

Create `internal/domain/models.go`:

```go
// Package domain provides core domain models for the literature review service.
package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// ReviewStatus represents the status of a literature review.
type ReviewStatus string

const (
	ReviewStatusPending            ReviewStatus = "pending"
	ReviewStatusExtractingKeywords ReviewStatus = "extracting_keywords"
	ReviewStatusSearching          ReviewStatus = "searching"
	ReviewStatusExpanding          ReviewStatus = "expanding"
	ReviewStatusIngesting          ReviewStatus = "ingesting"
	ReviewStatusCompleted          ReviewStatus = "completed"
	ReviewStatusFailed             ReviewStatus = "failed"
	ReviewStatusCancelled          ReviewStatus = "cancelled"
)

// IsTerminal returns true if this status represents a terminal state.
func (s ReviewStatus) IsTerminal() bool {
	return s == ReviewStatusCompleted || s == ReviewStatusFailed || s == ReviewStatusCancelled
}

// SearchStatus represents the status of a keyword search.
type SearchStatus string

const (
	SearchStatusPending    SearchStatus = "pending"
	SearchStatusInProgress SearchStatus = "in_progress"
	SearchStatusCompleted  SearchStatus = "completed"
	SearchStatusFailed     SearchStatus = "failed"
)

// IngestionStatus represents the ingestion status of a paper.
type IngestionStatus string

const (
	IngestionStatusPending   IngestionStatus = "pending"
	IngestionStatusQueued    IngestionStatus = "queued"
	IngestionStatusIngesting IngestionStatus = "ingesting"
	IngestionStatusCompleted IngestionStatus = "completed"
	IngestionStatusFailed    IngestionStatus = "failed"
	IngestionStatusSkipped   IngestionStatus = "skipped"
)

// MappingType represents how a keyword-paper mapping was created.
type MappingType string

const (
	MappingTypeSearchResult MappingType = "search_result"
	MappingTypeExtracted    MappingType = "extracted"
)

// SourceType represents the origin of a keyword.
type SourceType string

const (
	SourceTypeUserQuery       SourceType = "user_query"
	SourceTypePaperExtraction SourceType = "paper_extraction"
)

// IdentifierType represents the type of paper identifier.
type IdentifierType string

const (
	IdentifierTypeDOI             IdentifierType = "doi"
	IdentifierTypeArXiv           IdentifierType = "arxiv_id"
	IdentifierTypePubMed          IdentifierType = "pubmed_id"
	IdentifierTypePMCID           IdentifierType = "pmcid"
	IdentifierTypeSemanticScholar IdentifierType = "semantic_scholar_id"
	IdentifierTypeOpenAlex        IdentifierType = "openalex_id"
	IdentifierTypeScopus          IdentifierType = "scopus_id"
)

// Tenant represents the tenant context for multi-tenancy.
type Tenant struct {
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	UserID    string
}
```

Create `internal/domain/keyword.go`:

```go
package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var whitespaceRegex = regexp.MustCompile(`\s+`)

// Keyword represents a search keyword.
type Keyword struct {
	ID                uuid.UUID
	Keyword           string
	NormalizedKeyword string
	CreatedAt         time.Time
}

// NormalizeKeyword normalizes a keyword string.
// - Converts to lowercase
// - Trims whitespace
// - Collapses multiple whitespace to single space
func NormalizeKeyword(keyword string) string {
	normalized := strings.ToLower(strings.TrimSpace(keyword))
	normalized = whitespaceRegex.ReplaceAllString(normalized, " ")
	return normalized
}

// NewKeyword creates a new keyword with automatic normalization.
func NewKeyword(keyword string) *Keyword {
	return &Keyword{
		ID:                uuid.New(),
		Keyword:           strings.TrimSpace(keyword),
		NormalizedKeyword: NormalizeKeyword(keyword),
		CreatedAt:         time.Now(),
	}
}

// KeywordSearch represents a search operation for a keyword.
type KeywordSearch struct {
	ID               uuid.UUID
	KeywordID        uuid.UUID
	SourceAPI        string
	SearchedAt       time.Time
	SearchFromDate   *time.Time
	SearchToDate     time.Time
	SearchWindowHash string
	PapersFound      int
	Status           SearchStatus
	ErrorMessage     *string
}

// KeywordPaperMapping represents a mapping between a keyword and a paper.
type KeywordPaperMapping struct {
	ID              uuid.UUID
	KeywordID       uuid.UUID
	PaperID         uuid.UUID
	MappingType     MappingType
	SourceType      SourceType
	ConfidenceScore *float64
	CreatedAt       time.Time
}
```

Create `internal/domain/paper.go`:

```go
package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PaperIdentifiers holds all possible external identifiers for a paper.
type PaperIdentifiers struct {
	DOI               string
	ArXivID           string
	PubMedID          string
	PMCID             string
	SemanticScholarID string
	OpenAlexID        string
	ScopusID          string
}

// GenerateCanonicalID generates a canonical identifier from available identifiers.
// Priority: DOI > ArXiv > PubMed > Semantic Scholar > OpenAlex > Scopus
func GenerateCanonicalID(ids PaperIdentifiers) (string, error) {
	if ids.DOI != "" {
		return "doi:" + strings.ToLower(strings.TrimSpace(ids.DOI)), nil
	}
	if ids.ArXivID != "" {
		return "arxiv:" + strings.ToLower(strings.TrimSpace(ids.ArXivID)), nil
	}
	if ids.PubMedID != "" {
		return "pmid:" + strings.TrimSpace(ids.PubMedID), nil
	}
	if ids.SemanticScholarID != "" {
		return "ss:" + strings.TrimSpace(ids.SemanticScholarID), nil
	}
	if ids.OpenAlexID != "" {
		return "oalex:" + strings.TrimSpace(ids.OpenAlexID), nil
	}
	if ids.ScopusID != "" {
		return "scopus:" + strings.TrimSpace(ids.ScopusID), nil
	}
	return "", fmt.Errorf("at least one identifier is required")
}

// Author represents a paper author.
type Author struct {
	Name        string `json:"name"`
	Affiliation string `json:"affiliation,omitempty"`
	ORCID       string `json:"orcid,omitempty"`
}

// String returns a string representation of the author.
func (a Author) String() string {
	if a.Affiliation != "" {
		return fmt.Sprintf("%s (%s)", a.Name, a.Affiliation)
	}
	return a.Name
}

// Paper represents an academic paper.
type Paper struct {
	ID                  uuid.UUID
	CanonicalID         string
	Title               string
	Abstract            *string
	Authors             []Author
	PublicationDate     *time.Time
	PublicationYear     *int
	Venue               *string
	Journal             *string
	Volume              *string
	Issue               *string
	Pages               *string
	CitationCount       *int
	ReferenceCount      *int
	PDFURL              *string
	OpenAccess          bool
	FirstDiscoveredAt   time.Time
	LastUpdatedAt       time.Time
	KeywordsExtracted   bool
	KeywordsExtractedAt *time.Time
	RawMetadata         json.RawMessage
}

// PaperIdentifier represents an external identifier for a paper.
type PaperIdentifier struct {
	ID              uuid.UUID
	PaperID         uuid.UUID
	IdentifierType  IdentifierType
	IdentifierValue string
	SourceAPI       string
	DiscoveredAt    time.Time
}

// PaperSource represents a source that has seen a paper.
type PaperSource struct {
	ID             uuid.UUID
	PaperID        uuid.UUID
	SourceAPI      string
	FirstSeenAt    time.Time
	LastSeenAt     time.Time
	SourceMetadata json.RawMessage
}
```

Create `internal/domain/review.go`:

```go
package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// LiteratureReviewRequest represents a literature review job.
type LiteratureReviewRequest struct {
	ID                   uuid.UUID
	OrgID                uuid.UUID
	ProjectID            uuid.UUID
	UserID               string
	OriginalQuery        string
	TemporalWorkflowID   string
	TemporalRunID        *string
	Status               ReviewStatus
	InitialKeywordsCount *int
	PapersFoundCount     int
	PapersIngestedCount  int
	CurrentExpansionDepth int
	MaxExpansionDepth    int
	CreatedAt            time.Time
	StartedAt            *time.Time
	CompletedAt          *time.Time
	ErrorMessage         *string
	RetryCount           int
	ConfigSnapshot       json.RawMessage
	SourceFilters        []string
	DateFrom             *time.Time
	DateTo               *time.Time
}

// Duration returns the duration of the review if completed.
func (r *LiteratureReviewRequest) Duration() time.Duration {
	if r.StartedAt == nil {
		return 0
	}
	end := time.Now()
	if r.CompletedAt != nil {
		end = *r.CompletedAt
	}
	return end.Sub(*r.StartedAt)
}

// IsActive returns true if the review is still in progress.
func (r *LiteratureReviewRequest) IsActive() bool {
	return !r.Status.IsTerminal()
}

// RequestKeywordMapping links a request to keywords used.
type RequestKeywordMapping struct {
	ID              uuid.UUID
	RequestID       uuid.UUID
	KeywordID       uuid.UUID
	ExtractionRound int
	SourcePaperID   *uuid.UUID
	SourceType      SourceType
	CreatedAt       time.Time
}

// RequestPaperMapping links a request to papers discovered.
type RequestPaperMapping struct {
	ID                     uuid.UUID
	RequestID              uuid.UUID
	PaperID                uuid.UUID
	DiscoveredViaKeywordID *uuid.UUID
	DiscoveredViaSource    string
	ExpansionDepth         int
	IngestionStatus        IngestionStatus
	IngestionJobID         *string
	IngestionError         *string
	IngestedAt             *time.Time
	CreatedAt              time.Time
}

// ReviewProgressEvent represents a progress update.
type ReviewProgressEvent struct {
	ID        uuid.UUID
	RequestID uuid.UUID
	EventType string
	EventData json.RawMessage
	CreatedAt time.Time
}

// ReviewProgress holds the current progress state.
type ReviewProgress struct {
	InitialKeywordsCount    int
	TotalKeywordsProcessed  int
	PapersFound             int
	PapersNew               int
	PapersIngested          int
	PapersFailed            int
	CurrentExpansionDepth   int
	MaxExpansionDepth       int
	SourceProgress          map[string]SourceProgress
	ElapsedTime             time.Duration
	EstimatedRemaining      *time.Duration
}

// SourceProgress holds progress for a specific paper source.
type SourceProgress struct {
	SourceName       string
	QueriesCompleted int
	QueriesTotal     int
	PapersFound      int
	Errors           int
	RateLimited      bool
}
```

Create `internal/domain/errors.go`:

```go
package domain

import (
	"errors"
	"fmt"
)

// Domain errors
var (
	ErrNotFound            = errors.New("not found")
	ErrAlreadyExists       = errors.New("already exists")
	ErrInvalidInput        = errors.New("invalid input")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrRateLimited         = errors.New("rate limited")
	ErrServiceUnavailable  = errors.New("service unavailable")
	ErrInternalError       = errors.New("internal error")
	ErrWorkflowFailed      = errors.New("workflow failed")
	ErrCancelled           = errors.New("cancelled")
	ErrNoIdentifier        = errors.New("no identifier provided")
)

// ValidationError represents a validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error on %s: %s", e.Field, e.Message)
}

// NotFoundError represents a resource not found error.
type NotFoundError struct {
	Resource string
	ID       string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Resource, e.ID)
}

func (e NotFoundError) Unwrap() error {
	return ErrNotFound
}

// AlreadyExistsError represents a duplicate resource error.
type AlreadyExistsError struct {
	Resource string
	ID       string
}

func (e AlreadyExistsError) Error() string {
	return fmt.Sprintf("%s already exists: %s", e.Resource, e.ID)
}

func (e AlreadyExistsError) Unwrap() error {
	return ErrAlreadyExists
}

// RateLimitError represents a rate limit error from a paper source.
type RateLimitError struct {
	Source     string
	RetryAfter *int // seconds
}

func (e RateLimitError) Error() string {
	if e.RetryAfter != nil {
		return fmt.Sprintf("rate limited by %s, retry after %d seconds", e.Source, *e.RetryAfter)
	}
	return fmt.Sprintf("rate limited by %s", e.Source)
}

func (e RateLimitError) Unwrap() error {
	return ErrRateLimited
}

// ExternalAPIError represents an error from an external API.
type ExternalAPIError struct {
	Source     string
	StatusCode int
	Message    string
	Cause      error
}

func (e ExternalAPIError) Error() string {
	return fmt.Sprintf("%s API error (status %d): %s", e.Source, e.StatusCode, e.Message)
}

func (e ExternalAPIError) Unwrap() error {
	return e.Cause
}
```

Create `internal/domain/events.go`:

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Event types for the outbox
const (
	EventTypeReviewStarted       = "review.started"
	EventTypeReviewCompleted     = "review.completed"
	EventTypeReviewFailed        = "review.failed"
	EventTypeReviewCancelled     = "review.cancelled"
	EventTypeKeywordsExtracted   = "review.keywords_extracted"
	EventTypePapersDiscovered    = "review.papers_discovered"
	EventTypeIngestionStarted    = "review.ingestion_started"
	EventTypeIngestionCompleted  = "review.ingestion_completed"
)

// OutboxEvent represents an event to be published via the outbox.
type OutboxEvent struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   string
	EventType     string
	OrgID         uuid.UUID
	ProjectID     *uuid.UUID
	Payload       interface{}
	Metadata      map[string]string
	CreatedAt     time.Time
}

// NewOutboxEvent creates a new outbox event.
func NewOutboxEvent(aggregateType, aggregateID, eventType string, tenant Tenant, payload interface{}) *OutboxEvent {
	return &OutboxEvent{
		ID:            uuid.New(),
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		OrgID:         tenant.OrgID,
		ProjectID:     &tenant.ProjectID,
		Payload:       payload,
		Metadata:      make(map[string]string),
		CreatedAt:     time.Now(),
	}
}

// ReviewStartedPayload is the payload for review.started events.
type ReviewStartedPayload struct {
	ReviewID      string    `json:"review_id"`
	Query         string    `json:"query"`
	Keywords      []string  `json:"keywords"`
	Sources       []string  `json:"sources"`
	MaxDepth      int       `json:"max_depth"`
	StartedAt     time.Time `json:"started_at"`
}

// ReviewCompletedPayload is the payload for review.completed events.
type ReviewCompletedPayload struct {
	ReviewID        string        `json:"review_id"`
	PapersFound     int           `json:"papers_found"`
	PapersIngested  int           `json:"papers_ingested"`
	KeywordsUsed    int           `json:"keywords_used"`
	Duration        time.Duration `json:"duration"`
	CompletedAt     time.Time     `json:"completed_at"`
}

// ReviewFailedPayload is the payload for review.failed events.
type ReviewFailedPayload struct {
	ReviewID  string    `json:"review_id"`
	Error     string    `json:"error"`
	Phase     string    `json:"phase"`
	FailedAt  time.Time `json:"failed_at"`
}

// PapersDiscoveredPayload is the payload for papers discovered events.
type PapersDiscoveredPayload struct {
	ReviewID   string   `json:"review_id"`
	Source     string   `json:"source"`
	Keyword    string   `json:"keyword"`
	PaperIDs   []string `json:"paper_ids"`
	Count      int      `json:"count"`
	NewCount   int      `json:"new_count"`
}
```

**Step 4: Run tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/domain/... -v
```

Expected: All tests PASS

**Step 5: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/domain/
git commit -m "feat: implement domain models

- Add core types: ReviewStatus, SearchStatus, IngestionStatus, etc.
- Add Keyword with normalization logic
- Add Paper with canonical ID generation and identifiers
- Add LiteratureReviewRequest with progress tracking
- Add domain errors with proper wrapping
- Add outbox event types and payloads
- Add tests for keyword normalization and canonical ID generation"
```

---

## Task 7: Create Protobuf Definitions

**Files:**
- Create: `literature_service/api/proto/literaturereview/v1/literature_review.proto`
- Create: `literature_service/buf.yaml`
- Create: `literature_service/buf.gen.yaml`

**Step 1: Create buf.yaml**

Create `buf.yaml`:

```yaml
version: v2
modules:
  - path: api/proto
    name: buf.build/helixir/literature-review-service
lint:
  use:
    - STANDARD
  except:
    - PACKAGE_VERSION_SUFFIX
breaking:
  use:
    - FILE
deps:
  - buf.build/googleapis/googleapis
```

**Step 2: Create buf.gen.yaml**

Create `buf.gen.yaml`:

```yaml
version: v2
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/helixir/literature-review-service/gen/proto
plugins:
  - remote: buf.build/protocolbuffers/go
    out: gen/proto
    opt: paths=source_relative
  - remote: buf.build/grpc/go
    out: gen/proto
    opt: paths=source_relative
```

**Step 3: Create literature_review.proto**

Create `api/proto/literaturereview/v1/literature_review.proto`:

```protobuf
syntax = "proto3";

package literaturereview.v1;

option go_package = "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1;literaturereviewv1";

import "google/protobuf/timestamp.proto";
import "google/protobuf/duration.proto";
import "google/protobuf/wrappers.proto";

// LiteratureReviewService provides literature review capabilities.
service LiteratureReviewService {
  // StartLiteratureReview starts a new literature review from a query.
  rpc StartLiteratureReview(StartLiteratureReviewRequest) returns (StartLiteratureReviewResponse);

  // GetLiteratureReviewStatus gets the current status of a literature review.
  rpc GetLiteratureReviewStatus(GetLiteratureReviewStatusRequest) returns (GetLiteratureReviewStatusResponse);

  // CancelLiteratureReview cancels a running literature review.
  rpc CancelLiteratureReview(CancelLiteratureReviewRequest) returns (CancelLiteratureReviewResponse);

  // ListLiteratureReviews lists literature reviews for a project.
  rpc ListLiteratureReviews(ListLiteratureReviewsRequest) returns (ListLiteratureReviewsResponse);

  // GetLiteratureReviewPapers gets papers discovered in a review.
  rpc GetLiteratureReviewPapers(GetLiteratureReviewPapersRequest) returns (GetLiteratureReviewPapersResponse);

  // GetLiteratureReviewKeywords gets keywords used in a review.
  rpc GetLiteratureReviewKeywords(GetLiteratureReviewKeywordsRequest) returns (GetLiteratureReviewKeywordsResponse);

  // StreamLiteratureReviewProgress streams real-time progress updates.
  rpc StreamLiteratureReviewProgress(StreamLiteratureReviewProgressRequest) returns (stream LiteratureReviewProgressEvent);
}

// Enums

enum ReviewStatus {
  REVIEW_STATUS_UNSPECIFIED = 0;
  REVIEW_STATUS_PENDING = 1;
  REVIEW_STATUS_EXTRACTING_KEYWORDS = 2;
  REVIEW_STATUS_SEARCHING = 3;
  REVIEW_STATUS_EXPANDING = 4;
  REVIEW_STATUS_INGESTING = 5;
  REVIEW_STATUS_COMPLETED = 6;
  REVIEW_STATUS_FAILED = 7;
  REVIEW_STATUS_CANCELLED = 8;
}

enum IngestionStatus {
  INGESTION_STATUS_UNSPECIFIED = 0;
  INGESTION_STATUS_PENDING = 1;
  INGESTION_STATUS_QUEUED = 2;
  INGESTION_STATUS_INGESTING = 3;
  INGESTION_STATUS_COMPLETED = 4;
  INGESTION_STATUS_FAILED = 5;
  INGESTION_STATUS_SKIPPED = 6;
}

enum KeywordSourceType {
  KEYWORD_SOURCE_TYPE_UNSPECIFIED = 0;
  KEYWORD_SOURCE_TYPE_USER_QUERY = 1;
  KEYWORD_SOURCE_TYPE_PAPER_EXTRACTION = 2;
}

enum SortOrder {
  SORT_ORDER_UNSPECIFIED = 0;
  SORT_ORDER_ASC = 1;
  SORT_ORDER_DESC = 2;
}

// Request/Response messages

message StartLiteratureReviewRequest {
  string org_id = 1;
  string project_id = 2;
  string query = 3;
  google.protobuf.Int32Value initial_keyword_count = 4;
  google.protobuf.Int32Value paper_keyword_count = 5;
  google.protobuf.Int32Value max_expansion_depth = 6;
  repeated string source_filters = 7;
  google.protobuf.Timestamp date_from = 8;
  google.protobuf.Timestamp date_to = 9;
}

message StartLiteratureReviewResponse {
  string review_id = 1;
  string workflow_id = 2;
  ReviewStatus status = 3;
  repeated string initial_keywords = 4;
  google.protobuf.Timestamp created_at = 5;
  string message = 6;
}

message GetLiteratureReviewStatusRequest {
  string org_id = 1;
  string project_id = 2;
  string review_id = 3;
}

message GetLiteratureReviewStatusResponse {
  string review_id = 1;
  ReviewStatus status = 2;
  ReviewProgress progress = 3;
  string error_message = 4;
  google.protobuf.Timestamp created_at = 5;
  google.protobuf.Timestamp started_at = 6;
  google.protobuf.Timestamp completed_at = 7;
  google.protobuf.Duration duration = 8;
  ReviewConfiguration configuration = 9;
}

message CancelLiteratureReviewRequest {
  string org_id = 1;
  string project_id = 2;
  string review_id = 3;
  string reason = 4;
}

message CancelLiteratureReviewResponse {
  bool success = 1;
  string message = 2;
  ReviewStatus final_status = 3;
}

message ListLiteratureReviewsRequest {
  string org_id = 1;
  string project_id = 2;
  int32 page_size = 3;
  string page_token = 4;
  ReviewStatus status_filter = 5;
  google.protobuf.Timestamp created_after = 6;
  google.protobuf.Timestamp created_before = 7;
}

message ListLiteratureReviewsResponse {
  repeated LiteratureReviewSummary reviews = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}

message GetLiteratureReviewPapersRequest {
  string org_id = 1;
  string project_id = 2;
  string review_id = 3;
  int32 page_size = 4;
  string page_token = 5;
  IngestionStatus ingestion_status_filter = 6;
  string source_filter = 7;
}

message GetLiteratureReviewPapersResponse {
  repeated Paper papers = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}

message GetLiteratureReviewKeywordsRequest {
  string org_id = 1;
  string project_id = 2;
  string review_id = 3;
  int32 page_size = 4;
  string page_token = 5;
  google.protobuf.Int32Value extraction_round_filter = 6;
  KeywordSourceType source_type_filter = 7;
}

message GetLiteratureReviewKeywordsResponse {
  repeated ReviewKeyword keywords = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}

message StreamLiteratureReviewProgressRequest {
  string org_id = 1;
  string project_id = 2;
  string review_id = 3;
}

message LiteratureReviewProgressEvent {
  string review_id = 1;
  string event_type = 2;
  ReviewStatus status = 3;
  ReviewProgress progress = 4;
  string message = 5;
  google.protobuf.Timestamp timestamp = 6;

  oneof event_data {
    KeywordsExtractedEvent keywords_extracted = 10;
    PapersFoundEvent papers_found = 11;
    ExpansionStartedEvent expansion_started = 12;
    IngestionProgressEvent ingestion_progress = 13;
    ErrorEvent error = 14;
  }
}

// Domain messages

message ReviewProgress {
  int32 initial_keywords_count = 1;
  int32 total_keywords_processed = 2;
  int32 papers_found = 3;
  int32 papers_new = 4;
  int32 papers_ingested = 5;
  int32 papers_failed = 6;
  int32 current_expansion_depth = 7;
  int32 max_expansion_depth = 8;
  map<string, SourceProgress> source_progress = 9;
  google.protobuf.Duration elapsed_time = 10;
  google.protobuf.Duration estimated_remaining = 11;
}

message SourceProgress {
  string source_name = 1;
  int32 queries_completed = 2;
  int32 queries_total = 3;
  int32 papers_found = 4;
  int32 errors = 5;
  bool rate_limited = 6;
}

message ReviewConfiguration {
  int32 initial_keyword_count = 1;
  int32 paper_keyword_count = 2;
  int32 max_expansion_depth = 3;
  repeated string enabled_sources = 4;
  google.protobuf.Timestamp date_from = 5;
  google.protobuf.Timestamp date_to = 6;
}

message LiteratureReviewSummary {
  string review_id = 1;
  string original_query = 2;
  ReviewStatus status = 3;
  int32 papers_found = 4;
  int32 papers_ingested = 5;
  int32 keywords_used = 6;
  google.protobuf.Timestamp created_at = 7;
  google.protobuf.Timestamp completed_at = 8;
  google.protobuf.Duration duration = 9;
  string user_id = 10;
}

message Paper {
  string id = 1;
  string doi = 2;
  string arxiv_id = 3;
  string pubmed_id = 4;
  string semantic_scholar_id = 5;
  string openalex_id = 6;
  string title = 7;
  string abstract = 8;
  repeated Author authors = 9;
  google.protobuf.Timestamp publication_date = 10;
  int32 publication_year = 11;
  string venue = 12;
  string journal = 13;
  int32 citation_count = 14;
  string pdf_url = 15;
  bool open_access = 16;
  string discovered_via_source = 17;
  string discovered_via_keyword = 18;
  int32 expansion_depth = 19;
  IngestionStatus ingestion_status = 20;
  string ingestion_job_id = 21;
  repeated string extracted_keywords = 22;
}

message Author {
  string name = 1;
  string affiliation = 2;
  string orcid = 3;
}

message ReviewKeyword {
  string id = 1;
  string keyword = 2;
  string normalized_keyword = 3;
  KeywordSourceType source_type = 4;
  int32 extraction_round = 5;
  string source_paper_id = 6;
  string source_paper_title = 7;
  int32 papers_found = 8;
  float confidence_score = 9;
}

// Event messages

message KeywordsExtractedEvent {
  repeated string keywords = 1;
  int32 extraction_round = 2;
  string source_paper_id = 3;
}

message PapersFoundEvent {
  string source = 1;
  string keyword = 2;
  int32 count = 3;
  int32 new_count = 4;
}

message ExpansionStartedEvent {
  int32 depth = 1;
  int32 new_keywords = 2;
  int32 papers_to_process = 3;
}

message IngestionProgressEvent {
  int32 queued = 1;
  int32 completed = 2;
  int32 failed = 3;
}

message ErrorEvent {
  string error_code = 1;
  string error_message = 2;
  string phase = 3;
  bool recoverable = 4;
}
```

**Step 4: Generate protobuf code**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
buf dep update
buf generate
```

Expected: Generated Go code in `gen/proto/literaturereview/v1/`

**Step 5: Verify generated code compiles**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go build ./...
```

Expected: Build succeeds

**Step 6: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add api/proto/ buf.yaml buf.gen.yaml gen/
git commit -m "feat: add protobuf definitions

- Add LiteratureReviewService with all RPC methods
- Add enums: ReviewStatus, IngestionStatus, KeywordSourceType
- Add request/response messages for all operations
- Add domain messages: Paper, Author, ReviewKeyword, etc.
- Add event messages for progress streaming
- Add buf.yaml and buf.gen.yaml for code generation
- Generate Go code with grpc plugin"
```

---

## Task 8: Update go.work and Verify Integration

**Files:**
- Modify: `/Users/mehdiyazdani/dev/VsCodeProjects/Helixir/go.work`

**Step 1: Add literature_service to go.work**

Update the root `go.work` to include the new service:

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir
```

Add `./literature_service` to the `use` block in `go.work`.

**Step 2: Verify workspace builds**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir
go work sync
go build ./literature_service/...
```

Expected: Build succeeds

**Step 3: Run all tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
make test
```

Expected: All tests pass

**Step 4: Commit in root repo**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir
git add go.work
git commit -m "chore: add literature_service to go workspace"
```

**Step 5: Commit literature_service submodule reference**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir
git add literature_service
git commit -m "chore: add literature_service submodule"
```

---

## Phase 1 Completion Checklist

After completing all tasks:

- [ ] `make build` succeeds
- [ ] `make test` passes
- [ ] `make lint` passes (install golangci-lint if needed)
- [ ] Config loads from file + env vars
- [ ] Database migrations files exist
- [ ] Proto-generated code compiles
- [ ] All domain models have tests

---

## Next Steps

After Phase 1 is complete, proceed to **Phase 2: Infrastructure** which includes:
- Repository layer implementation
- Paper identity resolution
- Rate limiter pool
- Temporal client setup
- mTLS configuration
- Outbox integration
- Observability setup
