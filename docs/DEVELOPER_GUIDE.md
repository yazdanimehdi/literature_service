# Developer Guide

Comprehensive guide for developing, testing, and extending the Literature Review Service.

## Table of Contents

- [Development Setup](#development-setup)
- [Building](#building)
- [Running Tests](#running-tests)
- [Code Generation](#code-generation)
- [Database Migrations](#database-migrations)
- [Configuration Reference](#configuration-reference)
- [Project Architecture](#project-architecture)
- [Project Conventions](#project-conventions)
- [Adding a New Paper Source](#adding-a-new-paper-source)
- [Adding a New LLM Provider](#adding-a-new-llm-provider)
- [Workflow Development](#workflow-development)
- [Troubleshooting](#troubleshooting)

---

## Development Setup

### Prerequisites

| Tool             | Version  | Purpose                        |
|------------------|----------|--------------------------------|
| Go               | 1.25+    | Build and test                 |
| golangci-lint    | latest   | Linting                        |
| buf              | latest   | Protobuf code generation       |
| migrate          | latest   | Database migration CLI         |
| Docker + Compose | latest   | Local infrastructure           |

### Install development tools

```bash
make dev-setup
```

This installs `golangci-lint`, `buf`, and the `migrate` CLI via `go install`.

### Clone and initialize

```bash
git clone --recurse-submodules git@bitbucket.org:deepbio/helixir.git
cd helixir/literature_service
go mod download
```

The Go workspace file (`go.work` at the monorepo root) links all modules. The `go.mod` has `replace` directives pointing to sibling modules (`grpcauth`, `outbox`, `ingestion_service`, `llm`).

---

## Building

The service produces three binaries from `cmd/`:

| Binary                         | Source          | Purpose                                  |
|--------------------------------|-----------------|------------------------------------------|
| `literature-review-server`     | `cmd/server/`   | gRPC + HTTP REST API server              |
| `literature-review-worker`     | `cmd/worker/`   | Temporal workflow worker                 |
| `literature-review-migrate`    | `cmd/migrate/`  | Database migration CLI tool              |

### Build commands

```bash
make build              # All 3 binaries -> bin/
make build-server       # Server only
make build-worker       # Worker only
make build-migrate      # Migrate only
```

### Run locally

```bash
make run-server         # Builds and runs the server
make run-worker         # Builds and runs the worker
```

Both targets build before running.

### Clean

```bash
make clean              # Remove bin/, coverage files
```

---

## Running Tests

### Unit tests

```bash
make test               # Unit tests with race detector + verbose + coverage
make test-coverage      # Same, plus generates coverage.html report
```

Unit tests use the `-short` flag convention. Tests that require live infrastructure check `testing.Short()` and skip themselves when running under `make test`.

### Race detection (extended)

```bash
make test-race          # Runs the full suite 3 times with -race
```

### Integration tests

Integration tests run against real PostgreSQL, Temporal, and Kafka containers:

```bash
make test-integration   # Starts docker-compose.test.yml, runs tests, stops containers
```

Test infrastructure uses offset ports to avoid conflicts with development services:

| Service    | Dev Port | Test Port |
|------------|----------|-----------|
| PostgreSQL | 5432     | 5433      |
| Temporal   | 7233     | 7234      |
| Kafka      | 9092     | 9093      |

### Specialized test suites

```bash
make test-chaos         # Chaos/resilience tests (concurrent access, failure injection)
make test-security      # Security-focused tests (SQL injection, XSS, LIKE pattern injection)
make test-fuzz          # Fuzz testing (30-second fuzz runs)
make test-e2e           # End-to-end tests (requires full stack running)
make test-load          # Load tests with k6 (requires k6 CLI and running server)
```

### Run everything

```bash
make test-all           # Unit + race (3x) + chaos + security
```

### Test conventions

- **pgxmock** is used for repository-layer tests (no real database needed).
- **Temporal test environment** is used for workflow and activity tests.
- **testcontainers-go** is used in integration tests for on-demand containers.
- Test files follow `*_test.go` naming and use `testify` for assertions.

---

## Code Generation

### Protobuf

Protobuf definitions live in `api/proto/literaturereview/v1/`. Generated Go code is output to `gen/proto/literaturereview/v1/`.

```bash
make proto              # Generate Go code from .proto files (requires buf)
make proto-lint         # Lint protobuf definitions
```

Configuration files:

- `buf.yaml` -- module configuration and lint rules
- `buf.gen.yaml` -- code generation plugins

---

## Database Migrations

Migration files live in `migrations/` and use sequential numbering:

```
migrations/
  000001_init_extensions.up.sql
  000001_init_extensions.down.sql
  000002_enums.up.sql
  000002_enums.down.sql
  000003_core_tables.up.sql
  000003_core_tables.down.sql
  000004_tenant_tables.up.sql
  000004_tenant_tables.down.sql
  000005_outbox.up.sql
  000005_outbox.down.sql
  000006_functions.up.sql
  000006_functions.down.sql
  000007_triggers.up.sql
  000007_triggers.down.sql
  000008_performance_indexes.up.sql
  000008_performance_indexes.down.sql
```

### CLI usage

The `literature-review-migrate` binary supports these flags:

```bash
./bin/literature-review-migrate -up              # Apply all pending migrations
./bin/literature-review-migrate -down            # Roll back all migrations
./bin/literature-review-migrate -steps 1         # Apply 1 migration forward
./bin/literature-review-migrate -steps -1        # Roll back 1 migration
./bin/literature-review-migrate -version         # Print current version
./bin/literature-review-migrate -force 5         # Force version (recover from dirty state)
./bin/literature-review-migrate -path ./other    # Override migrations directory
```

### Makefile shortcuts

```bash
make migrate-up         # Apply pending migrations
make migrate-down       # Roll back last migration
make migrate-create     # Create a new migration (prompts for name)
```

### Auto-migration

Set `database.migration_auto_run: true` in `config.yaml` or `LITREVIEW_DATABASE_MIGRATION_AUTO_RUN=true` to run migrations automatically on server startup. This is disabled by default and not recommended for production.

---

## Configuration Reference

The service uses a layered configuration approach:

1. **`config/config.yaml`** -- all non-secret default values
2. **Environment variables** -- override any config value with `LITREVIEW_<SECTION>_<KEY>`
3. **`.env` file** -- loaded automatically for secrets (API keys, passwords)

### Key configuration sections

| Section              | Description                                      |
|----------------------|--------------------------------------------------|
| `server`             | HTTP/gRPC/metrics ports, timeouts                |
| `database`           | PostgreSQL connection pool, SSL mode, migrations |
| `temporal`           | Server address, namespace, task queue            |
| `logging`            | Level, format, output destination                |
| `metrics`            | Prometheus endpoint                              |
| `tracing`            | OpenTelemetry distributed tracing                |
| `llm`                | Provider selection, model config, resilience     |
| `kafka`              | Broker addresses, topic name                     |
| `outbox`             | Poll interval, batch size, worker count          |
| `paper_sources`      | Per-source enable/disable, rate limits, URLs     |
| `ingestion_service`  | gRPC address and timeout                         |

### Environment variable examples

```bash
# Database
LITREVIEW_DATABASE_HOST=db.example.com
LITREVIEW_DATABASE_PASSWORD=secret
LITREVIEW_DATABASE_SSL_MODE=require
LITREVIEW_DATABASE_MAX_CONNS=100

# Server
LITREVIEW_SERVER_HTTP_PORT=8080
LITREVIEW_SERVER_GRPC_PORT=9090

# LLM
LITREVIEW_LLM_PROVIDER=anthropic
LITREVIEW_LLM_ANTHROPIC_API_KEY=sk-ant-...
LITREVIEW_LLM_TIMEOUT=120s

# Temporal
LITREVIEW_TEMPORAL_HOST_PORT=temporal:7233
LITREVIEW_TEMPORAL_NAMESPACE=literature-review

# Paper source API keys (all optional)
LITREVIEW_PAPER_SOURCES_SEMANTIC_SCHOLAR_API_KEY=...
LITREVIEW_PAPER_SOURCES_OPENALEX_API_KEY=...
LITREVIEW_PAPER_SOURCES_PUBMED_API_KEY=...
```

See `.env.example` for the complete list of secret variables.

### LLM providers

| Provider    | Config key           | API key variable                   |
|-------------|----------------------|------------------------------------|
| OpenAI      | `llm.provider: openai`     | `LITREVIEW_LLM_OPENAI_API_KEY`      |
| Anthropic   | `llm.provider: anthropic`  | `LITREVIEW_LLM_ANTHROPIC_API_KEY`   |
| Azure OpenAI| `llm.provider: azure`      | `LITREVIEW_LLM_AZURE_API_KEY`       |
| AWS Bedrock | `llm.provider: bedrock`    | AWS credential chain (no key var)   |
| Google Gemini| `llm.provider: gemini`    | `LITREVIEW_LLM_GEMINI_API_KEY`      |
| Vertex AI   | `llm.provider: gemini`     | GCP Application Default Credentials |

### Paper sources

Enabled by default: Semantic Scholar, OpenAlex, PubMed, bioRxiv, arXiv.
Disabled by default: Scopus (requires API key).

Each source has independent configuration for `enabled`, `base_url`, `timeout`, `rate_limit`, and `max_results` under the `paper_sources` section.

---

## Project Architecture

### Directory structure

```
literature_service/
  cmd/
    server/          -- gRPC + HTTP server entrypoint
    worker/          -- Temporal worker entrypoint
    migrate/         -- Database migration CLI
  internal/
    config/          -- Viper-based configuration loading
    domain/          -- Domain models, errors, enums (no external dependencies)
    repository/      -- PostgreSQL repositories (pgx v5)
    database/        -- Connection pool management, migrator
    server/          -- gRPC handlers (protobuf service implementation)
    server/http/     -- HTTP REST API (chi router)
    temporal/        -- Temporal client, worker manager, signal/query constants
    temporal/workflows/   -- Workflow definitions (deterministic)
    temporal/activities/  -- Activity implementations (side effects)
    llm/             -- LLM keyword extractor using shared llm package
    papersources/    -- Paper source interface + registry
    papersources/semanticscholar/  -- Semantic Scholar client
    papersources/openalex/         -- OpenAlex client
    papersources/pubmed/           -- PubMed client
    ingestion/       -- Ingestion service gRPC client
    outbox/          -- Outbox pattern adapter and emitter
    observability/   -- Logging, metrics, Temporal logger adapter
    dedup/           -- Paper deduplication logic
    pagination/      -- Pagination helpers
  api/proto/         -- Protobuf definitions (.proto files)
  gen/proto/         -- Generated protobuf Go code
  migrations/        -- SQL migration files (sequential)
  config/            -- config.yaml (default configuration)
  deploy/helm/       -- Helm chart for Kubernetes deployment
  tests/
    integration/     -- Integration tests (require Docker)
    e2e/             -- End-to-end tests (require full stack)
    chaos/           -- Chaos/resilience tests
    security/        -- Security and fuzz tests
  pkg/testutil/      -- Shared test utilities
  scripts/           -- Helper scripts
```

### Request flow

1. Client sends HTTP POST to `/api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews`
2. HTTP handler validates input and creates a `LiteratureReviewRequest` in PostgreSQL
3. Handler starts a Temporal workflow via `ReviewWorkflowClient`
4. Temporal dispatches the `LiteratureReviewWorkflow` to a worker
5. Workflow executes activities in phases: extract keywords, search, expand, ingest
6. Activities write results to PostgreSQL and publish events via the outbox
7. Client polls status or streams progress via SSE

### Temporal workflow phases

```
Phase 1: ExtractKeywords   -- LLM extracts keywords from user query
Phase 2: SearchPapers      -- Query all enabled paper sources per keyword
Phase 3: Expansion         -- Extract keywords from paper abstracts, search again (N rounds)
Phase 4: Ingestion         -- Submit papers with PDFs to ingestion service
Phase 5: Complete          -- Update final status and publish completion event
```

---

## Project Conventions

### Logging

- **zerolog** for structured JSON logging.
- Use per-logger level configuration (`logger.Level()`), never `zerolog.SetGlobalLevel()`.
- When logging multiple errors, use `.AnErr("additional_error", err)` -- calling `.Err()` twice overwrites the first.

### Database

- **pgxpool** (pgx v5) for PostgreSQL connection pooling.
- **pgxmock** for repository unit tests.
- All queries are parameterized. LIKE queries escape `%`, `_`, and `\` to prevent pattern injection.
- `io.LimitReader` is used on all HTTP response body reads to prevent resource exhaustion.

### Error handling

- Use `errors.As` for wrapped error type checking, never type assertions.
- Internal error messages must not leak to gRPC or HTTP clients. Use generic messages.
- Domain errors (`domain.ErrNotFound`, `domain.ErrConflict`, etc.) are mapped to appropriate HTTP/gRPC status codes.

### Multi-tenancy

Every endpoint and repository operation is scoped to `org_id` and `project_id`. Tenant context is extracted from URL path parameters and propagated through middleware.

### Commits

Follow [Conventional Commits](https://www.conventionalcommits.org/): `feat(scope):`, `fix(scope):`, `docs:`, `refactor:`, `test:`, `chore:`.

### Code style

- Stateless methods on structs should be package-level functions.
- `time.After` must not be used in long-lived goroutines (use `time.NewTimer` + `defer timer.Stop()`).
- Configuration uses Viper with `mapstructure` tags.

---

## Adding a New Paper Source

Follow these steps to integrate a new academic database.

### 1. Create the package

```
internal/papersources/newsource/
  client.go       -- Client implementation
  client_test.go  -- Tests
  types.go        -- Source-specific response types
```

### 2. Implement the PaperSource interface

```go
// internal/papersources/source.go defines the interface:
type PaperSource interface {
    Search(ctx context.Context, params SearchParams) (*SearchResult, error)
    GetByID(ctx context.Context, id string) (*domain.Paper, error)
    SourceType() domain.SourceType
    Name() string
    IsEnabled() bool
}
```

Your implementation must:

- Respect context cancellation and deadlines.
- Apply rate limiting using `papersources.NewRateLimiter()`.
- Transform source-specific responses into `domain.Paper`.
- Use `io.LimitReader` when reading HTTP response bodies.
- Wrap errors with source context.

### 3. Add the source type enum

In `internal/domain/models.go`, add a new `SourceType` constant:

```go
SourceTypeNewSource SourceType = "new_source"
```

Add a corresponding value to the database enum migration.

### 4. Add configuration

In `config/config.yaml` under `paper_sources`:

```yaml
new_source:
  enabled: false
  base_url: https://api.newsource.org
  timeout: 30s
  rate_limit: 5.0
  max_results: 100
```

Add the corresponding Go struct fields in the config package.

### 5. Register in the worker

In `cmd/worker/main.go`, add registration in `registerPaperSources()`:

```go
if cfg.PaperSources.NewSource.Enabled {
    nsCfg := cfg.PaperSources.NewSource
    nsClient := newsource.New(newsource.Config{
        BaseURL:    nsCfg.BaseURL,
        APIKey:     nsCfg.APIKey,
        Timeout:    nsCfg.Timeout,
        RateLimit:  nsCfg.RateLimit,
        MaxResults: nsCfg.MaxResults,
        Enabled:    true,
    })
    registry.Register(nsClient)
    logger.Info().Msg("registered paper source: NewSource")
}
```

### 6. Add API key to .env.example

```bash
LITREVIEW_PAPER_SOURCES_NEW_SOURCE_API_KEY=
```

### 7. Write tests

Write tests using `httptest.NewServer` to mock the external API. Test error handling, rate limiting, and response parsing.

---

## Adding a New LLM Provider

The service uses the shared `github.com/helixir/llm` package for LLM operations. The `llm` package implements a factory pattern with a resilience middleware layer.

### 1. Implement the Client interface

In the `llm` package (separate repository), implement the `Client` interface:

```go
type Client interface {
    Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
    Close() error
}
```

### 2. Add to the factory

Add a case to `createClient()` in `factory.go` within the `llm` package.

### 3. Add configuration

Add config fields to `FactoryConfig` in the `llm` package and to `config.go` in the literature service:

```yaml
llm:
  new_provider:
    model: "model-name"
    base_url: "https://api.newprovider.com"
```

### 4. Add startup validation

Ensure the config package validates that the required API key is set when the provider is selected.

### 5. Add the API key variable

In `.env.example`:

```bash
LITREVIEW_LLM_NEW_PROVIDER_API_KEY=
```

---

## Workflow Development

### Determinism rules

Temporal workflows must be deterministic. The `internal/temporal/workflows/determinism.go` module documents these constraints. Key rules:

- Use `workflow.Now(ctx)` instead of `time.Now()`.
- Use `workflow.Sleep(ctx, d)` instead of `time.Sleep(d)`.
- Use `workflow.Go(ctx, fn)` instead of `go fn()`.
- Never make network calls, file I/O, or use random numbers in workflow code.
- All side effects must be in activities.

### Replay testing

Workflow determinism is verified with replay tests in `internal/temporal/workflows/replay_test.go`. When changing workflow logic, update the replay test fixtures.

### Activity timeouts

| Activity Type  | Start-to-Close Timeout | Heartbeat Timeout |
|----------------|------------------------|-------------------|
| LLM extraction | 2 minutes              | --                |
| Paper search   | 5 minutes              | --                |
| Status update  | 30 seconds             | --                |
| Ingestion      | 5 minutes              | 2 minutes         |
| Event publish  | 30 seconds             | --                |

### Signals and queries

- **Cancel signal** (`cancel`): Cancels a running workflow. Sent by the DELETE endpoint.
- **Progress query** (`progress`): Returns current `workflowProgress` snapshot. Used by the status and streaming endpoints.

---

## Troubleshooting

### Build errors

```bash
# Refresh dependencies
go mod download
go mod tidy

# If sibling module changes are not picked up
cd .. && go work sync && cd literature_service
```

### Test failures

```bash
# Run unit tests only (skips integration tests)
make test

# Run a specific test
go test -v -run TestSpecificName ./internal/server/http/...
```

### Missing protobuf generated code

```bash
make proto
```

Ensure `buf` is installed: `make dev-setup`.

### Database connection errors

- Verify PostgreSQL is running: `docker compose ps`
- Check connection settings: `LITREVIEW_DATABASE_HOST`, `LITREVIEW_DATABASE_PORT`
- For local development, set `LITREVIEW_DATABASE_SSL_MODE=disable`
- Run migrations: `./bin/literature-review-migrate -up`

### Temporal connection errors

- Verify Temporal is running: `docker compose ps`
- Check `LITREVIEW_TEMPORAL_HOST_PORT` (default: `localhost:7233`)
- Inspect workflows in the Temporal UI at [http://localhost:8088](http://localhost:8088)
- Ensure the namespace exists: `temporal operator namespace describe literature-review`

### Worker not processing workflows

- Verify the worker is connected to the correct task queue (`literature-review-tasks`)
- Check that the worker registered both workflows and activities (see startup logs)
- Look for errors in the Temporal UI workflow history

### LLM errors

- Verify the API key is set for the configured provider
- Check the provider name matches `llm.provider` in config
- Rate limit or budget errors appear in worker logs; enable `llm.resilience.enabled: true` to add automatic retry/circuit breaker

### Lint errors

```bash
make lint
```

The project uses `golangci-lint`. If you need to update the linter configuration, edit `.golangci.yml`.

### Docker build context

The Dockerfile must be built from the monorepo root because `go.mod` has `replace` directives pointing to sibling modules:

```bash
# From literature_service/ directory:
make docker-build    # Automatically uses cd .. as build context
```
