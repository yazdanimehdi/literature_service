# Resumable Jobs Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make literature review jobs resumable after pause, stop, or budget exhaustion.

**Architecture:** Signal-based pause/resume using Temporal's native workflow signals. Hybrid resume strategy: auto-resume for budget refill, manual resume for user-initiated pause. Three job controls: Pause (temporary), Stop (graceful with partial results), Cancel (immediate abort).

**Tech Stack:** Go, Temporal, gRPC, PostgreSQL, Kafka

---

## Design Overview

### Job Control Semantics

| Action | Behavior | Can Resume? | Final Status |
|--------|----------|-------------|--------------|
| **Pause** | Suspend at next checkpoint, keep state | Yes | `paused` |
| **Stop** | Graceful stop, save partial results | No | `partial` |
| **Cancel** | Abort immediately, discard in-flight work | No | `cancelled` |

### State Flow

```
Running ──► Pause ──► Paused ──► Resume ──► Running
   │                    │
   │                    └──► Stop ──► Partial (saved results)
   │
   ├──► Stop ──► Partial (saved results)
   │
   └──► Cancel ──► Cancelled (immediate abort)
```

### Auto-Resume Flow (Budget Refill)

```
┌──────────────┐    1. User adds credits    ┌──────────────┐
│    User      │ ─────────────────────────► │ core_service │
└──────────────┘                            └──────┬───────┘
                                                   │
                    2. Publish event               │
                    "budget.refilled"              │
                                                   ▼
                                            ┌──────────────┐
                                            │    Kafka     │
                                            └──────┬───────┘
                                                   │
                    3. Consume event               │
                                                   ▼
┌───────────────────┐    4. Query paused    ┌──────────────────┐
│ Temporal Workflow │ ◄──── workflows ───── │ literature_svc   │
│  (waiting)        │    5. Send resume     │ (budget listener)│
└───────────────────┘       signal          └──────────────────┘
```

### Pause Checkpoints

Workflow pauses at these safe points:
- After each keyword extraction
- After each search batch completes
- After each child workflow batch spawns
- Before starting each expansion round

---

## Phase 1: Domain & Database

### Task 1.1: Add ReviewStatusPaused to domain status enum

**Files:**
- Modify: `internal/domain/models.go`

**Step 1: Write the test**

```go
// internal/domain/models_test.go
func TestReviewStatus_IsPaused(t *testing.T) {
    assert.False(t, ReviewStatusPending.IsTerminal())
    assert.False(t, ReviewStatusPaused.IsTerminal())
    assert.True(t, ReviewStatusCompleted.IsTerminal())
}
```

**Step 2: Add the status constant**

```go
// internal/domain/models.go

// ReviewStatusPaused indicates the review has been paused by user or budget exhaustion.
ReviewStatusPaused ReviewStatus = "paused"
```

**Step 3: Run tests**

```bash
go test ./internal/domain/... -v -run TestReviewStatus
```

**Step 4: Commit**

```bash
git add internal/domain/models.go internal/domain/models_test.go
git commit -m "feat(domain): add ReviewStatusPaused status"
```

---

### Task 1.2: Add PauseReason type

**Files:**
- Modify: `internal/domain/models.go`
- Modify: `internal/domain/models_test.go`

**Step 1: Write the test**

```go
// internal/domain/models_test.go
func TestPauseReason_String(t *testing.T) {
    tests := []struct {
        reason   PauseReason
        expected string
    }{
        {PauseReasonUser, "user"},
        {PauseReasonBudgetExhausted, "budget_exhausted"},
    }
    for _, tt := range tests {
        assert.Equal(t, tt.expected, string(tt.reason))
    }
}
```

**Step 2: Add PauseReason type**

```go
// internal/domain/models.go

// PauseReason indicates why a workflow was paused.
type PauseReason string

const (
    // PauseReasonUser indicates the user manually paused the review.
    PauseReasonUser PauseReason = "user"

    // PauseReasonBudgetExhausted indicates the review paused due to budget exhaustion.
    PauseReasonBudgetExhausted PauseReason = "budget_exhausted"
)
```

**Step 3: Run tests**

```bash
go test ./internal/domain/... -v -run TestPauseReason
```

**Step 4: Commit**

```bash
git add internal/domain/models.go internal/domain/models_test.go
git commit -m "feat(domain): add PauseReason type with user and budget_exhausted values"
```

---

### Task 1.3: Database migration for pause fields

**Files:**
- Create: `migrations/XXXXXX_add_pause_fields.up.sql`
- Create: `migrations/XXXXXX_add_pause_fields.down.sql`

**Step 1: Create up migration**

```sql
-- migrations/000008_add_pause_fields.up.sql

-- Add pause tracking fields to review_requests
ALTER TABLE review_requests
ADD COLUMN pause_reason TEXT,
ADD COLUMN paused_at TIMESTAMPTZ,
ADD COLUMN paused_at_phase TEXT;

-- Add index for querying paused reviews by reason
CREATE INDEX idx_review_requests_pause_reason
ON review_requests (org_id, status, pause_reason)
WHERE status = 'paused';

COMMENT ON COLUMN review_requests.pause_reason IS 'Why the review was paused: user or budget_exhausted';
COMMENT ON COLUMN review_requests.paused_at IS 'When the review was paused';
COMMENT ON COLUMN review_requests.paused_at_phase IS 'Which workflow phase the review was in when paused';
```

**Step 2: Create down migration**

```sql
-- migrations/000008_add_pause_fields.down.sql

DROP INDEX IF EXISTS idx_review_requests_pause_reason;

ALTER TABLE review_requests
DROP COLUMN IF EXISTS pause_reason,
DROP COLUMN IF EXISTS paused_at,
DROP COLUMN IF EXISTS paused_at_phase;
```

**Step 3: Run migration**

```bash
make migrate-up
```

**Step 4: Verify migration**

```bash
psql -h localhost -U litreview -d literature_review -c "\d review_requests"
```

**Step 5: Commit**

```bash
git add migrations/
git commit -m "feat(db): add pause_reason, paused_at, paused_at_phase columns to review_requests"
```

---

### Task 1.4: Update ReviewRequest domain model

**Files:**
- Modify: `internal/domain/review.go`
- Modify: `internal/domain/review_test.go`

**Step 1: Write the test**

```go
// internal/domain/review_test.go
func TestReviewRequest_PauseFields(t *testing.T) {
    now := time.Now()
    req := &ReviewRequest{
        ID:            uuid.New(),
        Status:        ReviewStatusPaused,
        PauseReason:   PauseReasonBudgetExhausted,
        PausedAt:      &now,
        PausedAtPhase: "searching",
    }

    assert.Equal(t, ReviewStatusPaused, req.Status)
    assert.Equal(t, PauseReasonBudgetExhausted, req.PauseReason)
    assert.NotNil(t, req.PausedAt)
    assert.Equal(t, "searching", req.PausedAtPhase)
}
```

**Step 2: Add fields to ReviewRequest**

```go
// internal/domain/review.go

type ReviewRequest struct {
    // ... existing fields ...

    // Pause state
    PauseReason   PauseReason `json:"pause_reason,omitempty"`
    PausedAt      *time.Time  `json:"paused_at,omitempty"`
    PausedAtPhase string      `json:"paused_at_phase,omitempty"`
}
```

**Step 3: Run tests**

```bash
go test ./internal/domain/... -v -run TestReviewRequest_PauseFields
```

**Step 4: Commit**

```bash
git add internal/domain/review.go internal/domain/review_test.go
git commit -m "feat(domain): add pause fields to ReviewRequest model"
```

---

### Task 1.5: Add repository method FindPausedByReason

**Files:**
- Modify: `internal/repository/review_repository.go`
- Modify: `internal/repository/review_repository_test.go`

**Step 1: Write the test**

```go
// internal/repository/review_repository_test.go
func TestReviewRepository_FindPausedByReason(t *testing.T) {
    // Setup mock
    mock, err := pgxmock.NewPool()
    require.NoError(t, err)
    defer mock.Close()

    repo := NewReviewRepository(mock)
    ctx := context.Background()

    t.Run("finds budget-paused reviews", func(t *testing.T) {
        rows := pgxmock.NewRows([]string{
            "id", "workflow_id", "org_id", "project_id", "status",
            "pause_reason", "paused_at", "paused_at_phase",
        }).AddRow(
            uuid.New(), "wf-123", "org-1", "proj-1", "paused",
            "budget_exhausted", time.Now(), "searching",
        )

        mock.ExpectQuery("SELECT .* FROM review_requests").
            WithArgs("org-1", "", "budget_exhausted").
            WillReturnRows(rows)

        reviews, err := repo.FindPausedByReason(ctx, "org-1", "", domain.PauseReasonBudgetExhausted)
        require.NoError(t, err)
        assert.Len(t, reviews, 1)
    })
}
```

**Step 2: Implement FindPausedByReason**

```go
// internal/repository/review_repository.go

// FindPausedByReason returns all paused reviews matching the given org, project, and reason.
// If projectID is empty, returns paused reviews for all projects in the org.
func (r *ReviewRepository) FindPausedByReason(
    ctx context.Context,
    orgID, projectID string,
    reason domain.PauseReason,
) ([]*domain.ReviewRequest, error) {
    query := `
        SELECT id, workflow_id, org_id, project_id, query, status,
               pause_reason, paused_at, paused_at_phase, created_at, updated_at
        FROM review_requests
        WHERE org_id = $1
          AND ($2 = '' OR project_id = $2)
          AND status = 'paused'
          AND pause_reason = $3
        ORDER BY paused_at ASC
    `

    rows, err := r.pool.Query(ctx, query, orgID, projectID, string(reason))
    if err != nil {
        return nil, fmt.Errorf("query paused reviews: %w", err)
    }
    defer rows.Close()

    var reviews []*domain.ReviewRequest
    for rows.Next() {
        var review domain.ReviewRequest
        err := rows.Scan(
            &review.ID, &review.WorkflowID, &review.OrgID, &review.ProjectID,
            &review.Query, &review.Status, &review.PauseReason, &review.PausedAt,
            &review.PausedAtPhase, &review.CreatedAt, &review.UpdatedAt,
        )
        if err != nil {
            return nil, fmt.Errorf("scan paused review: %w", err)
        }
        reviews = append(reviews, &review)
    }

    return reviews, nil
}
```

**Step 3: Run tests**

```bash
go test ./internal/repository/... -v -run TestReviewRepository_FindPausedByReason
```

**Step 4: Commit**

```bash
git add internal/repository/review_repository.go internal/repository/review_repository_test.go
git commit -m "feat(repository): add FindPausedByReason method"
```

---

## Phase 2: Temporal Signals & Workflow

### Task 2.1: Define signal constants

**Files:**
- Modify: `internal/temporal/signals.go`

**Step 1: Add signal constants**

```go
// internal/temporal/signals.go

const (
    // SignalCancel requests immediate workflow cancellation.
    SignalCancel = "cancel"

    // SignalPause requests workflow pause at next checkpoint.
    SignalPause = "pause"

    // SignalResume requests workflow resume from paused state.
    SignalResume = "resume"

    // SignalStop requests graceful workflow stop with partial results.
    SignalStop = "stop"

    // SignalBatchComplete signals batch processing completion.
    SignalBatchComplete = "batch_complete"
)
```

**Step 2: Commit**

```bash
git add internal/temporal/signals.go
git commit -m "feat(temporal): add SignalPause, SignalResume, SignalStop constants"
```

---

### Task 2.2: Define signal structs

**Files:**
- Create: `internal/temporal/workflows/signals.go`
- Create: `internal/temporal/workflows/signals_test.go`

**Step 1: Write the test**

```go
// internal/temporal/workflows/signals_test.go
package workflows

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/helixir/literature-review-service/internal/domain"
)

func TestPauseSignal(t *testing.T) {
    signal := PauseSignal{
        Reason:  domain.PauseReasonUser,
        Message: "User requested pause",
    }
    assert.Equal(t, domain.PauseReasonUser, signal.Reason)
}

func TestResumeSignal(t *testing.T) {
    signal := ResumeSignal{ResumedBy: "user"}
    assert.Equal(t, "user", signal.ResumedBy)
}

func TestStopSignal(t *testing.T) {
    signal := StopSignal{Reason: "user_requested"}
    assert.Equal(t, "user_requested", signal.Reason)
}
```

**Step 2: Create signal structs**

```go
// internal/temporal/workflows/signals.go
package workflows

import "github.com/helixir/literature-review-service/internal/domain"

// PauseSignal carries the reason for pausing a workflow.
type PauseSignal struct {
    // Reason indicates why the workflow is being paused.
    Reason domain.PauseReason
    // Message provides optional context about the pause.
    Message string
}

// ResumeSignal carries information about workflow resumption.
type ResumeSignal struct {
    // ResumedBy indicates who/what triggered the resume (e.g., "user", "budget_refill").
    ResumedBy string
}

// StopSignal requests graceful workflow termination with partial results.
type StopSignal struct {
    // Reason provides optional context about why the stop was requested.
    Reason string
}
```

**Step 3: Run tests**

```bash
go test ./internal/temporal/workflows/... -v -run TestPauseSignal
go test ./internal/temporal/workflows/... -v -run TestResumeSignal
go test ./internal/temporal/workflows/... -v -run TestStopSignal
```

**Step 4: Commit**

```bash
git add internal/temporal/workflows/signals.go internal/temporal/workflows/signals_test.go
git commit -m "feat(temporal): add PauseSignal, ResumeSignal, StopSignal structs"
```

---

### Task 2.3: Add signal channels and handlers to workflow

**Files:**
- Modify: `internal/temporal/workflows/review_workflow.go`

**Step 1: Add pause state to workflowProgress**

```go
// workflowProgress tracks the internal progress state of the workflow.
type workflowProgress struct {
    // ... existing fields ...

    // Pause state
    IsPaused      bool
    PauseReason   domain.PauseReason
    PausedAt      time.Time
    PausedAtPhase string
}
```

**Step 2: Add signal channels in workflow function**

```go
// Set up pause signal handling
pauseCh := workflow.GetSignalChannel(ctx, SignalPause)
resumeCh := workflow.GetSignalChannel(ctx, SignalResume)
stopCh := workflow.GetSignalChannel(ctx, SignalStop)

var stopRequested bool

// Pause signal handler
workflow.Go(ctx, func(gCtx workflow.Context) {
    for {
        var signal PauseSignal
        if !pauseCh.Receive(gCtx, &signal) {
            return
        }
        progress.IsPaused = true
        progress.PauseReason = signal.Reason
        progress.PausedAt = workflow.Now(gCtx)
        progress.PausedAtPhase = progress.Phase
        logger.Info("workflow paused", "reason", signal.Reason, "phase", progress.Phase)
    }
})

// Stop signal handler
workflow.Go(ctx, func(gCtx workflow.Context) {
    stopCh.Receive(gCtx, nil)
    stopRequested = true
    logger.Info("stop requested, will complete current phase")
})
```

**Step 3: Commit**

```bash
git add internal/temporal/workflows/review_workflow.go
git commit -m "feat(temporal): add pause/resume/stop signal handlers to workflow"
```

---

### Task 2.4: Add checkPausePoint helper

**Files:**
- Modify: `internal/temporal/workflows/review_workflow.go`

**Step 1: Implement checkPausePoint**

```go
// checkPausePoint checks if the workflow is paused and waits for resume if so.
// Returns an error if the context is cancelled while waiting.
func checkPausePoint(
    ctx workflow.Context,
    cancelCtx workflow.Context,
    progress *workflowProgress,
    resumeCh workflow.ReceiveChannel,
    statusAct *activities.StatusActivities,
    input ReviewWorkflowInput,
    logger log.Logger,
) error {
    if !progress.IsPaused {
        return nil
    }

    logger.Info("entering pause state", "reason", progress.PauseReason, "phase", progress.PausedAtPhase)

    // Update status to paused in database
    statusCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
        StartToCloseTimeout: statusActivityTimeout,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    500 * time.Millisecond,
            BackoffCoefficient: 2.0,
            MaximumInterval:    10 * time.Second,
            MaximumAttempts:    5,
        },
    })

    err := workflow.ExecuteActivity(statusCtx, statusAct.UpdatePauseState, activities.UpdatePauseStateInput{
        OrgID:         input.OrgID,
        ProjectID:     input.ProjectID,
        RequestID:     input.RequestID,
        Status:        domain.ReviewStatusPaused,
        PauseReason:   progress.PauseReason,
        PausedAtPhase: progress.PausedAtPhase,
    }).Get(ctx, nil)
    if err != nil {
        logger.Error("failed to update pause state", "error", err)
        // Continue anyway - don't fail the workflow for status update failure
    }

    // Wait for resume signal
    var resumeSignal ResumeSignal
    resumeCh.Receive(ctx, &resumeSignal)

    logger.Info("workflow resumed", "by", resumeSignal.ResumedBy)

    // Clear pause state
    progress.IsPaused = false
    progress.PauseReason = ""

    // Update status back to previous phase
    previousStatus := phaseToStatus(progress.Phase)
    err = workflow.ExecuteActivity(statusCtx, statusAct.UpdateStatus, activities.UpdateStatusInput{
        OrgID:     input.OrgID,
        ProjectID: input.ProjectID,
        RequestID: input.RequestID,
        Status:    previousStatus,
        ErrorMsg:  "",
    }).Get(ctx, nil)
    if err != nil {
        logger.Error("failed to update status after resume", "error", err)
    }

    return nil
}

// phaseToStatus maps workflow phase to ReviewStatus.
func phaseToStatus(phase string) domain.ReviewStatus {
    switch phase {
    case "extracting_keywords":
        return domain.ReviewStatusExtractingKeywords
    case "searching":
        return domain.ReviewStatusSearching
    case "batching", "processing":
        return domain.ReviewStatusIngesting
    case "expanding":
        return domain.ReviewStatusExpanding
    default:
        return domain.ReviewStatusPending
    }
}
```

**Step 2: Commit**

```bash
git add internal/temporal/workflows/review_workflow.go
git commit -m "feat(temporal): add checkPausePoint helper for pause/resume handling"
```

---

### Task 2.5: Add shouldStop check and graceful stop logic

**Files:**
- Modify: `internal/temporal/workflows/review_workflow.go`

**Step 1: Add checkStopPoint helper**

```go
// checkStopPoint checks if stop was requested and returns partial results if so.
// Returns (shouldReturn, result, error).
func checkStopPoint(
    ctx workflow.Context,
    stopRequested bool,
    progress *workflowProgress,
    input ReviewWorkflowInput,
    startTime time.Time,
    totalKeywords, totalPapersFound, expansionRounds int,
    statusAct *activities.StatusActivities,
    eventAct *activities.EventActivities,
    logger log.Logger,
) (bool, *ReviewWorkflowResult, error) {
    if !stopRequested {
        return false, nil, nil
    }

    logger.Info("stopping workflow gracefully", "phase", progress.Phase)

    // Update status to partial
    statusCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
        StartToCloseTimeout: statusActivityTimeout,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    500 * time.Millisecond,
            BackoffCoefficient: 2.0,
            MaximumInterval:    10 * time.Second,
            MaximumAttempts:    5,
        },
    })

    _ = workflow.ExecuteActivity(statusCtx, statusAct.UpdateStatus, activities.UpdateStatusInput{
        OrgID:     input.OrgID,
        ProjectID: input.ProjectID,
        RequestID: input.RequestID,
        Status:    domain.ReviewStatusPartial,
        ErrorMsg:  "stopped by user",
    }).Get(ctx, nil)

    duration := workflow.Now(ctx).Sub(startTime).Seconds()

    result := &ReviewWorkflowResult{
        RequestID:       input.RequestID,
        Status:          string(domain.ReviewStatusPartial),
        KeywordsFound:   totalKeywords,
        PapersFound:     totalPapersFound,
        PapersIngested:  progress.PapersIngested,
        DuplicatesFound: progress.DuplicatesFound,
        PapersFailed:    progress.PapersFailed,
        ExpansionRounds: expansionRounds,
        Duration:        duration,
    }

    // Publish stopped event
    _ = workflow.ExecuteActivity(statusCtx, eventAct.PublishEvent, activities.PublishEventInput{
        EventType: "review.stopped",
        RequestID: input.RequestID,
        OrgID:     input.OrgID,
        ProjectID: input.ProjectID,
        Payload: map[string]interface{}{
            "stopped_at_phase": progress.Phase,
            "papers_ingested":  progress.PapersIngested,
        },
    }).Get(ctx, nil)

    return true, result, nil
}
```

**Step 2: Commit**

```bash
git add internal/temporal/workflows/review_workflow.go
git commit -m "feat(temporal): add checkStopPoint helper for graceful stop with partial results"
```

---

### Task 2.6: Add isBudgetExhausted helper

**Files:**
- Modify: `internal/temporal/workflows/review_workflow.go`

**Step 1: Add budget check helper**

```go
import sharedllm "github.com/helixir/llm"

// isBudgetExhausted checks if an error is due to budget exhaustion.
func isBudgetExhausted(err error) bool {
    return sharedllm.ErrorKindOf(err) == sharedllm.ErrBudgetExceeded
}
```

**Step 2: Commit**

```bash
git add internal/temporal/workflows/review_workflow.go
git commit -m "feat(temporal): add isBudgetExhausted helper"
```

---

### Task 2.7: Add executeWithBudgetPause wrapper

**Files:**
- Modify: `internal/temporal/workflows/review_workflow.go`

**Step 1: Implement executeWithBudgetPause**

```go
// executeWithBudgetPause executes an activity and handles budget exhaustion by pausing.
// If budget is exhausted, it sets pause state and waits for resume before retrying.
func executeWithBudgetPause[T any](
    ctx workflow.Context,
    progress *workflowProgress,
    resumeCh workflow.ReceiveChannel,
    statusAct *activities.StatusActivities,
    input ReviewWorkflowInput,
    logger log.Logger,
    activity interface{},
    activityInput interface{},
) (T, error) {
    var result T

    for {
        future := workflow.ExecuteActivity(ctx, activity, activityInput)
        err := future.Get(ctx, &result)

        if err == nil {
            return result, nil
        }

        if !isBudgetExhausted(err) {
            return result, err // Non-budget error, propagate
        }

        // Budget exhausted - pause and wait
        logger.Warn("budget exhausted, pausing workflow", "phase", progress.Phase)

        progress.IsPaused = true
        progress.PauseReason = domain.PauseReasonBudgetExhausted
        progress.PausedAt = workflow.Now(ctx)
        progress.PausedAtPhase = progress.Phase

        if err := checkPausePoint(ctx, ctx, progress, resumeCh, statusAct, input, logger); err != nil {
            return result, err
        }

        // Retry after resume
        logger.Info("retrying activity after budget refill")
    }
}
```

**Step 2: Commit**

```bash
git add internal/temporal/workflows/review_workflow.go
git commit -m "feat(temporal): add executeWithBudgetPause wrapper for auto-pause on budget exhaustion"
```

---

### Task 2.8: Insert pause checkpoints in workflow

**Files:**
- Modify: `internal/temporal/workflows/review_workflow.go`

**Step 1: Add checkpoint after keyword extraction**

After initial keyword extraction:
```go
// Check for pause/stop after keyword extraction
if err := checkPausePoint(ctx, cancelCtx, progress, resumeCh, statusAct, input, logger); err != nil {
    return handleFailure(err)
}
if shouldReturn, result, err := checkStopPoint(ctx, stopRequested, progress, input, startTime, totalKeywords, totalPapersFound, expansionRounds, statusAct, eventAct, logger); shouldReturn {
    return result, err
}
```

**Step 2: Add checkpoint after searches complete**

After `logger.Info("all searches completed", ...)`:
```go
// Check for pause/stop after searches
if err := checkPausePoint(ctx, cancelCtx, progress, resumeCh, statusAct, input, logger); err != nil {
    return handleFailure(err)
}
if shouldReturn, result, err := checkStopPoint(ctx, stopRequested, progress, input, startTime, totalKeywords, totalPapersFound, expansionRounds, statusAct, eventAct, logger); shouldReturn {
    return result, err
}
```

**Step 3: Add checkpoint after batch spawn**

After spawning batch workflows:
```go
// Check for pause/stop after batch spawn
if err := checkPausePoint(ctx, cancelCtx, progress, resumeCh, statusAct, input, logger); err != nil {
    return handleFailure(err)
}
if shouldReturn, result, err := checkStopPoint(ctx, stopRequested, progress, input, startTime, totalKeywords, totalPapersFound, expansionRounds, statusAct, eventAct, logger); shouldReturn {
    return result, err
}
```

**Step 4: Add checkpoint before each expansion round**

At start of expansion loop:
```go
for round := 1; round <= input.Config.MaxExpansionDepth; round++ {
    // Check for pause/stop before expansion round
    if err := checkPausePoint(ctx, cancelCtx, progress, resumeCh, statusAct, input, logger); err != nil {
        return handleFailure(err)
    }
    if shouldReturn, result, err := checkStopPoint(ctx, stopRequested, progress, input, startTime, totalKeywords, totalPapersFound, expansionRounds, statusAct, eventAct, logger); shouldReturn {
        return result, err
    }

    // ... rest of expansion loop
}
```

**Step 5: Run tests**

```bash
go test ./internal/temporal/workflows/... -v
```

**Step 6: Commit**

```bash
git add internal/temporal/workflows/review_workflow.go
git commit -m "feat(temporal): insert pause/stop checkpoints throughout workflow"
```

---

### Task 2.9: Update workflowProgress struct

**Files:**
- Modify: `internal/temporal/workflows/review_workflow.go`

Already done in Task 2.3. Verify the struct includes:

```go
type workflowProgress struct {
    Status            string
    Phase             string
    KeywordsFound     int
    PapersFound       int
    PapersIngested    int
    PapersFailed      int
    DuplicatesFound   int
    BatchesSpawned    int
    BatchesCompleted  int
    ExpansionRound    int
    MaxExpansionDepth int

    // Pause state
    IsPaused      bool
    PauseReason   domain.PauseReason
    PausedAt      time.Time
    PausedAtPhase string
}
```

---

### Task 2.10: Add UpdatePauseState activity

**Files:**
- Modify: `internal/temporal/activities/status_activities.go`
- Modify: `internal/temporal/activities/types.go`

**Step 1: Add input type**

```go
// internal/temporal/activities/types.go

// UpdatePauseStateInput contains the data needed to update a review's pause state.
type UpdatePauseStateInput struct {
    OrgID         string
    ProjectID     string
    RequestID     uuid.UUID
    Status        domain.ReviewStatus
    PauseReason   domain.PauseReason
    PausedAtPhase string
}
```

**Step 2: Implement activity**

```go
// internal/temporal/activities/status_activities.go

// UpdatePauseState updates the pause state of a review request.
func (a *StatusActivities) UpdatePauseState(ctx context.Context, input UpdatePauseStateInput) error {
    logger := activity.GetLogger(ctx)
    logger.Info("updating pause state",
        "requestID", input.RequestID,
        "status", input.Status,
        "pauseReason", input.PauseReason,
        "phase", input.PausedAtPhase,
    )

    return a.repo.UpdatePauseState(ctx, input.OrgID, input.ProjectID, input.RequestID,
        input.Status, input.PauseReason, input.PausedAtPhase)
}
```

**Step 3: Add repository method**

```go
// internal/repository/review_repository.go

// UpdatePauseState updates the pause-related fields of a review request.
func (r *ReviewRepository) UpdatePauseState(
    ctx context.Context,
    orgID, projectID string,
    requestID uuid.UUID,
    status domain.ReviewStatus,
    pauseReason domain.PauseReason,
    pausedAtPhase string,
) error {
    query := `
        UPDATE review_requests
        SET status = $4,
            pause_reason = $5,
            paused_at = NOW(),
            paused_at_phase = $6,
            updated_at = NOW()
        WHERE org_id = $1 AND project_id = $2 AND id = $3
    `

    _, err := r.pool.Exec(ctx, query, orgID, projectID, requestID,
        string(status), string(pauseReason), pausedAtPhase)
    if err != nil {
        return fmt.Errorf("update pause state: %w", err)
    }

    return nil
}
```

**Step 4: Commit**

```bash
git add internal/temporal/activities/status_activities.go internal/temporal/activities/types.go internal/repository/review_repository.go
git commit -m "feat(temporal): add UpdatePauseState activity"
```

---

## Phase 3: gRPC API

### Task 3.1: Add Pause/Resume/Stop RPCs to proto

**Files:**
- Modify: `api/proto/literature/v1/literature.proto`

**Step 1: Add RPC definitions**

```protobuf
service LiteratureReviewService {
    // ... existing RPCs ...

    // PauseReview pauses a running review at the next checkpoint.
    rpc PauseReview(PauseReviewRequest) returns (PauseReviewResponse);

    // ResumeReview resumes a paused review.
    rpc ResumeReview(ResumeReviewRequest) returns (ResumeReviewResponse);

    // StopReview gracefully stops a review and saves partial results.
    rpc StopReview(StopReviewRequest) returns (StopReviewResponse);
}

message PauseReviewRequest {
    string org_id = 1;
    string project_id = 2;
    string request_id = 3;
}

message PauseReviewResponse {
    bool success = 1;
    string message = 2;
}

message ResumeReviewRequest {
    string org_id = 1;
    string project_id = 2;
    string request_id = 3;
}

message ResumeReviewResponse {
    bool success = 1;
    string message = 2;
}

message StopReviewRequest {
    string org_id = 1;
    string project_id = 2;
    string request_id = 3;
}

message StopReviewResponse {
    bool success = 1;
    string message = 2;
    ReviewProgress final_progress = 3;
}
```

**Step 2: Commit**

```bash
git add api/proto/literature/v1/literature.proto
git commit -m "feat(proto): add PauseReview, ResumeReview, StopReview RPCs"
```

---

### Task 3.2: Add ListPausedReviews RPC to proto

**Files:**
- Modify: `api/proto/literature/v1/literature.proto`

**Step 1: Add RPC and messages**

```protobuf
// ListPausedReviews returns all paused reviews for an org/project.
rpc ListPausedReviews(ListPausedReviewsRequest) returns (ListPausedReviewsResponse);

message ListPausedReviewsRequest {
    string org_id = 1;
    string project_id = 2;  // Optional - empty returns all projects in org
    string pause_reason = 3; // Optional - filter by "user" or "budget_exhausted"
}

message ListPausedReviewsResponse {
    repeated PausedReview reviews = 1;
}

message PausedReview {
    string request_id = 1;
    string project_id = 2;
    string pause_reason = 3;
    google.protobuf.Timestamp paused_at = 4;
    string paused_at_phase = 5;
    ReviewProgress progress = 6;
}
```

**Step 2: Commit**

```bash
git add api/proto/literature/v1/literature.proto
git commit -m "feat(proto): add ListPausedReviews RPC"
```

---

### Task 3.3-3.4: Generate protobuf code

**Step 1: Generate**

```bash
make proto
```

**Step 2: Commit**

```bash
git add gen/proto/
git commit -m "chore(proto): regenerate protobuf code"
```

---

### Task 3.5-3.8: Implement gRPC handlers

**Files:**
- Modify: `internal/server/review_handler.go`

**Step 1: Implement PauseReview**

```go
func (h *ReviewHandler) PauseReview(ctx context.Context, req *pb.PauseReviewRequest) (*pb.PauseReviewResponse, error) {
    review, err := h.repo.GetByID(ctx, req.OrgId, req.ProjectId, uuid.MustParse(req.RequestId))
    if err != nil {
        return nil, status.Error(codes.NotFound, "review not found")
    }

    if review.Status.IsTerminal() || review.Status == domain.ReviewStatusPaused {
        return nil, status.Error(codes.FailedPrecondition,
            fmt.Sprintf("cannot pause review in %s status", review.Status))
    }

    err = h.workflowClient.SignalWorkflow(ctx, review.WorkflowID, "",
        litemporal.SignalPause,
        workflows.PauseSignal{Reason: domain.PauseReasonUser},
    )
    if err != nil {
        return nil, status.Error(codes.Internal, "failed to pause workflow")
    }

    return &pb.PauseReviewResponse{Success: true, Message: "Review pausing"}, nil
}
```

**Step 2: Implement ResumeReview**

```go
func (h *ReviewHandler) ResumeReview(ctx context.Context, req *pb.ResumeReviewRequest) (*pb.ResumeReviewResponse, error) {
    review, err := h.repo.GetByID(ctx, req.OrgId, req.ProjectId, uuid.MustParse(req.RequestId))
    if err != nil {
        return nil, status.Error(codes.NotFound, "review not found")
    }

    if review.Status != domain.ReviewStatusPaused {
        return nil, status.Error(codes.FailedPrecondition,
            fmt.Sprintf("cannot resume review in %s status", review.Status))
    }

    err = h.workflowClient.SignalWorkflow(ctx, review.WorkflowID, "",
        litemporal.SignalResume,
        workflows.ResumeSignal{ResumedBy: "user"},
    )
    if err != nil {
        return nil, status.Error(codes.Internal, "failed to resume workflow")
    }

    return &pb.ResumeReviewResponse{Success: true, Message: "Review resuming"}, nil
}
```

**Step 3: Implement StopReview**

```go
func (h *ReviewHandler) StopReview(ctx context.Context, req *pb.StopReviewRequest) (*pb.StopReviewResponse, error) {
    review, err := h.repo.GetByID(ctx, req.OrgId, req.ProjectId, uuid.MustParse(req.RequestId))
    if err != nil {
        return nil, status.Error(codes.NotFound, "review not found")
    }

    if review.Status.IsTerminal() {
        return nil, status.Error(codes.FailedPrecondition,
            fmt.Sprintf("cannot stop review in %s status", review.Status))
    }

    // If paused, resume first then stop
    if review.Status == domain.ReviewStatusPaused {
        _ = h.workflowClient.SignalWorkflow(ctx, review.WorkflowID, "",
            litemporal.SignalResume,
            workflows.ResumeSignal{ResumedBy: "stop_request"},
        )
    }

    err = h.workflowClient.SignalWorkflow(ctx, review.WorkflowID, "",
        litemporal.SignalStop,
        workflows.StopSignal{Reason: "user_requested"},
    )
    if err != nil {
        return nil, status.Error(codes.Internal, "failed to stop workflow")
    }

    return &pb.StopReviewResponse{Success: true, Message: "Review stopping"}, nil
}
```

**Step 4: Implement ListPausedReviews**

```go
func (h *ReviewHandler) ListPausedReviews(ctx context.Context, req *pb.ListPausedReviewsRequest) (*pb.ListPausedReviewsResponse, error) {
    var reason domain.PauseReason
    if req.PauseReason != "" {
        reason = domain.PauseReason(req.PauseReason)
    }

    reviews, err := h.repo.FindPausedByReason(ctx, req.OrgId, req.ProjectId, reason)
    if err != nil {
        return nil, status.Error(codes.Internal, "failed to list paused reviews")
    }

    pbReviews := make([]*pb.PausedReview, 0, len(reviews))
    for _, r := range reviews {
        pbReview := &pb.PausedReview{
            RequestId:     r.ID.String(),
            ProjectId:     r.ProjectID,
            PauseReason:   string(r.PauseReason),
            PausedAtPhase: r.PausedAtPhase,
        }
        if r.PausedAt != nil {
            pbReview.PausedAt = timestamppb.New(*r.PausedAt)
        }
        pbReviews = append(pbReviews, pbReview)
    }

    return &pb.ListPausedReviewsResponse{Reviews: pbReviews}, nil
}
```

**Step 5: Commit**

```bash
git add internal/server/review_handler.go
git commit -m "feat(server): implement PauseReview, ResumeReview, StopReview, ListPausedReviews handlers"
```

---

## Phase 4: Budget Auto-Resume

### Task 4.1-4.5: Budget Listener

**Files:**
- Create: `internal/budget/listener.go`
- Create: `internal/budget/listener_test.go`
- Modify: `cmd/worker/main.go`

**Step 1: Create BudgetListener**

```go
// internal/budget/listener.go
package budget

import (
    "context"
    "encoding/json"

    "github.com/rs/zerolog"
    "github.com/segmentio/kafka-go"
    "go.temporal.io/sdk/client"

    "github.com/helixir/literature-review-service/internal/domain"
    "github.com/helixir/literature-review-service/internal/repository"
    litemporal "github.com/helixir/literature-review-service/internal/temporal"
    "github.com/helixir/literature-review-service/internal/temporal/workflows"
)

// BudgetRefilledEvent represents the event from core_service when budget is refilled.
type BudgetRefilledEvent struct {
    OrgID         string  `json:"org_id"`
    ProjectID     string  `json:"project_id"` // Empty = org-wide
    AmountUSD     float64 `json:"amount_usd"`
    NewBalanceUSD float64 `json:"new_balance_usd"`
}

// Listener consumes budget events and auto-resumes paused workflows.
type Listener struct {
    reader         *kafka.Reader
    workflowClient client.Client
    reviewRepo     repository.ReviewRepository
    logger         zerolog.Logger
}

// NewListener creates a new budget event listener.
func NewListener(
    brokers []string,
    topic string,
    groupID string,
    workflowClient client.Client,
    reviewRepo repository.ReviewRepository,
    logger zerolog.Logger,
) *Listener {
    reader := kafka.NewReader(kafka.ReaderConfig{
        Brokers:  brokers,
        Topic:    topic,
        GroupID:  groupID,
        MinBytes: 1,
        MaxBytes: 10e6,
    })

    return &Listener{
        reader:         reader,
        workflowClient: workflowClient,
        reviewRepo:     reviewRepo,
        logger:         logger.With().Str("component", "budget_listener").Logger(),
    }
}

// Run starts the listener loop. Blocks until context is cancelled.
func (l *Listener) Run(ctx context.Context) error {
    l.logger.Info().Msg("starting budget listener")

    for {
        msg, err := l.reader.ReadMessage(ctx)
        if err != nil {
            if ctx.Err() != nil {
                return ctx.Err()
            }
            l.logger.Error().Err(err).Msg("failed to read message")
            continue
        }

        var event BudgetRefilledEvent
        if err := json.Unmarshal(msg.Value, &event); err != nil {
            l.logger.Error().Err(err).Msg("failed to unmarshal budget event")
            continue
        }

        if err := l.handleBudgetRefilled(ctx, event); err != nil {
            l.logger.Error().Err(err).
                Str("orgID", event.OrgID).
                Msg("failed to handle budget refill")
        }
    }
}

func (l *Listener) handleBudgetRefilled(ctx context.Context, event BudgetRefilledEvent) error {
    l.logger.Info().
        Str("orgID", event.OrgID).
        Str("projectID", event.ProjectID).
        Float64("amount", event.AmountUSD).
        Msg("handling budget refill")

    // Find all budget-paused workflows
    paused, err := l.reviewRepo.FindPausedByReason(ctx,
        event.OrgID,
        event.ProjectID,
        domain.PauseReasonBudgetExhausted,
    )
    if err != nil {
        return err
    }

    if len(paused) == 0 {
        l.logger.Debug().Msg("no budget-paused workflows to resume")
        return nil
    }

    l.logger.Info().Int("count", len(paused)).Msg("resuming budget-paused workflows")

    for _, review := range paused {
        err := l.workflowClient.SignalWorkflow(ctx,
            review.WorkflowID,
            "",
            litemporal.SignalResume,
            workflows.ResumeSignal{ResumedBy: "budget_refill"},
        )
        if err != nil {
            l.logger.Error().Err(err).
                Str("workflowID", review.WorkflowID).
                Msg("failed to resume workflow")
            // Continue with others
        } else {
            l.logger.Info().
                Str("workflowID", review.WorkflowID).
                Msg("sent resume signal")
        }
    }

    return nil
}

// Close closes the Kafka reader.
func (l *Listener) Close() error {
    return l.reader.Close()
}
```

**Step 2: Integrate in worker main**

```go
// cmd/worker/main.go

// Start budget listener in background
if cfg.Kafka.Enabled {
    budgetListener := budget.NewListener(
        cfg.Kafka.Brokers,
        "budget.refilled",
        "literature-service-budget",
        temporalClient,
        reviewRepo,
        logger,
    )

    go func() {
        if err := budgetListener.Run(ctx); err != nil && err != context.Canceled {
            logger.Error().Err(err).Msg("budget listener error")
        }
    }()

    defer budgetListener.Close()
}
```

**Step 3: Commit**

```bash
git add internal/budget/ cmd/worker/main.go
git commit -m "feat(budget): add BudgetListener for auto-resume on budget refill"
```

---

## Phase 5: Testing

### Task 5.1-5.7: Tests

Create comprehensive tests for all new functionality:

- Unit tests for signal handling
- Unit tests for pause/resume/stop logic
- Unit tests for budget exhaustion detection
- Integration tests with Temporal test environment
- Multi-tenant isolation tests

**Files:**
- Create: `internal/temporal/workflows/review_workflow_pause_test.go`
- Create: `internal/budget/listener_test.go`
- Modify: `internal/server/review_handler_test.go`

---

## Files Summary

**Create:**
- `migrations/000008_add_pause_fields.up.sql`
- `migrations/000008_add_pause_fields.down.sql`
- `internal/temporal/workflows/signals.go`
- `internal/temporal/workflows/signals_test.go`
- `internal/budget/listener.go`
- `internal/budget/listener_test.go`
- `internal/temporal/workflows/review_workflow_pause_test.go`

**Modify:**
- `internal/domain/models.go`
- `internal/domain/models_test.go`
- `internal/domain/review.go`
- `internal/domain/review_test.go`
- `internal/repository/review_repository.go`
- `internal/repository/review_repository_test.go`
- `internal/temporal/signals.go`
- `internal/temporal/workflows/review_workflow.go`
- `internal/temporal/activities/status_activities.go`
- `internal/temporal/activities/types.go`
- `api/proto/literature/v1/literature.proto`
- `gen/proto/literature/v1/*.go`
- `internal/server/review_handler.go`
- `internal/server/review_handler_test.go`
- `cmd/worker/main.go`

---

**Estimated effort:** 3-4 days
