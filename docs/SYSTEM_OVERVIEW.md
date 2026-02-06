# Literature Review Service - System Overview

## Table of Contents

- [Purpose](#purpose)
- [Architecture Overview](#architecture-overview)
- [Tech Stack](#tech-stack)
- [Directory Structure](#directory-structure)
- [Three Binaries](#three-binaries)
- [Multi-Tenancy Model](#multi-tenancy-model)
- [Service Dependencies](#service-dependencies)
- [Workflow Pipeline](#workflow-pipeline)
- [gRPC API](#grpc-api)
- [HTTP REST API](#http-rest-api)
- [Database Schema](#database-schema)
- [Paper Source Clients](#paper-source-clients)
- [LLM Integration](#llm-integration)
- [Outbox Events](#outbox-events)
- [Observability](#observability)
- [Configuration](#configuration)
- [Deployment](#deployment)
- [Testing](#testing)
- [Domain Model](#domain-model)
- [Review Status Lifecycle](#review-status-lifecycle)
- [Dependencies and Shared Packages](#dependencies-and-shared-packages)

---

## Purpose

The Literature Review Service is a Go microservice in the Helixir monorepo that performs
automated literature reviews for biomedical and scientific research. Given a natural
language research query, the service:

1. **Accepts natural language research queries** from users via gRPC or HTTP REST APIs.
2. **Extracts keywords** from the query using an LLM (one of six supported providers:
   OpenAI, Anthropic, Azure OpenAI, AWS Bedrock, Google Gemini, or Vertex AI).
3. **Searches multiple academic databases concurrently** (Semantic Scholar, OpenAlex,
   PubMed) for papers matching those keywords.
4. **Recursively expands** the search by extracting new keywords from discovered paper
   abstracts, broadening coverage across up to five expansion rounds.
5. **Manages paper ingestion** through the companion Ingestion Service, submitting
   papers with PDF URLs for full-text download, parsing, and storage via gRPC.
6. **Publishes events to Kafka** via the transactional outbox pattern for downstream
   observability, analytics, and notifications.

The entire pipeline is orchestrated as a Temporal workflow, giving the service durable
execution, automatic retries, cancellation support, and real-time progress queries --
all without custom state machine code.

---

## Architecture Overview

```
                                    +-------------------+
                                    |     Clients       |
                                    | (Web UI, CLI, CI) |
                                    +---------+---------+
                                              |
                             +----------------+----------------+
                             |                                 |
                       gRPC :9090                         HTTP :8080
                             |                                 |
                  +----------v----------+          +-----------v-----------+
                  |   gRPC Server       |          |   HTTP REST Server    |
                  |   (protobuf)        |          |   (Chi v5 router)     |
                  |                     |          |                       |
                  |  - StartReview      |          |  - POST /reviews      |
                  |  - GetStatus        |          |  - GET  /reviews      |
                  |  - CancelReview     |          |  - GET  /reviews/:id  |
                  |  - ListReviews      |          |  - DELETE /reviews/:id|
                  |  - GetPapers        |          |  - GET  /papers       |
                  |  - GetKeywords      |          |  - GET  /keywords     |
                  |  - StreamProgress   |          |  - GET  /progress/sse |
                  +----------+----------+          +-----------+-----------+
                             |                                 |
                             |       grpcauth (JWT)            |
                             +----------------+----------------+
                                              |
                             +----------------v----------------+
                             |          PostgreSQL              |
                             |  - literature_review_requests    |
                             |  - papers, keywords              |
                             |  - request_*_mappings            |
                             |  - outbox_events                 |
                             |  - review_progress_events        |
                             |  (LISTEN/NOTIFY for real-time)   |
                             +----------------+----------------+
                                              |
                             +----------------v----------------+
                             |       Temporal Server            |
                             |  namespace: literature-review    |
                             |  task queue: literature-review-  |
                             |              tasks               |
                             +----------------+----------------+
                                              |
                  +---------------------------v---------------------------+
                  |                  Temporal Worker                      |
                  |                                                       |
                  |   LiteratureReviewWorkflow (parent)                   |
                  |     Phase 1: Extract Keywords  (LLM Activity)         |
                  |     Phase 2: Search Papers     (Search Activity)      |
                  |     Phase 3: Batch Processing  (Child Workflows)      |
                  |       +-> PaperProcessingWorkflow (per batch of 5)    |
                  |       |     Stage 1: Embed abstracts (Embedding Act.) |
                  |       |     Stage 2: Dedup via Qdrant (Dedup Act.)    |
                  |       |     Stage 3: Ingest papers   (Ingestion Act.) |
                  |       |     Stage 4: Update records  (Status Act.)    |
                  |       +-> Signals parent on completion                |
                  |     Phase 4: Expand Search     (LLM + Search)         |
                  |       +-> Relevance Gate (embedding-based filtering)  |
                  |       +-> Spawns child workflows per expansion round  |
                  |     Phase 5: Coverage Review   (LLM Activity)         |
                  |       +-> Gap expansion if coverage < threshold       |
                  |     Phase 6: Complete           (Status Activity)     |
                  |                                                       |
                  |   Signals: cancel, pause, resume, stop                |
                  |   Queries: progress (real-time counters)              |
                  +---+------+------+------+------+------+------+---+----+
                      |      |      |      |      |      |      |   |
                      v      v      v      v      v      v      v   v
                 +------+ +------+ +------+ +------+ +------+ +------+
                 | LLM  | |Seman.| |Open  | |PubMed| |Ingest| |Qdrant|
                 |Provdr | |Scholr| |Alex  | | API  | |Srvce | |Vector|
                 |(6opt) | | API  | | API  | |      | |(gRPC)| |Store |
                 +------+ +------+ +------+ +------+ +------+ +------+
                                                         |
                  +--------------------------------------+
                  |            Outbox Pattern                      |
                  |  outbox table --> relay --> Kafka topic         |
                  |  topic: events.outbox.literature_review_service|
                  +-----------------------------------------------+

                 Prometheus Metrics :9091  /metrics
```

### Network Ports

| Port | Protocol | Purpose                                |
|------|----------|----------------------------------------|
| 8080 | HTTP     | REST API and SSE progress streaming     |
| 9090 | gRPC     | Protobuf RPC API                       |
| 9091 | HTTP     | Prometheus metrics endpoint (`/metrics`)|

---

## Tech Stack

| Category             | Technology                                                       |
|----------------------|------------------------------------------------------------------|
| Language             | Go 1.25                                                          |
| gRPC / Protobuf      | google.golang.org/grpc v1.78, buf for code generation           |
| HTTP Router          | Chi v5 (`github.com/go-chi/chi/v5`)                             |
| Database             | PostgreSQL via pgx v5 (`jackc/pgx/v5`) with pgxpool             |
| Migrations           | golang-migrate/v4 with `postgres` driver                        |
| Workflow Engine      | Temporal SDK (`go.temporal.io/sdk` v1.39)                       |
| LLM Integration      | Shared `github.com/helixir/llm` package (ResilientClient)      |
| Auth                 | Shared `github.com/helixir/grpcauth` (JWT interceptors)        |
| Event Publishing     | Shared `github.com/helixir/outbox` (transactional outbox)       |
| Metrics              | Prometheus (`prometheus/client_golang`)                         |
| Logging              | zerolog (`rs/zerolog`) with structured JSON output              |
| Configuration        | Viper with `LITREVIEW_` env prefix, YAML config files           |
| Validation           | go-playground/validator/v10                                     |
| Testing              | testify, pgxmock, Temporal test environment, testcontainers     |

---

## Directory Structure

```
literature_service/
├── api/proto/literaturereview/v1/   # Protobuf service and message definitions
│   └── literature_review.proto      # LiteratureReviewService (7 RPCs)
├── cmd/
│   ├── server/main.go               # gRPC + HTTP server entrypoint
│   ├── worker/main.go               # Temporal worker entrypoint
│   └── migrate/main.go              # Database migration CLI
├── config/
│   └── config.yaml                  # Default configuration (non-secret values)
├── deploy/
│   └── helm/literature-review-service/  # Helm chart for Kubernetes
│       ├── templates/
│       │   ├── server-deployment.yaml
│       │   ├── worker-deployment.yaml
│       │   ├── server-hpa.yaml
│       │   ├── worker-hpa.yaml
│       │   ├── server-pdb.yaml
│       │   ├── worker-pdb.yaml
│       │   ├── service.yaml
│       │   ├── servicemonitor.yaml
│       │   ├── networkpolicy.yaml
│       │   ├── configmap.yaml
│       │   ├── externalsecret.yaml
│       │   └── kafka-topics-job.yaml
│       ├── values.yaml
│       ├── values-staging.yaml
│       └── values-production.yaml
├── gen/proto/literaturereview/v1/   # Generated protobuf Go code (buf)
├── internal/
│   ├── config/                      # Viper-based configuration loading
│   ├── database/                    # pgxpool connection pool + Migrator
│   ├── domain/                      # Domain models, enums, errors, events
│   │   ├── models.go                # ReviewStatus, SourceType, Tenant, enums
│   │   ├── review.go                # LiteratureReviewRequest, ReviewProgress
│   │   ├── paper.go                 # Paper, Author, CanonicalID generation
│   │   ├── keyword.go               # Keyword models
│   │   ├── events.go                # OutboxEvent, event type constants + payloads
│   │   └── errors.go                # Domain error types
│   ├── budget/                      # LLM budget tracking
│   ├── dedup/                       # Semantic deduplication logic
│   ├── ingestion/                   # gRPC client for the Ingestion Service
│   ├── llm/                         # LLM factory, KeywordExtractor interface
│   │   ├── extractor.go             # Prompt engineering, JSON response parsing
│   │   └── errors.go                # LLM-specific error types
│   ├── pdf/                         # PDF handling utilities
│   ├── qdrant/                      # Qdrant vector store integration
│   ├── observability/
│   │   ├── logger.go                # zerolog logger factory
│   │   ├── temporal_logger.go       # zerolog adapter for Temporal SDK logger
│   │   ├── metrics.go               # Prometheus metrics (reviews, searches, LLM)
│   │   └── context.go               # Context-based logging helpers
│   ├── outbox/                      # Outbox emitter, adapter, publisher
│   ├── papersources/                # Paper source abstraction layer
│   │   ├── source.go                # PaperSource interface
│   │   ├── registry.go              # Source registry (register/lookup by name)
│   │   ├── httpclient.go            # Shared HTTP client with rate limiting
│   │   ├── ratelimit.go             # Per-source token bucket rate limiter
│   │   ├── semanticscholar/         # Semantic Scholar API client
│   │   │   ├── client.go
│   │   │   └── types.go
│   │   ├── openalex/                # OpenAlex API client
│   │   │   ├── client.go
│   │   │   └── types.go
│   │   └── pubmed/                  # PubMed E-utilities API client
│   │       ├── client.go
│   │       └── types.go
│   ├── repository/                  # PostgreSQL repository layer
│   │   ├── repository.go            # Repository interfaces
│   │   ├── review_repository.go     # ReviewRepository interface
│   │   ├── paper_repository.go      # PaperRepository interface
│   │   ├── keyword_repository.go    # KeywordRepository interface
│   │   ├── pg_review_repository.go  # PostgreSQL ReviewRepository implementation
│   │   ├── pg_paper_repository.go   # PostgreSQL PaperRepository implementation
│   │   └── pg_keyword_repository.go # PostgreSQL KeywordRepository implementation
│   ├── server/                      # gRPC service handlers
│   │   ├── review_handlers.go       # Start, Get, Cancel, List reviews
│   │   ├── paper_handlers.go        # GetPapers, GetKeywords (gRPC)
│   │   ├── stream_handler.go        # StreamLiteratureReviewProgress (gRPC)
│   │   ├── convert.go               # Domain <-> Protobuf conversion helpers
│   │   └── http/                    # HTTP REST API handlers
│   │       ├── paper_handlers.go    # GET /papers, GET /keywords
│   │       ├── response.go          # JSON response helpers
│   │       └── middleware_test.go    # Auth middleware tests
│   └── temporal/                    # Temporal workflow orchestration
│       ├── client.go                # ReviewWorkflowClient, TLS config, signals
│       ├── worker.go                # WorkerManager, activity/workflow registration
│       ├── activities/
│       │   ├── types.go             # Activity input/output structs
│       │   ├── llm_activities.go    # ExtractKeywords + AssessCoverage activities
│       │   ├── search_activities.go # SearchSingleSource activity
│       │   ├── status_activities.go # UpdateStatus, SaveKeywords, SavePapers, UpdatePauseState
│       │   ├── ingestion_activities.go # DownloadAndIngestPapers activity
│       │   ├── event_activities.go  # PublishEvent activity (outbox)
│       │   ├── embedding_activities.go # EmbedText, EmbedPapers, ScoreKeywordRelevance
│       │   ├── dedup_activities.go  # BatchDedup against Qdrant vector store
│       │   └── budget_reporter.go   # LLM budget tracking via outbox
│       └── workflows/
│           ├── review_workflow.go   # LiteratureReviewWorkflow (parent orchestrator)
│           ├── paper_processing_workflow.go # PaperProcessingWorkflow (child, batch processing)
│           ├── pipeline_types.go    # Pipeline types, parent-child signals, batch types
│           ├── signals.go           # PauseSignal, ResumeSignal, StopSignal structs
│           ├── determinism.go       # Workflow determinism utilities
│           └── replay_test.go       # Workflow replay tests
├── migrations/                      # SQL migration files (8 migrations)
│   ├── 000001_init_extensions       # uuid-ossp, pg_trgm extensions
│   ├── 000002_enums                 # PostgreSQL enum types
│   ├── 000003_core_tables           # keywords, papers, identifiers, sources, searches
│   ├── 000004_tenant_tables         # review requests, keyword/paper mappings, progress
│   ├── 000005_outbox                # Transactional outbox table
│   ├── 000006_functions             # Database functions
│   ├── 000007_triggers              # Database triggers (updated_at, NOTIFY)
│   └── 000008_performance_indexes   # Composite and partial indexes
├── tests/
│   ├── chaos/                       # Chaos engineering tests
│   ├── e2e/                         # End-to-end tests (full stack)
│   ├── integration/                 # Integration tests (DB, Temporal, outbox)
│   ├── pipeline/                    # Pipeline workflow tests
│   ├── loadtest/                    # k6 load test scripts
│   └── security/                    # Fuzz testing, injection tests
├── Dockerfile                       # Multi-stage Docker build
├── docker-compose.yml               # Local development stack
├── Makefile                         # Build, test, lint, Docker, compose targets
├── buf.yaml / buf.gen.yaml          # Buf protobuf configuration
└── tools.go                         # Tool dependencies tracked in go.mod
```

---

## Three Binaries

The service compiles into three separate binaries, each with a distinct runtime role.

### 1. literature-review-server

**Entrypoint:** `cmd/server/main.go`

The API server that handles client requests. It runs three listeners concurrently:

- **gRPC server** on port 9090 -- serves the `LiteratureReviewService` with 7 RPCs
  (StartReview, GetStatus, CancelReview, ListReviews, GetPapers, GetKeywords,
  StreamProgress). Configured with grpcauth JWT interceptors, gRPC health checks,
  server reflection, 16 MB max message size, 100 max concurrent streams, and keepalive
  parameters.
- **HTTP REST server** on port 8080 -- Chi v5 router providing a REST API with
  Server-Sent Events (SSE) for real-time progress streaming. Write timeout is extended
  to 5 minutes to support long-lived SSE connections.
- **Prometheus metrics server** on port 9091 -- exposes `/metrics` endpoint.

On startup, the server connects to PostgreSQL, optionally runs auto-migrations, creates
repositories, connects to Temporal, initializes the grpcauth interceptor, and starts all
three listeners. Graceful shutdown handles SIGINT/SIGTERM with configurable timeout
(default 30s), draining active connections before exit.

### 2. literature-review-worker

**Entrypoint:** `cmd/worker/main.go`

The Temporal worker that polls the `literature-review-tasks` task queue and executes
workflow activities. On startup, it initializes:

- **LLM KeywordExtractor** -- creates the configured LLM provider (via factory pattern)
  with optional resilience middleware (rate limiter + circuit breaker + budget tracking).
- **Paper source registry** -- registers enabled source clients (Semantic Scholar,
  OpenAlex, PubMed) with per-source rate limiting.
- **Ingestion gRPC client** -- connects to the Ingestion Service.
- **Outbox publisher** -- for transactional event publishing to Kafka.
- **Prometheus metrics** -- shared metrics instance across all activities.
- **Seven activity structs** registered with the Temporal worker:
  - `LLMActivities` -- keyword extraction and coverage assessment via LLM
  - `SearchActivities` -- concurrent paper searches across sources
  - `StatusActivities` -- database status updates, save keywords/papers, pause state
  - `IngestionActivities` -- download and ingest papers via the Ingestion Service
  - `EventActivities` -- publish outbox events
  - `EmbeddingActivities` -- generate embeddings for paper abstracts and relevance scoring
  - `DedupActivities` -- semantic deduplication against Qdrant vector store

Multiple worker replicas can run in parallel for horizontal scaling.

### 3. literature-review-migrate

**Entrypoint:** `cmd/migrate/main.go`

A CLI tool for database migration management using golang-migrate. Supports:

| Flag        | Description                                    |
|-------------|------------------------------------------------|
| `-up`       | Run all pending migrations                     |
| `-down`     | Roll back all migrations                       |
| `-steps N`  | Run N steps (positive = up, negative = down)   |
| `-version`  | Print current migration version and dirty flag |
| `-force V`  | Force-set version (recover from dirty state)   |
| `-path DIR` | Override the migrations directory               |

Only one action flag may be specified per invocation.

---

## Multi-Tenancy Model

Every operation in the service is scoped to a specific organization and project. The
`Tenant` struct carries `OrgID`, `ProjectID`, and `UserID`.

| Layer          | Enforcement                                                     |
|----------------|------------------------------------------------------------------|
| **gRPC**       | grpcauth interceptor validates JWT, extracts `org_id` claim      |
| **HTTP**       | Auth middleware extracts tenant from request context              |
| **Server**     | Handlers validate `org_id` + `project_id` match JWT claims       |
| **Repository** | All queries include `WHERE org_id = $1 AND project_id = $2`      |
| **Database**   | Composite indexes on `(org_id, project_id)` for efficient scoping|
| **Workflow**   | `ReviewWorkflowInput` carries `OrgID` + `ProjectID`              |
| **Activities** | Every activity input propagates tenant context                    |
| **Outbox**     | Events tagged with `org_id` + `project_id` for downstream routing|

---

## Service Dependencies

| Dependency                | Purpose                                                       |
|---------------------------|---------------------------------------------------------------|
| **PostgreSQL**            | Data persistence, LISTEN/NOTIFY for real-time progress updates, outbox event storage |
| **Temporal Server**       | Workflow orchestration, automatic retries, cancellation, progress queries, durable execution |
| **Kafka**                 | Event streaming via the transactional outbox pattern           |
| **Qdrant**                | Vector store for semantic deduplication and embedding storage  |
| **Ingestion Service**     | Paper PDF ingestion and full-text processing (gRPC at port 9095) |
| **LLM Provider (1 of 6)**| Keyword extraction from queries and paper abstracts            |
| **Embedding Provider**    | Vector embeddings for relevance gating and semantic dedup (OpenAI embeddings) |
| **Semantic Scholar API**  | Academic paper search (graph-based, citation-aware)            |
| **OpenAlex API**          | Academic paper search (open bibliographic data)                |
| **PubMed E-utilities API**| Academic paper search (biomedical focus, MeSH terms)           |

---

## Workflow Pipeline

The `LiteratureReviewWorkflow` is the central orchestration unit. It is a deterministic
Temporal workflow that uses a concurrent two-phase pipeline architecture with parent-child
workflow coordination. The workflow proceeds through six phases:

```
Phase 1: Extract Keywords (status: extracting_keywords)
  Input:  Natural language title + description, optional seed keywords
  Action: LLM extracts 3-10 searchable academic keywords
          Optionally embeds query text for relevance gating
  Output: Keyword list saved to keywords + request_keyword_mappings
  Event:  review.started published to outbox

Phase 2: Search Papers (status: searching)
  Input:  Keywords from Phase 1
  Action: For each keyword, search all enabled sources concurrently
          with rate limiting (500ms stagger between sources)
  Output: Papers saved to papers + request_paper_mappings (deduplicated by canonical_id)
  Note:   Individual keyword search failures are non-fatal

Phase 3: Batch Processing (status: ingesting)
  Input:  All discovered papers, batched into groups of 5
  Action: Spawn PaperProcessingWorkflow child workflows per batch:
          Stage 1: Embed paper abstracts (EmbeddingActivities)
          Stage 2: Semantic dedup against Qdrant vector store (DedupActivities)
          Stage 3: Download & ingest non-duplicate papers with PDF URLs (IngestionActivities)
          Stage 4: Update paper records in database (StatusActivities)
  Output: Child workflows signal parent with BatchCompleteSignal on completion
  Note:   Dedup failure is non-fatal (continues with all papers).
          Embedding failure is fatal for the batch.

Phase 4: Expand Search (status: expanding, 0-N rounds)
  Input:  Top 5 papers with abstracts from the current result set
  Action: LLM extracts new keywords from abstracts (passing existing keywords to avoid dupes)
          Relevance Gate: if enabled, filters drifting keywords via embedding similarity
          Searches all sources with new keywords, spawns batch processing child workflows
  Output: Additional papers discovered, processed, and appended to result set
  Guard:  Stops if no new keywords, paper limit reached, no papers have abstracts,
          or all expansion keywords filtered by relevance gate

Phase 5: Coverage Review (status: reviewing, optional)
  Input:  Up to 20 papers with abstracts, all keywords, original query
  Action: LLM assesses corpus coverage against original research intent
          Returns: coverage_score (0.0-1.0), reasoning, gap_topics
  Output: If coverage < threshold and gap topics identified:
          Auto-triggers gap expansion round (searches gap topics, processes via child workflows)
  Note:   Non-fatal -- workflow completes without score on assessment failure

Phase 6: Complete (status: completed)
  Action: Update review status to completed
  Event:  review.completed published to outbox with final statistics
  Return: ReviewWorkflowResult (keywords, papers, ingested, duplicates,
          failed, rounds, coverage score, gap topics, duration)
```

### Signals and Queries

| Type   | Name        | Direction     | Purpose                                                        |
|--------|-------------|---------------|----------------------------------------------------------------|
| Signal | `cancel`    | Client -> WF  | Cancels the running workflow via `workflow.WithCancel` context  |
| Signal | `pause`     | Client -> WF  | Pauses the workflow at the next checkpoint (user or budget)    |
| Signal | `resume`    | Client -> WF  | Resumes a paused workflow                                      |
| Signal | `stop`      | Client -> WF  | Graceful stop with partial results at next checkpoint          |
| Query  | `progress`  | Client -> WF  | Returns real-time `workflowProgress` (status, phase, counters) |

**Parent-Child Signals:**

| Signal               | Direction       | Purpose                                           |
|----------------------|-----------------|---------------------------------------------------|
| `batch_complete`     | Child -> Parent | Reports batch processing results (counts, errors) |
| `claim_papers`       | Child -> Parent | Claims papers for processing (dedup coordination) |
| `register_embeddings`| Child -> Parent | Registers paper embeddings with parent            |

### Activity Timeout Configuration

| Activity     | Start-to-Close | Heartbeat | Max Attempts | Backoff                     |
|--------------|----------------|-----------|--------------|------------------------------|
| LLM          | 2 minutes      | --        | 3            | 1s initial, 2x, max 30s     |
| Search       | 5 minutes      | --        | 3            | 2s initial, 2x, max 1m      |
| Status (DB)  | 30 seconds     | --        | 5            | 500ms initial, 2x, max 10s  |
| Ingestion    | 5 minutes      | 2 min     | 5            | 2s initial, 2x, max 1m      |
| Events       | 30 seconds     | --        | 5            | 500ms initial, 2x, max 10s  |
| Embedding    | 2 minutes      | 1 min     | 5            | 1s initial, 2x, max 30s     |
| Dedup        | 1 minute       | 30s       | 3            | 500ms initial, 2x, max 10s  |

### Workflow Constants

| Constant                    | Value | Description                              |
|-----------------------------|-------|------------------------------------------|
| Batch size                  | 5     | Papers per child workflow batch           |
| Max papers for expansion    | 5     | Papers selected per round for keyword extraction |
| Default max keywords/round  | 10    | Maximum keywords extracted per LLM call  |
| Default min keywords (query)| 3     | Minimum keywords from initial query      |
| Default max papers          | 100   | Maximum total papers per review          |
| Search rate limit delay     | 500ms | Stagger between source searches          |
| Workflow execution timeout  | 4 hr  | Maximum workflow running time             |
| Stream max duration         | 4 hr  | Maximum gRPC progress stream duration    |
| Stream poll interval        | 2 sec | How often progress stream polls the DB   |
| Ingestion poll interval     | 30s   | How often to poll ingestion status        |
| Ingestion max poll time     | 30 min| Maximum time to wait for ingestion        |

---

## gRPC API

Defined in `api/proto/literaturereview/v1/literature_review.proto`. The service exposes
the `LiteratureReviewService`:

| RPC                                | Type             | Description                                    |
|------------------------------------|------------------|------------------------------------------------|
| `StartLiteratureReview`            | Unary            | Start a new review from a natural language query |
| `GetLiteratureReviewStatus`        | Unary            | Get current status, progress, and configuration |
| `CancelLiteratureReview`           | Unary            | Cancel a running review (sends signal to Temporal) |
| `ListLiteratureReviews`            | Unary            | Paginated list with status and date filters     |
| `GetLiteratureReviewPapers`        | Unary            | Paginated papers with source/ingestion filters  |
| `GetLiteratureReviewKeywords`      | Unary            | Paginated keywords with round/source type filters|
| `StreamLiteratureReviewProgress`   | Server streaming | Stream real-time progress until terminal state  |

Every request requires `org_id` and `project_id`. JWT claims are validated by grpcauth
interceptors before handlers execute. Health checks (`grpc.health.v1.Health`) and
reflection endpoints are excluded from authentication.

### Key Message Types

- `StartLiteratureReviewRequest` -- query, optional keyword counts, expansion depth,
  source filters, date range
- `ReviewProgress` -- keyword counts, paper counts, per-source progress, elapsed/remaining time
- `LiteratureReviewProgressEvent` -- streaming event with oneof payload (keywords extracted,
  papers found, expansion started, ingestion progress, error)
- `Paper` -- full bibliographic metadata with all identifier types and ingestion status

---

## HTTP REST API

Built with Chi v5 router. All endpoints are prefixed under `/api/v1/`.

| Method   | Path                                              | Description                    |
|----------|---------------------------------------------------|--------------------------------|
| `POST`   | `/api/v1/literature-reviews`                      | Start a new review             |
| `GET`    | `/api/v1/literature-reviews`                      | List reviews (paginated)       |
| `GET`    | `/api/v1/literature-reviews/{reviewID}`           | Get review status              |
| `DELETE` | `/api/v1/literature-reviews/{reviewID}`           | Cancel a review                |
| `GET`    | `/api/v1/literature-reviews/{reviewID}/papers`    | List papers (paginated)        |
| `GET`    | `/api/v1/literature-reviews/{reviewID}/keywords`  | List keywords (paginated)      |
| `GET`    | `/api/v1/literature-reviews/{reviewID}/progress`  | SSE progress stream            |
| `GET`    | `/health`                                          | Health check                   |

Query parameters for filtering:

- `source` -- filter papers by source type
- `ingestion_status` -- filter papers by ingestion status
- `extraction_round` -- filter keywords by round number
- `source_type` -- filter keywords by source type (query, paper_keywords, llm_extraction)
- `page_size`, `page_token` -- pagination

---

## Database Schema

The database uses PostgreSQL with 8 sequential migrations across these groups:

### Extensions (Migration 1)

- `uuid-ossp` -- UUID generation (`uuid_generate_v4()`)
- `pg_trgm` -- Trigram indexes for fuzzy text search

### Enum Types (Migration 2)

| Enum               | Values                                                                              |
|--------------------|-------------------------------------------------------------------------------------|
| `review_status`    | pending, extracting_keywords, searching, expanding, ingesting, reviewing, completed, partial, failed, cancelled, paused |
| `search_status`    | pending, in_progress, completed, failed, rate_limited                               |
| `ingestion_status` | pending, submitted, processing, completed, failed, skipped                          |
| `identifier_type`  | doi, arxiv_id, pubmed_id, semantic_scholar_id, openalex_id, scopus_id              |
| `mapping_type`     | author_keyword, mesh_term, extracted, query_match                                   |
| `source_type`      | semantic_scholar, openalex, scopus, pubmed, biorxiv, arxiv                         |
| `pause_reason`     | user, budget_exhausted                                                              |

### Core Tables (Migration 3)

| Table                    | Purpose                                                |
|--------------------------|--------------------------------------------------------|
| `keywords`               | Unique keywords with normalized form, trigram GIN index |
| `papers`                 | Central paper repository, deduplicated by `canonical_id`|
| `paper_identifiers`      | Maps papers to DOI, arXiv ID, PubMed ID, etc.          |
| `paper_sources`          | Tracks which APIs provided data for each paper          |
| `keyword_searches`       | Search audit log with idempotency hash                  |
| `keyword_paper_mappings` | Links keywords to papers with provenance + confidence   |

### Tenant Tables (Migration 4)

| Table                       | Purpose                                              |
|-----------------------------|------------------------------------------------------|
| `literature_review_requests`| Multi-tenant review requests with config snapshot    |
| `request_keyword_mappings`  | Links requests to keywords by extraction round       |
| `request_paper_mappings`    | Links requests to papers with ingestion tracking     |
| `review_progress_events`    | Real-time progress events for UI streaming           |

### Outbox (Migration 5)

| Table          | Purpose                                               |
|----------------|-------------------------------------------------------|
| `outbox_events`| Transactional outbox for Kafka event publishing       |

### Functions and Triggers (Migrations 6-7)

- Automatic `updated_at` timestamp updates
- NOTIFY triggers for real-time progress push

### Performance Indexes (Migration 8)

- Composite and partial indexes for common query patterns

### Indexing Strategy

- **Multi-tenancy:** Composite index on `(org_id, project_id)` for efficient tenant-scoped queries.
- **Deduplication:** Unique constraints on `papers.canonical_id`, `keywords.normalized_keyword`,
  and `keyword_searches.search_window_hash`.
- **Fuzzy search:** `pg_trgm` GIN indexes on `keywords.keyword`, `papers.title`, and `papers.abstract`.
- **Partial indexes:** Conditional indexes for active states (e.g., `ingestion_status = 'pending'`,
  `keywords_extracted = FALSE`).
- **Sort optimization:** Descending indexes on `citation_count` and `created_at`.

---

## Paper Source Clients

Each paper source implements the `PaperSource` interface and is registered with the
`papersources.Registry` during worker startup. Sources are searched concurrently per
keyword via the `SearchActivities.SearchPapers` activity.

| Source           | API Base URL                                  | Default Rate Limit | Max Results |
|------------------|-----------------------------------------------|--------------------|-------------|
| Semantic Scholar | `https://api.semanticscholar.org/graph/v1`    | 10 req/s           | 100         |
| OpenAlex         | `https://api.openalex.org`                    | 10 req/s           | 200         |
| PubMed           | `https://eutils.ncbi.nlm.nih.gov/entrez/eutils`| 3 req/s          | 100         |

Additional sources configured in `config.yaml` but not yet implemented as Go clients:
Scopus, bioRxiv, arXiv.

Each client includes:

- **Per-source rate limiting** via `golang.org/x/time/rate` token bucket.
- **Configurable HTTP timeouts** (default 30 seconds per source).
- **Response mapping** from source-specific types to `domain.Paper`.
- **Canonical ID generation** for cross-source deduplication
  (priority: DOI > arXiv > PubMed > Semantic Scholar > OpenAlex > Scopus).

---

## LLM Integration

Keyword extraction uses the shared `github.com/helixir/llm` package, which provides a
unified `Client` interface across six providers.

### Supported Providers

| Provider       | Config Section      | Auth Mechanism                    |
|----------------|---------------------|-----------------------------------|
| OpenAI         | `llm.openai`        | `LITREVIEW_LLM_OPENAI_API_KEY`    |
| Anthropic      | `llm.anthropic`     | `LITREVIEW_LLM_ANTHROPIC_API_KEY` |
| Azure OpenAI   | `llm.azure`         | `LITREVIEW_LLM_AZURE_API_KEY`     |
| AWS Bedrock    | `llm.bedrock`       | AWS credential chain / IAM role   |
| Google Gemini  | `llm.gemini`        | `LITREVIEW_LLM_GEMINI_API_KEY`    |
| Vertex AI      | `llm.gemini`        | GCP project + location (ADC)      |

### Extraction Modes

The `KeywordExtractor` interface wraps the shared LLM client with domain-specific prompt
engineering. Two extraction modes are supported:

- **Query mode** (`ExtractionModeQuery`): Extracts keywords from a user's natural language
  research query, focusing on core research topics, methodologies, and domain-specific
  terms. Targets between `min_keywords` (3) and `max_keywords_per_round` (default 10).
- **Abstract mode** (`ExtractionModeAbstract`): Extracts keywords from paper abstracts
  during expansion rounds. Receives existing keywords to avoid duplicates and the
  original query as domain context.

The LLM is instructed to return JSON with a `keywords` array and optional `reasoning`.
The prompt includes guidelines for extracting specific academic terms, including
synonyms, MeSH-style terms, and both abbreviated and expanded forms.

### Resilience Middleware

When `llm.resilience.enabled` is set to `true`, the shared LLM package wraps the provider
with a `ResilientClient` middleware stack:

1. **Circuit breaker** -- Opens after consecutive failures (`cb_consecutive_threshold: 5`)
   or high failure rate (`cb_failure_rate_threshold: 0.5`). Probes in half-open state
   after cooldown (`cb_cooldown_sec: 30`).
2. **Budget tracker** -- Lease-based cost tracking via `BudgetLease` interface. The
   `OutboxBudgetReporter` writes usage reports to the outbox table.
3. **Adaptive rate limiter** -- Reservation-based token bucket (`rate_limit_rps: 10`,
   `rate_limit_burst: 20`). Backs off automatically on rate-limit responses and recovers
   over `rate_limit_recovery_sec: 60`.

---

## Outbox Events

The service publishes lifecycle events via the transactional outbox pattern (shared
`github.com/helixir/outbox` package). Events are written to the `outbox_events` table
within the same database transaction as the state change, then asynchronously relayed
to Kafka by the outbox publisher.

### Event Types

| Event Type                     | Trigger                                        |
|--------------------------------|------------------------------------------------|
| `review.started`               | Workflow begins keyword extraction              |
| `review.completed`             | Workflow finishes successfully                  |
| `review.failed`                | Workflow encounters unrecoverable error         |
| `review.stopped`               | Workflow stopped gracefully with partial results|
| `review.cancelled`             | User or system cancels the review              |
| `review.keywords_extracted`    | LLM extracts keywords from query or paper      |
| `review.papers_discovered`     | New papers found from source search            |
| `review.search_completed`      | Keyword search against a source finishes       |
| `review.ingestion_started`     | Paper ingestion batch submitted                |
| `review.ingestion_completed`   | Paper ingestion batch finished                 |
| `review.progress_updated`      | Periodic progress snapshot                     |

### Kafka Configuration

| Setting        | Default Value                               |
|----------------|---------------------------------------------|
| Topic          | `events.outbox.literature_review_service`   |
| Brokers        | `localhost:9092`                            |
| Batch size     | 100 messages                                |
| Batch timeout  | 10ms                                        |

### Outbox Relay Configuration

| Setting         | Default Value |
|-----------------|---------------|
| Poll interval   | 1 second      |
| Batch size      | 100 events    |
| Workers         | 4 concurrent  |
| Max retries     | 5 per event   |
| Lease duration  | 30 seconds    |

---

## Observability

### Prometheus Metrics (Port 9091)

All metrics use the `literature_review` namespace prefix. Key metrics organized by
subsystem:

| Subsystem    | Metrics                                                                               |
|--------------|---------------------------------------------------------------------------------------|
| Reviews      | `reviews_started_total`, `reviews_completed_total`, `reviews_failed_total`, `reviews_cancelled_total`, `review_duration_seconds` |
| Keywords     | `keywords_extracted_total`, `keyword_extractions_total{source}`, `keywords_per_review` |
| Searches     | `searches_started_total{source}`, `searches_completed_total{source}`, `searches_failed_total{source}`, `search_duration_seconds{source}`, `papers_per_search{source}` |
| Papers       | `papers_discovered_total`, `papers_ingested_total`, `papers_skipped_total`, `papers_duplicate_total`, `papers_by_source_total{source}` |
| Source HTTP   | `source_requests_total{source,endpoint}`, `source_requests_failed_total{source,endpoint,error_type}`, `source_request_duration_seconds{source,endpoint}`, `source_rate_limited_total{source}` |
| Ingestion    | `ingestion_requests_started_total`, `ingestion_requests_completed_total`, `ingestion_requests_failed_total` |
| LLM          | `llm_requests_total{operation,model}`, `llm_requests_failed_total{operation,model,error_type}`, `llm_request_duration_seconds{operation,model}`, `llm_tokens_used_total{operation,model,token_type}` |

### Structured Logging

All logging uses zerolog with structured JSON output. Log fields include:

- `component` -- `server`, `worker`, or `migrate`
- `org_id`, `project_id` -- tenant context (where applicable)
- `request_id` -- review request identifier
- `keyword`, `source` -- contextual fields per operation

Configuration:

```yaml
logging:
  level: info          # trace, debug, info, warn, error, fatal, panic
  format: json         # json or console
  output: stdout       # stdout, stderr, or file path
  add_source: false    # include source file:line in output
  time_format: "2006-01-02T15:04:05Z07:00"
```

### Distributed Tracing

OpenTelemetry tracing support (disabled by default):

```yaml
tracing:
  enabled: true
  endpoint: "otel-collector:4317"
  service_name: literature-review-service
  sample_rate: 0.1
```

### Temporal Logger Adapter

The `TemporalLogger` bridges zerolog to the Temporal SDK's logging interface, ensuring
workflow and activity logs appear in the same structured format as application logs.

---

## Configuration

Configuration is loaded via Viper with a layered approach:

1. **Base config:** `config/config.yaml` (non-secret defaults, checked into source control)
2. **Environment overrides:** Any config value can be overridden with
   `LITREVIEW_<SECTION>_<KEY>` environment variables
3. **Secrets:** API keys and passwords must be set via environment variables only

### Configuration Sections

| Section              | Env Prefix                        | Purpose                                |
|----------------------|-----------------------------------|----------------------------------------|
| `server`             | `LITREVIEW_SERVER_`               | Bind address, ports, timeouts          |
| `database`           | `LITREVIEW_DATABASE_`             | PostgreSQL connection, pool, SSL, migrations |
| `temporal`           | `LITREVIEW_TEMPORAL_`             | Server address, namespace, task queue  |
| `logging`            | `LITREVIEW_LOGGING_`              | Log level, format, output              |
| `metrics`            | `LITREVIEW_METRICS_`              | Enable/disable, endpoint path          |
| `tracing`            | `LITREVIEW_TRACING_`              | OTLP endpoint, sampling rate           |
| `llm`                | `LITREVIEW_LLM_`                  | Provider, model, timeouts, resilience  |
| `kafka`              | `LITREVIEW_KAFKA_`                | Brokers, topic, batching               |
| `outbox`             | `LITREVIEW_OUTBOX_`               | Poll interval, batch size, workers     |
| `paper_sources`      | `LITREVIEW_PAPER_SOURCES_`        | Per-source enable, URLs, rate limits   |
| `ingestion_service`  | `LITREVIEW_INGESTION_SERVICE_`    | gRPC address, timeout                  |

### Environment Variable Examples

```bash
# Database
LITREVIEW_DATABASE_HOST=db.prod.internal
LITREVIEW_DATABASE_PASSWORD=secret
LITREVIEW_DATABASE_SSL_MODE=verify-full
LITREVIEW_DATABASE_MAX_CONNS=50

# LLM
LITREVIEW_LLM_PROVIDER=openai
LITREVIEW_LLM_OPENAI_API_KEY=sk-...
LITREVIEW_LLM_TIMEOUT=120s

# Paper Sources
LITREVIEW_PAPER_SOURCES_SEMANTIC_SCHOLAR_API_KEY=...
LITREVIEW_PAPER_SOURCES_PUBMED_API_KEY=...

# Temporal
LITREVIEW_TEMPORAL_HOST_PORT=temporal.prod.internal:7233
LITREVIEW_TEMPORAL_NAMESPACE=literature-review
```

---

## Deployment

### Docker

Multi-stage Dockerfile producing a minimal image. Build context is the monorepo root
to resolve `replace` directives for sibling modules (`grpcauth`, `outbox`,
`ingestion_service`, `llm`).

```bash
make docker-build        # Build from monorepo root
make docker-run-server   # Run server container with dev config
make docker-run-worker   # Run worker container (override entrypoint)
```

The image contains all three binaries. The default entrypoint is the server; override
with `--entrypoint` for worker or migrate.

### Docker Compose

```bash
make compose-up          # Start infrastructure (postgres, temporal, kafka)
make compose-up-full     # Start all services including server and worker
make compose-down        # Stop everything
make compose-logs        # Tail logs
```

### Kubernetes / Helm

A Helm chart is provided at `deploy/helm/literature-review-service/` with:

- Separate Deployment resources for server and worker
- Horizontal Pod Autoscaler (HPA) for both server and worker
- PodDisruptionBudget (PDB) for high availability
- ServiceMonitor for Prometheus Operator scraping
- NetworkPolicy restricting ingress/egress
- ExternalSecret integration for secret management
- ConfigMap for `config.yaml`
- Kafka topic provisioning Job
- Environment-specific values files (`values-staging.yaml`, `values-production.yaml`)

---

## Testing

### Test Commands

```bash
make test                  # Unit tests with race detector and coverage
make test-race             # Race detector, 3 passes
make test-integration      # Integration tests with Docker Compose / testcontainers
make test-e2e              # End-to-end tests (full stack)
make test-chaos            # Chaos engineering tests
make test-security         # SQL injection, XSS, LIKE pattern injection, fuzz
make test-fuzz             # Fuzz testing (30s fuzz time)
make test-load             # k6 load tests (review_lifecycle.js, list_reviews.js)
make test-all              # Unit + race + chaos + security
```

### Test Categories

| Category    | Location                                   | Description                                          |
|-------------|--------------------------------------------|------------------------------------------------------|
| Unit        | `internal/*/..._test.go`                   | Per-package tests using testify and pgxmock          |
| Repository  | `internal/repository/*_test.go`            | Database layer tests with pgxmock                    |
| Workflow    | `internal/temporal/workflows/*_test.go`    | Temporal test suite for workflow logic                |
| Activity    | `internal/temporal/activities/*_test.go`   | Activity tests with mocked dependencies              |
| Replay      | `internal/temporal/workflows/replay_test.go`| Workflow determinism verification                   |
| HTTP        | `internal/server/http/*_test.go`           | HTTP handler, middleware, and security tests          |
| Integration | `tests/integration/`                       | Real PostgreSQL (testcontainers) and Temporal tests  |
| E2E         | `tests/e2e/`                               | Full stack end-to-end tests                          |
| Chaos       | `tests/chaos/`                             | Resilience and failure injection tests               |
| Security    | `tests/security/`                          | Fuzz tests and injection tests                       |
| Load        | `tests/loadtest/`                          | k6 JavaScript load test scripts                      |

### Testing Tools

| Tool                  | Purpose                                         |
|-----------------------|-------------------------------------------------|
| testify               | Assertions and mocking (`assert`, `require`)    |
| pgxmock               | PostgreSQL mock for repository unit tests       |
| Temporal test env     | In-memory workflow execution for deterministic testing |
| testcontainers        | Disposable Docker containers for integration tests |
| k6                    | Load testing with scripted scenarios             |

---

## Domain Model

### Core Entities

**LiteratureReviewRequest** -- The top-level aggregate. Represents a user's literature
review session, tracking the original query, Temporal workflow reference, configuration
snapshot (stored as JSONB), progress counters, and lifecycle timestamps. Scoped by
`org_id`, `project_id`, and `user_id`.

**Paper** -- An academic paper in the central repository. Deduplicated globally by
`canonical_id` (format: `prefix:value`, e.g., `doi:10.1234/example`). Contains
bibliographic metadata (title, abstract, authors as JSONB, journal, venue, citation
count, reference count), a `pdf_url` for ingestion, and `raw_metadata` preserving
the original source API response.

**Keyword** -- A normalized search term. Stored once globally with both the original
form and a lowercased, trimmed `normalized_keyword` for deduplication. Trigram GIN
indexes enable fuzzy matching.

### Mapping Entities

**RequestKeywordMapping** -- Links a review request to a keyword. Records which
extraction round produced the keyword (0 = initial query, 1+ = expansion), the source
type (`query`, `paper_keywords`, `llm_extraction`), and optionally the source paper.

**RequestPaperMapping** -- Links a review request to a discovered paper. Tracks
discovery provenance (keyword, source, expansion depth) and ingestion lifecycle
(`pending` -> `submitted` -> `processing` -> `completed`/`failed`/`skipped`).

**PaperIdentifier** -- Maps a paper to its external identifiers across sources (DOI,
arXiv ID, PubMed ID, Semantic Scholar ID, OpenAlex ID, Scopus ID).

**PaperSource** -- Records which API provided data for a paper, with source-specific
metadata (relevance scores, result rankings) as JSONB.

**KeywordPaperMapping** -- Links keywords to papers with provenance type
(author_keyword, mesh_term, extracted, query_match) and an optional confidence score.

**KeywordSearch** -- Audit log for search operations. Uses a `search_window_hash`
(SHA-256 of keyword + source + date range) for idempotency.

---

## Review Status Lifecycle

```
                                +-----------+
                                |  pending  |
                                +-----+-----+
                                      |
                         +------------v-------------+
                         | extracting_keywords      |<----+
                         +------------+-------------+     |
                                      |                    |
                           +----------v----------+         |
                           |     searching       |<---+    |
                           +----------+----------+    |    |
                                      |               |    |  resume
                           +----------v----------+    |    |  signal
                           |     ingesting       |<-+ |    |
                           +----------+----------+  | |    |
                                      |             | |    |
                      +---------------v-----------+ | |    |
                      |  expanding (rounds 1..N)  |-+ |    |
                      +---------------+-----------+   |    |
                      |               |               |    |
                      |  (more rounds)|---------------+    |
                      |               |                    |
                      +-------v-------+                    |
                      |   reviewing   |                    |
                      |   (optional)  |                    |
                      +---+---+---+---+                    |
                          |   |   |                        |
       +-----+   gap     |   |   |                        |
       |     |  expansion |   |   |                        |
       | expanding <------+   |   |                        |
       |     |                |   |                        |
       +-----+                |   |       +-----------+    |
                              |   |       |  paused   |----+
              +-------+-------+---+---+   +-----------+
              |       |               |     ^         ^
        +-----v-+  +-v------+  +-----v--+  |         |
        |completed| |partial |  |failed  |  | pause   | budget
        +---------+ +--------+  +--------+  | signal  | exhausted
                                            |         |
        +-----------+              (from any non-terminal state)
        | cancelled |
        +-----------+
```

**11 Review States:**
- **Active states:** `pending`, `extracting_keywords`, `searching`, `ingesting`, `expanding`, `reviewing`
- **Pausable state:** `paused` (reachable from any non-terminal state via pause signal or budget exhaustion)
- **Terminal states:** `completed`, `partial`, `failed`, `cancelled`

**Pause reasons:** `user` (manual pause), `budget_exhausted` (automatic when LLM budget runs out)

---

## Dependencies and Shared Packages

### Helixir Shared Packages

These are sibling modules in the monorepo, referenced via `replace` directives in
`go.mod`:

| Package            | Import Path                          | Purpose                                    |
|--------------------|--------------------------------------|--------------------------------------------|
| grpcauth           | `github.com/helixir/grpcauth`        | JWT auth interceptors, env-based config    |
| outbox             | `github.com/helixir/outbox`          | Transactional outbox (PostgreSQL + Kafka)  |
| llm                | `github.com/helixir/llm`             | Multi-provider LLM client + resilience     |
| ingestion-service  | `github.com/helixir/ingestion-service`| Generated gRPC client for ingestion       |

### Key External Dependencies

| Dependency                            | Version  | Purpose                            |
|---------------------------------------|----------|------------------------------------|
| `go.temporal.io/sdk`                  | v1.39.0  | Temporal workflow SDK              |
| `github.com/jackc/pgx/v5`            | v5.8.0   | PostgreSQL driver + connection pool|
| `github.com/go-chi/chi/v5`           | v5.2.4   | HTTP router                        |
| `github.com/rs/zerolog`              | v1.34.0  | Structured JSON logging            |
| `github.com/spf13/viper`             | v1.21.0  | Configuration management           |
| `github.com/prometheus/client_golang` | v1.23.2  | Prometheus metrics                 |
| `github.com/segmentio/kafka-go`      | v0.4.50  | Kafka client                       |
| `github.com/golang-migrate/migrate/v4`| v4.19.1 | Database migrations                |
| `github.com/go-playground/validator/v10`| v10.30.1| Struct validation                |
| `github.com/google/uuid`             | v1.6.0   | UUID generation                    |
| `google.golang.org/grpc`             | v1.78.0  | gRPC framework                     |
| `google.golang.org/protobuf`         | v1.36.11 | Protocol Buffers runtime           |
| `golang.org/x/time`                  | v0.14.0  | Rate limiting (token bucket)       |
| `github.com/stretchr/testify`        | v1.11.1  | Test assertions                    |
| `github.com/pashagolub/pgxmock/v4`   | v4.9.0   | PostgreSQL mock for testing        |
| `github.com/testcontainers/testcontainers-go` | v0.40.0 | Docker containers for tests |

### Build Tooling

| Tool             | Purpose                                |
|------------------|----------------------------------------|
| `buf`            | Protobuf linting and code generation   |
| `golangci-lint`  | Go linter aggregator                   |
| `golang-migrate` | Migration file creation CLI            |
| `k6`             | Load testing                           |
