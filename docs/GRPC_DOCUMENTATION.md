# Literature Review Service -- gRPC API Documentation

## Overview

The Literature Review Service gRPC API provides a strongly-typed interface for starting, monitoring, and managing automated literature reviews. The service is defined using Protocol Buffers v3 and exposes both unary and server-streaming RPCs.

## Service Definition

```protobuf
package literaturereview.v1;

service LiteratureReviewService { ... }
```

| Property | Value |
|----------|-------|
| Proto package | `literaturereview.v1` |
| Go package | `github.com/helixir/literature-review-service/gen/proto/literaturereview/v1;literaturereviewv1` |
| Proto file | `api/proto/literaturereview/v1/literature_review.proto` |
| Imports | `google/protobuf/timestamp.proto`, `google/protobuf/duration.proto`, `google/protobuf/wrappers.proto` |

## Authentication

All RPCs enforce multi-tenant access control via the shared `grpcauth` package. Authentication metadata must be provided as gRPC metadata:

- JWT tokens are verified by a gRPC interceptor.
- `org_id` and `project_id` fields in every request are validated against the JWT claims.
- If the auth context is absent (e.g., service-to-service calls), requests are processed in permissive mode.
- Tenant access is verified: the user must have org-level access and either be an org admin or have a project-level role.

---

## RPCs

### 1. StartLiteratureReview

Creates a new literature review record and starts the Temporal workflow that orchestrates keyword extraction, paper search, expansion, and ingestion.

```protobuf
rpc StartLiteratureReview(StartLiteratureReviewRequest) returns (StartLiteratureReviewResponse);
```

**Type:** Unary

**Request: `StartLiteratureReviewRequest`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `org_id` | `string` | 1 | Organization identifier. Required. |
| `project_id` | `string` | 2 | Project identifier. Required. |
| `query` | `string` | 3 | Natural language research query. Required. Must be 3--10,000 characters. |
| `initial_keyword_count` | `google.protobuf.Int32Value` | 4 | Maximum keywords to extract from the initial query. Optional. Default: 10. |
| `paper_keyword_count` | `google.protobuf.Int32Value` | 5 | Maximum keywords to extract per paper during expansion. Optional. Defaults to `initial_keyword_count`. |
| `max_expansion_depth` | `google.protobuf.Int32Value` | 6 | Maximum recursive search depth. Optional. Default: 2. |
| `source_filters` | `repeated string` | 7 | Paper sources to search (e.g., `"semantic_scholar"`, `"openalex"`, `"pubmed"`). Optional. Defaults to `["semantic_scholar", "openalex", "pubmed"]`. |
| `date_from` | `google.protobuf.Timestamp` | 8 | Earliest publication date filter. Optional. |
| `date_to` | `google.protobuf.Timestamp` | 9 | Latest publication date filter. Optional. |

**Response: `StartLiteratureReviewResponse`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `review_id` | `string` | 1 | UUID of the created review |
| `workflow_id` | `string` | 2 | Temporal workflow ID |
| `status` | `ReviewStatus` | 3 | Initial status (always `REVIEW_STATUS_PENDING`) |
| `initial_keywords` | `repeated string` | 4 | Reserved for future use (initial keywords extracted) |
| `created_at` | `google.protobuf.Timestamp` | 5 | Timestamp when the review was created |
| `message` | `string` | 6 | Human-readable status message |

---

### 2. GetLiteratureReviewStatus

Retrieves the current status, progress, configuration, and timestamps for a specific literature review.

```protobuf
rpc GetLiteratureReviewStatus(GetLiteratureReviewStatusRequest) returns (GetLiteratureReviewStatusResponse);
```

**Type:** Unary

**Request: `GetLiteratureReviewStatusRequest`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `org_id` | `string` | 1 | Organization identifier. Required. |
| `project_id` | `string` | 2 | Project identifier. Required. |
| `review_id` | `string` | 3 | UUID of the review. Required. |

**Response: `GetLiteratureReviewStatusResponse`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `review_id` | `string` | 1 | UUID of the review |
| `status` | `ReviewStatus` | 2 | Current review lifecycle status |
| `progress` | `ReviewProgress` | 3 | Current progress counters and source breakdown |
| `error_message` | `string` | 4 | Error details if the review failed (empty otherwise) |
| `created_at` | `google.protobuf.Timestamp` | 5 | When the review was created |
| `started_at` | `google.protobuf.Timestamp` | 6 | When the workflow started processing (null if not started) |
| `completed_at` | `google.protobuf.Timestamp` | 7 | When the review reached a terminal state (null if running) |
| `duration` | `google.protobuf.Duration` | 8 | Elapsed or total duration (null if not started) |
| `configuration` | `ReviewConfiguration` | 9 | The review configuration that was used |

---

### 3. CancelLiteratureReview

Requests cancellation of a running literature review by sending a cancel signal to the Temporal workflow. The review must be in a non-terminal state.

```protobuf
rpc CancelLiteratureReview(CancelLiteratureReviewRequest) returns (CancelLiteratureReviewResponse);
```

**Type:** Unary

**Request: `CancelLiteratureReviewRequest`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `org_id` | `string` | 1 | Organization identifier. Required. |
| `project_id` | `string` | 2 | Project identifier. Required. |
| `review_id` | `string` | 3 | UUID of the review. Required. |
| `reason` | `string` | 4 | Human-readable reason for cancellation. Optional. |

**Response: `CancelLiteratureReviewResponse`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `success` | `bool` | 1 | Whether the cancellation signal was sent |
| `message` | `string` | 2 | Human-readable status message |
| `final_status` | `ReviewStatus` | 3 | The review status at the time of the cancellation request |

**Error:** Returns `FAILED_PRECONDITION` if the review is already in a terminal state.

---

### 4. ListLiteratureReviews

Returns a paginated list of literature review summaries for a given organization and project, with optional filtering by status and date range.

```protobuf
rpc ListLiteratureReviews(ListLiteratureReviewsRequest) returns (ListLiteratureReviewsResponse);
```

**Type:** Unary

**Request: `ListLiteratureReviewsRequest`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `org_id` | `string` | 1 | Organization identifier. Required. |
| `project_id` | `string` | 2 | Project identifier. Required. |
| `page_size` | `int32` | 3 | Number of results per page. Range: 1--100. Default: 50. |
| `page_token` | `string` | 4 | Pagination token from a previous response. |
| `status_filter` | `ReviewStatus` | 5 | Filter by review status. Use `REVIEW_STATUS_UNSPECIFIED` for no filter. |
| `created_after` | `google.protobuf.Timestamp` | 6 | Only return reviews created after this timestamp. Optional. |
| `created_before` | `google.protobuf.Timestamp` | 7 | Only return reviews created before this timestamp. Optional. |

**Response: `ListLiteratureReviewsResponse`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `reviews` | `repeated LiteratureReviewSummary` | 1 | List of review summaries |
| `next_page_token` | `string` | 2 | Token for the next page. Empty if no more results. |
| `total_count` | `int32` | 3 | Total number of reviews matching the filters |

---

### 5. GetLiteratureReviewPapers

Retrieves a paginated list of papers discovered during a literature review, with optional filtering by ingestion status and paper source.

```protobuf
rpc GetLiteratureReviewPapers(GetLiteratureReviewPapersRequest) returns (GetLiteratureReviewPapersResponse);
```

**Type:** Unary

**Request: `GetLiteratureReviewPapersRequest`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `org_id` | `string` | 1 | Organization identifier. Required. |
| `project_id` | `string` | 2 | Project identifier. Required. |
| `review_id` | `string` | 3 | UUID of the review. Required. |
| `page_size` | `int32` | 4 | Number of results per page. Range: 1--100. Default: 50. |
| `page_token` | `string` | 5 | Pagination token from a previous response. |
| `ingestion_status_filter` | `IngestionStatus` | 6 | Filter by ingestion status. Use `INGESTION_STATUS_UNSPECIFIED` for no filter. |
| `source_filter` | `string` | 7 | Filter by paper source (e.g., `"semantic_scholar"`). |

**Response: `GetLiteratureReviewPapersResponse`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `papers` | `repeated Paper` | 1 | List of paper objects |
| `next_page_token` | `string` | 2 | Token for the next page. Empty if no more results. |
| `total_count` | `int32` | 3 | Total number of papers matching the filters |

---

### 6. GetLiteratureReviewKeywords

Retrieves a paginated list of keywords extracted and used during a literature review, with optional filtering by extraction round and source type.

```protobuf
rpc GetLiteratureReviewKeywords(GetLiteratureReviewKeywordsRequest) returns (GetLiteratureReviewKeywordsResponse);
```

**Type:** Unary

**Request: `GetLiteratureReviewKeywordsRequest`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `org_id` | `string` | 1 | Organization identifier. Required. |
| `project_id` | `string` | 2 | Project identifier. Required. |
| `review_id` | `string` | 3 | UUID of the review. Required. |
| `page_size` | `int32` | 4 | Number of results per page. Range: 1--100. Default: 50. |
| `page_token` | `string` | 5 | Pagination token from a previous response. |
| `extraction_round_filter` | `google.protobuf.Int32Value` | 6 | Filter by extraction round (0 = initial, 1+ = expansion). Optional. |
| `source_type_filter` | `KeywordSourceType` | 7 | Filter by keyword source type. Use `KEYWORD_SOURCE_TYPE_UNSPECIFIED` for no filter. |

**Response: `GetLiteratureReviewKeywordsResponse`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `keywords` | `repeated ReviewKeyword` | 1 | List of keyword objects |
| `next_page_token` | `string` | 2 | Token for the next page. Empty if no more results. |
| `total_count` | `int32` | 3 | Total number of keywords matching the filters |

---

### 7. StreamLiteratureReviewProgress

Streams real-time progress updates for a literature review. The server polls the database at 2-second intervals and sends progress events until the review reaches a terminal state or the 4-hour maximum stream duration is exceeded.

```protobuf
rpc StreamLiteratureReviewProgress(StreamLiteratureReviewProgressRequest) returns (stream LiteratureReviewProgressEvent);
```

**Type:** Server streaming

**Request: `StreamLiteratureReviewProgressRequest`**

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `org_id` | `string` | 1 | Organization identifier. Required. |
| `project_id` | `string` | 2 | Project identifier. Required. |
| `review_id` | `string` | 3 | UUID of the review. Required. |

**Response: `stream LiteratureReviewProgressEvent`**

A stream of `LiteratureReviewProgressEvent` messages. See the [LiteratureReviewProgressEvent](#literaturereviewprogressevent) section below for the message schema.

**Stream behavior:**

- If the review is already in a terminal state, a single event with `event_type = "completed"` is sent and the stream closes.
- If the review has no associated Temporal workflow, a `FAILED_PRECONDITION` error is returned.
- The stream sends a `"stream_started"` event immediately upon connection.
- Progress updates (`"progress_update"`) are sent every 2 seconds.
- When the review reaches a terminal state, a `"completed"` event is sent and the stream closes.
- The maximum stream duration is 4 hours; after that, the stream closes with `DEADLINE_EXCEEDED`.

---

## Enums

### ReviewStatus

Represents the lifecycle states of a literature review.

| Value | Number | Description |
|-------|--------|-------------|
| `REVIEW_STATUS_UNSPECIFIED` | 0 | Default/unset value |
| `REVIEW_STATUS_PENDING` | 1 | Review created, workflow not yet started |
| `REVIEW_STATUS_EXTRACTING_KEYWORDS` | 2 | LLM is extracting search keywords from the query |
| `REVIEW_STATUS_SEARCHING` | 3 | Paper sources are being queried |
| `REVIEW_STATUS_EXPANDING` | 4 | Recursive expansion round in progress |
| `REVIEW_STATUS_INGESTING` | 5 | Papers being submitted to the ingestion service |
| `REVIEW_STATUS_COMPLETED` | 6 | Review finished successfully (terminal) |
| `REVIEW_STATUS_FAILED` | 7 | Unrecoverable error (terminal) |
| `REVIEW_STATUS_CANCELLED` | 8 | Cancelled by user or system (terminal) |
| `REVIEW_STATUS_PARTIAL` | 9 | Finished with some failures or skips (terminal) |

**Domain mapping:**

| Proto Value | Domain String |
|-------------|--------------|
| `REVIEW_STATUS_PENDING` | `"pending"` |
| `REVIEW_STATUS_EXTRACTING_KEYWORDS` | `"extracting_keywords"` |
| `REVIEW_STATUS_SEARCHING` | `"searching"` |
| `REVIEW_STATUS_EXPANDING` | `"expanding"` |
| `REVIEW_STATUS_INGESTING` | `"ingesting"` |
| `REVIEW_STATUS_COMPLETED` | `"completed"` |
| `REVIEW_STATUS_FAILED` | `"failed"` |
| `REVIEW_STATUS_CANCELLED` | `"cancelled"` |
| `REVIEW_STATUS_PARTIAL` | `"partial"` |

### IngestionStatus

Represents the ingestion state of a paper within a review.

| Value | Number | Description |
|-------|--------|-------------|
| `INGESTION_STATUS_UNSPECIFIED` | 0 | Default/unset value |
| `INGESTION_STATUS_PENDING` | 1 | Not yet submitted for ingestion |
| `INGESTION_STATUS_QUEUED` | 2 | Submitted to the ingestion service |
| `INGESTION_STATUS_INGESTING` | 3 | Ingestion service is actively processing |
| `INGESTION_STATUS_COMPLETED` | 4 | Successfully ingested |
| `INGESTION_STATUS_FAILED` | 5 | Ingestion failed |
| `INGESTION_STATUS_SKIPPED` | 6 | Skipped (e.g., no PDF URL available) |

**Domain mapping:**

| Proto Value | Domain String |
|-------------|--------------|
| `INGESTION_STATUS_PENDING` | `"pending"` |
| `INGESTION_STATUS_QUEUED` | `"submitted"` |
| `INGESTION_STATUS_INGESTING` | `"processing"` |
| `INGESTION_STATUS_COMPLETED` | `"completed"` |
| `INGESTION_STATUS_FAILED` | `"failed"` |
| `INGESTION_STATUS_SKIPPED` | `"skipped"` |

### KeywordSourceType

Identifies how a keyword was obtained.

| Value | Number | Description |
|-------|--------|-------------|
| `KEYWORD_SOURCE_TYPE_UNSPECIFIED` | 0 | Default/unset value |
| `KEYWORD_SOURCE_TYPE_USER_QUERY` | 1 | Extracted from the user's original query |
| `KEYWORD_SOURCE_TYPE_PAPER_EXTRACTION` | 2 | Extracted from a discovered paper by the LLM |

**Domain mapping:**

| Proto Value | Domain String |
|-------------|--------------|
| `KEYWORD_SOURCE_TYPE_USER_QUERY` | `"query"` |
| `KEYWORD_SOURCE_TYPE_PAPER_EXTRACTION` | `"llm_extraction"` |

### SortOrder

Sort direction for list queries.

| Value | Number | Description |
|-------|--------|-------------|
| `SORT_ORDER_UNSPECIFIED` | 0 | Default (server decides) |
| `SORT_ORDER_ASC` | 1 | Ascending |
| `SORT_ORDER_DESC` | 2 | Descending |

---

## Domain Messages

### ReviewProgress

Tracks the overall progress of a literature review, including per-source breakdowns.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `initial_keywords_count` | `int32` | 1 | Number of keywords configured for initial extraction |
| `total_keywords_processed` | `int32` | 2 | Total keywords extracted and searched so far |
| `papers_found` | `int32` | 3 | Total unique papers discovered |
| `papers_new` | `int32` | 4 | New papers not previously known to the system |
| `papers_ingested` | `int32` | 5 | Papers successfully ingested |
| `papers_failed` | `int32` | 6 | Papers that failed during ingestion |
| `current_expansion_depth` | `int32` | 7 | Current recursive expansion depth |
| `max_expansion_depth` | `int32` | 8 | Configured maximum expansion depth |
| `source_progress` | `map<string, SourceProgress>` | 9 | Per-source progress, keyed by source name |
| `elapsed_time` | `google.protobuf.Duration` | 10 | Time elapsed since the review started |
| `estimated_remaining` | `google.protobuf.Duration` | 11 | Estimated time remaining (if available) |

### SourceProgress

Progress counters for a single paper source API.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `source_name` | `string` | 1 | Name of the paper source (e.g., `"semantic_scholar"`) |
| `queries_completed` | `int32` | 2 | Number of keyword queries completed against this source |
| `queries_total` | `int32` | 3 | Total keyword queries planned for this source |
| `papers_found` | `int32` | 4 | Papers discovered from this source |
| `errors` | `int32` | 5 | Number of errors encountered querying this source |
| `rate_limited` | `bool` | 6 | Whether this source is currently rate-limiting requests |

### ReviewConfiguration

The configuration parameters used for a literature review.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `initial_keyword_count` | `int32` | 1 | Maximum keywords to extract from the initial query |
| `paper_keyword_count` | `int32` | 2 | Maximum keywords to extract per paper during expansion |
| `max_expansion_depth` | `int32` | 3 | Maximum recursive search depth |
| `enabled_sources` | `repeated string` | 4 | Paper sources that are enabled for this review |
| `date_from` | `google.protobuf.Timestamp` | 5 | Earliest publication date filter |
| `date_to` | `google.protobuf.Timestamp` | 6 | Latest publication date filter |

### LiteratureReviewSummary

A compact summary of a literature review, used in list responses.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `review_id` | `string` | 1 | UUID of the review |
| `original_query` | `string` | 2 | The user's original research query |
| `status` | `ReviewStatus` | 3 | Current review lifecycle status |
| `papers_found` | `int32` | 4 | Total papers discovered |
| `papers_ingested` | `int32` | 5 | Papers successfully ingested |
| `keywords_used` | `int32` | 6 | Total keywords extracted and used |
| `created_at` | `google.protobuf.Timestamp` | 7 | When the review was created |
| `completed_at` | `google.protobuf.Timestamp` | 8 | When the review completed (null if running) |
| `duration` | `google.protobuf.Duration` | 9 | Elapsed or total duration |
| `user_id` | `string` | 10 | ID of the user who initiated the review |

### Paper

Represents an academic paper discovered during a literature review.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `id` | `string` | 1 | Internal UUID |
| `doi` | `string` | 2 | Digital Object Identifier |
| `arxiv_id` | `string` | 3 | arXiv paper identifier |
| `pubmed_id` | `string` | 4 | PubMed identifier (PMID) |
| `semantic_scholar_id` | `string` | 5 | Semantic Scholar corpus ID |
| `openalex_id` | `string` | 6 | OpenAlex work ID |
| `title` | `string` | 7 | Paper title |
| `abstract` | `string` | 8 | Paper abstract |
| `authors` | `repeated Author` | 9 | List of authors |
| `publication_date` | `google.protobuf.Timestamp` | 10 | Publication date |
| `publication_year` | `int32` | 11 | Publication year |
| `venue` | `string` | 12 | Publication venue |
| `journal` | `string` | 13 | Journal name |
| `citation_count` | `int32` | 14 | Number of citations |
| `pdf_url` | `string` | 15 | URL to the paper PDF |
| `open_access` | `bool` | 16 | Whether the paper is open access |
| `discovered_via_source` | `string` | 17 | Source API that discovered this paper |
| `discovered_via_keyword` | `string` | 18 | Keyword that led to this paper's discovery |
| `expansion_depth` | `int32` | 19 | Expansion depth at which this paper was found |
| `ingestion_status` | `IngestionStatus` | 20 | Current ingestion status |
| `ingestion_job_id` | `string` | 21 | Ingestion service job ID |
| `extracted_keywords` | `repeated string` | 22 | Keywords extracted from this paper by the LLM |

### Author

Represents a paper author.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `name` | `string` | 1 | Author full name |
| `affiliation` | `string` | 2 | Institutional affiliation |
| `orcid` | `string` | 3 | ORCID identifier |

### ReviewKeyword

Represents a keyword extracted and used during a literature review.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `id` | `string` | 1 | Internal UUID |
| `keyword` | `string` | 2 | The keyword as extracted |
| `normalized_keyword` | `string` | 3 | Lowercased, trimmed form used for deduplication |
| `source_type` | `KeywordSourceType` | 4 | How the keyword was obtained |
| `extraction_round` | `int32` | 5 | Extraction round (0 = initial, 1+ = expansion) |
| `source_paper_id` | `string` | 6 | UUID of the paper this keyword was extracted from (if applicable) |
| `source_paper_title` | `string` | 7 | Title of the source paper (if applicable) |
| `papers_found` | `int32` | 8 | Number of papers found using this keyword |
| `confidence_score` | `float` | 9 | LLM confidence score for this keyword extraction |

---

## Event Messages

### LiteratureReviewProgressEvent

The top-level event message streamed by `StreamLiteratureReviewProgress`. Contains common metadata fields and a `oneof event_data` for type-specific payloads.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `review_id` | `string` | 1 | UUID of the review this event belongs to |
| `event_type` | `string` | 2 | Event type identifier (e.g., `"progress_update"`, `"completed"`) |
| `status` | `ReviewStatus` | 3 | Current review status at the time of the event |
| `progress` | `ReviewProgress` | 4 | Current progress snapshot |
| `message` | `string` | 5 | Human-readable event description |
| `timestamp` | `google.protobuf.Timestamp` | 6 | When the event was generated |
| `event_data` | `oneof` | 10--14 | Type-specific event payload (see below) |

**`oneof event_data` variants:**

| Variant | Type | Number | Description |
|---------|------|--------|-------------|
| `keywords_extracted` | `KeywordsExtractedEvent` | 10 | Keywords were extracted from a query or paper |
| `papers_found` | `PapersFoundEvent` | 11 | Papers were discovered from a source |
| `expansion_started` | `ExpansionStartedEvent` | 12 | A new expansion round began |
| `ingestion_progress` | `IngestionProgressEvent` | 13 | Ingestion queue status update |
| `error` | `ErrorEvent` | 14 | An error occurred during processing |

### KeywordsExtractedEvent

Emitted when keywords are extracted from the user query or from a discovered paper.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `keywords` | `repeated string` | 1 | List of extracted keywords |
| `extraction_round` | `int32` | 2 | Round number (0 = initial query, 1+ = paper expansion) |
| `source_paper_id` | `string` | 3 | UUID of the source paper (empty for round 0) |

### PapersFoundEvent

Emitted when a source search returns results for a keyword.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `source` | `string` | 1 | Paper source name (e.g., `"semantic_scholar"`) |
| `keyword` | `string` | 2 | The keyword that was searched |
| `count` | `int32` | 3 | Total papers returned by the source |
| `new_count` | `int32` | 4 | Papers that are new (not previously discovered) |

### ExpansionStartedEvent

Emitted at the start of each recursive expansion round.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `depth` | `int32` | 1 | Current expansion depth (1-based) |
| `new_keywords` | `int32` | 2 | Number of new keywords to search in this round |
| `papers_to_process` | `int32` | 3 | Number of papers to extract keywords from |

### IngestionProgressEvent

Emitted as papers move through the ingestion pipeline.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `queued` | `int32` | 1 | Papers currently queued for ingestion |
| `completed` | `int32` | 2 | Papers successfully ingested |
| `failed` | `int32` | 3 | Papers that failed ingestion |

### ErrorEvent

Emitted when an error occurs during any phase of the review.

| Field | Type | Number | Description |
|-------|------|--------|-------------|
| `error_code` | `string` | 1 | Machine-readable error code |
| `error_message` | `string` | 2 | Human-readable error description |
| `phase` | `string` | 3 | Processing phase where the error occurred (e.g., `"searching"`, `"ingesting"`) |
| `recoverable` | `bool` | 4 | Whether the review can continue despite this error |

---

## Error Codes

The gRPC server maps domain errors to standard gRPC status codes. Internal error details are never leaked to clients.

### Error Code Mapping

| Domain Error | gRPC Code | Status Message |
|-------------|-----------|----------------|
| `ErrNotFound` | `NOT_FOUND` | `"resource not found"` |
| `ErrInvalidInput` | `INVALID_ARGUMENT` | Original validation message |
| `ErrAlreadyExists` | `ALREADY_EXISTS` | `"resource already exists"` |
| `ErrUnauthorized` | `UNAUTHENTICATED` | `"unauthorized"` |
| `ErrForbidden` | `PERMISSION_DENIED` | `"forbidden"` |
| `ErrRateLimited` | `RESOURCE_EXHAUSTED` | `"rate limit exceeded"` |
| `ErrServiceUnavailable` | `UNAVAILABLE` | `"service temporarily unavailable"` |
| `ErrCancelled` | `CANCELED` | `"request cancelled"` |
| `ErrWorkflowNotFound` | `NOT_FOUND` | `"resource not found"` |
| `ErrWorkflowAlreadyStarted` | `ALREADY_EXISTS` | `"resource already exists"` |
| Any other error | `INTERNAL` | `"internal server error"` |

### Validation Errors

| Condition | gRPC Code | Message |
|-----------|-----------|---------|
| Missing `org_id` | `INVALID_ARGUMENT` | `"org_id is required"` |
| Missing `project_id` | `INVALID_ARGUMENT` | `"project_id is required"` |
| Missing `query` | `INVALID_ARGUMENT` | `"query is required"` |
| Query too short | `INVALID_ARGUMENT` | `"query must be at least 3 characters"` |
| Query too long | `INVALID_ARGUMENT` | `"query must be at most 10000 characters"` |
| Invalid UUID | `INVALID_ARGUMENT` | `"invalid {field_name}: {details}"` |
| No org access | `PERMISSION_DENIED` | `"access denied to organization {orgID}"` |
| No project access | `PERMISSION_DENIED` | `"access denied to project {projectID}"` |
| Review already terminal | `FAILED_PRECONDITION` | `"review is already in terminal state"` |
| No associated workflow | `FAILED_PRECONDITION` | `"review has no associated workflow"` |
| Stream duration exceeded | `DEADLINE_EXCEEDED` | `"stream max duration exceeded"` |

---

## Pagination

All list RPCs use cursor-based pagination with base64-encoded page tokens.

| Parameter | Default | Maximum |
|-----------|---------|---------|
| `page_size` | 50 | 100 |

**Flow:**

1. Send a request with the desired `page_size` (or omit for the default of 50).
2. The response includes a `next_page_token`.
3. If `next_page_token` is non-empty, pass it as `page_token` in the next request.
4. If `next_page_token` is empty, all results have been returned.
5. The `total_count` field gives the total number of matching records.

---

## Client Example (Go)

The following example demonstrates connecting to the Literature Review Service and starting a new literature review using the generated Go client.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
)

func main() {
	// Connect to the gRPC server.
	conn, err := grpc.NewClient(
		"localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewLiteratureReviewServiceClient(conn)

	// Add JWT token to the request metadata.
	ctx := metadata.AppendToOutgoingContext(
		context.Background(),
		"authorization", "Bearer <your-jwt-token>",
	)

	// Start a literature review.
	resp, err := client.StartLiteratureReview(ctx, &pb.StartLiteratureReviewRequest{
		OrgId:     "org-123",
		ProjectId: "proj-456",
		Query:     "machine learning applications in drug discovery",
		InitialKeywordCount: wrapperspb.Int32(10),
		MaxExpansionDepth:   wrapperspb.Int32(2),
		SourceFilters:       []string{"semantic_scholar", "openalex"},
		DateFrom: timestamppb.New(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
		DateTo:   timestamppb.New(time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)),
	})
	if err != nil {
		log.Fatalf("failed to start review: %v", err)
	}

	fmt.Printf("Review started:\n")
	fmt.Printf("  Review ID:   %s\n", resp.ReviewId)
	fmt.Printf("  Workflow ID: %s\n", resp.WorkflowId)
	fmt.Printf("  Status:      %s\n", resp.Status)
	fmt.Printf("  Created At:  %s\n", resp.CreatedAt.AsTime())
	fmt.Printf("  Message:     %s\n", resp.Message)

	// Stream progress updates.
	stream, err := client.StreamLiteratureReviewProgress(ctx, &pb.StreamLiteratureReviewProgressRequest{
		OrgId:     "org-123",
		ProjectId: "proj-456",
		ReviewId:  resp.ReviewId,
	})
	if err != nil {
		log.Fatalf("failed to start progress stream: %v", err)
	}

	for {
		event, err := stream.Recv()
		if err != nil {
			log.Printf("stream closed: %v", err)
			break
		}

		fmt.Printf("[%s] %s - %s (papers: %d, ingested: %d)\n",
			event.Timestamp.AsTime().Format(time.RFC3339),
			event.EventType,
			event.Message,
			event.Progress.GetPapersFound(),
			event.Progress.GetPapersIngested(),
		)

		// Check for terminal event.
		if event.EventType == "completed" {
			break
		}
	}

	// Get final status.
	status, err := client.GetLiteratureReviewStatus(ctx, &pb.GetLiteratureReviewStatusRequest{
		OrgId:     "org-123",
		ProjectId: "proj-456",
		ReviewId:  resp.ReviewId,
	})
	if err != nil {
		log.Fatalf("failed to get status: %v", err)
	}

	fmt.Printf("\nFinal status: %s\n", status.Status)
	fmt.Printf("Papers found:    %d\n", status.Progress.PapersFound)
	fmt.Printf("Papers ingested: %d\n", status.Progress.PapersIngested)
	fmt.Printf("Papers failed:   %d\n", status.Progress.PapersFailed)
	fmt.Printf("Duration:        %s\n", status.Duration.AsDuration())

	// List papers from the review.
	papersResp, err := client.GetLiteratureReviewPapers(ctx, &pb.GetLiteratureReviewPapersRequest{
		OrgId:     "org-123",
		ProjectId: "proj-456",
		ReviewId:  resp.ReviewId,
		PageSize:  20,
	})
	if err != nil {
		log.Fatalf("failed to list papers: %v", err)
	}

	fmt.Printf("\nFirst %d of %d papers:\n", len(papersResp.Papers), papersResp.TotalCount)
	for _, p := range papersResp.Papers {
		fmt.Printf("  - [%d citations] %s\n", p.CitationCount, p.Title)
	}
}
```

---

## Proto File Location

The canonical service definition is located at:

```
api/proto/literaturereview/v1/literature_review.proto
```

Generated Go code is output to:

```
gen/proto/literaturereview/v1/
```

To regenerate the protobuf code:

```bash
make proto
```
