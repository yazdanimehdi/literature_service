# Literature Review Service -- HTTP REST API Documentation

## Overview

The Literature Review Service HTTP REST API provides endpoints for starting, monitoring, and managing automated literature reviews. The service extracts keywords from natural language queries using LLMs, searches multiple academic databases concurrently, recursively expands searches based on discovered papers, and orchestrates paper ingestion through the Ingestion Service.

## Base URL

```
/api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews
```

All literature review endpoints are scoped to an organization and project. The `{orgID}` and `{projectID}` path parameters are required for every API endpoint (except health checks) and enforce multi-tenant isolation.

## Authentication

All endpoints under `/api/v1/` require JWT authentication provided via the `Authorization` header:

```
Authorization: Bearer <jwt-token>
```

The JWT is verified by the auth middleware (powered by the shared `grpcauth` package). The token must contain claims granting access to the specified `orgID` and `projectID`.

Health check endpoints (`/healthz` and `/readyz`) do **not** require authentication.

## Middleware Stack

Requests pass through the following middleware in order:

| Order | Middleware | Source | Description |
|-------|-----------|--------|-------------|
| 1 | RequestID | Chi | Assigns a unique request ID to each request |
| 2 | RealIP | Chi | Extracts the real client IP from `X-Forwarded-For` / `X-Real-IP` headers |
| 3 | Recoverer | Chi | Recovers from panics and returns a 500 response |
| 4 | correlationIDMiddleware | Custom | Ensures every request has an `X-Correlation-ID` header (uses request ID, incoming header, or generates one) |
| 5 | jsonContentTypeMiddleware | Custom | Sets `Content-Type: application/json` on all responses |
| 6 | authMiddleware | grpcauth | Verifies JWT tokens and populates the auth context (applied only to `/api/v1/` routes) |
| 7 | tenantContextMiddleware | Custom | Extracts `orgID` and `projectID` from URL path parameters, validates tenant access against JWT claims |

## Common Headers

### Request Headers

| Header | Required | Description |
|--------|----------|-------------|
| `Authorization` | Yes (API routes) | Bearer JWT token |
| `Content-Type` | Yes (POST/DELETE with body) | Must be `application/json` |
| `X-Correlation-ID` | No | Client-provided correlation ID for distributed tracing. Auto-generated if absent. |

### Response Headers

| Header | Description |
|--------|-------------|
| `Content-Type` | `application/json` for all endpoints except SSE (`text/event-stream`) |
| `X-Correlation-ID` | Correlation ID for the request (echoed or generated) |

---

## Endpoints

### Start a Literature Review

```
POST /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews
```

Creates a new literature review request and starts the Temporal workflow that orchestrates the review pipeline (keyword extraction, paper search, expansion, and ingestion).

#### Request Body

```json
{
  "query": "machine learning applications in drug discovery",
  "initial_keyword_count": 10,
  "paper_keyword_count": 5,
  "max_expansion_depth": 2,
  "source_filters": ["semantic_scholar", "openalex"],
  "date_from": "2020-01-01T00:00:00Z",
  "date_to": "2024-12-31T23:59:59Z"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | **Yes** | Natural language research query. Must be 3--10,000 characters after trimming. |
| `initial_keyword_count` | integer | No | Maximum keywords to extract from the initial query. Default: 10. |
| `paper_keyword_count` | integer | No | Maximum keywords to extract per paper during expansion rounds. Defaults to `initial_keyword_count`. |
| `max_expansion_depth` | integer | No | Maximum recursive search depth. Default: 2. |
| `source_filters` | string[] | No | Paper sources to search. Default: `["semantic_scholar", "openalex", "pubmed"]`. Valid values: `semantic_scholar`, `openalex`, `pubmed`, `scopus`, `biorxiv`, `arxiv`. |
| `date_from` | string (RFC 3339) | No | Earliest publication date filter. |
| `date_to` | string (RFC 3339) | No | Latest publication date filter. |

#### Response (201 Created)

```json
{
  "review_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "workflow_id": "litreview-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "status": "pending",
  "created_at": "2024-01-15T10:30:00Z",
  "message": "literature review started"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `review_id` | string (UUID) | Unique identifier for the review |
| `workflow_id` | string | Temporal workflow ID for the review pipeline |
| `status` | string | Initial status, always `"pending"` |
| `created_at` | string (RFC 3339) | Timestamp when the review was created |
| `message` | string | Human-readable status message |

#### Example

```bash
curl -X POST \
  "https://api.example.com/api/v1/orgs/org-123/projects/proj-456/literature-reviews" \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "machine learning applications in drug discovery",
    "initial_keyword_count": 10,
    "max_expansion_depth": 2,
    "source_filters": ["semantic_scholar", "openalex"],
    "date_from": "2020-01-01T00:00:00Z",
    "date_to": "2024-12-31T23:59:59Z"
  }'
```

---

### List Literature Reviews

```
GET /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews
```

Returns a paginated list of literature review summaries for the specified organization and project.

#### Query Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page_size` | integer | No | Number of results per page. Range: 1--100. Default: 50. |
| `page_token` | string | No | Base64-encoded pagination token from a previous response. |
| `status` | string | No | Filter by review status. Valid values: `pending`, `extracting_keywords`, `searching`, `expanding`, `ingesting`, `completed`, `failed`, `cancelled`, `partial`. |
| `created_after` | string (RFC 3339) | No | Only return reviews created after this timestamp. |
| `created_before` | string (RFC 3339) | No | Only return reviews created before this timestamp. |

#### Response (200 OK)

```json
{
  "reviews": [
    {
      "review_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "original_query": "machine learning applications in drug discovery",
      "status": "completed",
      "papers_found": 87,
      "papers_ingested": 82,
      "keywords_used": 24,
      "created_at": "2024-01-15T10:30:00Z",
      "completed_at": "2024-01-15T11:15:42Z",
      "duration": "45m42s"
    }
  ],
  "next_page_token": "NTA=",
  "total_count": 42
}
```

| Field | Type | Description |
|-------|------|-------------|
| `reviews` | array | List of review summaries |
| `reviews[].review_id` | string (UUID) | Unique review identifier |
| `reviews[].original_query` | string | The original research query |
| `reviews[].status` | string | Current review status |
| `reviews[].papers_found` | integer | Total papers discovered |
| `reviews[].papers_ingested` | integer | Papers successfully ingested |
| `reviews[].keywords_used` | integer | Total keywords extracted and used |
| `reviews[].created_at` | string (RFC 3339) | When the review was created |
| `reviews[].completed_at` | string (RFC 3339) or null | When the review completed (null if still running) |
| `reviews[].duration` | string | Elapsed or total duration (Go duration format, e.g. `"45m42s"`) |
| `next_page_token` | string | Token for the next page. Empty string if no more results. |
| `total_count` | integer | Total number of reviews matching the filters |

#### Example

```bash
curl -X GET \
  "https://api.example.com/api/v1/orgs/org-123/projects/proj-456/literature-reviews?page_size=10&status=completed" \
  -H "Authorization: Bearer ${JWT_TOKEN}"
```

---

### Get Literature Review Status

```
GET /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews/{reviewID}
```

Returns the full status, progress, configuration, and timestamps for a specific literature review.

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `reviewID` | string (UUID) | The review identifier |

#### Response (200 OK)

```json
{
  "review_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "status": "searching",
  "progress": {
    "initial_keywords_count": 10,
    "total_keywords_processed": 10,
    "papers_found": 45,
    "papers_new": 45,
    "papers_ingested": 0,
    "papers_failed": 0,
    "max_expansion_depth": 2
  },
  "error_message": "",
  "created_at": "2024-01-15T10:30:00Z",
  "started_at": "2024-01-15T10:30:02Z",
  "completed_at": null,
  "duration": "5m12s",
  "configuration": {
    "initial_keyword_count": 10,
    "paper_keyword_count": 5,
    "max_expansion_depth": 2,
    "enabled_sources": ["semantic_scholar", "openalex"],
    "date_from": "2020-01-01T00:00:00Z",
    "date_to": "2024-12-31T23:59:59Z"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `review_id` | string (UUID) | Unique review identifier |
| `status` | string | Current review status |
| `progress` | object | Current progress counters |
| `progress.initial_keywords_count` | integer | Number of keywords configured for initial extraction |
| `progress.total_keywords_processed` | integer | Total keywords extracted so far |
| `progress.papers_found` | integer | Total unique papers discovered |
| `progress.papers_new` | integer | New papers (not previously known) |
| `progress.papers_ingested` | integer | Papers successfully ingested |
| `progress.papers_failed` | integer | Papers that failed ingestion |
| `progress.max_expansion_depth` | integer | Configured maximum expansion depth |
| `error_message` | string | Error details if the review failed (empty otherwise) |
| `created_at` | string (RFC 3339) | When the review was created |
| `started_at` | string (RFC 3339) or null | When the workflow started processing |
| `completed_at` | string (RFC 3339) or null | When the review reached a terminal state |
| `duration` | string | Elapsed or total duration |
| `configuration` | object | The review configuration used |

#### Example

```bash
curl -X GET \
  "https://api.example.com/api/v1/orgs/org-123/projects/proj-456/literature-reviews/a1b2c3d4-e5f6-7890-abcd-ef1234567890" \
  -H "Authorization: Bearer ${JWT_TOKEN}"
```

---

### Cancel a Literature Review

```
DELETE /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews/{reviewID}
```

Requests cancellation of a running literature review by sending a cancel signal to the Temporal workflow. The review must be in a non-terminal state.

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `reviewID` | string (UUID) | The review identifier |

#### Request Body (Optional)

```json
{
  "reason": "no longer needed"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `reason` | string | No | Human-readable reason for cancellation |

#### Response (200 OK)

```json
{
  "success": true,
  "message": "cancellation requested",
  "final_status": "searching"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `success` | boolean | Whether the cancellation signal was sent |
| `message` | string | Human-readable status message |
| `final_status` | string | The status of the review at the time of the cancellation request |

#### Error: Already Terminal (409 Conflict)

If the review is already in a terminal state (`completed`, `failed`, `cancelled`, `partial`):

```json
{
  "error": "review is already in terminal state"
}
```

#### Example

```bash
curl -X DELETE \
  "https://api.example.com/api/v1/orgs/org-123/projects/proj-456/literature-reviews/a1b2c3d4-e5f6-7890-abcd-ef1234567890" \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"reason": "no longer needed"}'
```

---

### List Papers for a Review

```
GET /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews/{reviewID}/papers
```

Returns a paginated list of papers discovered during a literature review.

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `reviewID` | string (UUID) | The review identifier |

#### Query Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page_size` | integer | No | Number of results per page. Range: 1--100. Default: 50. |
| `page_token` | string | No | Base64-encoded pagination token from a previous response. |
| `ingestion_status` | string | No | Filter by ingestion status. Valid values: `pending`, `submitted`, `processing`, `completed`, `failed`, `skipped`. |
| `source` | string | No | Filter by paper source. Valid values: `semantic_scholar`, `openalex`, `pubmed`, `scopus`, `biorxiv`, `arxiv`. |

#### Response (200 OK)

```json
{
  "papers": [
    {
      "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
      "canonical_id": "10.1038/s41586-023-06221-2",
      "title": "Machine learning for drug discovery: a comprehensive review",
      "abstract": "This review covers recent advances in...",
      "authors": [
        {
          "name": "Jane Smith",
          "affiliation": "MIT",
          "orcid": "0000-0001-2345-6789"
        }
      ],
      "publication_date": "2023-06-15T00:00:00Z",
      "publication_year": 2023,
      "venue": "Nature",
      "journal": "Nature",
      "citation_count": 142,
      "pdf_url": "https://example.com/paper.pdf",
      "open_access": true
    }
  ],
  "next_page_token": "NTA=",
  "total_count": 87
}
```

| Field | Type | Description |
|-------|------|-------------|
| `papers` | array | List of paper objects |
| `papers[].id` | string (UUID) | Internal paper identifier |
| `papers[].canonical_id` | string | Canonical external identifier (typically DOI) |
| `papers[].title` | string | Paper title |
| `papers[].abstract` | string | Paper abstract |
| `papers[].authors` | array | List of authors |
| `papers[].authors[].name` | string | Author full name |
| `papers[].authors[].affiliation` | string | Author institutional affiliation |
| `papers[].authors[].orcid` | string | Author ORCID identifier |
| `papers[].publication_date` | string (RFC 3339) or null | Publication date |
| `papers[].publication_year` | integer | Publication year |
| `papers[].venue` | string | Publication venue (conference or journal name) |
| `papers[].journal` | string | Journal name |
| `papers[].citation_count` | integer | Number of citations |
| `papers[].pdf_url` | string | URL to the paper PDF |
| `papers[].open_access` | boolean | Whether the paper is open access |
| `next_page_token` | string | Token for the next page |
| `total_count` | integer | Total number of papers matching the filters |

#### Example

```bash
curl -X GET \
  "https://api.example.com/api/v1/orgs/org-123/projects/proj-456/literature-reviews/a1b2c3d4-e5f6-7890-abcd-ef1234567890/papers?page_size=20&source=semantic_scholar" \
  -H "Authorization: Bearer ${JWT_TOKEN}"
```

---

### List Keywords for a Review

```
GET /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews/{reviewID}/keywords
```

Returns a paginated list of keywords extracted and used during a literature review.

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `reviewID` | string (UUID) | The review identifier |

#### Query Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page_size` | integer | No | Number of results per page. Range: 1--100. Default: 50. |
| `page_token` | string | No | Base64-encoded pagination token from a previous response. |
| `extraction_round` | integer | No | Filter by extraction round (0 = initial query extraction, 1+ = expansion rounds). |
| `source_type` | string | No | Filter by keyword source. Valid values: `query`, `llm_extraction`. |

#### Response (200 OK)

```json
{
  "keywords": [
    {
      "id": "c3d4e5f6-a7b8-9012-cdef-123456789012",
      "keyword": "drug discovery",
      "normalized_keyword": "drug discovery"
    },
    {
      "id": "d4e5f6a7-b8c9-0123-defa-234567890123",
      "keyword": "molecular docking",
      "normalized_keyword": "molecular docking"
    }
  ],
  "next_page_token": "",
  "total_count": 24
}
```

| Field | Type | Description |
|-------|------|-------------|
| `keywords` | array | List of keyword objects |
| `keywords[].id` | string (UUID) | Internal keyword identifier |
| `keywords[].keyword` | string | The keyword as extracted |
| `keywords[].normalized_keyword` | string | Lowercased, trimmed form used for deduplication |
| `next_page_token` | string | Token for the next page |
| `total_count` | integer | Total number of keywords matching the filters |

#### Example

```bash
curl -X GET \
  "https://api.example.com/api/v1/orgs/org-123/projects/proj-456/literature-reviews/a1b2c3d4-e5f6-7890-abcd-ef1234567890/keywords?extraction_round=0" \
  -H "Authorization: Bearer ${JWT_TOKEN}"
```

---

### Stream Review Progress (SSE)

```
GET /api/v1/orgs/{orgID}/projects/{projectID}/literature-reviews/{reviewID}/progress
```

Opens a Server-Sent Events (SSE) stream for real-time progress updates on a literature review. The stream polls the database at 2-second intervals and also receives Postgres NOTIFY events for immediate updates.

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `reviewID` | string (UUID) | The review identifier |

#### Response Headers

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

#### Event Format

Events follow the standard SSE format:

```
event: <event_type>
data: <json_payload>

```

#### Event Types

| Event Type | Description | When Sent |
|------------|-------------|-----------|
| `stream_started` | Stream successfully connected | Immediately on connection |
| `progress_update` | Periodic status and progress snapshot | Every 2 seconds while active |
| `keywords_extracted` | Keywords extracted from query or paper | After keyword extraction completes |
| `papers_found` | Papers discovered from a source search | After each source query returns results |
| `expansion_started` | A new expansion round has begun | At the start of each expansion depth |
| `ingestion_progress` | Ingestion queue status update | As papers are ingested |
| `error` | A recoverable or non-recoverable error occurred | On error during any phase |
| `completed` | Review reached a terminal state | When status becomes terminal |
| `timeout` | Stream maximum duration exceeded | After 4 hours |

#### Event Data Schema

**progress_update / stream_started / completed**

```json
{
  "event_type": "progress_update",
  "review_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "status": "searching",
  "progress": {
    "initial_keywords_count": 10,
    "total_keywords_processed": 10,
    "papers_found": 45,
    "papers_new": 45,
    "papers_ingested": 0,
    "papers_failed": 0,
    "max_expansion_depth": 2
  },
  "message": "status: searching",
  "timestamp": "2024-01-15T10:35:12Z"
}
```

**keywords_extracted** (via Postgres NOTIFY)

```json
{
  "event_type": "keywords_extracted",
  "review_id": "a1b2c3d4-...",
  "status": "extracting_keywords",
  "message": "extracted 10 keywords",
  "timestamp": "2024-01-15T10:30:05Z"
}
```

**papers_found** (via Postgres NOTIFY)

```json
{
  "event_type": "papers_found",
  "review_id": "a1b2c3d4-...",
  "status": "searching",
  "message": "found 15 papers from semantic_scholar for keyword 'drug discovery'",
  "timestamp": "2024-01-15T10:31:00Z"
}
```

**error** (via Postgres NOTIFY)

```json
{
  "event_type": "error",
  "review_id": "a1b2c3d4-...",
  "status": "searching",
  "message": "rate limited by semantic_scholar",
  "timestamp": "2024-01-15T10:31:30Z"
}
```

#### Stream Behavior

- If the review is already in a **terminal state** when the connection is made, a single `completed` event is sent and the stream closes.
- If the review has **no associated workflow**, an `error` event is sent and the stream closes.
- The stream has a **maximum duration of 4 hours**. After this, a `timeout` event is sent and the stream closes.
- The stream uses **Postgres LISTEN/NOTIFY** on channel `review_progress_{reviewID}` for immediate event delivery, with database polling as a fallback.
- Terminal event types (`completed`, `failed`, `cancelled`) cause the stream to close.

#### Example

```bash
curl -N \
  "https://api.example.com/api/v1/orgs/org-123/projects/proj-456/literature-reviews/a1b2c3d4-e5f6-7890-abcd-ef1234567890/progress" \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -H "Accept: text/event-stream"
```

---

### Health Check (Liveness)

```
GET /healthz
```

Returns the liveness status of the service. Does **not** require authentication.

#### Response (200 OK)

```json
{
  "database": "healthy",
  "status": "ok"
}
```

#### Response (503 Service Unavailable)

```json
{
  "database": "unhealthy",
  "error": "connection refused",
  "status": "unhealthy"
}
```

#### Example

```bash
curl -X GET "https://api.example.com/healthz"
```

---

### Readiness Check

```
GET /readyz
```

Returns the readiness status of the service, including database connectivity. Does **not** require authentication.

#### Response (200 OK)

```json
{
  "database": "healthy",
  "status": "ready"
}
```

#### Response (503 Service Unavailable)

```json
{
  "database": "unhealthy",
  "error": "connection refused",
  "status": "not_ready"
}
```

#### Example

```bash
curl -X GET "https://api.example.com/readyz"
```

---

## Error Responses

All errors are returned as JSON with a single `error` field:

```json
{
  "error": "human-readable error message"
}
```

### HTTP Status Code Reference

| Status Code | Meaning | Common Causes |
|-------------|---------|---------------|
| 400 Bad Request | Invalid input | Missing required fields, malformed UUID, invalid date format, query too short/long |
| 401 Unauthorized | Authentication failed | Missing or invalid JWT token |
| 403 Forbidden | Access denied | JWT does not grant access to the specified organization or project |
| 404 Not Found | Resource not found | Review ID does not exist, workflow not found |
| 409 Conflict | State conflict | Review already exists, review already in terminal state, workflow already started |
| 429 Too Many Requests | Rate limited | Too many requests to the service or upstream paper sources |
| 500 Internal Server Error | Server error | Unhandled internal error. Details are **never** leaked to clients; the message is always `"internal server error"`. |
| 503 Service Unavailable | Service down | Database unreachable, external dependency unavailable |

### Error Mapping from Domain Errors

| Domain Error | HTTP Status | Response Message |
|-------------|-------------|-----------------|
| `ErrNotFound` | 404 | `"resource not found"` |
| `ErrInvalidInput` | 400 | Original validation message |
| `ErrAlreadyExists` | 409 | `"resource already exists"` |
| `ErrUnauthorized` | 401 | `"unauthorized"` |
| `ErrForbidden` | 403 | `"forbidden"` |
| `ErrRateLimited` | 429 | `"rate limited"` |
| `ErrServiceUnavailable` | 503 | `"service unavailable"` |
| `ErrCancelled` | 409 | `"operation cancelled"` |
| `ErrWorkflowNotFound` | 404 | `"workflow not found"` |
| `ErrWorkflowAlreadyStarted` | 409 | `"workflow already started"` |
| Any other error | 500 | `"internal server error"` |

---

## Pagination

The API uses **cursor-based pagination** with base64-encoded page tokens.

### How It Works

1. Make a request with an optional `page_size` parameter (default: 50, max: 100).
2. The response includes a `next_page_token` field.
3. If `next_page_token` is a non-empty string, pass it as the `page_token` query parameter in the next request to retrieve the next page.
4. If `next_page_token` is an empty string, there are no more results.
5. The `total_count` field in the response gives the total number of matching records across all pages.

### Example: Paginating Through Reviews

```bash
# First page
curl -X GET \
  "https://api.example.com/api/v1/orgs/org-123/projects/proj-456/literature-reviews?page_size=10" \
  -H "Authorization: Bearer ${JWT_TOKEN}"

# Response includes: "next_page_token": "MTA="

# Second page
curl -X GET \
  "https://api.example.com/api/v1/orgs/org-123/projects/proj-456/literature-reviews?page_size=10&page_token=MTA=" \
  -H "Authorization: Bearer ${JWT_TOKEN}"
```

---

## Review Status Lifecycle

A literature review transitions through the following statuses:

```
pending -> extracting_keywords -> searching -> expanding -> ingesting -> completed
                                                                     -> partial
                                                         -> failed
                                             -> cancelled (at any non-terminal stage)
```

| Status | Description | Terminal |
|--------|-------------|----------|
| `pending` | Review created, workflow not yet started | No |
| `extracting_keywords` | LLM is extracting search keywords from the query | No |
| `searching` | Paper sources are being queried with extracted keywords | No |
| `expanding` | Recursive expansion round in progress (new keywords from papers) | No |
| `ingesting` | Discovered papers are being submitted to the ingestion service | No |
| `completed` | Review finished successfully; all papers processed | **Yes** |
| `partial` | Review finished but some operations failed or were skipped | **Yes** |
| `failed` | Review encountered an unrecoverable error | **Yes** |
| `cancelled` | Review was cancelled by a user or system action | **Yes** |

---

## Request Size Limits

| Limit | Value |
|-------|-------|
| Maximum request body size | 1 MB |
| Minimum query length | 3 characters |
| Maximum query length | 10,000 characters |
| Maximum page size | 100 |
| Default page size | 50 |
| SSE stream maximum duration | 4 hours |
| SSE poll interval | 2 seconds |
