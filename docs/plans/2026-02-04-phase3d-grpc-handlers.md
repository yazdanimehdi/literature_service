# Phase 3D: gRPC Handlers Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement all 7 gRPC handler methods for the `LiteratureReviewServiceServer` interface, with proto↔domain conversion, input validation, and proper gRPC error mapping.

**Architecture:** A `LiteratureReviewServer` struct embeds `UnimplementedLiteratureReviewServiceServer` and depends on `ReviewWorkflowClient` (for Start/Cancel RPCs), plus `ReviewRepository`, `PaperRepository`, and `KeywordRepository` (for read RPCs). Conversion helpers translate between domain models and proto messages. Domain errors are mapped to gRPC status codes centrally. The streaming RPC polls the Temporal workflow query at intervals.

**Tech Stack:** Go 1.25, gRPC (google.golang.org/grpc), protobuf, Temporal SDK, google/uuid

---

## Context for All Tasks

**Generated interface to implement** (`gen/proto/literaturereview/v1/literature_review_grpc.pb.go:145-161`):

```go
type LiteratureReviewServiceServer interface {
    StartLiteratureReview(context.Context, *StartLiteratureReviewRequest) (*StartLiteratureReviewResponse, error)
    GetLiteratureReviewStatus(context.Context, *GetLiteratureReviewStatusRequest) (*GetLiteratureReviewStatusResponse, error)
    CancelLiteratureReview(context.Context, *CancelLiteratureReviewRequest) (*CancelLiteratureReviewResponse, error)
    ListLiteratureReviews(context.Context, *ListLiteratureReviewsRequest) (*ListLiteratureReviewsResponse, error)
    GetLiteratureReviewPapers(context.Context, *GetLiteratureReviewPapersRequest) (*GetLiteratureReviewPapersResponse, error)
    GetLiteratureReviewKeywords(context.Context, *GetLiteratureReviewKeywordsRequest) (*GetLiteratureReviewKeywordsResponse, error)
    StreamLiteratureReviewProgress(*StreamLiteratureReviewProgressRequest, grpc.ServerStreamingServer[LiteratureReviewProgressEvent]) error
    mustEmbedUnimplementedLiteratureReviewServiceServer()
}
```

**Key dependencies:**
- `internal/temporal/client.go` — `ReviewWorkflowClient` with `StartReviewWorkflow`, `CancelWorkflow`, `SignalWorkflow`, `QueryWorkflow`, `DescribeWorkflow`
- `internal/temporal/workflows/review_workflow.go` — `ReviewWorkflowInput`, `ReviewWorkflowResult`, `LiteratureReviewWorkflow` func, signal/query constants
- `internal/repository/review_repository.go` — `ReviewRepository` with `Create`, `Get`, `List`, `GetByWorkflowID`
- `internal/repository/paper_repository.go` — `PaperRepository` with `List`
- `internal/repository/keyword_repository.go` — `KeywordRepository` with `List`
- `internal/domain/models.go` — `ReviewStatus`, `SourceType`, `IngestionStatus` enums
- `internal/domain/review.go` — `LiteratureReviewRequest`, `ReviewConfiguration`, `ReviewProgress`
- `internal/domain/paper.go` — `Paper`, `Author`
- `internal/domain/keyword.go` — `Keyword`
- `internal/domain/errors.go` — `ErrNotFound`, `ErrInvalidInput`, `ErrAlreadyExists`, `ValidationError`, etc.

**Proto package import:** `pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"`

---

### Task 1: Server Foundation + Conversion Helpers

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/convert.go`

**Step 1: Create `internal/server/server.go`**

This file defines:
1. `LiteratureReviewServer` struct embedding `pb.UnimplementedLiteratureReviewServiceServer` with dependencies
2. `NewLiteratureReviewServer` constructor
3. `domainErrToGRPC(err error) error` — maps domain errors to gRPC status codes:
   - `domain.ErrNotFound` / `*domain.NotFoundError` → `codes.NotFound`
   - `domain.ErrInvalidInput` / `*domain.ValidationError` → `codes.InvalidArgument`
   - `domain.ErrAlreadyExists` → `codes.AlreadyExists`
   - `domain.ErrUnauthorized` → `codes.Unauthenticated`
   - `domain.ErrForbidden` → `codes.PermissionDenied`
   - `domain.ErrRateLimited` → `codes.ResourceExhausted`
   - `domain.ErrServiceUnavailable` → `codes.Unavailable`
   - `domain.ErrCancelled` → `codes.Cancelled`
   - `temporal.ErrWorkflowNotFound` → `codes.NotFound`
   - `temporal.ErrWorkflowAlreadyStarted` → `codes.AlreadyExists`
   - default → `codes.Internal`
4. `validateOrgProject(orgID, projectID string) error` — common validation returning `codes.InvalidArgument`
5. `validateUUID(s, fieldName string) (uuid.UUID, error)` — parse UUID or return `codes.InvalidArgument`

```go
package server

import (
    "errors"

    "github.com/google/uuid"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"

    "github.com/helixir/literature-review-service/internal/domain"
    "github.com/helixir/literature-review-service/internal/repository"
    "github.com/helixir/literature-review-service/internal/temporal"
    "github.com/helixir/literature-review-service/internal/temporal/workflows"
    pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
)

type LiteratureReviewServer struct {
    pb.UnimplementedLiteratureReviewServiceServer

    workflowClient *temporal.ReviewWorkflowClient
    reviewRepo     repository.ReviewRepository
    paperRepo      repository.PaperRepository
    keywordRepo    repository.KeywordRepository
}

func NewLiteratureReviewServer(
    workflowClient *temporal.ReviewWorkflowClient,
    reviewRepo repository.ReviewRepository,
    paperRepo repository.PaperRepository,
    keywordRepo repository.KeywordRepository,
) *LiteratureReviewServer { ... }
```

**Step 2: Create `internal/server/convert.go`**

Conversion functions between domain and proto types. Each function is a standalone helper:

- `reviewStatusToProto(s domain.ReviewStatus) pb.ReviewStatus` — maps domain status string to proto enum
- `protoToReviewStatus(s pb.ReviewStatus) domain.ReviewStatus` — maps proto enum to domain status
- `ingestionStatusToProto(s domain.IngestionStatus) pb.IngestionStatus`
- `reviewToSummaryProto(r *domain.LiteratureReviewRequest) *pb.LiteratureReviewSummary`
- `reviewToStatusProto(r *domain.LiteratureReviewRequest) *pb.GetLiteratureReviewStatusResponse`
- `paperToProto(p *domain.Paper) *pb.Paper`
- `authorToProto(a domain.Author) *pb.Author`
- `reviewConfigToProto(c domain.ReviewConfiguration) *pb.ReviewConfiguration`
- `sourceTypeToProto(s domain.SourceType) string` — returns the string representation
- `timeToProtoTimestamp(t time.Time) *timestamppb.Timestamp`
- `timeToProtoDuration(d time.Duration) *durationpb.Duration`
- `optionalTimeToProtoTimestamp(t *time.Time) *timestamppb.Timestamp`

**Step 3: Verify compilation**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go build ./internal/server/...
```

**Step 4: Run existing tests to ensure no regressions**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test ./... -count=1 -timeout 120s
```

**Step 5: Commit**

```bash
git add internal/server/server.go internal/server/convert.go
git commit -m "feat(server): add gRPC server foundation and proto↔domain conversions"
```

---

### Task 2: Review Handlers (Start, GetStatus, Cancel, List) + Tests

**Depends on:** Task 1

**Files:**
- Create: `internal/server/review_handlers.go`
- Create: `internal/server/review_handlers_test.go`

**Step 1: Implement the four review-focused RPC handlers in `review_handlers.go`**

**StartLiteratureReview:**
1. Validate org_id, project_id, query (non-empty, max 10000 chars)
2. Create `uuid.New()` for request ID
3. Build `domain.ReviewConfiguration` from request (use defaults for nil optional fields from `domain.DefaultReviewConfiguration()`)
4. Create `domain.LiteratureReviewRequest` and persist via `reviewRepo.Create()`
5. Build `workflows.ReviewWorkflowInput` and start workflow via `workflowClient.StartReviewWorkflow()`
6. Update the review with workflow ID/run ID via `reviewRepo.Update()`
7. Return `StartLiteratureReviewResponse` with review_id, workflow_id, status=PENDING

**GetLiteratureReviewStatus:**
1. Validate org_id, project_id, review_id (valid UUID)
2. Fetch review via `reviewRepo.Get()`
3. Convert to `GetLiteratureReviewStatusResponse` using `reviewToStatusProto()`

**CancelLiteratureReview:**
1. Validate org_id, project_id, review_id
2. Fetch review via `reviewRepo.Get()`
3. Check review is not already terminal
4. Send cancel signal via `workflowClient.SignalWorkflow()` with `workflows.SignalCancel`
5. Return success response

**ListLiteratureReviews:**
1. Validate org_id, project_id
2. Build `repository.ReviewFilter` from request (page_size → limit, page_token → offset via base64 encoding)
3. Call `reviewRepo.List()`
4. Convert results to `LiteratureReviewSummary` protos
5. Build next_page_token if more results exist

**Step 2: Write tests in `review_handlers_test.go`**

Create mock implementations of all dependencies (ReviewRepository, PaperRepository, KeywordRepository, ReviewWorkflowClient wrapper). Tests:

- `TestStartLiteratureReview_Success` — happy path, verify workflow started and review created
- `TestStartLiteratureReview_ValidationError` — empty query, empty org_id
- `TestGetLiteratureReviewStatus_Success` — review found, response correctly populated
- `TestGetLiteratureReviewStatus_NotFound` — review not found → codes.NotFound
- `TestCancelLiteratureReview_Success` — running review cancelled
- `TestCancelLiteratureReview_AlreadyCompleted` — terminal review → codes.FailedPrecondition
- `TestListLiteratureReviews_Success` — returns reviews with pagination
- `TestListLiteratureReviews_Empty` — empty results

**Step 3: Run tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test ./internal/server/... -v -count=1 -race -timeout 120s
```

**Step 4: Commit**

```bash
git add internal/server/review_handlers.go internal/server/review_handlers_test.go
git commit -m "feat(server): implement Start, GetStatus, Cancel, List review gRPC handlers"
```

---

### Task 3: Paper & Keyword Handlers + Tests

**Depends on:** Task 1

**Files:**
- Create: `internal/server/paper_handlers.go`
- Create: `internal/server/paper_handlers_test.go`

**Step 1: Implement GetLiteratureReviewPapers and GetLiteratureReviewKeywords in `paper_handlers.go`**

**GetLiteratureReviewPapers:**
1. Validate org_id, project_id, review_id
2. Verify the review exists via `reviewRepo.Get()` (tenant isolation)
3. Build `repository.PaperFilter` from request:
   - page_size → limit (default 50, max 100)
   - page_token → offset (base64-encoded integer)
   - source_filter → filter by source
   - ingestion_status_filter → not directly on PaperFilter (may need to handle at application level or extend filter — for now, list all and note as limitation)
4. Call `paperRepo.List()`
5. Convert papers to proto messages
6. Build next_page_token

**GetLiteratureReviewKeywords:**
1. Validate org_id, project_id, review_id
2. Verify review exists
3. Build `repository.KeywordFilter` from request:
   - page_size → limit
   - page_token → offset
4. Call `keywordRepo.List()`
5. Convert keywords to `ReviewKeyword` proto messages
6. Build next_page_token

**Step 2: Write tests in `paper_handlers_test.go`**

- `TestGetLiteratureReviewPapers_Success` — papers returned with pagination
- `TestGetLiteratureReviewPapers_ReviewNotFound` — review doesn't exist → codes.NotFound
- `TestGetLiteratureReviewKeywords_Success` — keywords returned
- `TestGetLiteratureReviewKeywords_Empty` — no keywords found

**Step 3: Run tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test ./internal/server/... -v -count=1 -race -timeout 120s
```

**Step 4: Commit**

```bash
git add internal/server/paper_handlers.go internal/server/paper_handlers_test.go
git commit -m "feat(server): implement GetPapers and GetKeywords gRPC handlers"
```

---

### Task 4: StreamLiteratureReviewProgress Handler + Test

**Depends on:** Task 1

**Files:**
- Create: `internal/server/stream_handler.go`
- Create: `internal/server/stream_handler_test.go`

**Step 1: Implement StreamLiteratureReviewProgress in `stream_handler.go`**

The streaming RPC polls the Temporal workflow for progress at regular intervals:

1. Validate org_id, project_id, review_id
2. Fetch review to get workflow ID via `reviewRepo.Get()`
3. If review is already terminal, send one final event with the terminal status and return
4. Poll loop (every 2 seconds):
   a. Check stream context for cancellation
   b. Query workflow progress via `workflowClient.QueryWorkflow()` with `workflows.QueryProgress`
   c. Build `LiteratureReviewProgressEvent` from the query result
   d. Send event via `stream.Send()`
   e. If status is terminal, send final event and return
   f. Sleep 2 seconds (using `time.NewTimer` + select for cancellation)

Constants:
- `streamPollInterval = 2 * time.Second`
- `streamMaxDuration = 4 * time.Hour` (safety timeout matching workflow timeout)

**Step 2: Write tests in `stream_handler_test.go`**

- `TestStreamProgress_TerminalReview` — review already completed, sends single event
- `TestStreamProgress_ReviewNotFound` — review not found → codes.NotFound
- `TestStreamProgress_ValidationError` — empty fields → codes.InvalidArgument

Note: Full streaming tests with mock poll loops are complex. The core validation and terminal-status path are the most testable parts. The poll loop relies on the Temporal client which is harder to mock in unit tests.

**Step 3: Run tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test ./internal/server/... -v -count=1 -race -timeout 120s
```

**Step 4: Commit**

```bash
git add internal/server/stream_handler.go internal/server/stream_handler_test.go
git commit -m "feat(server): implement StreamLiteratureReviewProgress gRPC handler"
```

---

### Task 5: Full Test Suite Verification

**Depends on:** Tasks 2, 3, 4

**Step 1: Run all tests with race detector**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test ./... -v -count=1 -race -timeout 120s
```

**Step 2: Run go vet**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go vet ./...
```

**Step 3: Verify build**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go build ./...
```

**Step 4: Verify all server methods compile against the interface**

The embedding of `pb.UnimplementedLiteratureReviewServiceServer` ensures forward compatibility. The compile-time check in the generated code verifies the struct satisfies the interface when registered.

---

## Notes

- **Page tokens:** Use base64-encoded integer offsets for simplicity. The token encodes the next offset. When `offset + limit < total_count`, generate a next_page_token.
- **Multi-tenancy:** All read operations verify the review belongs to the requesting org/project before proceeding. The repositories enforce tenant isolation via orgID/projectID parameters.
- **Error mapping:** All handler errors go through `domainErrToGRPC()` for consistent gRPC status codes.
- **Streaming safety:** The poll loop respects context cancellation and has a maximum duration to prevent resource leaks.
