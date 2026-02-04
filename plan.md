# Literature Review Service
## Complete Engineering Specification v2.0

---

# Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [System Architecture](#2-system-architecture)
3. [Configuration Schema](#3-configuration-schema)
4. [Database Schema](#4-database-schema)
5. [Protocol Buffer Definitions](#5-protocol-buffer-definitions)
6. [Project Structure](#6-project-structure)
7. [Core Components](#7-core-components)
8. [Temporal Workflows & Activities](#8-temporal-workflows--activities)
9. [External Integrations](#9-external-integrations)
10. [API Layer](#10-api-layer)
11. [Outbox & Event Publishing](#11-outbox--event-publishing)
12. [Observability](#12-observability)
13. [Security](#13-security)
14. [Implementation Phases & Deliverables](#14-implementation-phases--deliverables)
15. [Technical Decisions](#15-technical-decisions)
16. [Risk Mitigation](#16-risk-mitigation)
17. [Appendices](#17-appendices)

---

# 1. Executive Summary

## 1.1 Purpose

The Literature Review Service is a Go-based microservice that performs automated literature reviews by:
- Extracting keywords from natural language queries using LLM
- Searching multiple academic databases concurrently
- Recursively expanding searches based on discovered paper keywords
- Managing paper ingestion through a separate ingestion service
- Publishing events to Kafka for observability

## 1.2 Key Design Principles

| Principle | Implementation |
|-----------|----------------|
| **Durability** | Temporal workflows for retry, checkpointing, and recovery |
| **Idempotency** | Canonical paper identifiers, search window hashing, outbox pattern |
| **Multi-tenancy** | Organization and project scoping throughout |
| **Observability** | Structured logging, metrics, tracing, Kafka events |
| **Rate Limiting** | Per-source token buckets with local 429 backoff |
| **Security** | mTLS for service-to-service, JWT for user authentication |

## 1.3 Integration Points

| System | Protocol | Purpose |
|--------|----------|---------|
| Ingestion Service | gRPC + mTLS | Paper content ingestion |
| LLM Provider | HTTPS | Keyword extraction |
| Paper Sources (6) | HTTPS | Academic paper search |
| PostgreSQL | TCP + TLS | Persistence |
| Temporal | gRPC | Workflow orchestration |
| Kafka | TCP + TLS | Event publishing |

---

# 2. System Architecture

## 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                              Literature Review Service                                   │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  ┌────────────────────────────────────────────────────────────────────────────────────┐ │
│  │                              API Layer                                              │ │
│  │  ┌──────────────────┐     ┌──────────────────┐     ┌────────────────────────────┐  │ │
│  │  │   HTTP Server    │     │   gRPC Server    │     │     Shared Packages        │  │ │
│  │  │   (REST + SSE)   │     │   (Protobuf)     │     │  • grpcauth (middleware)   │  │ │
│  │  │                  │     │                  │     │  • outbox (Kafka publish)  │  │ │
│  │  │  • mTLS optional │     │  • mTLS required │     │                            │  │ │
│  │  │  • JWT required  │     │  • JWT required  │     │                            │  │ │
│  │  └────────┬─────────┘     └────────┬─────────┘     └────────────────────────────┘  │ │
│  │           │                        │                                                │ │
│  │           └───────────┬────────────┘                                                │ │
│  │                       ▼                                                             │ │
│  │  ┌──────────────────────────────────────────────────────────────────────────────┐  │ │
│  │  │                         Service Layer                                         │  │ │
│  │  │  • Request validation          • Tenant context extraction                    │  │ │
│  │  │  • Workflow initiation         • Progress streaming                           │  │ │
│  │  └──────────────────────────────────────────────────────────────────────────────┘  │ │
│  └────────────────────────────────────────────────────────────────────────────────────┘ │
│                                         │                                                │
│  ┌────────────────────────────────────────────────────────────────────────────────────┐ │
│  │                           Temporal Workflow Engine                                  │ │
│  │                                                                                     │ │
│  │  ┌─────────────────────────────────────────────────────────────────────────────┐  │ │
│  │  │                        Main Literature Review Workflow                       │  │ │
│  │  │                                                                              │  │ │
│  │  │  • Deterministic execution (sorted map iterations)                          │  │ │
│  │  │  • Query handler for progress state                                         │  │ │
│  │  │  • ContinueAsNew for large paper sets (>500)                               │  │ │
│  │  │  • Proper error handling with event publishing                              │  │ │
│  │  └─────────────────────────────────────────────────────────────────────────────┘  │ │
│  │                                      │                                             │ │
│  │  ┌───────────────────────────────────┴───────────────────────────────────────┐    │ │
│  │  │                         Child Workflows                                    │    │ │
│  │  │  ┌─────────────────────────┐  ┌─────────────────────────────────────────┐ │    │ │
│  │  │  │  Paper Search Workflow  │  │  Keyword Expansion Workflow             │ │    │ │
│  │  │  │  (Parallel by source)   │  │  (LLM extraction from papers)           │ │    │ │
│  │  │  └─────────────────────────┘  └─────────────────────────────────────────┘ │    │ │
│  │  └───────────────────────────────────────────────────────────────────────────┘    │ │
│  │                                      │                                             │ │
│  │  ┌───────────────────────────────────┴───────────────────────────────────────┐    │ │
│  │  │                              Activities                                    │    │ │
│  │  │                                                                            │    │ │
│  │  │  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐              │    │ │
│  │  │  │    LLM     │ │   Search   │ │  Database  │ │ Ingestion  │              │    │ │
│  │  │  │ Activities │ │ Activities │ │ Activities │ │ Activities │              │    │ │
│  │  │  └────────────┘ └────────────┘ └────────────┘ └────────────┘              │    │ │
│  │  │                                                                            │    │ │
│  │  │  • Local 429/503 backoff before Temporal retry                            │    │ │
│  │  │  • Heartbeat for long-running operations                                  │    │ │
│  │  │  • Idempotent external calls                                              │    │ │
│  │  └───────────────────────────────────────────────────────────────────────────┘    │ │
│  └────────────────────────────────────────────────────────────────────────────────────┘ │
│                                         │                                                │
│  ┌────────────────────────────────────────────────────────────────────────────────────┐ │
│  │                              Data Layer                                             │ │
│  │                                                                                     │ │
│  │  ┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────────────┐│ │
│  │  │     PostgreSQL      │  │    Outbox Table     │  │   Outbox Worker (shared)    ││ │
│  │  │                     │  │                     │  │                             ││ │
│  │  │  • Papers (global)  │  │  • Transactional    │──│  • Polls unpublished events ││ │
│  │  │  • Identifiers      │  │    event storage    │  │  • Publishes to Kafka       ││ │
│  │  │  • Keywords         │  │  • Delivery         │  │  • Handles retries          ││ │
│  │  │  • Searches         │  │    guarantee        │  │                             ││ │
│  │  │  • Requests         │  │                     │  │                             ││ │
│  │  │  • Progress events  │  │                     │  │                             ││ │
│  │  │                     │  │                     │  │                             ││ │
│  │  │  + LISTEN/NOTIFY    │  │                     │  │                             ││ │
│  │  │    for streaming    │  │                     │  │                             ││ │
│  │  └─────────────────────┘  └─────────────────────┘  └─────────────────────────────┘│ │
│  │                                                                                     │ │
│  │  ┌─────────────────────────────────────────────────────────────────────────────┐  │ │
│  │  │                         Rate Limiter Pool                                    │  │ │
│  │  │  • Per-source token bucket limiters                                         │  │ │
│  │  │  • Configurable concurrency per source                                      │  │ │
│  │  │  • Metrics on throttling                                                    │  │ │
│  │  └─────────────────────────────────────────────────────────────────────────────┘  │ │
│  └────────────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                          │
└─────────────────────────────────────────────────────────────────────────────────────────┘
                                          │
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                                  External Systems                                        │
├─────────────┬─────────────┬─────────────────┬─────────────────┬─────────────────────────┤
│             │             │                 │                 │                         │
│ PostgreSQL  │  Temporal   │ Ingestion Svc   │   Paper APIs    │        Kafka            │
│             │  Server     │ (gRPC+mTLS)     │   (6 sources)   │   (Observability)       │
│             │             │                 │                 │                         │
│  • Main DB  │  • History  │  • Ingest PDF   │  • Semantic     │  • Event streaming      │
│  • Outbox   │  • Tasks    │  • Extract text │    Scholar      │  • Observability svc    │
│             │  • Queries  │  • Index        │  • OpenAlex     │    consumes events      │
│             │             │                 │  • Scopus       │                         │
│             │             │                 │  • PubMed       │                         │
│             │             │                 │  • bioRxiv      │                         │
│             │             │                 │  • arXiv        │                         │
│             │             │                 │                 │                         │
└─────────────┴─────────────┴─────────────────┴─────────────────┴─────────────────────────┘
```

## 2.2 Request Flow Diagram

```
┌──────────┐      ┌─────────────┐      ┌───────────────┐      ┌──────────────┐
│  Client  │──────│  HTTP/gRPC  │──────│  Service      │──────│   Temporal   │
│          │      │  Server     │      │  Layer        │      │   Client     │
└──────────┘      └─────────────┘      └───────────────┘      └──────────────┘
     │                  │                     │                      │
     │  POST /reviews   │                     │                      │
     │─────────────────>│                     │                      │
     │                  │                     │                      │
     │                  │  Validate + Auth    │                      │
     │                  │────────────────────>│                      │
     │                  │                     │                      │
     │                  │                     │  Start Workflow      │
     │                  │                     │─────────────────────>│
     │                  │                     │                      │
     │                  │                     │  Workflow ID         │
     │                  │                     │<─────────────────────│
     │                  │                     │                      │
     │                  │  Save Request       │                      │
     │                  │  + Publish Event    │                      │
     │                  │<────────────────────│                      │
     │                  │                     │                      │
     │  202 Accepted    │                     │                      │
     │  {review_id}     │                     │                      │
     │<─────────────────│                     │                      │
     │                  │                     │                      │
     │                  │                     │                      │
     │  GET /reviews/{id}/progress (SSE)     │                      │
     │─────────────────>│                     │                      │
     │                  │                     │                      │
     │                  │  LISTEN review_progress_{id}               │
     │                  │────────────────────────────────────────────│───> PostgreSQL
     │                  │                     │                      │
     │                  │  Query Workflow     │                      │
     │                  │────────────────────────────────────────────│───> Temporal
     │                  │                     │                      │
     │  SSE: progress   │                     │                      │
     │<─────────────────│                     │                      │
     │  SSE: progress   │                     │                      │
     │<─────────────────│                     │                      │
     │  SSE: completed  │                     │                      │
     │<─────────────────│                     │                      │
     │                  │                     │                      │
```

## 2.3 Workflow Execution Flow

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                           Literature Review Workflow                                 │
├─────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                      │
│  ┌─────────────────────────────────────────────────────────────────────────────┐    │
│  │ PHASE 1: Keyword Extraction                                                  │    │
│  │                                                                              │    │
│  │  User Query ──> LLM Activity ──> N Keywords ──> Save to DB                  │    │
│  │                                                                              │    │
│  │  "machine learning for protein folding" ──> ["protein folding", "deep       │    │
│  │   learning", "alphafold", "molecular dynamics", "structure prediction"]     │    │
│  └─────────────────────────────────────────────────────────────────────────────┘    │
│                                       │                                              │
│                                       ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────────────┐    │
│  │ PHASE 2: Paper Search (Depth 0)                                              │    │
│  │                                                                              │    │
│  │  For each keyword, for each enabled source:                                  │    │
│  │                                                                              │    │
│  │  ┌─────────────────────────────────────────────────────────────────────┐    │    │
│  │  │ Child Workflow: PaperSearchWorkflow                                  │    │    │
│  │  │                                                                      │    │    │
│  │  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐                 │    │    │
│  │  │  │  Semantic    │ │   OpenAlex   │ │    Scopus    │  ... (parallel) │    │    │
│  │  │  │  Scholar     │ │              │ │              │                 │    │    │
│  │  │  │  Activity    │ │   Activity   │ │   Activity   │                 │    │    │
│  │  │  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘                 │    │    │
│  │  │         │                │                │                          │    │    │
│  │  │         │   Rate Limit   │   Rate Limit   │   Rate Limit            │    │    │
│  │  │         │   + Backoff    │   + Backoff    │   + Backoff             │    │    │
│  │  │         │                │                │                          │    │    │
│  │  │         └────────────────┴────────────────┘                          │    │    │
│  │  │                          │                                           │    │    │
│  │  │                          ▼                                           │    │    │
│  │  │                   Deduplicated Papers                                │    │    │
│  │  │                   (via canonical IDs)                                │    │    │
│  │  └─────────────────────────────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────────────────────────────┘    │
│                                       │                                              │
│                                       ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────────────┐    │
│  │ PHASE 3: Keyword Expansion (Depth 1...MaxDepth)                              │    │
│  │                                                                              │    │
│  │  For each new paper found:                                                   │    │
│  │                                                                              │    │
│  │  ┌─────────────────────────────────────────────────────────────────────┐    │    │
│  │  │ Child Workflow: KeywordExpansionWorkflow                             │    │    │
│  │  │                                                                      │    │    │
│  │  │  Paper Title + Abstract ──> LLM Activity ──> M Keywords              │    │    │
│  │  │                                                                      │    │    │
│  │  │  Filter: Only NEW keywords not already searched                      │    │    │
│  │  │                                                                      │    │    │
│  │  └─────────────────────────────────────────────────────────────────────┘    │    │
│  │                                                                              │    │
│  │  Loop back to PHASE 2 with new keywords until:                              │    │
│  │    • MaxDepth reached                                                        │    │
│  │    • No new keywords found                                                   │    │
│  │    • Paper count > 500 (ContinueAsNew)                                      │    │
│  └─────────────────────────────────────────────────────────────────────────────┘    │
│                                       │                                              │
│                                       ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────────────┐    │
│  │ PHASE 4: Ingestion                                                           │    │
│  │                                                                              │    │
│  │  ┌─────────────────────────────────────────────────────────────────────┐    │    │
│  │  │ Activity: SendToIngestionActivity                                    │    │    │
│  │  │                                                                      │    │    │
│  │  │  • Batch papers (50 per request)                                    │    │    │
│  │  │  • Call Ingestion Service gRPC                                      │    │    │
│  │  │  • Idempotent via paper_id + review_id                              │    │    │
│  │  │  • Update request_paper_mappings.ingestion_status                   │    │    │
│  │  └─────────────────────────────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────────────────────────────┘    │
│                                       │                                              │
│                                       ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────────────┐    │
│  │ PHASE 5: Completion                                                          │    │
│  │                                                                              │    │
│  │  • Update literature_review_requests.status = 'completed'                   │    │
│  │  • Publish review.completed event to outbox                                 │    │
│  │  • Insert progress event for NOTIFY                                         │    │
│  │  • Return LiteratureReviewOutput                                            │    │
│  └─────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                      │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

---

# 3. Configuration Schema

```yaml
# config.yaml - Complete configuration schema

# ============================================
# SERVICE CONFIGURATION
# ============================================
service:
  name: "literature-review-service"
  version: "2.0.0"
  http_port: 8080
  grpc_port: 9090
  metrics_port: 9091
  environment: "production"  # development | staging | production
  
  # Graceful shutdown
  shutdown_timeout_seconds: 30
  
  # Request limits
  max_request_size_mb: 10
  request_timeout_seconds: 300

# ============================================
# AUTHENTICATION & AUTHORIZATION
# ============================================
auth:
  mtls:
    enabled: true
    ca_cert_path: "/certs/ca.pem"
    server_cert_path: "/certs/server.pem"
    server_key_path: "/certs/server-key.pem"
    client_ca_path: "/certs/client-ca.pem"
    
    # Whether to enforce client cert verification on HTTP
    # Set to false for browser clients (they use JWT only)
    # Set to true for service-to-service only endpoints
    enforce_http_mtls: false
    
    # gRPC always enforces mTLS
    
  jwt:
    issuer: "auth.company.com"
    audience: "literature-review-service"
    jwks_url: "https://auth.company.com/.well-known/jwks.json"
    jwks_refresh_interval_minutes: 60
    
    # Required claims for authorization
    required_claims:
      - "sub"      # user_id
      - "org_id"   # organization
      - "roles"    # permissions

# ============================================
# LLM CONFIGURATION
# ============================================
llm:
  provider: "openai"  # openai | anthropic | azure_openai
  
  openai:
    model: "gpt-4-turbo"
    api_key_env: "OPENAI_API_KEY"
    base_url: "https://api.openai.com/v1"
    
  anthropic:
    model: "claude-3-sonnet-20240229"
    api_key_env: "ANTHROPIC_API_KEY"
    base_url: "https://api.anthropic.com"
    
  azure_openai:
    deployment_name: "gpt-4-turbo"
    api_key_env: "AZURE_OPENAI_API_KEY"
    endpoint_env: "AZURE_OPENAI_ENDPOINT"
    api_version: "2024-02-15-preview"
  
  # Extraction settings
  initial_keyword_count: 5        # 'n' keywords from user query
  paper_keyword_count: 3          # 'm' keywords per paper
  max_expansion_depth: 2          # recursive search depth
  
  # Request settings
  timeout_seconds: 60
  max_retries: 3
  retry_initial_interval_ms: 1000
  retry_max_interval_ms: 30000
  retry_multiplier: 2.0
  
  # Token limits
  max_input_tokens: 4000
  max_output_tokens: 1000
  temperature: 0.3

# ============================================
# PAPER SOURCE CONFIGURATIONS
# ============================================
paper_sources:
  # Semantic Scholar
  semantic_scholar:
    enabled: true
    base_url: "https://api.semanticscholar.org/graph/v1"
    api_key_env: "SEMANTIC_SCHOLAR_API_KEY"
    
    rate_limit:
      requests_per_second: 10
      burst: 20
      
    concurrency: 5  # Max parallel requests
    timeout_seconds: 30
    max_results_per_query: 100
    
    backoff:
      initial_interval_ms: 1000
      max_interval_ms: 60000
      multiplier: 2.0
      max_retries: 5
      
    # Fields to request
    fields:
      - "paperId"
      - "externalIds"
      - "title"
      - "abstract"
      - "authors"
      - "year"
      - "venue"
      - "citationCount"
      - "openAccessPdf"

  # OpenAlex
  openalex:
    enabled: true
    base_url: "https://api.openalex.org"
    # OpenAlex uses polite pool with email
    email: "literature-review@company.com"
    
    rate_limit:
      requests_per_second: 10
      burst: 30
      
    concurrency: 8
    timeout_seconds: 20
    max_results_per_query: 100
    
    backoff:
      initial_interval_ms: 500
      max_interval_ms: 30000
      multiplier: 2.0
      max_retries: 5

  # Scopus (Elsevier)
  scopus:
    enabled: true
    base_url: "https://api.elsevier.com/content/search/scopus"
    api_key_env: "SCOPUS_API_KEY"
    inst_token_env: "SCOPUS_INST_TOKEN"  # Optional institutional token
    
    rate_limit:
      requests_per_second: 3
      burst: 5
      
    concurrency: 3
    timeout_seconds: 30
    max_results_per_query: 100
    
    backoff:
      initial_interval_ms: 2000
      max_interval_ms: 120000
      multiplier: 2.0
      max_retries: 5

  # PubMed (NCBI E-utilities)
  pubmed:
    enabled: true
    base_url: "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"
    api_key_env: "PUBMED_API_KEY"
    
    rate_limit:
      requests_per_second: 10  # 10/sec with API key, 3/sec without
      burst: 10
      
    concurrency: 5
    timeout_seconds: 30
    max_results_per_query: 100
    
    backoff:
      initial_interval_ms: 1000
      max_interval_ms: 60000
      multiplier: 2.0
      max_retries: 5

  # bioRxiv/medRxiv
  biorxiv:
    enabled: true
    base_url: "https://api.biorxiv.org/details"
    
    rate_limit:
      requests_per_second: 5
      burst: 10
      
    concurrency: 4
    timeout_seconds: 20
    max_results_per_query: 100
    
    backoff:
      initial_interval_ms: 1000
      max_interval_ms: 60000
      multiplier: 2.0
      max_retries: 5
    
    # Include medRxiv
    include_medrxiv: true

  # arXiv
  arxiv:
    enabled: true
    base_url: "http://export.arxiv.org/api/query"
    
    rate_limit:
      requests_per_second: 1  # arXiv is strict
      burst: 3
      
    concurrency: 2
    timeout_seconds: 45
    max_results_per_query: 100
    
    backoff:
      initial_interval_ms: 3000
      max_interval_ms: 180000
      multiplier: 2.0
      max_retries: 5

# ============================================
# DATABASE CONFIGURATION
# ============================================
database:
  host: "localhost"
  port: 5432
  name: "literature_review"
  user_env: "DB_USER"
  password_env: "DB_PASSWORD"
  
  ssl_mode: "require"  # disable | require | verify-ca | verify-full
  ssl_ca_path: "/certs/db-ca.pem"
  
  # Connection pool
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_minutes: 30
  conn_max_idle_time_minutes: 5
  
  # Query settings
  default_query_timeout_seconds: 30
  
  # Migrations
  migrations_path: "/app/migrations"
  auto_migrate: true

# ============================================
# TEMPORAL CONFIGURATION
# ============================================
temporal:
  host: "temporal.default.svc.cluster.local:7233"
  namespace: "literature-review"
  task_queue: "literature-review-tasks"
  
  # TLS (optional)
  tls:
    enabled: false
    ca_cert_path: "/certs/temporal-ca.pem"
    client_cert_path: "/certs/temporal-client.pem"
    client_key_path: "/certs/temporal-client-key.pem"
  
  # Workflow settings
  workflow_execution_timeout_hours: 24
  workflow_run_timeout_hours: 12
  workflow_task_timeout_seconds: 60
  
  # Activity settings
  activity_start_to_close_timeout_seconds: 300
  activity_schedule_to_close_timeout_seconds: 600
  activity_heartbeat_timeout_seconds: 60
  
  # Retry policy
  activity_retry:
    initial_interval_seconds: 1
    backoff_coefficient: 2.0
    max_interval_seconds: 60
    max_attempts: 5
    non_retryable_errors:
      - "ValidationError"
      - "AuthorizationError"
      - "NotFoundError"
  
  # Worker settings
  worker:
    max_concurrent_workflow_tasks: 100
    max_concurrent_activity_tasks: 50
    max_concurrent_local_activity_tasks: 50
  
  # History management
  max_papers_per_workflow_batch: 500  # ContinueAsNew threshold

# ============================================
# INGESTION SERVICE CONFIGURATION
# ============================================
ingestion_service:
  grpc_address: "ingestion-service.default.svc.cluster.local:9090"
  
  mtls:
    enabled: true
    ca_cert_path: "/certs/ingestion-ca.pem"
    client_cert_path: "/certs/ingestion-client.pem"
    client_key_path: "/certs/ingestion-client-key.pem"
    server_name: "ingestion-service"
  
  timeout_seconds: 60
  batch_size: 50
  
  # Retry (handled by Temporal, but we set reasonable call timeout)
  call_retry:
    enabled: false  # Let Temporal handle retries

# ============================================
# OUTBOX CONFIGURATION
# ============================================
outbox:
  enabled: true
  
  # Polling settings
  poll_interval_ms: 100
  batch_size: 100
  max_retries: 5
  retry_interval_ms: 1000
  
  # Cleanup
  retention_hours: 168  # 7 days
  cleanup_interval_hours: 1
  
  # Kafka producer
  kafka:
    brokers:
      - "kafka-0.kafka.default.svc.cluster.local:9092"
      - "kafka-1.kafka.default.svc.cluster.local:9092"
      - "kafka-2.kafka.default.svc.cluster.local:9092"
    
    topic_prefix: "literature-review"
    
    # Topics will be: literature-review.events, literature-review.events.dlq
    
    tls:
      enabled: true
      ca_cert_path: "/certs/kafka-ca.pem"
      client_cert_path: "/certs/kafka-client.pem"
      client_key_path: "/certs/kafka-client-key.pem"
    
    sasl:
      enabled: false
      mechanism: "PLAIN"  # PLAIN | SCRAM-SHA-256 | SCRAM-SHA-512
      username_env: "KAFKA_SASL_USERNAME"
      password_env: "KAFKA_SASL_PASSWORD"
    
    producer:
      acks: "all"  # 0 | 1 | all
      retries: 3
      batch_size: 16384
      linger_ms: 5
      compression: "snappy"  # none | gzip | snappy | lz4 | zstd
      idempotent: true

# ============================================
# OBSERVABILITY CONFIGURATION
# ============================================
observability:
  # Distributed tracing
  tracing:
    enabled: true
    exporter: "otlp"  # otlp | jaeger | zipkin
    otlp_endpoint: "otel-collector.observability.svc.cluster.local:4317"
    otlp_insecure: false
    sample_rate: 0.1
    service_name: "literature-review-service"
    
  # Prometheus metrics
  metrics:
    enabled: true
    port: 9091
    path: "/metrics"
    
    # Custom buckets for histograms
    latency_buckets: [0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120]
    
  # Structured logging
  logging:
    level: "info"  # debug | info | warn | error
    format: "json"  # json | text
    
    # Include these fields in all logs
    default_fields:
      service: "literature-review-service"
      version: "${SERVICE_VERSION}"
      
    # Sampling for high-volume logs
    sampling:
      enabled: true
      initial: 100       # Log first 100
      thereafter: 100    # Then every 100th

# ============================================
# PAGINATION CONFIGURATION
# ============================================
pagination:
  default_page_size: 20
  max_page_size: 100
  
  # Token signing for opaque pagination tokens
  token_secret_env: "PAGINATION_TOKEN_SECRET"
  token_ttl_hours: 24

# ============================================
# FEATURE FLAGS
# ============================================
features:
  # Enable/disable specific paper sources dynamically
  paper_sources:
    semantic_scholar: true
    openalex: true
    scopus: true
    pubmed: true
    biorxiv: true
    arxiv: true
  
  # Experimental features
  experimental:
    parallel_keyword_extraction: false
    smart_deduplication: false
```

---

# 4. Database Schema

## 4.1 Entity Relationship Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                                   DATABASE SCHEMA                                        │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  ┌─────────────────────┐         ┌─────────────────────┐         ┌──────────────────┐  │
│  │      keywords       │         │       papers        │         │ paper_identifiers│  │
│  ├─────────────────────┤         ├─────────────────────┤         ├──────────────────┤  │
│  │ id (PK)             │         │ id (PK)             │◄────────│ paper_id (FK)    │  │
│  │ keyword             │         │ canonical_id (UQ)   │         │ identifier_type  │  │
│  │ normalized_keyword  │         │ title               │         │ identifier_value │  │
│  │ created_at          │         │ abstract            │         │ source_api       │  │
│  └─────────┬───────────┘         │ authors (JSONB)     │         │ (UQ: type+value) │  │
│            │                     │ publication_date    │         └──────────────────┘  │
│            │                     │ keywords_extracted  │                                │
│            │                     │ ...                 │         ┌──────────────────┐  │
│            │                     └──────────┬──────────┘         │  paper_sources   │  │
│            │                                │                    ├──────────────────┤  │
│            │                                │◄───────────────────│ paper_id (FK)    │  │
│            │                                │                    │ source_api       │  │
│            │                                │                    │ first_seen_at    │  │
│            │                                │                    │ last_seen_at     │  │
│            │                                │                    └──────────────────┘  │
│            │                                │                                           │
│            │    ┌───────────────────────────┼───────────────────────────┐               │
│            │    │                           │                           │               │
│            ▼    ▼                           ▼                           │               │
│  ┌─────────────────────────┐    ┌─────────────────────────┐            │               │
│  │ keyword_paper_mappings  │    │    keyword_searches     │            │               │
│  ├─────────────────────────┤    ├─────────────────────────┤            │               │
│  │ id (PK)                 │    │ id (PK)                 │            │               │
│  │ keyword_id (FK)         │    │ keyword_id (FK)         │            │               │
│  │ paper_id (FK)           │    │ source_api              │            │               │
│  │ mapping_type            │    │ search_from_date        │            │               │
│  │ source_type             │    │ search_to_date          │            │               │
│  │ confidence_score        │    │ search_window_hash (UQ) │            │               │
│  └─────────────────────────┘    │ papers_found            │            │               │
│                                 │ status                  │            │               │
│                                 └─────────────────────────┘            │               │
│                                                                        │               │
│  ┌─────────────────────────────────────────────────────────────────────┼─────────────┐ │
│  │                    TENANT-SCOPED TABLES                             │             │ │
│  │                                                                     │             │ │
│  │  ┌─────────────────────────────┐    ┌─────────────────────────────┐│             │ │
│  │  │ literature_review_requests  │    │  request_keyword_mappings   ││             │ │
│  │  ├─────────────────────────────┤    ├─────────────────────────────┤│             │ │
│  │  │ id (PK)                     │◄───│ request_id (FK)             ││             │ │
│  │  │ org_id                      │    │ keyword_id (FK)             │┼─────────────┘ │
│  │  │ project_id                  │    │ extraction_round            │               │
│  │  │ user_id                     │    │ source_paper_id (FK)        │               │
│  │  │ original_query              │    │ source_type                 │               │
│  │  │ temporal_workflow_id        │    └─────────────────────────────┘               │
│  │  │ status                      │                                                   │
│  │  │ config_snapshot (JSONB)     │    ┌─────────────────────────────┐               │
│  │  │ ...                         │    │   request_paper_mappings    │               │
│  │  └─────────────┬───────────────┘    ├─────────────────────────────┤               │
│  │                │                    │ request_id (FK)             │               │
│  │                │◄───────────────────│ paper_id (FK)               │───────────────┘
│  │                │                    │ discovered_via_keyword_id   │
│  │                │                    │ discovered_via_source       │
│  │                │                    │ expansion_depth             │
│  │                │                    │ ingestion_status            │
│  │                │                    │ ingestion_job_id            │
│  │                │                    └─────────────────────────────┘
│  │                │
│  │                │                    ┌─────────────────────────────┐
│  │                │                    │   review_progress_events    │
│  │                └───────────────────►├─────────────────────────────┤
│  │                                     │ request_id (FK)             │
│  │                                     │ event_type                  │
│  │                                     │ event_data (JSONB)          │
│  │                                     │ created_at                  │
│  │                                     └─────────────────────────────┘
│  │                                                │
│  │                                                │ TRIGGER: pg_notify()
│  │                                                ▼
│  │                                     ┌─────────────────────────────┐
│  │                                     │    PostgreSQL NOTIFY        │
│  │                                     │    Channel: review_progress_│
│  │                                     │              {request_id}   │
│  │                                     └─────────────────────────────┘
│  └───────────────────────────────────────────────────────────────────────────────────┘
│                                                                                          │
│  ┌───────────────────────────────────────────────────────────────────────────────────┐  │
│  │                              OUTBOX TABLE                                          │  │
│  │  ┌─────────────────────────────┐                                                  │  │
│  │  │       outbox_events         │                                                  │  │
│  │  ├─────────────────────────────┤          ┌─────────────────────────────┐         │  │
│  │  │ id (PK)                     │          │      Outbox Worker          │         │  │
│  │  │ aggregate_type              │          │      (shared package)       │         │  │
│  │  │ aggregate_id                │─────────►│                             │────────►│  │
│  │  │ event_type                  │  polls   │  • Poll unpublished         │  Kafka  │  │
│  │  │ org_id                      │          │  • Publish to Kafka         │         │  │
│  │  │ project_id                  │          │  • Mark published           │         │  │
│  │  │ payload (JSONB)             │          │  • Handle failures          │         │  │
│  │  │ metadata (JSONB)            │          └─────────────────────────────┘         │  │
│  │  │ published_at (NULL=pending) │                                                  │  │
│  │  │ sequence_num                │                                                  │  │
│  │  └─────────────────────────────┘                                                  │  │
│  └───────────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                          │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

## 4.2 Complete SQL Schema

```sql
-- ============================================================================
-- LITERATURE REVIEW SERVICE - DATABASE SCHEMA
-- Version: 2.0.0
-- ============================================================================

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- CUSTOM TYPES
-- ============================================================================

CREATE TYPE identifier_type AS ENUM (
    'doi',
    'arxiv_id', 
    'pubmed_id',
    'pmcid',
    'semantic_scholar_id',
    'openalex_id',
    'scopus_id'
);

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

CREATE TYPE search_status AS ENUM (
    'pending',
    'in_progress',
    'completed',
    'failed'
);

CREATE TYPE ingestion_status AS ENUM (
    'pending',
    'queued',
    'ingesting',
    'completed',
    'failed',
    'skipped'
);

CREATE TYPE mapping_type AS ENUM (
    'search_result',
    'extracted'
);

CREATE TYPE source_type AS ENUM (
    'user_query',
    'paper_extraction'
);

-- ============================================================================
-- KEYWORDS TABLE
-- Global dictionary of normalized keywords (no source_type here)
-- ============================================================================

CREATE TABLE keywords (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Original keyword as provided
    keyword TEXT NOT NULL,
    
    -- Normalized: lowercase, trimmed, whitespace collapsed
    -- Note: No stemming - document says "lowercase, trimmed (no stemming)"
    normalized_keyword TEXT NOT NULL,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Each normalized keyword is unique globally
    CONSTRAINT uq_keywords_normalized UNIQUE (normalized_keyword)
);

-- Indexes
CREATE INDEX idx_keywords_normalized_trgm ON keywords 
    USING gin (normalized_keyword gin_trgm_ops);
CREATE INDEX idx_keywords_created_at ON keywords (created_at);

COMMENT ON TABLE keywords IS 'Global dictionary of unique normalized keywords';
COMMENT ON COLUMN keywords.normalized_keyword IS 'Lowercase, trimmed keyword - no stemming applied';

-- ============================================================================
-- PAPERS TABLE
-- Global cache of papers, tenant-agnostic
-- ============================================================================

CREATE TABLE papers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Canonical identifier derived from best available external ID
    -- Format: "doi:10.1234/..." or "arxiv:2301.00001" or "pmid:12345678" etc.
    canonical_id TEXT NOT NULL,
    
    -- Paper metadata
    title TEXT NOT NULL,
    abstract TEXT,
    authors JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- authors format: [{"name": "...", "affiliation": "...", "orcid": "..."}]
    
    publication_date DATE,
    publication_year INTEGER,
    venue TEXT,
    journal TEXT,
    volume TEXT,
    issue TEXT,
    pages TEXT,
    
    -- Citation metrics
    citation_count INTEGER,
    reference_count INTEGER,
    
    -- Access information
    pdf_url TEXT,
    open_access BOOLEAN DEFAULT FALSE,
    
    -- Discovery tracking (when we first found this paper)
    first_discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Keyword extraction status (global, done once per paper)
    keywords_extracted BOOLEAN NOT NULL DEFAULT FALSE,
    keywords_extracted_at TIMESTAMPTZ,
    
    -- Raw API response for debugging/reprocessing
    raw_metadata JSONB,
    
    CONSTRAINT uq_papers_canonical_id UNIQUE (canonical_id)
);

-- Indexes
CREATE INDEX idx_papers_canonical_id ON papers (canonical_id);
CREATE INDEX idx_papers_title_trgm ON papers USING gin (title gin_trgm_ops);
CREATE INDEX idx_papers_publication_date ON papers (publication_date DESC NULLS LAST);
CREATE INDEX idx_papers_publication_year ON papers (publication_year DESC NULLS LAST);
CREATE INDEX idx_papers_keywords_not_extracted ON papers (id) 
    WHERE keywords_extracted = FALSE;
CREATE INDEX idx_papers_created_at ON papers (first_discovered_at DESC);

COMMENT ON TABLE papers IS 'Global cache of academic papers, deduplicated by canonical_id';
COMMENT ON COLUMN papers.canonical_id IS 'Canonical identifier: doi:X, arxiv:X, pmid:X, ss:X, oalex:X, scopus:X';
COMMENT ON COLUMN papers.authors IS 'JSON array: [{"name": "...", "affiliation": "...", "orcid": "..."}]';

-- ============================================================================
-- PAPER IDENTIFIERS TABLE
-- Maps external identifiers to papers (handles cross-source identity)
-- ============================================================================

CREATE TABLE paper_identifiers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    
    -- Identifier type and value
    identifier_type identifier_type NOT NULL,
    identifier_value TEXT NOT NULL,
    
    -- Which API provided this identifier
    source_api TEXT NOT NULL,
    
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Each identifier value is globally unique per type
    -- e.g., there's only one "doi:10.1234/abc"
    CONSTRAINT uq_paper_identifiers_type_value UNIQUE (identifier_type, identifier_value)
);

-- Indexes
CREATE INDEX idx_paper_identifiers_paper_id ON paper_identifiers (paper_id);
CREATE INDEX idx_paper_identifiers_lookup ON paper_identifiers (identifier_type, identifier_value);
CREATE INDEX idx_paper_identifiers_source ON paper_identifiers (source_api);

COMMENT ON TABLE paper_identifiers IS 'Maps external identifiers (DOI, arXiv ID, etc.) to papers';
COMMENT ON CONSTRAINT uq_paper_identifiers_type_value ON paper_identifiers IS 
    'Each identifier value is globally unique per type - prevents duplicate paper creation';

-- ============================================================================
-- PAPER SOURCES TABLE
-- Tracks which sources have observed each paper
-- ============================================================================

CREATE TABLE paper_sources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    source_api TEXT NOT NULL,
    
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Source-specific metadata (API response fields we might want)
    source_metadata JSONB,
    
    CONSTRAINT uq_paper_sources_paper_source UNIQUE (paper_id, source_api)
);

-- Indexes
CREATE INDEX idx_paper_sources_paper_id ON paper_sources (paper_id);
CREATE INDEX idx_paper_sources_source_api ON paper_sources (source_api);

COMMENT ON TABLE paper_sources IS 'Tracks all API sources that have returned this paper';

-- ============================================================================
-- KEYWORD SEARCHES TABLE
-- Tracks when we searched each keyword in each source
-- ============================================================================

CREATE TABLE keyword_searches (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    keyword_id UUID NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    source_api TEXT NOT NULL,
    
    -- When the search was executed
    searched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Search window (NULL from_date means open-ended start)
    search_from_date DATE,
    search_to_date DATE NOT NULL,
    
    -- Hash of search parameters for idempotency
    -- Computed from: keyword_id + source_api + from_date + to_date
    search_window_hash TEXT NOT NULL,
    
    -- Results
    papers_found INTEGER NOT NULL DEFAULT 0,
    status search_status NOT NULL DEFAULT 'completed',
    error_message TEXT,
    
    -- Unique on the hash to prevent duplicate searches
    CONSTRAINT uq_keyword_searches_window UNIQUE (keyword_id, source_api, search_window_hash)
);

-- Indexes
CREATE INDEX idx_keyword_searches_keyword_id ON keyword_searches (keyword_id);
CREATE INDEX idx_keyword_searches_searched_at ON keyword_searches (searched_at DESC);
CREATE INDEX idx_keyword_searches_lookup ON keyword_searches (keyword_id, source_api, search_to_date DESC);
CREATE INDEX idx_keyword_searches_status ON keyword_searches (status) WHERE status != 'completed';

COMMENT ON TABLE keyword_searches IS 'Records each search execution for incremental searching';
COMMENT ON COLUMN keyword_searches.search_window_hash IS 
    'SHA256 hash of keyword_id|source_api|from_date|to_date for idempotency';

-- ============================================================================
-- KEYWORD-PAPER MAPPINGS TABLE
-- Many-to-many relationship with source tracking
-- ============================================================================

CREATE TABLE keyword_paper_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    keyword_id UUID NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    
    -- How was this mapping created?
    -- 'search_result' = paper was found when searching for this keyword
    -- 'extracted' = keyword was extracted from this paper via LLM
    mapping_type mapping_type NOT NULL,
    
    -- Where did the keyword come from originally?
    -- 'user_query' = extracted from user's natural language query
    -- 'paper_extraction' = extracted from a paper's title/abstract
    source_type source_type NOT NULL,
    
    -- LLM confidence score (for extracted keywords)
    confidence_score FLOAT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- A keyword-paper pair can have both search_result and extracted mappings
    CONSTRAINT uq_keyword_paper_mappings UNIQUE (keyword_id, paper_id, mapping_type)
);

-- Indexes
CREATE INDEX idx_kpm_keyword_id ON keyword_paper_mappings (keyword_id);
CREATE INDEX idx_kpm_paper_id ON keyword_paper_mappings (paper_id);
CREATE INDEX idx_kpm_mapping_type ON keyword_paper_mappings (mapping_type);
CREATE INDEX idx_kpm_source_type ON keyword_paper_mappings (source_type);

COMMENT ON TABLE keyword_paper_mappings IS 
    'Many-to-many: keywords found in papers or papers found by searching keywords';

-- ============================================================================
-- LITERATURE REVIEW REQUESTS TABLE
-- Tenant-scoped job tracking
-- ============================================================================

CREATE TABLE literature_review_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Tenant context (required)
    org_id UUID NOT NULL,
    project_id UUID NOT NULL,
    user_id TEXT NOT NULL,
    
    -- Request details
    original_query TEXT NOT NULL,
    
    -- Temporal workflow tracking
    temporal_workflow_id TEXT NOT NULL,
    temporal_run_id TEXT,
    
    -- Status
    status review_status NOT NULL DEFAULT 'pending',
    
    -- Progress tracking
    initial_keywords_count INTEGER,
    papers_found_count INTEGER DEFAULT 0,
    papers_ingested_count INTEGER DEFAULT 0,
    current_expansion_depth INTEGER DEFAULT 0,
    max_expansion_depth INTEGER NOT NULL,
    
    -- Timing
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    
    -- Error handling
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    
    -- Configuration snapshot (exact settings used)
    config_snapshot JSONB NOT NULL,
    
    -- Explicit filters used
    source_filters TEXT[] NOT NULL DEFAULT '{}',
    date_from DATE,
    date_to DATE
);

-- Indexes
CREATE INDEX idx_lrr_tenant ON literature_review_requests (org_id, project_id);
CREATE INDEX idx_lrr_org_user ON literature_review_requests (org_id, user_id);
CREATE INDEX idx_lrr_status ON literature_review_requests (status);
CREATE INDEX idx_lrr_workflow_id ON literature_review_requests (temporal_workflow_id);
CREATE INDEX idx_lrr_created_at ON literature_review_requests (created_at DESC);
CREATE INDEX idx_lrr_active ON literature_review_requests (org_id, project_id, status) 
    WHERE status NOT IN ('completed', 'failed', 'cancelled');

COMMENT ON TABLE literature_review_requests IS 'Tenant-scoped literature review job tracking';
COMMENT ON COLUMN literature_review_requests.config_snapshot IS 
    'Complete configuration used for this request (for reproducibility)';

-- ============================================================================
-- REQUEST-KEYWORD MAPPINGS TABLE
-- Links requests to keywords used/discovered
-- ============================================================================

CREATE TABLE request_keyword_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    request_id UUID NOT NULL REFERENCES literature_review_requests(id) ON DELETE CASCADE,
    keyword_id UUID NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    
    -- Which expansion round discovered this keyword
    -- 0 = initial extraction from user query
    -- 1+ = extraction from papers found in previous rounds
    extraction_round INTEGER NOT NULL DEFAULT 0,
    
    -- If extracted from a paper, which paper?
    source_paper_id UUID REFERENCES papers(id) ON DELETE SET NULL,
    
    -- Source type for this specific mapping
    source_type source_type NOT NULL,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Each request uses each keyword at most once
    CONSTRAINT uq_request_keyword UNIQUE (request_id, keyword_id)
);

-- Indexes
CREATE INDEX idx_rkm_request_id ON request_keyword_mappings (request_id);
CREATE INDEX idx_rkm_keyword_id ON request_keyword_mappings (keyword_id);
CREATE INDEX idx_rkm_round ON request_keyword_mappings (request_id, extraction_round);

COMMENT ON TABLE request_keyword_mappings IS 'Keywords used or discovered in a literature review request';

-- ============================================================================
-- REQUEST-PAPER MAPPINGS TABLE
-- Tenant-scoped tracking of papers discovered in each request
-- ============================================================================

CREATE TABLE request_paper_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    request_id UUID NOT NULL REFERENCES literature_review_requests(id) ON DELETE CASCADE,
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    
    -- Discovery context
    discovered_via_keyword_id UUID REFERENCES keywords(id) ON DELETE SET NULL,
    discovered_via_source TEXT NOT NULL,
    expansion_depth INTEGER NOT NULL DEFAULT 0,
    
    -- Per-request ingestion status (tenant-scoped, not global)
    ingestion_status ingestion_status NOT NULL DEFAULT 'pending',
    ingestion_job_id TEXT,  -- String to match ingestion service proto
    ingestion_error TEXT,
    ingested_at TIMESTAMPTZ,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Each request discovers each paper at most once
    CONSTRAINT uq_request_paper UNIQUE (request_id, paper_id)
);

-- Indexes
CREATE INDEX idx_rpm_request_id ON request_paper_mappings (request_id);
CREATE INDEX idx_rpm_paper_id ON request_paper_mappings (paper_id);
CREATE INDEX idx_rpm_ingestion_pending ON request_paper_mappings (request_id, ingestion_status) 
    WHERE ingestion_status IN ('pending', 'queued');
CREATE INDEX idx_rpm_depth ON request_paper_mappings (request_id, expansion_depth);

COMMENT ON TABLE request_paper_mappings IS 
    'Papers discovered in each request with per-request ingestion tracking';

-- ============================================================================
-- REVIEW PROGRESS EVENTS TABLE
-- For real-time streaming via LISTEN/NOTIFY
-- ============================================================================

CREATE TABLE review_progress_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    request_id UUID NOT NULL REFERENCES literature_review_requests(id) ON DELETE CASCADE,
    
    event_type TEXT NOT NULL,
    -- Event types: 'keywords_extracted', 'search_started', 'papers_found',
    --              'expansion_started', 'ingestion_started', 'completed', 'failed'
    
    event_data JSONB NOT NULL,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_rpe_request_id ON review_progress_events (request_id, created_at DESC);
CREATE INDEX idx_rpe_created_at ON review_progress_events (created_at DESC);

-- Cleanup old events (keep last 24 hours)
CREATE INDEX idx_rpe_cleanup ON review_progress_events (created_at) 
    WHERE created_at < NOW() - INTERVAL '24 hours';

COMMENT ON TABLE review_progress_events IS 
    'Progress events for real-time streaming via PostgreSQL NOTIFY';

-- ============================================================================
-- TRIGGER: Notify on progress event insert
-- ============================================================================

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

CREATE TRIGGER trg_review_progress_notify
    AFTER INSERT ON review_progress_events
    FOR EACH ROW
    EXECUTE FUNCTION notify_review_progress();

COMMENT ON FUNCTION notify_review_progress() IS 
    'Sends PostgreSQL NOTIFY on progress event insert for real-time streaming';

-- ============================================================================
-- OUTBOX EVENTS TABLE
-- Transactional outbox for Kafka event publishing (shared package schema)
-- ============================================================================

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Event classification
    aggregate_type TEXT NOT NULL,
    -- e.g., 'literature_review', 'paper', 'keyword'
    
    aggregate_id TEXT NOT NULL,
    -- e.g., request_id, paper_id
    
    event_type TEXT NOT NULL,
    -- e.g., 'review.started', 'review.completed', 'papers.discovered'
    
    -- Tenant context for routing/filtering
    org_id UUID NOT NULL,
    project_id UUID,
    
    -- Event payload
    payload JSONB NOT NULL,
    
    -- Metadata (trace_id, span_id, correlation_id, etc.)
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    
    -- Processing state
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,  -- NULL = not yet published
    publish_attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    
    -- Ordering guarantee within aggregate
    sequence_num BIGSERIAL NOT NULL
);

-- Indexes for outbox worker
CREATE INDEX idx_outbox_unpublished ON outbox_events (sequence_num) 
    WHERE published_at IS NULL;
CREATE INDEX idx_outbox_aggregate ON outbox_events (aggregate_type, aggregate_id);
CREATE INDEX idx_outbox_created_at ON outbox_events (created_at);
CREATE INDEX idx_outbox_cleanup ON outbox_events (published_at) 
    WHERE published_at IS NOT NULL AND published_at < NOW() - INTERVAL '7 days';

COMMENT ON TABLE outbox_events IS 
    'Transactional outbox for reliable Kafka event publishing';
COMMENT ON COLUMN outbox_events.published_at IS 
    'NULL means not yet published; set by outbox worker after successful Kafka send';

-- ============================================================================
-- HELPER FUNCTIONS
-- ============================================================================

-- Generate canonical ID from identifiers (priority order)
CREATE OR REPLACE FUNCTION generate_canonical_id(
    p_doi TEXT DEFAULT NULL,
    p_arxiv_id TEXT DEFAULT NULL,
    p_pubmed_id TEXT DEFAULT NULL,
    p_semantic_scholar_id TEXT DEFAULT NULL,
    p_openalex_id TEXT DEFAULT NULL,
    p_scopus_id TEXT DEFAULT NULL
) RETURNS TEXT AS $$
BEGIN
    -- Priority: DOI > arXiv > PubMed > Semantic Scholar > OpenAlex > Scopus
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

COMMENT ON FUNCTION generate_canonical_id IS 
    'Generates canonical paper ID from available identifiers with priority ordering';

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

COMMENT ON FUNCTION generate_search_window_hash IS 
    'Generates deterministic hash for keyword search window to ensure idempotency';

-- Get or create keyword (idempotent)
CREATE OR REPLACE FUNCTION get_or_create_keyword(p_keyword TEXT)
RETURNS UUID AS $$
DECLARE
    v_normalized TEXT;
    v_id UUID;
BEGIN
    -- Normalize: lowercase, trim, collapse whitespace
    v_normalized := lower(trim(regexp_replace(p_keyword, '\s+', ' ', 'g')));
    
    -- Upsert
    INSERT INTO keywords (keyword, normalized_keyword)
    VALUES (trim(p_keyword), v_normalized)
    ON CONFLICT (normalized_keyword) 
    DO UPDATE SET keyword = keywords.keyword  -- No-op, just to return id
    RETURNING id INTO v_id;
    
    RETURN v_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION get_or_create_keyword IS 
    'Idempotently creates or retrieves a keyword by its normalized form';

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

COMMENT ON FUNCTION find_paper_by_identifier IS 
    'Finds paper_id by external identifier, returns NULL if not found';

-- Upsert paper with full identifier resolution (handles concurrent inserts)
CREATE OR REPLACE FUNCTION upsert_paper(
    p_identifiers JSONB,
    -- Format: [{"type": "doi", "value": "10.1234/...", "source": "semantic_scholar"}, ...]
    p_title TEXT,
    p_abstract TEXT DEFAULT NULL,
    p_authors JSONB DEFAULT '[]'::jsonb,
    p_publication_date DATE DEFAULT NULL,
    p_venue TEXT DEFAULT NULL,
    p_journal TEXT DEFAULT NULL,
    p_citation_count INTEGER DEFAULT NULL,
    p_pdf_url TEXT DEFAULT NULL,
    p_open_access BOOLEAN DEFAULT FALSE,
    p_source_api TEXT DEFAULT NULL,
    p_raw_metadata JSONB DEFAULT NULL
) RETURNS UUID AS $$
DECLARE
    v_paper_id UUID;
    v_canonical_id TEXT;
    v_identifier JSONB;
    v_found_paper_id UUID;
    v_best_type TEXT;
    v_best_value TEXT;
    v_type_priority INTEGER;
    v_best_priority INTEGER := 999;
BEGIN
    -- Step 1: Try to find existing paper by any provided identifier
    FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
    LOOP
        v_found_paper_id := find_paper_by_identifier(
            (v_identifier->>'type')::identifier_type,
            v_identifier->>'value'
        );
        
        IF v_found_paper_id IS NOT NULL THEN
            v_paper_id := v_found_paper_id;
            EXIT;
        END IF;
    END LOOP;
    
    -- Step 2: If found, update with any new information
    IF v_paper_id IS NOT NULL THEN
        UPDATE papers SET
            abstract = COALESCE(papers.abstract, p_abstract),
            authors = CASE 
                WHEN jsonb_array_length(papers.authors) < jsonb_array_length(p_authors) 
                THEN p_authors 
                ELSE papers.authors 
            END,
            publication_date = COALESCE(papers.publication_date, p_publication_date),
            publication_year = COALESCE(papers.publication_year, EXTRACT(YEAR FROM p_publication_date)::INTEGER),
            venue = COALESCE(papers.venue, p_venue),
            journal = COALESCE(papers.journal, p_journal),
            citation_count = GREATEST(papers.citation_count, p_citation_count),
            pdf_url = COALESCE(papers.pdf_url, p_pdf_url),
            open_access = papers.open_access OR p_open_access,
            last_updated_at = NOW()
        WHERE id = v_paper_id;
        
        -- Add any new identifiers
        FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
        LOOP
            INSERT INTO paper_identifiers (paper_id, identifier_type, identifier_value, source_api)
            VALUES (
                v_paper_id,
                (v_identifier->>'type')::identifier_type,
                lower(trim(v_identifier->>'value')),
                COALESCE(v_identifier->>'source', p_source_api)
            )
            ON CONFLICT (identifier_type, identifier_value) DO NOTHING;
        END LOOP;
        
        -- Update paper sources
        IF p_source_api IS NOT NULL THEN
            INSERT INTO paper_sources (paper_id, source_api, source_metadata)
            VALUES (v_paper_id, p_source_api, p_raw_metadata)
            ON CONFLICT (paper_id, source_api) DO UPDATE SET
                last_seen_at = NOW(),
                source_metadata = COALESCE(paper_sources.source_metadata, EXCLUDED.source_metadata);
        END IF;
        
        RETURN v_paper_id;
    END IF;
    
    -- Step 3: Paper doesn't exist, find best identifier for canonical ID
    FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
    LOOP
        v_type_priority := CASE (v_identifier->>'type')
            WHEN 'doi' THEN 1
            WHEN 'arxiv_id' THEN 2
            WHEN 'pubmed_id' THEN 3
            WHEN 'pmcid' THEN 4
            WHEN 'semantic_scholar_id' THEN 5
            WHEN 'openalex_id' THEN 6
            WHEN 'scopus_id' THEN 7
            ELSE 99
        END;
        
        IF v_type_priority < v_best_priority THEN
            v_best_priority := v_type_priority;
            v_best_type := v_identifier->>'type';
            v_best_value := v_identifier->>'value';
        END IF;
    END LOOP;
    
    IF v_best_type IS NULL THEN
        RAISE EXCEPTION 'At least one valid identifier is required';
    END IF;
    
    -- Generate canonical ID
    v_canonical_id := CASE v_best_type
        WHEN 'doi' THEN 'doi:' || lower(trim(v_best_value))
        WHEN 'arxiv_id' THEN 'arxiv:' || lower(trim(v_best_value))
        WHEN 'pubmed_id' THEN 'pmid:' || trim(v_best_value)
        WHEN 'pmcid' THEN 'pmcid:' || trim(v_best_value)
        WHEN 'semantic_scholar_id' THEN 'ss:' || trim(v_best_value)
        WHEN 'openalex_id' THEN 'oalex:' || trim(v_best_value)
        WHEN 'scopus_id' THEN 'scopus:' || trim(v_best_value)
    END;
    
    -- Step 4: Insert new paper (handle race condition)
    BEGIN
        INSERT INTO papers (
            canonical_id, title, abstract, authors,
            publication_date, publication_year, venue, journal,
            citation_count, pdf_url, open_access, raw_metadata
        ) VALUES (
            v_canonical_id, p_title, p_abstract, p_authors,
            p_publication_date, EXTRACT(YEAR FROM p_publication_date)::INTEGER,
            p_venue, p_journal, p_citation_count, p_pdf_url, p_open_access,
            p_raw_metadata
        )
        RETURNING id INTO v_paper_id;
    EXCEPTION WHEN unique_violation THEN
        -- Another worker created it, fetch the ID
        SELECT id INTO v_paper_id FROM papers WHERE canonical_id = v_canonical_id;
        IF v_paper_id IS NULL THEN
            RAISE EXCEPTION 'Concurrent insert failed and paper not found: %', v_canonical_id;
        END IF;
    END;
    
    -- Step 5: Add all identifiers
    FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
    LOOP
        INSERT INTO paper_identifiers (paper_id, identifier_type, identifier_value, source_api)
        VALUES (
            v_paper_id,
            (v_identifier->>'type')::identifier_type,
            lower(trim(v_identifier->>'value')),
            COALESCE(v_identifier->>'source', p_source_api)
        )
        ON CONFLICT (identifier_type, identifier_value) DO NOTHING;
    END LOOP;
    
    -- Step 6: Record paper source
    IF p_source_api IS NOT NULL THEN
        INSERT INTO paper_sources (paper_id, source_api, source_metadata)
        VALUES (v_paper_id, p_source_api, p_raw_metadata)
        ON CONFLICT (paper_id, source_api) DO NOTHING;
    END IF;
    
    RETURN v_paper_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION upsert_paper IS 
    'Idempotently creates or updates a paper with full identifier resolution and race condition handling';

-- Record keyword search (idempotent)
CREATE OR REPLACE FUNCTION record_keyword_search(
    p_keyword_id UUID,
    p_source_api TEXT,
    p_from_date DATE,
    p_to_date DATE,
    p_papers_found INTEGER,
    p_status search_status DEFAULT 'completed'
) RETURNS UUID AS $$
DECLARE
    v_hash TEXT;
    v_id UUID;
BEGIN
    v_hash := generate_search_window_hash(p_keyword_id, p_source_api, p_from_date, p_to_date);
    
    INSERT INTO keyword_searches (
        keyword_id, source_api, search_from_date, search_to_date, 
        search_window_hash, papers_found, status
    )
    VALUES (p_keyword_id, p_source_api, p_from_date, p_to_date, v_hash, p_papers_found, p_status)
    ON CONFLICT (keyword_id, source_api, search_window_hash) DO UPDATE SET
        papers_found = keyword_searches.papers_found + EXCLUDED.papers_found,
        searched_at = NOW(),
        status = EXCLUDED.status
    RETURNING id INTO v_id;
    
    RETURN v_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION record_keyword_search IS 
    'Records a keyword search execution, accumulating paper counts for repeated searches';

-- Get last search date for incremental searching
CREATE OR REPLACE FUNCTION get_last_search_date(
    p_keyword_id UUID,
    p_source_api TEXT
) RETURNS DATE AS $$
DECLARE
    v_last_date DATE;
BEGIN
    SELECT MAX(search_to_date) INTO v_last_date
    FROM keyword_searches
    WHERE keyword_id = p_keyword_id
      AND source_api = p_source_api
      AND status = 'completed';
    
    RETURN v_last_date;
END;
$$ LANGUAGE plpgsql STABLE;

COMMENT ON FUNCTION get_last_search_date IS 
    'Returns the last successful search date for incremental searching';

-- ============================================================================
-- VIEWS
-- ============================================================================

-- Pending keyword searches (for determining what needs searching)
CREATE OR REPLACE VIEW v_pending_keyword_searches AS
SELECT 
    k.id AS keyword_id,
    k.normalized_keyword,
    s.source_api,
    get_last_search_date(k.id, s.source_api) AS last_search_date
FROM keywords k
CROSS JOIN (
    VALUES 
        ('semantic_scholar'), 
        ('openalex'), 
        ('scopus'), 
        ('pubmed'), 
        ('biorxiv'), 
        ('arxiv')
) AS s(source_api);

COMMENT ON VIEW v_pending_keyword_searches IS 
    'Shows all keyword-source combinations with their last search date for incremental searching';

-- Review progress summary
CREATE OR REPLACE VIEW v_review_progress AS
SELECT 
    r.id AS review_id,
    r.org_id,
    r.project_id,
    r.user_id,
    r.status,
    r.original_query,
    r.initial_keywords_count,
    r.current_expansion_depth,
    r.max_expansion_depth,
    COUNT(DISTINCT rpm.paper_id) AS papers_found,
    COUNT(DISTINCT rpm.paper_id) FILTER (WHERE rpm.ingestion_status = 'completed') AS papers_ingested,
    COUNT(DISTINCT rkm.keyword_id) AS keywords_used,
    r.created_at,
    r.started_at,
    r.completed_at,
    EXTRACT(EPOCH FROM (COALESCE(r.completed_at, NOW()) - r.started_at)) AS duration_seconds
FROM literature_review_requests r
LEFT JOIN request_paper_mappings rpm ON r.id = rpm.request_id
LEFT JOIN request_keyword_mappings rkm ON r.id = rkm.request_id
GROUP BY r.id;

COMMENT ON VIEW v_review_progress IS 
    'Aggregated progress view for literature review requests';

-- ============================================================================
-- MAINTENANCE FUNCTIONS
-- ============================================================================

-- Cleanup old progress events (call periodically)
CREATE OR REPLACE FUNCTION cleanup_old_progress_events(p_retention_hours INTEGER DEFAULT 24)
RETURNS INTEGER AS $$
DECLARE
    v_deleted INTEGER;
BEGIN
    DELETE FROM review_progress_events
    WHERE created_at < NOW() - (p_retention_hours || ' hours')::INTERVAL;
    
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RETURN v_deleted;
END;
$$ LANGUAGE plpgsql;

-- Cleanup old published outbox events (call periodically)
CREATE OR REPLACE FUNCTION cleanup_old_outbox_events(p_retention_hours INTEGER DEFAULT 168)
RETURNS INTEGER AS $$
DECLARE
    v_deleted INTEGER;
BEGIN
    DELETE FROM outbox_events
    WHERE published_at IS NOT NULL
      AND published_at < NOW() - (p_retention_hours || ' hours')::INTERVAL;
    
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RETURN v_deleted;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- GRANTS (adjust role names as needed)
-- ============================================================================

-- Application role
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO literature_review_app;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO literature_review_app;
-- GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO literature_review_app;

-- Read-only role for reporting
-- GRANT SELECT ON ALL TABLES IN SCHEMA public TO literature_review_readonly;
-- GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO literature_review_readonly;
```

---

# 5. Protocol Buffer Definitions

## 5.1 Literature Review Service Proto

```protobuf
// api/proto/literaturereview/v1/literature_review.proto

syntax = "proto3";

package literaturereview.v1;

option go_package = "github.com/company/literature-review-service/gen/proto/literaturereview/v1;literaturereviewv1";

import "google/protobuf/timestamp.proto";
import "google/protobuf/duration.proto";
import "google/protobuf/wrappers.proto";

// ============================================================================
// SERVICE DEFINITION
// ============================================================================

service LiteratureReviewService {
  // Start a new literature review from a natural language query
  rpc StartLiteratureReview(StartLiteratureReviewRequest) returns (StartLiteratureReviewResponse);
  
  // Get the current status of a literature review
  rpc GetLiteratureReviewStatus(GetLiteratureReviewStatusRequest) returns (GetLiteratureReviewStatusResponse);
  
  // Cancel a running literature review
  rpc CancelLiteratureReview(CancelLiteratureReviewRequest) returns (CancelLiteratureReviewResponse);
  
  // List literature reviews for a project
  rpc ListLiteratureReviews(ListLiteratureReviewsRequest) returns (ListLiteratureReviewsResponse);
  
  // Get papers discovered in a literature review
  rpc GetLiteratureReviewPapers(GetLiteratureReviewPapersRequest) returns (GetLiteratureReviewPapersResponse);
  
  // Get keywords used/discovered in a literature review
  rpc GetLiteratureReviewKeywords(GetLiteratureReviewKeywordsRequest) returns (GetLiteratureReviewKeywordsResponse);
  
  // Stream real-time progress updates
  rpc StreamLiteratureReviewProgress(StreamLiteratureReviewProgressRequest) returns (stream LiteratureReviewProgressEvent);
}

// ============================================================================
// START LITERATURE REVIEW
// ============================================================================

message StartLiteratureReviewRequest {
  // Tenant context (required)
  string org_id = 1;
  string project_id = 2;
  // user_id extracted from JWT by grpcauth middleware
  
  // Natural language query describing the research topic
  string query = 3;
  
  // Override default configuration (optional)
  google.protobuf.Int32Value initial_keyword_count = 4;  // Default: 5
  google.protobuf.Int32Value paper_keyword_count = 5;    // Default: 3
  google.protobuf.Int32Value max_expansion_depth = 6;    // Default: 2
  
  // Filter to specific sources (empty = all enabled sources)
  repeated string source_filters = 7;
  
  // Date range filter for papers (optional)
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

// ============================================================================
// GET STATUS
// ============================================================================

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

// ============================================================================
// CANCEL REVIEW
// ============================================================================

message CancelLiteratureReviewRequest {
  string org_id = 1;
  string project_id = 2;
  string review_id = 3;
  string reason = 4;  // Optional cancellation reason
}

message CancelLiteratureReviewResponse {
  bool success = 1;
  string message = 2;
  ReviewStatus final_status = 3;
}

// ============================================================================
// LIST REVIEWS
// ============================================================================

message ListLiteratureReviewsRequest {
  string org_id = 1;
  string project_id = 2;
  
  // Pagination
  int32 page_size = 3;          // Default: 20, Max: 100
  string page_token = 4;        // Opaque, signed token
  
  // Filters
  ReviewStatus status_filter = 5;
  google.protobuf.Timestamp created_after = 6;
  google.protobuf.Timestamp created_before = 7;
  string user_id_filter = 8;    // Filter by creator (admin only)
  
  // Sorting
  ListSortField sort_field = 9;
  SortOrder sort_order = 10;
}

message ListLiteratureReviewsResponse {
  repeated LiteratureReviewSummary reviews = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}

// ============================================================================
// GET PAPERS
// ============================================================================

message GetLiteratureReviewPapersRequest {
  string org_id = 1;
  string project_id = 2;
  string review_id = 3;
  
  // Pagination
  int32 page_size = 4;
  string page_token = 5;
  
  // Filters
  IngestionStatus ingestion_status_filter = 6;
  string source_filter = 7;
  int32 expansion_depth_filter = 8;
  
  // Sorting
  PaperSortField sort_field = 9;
  SortOrder sort_order = 10;
}

message GetLiteratureReviewPapersResponse {
  repeated Paper papers = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}

// ============================================================================
// GET KEYWORDS
// ============================================================================

message GetLiteratureReviewKeywordsRequest {
  string org_id = 1;
  string project_id = 2;
  string review_id = 3;
  
  // Pagination
  int32 page_size = 4;
  string page_token = 5;
  
  // Filters
  int32 extraction_round_filter = 6;
  KeywordSourceType source_type_filter = 7;
}

message GetLiteratureReviewKeywordsResponse {
  repeated ReviewKeyword keywords = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}

// ============================================================================
// STREAM PROGRESS
// ============================================================================

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
  
  // Event-specific data
  oneof event_data {
    KeywordsExtractedEvent keywords_extracted = 10;
    PapersFoundEvent papers_found = 11;
    ExpansionStartedEvent expansion_started = 12;
    IngestionProgressEvent ingestion_progress = 13;
    ErrorEvent error = 14;
  }
}

// ============================================================================
// ENUMS
// ============================================================================

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

enum ListSortField {
  LIST_SORT_FIELD_UNSPECIFIED = 0;
  LIST_SORT_FIELD_CREATED_AT = 1;
  LIST_SORT_FIELD_COMPLETED_AT = 2;
  LIST_SORT_FIELD_PAPERS_FOUND = 3;
}

enum PaperSortField {
  PAPER_SORT_FIELD_UNSPECIFIED = 0;
  PAPER_SORT_FIELD_DISCOVERED_AT = 1;
  PAPER_SORT_FIELD_PUBLICATION_DATE = 2;
  PAPER_SORT_FIELD_CITATION_COUNT = 3;
  PAPER_SORT_FIELD_TITLE = 4;
}

enum SortOrder {
  SORT_ORDER_UNSPECIFIED = 0;
  SORT_ORDER_ASC = 1;
  SORT_ORDER_DESC = 2;
}

// ============================================================================
// DOMAIN MESSAGES
// ============================================================================

message ReviewProgress {
  // Keywords
  int32 initial_keywords_count = 1;
  int32 total_keywords_processed = 2;
  
  // Papers
  int32 papers_found = 3;
  int32 papers_new = 4;           // Papers not seen before
  int32 papers_ingested = 5;
  int32 papers_failed = 6;
  
  // Expansion
  int32 current_expansion_depth = 7;
  int32 max_expansion_depth = 8;
  
  // Per-source progress
  map<string, SourceProgress> source_progress = 9;
  
  // Timing
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
  
  // Identifiers
  string doi = 2;
  string arxiv_id = 3;
  string pubmed_id = 4;
  string semantic_scholar_id = 5;
  string openalex_id = 6;
  
  // Metadata
  string title = 7;
  string abstract = 8;
  repeated Author authors = 9;
  google.protobuf.Timestamp publication_date = 10;
  int32 publication_year = 11;
  string venue = 12;
  string journal = 13;
  
  // Metrics
  int32 citation_count = 14;
  
  // Access
  string pdf_url = 15;
  bool open_access = 16;
  
  // Discovery context (for this review)
  string discovered_via_source = 17;
  string discovered_via_keyword = 18;
  int32 expansion_depth = 19;
  
  // Ingestion status (for this review)
  IngestionStatus ingestion_status = 20;
  string ingestion_job_id = 21;
  
  // Keywords extracted from this paper
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
  string source_paper_id = 6;      // If extracted from a paper
  string source_paper_title = 7;   // Title for display
  int32 papers_found = 8;          // Papers found using this keyword
  float confidence_score = 9;      // LLM confidence (for extracted)
}

// ============================================================================
// EVENT-SPECIFIC MESSAGES
// ============================================================================

message KeywordsExtractedEvent {
  repeated string keywords = 1;
  int32 extraction_round = 2;
  string source_paper_id = 3;
}

message PapersFoundEvent {
  string source = 1;
  string keyword = 2;
  int32 count = 3;
  int32 new_count = 4;  // Papers not seen before
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

## 5.2 Ingestion Service Proto (Copied from Ingestion Service)

```protobuf
// api/proto/ingestion/v1/ingestion.proto
// COPIED FROM INGESTION SERVICE - DO NOT MODIFY LOCALLY

syntax = "proto3";

package ingestion.v1;

option go_package = "github.com/company/literature-review-service/gen/proto/ingestion/v1;ingestionv1";

import "google/protobuf/timestamp.proto";

// ============================================================================
// SERVICE DEFINITION
// ============================================================================

service IngestionService {
  // Ingest a single paper
  rpc IngestPaper(IngestPaperRequest) returns (IngestPaperResponse);
  
  // Batch ingest multiple papers
  rpc BatchIngestPapers(BatchIngestPapersRequest) returns (BatchIngestPapersResponse);
  
  // Get ingestion job status
  rpc GetIngestionStatus(GetIngestionStatusRequest) returns (GetIngestionStatusResponse);
  
  // Get multiple job statuses
  rpc BatchGetIngestionStatus(BatchGetIngestionStatusRequest) returns (BatchGetIngestionStatusResponse);
}

// ============================================================================
// INGEST PAPER
// ============================================================================

message IngestPaperRequest {
  // Unique paper identifier (for idempotency)
  string paper_id = 1;
  
  // External identifiers
  string doi = 2;
  string arxiv_id = 3;
  string pubmed_id = 4;
  
  // Content URLs
  string pdf_url = 5;
  string html_url = 6;
  
  // Metadata
  string title = 7;
  string abstract = 8;
  
  // Additional metadata as key-value pairs
  map<string, string> metadata = 9;
  
  // Source service for tracking
  string source_service = 10;
  
  // Callback information for async completion
  string callback_workflow_id = 11;
  string callback_signal_name = 12;
}

message IngestPaperResponse {
  string job_id = 1;
  string status = 2;  // accepted, rejected, duplicate
  string message = 3;
}

// ============================================================================
// BATCH INGEST
// ============================================================================

message BatchIngestPapersRequest {
  repeated IngestPaperRequest papers = 1;
  int32 priority = 2;  // 1-10, higher = more urgent
}

message BatchIngestPapersResponse {
  repeated IngestJobStatus jobs = 1;
  int32 accepted_count = 2;
  int32 rejected_count = 3;
  int32 duplicate_count = 4;
}

message IngestJobStatus {
  string paper_id = 1;
  string job_id = 2;
  string status = 3;  // accepted, rejected, duplicate
  string message = 4;
}

// ============================================================================
// GET STATUS
// ============================================================================

message GetIngestionStatusRequest {
  string job_id = 1;
}

message GetIngestionStatusResponse {
  string job_id = 1;
  string paper_id = 2;
  IngestionJobStatus status = 3;
  string error_message = 4;
  google.protobuf.Timestamp created_at = 5;
  google.protobuf.Timestamp started_at = 6;
  google.protobuf.Timestamp completed_at = 7;
  IngestionMetrics metrics = 8;
}

message BatchGetIngestionStatusRequest {
  repeated string job_ids = 1;
}

message BatchGetIngestionStatusResponse {
  repeated GetIngestionStatusResponse statuses = 1;
}

// ============================================================================
// ENUMS & SUPPORTING MESSAGES
// ============================================================================

enum IngestionJobStatus {
  INGESTION_JOB_STATUS_UNSPECIFIED = 0;
  INGESTION_JOB_STATUS_PENDING = 1;
  INGESTION_JOB_STATUS_DOWNLOADING = 2;
  INGESTION_JOB_STATUS_PROCESSING = 3;
  INGESTION_JOB_STATUS_INDEXING = 4;
  INGESTION_JOB_STATUS_COMPLETED = 5;
  INGESTION_JOB_STATUS_FAILED = 6;
}

message IngestionMetrics {
  int64 pdf_size_bytes = 1;
  int32 page_count = 2;
  int32 chunk_count = 3;
  int64 processing_time_ms = 4;
}
```

---

# 6. Project Structure

```
literature-review-service/
├── .github/
│   └── workflows/
│       ├── ci.yaml                      # CI pipeline
│       ├── cd.yaml                      # CD pipeline
│       └── security-scan.yaml           # Security scanning
│
├── api/
│   └── proto/
│       ├── literaturereview/
│       │   └── v1/
│       │       └── literature_review.proto
│       └── ingestion/
│           └── v1/
│               └── ingestion.proto      # Copied from ingestion service
│
├── buf.yaml                             # Buf configuration
├── buf.gen.yaml                         # Buf generation config
│
├── cmd/
│   ├── server/
│   │   └── main.go                      # HTTP + gRPC server entrypoint
│   ├── worker/
│   │   └── main.go                      # Temporal worker entrypoint
│   └── migrate/
│       └── main.go                      # Database migration tool
│
├── config/
│   ├── config.yaml                      # Default configuration
│   ├── config.dev.yaml                  # Development overrides
│   ├── config.staging.yaml              # Staging overrides
│   └── config.prod.yaml                 # Production overrides
│
├── deployments/
│   ├── docker/
│   │   ├── Dockerfile                   # Multi-stage build
│   │   ├── Dockerfile.worker            # Worker-specific image
│   │   └── docker-compose.yaml          # Local development
│   │
│   └── kubernetes/
│       ├── base/
│       │   ├── kustomization.yaml
│       │   ├── namespace.yaml
│       │   ├── deployment-server.yaml
│       │   ├── deployment-worker.yaml
│       │   ├── service.yaml
│       │   ├── configmap.yaml
│       │   ├── serviceaccount.yaml
│       │   └── hpa.yaml
│       │
│       ├── overlays/
│       │   ├── dev/
│       │   │   └── kustomization.yaml
│       │   ├── staging/
│       │   │   └── kustomization.yaml
│       │   └── prod/
│       │       └── kustomization.yaml
│       │
│       └── secrets/                     # Sealed secrets or external secrets
│           └── external-secrets.yaml
│
├── docs/
│   ├── architecture.md
│   ├── api.md
│   ├── runbook.md
│   └── adr/                             # Architecture Decision Records
│       ├── 001-temporal-for-workflows.md
│       ├── 002-outbox-for-events.md
│       └── 003-canonical-paper-ids.md
│
├── gen/
│   └── proto/
│       ├── literaturereview/
│       │   └── v1/
│       │       ├── literature_review.pb.go
│       │       └── literature_review_grpc.pb.go
│       └── ingestion/
│           └── v1/
│               ├── ingestion.pb.go
│               └── ingestion_grpc.pb.go
│
├── internal/
│   ├── config/
│   │   ├── config.go                    # Configuration struct & loading
│   │   ├── validation.go                # Config validation
│   │   └── config_test.go
│   │
│   ├── domain/
│   │   ├── models.go                    # Core domain models
│   │   ├── paper.go                     # Paper entity
│   │   ├── keyword.go                   # Keyword entity
│   │   ├── review.go                    # LiteratureReview entity
│   │   ├── errors.go                    # Domain errors
│   │   └── events.go                    # Domain events
│   │
│   ├── repository/
│   │   ├── repository.go                # Repository interfaces
│   │   ├── postgres/
│   │   │   ├── postgres.go              # PostgreSQL implementation
│   │   │   ├── papers.go                # Paper repository
│   │   │   ├── keywords.go              # Keyword repository
│   │   │   ├── reviews.go               # Review repository
│   │   │   ├── outbox.go                # Outbox repository
│   │   │   ├── tx.go                    # Transaction helpers
│   │   │   └── migrations/
│   │   │       ├── 000001_initial.up.sql
│   │   │       ├── 000001_initial.down.sql
│   │   │       └── embed.go
│   │   │
│   │   └── queries/
│   │       └── queries.sql              # sqlc query definitions
│   │
│   ├── service/
│   │   ├── service.go                   # Main service interface
│   │   ├── literature_review.go         # Literature review service
│   │   └── progress.go                  # Progress tracking service
│   │
│   ├── server/
│   │   ├── grpc/
│   │   │   ├── server.go                # gRPC server setup
│   │   │   ├── handlers.go              # gRPC handlers
│   │   │   ├── interceptors.go          # Custom interceptors
│   │   │   └── errors.go                # Error mapping
│   │   │
│   │   └── http/
│   │       ├── server.go                # HTTP server setup
│   │       ├── handlers.go              # HTTP handlers
│   │       ├── middleware.go            # HTTP middleware
│   │       ├── progress_stream.go       # SSE streaming
│   │       └── errors.go                # Error responses
│   │
│   ├── temporal/
│   │   ├── client.go                    # Temporal client wrapper
│   │   ├── worker.go                    # Worker setup
│   │   │
│   │   ├── workflows/
│   │   │   ├── literature_review.go     # Main workflow
│   │   │   ├── paper_search.go          # Search child workflow
│   │   │   ├── keyword_expansion.go     # Expansion child workflow
│   │   │   ├── types.go                 # Workflow input/output types
│   │   │   └── determinism.go           # Determinism helpers
│   │   │
│   │   └── activities/
│   │       ├── activities.go            # Activity registration
│   │       ├── llm_activities.go        # LLM keyword extraction
│   │       ├── search_activities.go     # Paper search
│   │       ├── db_activities.go         # Database operations
│   │       ├── ingestion_activities.go  # Ingestion service calls
│   │       ├── event_activities.go      # Event publishing
│   │       └── types.go                 # Activity input/output types
│   │
│   ├── llm/
│   │   ├── client.go                    # LLM client interface
│   │   ├── openai/
│   │   │   └── client.go                # OpenAI implementation
│   │   ├── anthropic/
│   │   │   └── client.go                # Anthropic implementation
│   │   ├── azure/
│   │   │   └── client.go                # Azure OpenAI implementation
│   │   └── prompts/
│   │       ├── keyword_extraction.go    # Extraction prompts
│   │       └── templates/
│   │           ├── query_keywords.tmpl
│   │           └── paper_keywords.tmpl
│   │
│   ├── papersources/
│   │   ├── client.go                    # Paper source interface
│   │   ├── models.go                    # Shared models
│   │   ├── errors.go                    # API errors
│   │   ├── semanticscholar/
│   │   │   ├── client.go
│   │   │   ├── types.go
│   │   │   └── client_test.go
│   │   ├── openalex/
│   │   │   ├── client.go
│   │   │   ├── types.go
│   │   │   └── client_test.go
│   │   ├── scopus/
│   │   │   ├── client.go
│   │   │   ├── types.go
│   │   │   └── client_test.go
│   │   ├── pubmed/
│   │   │   ├── client.go
│   │   │   ├── types.go
│   │   │   └── client_test.go
│   │   ├── biorxiv/
│   │   │   ├── client.go
│   │   │   ├── types.go
│   │   │   └── client_test.go
│   │   └── arxiv/
│   │       ├── client.go
│   │       ├── types.go
│   │       └── client_test.go
│   │
│   ├── ratelimit/
│   │   ├── limiter.go                   # Rate limiter interface
│   │   ├── token_bucket.go              # Token bucket implementation
│   │   ├── pool.go                      # Rate limiter pool
│   │   └── backoff.go                   # Backoff helpers
│   │
│   ├── ingestion/
│   │   ├── client.go                    # Ingestion gRPC client
│   │   └── client_test.go
│   │
│   ├── outbox/
│   │   ├── publisher.go                 # Event publisher
│   │   ├── events.go                    # Event types
│   │   └── transactional.go             # Transactional publishing
│   │
│   ├── pagination/
│   │   ├── token.go                     # Pagination token
│   │   ├── cursor.go                    # Cursor-based pagination
│   │   └── token_test.go
│   │
│   ├── dedup/
│   │   └── deduplicator.go              # Deduplication logic
│   │
│   └── observability/
│       ├── tracing.go                   # OpenTelemetry tracing
│       ├── metrics.go                   # Prometheus metrics
│       ├── logging.go                   # Structured logging
│       └── middleware.go                # Observability middleware
│
├── migrations/
│   ├── 000001_initial_schema.up.sql
│   ├── 000001_initial_schema.down.sql
│   └── atlas.hcl                        # Atlas migration config
│
├── pkg/
│   └── testutil/
│       ├── docker.go                    # Docker test helpers
│       ├── temporal.go                  # Temporal test helpers
│       └── fixtures.go                  # Test fixtures
│
├── scripts/
│   ├── generate-proto.sh                # Proto generation
│   ├── run-migrations.sh                # Migration runner
│   ├── setup-local.sh                   # Local dev setup
│   └── load-test.sh                     # Load testing
│
├── tests/
│   ├── integration/
│   │   ├── workflow_test.go             # Workflow integration tests
│   │   ├── api_test.go                  # API integration tests
│   │   └── testdata/
│   │
│   └── e2e/
│       ├── e2e_test.go                  # End-to-end tests
│       └── testdata/
│
├── tools/
│   └── tools.go                         # Tool dependencies
│
├── .air.toml                            # Air live reload config
├── .envrc                               # direnv config
├── .golangci.yml                        # Linter config
├── .pre-commit-config.yaml              # Pre-commit hooks
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

# 7. Core Components

## 7.1 Rate Limiter Pool

```go
// internal/ratelimit/pool.go

package ratelimit

import (
    "context"
    "fmt"
    "sync"
    "time"

    "golang.org/x/time/rate"
)

// Config holds rate limiter configuration for a source
type Config struct {
    RequestsPerSecond float64
    Burst             int
    Concurrency       int
}

// BackoffConfig holds backoff configuration
type BackoffConfig struct {
    InitialInterval time.Duration
    MaxInterval     time.Duration
    Multiplier      float64
    MaxRetries      int
}

// Pool manages rate limiters for multiple sources
type Pool struct {
    limiters    map[string]*rate.Limiter
    configs     map[string]Config
    backoffs    map[string]BackoffConfig
    semaphores  map[string]chan struct{}
    mu          sync.RWMutex
}

// NewPool creates a new rate limiter pool
func NewPool(configs map[string]Config, backoffs map[string]BackoffConfig) *Pool {
    p := &Pool{
        limiters:   make(map[string]*rate.Limiter),
        configs:    configs,
        backoffs:   backoffs,
        semaphores: make(map[string]chan struct{}),
    }

    for name, cfg := range configs {
        p.limiters[name] = rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.Burst)
        p.semaphores[name] = make(chan struct{}, cfg.Concurrency)
    }

    return p
}

// GetLimiter returns the rate limiter for a source
func (p *Pool) GetLimiter(name string) *rate.Limiter {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.limiters[name]
}

// GetBackoffConfig returns the backoff configuration for a source
func (p *Pool) GetBackoffConfig(name string) BackoffConfig {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.backoffs[name]
}

// Wait waits for rate limiter permission
func (p *Pool) Wait(ctx context.Context, name string) error {
    limiter := p.GetLimiter(name)
    if limiter == nil {
        return nil
    }
    return limiter.Wait(ctx)
}

// Acquire acquires a concurrency slot, returns release function
func (p *Pool) Acquire(ctx context.Context, name string) (func(), error) {
    p.mu.RLock()
    sem := p.semaphores[name]
    p.mu.RUnlock()

    if sem == nil {
        return func() {}, nil
    }

    select {
    case sem <- struct{}{}:
        return func() { <-sem }, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

// Metrics returns current metrics for all limiters
func (p *Pool) Metrics() map[string]LimiterMetrics {
    p.mu.RLock()
    defer p.mu.RUnlock()

    metrics := make(map[string]LimiterMetrics)
    for name, limiter := range p.limiters {
        sem := p.semaphores[name]
        metrics[name] = LimiterMetrics{
            TokensAvailable:    limiter.Tokens(),
            ConcurrencyInUse:   len(sem),
            ConcurrencyLimit:   cap(sem),
        }
    }
    return metrics
}

type LimiterMetrics struct {
    TokensAvailable  float64
    ConcurrencyInUse int
    ConcurrencyLimit int
}
```

## 7.2 Backoff Helper

```go
// internal/ratelimit/backoff.go

package ratelimit

import (
    "context"
    "errors"
    "math/rand"
    "time"

    "github.com/company/literature-review-service/internal/papersources"
)

// Backoff executes a function with exponential backoff for rate limits
func Backoff(
    ctx context.Context,
    cfg BackoffConfig,
    fn func() error,
) error {
    backoff := cfg.InitialInterval

    for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
        err := fn()
        if err == nil {
            return nil
        }

        // Check if retryable
        var apiErr *papersources.APIError
        if errors.As(err, &apiErr) {
            switch apiErr.StatusCode {
            case 429: // Rate limited
                waitTime := backoff
                
                // Use Retry-After header if present
                if apiErr.RetryAfter > 0 {
                    waitTime = time.Duration(apiErr.RetryAfter) * time.Second
                }
                
                // Add jitter (10-20%)
                jitter := time.Duration(float64(waitTime) * (0.1 + rand.Float64()*0.1))
                waitTime += jitter
                
                select {
                case <-ctx.Done():
                    return ctx.Err()
                case <-time.After(waitTime):
                    backoff = minDuration(
                        time.Duration(float64(backoff)*cfg.Multiplier),
                        cfg.MaxInterval,
                    )
                    continue
                }

            case 502, 503, 504: // Server unavailable
                select {
                case <-ctx.Done():
                    return ctx.Err()
                case <-time.After(backoff):
                    backoff = minDuration(
                        time.Duration(float64(backoff)*cfg.Multiplier),
                        cfg.MaxInterval,
                    )
                    continue
                }
            }
        }

        // Non-retryable error
        return err
    }

    return errors.New("max retries exceeded")
}

func minDuration(a, b time.Duration) time.Duration {
    if a < b {
        return a
    }
    return b
}
```

## 7.3 Pagination Token

```go
// internal/pagination/token.go

package pagination

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "errors"
    "time"
)

var (
    ErrInvalidToken  = errors.New("invalid pagination token")
    ErrExpiredToken  = errors.New("pagination token expired")
)

// TokenData holds the pagination cursor data
type TokenData struct {
    // Cursor values for keyset pagination
    LastID        string    `json:"lid,omitempty"`
    LastCreatedAt time.Time `json:"lca,omitempty"`
    LastSortValue string    `json:"lsv,omitempty"`

    // Context
    SortField string `json:"sf"`
    SortOrder string `json:"so"`

    // Security
    ExpiresAt time.Time `json:"exp"`
}

// Paginator creates and validates pagination tokens
type Paginator struct {
    secret []byte
    ttl    time.Duration
}

// NewPaginator creates a new paginator
func NewPaginator(secret string, ttl time.Duration) *Paginator {
    return &Paginator{
        secret: []byte(secret),
        ttl:    ttl,
    }
}

// CreateToken creates a signed, opaque pagination token
func (p *Paginator) CreateToken(data TokenData) (string, error) {
    data.ExpiresAt = time.Now().Add(p.ttl)

    payload, err := json.Marshal(data)
    if err != nil {
        return "", err
    }

    // Sign payload
    h := hmac.New(sha256.New, p.secret)
    h.Write(payload)
    sig := h.Sum(nil)

    // Combine: payload + signature
    combined := append(payload, sig...)

    return base64.URLEncoding.EncodeToString(combined), nil
}

// ParseToken validates and parses a pagination token
func (p *Paginator) ParseToken(token string) (*TokenData, error) {
    if token == "" {
        return nil, nil // First page
    }

    combined, err := base64.URLEncoding.DecodeString(token)
    if err != nil {
        return nil, ErrInvalidToken
    }

    if len(combined) < sha256.Size {
        return nil, ErrInvalidToken
    }

    payload := combined[:len(combined)-sha256.Size]
    sig := combined[len(combined)-sha256.Size:]

    // Verify signature
    h := hmac.New(sha256.New, p.secret)
    h.Write(payload)
    expectedSig := h.Sum(nil)

    if !hmac.Equal(sig, expectedSig) {
        return nil, ErrInvalidToken
    }

    var data TokenData
    if err := json.Unmarshal(payload, &data); err != nil {
        return nil, ErrInvalidToken
    }

    // Check expiry
    if time.Now().After(data.ExpiresAt) {
        return nil, ErrExpiredToken
    }

    return &data, nil
}
```

## 7.4 Deduplicator

```go
// internal/dedup/deduplicator.go

package dedup

import (
    "context"
    "time"
)

// Repository interface for deduplication queries
type Repository interface {
    GetLastSearchDate(ctx context.Context, keywordID, source string) (*time.Time, error)
    FindPaperByIdentifier(ctx context.Context, idType, idValue string) (string, error)
    IsKeywordPaperMappingExists(ctx context.Context, keywordID, paperID, mappingType string) (bool, error)
    ArePaperKeywordsExtracted(ctx context.Context, paperID string) (bool, error)
}

// Deduplicator handles deduplication logic
type Deduplicator struct {
    repo Repository
}

// NewDeduplicator creates a new deduplicator
func NewDeduplicator(repo Repository) *Deduplicator {
    return &Deduplicator{repo: repo}
}

// ShouldSearchKeyword determines if a keyword needs searching for a source
// Returns (shouldSearch, fromDate for incremental search)
func (d *Deduplicator) ShouldSearchKeyword(
    ctx context.Context,
    keywordID string,
    source string,
) (bool, *time.Time, error) {
    lastDate, err := d.repo.GetLastSearchDate(ctx, keywordID, source)
    if err != nil {
        return false, nil, err
    }

    // If we searched today, skip
    if lastDate != nil && isSameDay(*lastDate, time.Now()) {
        return false, nil, nil
    }

    // Return last search date for incremental search
    return true, lastDate, nil
}

// PaperIdentifiers holds all possible identifiers for a paper
type PaperIdentifiers struct {
    DOI               string
    ArxivID           string
    PubMedID          string
    SemanticScholarID string
    OpenAlexID        string
    ScopusID          string
}

// FindExistingPaper checks if a paper exists by any identifier
func (d *Deduplicator) FindExistingPaper(
    ctx context.Context,
    ids PaperIdentifiers,
) (string, error) {
    checks := []struct {
        idType  string
        idValue string
    }{
        {"doi", ids.DOI},
        {"arxiv_id", ids.ArxivID},
        {"pubmed_id", ids.PubMedID},
        {"semantic_scholar_id", ids.SemanticScholarID},
        {"openalex_id", ids.OpenAlexID},
        {"scopus_id", ids.ScopusID},
    }

    for _, check := range checks {
        if check.idValue == "" {
            continue
        }
        
        paperID, err := d.repo.FindPaperByIdentifier(ctx, check.idType, check.idValue)
        if err != nil {
            return "", err
        }
        if paperID != "" {
            return paperID, nil
        }
    }

    return "", nil
}

// ShouldCreateMapping checks if a keyword-paper mapping needs creation
func (d *Deduplicator) ShouldCreateMapping(
    ctx context.Context,
    keywordID string,
    paperID string,
    mappingType string,
) (bool, error) {
    exists, err := d.repo.IsKeywordPaperMappingExists(ctx, keywordID, paperID, mappingType)
    if err != nil {
        return false, err
    }
    return !exists, nil
}

// ShouldExtractKeywords checks if keywords need extraction from a paper
func (d *Deduplicator) ShouldExtractKeywords(
    ctx context.Context,
    paperID string,
) (bool, error) {
    extracted, err := d.repo.ArePaperKeywordsExtracted(ctx, paperID)
    if err != nil {
        return false, err
    }
    return !extracted, nil
}

func isSameDay(t1, t2 time.Time) bool {
    y1, m1, d1 := t1.Date()
    y2, m2, d2 := t2.Date()
    return y1 == y2 && m1 == m2 && d1 == d2
}
```

---

# 8. Temporal Workflows & Activities

## 8.1 Determinism Helpers

```go
// internal/temporal/workflows/determinism.go

package workflows

import (
    "sort"
)

// SortedMapKeys returns sorted keys from a map for deterministic iteration
func SortedMapKeys[K ~string, V any](m map[K]V) []K {
    keys := make([]K, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    sort.Slice(keys, func(i, j int) bool {
        return keys[i] < keys[j]
    })
    return keys
}

// SortedStringSlice returns a sorted copy of a string slice
func SortedStringSlice(s []string) []string {
    result := make([]string, len(s))
    copy(result, s)
    sort.Strings(result)
    return result
}

// DeduplicateStrings removes duplicates and returns sorted result
func DeduplicateStrings(s []string) []string {
    seen := make(map[string]bool)
    result := make([]string, 0, len(s))
    
    for _, v := range s {
        if !seen[v] {
            seen[v] = true
            result = append(result, v)
        }
    }
    
    sort.Strings(result)
    return result
}
```

## 8.2 Main Literature Review Workflow

```go
// internal/temporal/workflows/literature_review.go

package workflows

import (
    "fmt"
    "time"

    "go.temporal.io/sdk/temporal"
    "go.temporal.io/sdk/workflow"
)

const (
    // Query handler name for progress
    QueryProgress = "getProgress"
    
    // Max papers before ContinueAsNew
    MaxPapersPerBatch = 500
)

// LiteratureReviewInput is the workflow input
type LiteratureReviewInput struct {
    ReviewID            string
    OrgID               string
    ProjectID           string
    UserID              string
    Query               string
    InitialKeywordCount int
    PaperKeywordCount   int
    MaxExpansionDepth   int
    EnabledSources      []string
    DateFrom            *time.Time
    DateTo              *time.Time
    
    // For ContinueAsNew
    StartDepth          int
    ExistingPaperIDs    []string
}

// LiteratureReviewOutput is the workflow output
type LiteratureReviewOutput struct {
    ReviewID       string
    TotalPapers    int
    TotalKeywords  int
    IngestedPapers int
    Duration       time.Duration
    Error          string
}

// WorkflowState holds queryable progress state
type WorkflowState struct {
    Status           string                 `json:"status"`
    CurrentDepth     int                    `json:"current_depth"`
    MaxDepth         int                    `json:"max_depth"`
    InitialKeywords  []string               `json:"initial_keywords"`
    TotalKeywords    int                    `json:"total_keywords"`
    PapersFound      int                    `json:"papers_found"`
    PapersIngested   int                    `json:"papers_ingested"`
    SourceProgress   map[string]SourceStats `json:"source_progress"`
    LastUpdated      time.Time              `json:"last_updated"`
    ErrorMessage     string                 `json:"error_message,omitempty"`
}

type SourceStats struct {
    QueriesCompleted int `json:"queries_completed"`
    PapersFound      int `json:"papers_found"`
    Errors           int `json:"errors"`
}

// LiteratureReviewWorkflow orchestrates the entire literature review process
func LiteratureReviewWorkflow(ctx workflow.Context, input LiteratureReviewInput) (*LiteratureReviewOutput, error) {
    logger := workflow.GetLogger(ctx)
    logger.Info("Starting literature review workflow",
        "reviewID", input.ReviewID,
        "orgID", input.OrgID,
        "projectID", input.ProjectID,
        "startDepth", input.StartDepth)

    // Initialize state
    state := &WorkflowState{
        Status:          "initializing",
        MaxDepth:        input.MaxExpansionDepth,
        CurrentDepth:    input.StartDepth,
        SourceProgress:  make(map[string]SourceStats),
        LastUpdated:     workflow.Now(ctx),
    }

    // Register query handler
    if err := workflow.SetQueryHandler(ctx, QueryProgress, func() (*WorkflowState, error) {
        return state, nil
    }); err != nil {
        return nil, fmt.Errorf("failed to register query handler: %w", err)
    }

    // Configure activity options
    activityOpts := workflow.ActivityOptions{
        StartToCloseTimeout: 5 * time.Minute,
        HeartbeatTimeout:    30 * time.Second,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    time.Second,
            BackoffCoefficient: 2.0,
            MaximumInterval:    time.Minute,
            MaximumAttempts:    5,
            NonRetryableErrorTypes: []string{
                "ValidationError",
                "AuthorizationError",
            },
        },
    }
    ctx = workflow.WithActivityOptions(ctx, activityOpts)

    startTime := workflow.Now(ctx)
    output := &LiteratureReviewOutput{ReviewID: input.ReviewID}

    // Initialize paper tracking (deterministic from sorted existing IDs)
    allPaperIDs := make(map[string]bool)
    for _, id := range input.ExistingPaperIDs {
        allPaperIDs[id] = true
    }

    // ========================================
    // PHASE 1: Extract initial keywords (skip if continuing)
    // ========================================
    var initialKeywords []string
    
    if input.StartDepth == 0 {
        state.Status = "extracting_keywords"
        state.LastUpdated = workflow.Now(ctx)

        // Publish start event
        if err := executeActivity(ctx, PublishEventActivity, PublishEventInput{
            EventType: "review.started",
            ReviewID:  input.ReviewID,
            OrgID:     input.OrgID,
            ProjectID: input.ProjectID,
            Data: map[string]interface{}{
                "query":      input.Query,
                "max_depth":  input.MaxExpansionDepth,
                "sources":    input.EnabledSources,
                "started_at": startTime,
            },
        }); err != nil {
            logger.Warn("Failed to publish start event", "error", err)
        }

        // Extract keywords from query
        var extractResult ExtractKeywordsResult
        err := workflow.ExecuteActivity(ctx, ExtractKeywordsFromQueryActivity, ExtractKeywordsInput{
            ReviewID:     input.ReviewID,
            OrgID:        input.OrgID,
            ProjectID:    input.ProjectID,
            Query:        input.Query,
            KeywordCount: input.InitialKeywordCount,
        }).Get(ctx, &extractResult)
        if err != nil {
            return handleWorkflowError(ctx, state, input, "keyword_extraction", err)
        }

        initialKeywords = extractResult.Keywords
        state.InitialKeywords = initialKeywords
        state.TotalKeywords = len(initialKeywords)

        // Save keywords
        var saveResult SaveKeywordsResult
        err = workflow.ExecuteActivity(ctx, SaveKeywordsActivity, SaveKeywordsInput{
            ReviewID:   input.ReviewID,
            OrgID:      input.OrgID,
            ProjectID:  input.ProjectID,
            Keywords:   initialKeywords,
            SourceType: "user_query",
            Round:      0,
        }).Get(ctx, &saveResult)
        if err != nil {
            return handleWorkflowError(ctx, state, input, "save_keywords", err)
        }
    }

    // ========================================
    // PHASE 2: Search papers with expansion
    // ========================================
    for depth := input.StartDepth; depth <= input.MaxExpansionDepth; depth++ {
        state.Status = statusForDepth(depth)
        state.CurrentDepth = depth
        state.LastUpdated = workflow.Now(ctx)

        // Update DB status
        if err := executeActivity(ctx, UpdateReviewStatusActivity, UpdateStatusInput{
            ReviewID: input.ReviewID,
            Status:   state.Status,
            Depth:    depth,
        }); err != nil {
            logger.Warn("Failed to update status", "error", err, "depth", depth)
        }

        // Get keywords to search
        var keywordsResult GetKeywordsToSearchResult
        err := workflow.ExecuteActivity(ctx, GetKeywordsToSearchActivity, GetKeywordsToSearchInput{
            ReviewID:       input.ReviewID,
            Round:          depth,
            EnabledSources: input.EnabledSources,
        }).Get(ctx, &keywordsResult)
        if err != nil {
            return handleWorkflowError(ctx, state, input, "get_keywords", err)
        }

        if len(keywordsResult.SearchTasks) == 0 {
            logger.Info("No new keywords to search", "depth", depth)
            break
        }

        // Execute search via child workflow
        // FIXED: Use fmt.Sprintf for deterministic workflow ID
        childOpts := workflow.ChildWorkflowOptions{
            WorkflowID: fmt.Sprintf("%s-search-%d", input.ReviewID, depth),
        }
        childCtx := workflow.WithChildOptions(ctx, childOpts)

        var searchResult PaperSearchResult
        err = workflow.ExecuteChildWorkflow(childCtx, PaperSearchWorkflow, PaperSearchInput{
            ReviewID:    input.ReviewID,
            OrgID:       input.OrgID,
            ProjectID:   input.ProjectID,
            SearchTasks: keywordsResult.SearchTasks,
            DateFrom:    input.DateFrom,
            DateTo:      input.DateTo,
        }).Get(ctx, &searchResult)
        if err != nil {
            logger.Warn("Search workflow had errors", "error", err, "depth", depth)
        }

        // FIXED: Sort paper IDs for deterministic processing
        sortedPaperIDs := SortedStringSlice(searchResult.PaperIDs)

        newPapers := 0
        for _, paperID := range sortedPaperIDs {
            if !allPaperIDs[paperID] {
                allPaperIDs[paperID] = true
                newPapers++
            }
        }

        // FIXED: Update source progress with deterministic iteration
        for _, source := range SortedMapKeys(searchResult.SourceStats) {
            stats := searchResult.SourceStats[source]
            existing := state.SourceProgress[source]
            state.SourceProgress[source] = SourceStats{
                QueriesCompleted: existing.QueriesCompleted + stats.QueriesCompleted,
                PapersFound:      existing.PapersFound + stats.PapersFound,
                Errors:           existing.Errors + stats.Errors,
            }
        }
        state.PapersFound = len(allPaperIDs)
        state.LastUpdated = workflow.Now(ctx)

        logger.Info("Search round completed",
            "depth", depth,
            "newPapers", newPapers,
            "totalPapers", len(allPaperIDs))

        // Expand keywords if not last round and found new papers
        if depth < input.MaxExpansionDepth && newPapers > 0 {
            expandOpts := workflow.ChildWorkflowOptions{
                WorkflowID: fmt.Sprintf("%s-expand-%d", input.ReviewID, depth),
            }
            expandCtx := workflow.WithChildOptions(ctx, expandOpts)

            var expandResult KeywordExpansionResult
            err = workflow.ExecuteChildWorkflow(expandCtx, KeywordExpansionWorkflow, KeywordExpansionInput{
                ReviewID:         input.ReviewID,
                OrgID:            input.OrgID,
                ProjectID:        input.ProjectID,
                PaperIDs:         sortedPaperIDs,
                KeywordsPerPaper: input.PaperKeywordCount,
                Round:            depth + 1,
            }).Get(ctx, &expandResult)
            if err != nil {
                logger.Warn("Keyword expansion had errors", "error", err)
            }

            state.TotalKeywords += expandResult.NewKeywordsCount
            state.LastUpdated = workflow.Now(ctx)
        }

        // Check if we need ContinueAsNew
        if len(allPaperIDs) > MaxPapersPerBatch && depth < input.MaxExpansionDepth {
            logger.Info("Continuing as new due to large paper count",
                "papers", len(allPaperIDs),
                "depth", depth)
            return continueAsNew(ctx, input, allPaperIDs, depth+1)
        }
    }

    // ========================================
    // PHASE 3: Send to ingestion
    // ========================================
    state.Status = "ingesting"
    state.LastUpdated = workflow.Now(ctx)

    // FIXED: Convert map to sorted slice for determinism
    paperIDList := make([]string, 0, len(allPaperIDs))
    for id := range allPaperIDs {
        paperIDList = append(paperIDList, id)
    }
    sort.Strings(paperIDList)

    var ingestionResult IngestionResult
    err := workflow.ExecuteActivity(ctx, SendToIngestionActivity, SendToIngestionInput{
        ReviewID:  input.ReviewID,
        OrgID:     input.OrgID,
        ProjectID: input.ProjectID,
        PaperIDs:  paperIDList,
    }).Get(ctx, &ingestionResult)
    if err != nil {
        logger.Warn("Ingestion had errors", "error", err)
    }

    state.PapersIngested = ingestionResult.SuccessCount

    // ========================================
    // PHASE 4: Complete
    // ========================================
    state.Status = "completed"
    state.LastUpdated = workflow.Now(ctx)

    output.TotalPapers = len(allPaperIDs)
    output.TotalKeywords = state.TotalKeywords
    output.IngestedPapers = ingestionResult.SuccessCount
    output.Duration = workflow.Now(ctx).Sub(startTime)

    // Final status update
    if err := executeActivity(ctx, UpdateReviewStatusActivity, UpdateStatusInput{
        ReviewID:       input.ReviewID,
        Status:         "completed",
        PapersFound:    output.TotalPapers,
        PapersIngested: output.IngestedPapers,
    }); err != nil {
        logger.Warn("Failed to update final status", "error", err)
    }

    // Publish completion event
    _ = executeActivity(ctx, PublishEventActivity, PublishEventInput{
        EventType: "review.completed",
        ReviewID:  input.ReviewID,
        OrgID:     input.OrgID,
        ProjectID: input.ProjectID,
        Data: map[string]interface{}{
            "total_papers":    output.TotalPapers,
            "ingested_papers": output.IngestedPapers,
            "total_keywords":  output.TotalKeywords,
            "duration_ms":     output.Duration.Milliseconds(),
            "completed_at":    workflow.Now(ctx),
        },
    })

    logger.Info("Literature review completed",
        "reviewID", input.ReviewID,
        "totalPapers", output.TotalPapers,
        "duration", output.Duration)

    return output, nil
}

// Helper to execute activity and handle errors consistently
func executeActivity(ctx workflow.Context, activity interface{}, input interface{}) error {
    return workflow.ExecuteActivity(ctx, activity, input).Get(ctx, nil)
}

func handleWorkflowError(
    ctx workflow.Context,
    state *WorkflowState,
    input LiteratureReviewInput,
    phase string,
    err error,
) (*LiteratureReviewOutput, error) {
    logger := workflow.GetLogger(ctx)
    logger.Error("Workflow failed", "phase", phase, "error", err)

    state.Status = "failed"
    state.ErrorMessage = fmt.Sprintf("%s: %v", phase, err)
    state.LastUpdated = workflow.Now(ctx)

    // Update DB status
    _ = executeActivity(ctx, UpdateReviewStatusActivity, UpdateStatusInput{
        ReviewID:     input.ReviewID,
        Status:       "failed",
        ErrorMessage: state.ErrorMessage,
    })

    // Publish failure event
    _ = executeActivity(ctx, PublishEventActivity, PublishEventInput{
        EventType: "review.failed",
        ReviewID:  input.ReviewID,
        OrgID:     input.OrgID,
        ProjectID: input.ProjectID,
        Data: map[string]interface{}{
            "phase":     phase,
            "error":     err.Error(),
            "failed_at": workflow.Now(ctx),
        },
    })

    return &LiteratureReviewOutput{
        ReviewID: input.ReviewID,
        Error:    state.ErrorMessage,
    }, fmt.Errorf("workflow failed at %s: %w", phase, err)
}

func statusForDepth(depth int) string {
    if depth == 0 {
        return "searching"
    }
    return "expanding"
}

func continueAsNew(
    ctx workflow.Context,
    input LiteratureReviewInput,
    paperIDs map[string]bool,
    startDepth int,
) (*LiteratureReviewOutput, error) {
    // Convert to sorted slice for determinism
    sortedIDs := make([]string, 0, len(paperIDs))
    for id := range paperIDs {
        sortedIDs = append(sortedIDs, id)
    }
    sort.Strings(sortedIDs)

    // Persist progress before continuing
    _ = executeActivity(ctx, PersistProgressActivity, PersistProgressInput{
        ReviewID: input.ReviewID,
        PaperIDs: sortedIDs,
        Depth:    startDepth,
    })

    // Continue as new with updated input
    return nil, workflow.NewContinueAsNewError(ctx, LiteratureReviewWorkflow, LiteratureReviewInput{
        ReviewID:            input.ReviewID,
        OrgID:               input.OrgID,
        ProjectID:           input.ProjectID,
        UserID:              input.UserID,
        Query:               input.Query,
        InitialKeywordCount: input.InitialKeywordCount,
        PaperKeywordCount:   input.PaperKeywordCount,
        MaxExpansionDepth:   input.MaxExpansionDepth,
        EnabledSources:      input.EnabledSources,
        DateFrom:            input.DateFrom,
        DateTo:              input.DateTo,
        StartDepth:          startDepth,
        ExistingPaperIDs:    sortedIDs,
    })
}
```

## 8.3 Paper Search Workflow

```go
// internal/temporal/workflows/paper_search.go

package workflows

import (
    "fmt"
    "time"

    "go.temporal.io/sdk/temporal"
    "go.temporal.io/sdk/workflow"
)

type PaperSearchInput struct {
    ReviewID    string
    OrgID       string
    ProjectID   string
    SearchTasks []SearchTask
    DateFrom    *time.Time
    DateTo      *time.Time
}

type SearchTask struct {
    KeywordID      string
    Keyword        string
    Source         string
    SourceType     string
    LastSearchDate *time.Time
    Depth          int
}

type PaperSearchResult struct {
    PaperIDs    []string
    TotalFound  int
    SourceStats map[string]SourceSearchStats
}

type SourceSearchStats struct {
    QueriesCompleted int
    PapersFound      int
    Errors           int
}

func PaperSearchWorkflow(ctx workflow.Context, input PaperSearchInput) (*PaperSearchResult, error) {
    logger := workflow.GetLogger(ctx)

    // Group tasks by source for parallel execution
    tasksBySource := make(map[string][]SearchTask)
    for _, task := range input.SearchTasks {
        tasksBySource[task.Source] = append(tasksBySource[task.Source], task)
    }

    // Activity options with longer timeout for API calls
    activityOpts := workflow.ActivityOptions{
        StartToCloseTimeout: 3 * time.Minute,
        HeartbeatTimeout:    30 * time.Second,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    2 * time.Second,
            BackoffCoefficient: 2.0,
            MaximumInterval:    2 * time.Minute,
            MaximumAttempts:    3,
        },
    }
    ctx = workflow.WithActivityOptions(ctx, activityOpts)

    // Execute searches per source in parallel
    // FIXED: Iterate sources in deterministic order
    sortedSources := SortedMapKeys(tasksBySource)
    futures := make(map[string]workflow.Future)

    for _, source := range sortedSources {
        tasks := tasksBySource[source]
        future := workflow.ExecuteActivity(ctx, SearchPapersActivity, SearchPapersInput{
            ReviewID:  input.ReviewID,
            OrgID:     input.OrgID,
            ProjectID: input.ProjectID,
            Source:    source,
            Tasks:     tasks,
            DateFrom:  input.DateFrom,
            DateTo:    input.DateTo,
        })
        futures[source] = future
    }

    // Collect results
    result := &PaperSearchResult{
        PaperIDs:    make([]string, 0),
        SourceStats: make(map[string]SourceSearchStats),
    }

    seenPapers := make(map[string]bool)

    // FIXED: Collect results in deterministic order
    for _, source := range sortedSources {
        future := futures[source]
        var searchResult SearchPapersResult
        err := future.Get(ctx, &searchResult)
        if err != nil {
            logger.Warn("Search failed for source", "source", source, "error", err)
            result.SourceStats[source] = SourceSearchStats{Errors: 1}
            continue
        }

        result.SourceStats[source] = SourceSearchStats{
            QueriesCompleted: searchResult.QueriesCompleted,
            PapersFound:      len(searchResult.PaperIDs),
            Errors:           len(searchResult.Errors),
        }

        for _, paperID := range searchResult.PaperIDs {
            if !seenPapers[paperID] {
                seenPapers[paperID] = true
                result.PaperIDs = append(result.PaperIDs, paperID)
            }
        }
    }

    // Sort final paper IDs for determinism
    result.PaperIDs = SortedStringSlice(result.PaperIDs)
    result.TotalFound = len(result.PaperIDs)

    logger.Info("Paper search workflow completed",
        "reviewID", input.ReviewID,
        "totalPapers", result.TotalFound,
        "sources", len(sortedSources))

    return result, nil
}
```

## 8.4 Search Activities with Local Backoff

```go
// internal/temporal/activities/search_activities.go

package activities

import (
    "context"
    "fmt"
    "sort"
    "sync"

    "golang.org/x/sync/errgroup"

    "github.com/company/literature-review-service/internal/outbox"
    "github.com/company/literature-review-service/internal/papersources"
    "github.com/company/literature-review-service/internal/ratelimit"
    "github.com/company/literature-review-service/internal/repository"
)

type SearchActivities struct {
    paperSources  map[string]papersources.Client
    rateLimitPool *ratelimit.Pool
    repo          repository.Repository
    outbox        *outbox.Publisher
}

type SearchPapersInput struct {
    ReviewID  string
    OrgID     string
    ProjectID string
    Source    string
    Tasks     []SearchTask
    DateFrom  *time.Time
    DateTo    *time.Time
}

type SearchTask struct {
    KeywordID      string
    Keyword        string
    SourceType     string
    LastSearchDate *time.Time
    Depth          int
}

type SearchPapersResult struct {
    PaperIDs         []string
    QueriesCompleted int
    Errors           []string
}

func (a *SearchActivities) SearchPapersActivity(
    ctx context.Context,
    input SearchPapersInput,
) (*SearchPapersResult, error) {
    client, ok := a.paperSources[input.Source]
    if !ok {
        return nil, fmt.Errorf("unknown source: %s", input.Source)
    }

    backoffCfg := a.rateLimitPool.GetBackoffConfig(input.Source)

    result := &SearchPapersResult{
        PaperIDs: make([]string, 0),
    }

    var mu sync.Mutex
    g, gctx := errgroup.WithContext(ctx)

    for _, task := range input.Tasks {
        task := task // capture

        g.Go(func() error {
            // Acquire concurrency slot
            release, err := a.rateLimitPool.Acquire(gctx, input.Source)
            if err != nil {
                return err
            }
            defer release()

            // Wait for rate limiter
            if err := a.rateLimitPool.Wait(gctx, input.Source); err != nil {
                return err
            }

            // Execute with local backoff for 429/503
            papers, err := a.searchWithBackoff(gctx, client, task, input, backoffCfg)
            if err != nil {
                mu.Lock()
                result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", task.Keyword, err))
                mu.Unlock()
                return nil // Don't fail entire batch
            }

            // Process results
            paperIDs, err := a.processPapers(gctx, papers, task, input)
            if err != nil {
                mu.Lock()
                result.Errors = append(result.Errors, fmt.Sprintf("process %s: %v", task.Keyword, err))
                mu.Unlock()
                return nil
            }

            mu.Lock()
            result.PaperIDs = append(result.PaperIDs, paperIDs...)
            result.QueriesCompleted++
            mu.Unlock()

            return nil
        })
    }

    if err := g.Wait(); err != nil {
        return result, err
    }

    // Sort for deterministic output
    sort.Strings(result.PaperIDs)

    return result, nil
}

func (a *SearchActivities) searchWithBackoff(
    ctx context.Context,
    client papersources.Client,
    task SearchTask,
    input SearchPapersInput,
    cfg ratelimit.BackoffConfig,
) ([]papersources.Paper, error) {
    var papers []papersources.Paper
    
    err := ratelimit.Backoff(ctx, cfg, func() error {
        fromDate := input.DateFrom
        if task.LastSearchDate != nil {
            fromDate = task.LastSearchDate
        }

        var searchErr error
        papers, searchErr = client.Search(ctx, papersources.SearchParams{
            Query:    task.Keyword,
            FromDate: fromDate,
            ToDate:   input.DateTo,
        })
        return searchErr
    })

    return papers, err
}

func (a *SearchActivities) processPapers(
    ctx context.Context,
    papers []papersources.Paper,
    task SearchTask,
    input SearchPapersInput,
) ([]string, error) {
    paperIDs := make([]string, 0, len(papers))

    for _, paper := range papers {
        // Build identifiers for upsert
        identifiers := paper.ToIdentifiersJSON(input.Source)

        paperID, err := a.repo.UpsertPaper(ctx, repository.UpsertPaperInput{
            Identifiers:     identifiers,
            Title:           paper.Title,
            Abstract:        paper.Abstract,
            Authors:         paper.Authors,
            PublicationDate: paper.PublicationDate,
            SourceAPI:       input.Source,
            RawMetadata:     paper.RawMetadata,
        })
        if err != nil {
            continue
        }

        paperIDs = append(paperIDs, paperID)

        // Create keyword-paper mapping
        _ = a.repo.CreateKeywordPaperMapping(ctx, repository.KeywordPaperMapping{
            KeywordID:   task.KeywordID,
            PaperID:     paperID,
            MappingType: "search_result",
            SourceType:  task.SourceType,
        })

        // Create request-paper mapping (tenant-scoped)
        _ = a.repo.CreateRequestPaperMapping(ctx, repository.RequestPaperMapping{
            RequestID:            input.ReviewID,
            PaperID:              paperID,
            DiscoveredViaKeyword: task.KeywordID,
            DiscoveredViaSource:  input.Source,
            ExpansionDepth:       task.Depth,
        })
    }

    // Record the search (idempotent)
    fromDate := input.DateFrom
    if task.LastSearchDate != nil {
        fromDate = task.LastSearchDate
    }

    _ = a.repo.RecordKeywordSearch(ctx, repository.KeywordSearch{
        KeywordID:      task.KeywordID,
        SourceAPI:      input.Source,
        SearchFromDate: fromDate,
        SearchToDate:   input.DateTo,
        PapersFound:    len(papers),
    })

    // Publish discovery event
    if len(paperIDs) > 0 {
        _ = a.outbox.PublishPapersDiscovered(ctx, outbox.PapersDiscoveredEvent{
            ReviewID:      input.ReviewID,
            OrgID:         input.OrgID,
            ProjectID:     input.ProjectID,
            Source:        input.Source,
            Keyword:       task.Keyword,
            PaperIDs:      paperIDs,
            Count:         len(paperIDs),
            IsIncremental: task.LastSearchDate != nil,
        })
    }

    return paperIDs, nil
}
```

## 8.5 LLM Activities

```go
// internal/temporal/activities/llm_activities.go

package activities

import (
    "context"
    "fmt"

    "github.com/company/literature-review-service/internal/llm"
    "github.com/company/literature-review-service/internal/repository"
)

type LLMActivities struct {
    llmClient llm.Client
    repo      repository.Repository
}

type ExtractKeywordsInput struct {
    ReviewID     string
    OrgID        string
    ProjectID    string
    Query        string
    KeywordCount int
}

type ExtractKeywordsResult struct {
    Keywords []string
}

func (a *LLMActivities) ExtractKeywordsFromQueryActivity(
    ctx context.Context,
    input ExtractKeywordsInput,
) (*ExtractKeywordsResult, error) {
    keywords, err := a.llmClient.ExtractKeywords(ctx, llm.ExtractionInput{
        Text:        input.Query,
        MaxKeywords: input.KeywordCount,
        Type:        llm.ExtractionTypeQuery,
    })
    if err != nil {
        return nil, fmt.Errorf("LLM extraction failed: %w", err)
    }

    return &ExtractKeywordsResult{Keywords: keywords}, nil
}

type ExtractPaperKeywordsInput struct {
    ReviewID     string
    OrgID        string
    ProjectID    string
    PaperID      string
    Title        string
    Abstract     string
    KeywordCount int
}

type ExtractPaperKeywordsResult struct {
    Keywords   []string
    FromCache  bool
}

func (a *LLMActivities) ExtractPaperKeywordsActivity(
    ctx context.Context,
    input ExtractPaperKeywordsInput,
) (*ExtractPaperKeywordsResult, error) {
    // Check if already extracted (deduplication)
    extracted, err := a.repo.ArePaperKeywordsExtracted(ctx, input.PaperID)
    if err != nil {
        return nil, err
    }
    
    if extracted {
        // Return existing keywords
        keywords, err := a.repo.GetPaperKeywords(ctx, input.PaperID)
        if err != nil {
            return nil, err
        }
        return &ExtractPaperKeywordsResult{
            Keywords:  keywords,
            FromCache: true,
        }, nil
    }

    // Extract new keywords
    text := input.Title
    if input.Abstract != "" {
        text = text + "\n\n" + input.Abstract
    }

    keywords, err := a.llmClient.ExtractKeywords(ctx, llm.ExtractionInput{
        Text:        text,
        MaxKeywords: input.KeywordCount,
        Type:        llm.ExtractionTypePaper,
    })
    if err != nil {
        return nil, fmt.Errorf("LLM extraction failed: %w", err)
    }

    // Save keywords and mark paper as extracted
    for _, kw := range keywords {
        keywordID, err := a.repo.GetOrCreateKeyword(ctx, kw)
        if err != nil {
            continue
        }
        _ = a.repo.CreateKeywordPaperMapping(ctx, repository.KeywordPaperMapping{
            KeywordID:   keywordID,
            PaperID:     input.PaperID,
            MappingType: "extracted",
            SourceType:  "paper_extraction",
        })
    }

    _ = a.repo.MarkPaperKeywordsExtracted(ctx, input.PaperID)

    return &ExtractPaperKeywordsResult{
        Keywords:  keywords,
        FromCache: false,
    }, nil
}
```

## 8.6 Ingestion Activities

```go
// internal/temporal/activities/ingestion_activities.go

package activities

import (
    "context"
    "fmt"

    ingestionv1 "github.com/company/literature-review-service/gen/proto/ingestion/v1"
    "github.com/company/literature-review-service/internal/repository"
)

type IngestionActivities struct {
    ingestionClient ingestionv1.IngestionServiceClient
    repo            repository.Repository
    batchSize       int
}

type SendToIngestionInput struct {
    ReviewID  string
    OrgID     string
    ProjectID string
    PaperIDs  []string
}

type IngestionResult struct {
    SuccessCount int
    FailureCount int
    SkippedCount int
    JobIDs       map[string]string
}

func (a *IngestionActivities) SendToIngestionActivity(
    ctx context.Context,
    input SendToIngestionInput,
) (*IngestionResult, error) {
    result := &IngestionResult{
        JobIDs: make(map[string]string),
    }

    // Get papers that need ingestion
    papers, err := a.repo.GetPapersForIngestion(ctx, input.ReviewID, input.PaperIDs)
    if err != nil {
        return nil, fmt.Errorf("get papers: %w", err)
    }

    // Filter papers not already ingested for this request
    toIngest := make([]*ingestionv1.IngestPaperRequest, 0)
    for _, paper := range papers {
        if paper.IngestionStatus == "completed" || paper.IngestionStatus == "queued" {
            result.SkippedCount++
            continue
        }

        toIngest = append(toIngest, &ingestionv1.IngestPaperRequest{
            PaperId:            paper.ID,
            Doi:                paper.DOI,
            ArxivId:            paper.ArxivID,
            PubmedId:           paper.PubMedID,
            PdfUrl:             paper.PDFURL,
            Title:              paper.Title,
            Abstract:           paper.Abstract,
            SourceService:      "literature-review-service",
            CallbackWorkflowId: input.ReviewID,
            Metadata: map[string]string{
                "org_id":     input.OrgID,
                "project_id": input.ProjectID,
                "review_id":  input.ReviewID,
            },
        })
    }

    if len(toIngest) == 0 {
        return result, nil
    }

    // Batch send to ingestion service
    for i := 0; i < len(toIngest); i += a.batchSize {
        end := i + a.batchSize
        if end > len(toIngest) {
            end = len(toIngest)
        }
        batch := toIngest[i:end]

        resp, err := a.ingestionClient.BatchIngestPapers(ctx, &ingestionv1.BatchIngestPapersRequest{
            Papers:   batch,
            Priority: 5,
        })
        if err != nil {
            return nil, fmt.Errorf("ingestion service error: %w", err)
        }

        // Update paper statuses
        for _, job := range resp.Jobs {
            result.JobIDs[job.PaperId] = job.JobId

            status := "queued"
            if job.Status == "rejected" {
                status = "failed"
                result.FailureCount++
            } else {
                result.SuccessCount++
            }

            _ = a.repo.UpdateRequestPaperIngestionStatus(ctx, repository.IngestionStatusUpdate{
                RequestID:    input.ReviewID,
                PaperID:      job.PaperId,
                Status:       status,
                IngestionJobID: job.JobId,
            })
        }
    }

    return result, nil
}
```

## 8.7 Event Activities

```go
// internal/temporal/activities/event_activities.go

package activities

import (
    "context"

    "github.com/company/literature-review-service/internal/outbox"
    "github.com/company/literature-review-service/internal/repository"
)

type EventActivities struct {
    outbox *outbox.Publisher
    repo   repository.Repository
}

type PublishEventInput struct {
    EventType string
    ReviewID  string
    OrgID     string
    ProjectID string
    Data      map[string]interface{}
}

func (a *EventActivities) PublishEventActivity(
    ctx context.Context,
    input PublishEventInput,
) error {
    return a.outbox.Publish(ctx, outbox.Event{
        AggregateType: "literature_review",
        AggregateID:   input.ReviewID,
        EventType:     input.EventType,
        OrgID:         input.OrgID,
        ProjectID:     input.ProjectID,
        Payload:       input.Data,
    })
}

type UpdateStatusInput struct {
    ReviewID       string
    Status         string
    Depth          int
    PapersFound    int
    PapersIngested int
    ErrorMessage   string
}

func (a *EventActivities) UpdateReviewStatusActivity(
    ctx context.Context,
    input UpdateStatusInput,
) error {
    // Update DB
    err := a.repo.UpdateReviewStatus(ctx, repository.ReviewStatusUpdate{
        ReviewID:       input.ReviewID,
        Status:         input.Status,
        CurrentDepth:   input.Depth,
        PapersFound:    input.PapersFound,
        PapersIngested: input.PapersIngested,
        ErrorMessage:   input.ErrorMessage,
    })
    if err != nil {
        return err
    }

    // Insert progress event for NOTIFY
    return a.repo.InsertProgressEvent(ctx, repository.ProgressEvent{
        RequestID: input.ReviewID,
        EventType: "status_changed",
        EventData: map[string]interface{}{
            "status":          input.Status,
            "depth":           input.Depth,
            "papers_found":    input.PapersFound,
            "papers_ingested": input.PapersIngested,
        },
    })
}
```

---

# 9. External Integrations

## 9.1 Paper Source Interface

```go
// internal/papersources/client.go

package papersources

import (
    "context"
    "encoding/json"
    "time"
)

// Client interface for paper source APIs
type Client interface {
    // Search searches for papers matching the query
    Search(ctx context.Context, params SearchParams) ([]Paper, error)
    
    // GetPaper retrieves a specific paper by ID
    GetPaper(ctx context.Context, id string) (*Paper, error)
    
    // Name returns the source name
    Name() string
    
    // SupportsDateFiltering returns whether the source supports date filtering
    SupportsDateFiltering() bool
}

// SearchParams holds search parameters
type SearchParams struct {
    Query    string
    FromDate *time.Time
    ToDate   *time.Time
    Limit    int
    Offset   int
}

// Paper represents a paper from any source
type Paper struct {
    // Identifiers
    DOI               string
    ArxivID           string
    PubMedID          string
    PMCID             string
    SemanticScholarID string
    OpenAlexID        string
    ScopusID          string
    
    // Metadata
    Title           string
    Abstract        string
    Authors         []Author
    PublicationDate *time.Time
    Year            int
    Venue           string
    Journal         string
    Volume          string
    Issue           string
    Pages           string
    
    // Metrics
    CitationCount  int
    ReferenceCount int
    
    // Access
    PDFURL     string
    OpenAccess bool
    
    // Raw response for storage
    RawMetadata json.RawMessage
}

// Author represents a paper author
type Author struct {
    Name        string `json:"name"`
    Affiliation string `json:"affiliation,omitempty"`
    ORCID       string `json:"orcid,omitempty"`
}

// ToIdentifiersJSON converts paper identifiers to JSON for database upsert
func (p *Paper) ToIdentifiersJSON(source string) json.RawMessage {
    identifiers := make([]map[string]string, 0)
    
    add := func(idType, value string) {
        if value != "" {
            identifiers = append(identifiers, map[string]string{
                "type":   idType,
                "value":  value,
                "source": source,
            })
        }
    }
    
    add("doi", p.DOI)
    add("arxiv_id", p.ArxivID)
    add("pubmed_id", p.PubMedID)
    add("pmcid", p.PMCID)
    add("semantic_scholar_id", p.SemanticScholarID)
    add("openalex_id", p.OpenAlexID)
    add("scopus_id", p.ScopusID)
    
    data, _ := json.Marshal(identifiers)
    return data
}

// APIError represents an API error with status code
type APIError struct {
    StatusCode int
    Message    string
    RetryAfter int // Seconds, from Retry-After header
}

func (e *APIError) Error() string {
    return e.Message
}
```

## 9.2 Semantic Scholar Client

```go
// internal/papersources/semanticscholar/client.go

package semanticscholar

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "strconv"
    "strings"
    "time"

    "github.com/company/literature-review-service/internal/papersources"
)

type Client struct {
    httpClient *http.Client
    baseURL    string
    apiKey     string
    fields     []string
}

type Config struct {
    BaseURL   string
    APIKey    string
    Timeout   time.Duration
    Fields    []string
}

func NewClient(cfg Config) *Client {
    return &Client{
        httpClient: &http.Client{Timeout: cfg.Timeout},
        baseURL:    cfg.BaseURL,
        apiKey:     cfg.APIKey,
        fields:     cfg.Fields,
    }
}

func (c *Client) Name() string {
    return "semantic_scholar"
}

func (c *Client) SupportsDateFiltering() bool {
    return true
}

func (c *Client) Search(ctx context.Context, params papersources.SearchParams) ([]papersources.Paper, error) {
    // Build query URL
    u, _ := url.Parse(c.baseURL + "/paper/search")
    q := u.Query()
    q.Set("query", params.Query)
    q.Set("fields", strings.Join(c.fields, ","))
    
    if params.Limit > 0 {
        q.Set("limit", strconv.Itoa(params.Limit))
    } else {
        q.Set("limit", "100")
    }
    
    if params.Offset > 0 {
        q.Set("offset", strconv.Itoa(params.Offset))
    }
    
    // Date filtering
    if params.FromDate != nil || params.ToDate != nil {
        yearRange := ""
        if params.FromDate != nil {
            yearRange = strconv.Itoa(params.FromDate.Year())
        }
        yearRange += "-"
        if params.ToDate != nil {
            yearRange += strconv.Itoa(params.ToDate.Year())
        }
        q.Set("year", yearRange)
    }
    
    u.RawQuery = q.Encode()

    // Create request
    req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
    if err != nil {
        return nil, err
    }
    
    if c.apiKey != "" {
        req.Header.Set("x-api-key", c.apiKey)
    }

    // Execute request
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    // Handle errors
    if resp.StatusCode != http.StatusOK {
        return nil, c.handleError(resp)
    }

    // Parse response
    var result searchResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("decode response: %w", err)
    }

    // Convert to common format
    papers := make([]papersources.Paper, 0, len(result.Data))
    for _, p := range result.Data {
        papers = append(papers, c.toPaper(p))
    }

    return papers, nil
}

func (c *Client) GetPaper(ctx context.Context, id string) (*papersources.Paper, error) {
    u := fmt.Sprintf("%s/paper/%s?fields=%s", c.baseURL, id, strings.Join(c.fields, ","))
    
    req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
    if err != nil {
        return nil, err
    }
    
    if c.apiKey != "" {
        req.Header.Set("x-api-key", c.apiKey)
    }

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, c.handleError(resp)
    }

    var p paperResponse
    if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
        return nil, err
    }

    paper := c.toPaper(p)
    return &paper, nil
}

func (c *Client) handleError(resp *http.Response) error {
    apiErr := &papersources.APIError{
        StatusCode: resp.StatusCode,
        Message:    fmt.Sprintf("Semantic Scholar API error: %d", resp.StatusCode),
    }
    
    if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
        if seconds, err := strconv.Atoi(retryAfter); err == nil {
            apiErr.RetryAfter = seconds
        }
    }
    
    return apiErr
}

func (c *Client) toPaper(p paperResponse) papersources.Paper {
    var pubDate *time.Time
    if p.PublicationDate != "" {
        if t, err := time.Parse("2006-01-02", p.PublicationDate); err == nil {
            pubDate = &t
        }
    }
    
    authors := make([]papersources.Author, 0, len(p.Authors))
    for _, a := range p.Authors {
        authors = append(authors, papersources.Author{
            Name: a.Name,
        })
    }
    
    rawMetadata, _ := json.Marshal(p)
    
    return papersources.Paper{
        SemanticScholarID: p.PaperID,
        DOI:               p.ExternalIDs.DOI,
        ArxivID:           p.ExternalIDs.ArXiv,
        PubMedID:          p.ExternalIDs.PubMed,
        Title:             p.Title,
        Abstract:          p.Abstract,
        Authors:           authors,
        PublicationDate:   pubDate,
        Year:              p.Year,
        Venue:             p.Venue,
        CitationCount:     p.CitationCount,
        PDFURL:            p.OpenAccessPDF.URL,
        OpenAccess:        p.OpenAccessPDF.URL != "",
        RawMetadata:       rawMetadata,
    }
}

// Response types
type searchResponse struct {
    Total  int             `json:"total"`
    Offset int             `json:"offset"`
    Data   []paperResponse `json:"data"`
}

type paperResponse struct {
    PaperID         string      `json:"paperId"`
    ExternalIDs     externalIDs `json:"externalIds"`
    Title           string      `json:"title"`
    Abstract        string      `json:"abstract"`
    Year            int         `json:"year"`
    PublicationDate string      `json:"publicationDate"`
    Venue           string      `json:"venue"`
    CitationCount   int         `json:"citationCount"`
    Authors         []authorResponse `json:"authors"`
    OpenAccessPDF   openAccessPDF `json:"openAccessPdf"`
}

type externalIDs struct {
    DOI    string `json:"DOI"`
    ArXiv  string `json:"ArXiv"`
    PubMed string `json:"PubMed"`
}

type authorResponse struct {
    AuthorID string `json:"authorId"`
    Name     string `json:"name"`
}

type openAccessPDF struct {
    URL    string `json:"url"`
    Status string `json:"status"`
}
```

## 9.3 LLM Client Interface

```go
// internal/llm/client.go

package llm

import (
    "context"
)

// ExtractionType indicates the type of text being processed
type ExtractionType string

const (
    ExtractionTypeQuery ExtractionType = "query"
    ExtractionTypePaper ExtractionType = "paper"
)

// Client interface for LLM providers
type Client interface {
    // ExtractKeywords extracts keywords from text
    ExtractKeywords(ctx context.Context, input ExtractionInput) ([]string, error)
}

// ExtractionInput holds input for keyword extraction
type ExtractionInput struct {
    Text        string
    MaxKeywords int
    Type        ExtractionType
}

// Config holds LLM client configuration
type Config struct {
    Provider    string
    Model       string
    APIKey      string
    BaseURL     string
    Timeout     time.Duration
    MaxRetries  int
    Temperature float32
}
```

## 9.4 OpenAI Client

```go
// internal/llm/openai/client.go

package openai

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"

    "github.com/sashabaranov/go-openai"

    "github.com/company/literature-review-service/internal/llm"
)

type Client struct {
    client      *openai.Client
    model       string
    temperature float32
}

func NewClient(cfg llm.Config) *Client {
    config := openai.DefaultConfig(cfg.APIKey)
    if cfg.BaseURL != "" {
        config.BaseURL = cfg.BaseURL
    }

    return &Client{
        client:      openai.NewClientWithConfig(config),
        model:       cfg.Model,
        temperature: cfg.Temperature,
    }
}

func (c *Client) ExtractKeywords(ctx context.Context, input llm.ExtractionInput) ([]string, error) {
    systemPrompt := c.getSystemPrompt(input.Type)
    userPrompt := c.getUserPrompt(input)

    resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
        Model: c.model,
        Messages: []openai.ChatCompletionMessage{
            {Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
            {Role: openai.ChatMessageRoleUser, Content: userPrompt},
        },
        Temperature: c.temperature,
        ResponseFormat: &openai.ChatCompletionResponseFormat{
            Type: openai.ChatCompletionResponseFormatTypeJSONObject,
        },
    })
    if err != nil {
        return nil, fmt.Errorf("OpenAI API error: %w", err)
    }

    if len(resp.Choices) == 0 {
        return nil, fmt.Errorf("no response from OpenAI")
    }

    // Parse JSON response
    var result struct {
        Keywords []string `json:"keywords"`
    }
    if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
        return nil, fmt.Errorf("parse response: %w", err)
    }

    // Normalize keywords
    keywords := make([]string, 0, len(result.Keywords))
    for _, kw := range result.Keywords {
        kw = strings.TrimSpace(strings.ToLower(kw))
        if kw != "" {
            keywords = append(keywords, kw)
        }
    }

    return keywords, nil
}

func (c *Client) getSystemPrompt(extractionType llm.ExtractionType) string {
    switch extractionType {
    case llm.ExtractionTypeQuery:
        return `You are a research assistant specializing in academic literature search.
Your task is to extract the most relevant search keywords from a natural language research query.
Focus on:
- Key scientific concepts and terminology
- Specific methods, techniques, or algorithms
- Important entities (proteins, diseases, materials, etc.)
- Domain-specific terms

Return keywords that would be effective for searching academic databases like PubMed, Semantic Scholar, and arXiv.
Output as JSON with a "keywords" array.`

    case llm.ExtractionTypePaper:
        return `You are a research assistant analyzing academic papers.
Your task is to extract the most important keywords from a paper's title and abstract.
Focus on:
- Core research topics and concepts
- Methods and techniques used
- Key findings or contributions
- Domain-specific terminology

These keywords will be used to find related papers.
Output as JSON with a "keywords" array.`

    default:
        return ""
    }
}

func (c *Client) getUserPrompt(input llm.ExtractionInput) string {
    return fmt.Sprintf(`Extract exactly %d keywords from the following text:

%s

Return as JSON: {"keywords": ["keyword1", "keyword2", ...]}`, input.MaxKeywords, input.Text)
}
```

---

# 10. API Layer

## 10.1 gRPC Server

```go
// internal/server/grpc/server.go

package grpc

import (
    "context"
    "crypto/tls"
    "crypto/x509"
    "fmt"
    "net"
    "os"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"

    "github.com/company/grpcauth"
    literaturereviewv1 "github.com/company/literature-review-service/gen/proto/literaturereview/v1"
    "github.com/company/literature-review-service/internal/config"
    "github.com/company/literature-review-service/internal/service"
)

type Server struct {
    literaturereviewv1.UnimplementedLiteratureReviewServiceServer
    
    config  *config.Config
    service *service.LiteratureReviewService
}

func NewServer(cfg *config.Config, svc *service.LiteratureReviewService) *Server {
    return &Server{
        config:  cfg,
        service: svc,
    }
}

func (s *Server) Start(ctx context.Context) error {
    // Load mTLS config
    tlsConfig, err := s.loadMTLSConfig()
    if err != nil {
        return fmt.Errorf("load mTLS config: %w", err)
    }

    // Create gRPC server with interceptors
    grpcServer := grpc.NewServer(
        grpc.Creds(credentials.NewTLS(tlsConfig)),
        grpc.ChainUnaryInterceptor(
            grpcauth.UnaryJWTAuthInterceptor(grpcauth.JWTConfig{
                Issuer:   s.config.Auth.JWT.Issuer,
                Audience: s.config.Auth.JWT.Audience,
                JWKSURL:  s.config.Auth.JWT.JWKSURL,
            }),
            grpcauth.UnaryAuthorizationInterceptor(),
            grpcauth.UnaryLoggingInterceptor(),
            grpcauth.UnaryMetricsInterceptor(),
        ),
        grpc.ChainStreamInterceptor(
            grpcauth.StreamJWTAuthInterceptor(grpcauth.JWTConfig{
                Issuer:   s.config.Auth.JWT.Issuer,
                Audience: s.config.Auth.JWT.Audience,
                JWKSURL:  s.config.Auth.JWT.JWKSURL,
            }),
            grpcauth.StreamAuthorizationInterceptor(),
            grpcauth.StreamLoggingInterceptor(),
        ),
    )

    literaturereviewv1.RegisterLiteratureReviewServiceServer(grpcServer, s)

    lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.config.Service.GRPCPort))
    if err != nil {
        return fmt.Errorf("listen: %w", err)
    }

    go func() {
        <-ctx.Done()
        grpcServer.GracefulStop()
    }()

    return grpcServer.Serve(lis)
}

func (s *Server) loadMTLSConfig() (*tls.Config, error) {
    // Load server certificate
    serverCert, err := tls.LoadX509KeyPair(
        s.config.Auth.MTLS.ServerCertPath,
        s.config.Auth.MTLS.ServerKeyPath,
    )
    if err != nil {
        return nil, fmt.Errorf("load server cert: %w", err)
    }

    // Load CA cert for client verification
    caCert, err := os.ReadFile(s.config.Auth.MTLS.ClientCAPath)
    if err != nil {
        return nil, fmt.Errorf("read client CA: %w", err)
    }

    caCertPool := x509.NewCertPool()
    if !caCertPool.AppendCertsFromPEM(caCert) {
        return nil, fmt.Errorf("failed to parse client CA cert")
    }

    return &tls.Config{
        Certificates: []tls.Certificate{serverCert},
        ClientCAs:    caCertPool,
        ClientAuth:   tls.RequireAndVerifyClientCert,
        MinVersion:   tls.VersionTLS13,
    }, nil
}
```

## 10.2 gRPC Handlers

```go
// internal/server/grpc/handlers.go

package grpc

import (
    "context"

    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
    "google.golang.org/protobuf/types/known/durationpb"
    "google.golang.org/protobuf/types/known/timestamppb"

    "github.com/company/grpcauth"
    literaturereviewv1 "github.com/company/literature-review-service/gen/proto/literaturereview/v1"
    "github.com/company/literature-review-service/internal/service"
)

func (s *Server) StartLiteratureReview(
    ctx context.Context,
    req *literaturereviewv1.StartLiteratureReviewRequest,
) (*literaturereviewv1.StartLiteratureReviewResponse, error) {
    // Extract user from context (set by grpcauth middleware)
    userID, err := grpcauth.GetUserID(ctx)
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, "user not authenticated")
    }

    // Validate tenant access
    if err := s.validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
        return nil, err
    }

    // Build service request
    svcReq := service.StartReviewRequest{
        OrgID:               req.OrgId,
        ProjectID:           req.ProjectId,
        UserID:              userID,
        Query:               req.Query,
        InitialKeywordCount: int(req.GetInitialKeywordCount().GetValue()),
        PaperKeywordCount:   int(req.GetPaperKeywordCount().GetValue()),
        MaxExpansionDepth:   int(req.GetMaxExpansionDepth().GetValue()),
        SourceFilters:       req.SourceFilters,
    }

    if req.DateFrom != nil {
        t := req.DateFrom.AsTime()
        svcReq.DateFrom = &t
    }
    if req.DateTo != nil {
        t := req.DateTo.AsTime()
        svcReq.DateTo = &t
    }

    // Start review
    result, err := s.service.StartReview(ctx, svcReq)
    if err != nil {
        return nil, toGRPCError(err)
    }

    return &literaturereviewv1.StartLiteratureReviewResponse{
        ReviewId:        result.ReviewID,
        WorkflowId:      result.WorkflowID,
        Status:          toProtoStatus(result.Status),
        InitialKeywords: result.InitialKeywords,
        CreatedAt:       timestamppb.New(result.CreatedAt),
        Message:         "Literature review started successfully",
    }, nil
}

func (s *Server) GetLiteratureReviewStatus(
    ctx context.Context,
    req *literaturereviewv1.GetLiteratureReviewStatusRequest,
) (*literaturereviewv1.GetLiteratureReviewStatusResponse, error) {
    if err := s.validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
        return nil, err
    }

    result, err := s.service.GetReviewStatus(ctx, service.GetStatusRequest{
        OrgID:     req.OrgId,
        ProjectID: req.ProjectId,
        ReviewID:  req.ReviewId,
    })
    if err != nil {
        return nil, toGRPCError(err)
    }

    resp := &literaturereviewv1.GetLiteratureReviewStatusResponse{
        ReviewId:     result.ReviewID,
        Status:       toProtoStatus(result.Status),
        ErrorMessage: result.ErrorMessage,
        CreatedAt:    timestamppb.New(result.CreatedAt),
        Progress:     toProtoProgress(result.Progress),
    }

    if result.StartedAt != nil {
        resp.StartedAt = timestamppb.New(*result.StartedAt)
    }
    if result.CompletedAt != nil {
        resp.CompletedAt = timestamppb.New(*result.CompletedAt)
    }
    if result.Duration > 0 {
        resp.Duration = durationpb.New(result.Duration)
    }

    return resp, nil
}

func (s *Server) StreamLiteratureReviewProgress(
    req *literaturereviewv1.StreamLiteratureReviewProgressRequest,
    stream literaturereviewv1.LiteratureReviewService_StreamLiteratureReviewProgressServer,
) error {
    ctx := stream.Context()

    if err := s.validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
        return err
    }

    // Create progress channel
    progressCh, err := s.service.StreamProgress(ctx, service.StreamProgressRequest{
        OrgID:     req.OrgId,
        ProjectID: req.ProjectId,
        ReviewID:  req.ReviewId,
    })
    if err != nil {
        return toGRPCError(err)
    }

    for {
        select {
        case <-ctx.Done():
            return nil
        case event, ok := <-progressCh:
            if !ok {
                return nil
            }

            if err := stream.Send(toProtoProgressEvent(event)); err != nil {
                return err
            }
        }
    }
}

func (s *Server) validateTenantAccess(ctx context.Context, orgID, projectID string) error {
    // Get user's allowed orgs/projects from JWT claims
    claims, err := grpcauth.GetClaims(ctx)
    if err != nil {
        return status.Error(codes.Unauthenticated, "invalid token")
    }

    // Check if user has access to this org/project
    if !grpcauth.HasOrgAccess(claims, orgID) {
        return status.Error(codes.PermissionDenied, "access denied to organization")
    }

    if !grpcauth.HasProjectAccess(claims, orgID, projectID) {
        return status.Error(codes.PermissionDenied, "access denied to project")
    }

    return nil
}

// Helper functions for proto conversion
func toProtoStatus(s string) literaturereviewv1.ReviewStatus {
    switch s {
    case "pending":
        return literaturereviewv1.ReviewStatus_REVIEW_STATUS_PENDING
    case "extracting_keywords":
        return literaturereviewv1.ReviewStatus_REVIEW_STATUS_EXTRACTING_KEYWORDS
    case "searching":
        return literaturereviewv1.ReviewStatus_REVIEW_STATUS_SEARCHING
    case "expanding":
        return literaturereviewv1.ReviewStatus_REVIEW_STATUS_EXPANDING
    case "ingesting":
        return literaturereviewv1.ReviewStatus_REVIEW_STATUS_INGESTING
    case "completed":
        return literaturereviewv1.ReviewStatus_REVIEW_STATUS_COMPLETED
    case "failed":
        return literaturereviewv1.ReviewStatus_REVIEW_STATUS_FAILED
    case "cancelled":
        return literaturereviewv1.ReviewStatus_REVIEW_STATUS_CANCELLED
    default:
        return literaturereviewv1.ReviewStatus_REVIEW_STATUS_UNSPECIFIED
    }
}

func toProtoProgress(p *service.ReviewProgress) *literaturereviewv1.ReviewProgress {
    if p == nil {
        return nil
    }

    sourceProgress := make(map[string]*literaturereviewv1.SourceProgress)
    for source, sp := range p.SourceProgress {
        sourceProgress[source] = &literaturereviewv1.SourceProgress{
            SourceName:       source,
            QueriesCompleted: int32(sp.QueriesCompleted),
            QueriesTotal:     int32(sp.QueriesTotal),
            PapersFound:      int32(sp.PapersFound),
            Errors:           int32(sp.Errors),
        }
    }

    return &literaturereviewv1.ReviewProgress{
        InitialKeywordsCount:   int32(p.InitialKeywordsCount),
        TotalKeywordsProcessed: int32(p.TotalKeywordsProcessed),
        PapersFound:            int32(p.PapersFound),
        PapersNew:              int32(p.PapersNew),
        PapersIngested:         int32(p.PapersIngested),
        PapersFailed:           int32(p.PapersFailed),
        CurrentExpansionDepth:  int32(p.CurrentExpansionDepth),
        MaxExpansionDepth:      int32(p.MaxExpansionDepth),
        SourceProgress:         sourceProgress,
    }
}

func toProtoProgressEvent(e service.ProgressEvent) *literaturereviewv1.LiteratureReviewProgressEvent {
    return &literaturereviewv1.LiteratureReviewProgressEvent{
        ReviewId:  e.ReviewID,
        EventType: e.EventType,
        Status:    toProtoStatus(e.Status),
        Progress:  toProtoProgress(e.Progress),
        Message:   e.Message,
        Timestamp: timestamppb.New(e.Timestamp),
    }
}

func toGRPCError(err error) error {
    // Map domain errors to gRPC status codes
    switch {
    case service.IsNotFoundError(err):
        return status.Error(codes.NotFound, err.Error())
    case service.IsValidationError(err):
        return status.Error(codes.InvalidArgument, err.Error())
    case service.IsAuthorizationError(err):
        return status.Error(codes.PermissionDenied, err.Error())
    default:
        return status.Error(codes.Internal, err.Error())
    }
}
```

## 10.3 HTTP Server with Proper mTLS

```go
// internal/server/http/server.go

package http

import (
    "context"
    "crypto/tls"
    "crypto/x509"
    "fmt"
    "net/http"
    "os"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"

    "github.com/company/grpcauth"
    "github.com/company/literature-review-service/internal/config"
    "github.com/company/literature-review-service/internal/service"
)

type Server struct {
    config  *config.Config
    service *service.LiteratureReviewService
}

func NewServer(cfg *config.Config, svc *service.LiteratureReviewService) *Server {
    return &Server{
        config:  cfg,
        service: svc,
    }
}

func (s *Server) Start(ctx context.Context) error {
    r := chi.NewRouter()

    // Standard middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Recoverer)
    r.Use(middleware.Timeout(time.Duration(s.config.Service.RequestTimeoutSeconds) * time.Second))
    r.Use(correlationIDMiddleware)

    // Auth middleware from grpcauth package
    r.Use(grpcauth.HTTPJWTAuthMiddleware(grpcauth.JWTConfig{
        Issuer:   s.config.Auth.JWT.Issuer,
        Audience: s.config.Auth.JWT.Audience,
        JWKSURL:  s.config.Auth.JWT.JWKSURL,
    }))

    // API routes with tenant context
    r.Route("/api/v1/orgs/{orgID}/projects/{projectID}", func(r chi.Router) {
        r.Use(tenantContextMiddleware)
        r.Use(grpcauth.HTTPAuthorizationMiddleware())

        r.Post("/literature-reviews", s.StartLiteratureReview)
        r.Get("/literature-reviews/{reviewID}", s.GetLiteratureReviewStatus)
        r.Delete("/literature-reviews/{reviewID}", s.CancelLiteratureReview)
        r.Get("/literature-reviews", s.ListLiteratureReviews)
        r.Get("/literature-reviews/{reviewID}/papers", s.GetLiteratureReviewPapers)
        r.Get("/literature-reviews/{reviewID}/keywords", s.GetLiteratureReviewKeywords)
        r.Get("/literature-reviews/{reviewID}/progress", s.StreamProgress)
    })

    // Health endpoints (no auth)
    r.Get("/health", s.healthHandler)
    r.Get("/health/live", s.livenessHandler)
    r.Get("/health/ready", s.readinessHandler)

    // Build TLS config
    tlsConfig, err := s.buildTLSConfig()
    if err != nil {
        return fmt.Errorf("build TLS config: %w", err)
    }

    server := &http.Server{
        Addr:         fmt.Sprintf(":%d", s.config.Service.HTTPPort),
        Handler:      r,
        TLSConfig:    tlsConfig,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 300 * time.Second,
        IdleTimeout:  120 * time.Second,
    }

    // Graceful shutdown
    go func() {
        <-ctx.Done()
        shutdownCtx, cancel := context.WithTimeout(
            context.Background(),
            time.Duration(s.config.Service.ShutdownTimeoutSeconds)*time.Second,
        )
        defer cancel()
        _ = server.Shutdown(shutdownCtx)
    }()

    // Use empty strings to use certs from TLSConfig
    return server.ListenAndServeTLS("", "")
}

func (s *Server) buildTLSConfig() (*tls.Config, error) {
    // Load server certificate
    serverCert, err := tls.LoadX509KeyPair(
        s.config.Auth.MTLS.ServerCertPath,
        s.config.Auth.MTLS.ServerKeyPath,
    )
    if err != nil {
        return nil, fmt.Errorf("load server cert: %w", err)
    }

    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{serverCert},
        MinVersion:   tls.VersionTLS13,
    }

    // FIXED: Properly enforce mTLS if configured
    if s.config.Auth.MTLS.EnforceHTTPMTLS {
        caCert, err := os.ReadFile(s.config.Auth.MTLS.ClientCAPath)
        if err != nil {
            return nil, fmt.Errorf("read client CA: %w", err)
        }

        caCertPool := x509.NewCertPool()
        if !caCertPool.AppendCertsFromPEM(caCert) {
            return nil, fmt.Errorf("failed to parse client CA cert")
        }

        tlsConfig.ClientCAs = caCertPool
        tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
    }

    return tlsConfig, nil
}

// Middleware

func tenantContextMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        orgID := chi.URLParam(r, "orgID")
        projectID := chi.URLParam(r, "projectID")

        if orgID == "" || projectID == "" {
            http.Error(w, `{"error": "org_id and project_id are required"}`, http.StatusBadRequest)
            return
        }

        ctx := context.WithValue(r.Context(), ctxKeyOrgID, orgID)
        ctx = context.WithValue(ctx, ctxKeyProjectID, projectID)

        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func correlationIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        correlationID := r.Header.Get("X-Correlation-ID")
        if correlationID == "" {
            correlationID = middleware.GetReqID(r.Context())
        }

        ctx := context.WithValue(r.Context(), ctxKeyCorrelationID, correlationID)
        w.Header().Set("X-Correlation-ID", correlationID)

        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

type contextKey string

const (
    ctxKeyOrgID         contextKey = "org_id"
    ctxKeyProjectID     contextKey = "project_id"
    ctxKeyCorrelationID contextKey = "correlation_id"
)
```

## 10.4 Progress Streaming (SSE)

```go
// internal/server/http/progress_stream.go

package http

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/company/literature-review-service/internal/service"
)

func (s *Server) StreamProgress(w http.ResponseWriter, r *http.Request) {
    reviewID := chi.URLParam(r, "reviewID")
    orgID := r.Context().Value(ctxKeyOrgID).(string)
    projectID := r.Context().Value(ctxKeyProjectID).(string)

    // Verify access
    review, err := s.service.GetReviewByID(r.Context(), reviewID)
    if err != nil {
        http.Error(w, `{"error": "Review not found"}`, http.StatusNotFound)
        return
    }
    if review.OrgID != orgID || review.ProjectID != projectID {
        http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
        return
    }

    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no")

    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, `{"error": "Streaming not supported"}`, http.StatusInternalServerError)
        return
    }

    ctx, cancel := context.WithCancel(r.Context())
    defer cancel()

    // Create channels for different event sources
    notifyChan := make(chan ProgressEvent, 100)
    defer close(notifyChan)

    // Start listening for Postgres NOTIFY events
    go s.listenForProgressNotifications(ctx, reviewID, notifyChan)

    // Poll Temporal workflow query periodically
    queryTicker := time.NewTicker(2 * time.Second)
    defer queryTicker.Stop()

    // Send initial state
    if err := s.sendWorkflowProgress(w, flusher, review.TemporalWorkflowID); err != nil {
        return
    }

    for {
        select {
        case <-ctx.Done():
            return

        case event := <-notifyChan:
            // Real-time event from Postgres NOTIFY
            data, _ := json.Marshal(event)
            fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data)
            flusher.Flush()

            // Check if terminal state
            if event.EventType == "completed" || event.EventType == "failed" || event.EventType == "cancelled" {
                return
            }

        case <-queryTicker.C:
            // Poll Temporal for authoritative state
            if err := s.sendWorkflowProgress(w, flusher, review.TemporalWorkflowID); err != nil {
                // Workflow might be completed
                return
            }
        }
    }
}

type ProgressEvent struct {
    EventType string                 `json:"event_type"`
    Data      map[string]interface{} `json:"data"`
    Timestamp time.Time              `json:"timestamp"`
}

func (s *Server) listenForProgressNotifications(ctx context.Context, reviewID string, out chan<- ProgressEvent) {
    conn, err := s.service.AcquireDBConn(ctx)
    if err != nil {
        return
    }
    defer conn.Release()

    channel := fmt.Sprintf("review_progress_%s", reviewID)
    _, err = conn.Exec(ctx, fmt.Sprintf("LISTEN %s", channel))
    if err != nil {
        return
    }
    defer conn.Exec(context.Background(), fmt.Sprintf("UNLISTEN %s", channel))

    for {
        notification, err := conn.Conn().WaitForNotification(ctx)
        if err != nil {
            return
        }

        var event ProgressEvent
        if err := json.Unmarshal([]byte(notification.Payload), &event); err != nil {
            continue
        }

        select {
        case out <- event:
        case <-ctx.Done():
            return
        default:
            // Channel full, skip
        }
    }
}

func (s *Server) sendWorkflowProgress(w http.ResponseWriter, flusher http.Flusher, workflowID string) error {
    state, err := s.service.QueryWorkflowProgress(context.Background(), workflowID)
    if err != nil {
        return err
    }

    data, _ := json.Marshal(map[string]interface{}{
        "event_type": "state",
        "data":       state,
        "timestamp":  time.Now(),
    })

    fmt.Fprintf(w, "event: state\ndata: %s\n\n", data)
    flusher.Flush()

    return nil
}
```

---

# 11. Outbox & Event Publishing

## 11.1 Event Types

```go
// internal/outbox/events.go

package outbox

import (
    "time"
)

// Event types published by this service
const (
    EventReviewStarted       = "literature_review.started"
    EventReviewCompleted     = "literature_review.completed"
    EventReviewFailed        = "literature_review.failed"
    EventReviewCancelled     = "literature_review.cancelled"
    EventKeywordsExtracted   = "literature_review.keywords_extracted"
    EventPapersDiscovered    = "literature_review.papers_discovered"
    EventSearchCompleted     = "literature_review.search_completed"
    EventExpansionCompleted  = "literature_review.expansion_completed"
    EventIngestionStarted    = "literature_review.ingestion_started"
    EventIngestionCompleted  = "literature_review.ingestion_completed"
)

// Event represents an outbox event
type Event struct {
    AggregateType string
    AggregateID   string
    EventType     string
    OrgID         string
    ProjectID     string
    Payload       interface{}
    Metadata      map[string]string
}

// ReviewStartedEvent is published when a literature review begins
type ReviewStartedEvent struct {
    ReviewID       string    `json:"review_id"`
    OrgID          string    `json:"org_id"`
    ProjectID      string    `json:"project_id"`
    UserID         string    `json:"user_id"`
    Query          string    `json:"query"`
    MaxDepth       int       `json:"max_depth"`
    EnabledSources []string  `json:"enabled_sources"`
    StartedAt      time.Time `json:"started_at"`
}

// ReviewCompletedEvent is published when a literature review finishes
type ReviewCompletedEvent struct {
    ReviewID       string        `json:"review_id"`
    OrgID          string        `json:"org_id"`
    ProjectID      string        `json:"project_id"`
    UserID         string        `json:"user_id"`
    TotalPapers    int           `json:"total_papers"`
    IngestedPapers int           `json:"ingested_papers"`
    TotalKeywords  int           `json:"total_keywords"`
    ExpansionDepth int           `json:"expansion_depth"`
    DurationMs     int64         `json:"duration_ms"`
    CompletedAt    time.Time     `json:"completed_at"`
}

// ReviewFailedEvent is published when a literature review fails
type ReviewFailedEvent struct {
    ReviewID    string    `json:"review_id"`
    OrgID       string    `json:"org_id"`
    ProjectID   string    `json:"project_id"`
    Phase       string    `json:"phase"`
    Error       string    `json:"error"`
    FailedAt    time.Time `json:"failed_at"`
}

// PapersDiscoveredEvent is published when papers are found
type PapersDiscoveredEvent struct {
    ReviewID      string   `json:"review_id"`
    OrgID         string   `json:"org_id"`
    ProjectID     string   `json:"project_id"`
    Source        string   `json:"source"`
    Keyword       string   `json:"keyword"`
    PaperIDs      []string `json:"paper_ids"`
    Count         int      `json:"count"`
    IsIncremental bool     `json:"is_incremental"`
}

// KeywordsExtractedEvent is published when keywords are extracted
type KeywordsExtractedEvent struct {
    ReviewID        string   `json:"review_id"`
    OrgID           string   `json:"org_id"`
    ProjectID       string   `json:"project_id"`
    Keywords        []string `json:"keywords"`
    ExtractionRound int      `json:"extraction_round"`
    SourcePaperID   string   `json:"source_paper_id,omitempty"`
}
```

## 11.2 Publisher

```go
// internal/outbox/publisher.go

package outbox

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"

    sharedoutbox "github.com/company/outbox"
)

// Publisher handles event publishing via outbox
type Publisher struct {
    outbox sharedoutbox.Outbox
    db     *sql.DB
}

// NewPublisher creates a new event publisher
func NewPublisher(db *sql.DB, cfg sharedoutbox.Config) (*Publisher, error) {
    ob, err := sharedoutbox.New(db, cfg)
    if err != nil {
        return nil, err
    }
    return &Publisher{outbox: ob, db: db}, nil
}

// Publish publishes an event to the outbox
func (p *Publisher) Publish(ctx context.Context, event Event) error {
    metadata := event.Metadata
    if metadata == nil {
        metadata = make(map[string]string)
    }
    
    // Add trace context
    if traceID := ctx.Value("trace_id"); traceID != nil {
        metadata["trace_id"] = traceID.(string)
    }
    if correlationID := ctx.Value("correlation_id"); correlationID != nil {
        metadata["correlation_id"] = correlationID.(string)
    }

    return p.outbox.Publish(ctx, sharedoutbox.Event{
        AggregateType: event.AggregateType,
        AggregateID:   event.AggregateID,
        EventType:     event.EventType,
        OrgID:         event.OrgID,
        ProjectID:     event.ProjectID,
        Payload:       event.Payload,
        Metadata:      metadata,
    })
}

// PublishReviewStarted publishes a review started event
func (p *Publisher) PublishReviewStarted(ctx context.Context, event ReviewStartedEvent) error {
    return p.Publish(ctx, Event{
        AggregateType: "literature_review",
        AggregateID:   event.ReviewID,
        EventType:     EventReviewStarted,
        OrgID:         event.OrgID,
        ProjectID:     event.ProjectID,
        Payload:       event,
    })
}

// PublishReviewCompleted publishes a review completed event
func (p *Publisher) PublishReviewCompleted(ctx context.Context, event ReviewCompletedEvent) error {
    return p.Publish(ctx, Event{
        AggregateType: "literature_review",
        AggregateID:   event.ReviewID,
        EventType:     EventReviewCompleted,
        OrgID:         event.OrgID,
        ProjectID:     event.ProjectID,
        Payload:       event,
    })
}

// PublishReviewFailed publishes a review failed event
func (p *Publisher) PublishReviewFailed(ctx context.Context, event ReviewFailedEvent) error {
    return p.Publish(ctx, Event{
        AggregateType: "literature_review",
        AggregateID:   event.ReviewID,
        EventType:     EventReviewFailed,
        OrgID:         event.OrgID,
        ProjectID:     event.ProjectID,
        Payload:       event,
    })
}

// PublishPapersDiscovered publishes a papers discovered event
func (p *Publisher) PublishPapersDiscovered(ctx context.Context, event PapersDiscoveredEvent) error {
    return p.Publish(ctx, Event{
        AggregateType: "literature_review",
        AggregateID:   event.ReviewID,
        EventType:     EventPapersDiscovered,
        OrgID:         event.OrgID,
        ProjectID:     event.ProjectID,
        Payload:       event,
    })
}

// StartWorker starts the outbox polling worker
func (p *Publisher) StartWorker(ctx context.Context) error {
    return p.outbox.StartWorker(ctx)
}
```

## 11.3 Transactional Publishing

```go
// internal/outbox/transactional.go

package outbox

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
)

// TransactionalPublisher allows publishing events within a database transaction
type TransactionalPublisher struct {
    db *sql.DB
}

// NewTransactionalPublisher creates a new transactional publisher
func NewTransactionalPublisher(db *sql.DB) *TransactionalPublisher {
    return &TransactionalPublisher{db: db}
}

// PublishInTx publishes an event as part of an existing transaction
func (p *TransactionalPublisher) PublishInTx(ctx context.Context, tx *sql.Tx, event Event) error {
    payload, err := json.Marshal(event.Payload)
    if err != nil {
        return fmt.Errorf("marshal payload: %w", err)
    }

    metadata, err := json.Marshal(event.Metadata)
    if err != nil {
        return fmt.Errorf("marshal metadata: %w", err)
    }

    _, err = tx.ExecContext(ctx, `
        INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, org_id, project_id, payload, metadata)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `, event.AggregateType, event.AggregateID, event.EventType, event.OrgID, event.ProjectID, payload, metadata)

    return err
}

// WithTransaction executes a function within a transaction and publishes events atomically
func (p *TransactionalPublisher) WithTransaction(
    ctx context.Context,
    fn func(tx *sql.Tx) ([]Event, error),
) error {
    tx, err := p.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    events, err := fn(tx)
    if err != nil {
        return err
    }

    // Publish all events in same transaction
    for _, event := range events {
        if err := p.PublishInTx(ctx, tx, event); err != nil {
            return err
        }
    }

    return tx.Commit()
}
```

---

# 12. Observability

## 12.1 Metrics

```go
// internal/observability/metrics.go

package observability

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Workflow metrics
    WorkflowsStarted = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "literature_review_workflows_started_total",
            Help: "Total workflows started",
        },
        []string{"org_id"},
    )

    WorkflowsCompleted = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "literature_review_workflows_completed_total",
            Help: "Total workflows completed",
        },
        []string{"org_id", "status"},
    )

    WorkflowDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "literature_review_workflow_duration_seconds",
            Help:    "Workflow duration by status",
            Buckets: prometheus.ExponentialBuckets(10, 2, 12), // 10s to ~11h
        },
        []string{"status"},
    )

    // Paper search metrics
    PaperSearchRequests = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "literature_review_paper_search_requests_total",
            Help: "Total paper search requests by source and status",
        },
        []string{"source", "status"},
    )

    PaperSearchLatency = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "literature_review_paper_search_latency_seconds",
            Help:    "Paper search latency by source",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
        },
        []string{"source"},
    )

    PapersDiscovered = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "literature_review_papers_discovered_total",
            Help: "Total papers discovered by source",
        },
        []string{"source", "org_id"},
    )

    // Rate limiting metrics
    RateLimitBackoffs = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "literature_review_rate_limit_backoffs_total",
            Help: "Number of rate limit backoffs by source",
        },
        []string{"source"},
    )

    RateLimitWaitTime = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "literature_review_rate_limit_wait_seconds",
            Help:    "Time spent waiting for rate limiter",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
        },
        []string{"source"},
    )

    // LLM metrics
    LLMRequests = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "literature_review_llm_requests_total",
            Help: "Total LLM requests by provider and status",
        },
        []string{"provider", "status"},
    )

    LLMLatency = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "literature_review_llm_latency_seconds",
            Help:    "LLM request latency",
            Buckets: prometheus.ExponentialBuckets(0.5, 2, 8),
        },
        []string{"provider", "operation"},
    )

    LLMTokensUsed = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "literature_review_llm_tokens_used_total",
            Help: "Total LLM tokens used",
        },
        []string{"provider", "token_type"},
    )

    // Ingestion metrics
    IngestionRequests = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "literature_review_ingestion_requests_total",
            Help: "Total ingestion requests by status",
        },
        []string{"status"},
    )

    // Outbox metrics
    OutboxEventsPublished = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "literature_review_outbox_events_published_total",
            Help: "Total outbox events published by type",
        },
        []string{"event_type"},
    )

    OutboxPublishLatency = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "literature_review_outbox_publish_latency_seconds",
            Help:    "Outbox event publish latency",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
        },
    )

    OutboxQueueSize = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "literature_review_outbox_queue_size",
            Help: "Current number of unpublished outbox events",
        },
    )

    // Database metrics
    DBQueryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "literature_review_db_query_duration_seconds",
            Help:    "Database query duration",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
        },
        []string{"operation"},
    )

    DBConnectionsOpen = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "literature_review_db_connections_open",
            Help: "Number of open database connections",
        },
    )
)
```

## 12.2 Tracing

```go
// internal/observability/tracing.go

package observability

import (
    "context"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/propagation"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
    "go.opentelemetry.io/otel/trace"
)

type TracingConfig struct {
    Enabled      bool
    Endpoint     string
    Insecure     bool
    SampleRate   float64
    ServiceName  string
    Environment  string
    Version      string
}

func InitTracing(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
    if !cfg.Enabled {
        return func(context.Context) error { return nil }, nil
    }

    // Create OTLP exporter
    opts := []otlptracegrpc.Option{
        otlptracegrpc.WithEndpoint(cfg.Endpoint),
    }
    if cfg.Insecure {
        opts = append(opts, otlptracegrpc.WithInsecure())
    }

    exporter, err := otlptracegrpc.New(ctx, opts...)
    if err != nil {
        return nil, err
    }

    // Create resource
    res, err := resource.Merge(
        resource.Default(),
        resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceName(cfg.ServiceName),
            semconv.ServiceVersion(cfg.Version),
            semconv.DeploymentEnvironment(cfg.Environment),
        ),
    )
    if err != nil {
        return nil, err
    }

    // Create tracer provider
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(res),
        sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SampleRate)),
    )

    otel.SetTracerProvider(tp)
    otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{},
        propagation.Baggage{},
    ))

    return tp.Shutdown, nil
}

// Tracer returns a tracer for the given component
func Tracer(component string) trace.Tracer {
    return otel.Tracer("literature-review-service/" + component)
}

// Common attribute keys
var (
    AttrReviewID   = attribute.Key("review.id")
    AttrOrgID      = attribute.Key("org.id")
    AttrProjectID  = attribute.Key("project.id")
    AttrUserID     = attribute.Key("user.id")
    AttrSource     = attribute.Key("paper.source")
    AttrKeyword    = attribute.Key("search.keyword")
    AttrDepth      = attribute.Key("expansion.depth")
    AttrPaperCount = attribute.Key("paper.count")
)
```

## 12.3 Structured Logging

```go
// internal/observability/logging.go

package observability

import (
    "context"
    "os"

    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

type LoggingConfig struct {
    Level         string
    Format        string
    DefaultFields map[string]string
}

var logger *zap.Logger

func InitLogging(cfg LoggingConfig) error {
    var level zapcore.Level
    if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
        level = zapcore.InfoLevel
    }

    var encoder zapcore.Encoder
    encoderConfig := zap.NewProductionEncoderConfig()
    encoderConfig.TimeKey = "timestamp"
    encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

    if cfg.Format == "json" {
        encoder = zapcore.NewJSONEncoder(encoderConfig)
    } else {
        encoder = zapcore.NewConsoleEncoder(encoderConfig)
    }

    core := zapcore.NewCore(
        encoder,
        zapcore.AddSync(os.Stdout),
        level,
    )

    // Add default fields
    fields := make([]zap.Field, 0, len(cfg.DefaultFields))
    for k, v := range cfg.DefaultFields {
        fields = append(fields, zap.String(k, v))
    }

    logger = zap.New(core).With(fields...)

    return nil
}

// Logger returns the global logger
func Logger() *zap.Logger {
    if logger == nil {
        logger, _ = zap.NewProduction()
    }
    return logger
}

// WithContext returns a logger with context values
func WithContext(ctx context.Context) *zap.Logger {
    l := Logger()

    if reviewID := ctx.Value("review_id"); reviewID != nil {
        l = l.With(zap.String("review_id", reviewID.(string)))
    }
    if orgID := ctx.Value("org_id"); orgID != nil {
        l = l.With(zap.String("org_id", orgID.(string)))
    }
    if projectID := ctx.Value("project_id"); projectID != nil {
        l = l.With(zap.String("project_id", projectID.(string)))
    }
    if correlationID := ctx.Value("correlation_id"); correlationID != nil {
        l = l.With(zap.String("correlation_id", correlationID.(string)))
    }
    if traceID := ctx.Value("trace_id"); traceID != nil {
        l = l.With(zap.String("trace_id", traceID.(string)))
    }

    return l
}
```

---

# 13. Security

## 13.1 Authentication Flow

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                         AUTHENTICATION FLOW                                   │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  ┌─────────────────┐                                                         │
│  │     Client      │                                                         │
│  │   (Browser or   │                                                         │
│  │    Service)     │                                                         │
│  └────────┬────────┘                                                         │
│           │                                                                   │
│           │ 1. Request with:                                                  │
│           │    - Authorization: Bearer <JWT>                                  │
│           │    - (Optional) Client certificate for mTLS                      │
│           ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │                         TLS Termination                                  │ │
│  │                                                                          │ │
│  │  HTTP Server:                        gRPC Server:                       │ │
│  │  - mTLS optional (configurable)     - mTLS required                     │ │
│  │  - If enforce_http_mtls=true,       - Always verifies client cert       │ │
│  │    verifies client cert                                                 │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│           │                                                                   │
│           │ 2. Certificate verified (if mTLS)                                │
│           ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │                    grpcauth Middleware                                   │ │
│  │                                                                          │ │
│  │  JWT Validation:                                                        │ │
│  │  1. Extract Bearer token from Authorization header                      │ │
│  │  2. Fetch JWKS from auth server (cached)                               │ │
│  │  3. Verify signature, issuer, audience, expiry                         │ │
│  │  4. Extract claims: sub (user_id), org_id, roles                       │ │
│  │  5. Add claims to context                                               │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│           │                                                                   │
│           │ 3. Claims in context                                             │
│           ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │                  Authorization Middleware                                │ │
│  │                                                                          │ │
│  │  1. Extract org_id and project_id from URL path                        │ │
│  │  2. Check user has access to org (from JWT claims)                     │ │
│  │  3. Check user has access to project (from JWT claims)                 │ │
│  │  4. Verify required roles/permissions for the action                   │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│           │                                                                   │
│           │ 4. Authorized                                                    │
│           ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │                       Handler                                            │ │
│  │                                                                          │ │
│  │  - Access user_id, org_id, project_id from context                     │ │
│  │  - All queries scoped to tenant                                        │ │
│  │  - Events include tenant context                                        │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
└──────────────────────────────────────────────────────────────────────────────┘
```

## 13.2 Tenant Isolation

```go
// All database queries are scoped by org_id and project_id

// Example: Get reviews for a project (tenant-scoped)
func (r *Repository) GetReviewsByProject(ctx context.Context, orgID, projectID string) ([]Review, error) {
    rows, err := r.db.QueryContext(ctx, `
        SELECT id, original_query, status, papers_found_count, created_at
        FROM literature_review_requests
        WHERE org_id = $1 AND project_id = $2
        ORDER BY created_at DESC
    `, orgID, projectID)
    // ...
}

// Example: Get review (with tenant verification)
func (r *Repository) GetReviewByID(ctx context.Context, orgID, projectID, reviewID string) (*Review, error) {
    var review Review
    err := r.db.QueryRowContext(ctx, `
        SELECT id, org_id, project_id, user_id, original_query, status, ...
        FROM literature_review_requests
        WHERE id = $1 AND org_id = $2 AND project_id = $3
    `, reviewID, orgID, projectID).Scan(...)
    // ...
}
```

---

# 14. Implementation Phases & Deliverables

## Phase 1: Foundation (Week 1-2)

| ID | Deliverable | Description | Acceptance Criteria |
|----|-------------|-------------|---------------------|
| D1.1 | Project Scaffolding | Initialize Go module, directory structure, Makefile, CI setup | `make build` succeeds, CI pipeline runs |
| D1.2 | Configuration System | Config struct, YAML loading, env overrides, validation | All config fields parsed, validation errors clear |
| D1.3 | Database Schema | Full PostgreSQL schema with all tables, indexes, functions | Migrations apply cleanly, rollback works |
| D1.4 | Proto Generation | Literature review + ingestion protos with buf | Generated Go code compiles, lint passes |
| D1.5 | Domain Models | Go structs matching schema with JSON/DB tags | Unit tests pass, serialization works |

**Phase 1 Milestones:**
- [ ] Repository created with CI/CD
- [ ] Config loads from file + env vars
- [ ] Database migrations run successfully
- [ ] Proto-generated code imports cleanly

---

## Phase 2: Infrastructure (Week 2-3)

| ID | Deliverable | Description | Acceptance Criteria |
|----|-------------|-------------|---------------------|
| D2.1 | Repository Layer | PostgreSQL repository with all CRUD operations | Integration tests pass with test DB |
| D2.2 | Paper Identity Resolution | `upsert_paper` function with concurrent safety | Concurrent insert tests pass, no duplicates |
| D2.3 | Rate Limiter Pool | Per-source token bucket + concurrency limits | Load tests respect configured limits |
| D2.4 | Temporal Client | Client setup, worker registration, connection | Worker connects, can execute simple workflow |
| D2.5 | mTLS Configuration | Cert loading for gRPC (required) and HTTP (optional) | Handshake succeeds with valid certs |
| D2.6 | Outbox Integration | Publisher, transactional helper, worker setup | Events appear in outbox table |
| D2.7 | Observability Setup | Tracing, metrics, structured logging | Metrics on /metrics, traces in collector |

**Phase 2 Milestones:**
- [ ] Repository CRUD operations work
- [ ] Rate limiters enforce configured limits
- [ ] Temporal worker starts and connects
- [ ] mTLS handshake works
- [ ] Outbox events inserted in transactions

---

## Phase 3: External Integrations (Week 3-5)

| ID | Deliverable | Description | Acceptance Criteria |
|----|-------------|-------------|---------------------|
| D3.1 | LLM Client Interface | Abstract client with OpenAI implementation | Keyword extraction returns valid results |
| D3.2 | Semantic Scholar Client | Full client with pagination, error handling | Search returns normalized papers |
| D3.3 | OpenAlex Client | Client with polite pool email | Search returns normalized papers |
| D3.4 | Scopus Client | Client with API key auth | Search returns normalized papers |
| D3.5 | PubMed Client | E-utilities client with proper parsing | Search returns normalized papers |
| D3.6 | bioRxiv Client | Client for preprints | Search returns normalized papers |
| D3.7 | arXiv Client | Atom API client with XML parsing | Search returns normalized papers |
| D3.8 | Ingestion gRPC Client | mTLS client with batch support | Batch ingest succeeds |
| D3.9 | Local Backoff Logic | 429/503 handling before Temporal retry | Rate limits handled gracefully |
| D3.10 | Pagination Tokens | Signed, opaque cursor tokens | Tokens validate correctly, expire |

**Phase 3 Milestones:**
- [ ] All 6 paper sources return normalized data
- [ ] LLM extracts meaningful keywords
- [ ] Ingestion service accepts batches
- [ ] 429 responses handled with backoff

---

## Phase 4: Temporal Workflows (Week 5-7)

| ID | Deliverable | Description | Acceptance Criteria |
|----|-------------|-------------|---------------------|
| D4.1 | Determinism Helpers | Sorted map iteration, safe ID generation | Replay tests pass |
| D4.2 | Main Workflow | Full LiteratureReviewWorkflow with all phases | End-to-end test completes |
| D4.3 | Paper Search Workflow | Parallel search child workflow | Searches execute concurrently |
| D4.4 | Keyword Expansion Workflow | LLM extraction child workflow | Keywords extracted from papers |
| D4.5 | Query Handler | Progress state queryable | Query returns current state |
| D4.6 | ContinueAsNew | History management for large sets | History stays bounded |
| D4.7 | LLM Activities | Keyword extraction with dedup check | No duplicate extractions |
| D4.8 | Search Activities | Paper search with rate limiting | Respects rate limits |
| D4.9 | DB Activities | Save/lookup operations | Idempotent operations |
| D4.10 | Ingestion Activities | Batch send to ingestion service | Papers queued successfully |
| D4.11 | Event Activities | Outbox event publishing | Events published atomically |

**Phase 4 Milestones:**
- [ ] Workflow replays deterministically
- [ ] Full review completes end-to-end
- [ ] Progress queryable during execution
- [ ] Events published at each phase

---

## Phase 5: API Layer (Week 7-8)

| ID | Deliverable | Description | Acceptance Criteria |
|----|-------------|-------------|---------------------|
| D5.1 | gRPC Server | Server with grpcauth, all handlers | All RPC methods work |
| D5.2 | HTTP Server | REST API with proper mTLS config | All endpoints work |
| D5.3 | Tenant Middleware | org_id/project_id extraction and validation | Unauthorized access blocked |
| D5.4 | Progress Streaming | SSE endpoint with Temporal + NOTIFY | Real-time updates received |
| D5.5 | Error Handling | Consistent error responses | Errors map to correct codes |
| D5.6 | Request Validation | Input validation for all endpoints | Invalid requests rejected |

**Phase 5 Milestones:**
- [ ] All API endpoints functional
- [ ] Authentication and authorization work
- [ ] Progress streaming delivers updates
- [ ] Error responses are consistent

---

## Phase 6: Testing & Hardening (Week 8-9)

| ID | Deliverable | Description | Acceptance Criteria |
|----|-------------|-------------|---------------------|
| D6.1 | Unit Tests | >80% coverage on business logic | `make test` passes |
| D6.2 | Temporal Replay Tests | Determinism verification | Replay tests pass |
| D6.3 | Integration Tests | DB, Temporal, mocked APIs | Tests pass with docker-compose |
| D6.4 | Concurrent Tests | Race condition detection | No races detected |
| D6.5 | E2E Tests | Full workflow execution | Complete review succeeds |
| D6.6 | Load Tests | Concurrent review requests | 50 concurrent reviews |
| D6.7 | Chaos Tests | Worker failures, network issues | Workflows recover |
| D6.8 | Security Tests | Auth bypass attempts, injection | All attacks blocked |

**Phase 6 Milestones:**
- [ ] Test coverage >80%
- [ ] No race conditions
- [ ] Load tests pass
- [ ] Chaos scenarios recover

---

## Phase 7: Deployment (Week 9-10)

| ID | Deliverable | Description | Acceptance Criteria |
|----|-------------|-------------|---------------------|
| D7.1 | Dockerfile | Multi-stage build for server + worker | Images <100MB |
| D7.2 | Docker Compose | Local development setup | `docker-compose up` works |
| D7.3 | Kubernetes Base | Deployment, Service, ConfigMap | Deploys to cluster |
| D7.4 | Kubernetes Overlays | Dev, staging, prod configs | Each env deploys correctly |
| D7.5 | Secrets Management | External secrets or sealed secrets | Secrets injected securely |
| D7.6 | HPA Configuration | Auto-scaling rules | Scales under load |
| D7.7 | Kafka Topics | Topic provisioning with retention | Topics exist with config |
| D7.8 | CI/CD Pipeline | Build, test, deploy automation | PR merge triggers deploy |
| D7.9 | Runbook | Operations documentation | On-call can debug |
| D7.10 | Dashboards | Grafana dashboards for metrics | Key metrics visible |

**Phase 7 Milestones:**
- [ ] Images built and pushed
- [ ] Deploys to staging
- [ ] Deploys to production
- [ ] Runbook complete
- [ ] Dashboards operational
