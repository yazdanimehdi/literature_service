# Literature Review Service -- Database Schema

Complete PostgreSQL schema reference for the Literature Review Service. All objects are
created by 8 sequential migrations managed through `golang-migrate`.

---

## Table of Contents

- [Migration History](#migration-history)
- [Extensions](#extensions)
- [Enum Types](#enum-types)
- [Tables](#tables)
  - [keywords](#keywords)
  - [papers](#papers)
  - [paper_identifiers](#paper_identifiers)
  - [paper_sources](#paper_sources)
  - [keyword_searches](#keyword_searches)
  - [keyword_paper_mappings](#keyword_paper_mappings)
  - [literature_review_requests](#literature_review_requests)
  - [request_keyword_mappings](#request_keyword_mappings)
  - [request_paper_mappings](#request_paper_mappings)
  - [review_progress_events](#review_progress_events)
  - [outbox_events](#outbox_events)
  - [outbox_dead_letter](#outbox_dead_letter)
- [Functions](#functions)
  - [generate_canonical_id](#generate_canonical_id)
  - [generate_search_window_hash](#generate_search_window_hash)
  - [get_or_create_keyword](#get_or_create_keyword)
  - [find_paper_by_identifier](#find_paper_by_identifier)
  - [find_paper_by_any_identifier](#find_paper_by_any_identifier)
  - [normalize_keyword](#normalize_keyword)
- [Trigger Functions](#trigger-functions)
- [Triggers](#triggers)
- [Performance Indexes](#performance-indexes)
- [Entity Relationship Diagram](#entity-relationship-diagram)

---

## Migration History

| # | File | Description |
|---|------|-------------|
| 1 | `000001_init_extensions` | Installs `uuid-ossp`, `pg_trgm`, and `pgcrypto` extensions |
| 2 | `000002_enums` | Creates all custom enum types |
| 3 | `000003_core_tables` | Keywords, papers, identifiers, sources, searches, keyword-paper mappings |
| 4 | `000004_tenant_tables` | Review requests, request-keyword/paper mappings, progress events |
| 5 | `000005_outbox` | Outbox events and dead letter tables for transactional event publishing |
| 6 | `000006_functions` | Helper functions (canonical ID, search hashing, keyword lookup, paper lookup) |
| 7 | `000007_triggers` | Automatic triggers (updated_at timestamps, progress NOTIFY) |
| 8 | `000008_performance_indexes` | Additional composite indexes for query optimization |

Migrations are located in `migrations/` and executed via `make migrate-up` / `make migrate-down`.

---

## Extensions

| Extension | Purpose |
|-----------|---------|
| `uuid-ossp` | UUID generation via `uuid_generate_v4()` |
| `pg_trgm` | Trigram matching for fuzzy text search on titles, abstracts, and keywords |
| `pgcrypto` | Cryptographic functions: `gen_random_uuid()`, `digest()` for SHA256 hash generation |

---

## Enum Types

### `identifier_type`

Types of academic paper identifiers from various sources.

| Value | Description |
|-------|-------------|
| `doi` | Digital Object Identifier |
| `arxiv_id` | arXiv identifier |
| `pubmed_id` | PubMed identifier |
| `semantic_scholar_id` | Semantic Scholar Paper ID |
| `openalex_id` | OpenAlex Work ID |
| `scopus_id` | Scopus EID |

### `review_status`

Lifecycle states for literature review requests.

| Value | Description |
|-------|-------------|
| `pending` | Request created, not yet started |
| `extracting_keywords` | LLM extracting keywords from query |
| `searching` | Searching paper sources |
| `expanding` | Recursive keyword expansion in progress |
| `ingesting` | Papers being sent to ingestion service |
| `completed` | Review completed successfully |
| `partial` | Review completed with some failures |
| `failed` | Review failed |
| `cancelled` | Review cancelled by user |

### `search_status`

States for keyword search operations.

| Value | Description |
|-------|-------------|
| `pending` | Search queued |
| `in_progress` | Search currently executing |
| `completed` | Search completed successfully |
| `failed` | Search failed |
| `rate_limited` | Search hit rate limit, will retry |

### `ingestion_status`

Ingestion states for papers within a review.

| Value | Description |
|-------|-------------|
| `pending` | Not yet submitted for ingestion |
| `submitted` | Submitted to ingestion service |
| `processing` | Ingestion in progress |
| `completed` | Ingestion completed successfully |
| `failed` | Ingestion failed |
| `skipped` | Skipped (duplicate, excluded, etc.) |

### `mapping_type`

How a keyword was associated with a paper.

| Value | Description |
|-------|-------------|
| `author_keyword` | Keyword from author-provided keywords |
| `mesh_term` | MeSH term from PubMed |
| `extracted` | Extracted from title/abstract by LLM |
| `query_match` | Direct match from user query |

### `source_type`

Paper source APIs.

| Value | Description |
|-------|-------------|
| `semantic_scholar` | Semantic Scholar API |
| `openalex` | OpenAlex API |
| `scopus` | Scopus API |
| `pubmed` | PubMed/Entrez API |
| `biorxiv` | bioRxiv API |
| `arxiv` | arXiv API |

---

## Tables

### `keywords`

Global keyword store for paper searches. Keywords are deduplicated via a normalized
(lowercased, trimmed) form.

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `UUID` | NO | `uuid_generate_v4()` | **PK** | Unique identifier |
| `keyword` | `TEXT` | NO | -- | | Original keyword text |
| `normalized_keyword` | `TEXT` | NO | -- | **UNIQUE** (`uq_keywords_normalized`) | Lowercased, trimmed for deduplication |
| `created_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Creation timestamp |

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_keywords_keyword` | `keyword` | B-tree | -- |
| `idx_keywords_normalized` | `normalized_keyword` | B-tree | -- |
| `idx_keywords_keyword_trgm` | `keyword` | GIN (`gin_trgm_ops`) | -- |

---

### `papers`

Central repository of discovered academic papers. Each paper is uniquely identified by a
`canonical_id` derived from the highest-priority available identifier
(DOI > arXiv > PubMed > S2 > OpenAlex > Scopus).

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `UUID` | NO | `uuid_generate_v4()` | **PK** | Unique identifier |
| `canonical_id` | `TEXT` | NO | -- | **UNIQUE** (`uq_papers_canonical_id`) | Derived identifier (priority: DOI > arXiv > PubMed > etc.) |
| `title` | `TEXT` | NO | -- | | Paper title |
| `abstract` | `TEXT` | YES | -- | | Paper abstract |
| `authors` | `JSONB` | YES | -- | | JSON array of author objects with name, affiliation, etc. |
| `publication_date` | `DATE` | YES | -- | | Full publication date |
| `publication_year` | `INTEGER` | YES | -- | | Publication year |
| `venue` | `TEXT` | YES | -- | | Conference or workshop venue |
| `journal` | `TEXT` | YES | -- | | Journal name |
| `volume` | `TEXT` | YES | -- | | Journal volume |
| `issue` | `TEXT` | YES | -- | | Journal issue |
| `pages` | `TEXT` | YES | -- | | Page range |
| `citation_count` | `INTEGER` | YES | `0` | | Citation count |
| `reference_count` | `INTEGER` | YES | `0` | | Reference count |
| `pdf_url` | `TEXT` | YES | -- | | PDF download URL |
| `open_access` | `BOOLEAN` | YES | `FALSE` | | Whether the paper is open access |
| `keywords_extracted` | `BOOLEAN` | YES | `FALSE` | | Whether keywords have been extracted from this paper |
| `raw_metadata` | `JSONB` | YES | -- | | Original metadata from source APIs for future re-processing |
| `created_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Last update timestamp (auto-updated by trigger) |

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_papers_canonical_id` | `canonical_id` | B-tree | -- |
| `idx_papers_publication_date` | `publication_date` | B-tree | -- |
| `idx_papers_publication_year` | `publication_year` | B-tree | -- |
| `idx_papers_citation_count` | `citation_count DESC` | B-tree | -- |
| `idx_papers_created_at` | `created_at` | B-tree | -- |
| `idx_papers_keywords_extracted` | `keywords_extracted` | B-tree (partial) | `WHERE NOT keywords_extracted` |
| `idx_papers_title_trgm` | `title` | GIN (`gin_trgm_ops`) | -- |
| `idx_papers_abstract_trgm` | `abstract` | GIN (`gin_trgm_ops`, partial) | `WHERE abstract IS NOT NULL` |

**Triggers:**

| Trigger | Event | Function |
|---------|-------|----------|
| `trg_papers_updated_at` | `BEFORE UPDATE` | `update_papers_timestamp()` |

---

### `paper_identifiers`

Maps papers to their various identifiers across sources. A paper may have multiple
identifiers (DOI, arXiv ID, PubMed ID, etc.) but each specific identifier value is
globally unique.

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `UUID` | NO | `uuid_generate_v4()` | **PK** | Unique identifier |
| `paper_id` | `UUID` | NO | -- | **FK** `papers(id) ON DELETE CASCADE` | Paper reference |
| `identifier_type` | `identifier_type` | NO | -- | **UNIQUE** with `identifier_value` (`uq_paper_identifiers`) | Type of identifier |
| `identifier_value` | `TEXT` | NO | -- | **UNIQUE** with `identifier_type` (`uq_paper_identifiers`) | The identifier value |
| `source_api` | `source_type` | YES | -- | | Which API provided this identifier |
| `discovered_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Discovery timestamp |

**Unique Constraint:** `(identifier_type, identifier_value)`

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_paper_identifiers_paper_id` | `paper_id` | B-tree | -- |
| `idx_paper_identifiers_type_value` | `identifier_type, identifier_value` | B-tree | -- |
| `idx_paper_identifiers_value` | `identifier_value` | B-tree | -- |

---

### `paper_sources`

Tracks which APIs have provided data for each paper. Stores source-specific metadata
such as rankings and scores. One record per paper per source.

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `UUID` | NO | `uuid_generate_v4()` | **PK** | Unique identifier |
| `paper_id` | `UUID` | NO | -- | **FK** `papers(id) ON DELETE CASCADE`, **UNIQUE** with `source_api` | Paper reference |
| `source_api` | `source_type` | NO | -- | **UNIQUE** with `paper_id` (`uq_paper_sources`) | Source API |
| `source_metadata` | `JSONB` | YES | -- | | Source-specific metadata (rankings, scores, etc.) |
| `created_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Last update timestamp (auto-updated by trigger) |

**Unique Constraint:** `(paper_id, source_api)`

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_paper_sources_paper_id` | `paper_id` | B-tree | -- |
| `idx_paper_sources_source_api` | `source_api` | B-tree | -- |

**Triggers:**

| Trigger | Event | Function |
|---------|-------|----------|
| `trg_paper_sources_updated_at` | `BEFORE UPDATE` | `update_paper_sources_timestamp()` |

---

### `keyword_searches`

Records search operations for idempotency and audit. Each search is uniquely identified
by a SHA256 hash of `keyword_id + source_api + date_range`, preventing duplicate
searches for the same keyword, source, and time window.

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `UUID` | NO | `uuid_generate_v4()` | **PK** | Unique identifier |
| `keyword_id` | `UUID` | NO | -- | **FK** `keywords(id) ON DELETE CASCADE` | Keyword reference |
| `source_api` | `source_type` | NO | -- | | Source API used for the search |
| `searched_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Search execution timestamp |
| `date_from` | `DATE` | YES | -- | | Start of date range filter (NULL for no filter) |
| `date_to` | `DATE` | YES | -- | | End of date range filter (NULL for no filter) |
| `search_window_hash` | `TEXT` | NO | -- | **UNIQUE** (`uq_keyword_searches`) | SHA256 hash of keyword_id+source_api+date_range for deduplication |
| `papers_found` | `INTEGER` | YES | `0` | | Number of papers found |
| `status` | `search_status` | NO | `'pending'` | | Search execution status |
| `error_message` | `TEXT` | YES | -- | | Error details on failure |

**Unique Constraint:** `(search_window_hash)`

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_keyword_searches_keyword_id` | `keyword_id` | B-tree | -- |
| `idx_keyword_searches_source_api` | `source_api` | B-tree | -- |
| `idx_keyword_searches_status` | `status` | B-tree | -- |
| `idx_keyword_searches_window_hash` | `search_window_hash` | B-tree | -- |
| `idx_keyword_searches_searched_at` | `searched_at` | B-tree | -- |
| `idx_keyword_searches_keyword_source_searched` | `keyword_id, source_api, searched_at DESC` | B-tree (composite) | -- |

The composite index `idx_keyword_searches_keyword_source_searched` (added in migration 8)
covers the `GetLastSearch` query pattern: `WHERE keyword_id = $1 AND source_api = $2 ORDER BY searched_at DESC LIMIT 1`.

---

### `keyword_paper_mappings`

Links keywords to papers with provenance information. Records how each keyword was
associated with a paper (author-provided, MeSH term, LLM-extracted, or direct query
match) along with an optional confidence score.

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `UUID` | NO | `uuid_generate_v4()` | **PK** | Unique identifier |
| `keyword_id` | `UUID` | NO | -- | **FK** `keywords(id) ON DELETE CASCADE` | Keyword reference |
| `paper_id` | `UUID` | NO | -- | **FK** `papers(id) ON DELETE CASCADE` | Paper reference |
| `mapping_type` | `mapping_type` | NO | -- | **UNIQUE** with `keyword_id, paper_id` | How the keyword was associated |
| `source_type` | `source_type` | NO | -- | | Source API that provided this mapping |
| `confidence_score` | `DECIMAL(5,4)` | YES | -- | | LLM confidence for extracted keywords (0.0000-1.0000) |
| `created_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Creation timestamp |

**Unique Constraint:** `(keyword_id, paper_id, mapping_type)`

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_keyword_paper_mappings_keyword_id` | `keyword_id` | B-tree | -- |
| `idx_keyword_paper_mappings_paper_id` | `paper_id` | B-tree | -- |
| `idx_keyword_paper_mappings_type` | `mapping_type` | B-tree | -- |
| `idx_keyword_paper_mappings_confidence` | `confidence_score DESC` | B-tree (partial) | `WHERE confidence_score IS NOT NULL` |
| `idx_keyword_paper_mappings_keyword_created` | `keyword_id, created_at DESC` | B-tree (composite) | -- |

The composite index `idx_keyword_paper_mappings_keyword_created` (added in migration 8)
covers the `GetPapersForKeyword` query pattern.

---

### `literature_review_requests`

Multi-tenant table storing user literature review requests. Each request is scoped to an
organization and project, and is tracked through a Temporal workflow.

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `UUID` | NO | `uuid_generate_v4()` | **PK** | Unique identifier |
| `org_id` | `TEXT` | NO | -- | | Organization ID for multi-tenancy |
| `project_id` | `TEXT` | NO | -- | | Project ID within organization |
| `user_id` | `TEXT` | NO | -- | | User who initiated the review |
| `original_query` | `TEXT` | NO | -- | | Natural language query submitted by user |
| `temporal_workflow_id` | `TEXT` | YES | -- | | Temporal workflow ID |
| `temporal_run_id` | `TEXT` | YES | -- | | Temporal run ID |
| `status` | `review_status` | NO | `'pending'` | | Current lifecycle state |
| `keywords_found_count` | `INTEGER` | YES | `0` | | Number of keywords discovered |
| `papers_found_count` | `INTEGER` | YES | `0` | | Total papers discovered |
| `papers_ingested_count` | `INTEGER` | YES | `0` | | Papers successfully ingested |
| `papers_failed_count` | `INTEGER` | YES | `0` | | Papers that failed ingestion |
| `expansion_depth` | `INTEGER` | NO | `1` | **CHECK** `(0 <= expansion_depth <= 5)` | Levels of recursive keyword expansion |
| `config_snapshot` | `JSONB` | YES | -- | | Snapshot of config at request time for reproducibility |
| `source_filters` | `JSONB` | YES | -- | | Which paper sources to include/exclude |
| `date_from` | `DATE` | YES | -- | | Start of publication date filter |
| `date_to` | `DATE` | YES | -- | | End of publication date filter |
| `created_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Request creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Last update timestamp (auto-updated by trigger) |
| `started_at` | `TIMESTAMPTZ` | YES | -- | | When processing started |
| `completed_at` | `TIMESTAMPTZ` | YES | -- | | When processing completed |

**Check Constraint:** `valid_expansion_depth` -- `expansion_depth >= 0 AND expansion_depth <= 5`

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_review_requests_org_id` | `org_id` | B-tree | -- |
| `idx_review_requests_project_id` | `project_id` | B-tree | -- |
| `idx_review_requests_user_id` | `user_id` | B-tree | -- |
| `idx_review_requests_org_project` | `org_id, project_id` | B-tree (composite) | -- |
| `idx_review_requests_status` | `status` | B-tree | -- |
| `idx_review_requests_created_at` | `created_at DESC` | B-tree | -- |
| `idx_review_requests_workflow_id` | `temporal_workflow_id` | B-tree (partial) | `WHERE temporal_workflow_id IS NOT NULL` |
| `idx_review_requests_org_project_created` | `org_id, project_id, created_at DESC` | B-tree (composite) | -- |
| `idx_review_requests_id_org_project` | `id, org_id, project_id` | B-tree (composite) | -- |

The last two composite indexes (added in migration 8) cover the tenant-scoped `List` and
`Get` query patterns used by the gRPC API.

**Triggers:**

| Trigger | Event | Function |
|---------|-------|----------|
| `trg_review_requests_updated_at` | `BEFORE UPDATE` | `update_review_request_timestamp()` |

---

### `request_keyword_mappings`

Links review requests to discovered keywords with provenance. Tracks which extraction
round produced each keyword and, for expanded keywords, which source paper they came from.

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `UUID` | NO | `uuid_generate_v4()` | **PK** | Unique identifier |
| `request_id` | `UUID` | NO | -- | **FK** `literature_review_requests(id) ON DELETE CASCADE` | Review request reference |
| `keyword_id` | `UUID` | NO | -- | **FK** `keywords(id) ON DELETE CASCADE` | Keyword reference |
| `extraction_round` | `INTEGER` | NO | `0` | | Expansion round (0=initial query, 1+=expanded) |
| `source_paper_id` | `UUID` | YES | -- | **FK** `papers(id) ON DELETE SET NULL` | Paper that yielded this keyword (NULL for initial query) |
| `source_type` | `TEXT` | NO | `'query'` | | How keyword was obtained: `query`, `paper_keywords`, `llm_extraction` |
| `created_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Creation timestamp |

**Unique Constraint:** `(request_id, keyword_id, extraction_round, source_paper_id)`

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_request_keyword_mappings_request_id` | `request_id` | B-tree | -- |
| `idx_request_keyword_mappings_keyword_id` | `keyword_id` | B-tree | -- |
| `idx_request_keyword_mappings_round` | `extraction_round` | B-tree | -- |
| `idx_request_keyword_mappings_source_paper` | `source_paper_id` | B-tree (partial) | `WHERE source_paper_id IS NOT NULL` |

---

### `request_paper_mappings`

Links review requests to discovered papers with discovery context. Tracks which keyword
and source led to each paper, the expansion depth at which it was found, and its
ingestion status through the pipeline.

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `UUID` | NO | `uuid_generate_v4()` | **PK** | Unique identifier |
| `request_id` | `UUID` | NO | -- | **FK** `literature_review_requests(id) ON DELETE CASCADE` | Review request reference |
| `paper_id` | `UUID` | NO | -- | **FK** `papers(id) ON DELETE CASCADE` | Paper reference |
| `discovered_via_keyword_id` | `UUID` | YES | -- | **FK** `keywords(id) ON DELETE SET NULL` | Keyword search that found this paper |
| `discovered_via_source` | `source_type` | YES | -- | | Source API where paper was discovered |
| `expansion_depth` | `INTEGER` | NO | `0` | | Expansion round that discovered this paper |
| `ingestion_status` | `ingestion_status` | NO | `'pending'` | | Current ingestion state |
| `ingestion_job_id` | `TEXT` | YES | -- | | Ingestion service job ID for tracking |
| `ingestion_error` | `TEXT` | YES | -- | | Error details on ingestion failure |
| `created_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Last update timestamp (auto-updated by trigger) |

**Unique Constraint:** `(request_id, paper_id)`

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_request_paper_mappings_request_id` | `request_id` | B-tree | -- |
| `idx_request_paper_mappings_paper_id` | `paper_id` | B-tree | -- |
| `idx_request_paper_mappings_keyword_id` | `discovered_via_keyword_id` | B-tree (partial) | `WHERE discovered_via_keyword_id IS NOT NULL` |
| `idx_request_paper_mappings_ingestion_status` | `ingestion_status` | B-tree | -- |
| `idx_request_paper_mappings_depth` | `expansion_depth` | B-tree | -- |
| `idx_request_paper_mappings_pending` | `request_id, ingestion_status` | B-tree (partial) | `WHERE ingestion_status = 'pending'` |

**Triggers:**

| Trigger | Event | Function |
|---------|-------|----------|
| `trg_request_paper_mappings_updated_at` | `BEFORE UPDATE` | `update_request_paper_mapping_timestamp()` |

---

### `review_progress_events`

Append-only log of real-time progress events for UI updates. On each INSERT, a
PostgreSQL `NOTIFY` is fired on the `review_progress` channel, allowing connected
clients to receive server-sent events (SSE) in real time.

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `UUID` | NO | `uuid_generate_v4()` | **PK** | Unique identifier |
| `request_id` | `UUID` | NO | -- | **FK** `literature_review_requests(id) ON DELETE CASCADE` | Review request reference |
| `event_type` | `TEXT` | NO | -- | | Event type (e.g., `keyword_extracted`, `paper_found`, `search_completed`) |
| `event_data` | `JSONB` | NO | `'{}'` | | Event-specific data payload |
| `created_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Event timestamp |

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_review_progress_events_request_id` | `request_id` | B-tree | -- |
| `idx_review_progress_events_type` | `event_type` | B-tree | -- |
| `idx_review_progress_events_created_at` | `created_at` | B-tree | -- |
| `idx_review_progress_events_request_created` | `request_id, created_at` | B-tree (composite) | -- |

**Triggers:**

| Trigger | Event | Function |
|---------|-------|----------|
| `trg_review_progress_notify` | `AFTER INSERT` | `notify_review_progress()` |

---

### `outbox_events`

Transactional outbox for reliable event publishing to Kafka. Supports lease-based
distributed processing where workers acquire short-lived locks on pending events,
publish them, and mark them as completed. Includes retry logic with exponential backoff.

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `BIGSERIAL` | NO | auto-increment | **PK** | Unique identifier |
| `event_id` | `TEXT` | NO | `gen_random_uuid()::text` | | Stable event ID for consumer deduplication |
| `event_version` | `INTEGER` | NO | `1` | | Event schema version |
| `aggregate_id` | `TEXT` | NO | -- | | Aggregate ID (e.g., review request UUID) |
| `aggregate_type` | `TEXT` | YES | -- | | Aggregate type (e.g., `literature_review`) |
| `event_type` | `TEXT` | NO | -- | | Event type (e.g., `review.completed`) |
| `payload` | `BYTEA` | NO | -- | | Binary event payload (protobuf or JSON) |
| `scope` | `TEXT` | NO | `'public'` | **CHECK** `IN ('public', 'org', 'project')` | Visibility scope |
| `org_id` | `TEXT` | YES | -- | | Organization ID for multi-tenancy |
| `project_id` | `TEXT` | YES | -- | | Project ID within organization |
| `metadata` | `JSONB` | YES | -- | | Tracing context, correlation IDs |
| `status` | `TEXT` | NO | `'pending'` | **CHECK** `IN ('pending', 'in_progress', 'published', 'failed')` | Processing state |
| `sequence_num` | `BIGSERIAL` | NO | auto-increment | | Monotonically increasing sequence for ordering |
| `publish_attempts` | `INTEGER` | NO | `0` | | Number of publish attempts |
| `max_attempts` | `INTEGER` | NO | `5` | | Maximum retry attempts before dead-lettering |
| `last_error` | `TEXT` | YES | -- | | Last publish error message |
| `next_attempt_at` | `TIMESTAMPTZ` | YES | -- | | Scheduled time for retry with exponential backoff |
| `locked_by` | `TEXT` | YES | -- | | Worker ID that holds the lease |
| `locked_at` | `TIMESTAMPTZ` | YES | -- | | When the lease was acquired |
| `lock_expires_at` | `TIMESTAMPTZ` | YES | -- | | Lease expiration time for distributed processing |
| `created_at` | `TIMESTAMPTZ` | NO | `NOW()` | | Event creation timestamp |
| `processed_at` | `TIMESTAMPTZ` | YES | -- | | When the event was successfully published |

**Check Constraints:**
- `valid_outbox_status` -- `status IN ('pending', 'in_progress', 'published', 'failed')`
- `valid_outbox_scope` -- `scope IN ('public', 'org', 'project')`

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_outbox_events_status_next_attempt` | `status, next_attempt_at` | B-tree (partial) | `WHERE status = 'pending'` |
| `idx_outbox_events_status_created` | `status, created_at` | B-tree (partial) | `WHERE status = 'pending'` |
| `idx_outbox_events_aggregate` | `aggregate_id, aggregate_type` | B-tree | -- |
| `idx_outbox_events_event_id` | `event_id` | B-tree | -- |
| `idx_outbox_events_lock_expires` | `lock_expires_at` | B-tree (partial) | `WHERE status = 'in_progress' AND lock_expires_at IS NOT NULL` |
| `idx_outbox_events_cleanup` | `processed_at` | B-tree (partial) | `WHERE status = 'published'` |
| `idx_outbox_events_scope` | `scope, org_id, project_id` | B-tree | -- |

---

### `outbox_dead_letter`

Dead letter queue for events that permanently failed after exhausting all retry attempts.
Supports manual inspection and optional reprocessing.

| Column | Type | Nullable | Default | Constraints | Description |
|--------|------|----------|---------|-------------|-------------|
| `id` | `BIGSERIAL` | NO | auto-increment | **PK** | Unique identifier |
| `original_event_id` | `TEXT` | NO | -- | | Event ID from the original `outbox_events` entry |
| `event_version` | `INTEGER` | NO | `1` | | Event schema version |
| `aggregate_type` | `TEXT` | YES | -- | | Aggregate type |
| `aggregate_id` | `TEXT` | NO | -- | | Aggregate ID |
| `event_type` | `TEXT` | NO | -- | | Event type |
| `org_id` | `TEXT` | YES | -- | | Organization ID |
| `project_id` | `TEXT` | YES | -- | | Project ID |
| `payload` | `BYTEA` | NO | -- | | Original binary payload |
| `metadata` | `JSONB` | YES | -- | | Original metadata |
| `original_created_at` | `TIMESTAMPTZ` | NO | -- | | When the original event was created |
| `failed_at` | `TIMESTAMPTZ` | NO | `NOW()` | | When the event was dead-lettered |
| `failure_reason` | `TEXT` | YES | -- | | Final error message that caused dead-lettering |
| `retry_count` | `INTEGER` | NO | `0` | | Number of retries before failure |
| `reprocessed_at` | `TIMESTAMPTZ` | YES | -- | | When the event was reprocessed (if ever) |
| `reprocess_count` | `INTEGER` | NO | `0` | | Number of reprocessing attempts |

**Indexes:**

| Index | Column(s) | Type | Condition |
|-------|-----------|------|-----------|
| `idx_outbox_dead_letter_event_id` | `original_event_id` | B-tree | -- |
| `idx_outbox_dead_letter_event_type` | `event_type` | B-tree | -- |
| `idx_outbox_dead_letter_aggregate` | `aggregate_id, aggregate_type` | B-tree | -- |
| `idx_outbox_dead_letter_failed_at` | `failed_at DESC` | B-tree | -- |
| `idx_outbox_dead_letter_pending_reprocess` | `failed_at` | B-tree (partial) | `WHERE reprocessed_at IS NULL` |

---

## Functions

### `generate_canonical_id`

Generates a canonical identifier for a paper based on identifier priority.

```sql
generate_canonical_id(
    p_doi TEXT DEFAULT NULL,
    p_arxiv_id TEXT DEFAULT NULL,
    p_pubmed_id TEXT DEFAULT NULL,
    p_semantic_scholar_id TEXT DEFAULT NULL,
    p_openalex_id TEXT DEFAULT NULL,
    p_scopus_id TEXT DEFAULT NULL
) RETURNS TEXT
```

**Volatility:** `IMMUTABLE`

**Priority order:** DOI > arXiv > PubMed > Semantic Scholar > OpenAlex > Scopus

**Prefix format:**

| Identifier | Prefix | Normalization |
|------------|--------|---------------|
| DOI | `doi:` | Lowercased, trimmed |
| arXiv | `arxiv:` | Lowercased, trimmed |
| PubMed | `pubmed:` | Trimmed |
| Semantic Scholar | `s2:` | Trimmed |
| OpenAlex | `openalex:` | Uppercased, trimmed |
| Scopus | `scopus:` | Trimmed |
| None provided | `unknown:` | Random UUID fallback |

**Example:**
```sql
SELECT generate_canonical_id(p_doi := '10.1234/example');
-- Returns: 'doi:10.1234/example'
```

---

### `generate_search_window_hash`

Creates a SHA256 hash for idempotent search deduplication.

```sql
generate_search_window_hash(
    p_keyword_id UUID,
    p_source_api source_type,
    p_date_from DATE DEFAULT NULL,
    p_date_to DATE DEFAULT NULL
) RETURNS TEXT
```

**Volatility:** `IMMUTABLE`

Constructs a deterministic string from `keyword_id|source_api|date_from|date_to` (using
`'NULL'` for absent date values) and returns its SHA256 hex digest. Used as the
`search_window_hash` column in `keyword_searches` for deduplication.

---

### `get_or_create_keyword`

Idempotently creates or retrieves a keyword by its normalized value.

```sql
get_or_create_keyword(p_keyword TEXT) RETURNS UUID
```

**Volatility:** default (volatile)

Normalizes the input via `LOWER(TRIM(...))`, attempts a lookup, and inserts with
`ON CONFLICT DO UPDATE` (no-op) if not found. Always returns the keyword's UUID.

---

### `find_paper_by_identifier`

Looks up a paper by a specific identifier type and value.

```sql
find_paper_by_identifier(
    p_identifier_type identifier_type,
    p_identifier_value TEXT
) RETURNS UUID
```

**Volatility:** `STABLE`

Returns the `paper_id` from `paper_identifiers` or `NULL` if not found.

---

### `find_paper_by_any_identifier`

Looks up a paper by checking all provided identifiers in priority order.

```sql
find_paper_by_any_identifier(
    p_doi TEXT DEFAULT NULL,
    p_arxiv_id TEXT DEFAULT NULL,
    p_pubmed_id TEXT DEFAULT NULL,
    p_semantic_scholar_id TEXT DEFAULT NULL,
    p_openalex_id TEXT DEFAULT NULL,
    p_scopus_id TEXT DEFAULT NULL
) RETURNS UUID
```

**Volatility:** `STABLE`

Iterates through identifiers in the same priority order as `generate_canonical_id`
(DOI > arXiv > PubMed > Semantic Scholar > OpenAlex > Scopus). Returns the first
matching `paper_id` or `NULL`.

---

### `normalize_keyword`

Normalizes keyword text to a lowercase, trimmed form.

```sql
normalize_keyword(p_keyword TEXT) RETURNS TEXT
```

**Volatility:** `IMMUTABLE`

Returns `LOWER(TRIM(p_keyword))`. Used for consistent keyword normalization across
application code and database functions.

---

## Trigger Functions

All trigger functions are `RETURNS TRIGGER` with `LANGUAGE plpgsql`.

| Function | Purpose |
|----------|---------|
| `notify_review_progress()` | Builds a JSONB payload from the inserted row and calls `pg_notify('review_progress', ...)` |
| `update_papers_timestamp()` | Sets `NEW.updated_at = NOW()` |
| `update_review_request_timestamp()` | Sets `NEW.updated_at = NOW()` |
| `update_request_paper_mapping_timestamp()` | Sets `NEW.updated_at = NOW()` |
| `update_paper_sources_timestamp()` | Sets `NEW.updated_at = NOW()` |

The `notify_review_progress()` payload format:

```json
{
  "event_id": "<uuid>",
  "request_id": "<uuid>",
  "event_type": "<string>",
  "event_data": { ... },
  "created_at": "<timestamptz>"
}
```

---

## Triggers

| Trigger | Table | Event | Timing | Function |
|---------|-------|-------|--------|----------|
| `trg_review_progress_notify` | `review_progress_events` | `INSERT` | `AFTER` | `notify_review_progress()` |
| `trg_papers_updated_at` | `papers` | `UPDATE` | `BEFORE` | `update_papers_timestamp()` |
| `trg_review_requests_updated_at` | `literature_review_requests` | `UPDATE` | `BEFORE` | `update_review_request_timestamp()` |
| `trg_request_paper_mappings_updated_at` | `request_paper_mappings` | `UPDATE` | `BEFORE` | `update_request_paper_mapping_timestamp()` |
| `trg_paper_sources_updated_at` | `paper_sources` | `UPDATE` | `BEFORE` | `update_paper_sources_timestamp()` |

---

## Performance Indexes

Migration 8 adds four composite indexes optimized for the most frequent query patterns:

| Index | Table | Column(s) | Query Pattern |
|-------|-------|-----------|---------------|
| `idx_keyword_searches_keyword_source_searched` | `keyword_searches` | `keyword_id, source_api, searched_at DESC` | `GetLastSearch`: `WHERE keyword_id = $1 AND source_api = $2 ORDER BY searched_at DESC LIMIT 1` |
| `idx_review_requests_org_project_created` | `literature_review_requests` | `org_id, project_id, created_at DESC` | `List`: `WHERE org_id = $1 AND project_id = $2 ORDER BY created_at DESC` |
| `idx_review_requests_id_org_project` | `literature_review_requests` | `id, org_id, project_id` | `Get`: `WHERE id = $1 AND org_id = $2 AND project_id = $3` |
| `idx_keyword_paper_mappings_keyword_created` | `keyword_paper_mappings` | `keyword_id, created_at DESC` | `GetPapersForKeyword`: `WHERE keyword_id = $1 ORDER BY created_at DESC` |

---

## Entity Relationship Diagram

```
 ┌──────────────────────────────────────────────────────────────────────────────────────┐
 │                              CORE DOMAIN TABLES                                     │
 │                                                                                     │
 │   ┌─────────────┐         ┌────────────────────┐         ┌──────────────────┐       │
 │   │  keywords    │         │      papers         │         │  paper_sources   │       │
 │   ├─────────────┤         ├────────────────────┤         ├──────────────────┤       │
 │   │ id (PK)     │         │ id (PK)            │────┐    │ id (PK)          │       │
 │   │ keyword     │         │ canonical_id (UQ)  │    │    │ paper_id (FK)  ──┼───┐   │
 │   │ normalized  │         │ title              │    │    │ source_api       │   │   │
 │   │  _keyword   │         │ abstract           │    │    │ source_metadata  │   │   │
 │   │ created_at  │         │ authors            │    │    │ created_at       │   │   │
 │   └──────┬──────┘         │ publication_date   │    │    │ updated_at       │   │   │
 │          │                │ publication_year   │    │    └──────────────────┘   │   │
 │          │                │ venue / journal    │    │                           │   │
 │          │                │ citation_count     │    │    ┌──────────────────┐   │   │
 │          │                │ pdf_url            │    │    │paper_identifiers │   │   │
 │          │                │ open_access        │    │    ├──────────────────┤   │   │
 │          │                │ keywords_extracted │    │    │ id (PK)          │   │   │
 │          │                │ raw_metadata       │    ├───▶│ paper_id (FK)    │   │   │
 │          │                │ created_at         │    │    │ identifier_type  │   │   │
 │          │                │ updated_at         │    │    │ identifier_value │   │   │
 │          │                └────────┬───────────┘    │    │ source_api       │   │   │
 │          │                         │                │    │ discovered_at    │   │   │
 │          │                         │                │    └──────────────────┘   │   │
 │          │     ┌───────────────────┘                │                           │   │
 │          │     │                                    │                           │   │
 │          ▼     ▼                                    │                           │   │
 │   ┌────────────────────────┐                        │                           │   │
 │   │ keyword_paper_mappings │◀───────────────────────┤                           │   │
 │   ├────────────────────────┤                        │                           │   │
 │   │ id (PK)               │                        │                           │   │
 │   │ keyword_id (FK) ──────┼── keywords              │                           │   │
 │   │ paper_id (FK) ────────┼── papers                │                           │   │
 │   │ mapping_type          │                        │                           │   │
 │   │ source_type           │                        │                           │   │
 │   │ confidence_score      │                        │                           │   │
 │   │ created_at            │                        │                           │   │
 │   └────────────────────────┘                        │                           │   │
 │                                                     │                           │   │
 │   ┌────────────────────┐                            │                           │   │
 │   │ keyword_searches   │                            │                           │   │
 │   ├────────────────────┤                            │                           │   │
 │   │ id (PK)            │                            │                           │   │
 │   │ keyword_id (FK) ───┼── keywords                 │                           │   │
 │   │ source_api         │                            │                           │   │
 │   │ searched_at        │                            │                           │   │
 │   │ date_from / date_to│                            │                           │   │
 │   │ search_window_hash │                            │                           │   │
 │   │ papers_found       │                            │                           │   │
 │   │ status             │                            │                           │   │
 │   │ error_message      │                            │                           │   │
 │   └────────────────────┘                            │                           │   │
 └─────────────────────────────────────────────────────┼───────────────────────────┼───┘
                                                       │                           │
 ┌─────────────────────────────────────────────────────┼───────────────────────────┼───┐
 │                          TENANT DOMAIN TABLES       │                           │   │
 │                                                     │                           │   │
 │   ┌────────────────────────────────┐                │                           │   │
 │   │ literature_review_requests     │                │                           │   │
 │   ├────────────────────────────────┤                │                           │   │
 │   │ id (PK)                       │──────┐         │                           │   │
 │   │ org_id                        │      │         │                           │   │
 │   │ project_id                    │      │         │                           │   │
 │   │ user_id                       │      │         │                           │   │
 │   │ original_query                │      │         │                           │   │
 │   │ temporal_workflow_id          │      │         │                           │   │
 │   │ temporal_run_id               │      │         │                           │   │
 │   │ status                        │      │         │                           │   │
 │   │ keywords/papers counts        │      │         │                           │   │
 │   │ expansion_depth (0-5)         │      │         │                           │   │
 │   │ config_snapshot               │      │         │                           │   │
 │   │ source_filters                │      │         │                           │   │
 │   │ date_from / date_to           │      │         │                           │   │
 │   │ created_at / updated_at       │      │         │                           │   │
 │   │ started_at / completed_at     │      │         │                           │   │
 │   └────────────────────────────────┘      │         │                           │   │
 │                                           │         │                           │   │
 │       ┌───────────────────────────────────┼─────────┤                           │   │
 │       │                                   │         │                           │   │
 │       ▼                                   ▼         ▼                           │   │
 │   ┌──────────────────────────┐    ┌──────────────────────────────┐              │   │
 │   │ request_keyword_mappings │    │  request_paper_mappings      │              │   │
 │   ├──────────────────────────┤    ├──────────────────────────────┤              │   │
 │   │ id (PK)                 │    │ id (PK)                      │              │   │
 │   │ request_id (FK) ────────┼─┐  │ request_id (FK) ─────────────┼─┐            │   │
 │   │ keyword_id (FK) ────────┼─┼──│── discovered_via_keyword_id  │ │            │   │
 │   │ extraction_round        │ │  │ paper_id (FK) ───────────────┼─┼────────────┘   │
 │   │ source_paper_id (FK) ───┼─┼──│── discovered_via_source      │ │                │
 │   │ source_type             │ │  │ expansion_depth              │ │                │
 │   │ created_at              │ │  │ ingestion_status             │ │                │
 │   └──────────────────────────┘ │  │ ingestion_job_id            │ │                │
 │                                │  │ ingestion_error             │ │                │
 │       literature_review ───────┘  │ created_at / updated_at     │ │                │
 │       _requests                   └──────────────────────────────┘ │                │
 │                                    literature_review ──────────────┘                │
 │                                    _requests                                        │
 │                                                                                     │
 │   ┌──────────────────────────┐                                                      │
 │   │ review_progress_events   │                                                      │
 │   ├──────────────────────────┤                                                      │
 │   │ id (PK)                 │                                                      │
 │   │ request_id (FK) ────────┼── literature_review_requests                          │
 │   │ event_type              │                                                      │
 │   │ event_data              │     AFTER INSERT ──▶ pg_notify('review_progress')     │
 │   │ created_at              │                                                      │
 │   └──────────────────────────┘                                                      │
 └─────────────────────────────────────────────────────────────────────────────────────┘

 ┌─────────────────────────────────────────────────────────────────────────────────────┐
 │                            OUTBOX PATTERN                                           │
 │                                                                                     │
 │   ┌─────────────────────────┐         ┌─────────────────────────┐                   │
 │   │    outbox_events        │         │   outbox_dead_letter    │                   │
 │   ├─────────────────────────┤         ├─────────────────────────┤                   │
 │   │ id (PK, BIGSERIAL)     │         │ id (PK, BIGSERIAL)     │                   │
 │   │ event_id               │────────▶│ original_event_id      │                   │
 │   │ event_version          │         │ event_version           │                   │
 │   │ aggregate_id/type      │         │ aggregate_id/type       │                   │
 │   │ event_type             │         │ event_type              │                   │
 │   │ payload (BYTEA)        │         │ payload (BYTEA)         │                   │
 │   │ scope / org / project  │         │ org_id / project_id     │                   │
 │   │ metadata               │         │ metadata                │                   │
 │   │ status                 │         │ failure_reason          │                   │
 │   │ publish_attempts       │         │ retry_count             │                   │
 │   │ locked_by / locked_at  │         │ original_created_at     │                   │
 │   │ lock_expires_at        │         │ failed_at               │                   │
 │   │ next_attempt_at        │         │ reprocessed_at          │                   │
 │   │ created_at             │         │ reprocess_count         │                   │
 │   │ processed_at           │         └─────────────────────────┘                   │
 │   └─────────────────────────┘                                                      │
 │                                                                                     │
 │   Flow: INSERT within TX ──▶ Worker polls pending ──▶ Lease lock ──▶ Publish to     │
 │         Kafka ──▶ Mark published  |  On max_attempts exceeded ──▶ Move to DLQ       │
 └─────────────────────────────────────────────────────────────────────────────────────┘
```

### Relationship Summary

| From | To | Relationship | FK Action |
|------|----|-------------|-----------|
| `paper_identifiers.paper_id` | `papers.id` | Many-to-one | `ON DELETE CASCADE` |
| `paper_sources.paper_id` | `papers.id` | Many-to-one | `ON DELETE CASCADE` |
| `keyword_searches.keyword_id` | `keywords.id` | Many-to-one | `ON DELETE CASCADE` |
| `keyword_paper_mappings.keyword_id` | `keywords.id` | Many-to-one | `ON DELETE CASCADE` |
| `keyword_paper_mappings.paper_id` | `papers.id` | Many-to-one | `ON DELETE CASCADE` |
| `request_keyword_mappings.request_id` | `literature_review_requests.id` | Many-to-one | `ON DELETE CASCADE` |
| `request_keyword_mappings.keyword_id` | `keywords.id` | Many-to-one | `ON DELETE CASCADE` |
| `request_keyword_mappings.source_paper_id` | `papers.id` | Many-to-one (optional) | `ON DELETE SET NULL` |
| `request_paper_mappings.request_id` | `literature_review_requests.id` | Many-to-one | `ON DELETE CASCADE` |
| `request_paper_mappings.paper_id` | `papers.id` | Many-to-one | `ON DELETE CASCADE` |
| `request_paper_mappings.discovered_via_keyword_id` | `keywords.id` | Many-to-one (optional) | `ON DELETE SET NULL` |
| `review_progress_events.request_id` | `literature_review_requests.id` | Many-to-one | `ON DELETE CASCADE` |

---

## Notes

**Multi-tenancy.** Tenant isolation is enforced at the application layer via `org_id` and
`project_id` on `literature_review_requests`. All queries to tenant-scoped data must
include both identifiers. Core tables (`papers`, `keywords`) are shared across tenants to
enable global deduplication.

**Temporal integration.** The `temporal_workflow_id` and `temporal_run_id` columns on
`literature_review_requests` link each review to its Temporal workflow execution. The
workflow coordinates keyword extraction, paper searching, expansion, and ingestion.

**Real-time progress.** The `review_progress_events` table combined with the
`trg_review_progress_notify` trigger provides a PostgreSQL NOTIFY-based mechanism for
pushing progress updates to the HTTP/SSE layer without polling.

**Outbox pattern.** Events are written to `outbox_events` within the same database
transaction as the domain operation. A background worker polls for pending events,
acquires lease-based locks, publishes to Kafka, and marks them as published. Events that
exhaust their `max_attempts` are moved to `outbox_dead_letter` for manual review.

**Fuzzy search.** GIN trigram indexes on `keywords.keyword`, `papers.title`, and
`papers.abstract` enable efficient `LIKE`, `ILIKE`, and similarity searches using the
`pg_trgm` extension.
