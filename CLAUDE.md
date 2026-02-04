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
