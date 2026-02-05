# Literature Review Service -- Architecture

## Table of Contents

1. [Overview](#overview)
2. [System Context](#system-context)
3. [Layered Architecture](#layered-architecture)
4. [Request Flow](#request-flow)
5. [Temporal Workflow Design](#temporal-workflow-design)
6. [LLM Integration](#llm-integration)
7. [Paper Source Architecture](#paper-source-architecture)
8. [Database Design](#database-design)
9. [Multi-Tenancy](#multi-tenancy)
10. [Error Handling](#error-handling)
11. [Observability](#observability)
12. [Configuration](#configuration)
13. [Binaries](#binaries)
14. [Testing Strategy](#testing-strategy)
15. [Shared Package Dependencies](#shared-package-dependencies)
16. [Key Design Decisions](#key-design-decisions)

---

## Overview

The Literature Review Service is a Go microservice in the Helixir monorepo that
automates systematic academic literature reviews. Given a natural language
research query, the service:

1. Extracts search keywords using a large language model (LLM).
2. Searches multiple academic databases concurrently (Semantic Scholar, OpenAlex,
   PubMed).
3. Recursively expands searches by extracting new keywords from discovered paper
   abstracts.
4. Submits papers with PDF URLs to the Ingestion Service for full-text processing.
5. Publishes lifecycle events to Kafka via the transactional outbox pattern.

The service exposes both a gRPC API and an HTTP REST API, uses Temporal for
durable workflow orchestration, PostgreSQL for persistence, and Prometheus for
metrics collection.

---

## System Context

```
                                +---------------------+
                                |   Client / UI       |
                                +----------+----------+
                                           |
                          gRPC (protobuf) / HTTP REST (JSON)
                                           |
                                +----------v----------+
                                | Literature Review   |
                                |     Service         |
                                +--+-----+-----+-----+
                                   |     |     |
                   +---------------+     |     +----------------+
                   |                     |                      |
          +--------v--------+  +---------v---------+  +---------v--------+
          |  Temporal Server |  | PostgreSQL        |  | Ingestion Service|
          |  (Workflows)     |  | (State + Outbox)  |  | (gRPC)           |
          +---------+--------+  +---------+---------+  +------------------+
                    |                     |
                    |             +-------v--------+
                    |             | Outbox Relay   |
                    |             +-------+--------+
                    |                     |
          +---------v--------+   +--------v--------+
          | LLM Providers    |   | Apache Kafka    |
          | (OpenAI,         |   | (Events)        |
          |  Anthropic, ...) |   +---------+-------+
          +------------------+             |
                                  +--------v--------+
          +------------------+    | Observability   |
          | Paper Source APIs |    | Service         |
          | (S2, OA, PubMed) |    +-----------------+
          +------------------+
```

The service has three binaries (`cmd/server/`, `cmd/worker/`, `cmd/migrate/`),
connects to Temporal for durable workflow execution, PostgreSQL for state and
the transactional outbox, external LLM providers for keyword extraction, three
academic paper source APIs, and the Ingestion Service for PDF processing.

---

## Layered Architecture

The service follows a strict layered architecture where dependencies flow
inward. The domain layer has zero infrastructure dependencies. Each outer layer
depends only on inner layers, never on peer layers at the same level.

```
+------------------------------------------------------------------+
|                  cmd/server  cmd/worker  cmd/migrate              |
|                    (entrypoints -- wire everything)               |
+------+-------------------+-------------------+-------------------+
       |                   |                   |
       v                   v                   v
+-------------+  +------------------+  +----------------+
| server/     |  | temporal/        |  | database/      |
| server/http |  |   activities/    |  |   migrator     |
|             |  |   workflows/     |  +-------+--------+
+------+------+  +--------+---------+          |
       |                   |                   |
       |    +--------------+------+            |
       |    |              |      |            |
       v    v              v      v            v
+----------+ +--------+ +------+ +----------+ +----------+
|repository| |  llm/  | |outbox| |papersrcs | |database/ |
+-----+----+ +----+---+ +--+---+ +----+-----+ +-----+----+
      |           |         |         |              |
      v           v         v         v              v
+------------------------------------------------------------------+
|                        domain/                                    |
|           (models, enums, errors, events -- zero deps)            |
+------------------------------------------------------------------+
```

### 1. Transport Layer -- `server/`, `server/http/`

**gRPC server** (`internal/server/`):
- Implements `LiteratureReviewServiceServer` from generated protobuf code.
- 7 RPC methods: `StartLiteratureReview`, `GetLiteratureReviewStatus`,
  `CancelLiteratureReview`, `ListLiteratureReviews`, `GetLiteratureReviewPapers`,
  `GetLiteratureReviewKeywords`, `StreamLiteratureReviewProgress`.
- Bidirectional conversion between protobuf messages and domain types (`convert.go`).
- `domainErrToGRPC()` maps domain errors to gRPC status codes.
- `validateTenantAccess()` checks JWT claims from `grpcauth` interceptor.
- Server streaming for real-time progress via `StreamLiteratureReviewProgress`.
- gRPC keepalive: 15-min idle, 30-min max age, 5-min grace period.
- Max message size: 16 MB send/receive. Max concurrent streams: 100.

**HTTP REST server** (`internal/server/http/`):
- Built on `chi/v5` router with middleware chain:
  1. `middleware.RequestID` -- assigns unique request ID.
  2. `middleware.RealIP` -- extracts client IP from proxy headers.
  3. `middleware.Recoverer` -- catches panics, returns 500.
  4. `correlationIDMiddleware` -- propagates or generates X-Correlation-ID.
  5. `jsonContentTypeMiddleware` -- sets Content-Type: application/json.
  6. Auth middleware (injected, validates JWT).
  7. `tenantContextMiddleware` -- extracts orgID/projectID from URL path.
- JSON request/response types defined in `response.go`.
- `writeDomainError()` maps domain errors to HTTP status codes.
- SSE (Server-Sent Events) endpoint for progress streaming with 5-min write timeout.
- Request body size limit: 1 MB via `io.LimitReader`.
- Query length validation: 3-10000 characters.

**Proto definition** (`api/proto/literaturereview/v1/`):
- Enums: `ReviewStatus` (9 values), `IngestionStatus` (6 values),
  `KeywordSourceType`, `SortOrder`.
- Streaming RPC: `StreamLiteratureReviewProgress` returns
  `stream LiteratureReviewProgressEvent`.
- Event data uses `oneof` for type-safe progress event variants:
  `KeywordsExtractedEvent`, `PapersFoundEvent`, `ExpansionStartedEvent`,
  `IngestionProgressEvent`, `ErrorEvent`.

**HTTP route structure:**
```
GET    /healthz
GET    /readyz
POST   /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews
GET    /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews
GET    /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews/{reviewID}
DELETE /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews/{reviewID}
GET    /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews/{reviewID}/papers
GET    /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews/{reviewID}/keywords
GET    /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews/{reviewID}/progress
```

### 2. Workflow Layer -- `temporal/workflows/`

Contains the `LiteratureReviewWorkflow` function -- a deterministic orchestration
function that Temporal replays on failure recovery. This layer is pure
orchestration logic with no side effects; all I/O is delegated to activities.

Key design rules for Temporal determinism:
- Uses `workflow.Now(ctx)` instead of `time.Now()`.
- Map iteration order is controlled via `SortedMapKeys()` helper.
- String slices are sorted via `SortedStringSlice()` before use.
- Deduplication via `DeduplicateStrings()` produces deterministic output.
- No direct I/O, HTTP calls, or database access.

### 3. Activity Layer -- `temporal/activities/`

Business logic execution units that run as Temporal activities with automatic
retry. Each activity struct groups related operations and is registered with
the Temporal worker.

| Activity Struct        | Responsibility                                            |
|------------------------|-----------------------------------------------------------|
| `LLMActivities`       | Keyword extraction via LLM, budget usage reporting        |
| `SearchActivities`    | Concurrent paper search across sources via registry       |
| `StatusActivities`    | Review status updates, keyword/paper persistence, counter increments |
| `IngestionActivities` | Paper submission to ingestion service (single + batch)    |
| `EventActivities`     | Outbox event publishing for Kafka relay                   |

All activity structs accept interfaces (not concrete types) for their
dependencies, enabling straightforward mock-based testing:
- `LLMActivities` depends on `llm.KeywordExtractor` and optional `BudgetUsageReporter`
- `SearchActivities` depends on `PaperSearcher` (subset of Registry)
- `StatusActivities` depends on `ReviewRepository`, `KeywordRepository`, `PaperRepository`
- `IngestionActivities` depends on `IngestionClient`
- `EventActivities` depends on `EventPublisher`

### 4. Domain Layer -- `domain/`

Core models, enums, errors, and business rules with zero external dependencies.

**Models**: `LiteratureReviewRequest`, `ReviewConfiguration`, `Paper`, `Author`,
`Keyword`, `KeywordSearch`, `KeywordPaperMapping`, `RequestKeywordMapping`,
`RequestPaperMapping`, `PaperIdentifier`, `PaperSource`, `Tenant`,
`ReviewProgress`, `ReviewProgressEvent`, `SourceProgress`, `OutboxEvent`

**Enums** (string-typed constants matching PostgreSQL enum types):

| Enum              | Count | Values                                                                |
|-------------------|-------|-----------------------------------------------------------------------|
| `ReviewStatus`    | 9     | pending, extracting_keywords, searching, expanding, ingesting, completed, partial, failed, cancelled |
| `SearchStatus`    | 5     | pending, in_progress, completed, failed, rate_limited                 |
| `IngestionStatus` | 6     | pending, submitted, processing, completed, failed, skipped            |
| `SourceType`      | 6     | semantic_scholar, openalex, scopus, pubmed, biorxiv, arxiv            |
| `IdentifierType`  | 6     | doi, arxiv_id, pubmed_id, semantic_scholar_id, openalex_id, scopus_id |
| `MappingType`     | 4     | author_keyword, mesh_term, extracted, query_match                     |

**Key Functions**:
- `GenerateCanonicalID(PaperIdentifiers)` -- deterministic paper deduplication
  with priority: DOI > ArXiv > PubMed > S2 > OpenAlex > Scopus.
  Format: `prefix:value` (e.g., `doi:10.1038/s41586-021-03819-2`).
- `NormalizeKeyword(string)` -- lowercase, trim, collapse whitespace.
- `ReviewStatus.IsTerminal()` -- true for completed/partial/failed/cancelled.
- `DefaultReviewConfiguration()` -- 100 max papers, depth 2, 10 keywords/round,
  3 default sources (Semantic Scholar, OpenAlex, PubMed).

**Events** (10 types published via transactional outbox):
```
review.started              review.cancelled
review.completed            review.keywords_extracted
review.failed               review.papers_discovered
review.search_completed     review.ingestion_started
review.ingestion_completed  review.progress_updated
```

### 5. Repository Layer -- `repository/`

PostgreSQL data access via the `pgx/v5` driver with connection pooling.

**Interfaces**:

| Interface          | Methods | Key Operations                                        |
|--------------------|---------|-------------------------------------------------------|
| `ReviewRepository` | 7       | Create, Get, Update (SELECT FOR UPDATE), UpdateStatus, List, IncrementCounters, GetByWorkflowID |
| `PaperRepository`  | 9       | Create, GetByCanonicalID, GetByID, FindByIdentifier, UpsertIdentifier, AddSource, List, MarkKeywordsExtracted, BulkUpsert |
| `KeywordRepository`| 12      | GetOrCreate, GetByID, GetByNormalized, BulkGetOrCreate, RecordSearch, GetLastSearch, NeedsSearch, ListSearches, AddPaperMapping, BulkAddPaperMappings, GetPapersForKeyword, List |

**Key patterns**:
- `DBTX` interface enables both pool and transaction contexts.
- `applyPaginationDefaults()` clamps limit to [1, 1000] with default 100.
- Optimistic locking via `SELECT FOR UPDATE` in `ReviewRepository.Update()`.
- Filter structs (`ReviewFilter`, `PaperFilter`, `KeywordFilter`) with optional
  pointer fields and validation.
- LIKE pattern injection prevented by escaping `%`, `_`, and `\`.
- Tests use `pgxmock/v4` for unit testing without a running database.

### 6. Infrastructure Layer -- `database/`, `config/`, `observability/`

Cross-cutting concerns that support all other layers.

**Database** (`internal/database/`):
- `DB` struct wraps `pgxpool.Pool` with health checks and advisory locks.
- Transaction helpers at multiple isolation levels: `WithTransaction` (default),
  `WithSerializableTransaction`, `WithRepeatableReadTransaction`,
  `WithReadOnlyTransaction`.
- Advisory lock methods: session-scoped (`AcquireAdvisoryLock`,
  `ReleaseAdvisoryLock`) and transaction-scoped (`AcquireAdvisoryLockTx`,
  `TryAcquireAdvisoryLockTx`).
- Connection pool logging hooks at trace level.
- `Migrator` for golang-migrate based schema management.

**Observability** (`internal/observability/`):
- `NewLogger()` creates per-instance zerolog loggers (avoids global level
  mutation via `zerolog.SetGlobalLevel`).
- `Metrics` struct with 28 Prometheus metrics across 7 subsystems.
- `TemporalLogger` adapter bridges zerolog to Temporal SDK's log interface.
- Context helpers for threading metadata: `WithRequestID`, `WithOrgProject`,
  `WithTraceSpan`, `WithWorkflow`.
- `ReviewContext` aggregates all context data for bulk propagation.

**Configuration** (`internal/config/`):
- Viper-based with YAML config file + environment variable overrides.
- Env prefix: `LITREVIEW_`.
- Secrets loaded from environment only (tagged `mapstructure:"-"`).

---

## Request Flow

Trace of a `StartLiteratureReview` request from client to completion:

```
Client
  |
  |  gRPC: StartLiteratureReview(org_id, project_id, query, config)
  |  HTTP:  POST /api/v1/orgs/{org}/projects/{project}/literature-reviews
  v
Transport Layer
  |  1. Validate org_id, project_id are non-empty
  |  2. Validate tenant access via grpcauth JWT claims
  |  3. Parse configuration, apply defaults via DefaultReviewConfiguration()
  |  4. Create LiteratureReviewRequest with status=pending
  |  5. Insert review record into PostgreSQL
  |  6. Build ReviewWorkflowInput{RequestID, OrgID, ProjectID, UserID, Query, Config}
  |  7. Call workflowClient.StartReviewWorkflow()
  |     -> workflowID = "review-{requestID}"
  |     -> timeout = 4 hours
  |  8. Update review record with temporal_workflow_id, temporal_run_id
  |  9. Return review_id + workflow_id to client
  v
Temporal Server
  |  Enqueues workflow task on configured task queue
  v
Worker (cmd/worker/)
  |  Picks up workflow task, dispatches to LiteratureReviewWorkflow
  v
Phase 1: Extract Keywords (status: extracting_keywords)
  |  Activity: LLMActivities.ExtractKeywords
  |    -> clientAdapter.ExtractKeywords()
  |    -> BuildExtractionPrompt(mode="query")
  |    -> sharedllm.Client.Complete() with JSON response format
  |    -> Parse {"keywords": [...], "reasoning": "..."}
  |    -> Record Prometheus metrics (latency, tokens)
  |    -> Report budget usage via outbox (if lease provided)
  |  Activity: StatusActivities.SaveKeywords
  |    -> KeywordRepository.BulkGetOrCreate()
  |  Activity (fire-and-forget): EventActivities.PublishEvent("review.started")
  v
Phase 2: Search Papers (status: searching)
  |  For each extracted keyword:
  |    Activity: SearchActivities.SearchPapers
  |      -> Registry.SearchSources(params, sourceTypes)
  |      -> Concurrent goroutines per source:
  |           Semantic Scholar, OpenAlex, PubMed
  |      -> Each: rate limit -> HTTP request -> parse -> domain.Paper
  |      -> Aggregate results, record per-source metrics
  |      -> On failure: log warning, continue with next keyword
  |  Activity: StatusActivities.SavePapers
  |    -> PaperRepository.BulkUpsert() (dedup by canonical_id)
  |    -> ReviewRepository.IncrementCounters()
  v
Phase 3: Expansion Rounds (status: expanding, 1..MaxExpansionDepth)
  |  Check paper limit: totalPapersFound >= MaxPapers? -> stop
  |  Select top 5 papers with non-empty abstracts
  |  For each selected paper:
  |    Activity: LLMActivities.ExtractKeywords(abstract, mode="abstract",
  |              existingKeywords, context=originalQuery)
  |    -> On failure: log warning, skip paper, continue
  |  Activity: StatusActivities.SaveKeywords(newKeywords, round=N)
  |  For each new keyword:
  |    Activity: SearchActivities.SearchPapers
  |  Activity: StatusActivities.SavePapers(expansionPapers)
  v
Phase 4: Ingestion (status: ingesting)
  |  Filter papers with non-empty PDF URLs
  |  Activity: IngestionActivities.SubmitPapersForIngestion
  |    -> For each paper: ingestionClient.StartIngestion()
  |    -> Heartbeat progress to Temporal
  |    -> Idempotency key: "litreview/{requestID}/{paperID}"
  |    -> Non-fatal: failure logged, workflow continues to completion
  v
Phase 5: Complete (status: completed)
  |  Activity: StatusActivities.UpdateStatus(completed)
  |  Activity (fire-and-forget): EventActivities.PublishEvent("review.completed")
  |  Return ReviewWorkflowResult{RequestID, Status, KeywordsFound, PapersFound,
  |         PapersIngested, ExpansionRounds, Duration}
  v
Client polls GetLiteratureReviewStatus or receives StreamProgress events
```

---

## Temporal Workflow Design

### Workflow Function Signature

```go
func LiteratureReviewWorkflow(ctx workflow.Context, input ReviewWorkflowInput) (*ReviewWorkflowResult, error)
```

The workflow is a single deterministic function. It contains no I/O -- all
external interaction happens through activities. Temporal guarantees
exactly-once execution semantics even across process restarts.

### Signals and Queries

| Mechanism | Name       | Direction      | Purpose                                    |
|-----------|------------|----------------|--------------------------------------------|
| Signal    | `cancel`   | Client -> WF   | Request graceful cancellation              |
| Query     | `progress` | Client -> WF   | Read current `workflowProgress` snapshot   |

Signal handling uses `workflow.WithCancel` to create a cancellable context. A
background goroutine (`workflow.Go`) listens on the signal channel and invokes
the cancel function when received. All activity contexts derive from this
cancellable context.

Query handling registers a synchronous handler that returns the
`workflowProgress` struct, which is updated in-place as the workflow progresses
through phases:

```go
type workflowProgress struct {
    Status            string
    Phase             string
    KeywordsFound     int
    PapersFound       int
    PapersIngested    int
    PapersFailed      int
    ExpansionRound    int
    MaxExpansionDepth int
}
```

### Input and Result Types

Defined in the `temporal` package (not `workflows`) so the server layer can
construct them without importing the workflow package:

```go
// temporal/client.go
type ReviewWorkflowInput struct {
    RequestID uuid.UUID
    OrgID     string
    ProjectID string
    UserID    string
    Query     string
    Config    domain.ReviewConfiguration
}

// temporal/workflows/review_workflow.go
type ReviewWorkflowResult struct {
    RequestID       uuid.UUID
    Status          string
    KeywordsFound   int
    PapersFound     int
    PapersIngested  int
    ExpansionRounds int
    Duration        float64  // seconds
}
```

### Activity Options

Each activity type has tailored timeout and retry policies:

| Activity Type | StartToClose | HeartbeatTimeout | Max Attempts | Backoff (init, multiplier, max) |
|---------------|-------------|------------------|-------------|----------------------------------|
| LLM           | 2 min       | --               | 3           | 1s, 2x, 30s                     |
| Search        | 5 min       | --               | 3           | 2s, 2x, 60s                     |
| Status        | 30 sec      | --               | 5           | 500ms, 2x, 10s                  |
| Ingestion     | 5 min       | 2 min            | 3           | 2s, 2x, 60s                     |
| Event         | 30 sec      | --               | 5           | 500ms, 2x, 10s                  |

### Error Handling Strategy

```
                    Error occurs in activity
                              |
              +---------------+---------------+
              |               |               |
        Search failure   LLM expansion    Status/Save
        (per-keyword)    failure           failure
              |          (per-paper)            |
              v               v               v
        Log warning,     Log warning,    handleFailure():
        continue with    skip paper,       1. Update status to "failed"
        next keyword     continue            using ROOT context
                                            2. Publish "review.failed"
                                               event (fire-and-forget)
                                            3. Return original error
```

- **Search failures** are per-keyword and non-fatal. The workflow logs a warning
  and continues with the next keyword.
- **LLM failures** during expansion are per-paper and non-fatal. The paper is
  skipped and the expansion continues.
- **Ingestion failures** are non-fatal. The workflow completes with partial
  results and logs the failure.
- **Status update**, **keyword save**, and **paper save** failures are fatal.
  The workflow enters `handleFailure()`.
- `handleFailure()` uses the **root context** (not the cancelled context) to
  ensure status updates and failure events are delivered even after cancellation.

### Worker Configuration

| Setting                                    | Default |
|-------------------------------------------|---------|
| Max concurrent activity executions         | 100     |
| Max concurrent workflow task executions    | 50      |
| Activity task pollers                      | 4       |
| Workflow task pollers                      | 2       |
| Workflow execution timeout                 | 4 hours |

---

## LLM Integration

### Architecture

```
                    +-------------------+
                    | llm/extractor.go  |
                    | KeywordExtractor  |  <-- interface
                    +--------+----------+
                             |
                    +--------v----------+
                    | clientAdapter      |
                    | wraps sharedllm.   |
                    | Client             |
                    +--------+----------+
                             |
               +-------------+-------------+
               |                           |
    +----------v---------+     +-----------v----------+
    | sharedllm.Client   |     | sharedllm.           |
    | (OpenAI, Anthropic,|     | ResilientClient      |
    |  Azure, Bedrock,   |     | CB -> Budget ->      |
    |  Gemini, Vertex)   |     | RateLimiter -> Call   |
    +--------------------+     +----------------------+
```

### KeywordExtractor Interface

```go
type KeywordExtractor interface {
    ExtractKeywords(ctx context.Context, req ExtractionRequest) (*ExtractionResult, error)
    Provider() string  // e.g., "openai", "anthropic"
    Model() string     // e.g., "gpt-4o", "claude-sonnet-4-20250514"
}
```

The `clientAdapter` bridges the shared `llm.Client` to this local interface:

1. Validates input text is non-empty.
2. Builds system + user prompts via `BuildExtractionPrompt()`.
3. Calls `client.Complete()` with `ResponseFormat: "json"`.
4. Parses the JSON response: `{"keywords": [...], "reasoning": "..."}`.
5. Validates at least one keyword was extracted.
6. Returns `ExtractionResult` with keyword list and token usage metadata.

### Prompt Engineering

Two extraction modes with distinct system prompts:

**Query mode** (`ExtractionModeQuery`): Focuses on core research topics,
methodologies, and domain-specific terms from a user's natural language research
question.

**Abstract mode** (`ExtractionModeAbstract`): Focuses on key findings,
methodologies, organisms, genes, pathways, diseases, and domain-specific
terminology from a paper abstract. Receives the original query as context and
the list of existing keywords to avoid duplicates.

The system prompt includes seven guidelines for keyword quality:
1. Extract specific, searchable academic terms.
2. Avoid overly broad terms ("study", "research", "analysis").
3. Include synonyms and related concepts.
4. Prefer MeSH-style terms where applicable.
5. Include both abbreviated and expanded forms.
6. Consider multi-word phrases that function as single concepts.
7. Prioritize by likely search effectiveness.

When existing keywords are provided, the prompt instructs the LLM to find
complementary terms, not repeat them.

### Budget Tracking

Budget usage flows through the outbox pattern:

```
LLMActivities.ExtractKeywords (successful call)
    |
    +-> sharedllm.EstimateCost(model, inputTokens, outputTokens)
    |
    +-> BudgetUsageReporter.ReportUsage(BudgetUsageParams{
            LeaseID, OrgID, ProjectID, Model,
            InputTokens, OutputTokens, TotalTokens, CostUSD
        })
    |
    +-> outboxBudgetReporter.ReportUsage()
    |
    +-> outbox.AddBudgetUsageEvent(ctx, pool, payload)
    |
    +-> outbox_events table (status: pending)
    |
    +-> Outbox Relay -> Kafka -> core_service (budget reconciliation)
```

Budget reporting is best-effort: failures are logged at WARN level but never
fail the activity. The `BudgetUsageReporter` is injected via the options
pattern (`WithBudgetReporter`), so budget reporting is entirely optional.

### LLM Resilience (via shared package)

When wired through `sharedllm.ResilientClient`, extraction benefits from:

- **Adaptive rate limiter** -- reservation-based token bucket prevents race
  conditions under high concurrency.
- **Circuit breaker** -- dual-trigger mechanism (consecutive failures AND failure
  rate threshold). Callbacks invoked outside lock to prevent deadlocks.
- **Budget management** -- lease-based cost tracking. `core_service` allocates
  leases; the LLM package tracks usage locally.

---

## Paper Source Architecture

### Registry Pattern

```
+---------------------+
| papersources/       |
| Registry            |
|  mu sync.RWMutex    |
|  sources: map[      |
|    SourceType ->    |
|    PaperSource      |
|  ]                  |
+--------+------------+
         |
   +-----+------+--------+
   |            |         |
+--v------+  +--v------+  +--v------+
|Semantic |  |OpenAlex |  | PubMed  |
|Scholar  |  |Client   |  | Client  |
|Client   |  |         |  |         |
+---------+  +---------+  +---------+
```

### PaperSource Interface

```go
type PaperSource interface {
    Search(ctx context.Context, params SearchParams) (*SearchResult, error)
    GetByID(ctx context.Context, id string) (*domain.Paper, error)
    SourceType() domain.SourceType
    Name() string
    IsEnabled() bool
}
```

Each implementation:
- Transforms source-specific API responses into `domain.Paper` structs.
- Applies per-source rate limiting with adaptive backoff.
- Uses `io.LimitReader` on all HTTP response bodies for safety.
- Returns `domain.ExternalAPIError` for API failures with source context.

### Concurrent Search

`Registry.SearchSources()` launches one goroutine per requested source type,
collects results via a buffered channel, and returns all `SourceResult` values
(including errors). The caller decides how to handle partial failures.

```
SearchSources(ctx, params, sourceTypes)
    |
    +-- goroutine: SemanticScholar.Search(ctx, params) -> resultChan
    +-- goroutine: OpenAlex.Search(ctx, params)        -> resultChan
    +-- goroutine: PubMed.Search(ctx, params)          -> resultChan
    |
    wg.Wait() -> close(resultChan)
    |
    collect []SourceResult from resultChan
    (each contains Source, *SearchResult, error)
```

If `sourceTypes` is nil or empty, all enabled sources are searched. Sources not
found in the registry are silently skipped.

### Rate Limiting

Each source client has its own `RateLimiter` based on `golang.org/x/time/rate`
(token bucket algorithm):

```go
type RateLimiter struct {
    limiter *rate.Limiter  // goroutine-safe
}
```

The rate limiter exposes `SetRate()` and `SetBurst()` for dynamic adjustment
based on API response headers (e.g., `Retry-After`, `X-RateLimit-Remaining`).

### Deduplication

Papers are deduplicated by canonical ID during `PaperRepository.BulkUpsert()`.
The canonical ID is generated from the paper's external identifiers:

```
Priority: DOI > ArXiv > PubMed > Semantic Scholar > OpenAlex > Scopus

Format: "prefix:value"
  doi:10.1038/s41586-021-03819-2
  arxiv:2103.14030
  pubmed:34108480
  s2:204e3073
  openalex:W2741809807
  scopus:85107451923
```

A matching SQL function `generate_canonical_id()` implements the same logic
server-side for consistency.

---

## Database Design

### PostgreSQL Extensions

| Extension   | Purpose                                                  |
|-------------|----------------------------------------------------------|
| `uuid-ossp` | `uuid_generate_v4()` for primary keys                   |
| `pg_trgm`   | Trigram indexes for fuzzy text search on titles/abstracts|
| `pgcrypto`  | SHA256 for search window hash generation                 |

### Entity-Relationship Diagram

```
+-----------------------------+          +----------------------+
| literature_review_requests  |          | keywords             |
|-----------------------------|          |----------------------|
| id (PK, UUID)              |          | id (PK, UUID)        |
| org_id                     |          | keyword              |
| project_id                 |          | normalized_keyword   |
| user_id                    |          |   (UNIQUE)           |
| original_query             |          | created_at           |
| temporal_workflow_id       |          +----------+-----------+
| temporal_run_id            |                     |
| status (review_status)     |                     |
| keywords_found_count       |                     |
| papers_found_count         |                     |
| papers_ingested_count      |                     |
| papers_failed_count        |                     |
| config_snapshot (JSONB)    |                     |
| source_filters (JSONB)     |                     |
| created_at / updated_at   |                     |
| started_at / completed_at |                     |
+------+-------+-------------+                     |
       |       |                                    |
       |  +----v-----------------+  +---------------v-----------+
       |  | request_keyword_     |  | keyword_searches          |
       |  | mappings             |  |---------------------------|
       |  |----------------------|  | id (PK, UUID)             |
       |  | request_id (FK)      |  | keyword_id (FK)           |
       |  | keyword_id (FK)      |  | source_api (source_type)  |
       |  | extraction_round     |  | search_window_hash (UQ)   |
       |  | source_paper_id (FK) |  | papers_found              |
       |  | source_type          |  | status (search_status)    |
       |  +----------------------+  | error_message             |
       |                            +---------------------------+
       |
       |  +----------------------+          +---------------------+
       |  | request_paper_       |          | papers              |
       |  | mappings             |          |---------------------|
       |  |----------------------|          | id (PK, UUID)       |
       +->| request_id (FK)      |  +------>| canonical_id (UQ)   |
          | paper_id (FK) -------+--+       | title               |
          | discovered_via_      |          | abstract            |
          |   keyword_id (FK)    |          | authors (JSONB)     |
          | discovered_via_      |          | publication_date    |
          |   source             |          | publication_year    |
          | expansion_depth      |          | venue, journal      |
          | ingestion_status     |          | citation_count      |
          | ingestion_job_id     |          | pdf_url             |
          | ingestion_error      |          | open_access         |
          +----------------------+          | keywords_extracted  |
                                            | raw_metadata (JSONB)|
                                            +----+----+-----+----+
                                                 |    |     |
                              +------------------+    |     +-----------+
                              |                       |                 |
                    +---------v--------+  +-----------v-------+  +-----v-----------+
                    | paper_identifiers|  | paper_sources     |  | keyword_paper_  |
                    |------------------|  |-------------------|  | mappings        |
                    | paper_id (FK)    |  | paper_id (FK)     |  |-----------------|
                    | identifier_type  |  | source_api        |  | keyword_id (FK) |
                    | identifier_value |  | source_metadata   |  | paper_id (FK)   |
                    | source_api       |  |   (JSONB)         |  | mapping_type    |
                    +------------------+  +-------------------+  | source_type     |
                                                                 | confidence_score|
                                                                 +-----------------+

+---------------------------+     +------------------------+
| review_progress_events    |     | outbox_events          |
|---------------------------|     |------------------------|
| request_id (FK)           |     | event_id               |
| event_type                |     | aggregate_id/type      |
| event_data (JSONB)        |     | event_type             |
| created_at                |     | payload (BYTEA)        |
| TRIGGER: pg_notify()      |     | scope, org_id,         |
+---------------------------+     |   project_id           |
                                  | status, locked_by      |
+---------------------------+     | lock_expires_at        |
| outbox_dead_letter        |     +------------------------+
|---------------------------|
| original_event_id         |
| failure_reason            |
| retry_count               |
| reprocessed_at            |
+---------------------------+
```

### Migration Sequence

| Migration | Description                                                             |
|-----------|-------------------------------------------------------------------------|
| 000001    | Enable extensions: `uuid-ossp`, `pg_trgm`, `pgcrypto`                 |
| 000002    | Create 6 enum types: identifier_type, review_status, search_status, ingestion_status, mapping_type, source_type |
| 000003    | Core tables: keywords, papers, paper_identifiers, paper_sources, keyword_searches, keyword_paper_mappings |
| 000004    | Tenant tables: literature_review_requests, request_keyword_mappings, request_paper_mappings, review_progress_events |
| 000005    | Outbox tables: outbox_events (lease-based), outbox_dead_letter         |
| 000006    | Functions: generate_canonical_id, generate_search_window_hash, get_or_create_keyword, find_paper_by_identifier, find_paper_by_any_identifier, normalize_keyword |
| 000007    | Triggers: review progress NOTIFY, updated_at auto-timestamps (4 tables)|
| 000008    | Performance composite indexes for common query patterns                |

### Key Database Patterns

**Transactional Outbox**: Events are written to `outbox_events` in the same
transaction as the state change. An outbox relay (separate process) reads
pending events with lease-based locking and publishes them to Kafka, providing
at-least-once delivery guarantees. Dead-lettered events (max 5 attempts) go to
`outbox_dead_letter` for manual inspection.

**LISTEN/NOTIFY for Progress**: When a row is inserted into
`review_progress_events`, the `trg_review_progress_notify` trigger fires
`pg_notify('review_progress', payload)` with a JSON payload containing the
event_id, request_id, event_type, event_data, and created_at. Connected
clients listening on this channel receive real-time updates for SSE streaming.

**Idempotent Search Deduplication**: `keyword_searches.search_window_hash` is a
SHA256 hash of `keyword_id + source_api + date_range` (via
`generate_search_window_hash()`). The UNIQUE constraint prevents duplicate
searches for the same keyword/source/window combination.

**Advisory Locks**: `DB.AcquireAdvisoryLock()` and `AcquireAdvisoryLockTx()`
provide PostgreSQL advisory locks for distributed coordination without
dedicated lock tables. Session-scoped locks require explicit release;
transaction-scoped locks auto-release on commit/rollback.

**Trigram Indexes**: GIN indexes with `gin_trgm_ops` on `papers.title`,
`papers.abstract`, and `keywords.keyword` enable efficient fuzzy text search.
Abstract index is partial (`WHERE abstract IS NOT NULL`).

**Automatic Timestamps**: BEFORE UPDATE triggers on 4 tables
(`papers`, `literature_review_requests`, `request_paper_mappings`,
`paper_sources`) set `updated_at = NOW()` automatically.

---

## Multi-Tenancy

Every operation in the service is scoped to a specific organization and project.

### Tenant Context Flow

```
JWT Token (via Authorization header)
    |
    v
grpcauth interceptor (UnaryServerInterceptor / StreamServerInterceptor)
    |  Extracts org_id, project_id, user_id from JWT claims
    |  Injects AuthContext into request context
    v
validateTenantAccess(ctx, req.OrgId, req.ProjectId)
    |  Verifies JWT claims match request parameters
    |  Checks org membership + project role
    v
Repository.Get(ctx, orgID, projectID, id)
    |  SQL: WHERE org_id = $1 AND project_id = $2 AND id = $3
    v
ReviewWorkflowInput{OrgID, ProjectID}
    |  Propagated through all activity inputs
    v
OutboxEvent.WithTenant(orgID, projectID)
    |  Attached to all published events
```

### Enforcement Points

1. **Transport Layer**: `validateTenantAccess()` in every gRPC handler;
   `tenantContextMiddleware` in every HTTP handler.
2. **Repository Layer**: All queries include `org_id` and `project_id` in WHERE
   clauses. Composite indexes on `(org_id, project_id)`.
3. **Workflow Layer**: `ReviewWorkflowInput` carries `OrgID` and `ProjectID`
   through all activities. Activities pass tenant IDs in every DB and API call.
4. **Event Layer**: All outbox events include `org_id` and `project_id` with
   `scope: "project"` for tenant-aware event routing.

### Authentication

Authentication is handled by the shared `grpcauth` package:

- `env.FromEnvironment()` loads auth configuration from environment variables.
- `interceptor.NewServerInterceptor(authConfig)` creates gRPC interceptors.
- Health check and reflection endpoints are excluded via `SkipMethods`:
  `/grpc.health.v1.Health/Check`, `/grpc.health.v1.Health/Watch`,
  `/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo`.

---

## Error Handling

### Domain Error Hierarchy

```
Sentinel errors (matched via errors.Is):
    ErrNotFound           -> gRPC NOT_FOUND          / HTTP 404
    ErrAlreadyExists      -> gRPC ALREADY_EXISTS      / HTTP 409
    ErrInvalidInput       -> gRPC INVALID_ARGUMENT    / HTTP 400
    ErrUnauthorized       -> gRPC UNAUTHENTICATED     / HTTP 401
    ErrForbidden          -> gRPC PERMISSION_DENIED   / HTTP 403
    ErrRateLimited        -> gRPC RESOURCE_EXHAUSTED  / HTTP 429
    ErrServiceUnavailable -> gRPC UNAVAILABLE         / HTTP 503
    ErrInternalError      -> gRPC INTERNAL            / HTTP 500
    ErrWorkflowFailed     -> gRPC INTERNAL            / HTTP 500
    ErrCancelled          -> gRPC CANCELLED           / HTTP 499
    ErrNoIdentifier       (internal, not exposed to clients)

Typed errors (matched via errors.As, Unwrap to sentinels):
    *ValidationError     -> wraps ErrInvalidInput  (includes Field, Message)
    *NotFoundError       -> wraps ErrNotFound      (includes Entity, ID)
    *AlreadyExistsError  -> wraps ErrAlreadyExists (includes Entity, ID)
    *RateLimitError      -> wraps ErrRateLimited   (includes Source, RetryAfter)
    *ExternalAPIError    -> wraps Cause or ErrServiceUnavailable
                            (includes Source, StatusCode, Message)
```

### Error Patterns

- **`errors.As`** is always used for wrapped error type checking (never type
  assertions).
- **Internal errors never leak to clients**: `domainErrToGRPC()` and
  `writeDomainError()` return generic messages for unrecognized errors.
- **Activity errors are classified for metrics** via `errorType()` which checks
  for `sharedllm.Error` and legacy `llm.APIError` using `errors.As`.
- **Temporal errors are wrapped** into `TemporalError` with a `Kind` sentinel
  for clean `errors.Is` checking. Nine sentinel errors cover workflow-not-found,
  already-started, connection-failed, permission-denied, namespace-not-found,
  resource-exhausted, deadline-exceeded, query-failed, and client-closed.

### Temporal Error Wrapping

```go
type TemporalError struct {
    Op         string  // Operation that failed (e.g., "StartReviewWorkflow")
    Kind       error   // Category sentinel (e.g., ErrWorkflowNotFound)
    WorkflowID string  // Workflow ID (if applicable)
    RunID      string  // Run ID (if applicable)
    Err        error   // Underlying SDK error
}
```

Helper functions: `IsWorkflowNotFound()`, `IsWorkflowAlreadyStarted()`,
`IsQueryFailed()`, `IsConnectionFailed()`.

---

## Observability

### Structured Logging

- **Library**: zerolog (JSON output by default, console output for development).
- **Per-logger level**: `log.Level(level)` instead of `zerolog.SetGlobalLevel()`
  to avoid mutating shared state (critical for test isolation).
- **Context enrichment helpers**:
  - `WithReviewContext(logger, requestID, orgID, projectID)`
  - `WithSearchContext(logger, keyword, source)`
  - `WithPaperContext(logger, paperID, externalID)`
  - `WithTraceContext(logger, traceID, spanID)`
  - `WithWorkflowContext(logger, workflowID, runID)`
  - `WithActivityContext(logger, activityType, attempt)`
- **Temporal SDK integration**: `TemporalLogger` adapter converts zerolog to
  Temporal's `log.Logger` interface, auto-adding `component: "temporal-sdk"`.
  Converts alternating key-value pairs to zerolog fields.

### Prometheus Metrics

28 metrics organized into 7 subsystems:

| Subsystem  | Metrics                                                              | Labels                           |
|------------|----------------------------------------------------------------------|----------------------------------|
| Reviews    | started, completed, failed, cancelled (counters); duration (histogram, buckets: 1s-1h) | --                          |
| Keywords   | extracted total (counter); extractions by source (counter vec); per-review (histogram) | source                     |
| Searches   | started/completed/failed by source (counter vecs); duration (histogram, buckets: 0.1s-60s); papers per search (histogram) | source |
| Papers     | discovered, ingested, skipped, duplicate (counters); by source (counter vec) | source                          |
| Sources    | requests total, failed (counter vecs); duration (histogram, buckets: 0.01s-10s); rate limited (counter vec) | source, endpoint, error_type |
| Ingestion  | started, completed, failed (counters)                                | --                               |
| LLM        | requests total, failed (counter vecs); duration (histogram, buckets: 0.1s-60s); tokens used (counter vec) | operation, model, error_type, token_type |

Metrics are exposed on a dedicated port via `/metrics` (Prometheus scrape
endpoint). The metrics server is optional and only started when
`cfg.Metrics.Enabled` is true.

### Context Propagation

The `observability` package provides typed context keys for threading request
metadata through the call stack:

```go
// Available context values
request_id                    // Review request UUID
org_id, project_id            // Tenant context
trace_id, span_id             // Distributed tracing
workflow_id, workflow_run_id   // Temporal workflow context
```

`ReviewContext` struct aggregates all values and can be injected/extracted via
`WithReviewContextFull()` / `ReviewContextFromContext()`.

### Progress Streaming

Two mechanisms for real-time progress updates:

**gRPC Server Streaming**: `StreamLiteratureReviewProgress` polls the review
repository every 2 seconds and sends `LiteratureReviewProgressEvent` messages
until the review reaches a terminal state or the 4-hour max duration is
exceeded. Sends an initial `stream_started` event immediately, followed by
periodic `progress_update` events, and a final `completed` event.

**PostgreSQL LISTEN/NOTIFY**: The `trg_review_progress_notify` trigger fires on
`INSERT` to `review_progress_events`, sending a JSON payload on the
`review_progress` channel:

```json
{
    "event_id": "...",
    "request_id": "...",
    "event_type": "...",
    "event_data": {...},
    "created_at": "..."
}
```

The HTTP SSE endpoint at `/progress` listens on this channel for push-based
updates without polling.

---

## Configuration

### Loading Strategy

```
YAML config file (optional, e.g., config.yaml)
        |
        v
    Viper merges
        |
        v
Environment variables (LITREVIEW_ prefix, automatic binding)
        |
        v
    Override values from env take precedence
        |
        v
Secrets (loaded via loadSecrets() from os.Getenv() only)
    Fields tagged mapstructure:"-" are never loaded from config file
        |
        v
    Final Config struct with validated values
```

### Key Configuration Sections

| Section    | Env Prefix               | Key Fields                                    |
|------------|--------------------------|-----------------------------------------------|
| Server     | `LITREVIEW_SERVER_`      | Host, GRPCPort, HTTPPort, MetricsPort, ReadTimeout, WriteTimeout, ShutdownTimeout |
| Database   | `LITREVIEW_DATABASE_`    | Host, Port, Name, User, Password (secret), MaxConns, MinConns, MigrationAutoRun, MigrationPath |
| Temporal   | `LITREVIEW_TEMPORAL_`    | HostPort, Namespace, TaskQueue, TLS (cert/key/CA paths) |
| LLM        | `LLM_`                   | Provider, Model, API keys (secret), Temperature, Timeout, MaxRetries |
| Logging    | `LITREVIEW_LOGGING_`     | Level, Format (json/console), Output (stdout/stderr), AddSource, TimeFormat |
| Metrics    | `LITREVIEW_METRICS_`     | Enabled, Path (/metrics)                      |

---

## Binaries

The service produces 3 binaries from the `cmd/` directory:

### `cmd/server/` -- gRPC + HTTP Server

Starts 3 listeners concurrently:
- **gRPC** server with grpcauth interceptors, health check
  (`grpc_health_v1.Health`), reflection enabled.
- **HTTP REST** server (chi router) with JSON API and SSE streaming.
  Write timeout set to 5 minutes for long-lived SSE connections.
- **Metrics** server (optional, Prometheus `/metrics` endpoint on separate port).

**Graceful shutdown sequence**:
1. Receive SIGINT/SIGTERM.
2. Mark gRPC health status as NOT_SERVING.
3. Shut down HTTP server with configurable timeout.
4. Shut down metrics server (if running).
5. Gracefully stop gRPC server (forced stop after timeout).
6. Close Temporal workflow client.
7. Close database connection pool.

### `cmd/worker/` -- Temporal Worker

Registers workflows and activity structs with the Temporal SDK worker:
- **Workflow**: `LiteratureReviewWorkflow`
- **Activities**: `LLMActivities`, `SearchActivities`, `StatusActivities`,
  `IngestionActivities`, `EventActivities`

Polls the configured task queue and executes dispatched tasks. Blocks until
context cancellation or worker error.

### `cmd/migrate/` -- Database Migration CLI

Runs golang-migrate against PostgreSQL. Supports `up`, `down`, and `version`
commands. Can also be triggered automatically at server startup via
`Database.MigrationAutoRun` configuration.

---

## Testing Strategy

The service has 4+ test levels across 16 test packages:

### Unit Tests (`*_test.go` alongside source)

| Package            | Strategy                                                     |
|--------------------|--------------------------------------------------------------|
| `repository/`      | `pgxmock/v4` for mock database interactions                 |
| `activities/`      | Mock interfaces: `PaperSearcher`, `IngestionClient`, `EventPublisher`, `BudgetUsageReporter` |
| `workflows/`       | Temporal test environment (`testsuite.WorkflowTestSuite`)   |
| `domain/`          | Pure unit tests, no mocks needed                            |
| `papersources/`    | HTTP test server for API client testing                     |
| `observability/`   | Logger, metrics, and context propagation tests              |
| `database/`        | Migrator tests with mock filesystem                         |
| `server/`          | Handler tests with mock repositories and workflow client    |
| `server/http/`     | Handler + middleware + security tests                       |
| `outbox/`          | Emitter and adapter tests with mock inserter                |

### Workflow Determinism Tests (`workflows/determinism_test.go`)

Verify that helper functions produce stable, deterministic output:
- `SortedMapKeys` returns keys in consistent order.
- `SortedStringSlice` produces sorted copies without mutation.
- `DeduplicateStrings` removes duplicates and sorts.

### Workflow Replay Tests (`workflows/replay_test.go`)

Ensure workflow compatibility across code changes by replaying recorded
workflow histories.

### Integration Tests (`tests/integration/`)

Require live PostgreSQL and Temporal. Test database operations, outbox
publishing, and Temporal workflow execution end-to-end. Skipped with `-short`
flag.

### E2E Tests (`tests/e2e/`)

Full system tests that exercise the complete request flow from gRPC/HTTP call
to workflow completion, including paper source mocks.

### Security / Fuzz Tests (`tests/security/`)

Fuzz tests for input validation, SQL injection prevention (LIKE pattern
escaping), and error message sanitization.

### Chaos Tests (`tests/chaos/`)

Tests for resilience under failure conditions: database connection drops,
Temporal unavailability, LLM provider timeouts.

---

## Shared Package Dependencies

| Package                         | Source               | Usage                                       |
|---------------------------------|----------------------|---------------------------------------------|
| `github.com/helixir/grpcauth`  | `deepbio/grpcauth`  | JWT auth interceptors for gRPC + HTTP       |
| `github.com/helixir/outbox`    | `deepbio/outbox`    | Transactional outbox event publishing + relay |
| `github.com/helixir/llm`       | shared llm package   | Multi-provider LLM client + resilience layer |

**grpcauth**: Provides `env.FromEnvironment()` for config, server interceptors
for unary and stream RPCs, JWT validation, org membership, and project role
checking.

**outbox**: Provides `EventBuilder`, `MetadataBuilder`, `Repository` (insert),
and relay for the transactional outbox pattern. Events are written to
`outbox_events` and asynchronously forwarded to Kafka with lease-based
distributed processing.

**llm**: Provides a unified `Client` interface across 6 providers (OpenAI,
Anthropic, Azure, Bedrock, Gemini, Vertex) with optional `ResilientClient`
middleware (circuit breaker, adaptive rate limiter, budget tracking). Cost
estimation via `EstimateCost()` with prefix matching for versioned model names.

---

## Key Design Decisions

1. **Temporal over in-process orchestration**: Literature reviews can take
   minutes to hours. Temporal provides durable execution, automatic retry,
   cancellation, and progress queries without building custom infrastructure.
   Workflow state survives process restarts and deployments.

2. **Transactional outbox over direct Kafka publishing**: Ensures events are
   never lost even if Kafka is temporarily unavailable. The transactional
   guarantee means state changes and events are always consistent. Dead letter
   queue captures permanently failed events for manual inspection.

3. **Repository interfaces over direct SQL**: Enables mock-based unit testing
   via `pgxmock` and supports both pool and transaction contexts through the
   `DBTX` interface. All data access is behind a clean interface boundary.

4. **Canonical ID deduplication**: Papers discovered from multiple sources are
   unified by a deterministic canonical identifier with fixed priority order,
   avoiding duplicates across Semantic Scholar, OpenAlex, PubMed, etc. Both
   Go code and SQL functions implement the same algorithm.

5. **Activity-per-concern over monolithic activities**: Each activity struct
   (LLM, Search, Status, Ingestion, Event) has focused responsibility,
   independent retry policies, and separate timeout configurations. This allows
   fine-tuned resilience per operation type.

6. **Shared types in `temporal/` not `temporal/workflows/`**: `ReviewWorkflowInput`
   and signal/query constants are defined in the parent `temporal` package so the
   server layer can use them without importing the `workflows` package, avoiding
   circular dependency risks.

7. **Per-logger level over global level**: `zerolog.SetGlobalLevel()` mutates
   package-level state, causing test interference and preventing coexistence of
   loggers with different levels. Per-logger `.Level()` avoids this entirely.

8. **Budget reporting as best-effort**: LLM budget usage events flow through
   the outbox but failures never block the main workflow. This keeps the
   critical path fast while still providing cost visibility through the
   `core_service` budget reconciliation system.

9. **Fire-and-forget event publishing**: Outbox events published from the
   workflow (e.g., `review.started`, `review.completed`) use fire-and-forget
   semantics. Event publishing failure never fails the workflow, because the
   events are informational, not transactional dependencies.

10. **Dual API (gRPC + HTTP)**: gRPC serves as the primary inter-service
    protocol with strong typing and streaming support. HTTP REST serves as the
    client-facing API with SSE streaming, providing browser compatibility
    without requiring gRPC-Web proxying.
