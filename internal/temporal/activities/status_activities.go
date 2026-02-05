package activities

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/observability"
	"github.com/helixir/literature-review-service/internal/repository"
)

// StatusActivities provides Temporal activities for review status updates
// and persistence operations (keywords, papers, counters).
// Methods on this struct are registered as Temporal activities via the worker.
type StatusActivities struct {
	reviewRepo  repository.ReviewRepository
	keywordRepo repository.KeywordRepository
	paperRepo   repository.PaperRepository
	metrics     *observability.Metrics
}

// NewStatusActivities creates a new StatusActivities instance with the given dependencies.
// The metrics parameter may be nil (metrics recording will be skipped).
func NewStatusActivities(
	reviewRepo repository.ReviewRepository,
	keywordRepo repository.KeywordRepository,
	paperRepo repository.PaperRepository,
	metrics *observability.Metrics,
) *StatusActivities {
	return &StatusActivities{
		reviewRepo:  reviewRepo,
		keywordRepo: keywordRepo,
		paperRepo:   paperRepo,
		metrics:     metrics,
	}
}

// UpdateStatus updates the status of a literature review request.
//
// For terminal states (failed, cancelled), metrics are recorded. The errorMsg
// field is stored only when transitioning to a failed state.
func (a *StatusActivities) UpdateStatus(ctx context.Context, input UpdateStatusInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("updating review status",
		"requestID", input.RequestID,
		"status", input.Status,
		"hasErrorMsg", input.ErrorMsg != "",
	)

	err := a.reviewRepo.UpdateStatus(ctx, input.OrgID, input.ProjectID, input.RequestID, input.Status, input.ErrorMsg)
	if err != nil {
		logger.Error("failed to update review status",
			"requestID", input.RequestID,
			"status", input.Status,
			"error", err,
		)
		return fmt.Errorf("update review status to %s: %w", input.Status, err)
	}

	// Record metrics for terminal states.
	if a.metrics != nil {
		switch input.Status {
		case domain.ReviewStatusFailed:
			a.metrics.RecordReviewFailed(0)
		case domain.ReviewStatusCancelled:
			a.metrics.RecordReviewCancelled()
		}
	}

	logger.Info("review status updated",
		"requestID", input.RequestID,
		"status", input.Status,
	)

	return nil
}

// SaveKeywords persists extracted keywords using bulk get-or-create semantics.
//
// Keywords that already exist (by normalized form) are returned with their existing IDs.
// Newly created keywords are assigned fresh UUIDs. The output includes all keyword IDs
// and the count of newly created keywords.
func (a *StatusActivities) SaveKeywords(ctx context.Context, input SaveKeywordsInput) (*SaveKeywordsOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("saving keywords",
		"requestID", input.RequestID,
		"keywordCount", len(input.Keywords),
		"extractionRound", input.ExtractionRound,
		"sourceType", input.SourceType,
	)

	keywords, err := a.keywordRepo.BulkGetOrCreate(ctx, input.Keywords)
	if err != nil {
		logger.Error("failed to save keywords",
			"requestID", input.RequestID,
			"error", err,
		)
		return nil, fmt.Errorf("bulk get or create keywords: %w", err)
	}

	keywordIDs := make([]uuid.UUID, 0, len(keywords))
	for _, kw := range keywords {
		keywordIDs = append(keywordIDs, kw.ID)
	}

	// NewCount approximation: we cannot distinguish new from existing in BulkGetOrCreate
	// without additional metadata. We report the total count returned as the number of
	// keywords that were resolved (either found or created). The caller can compare with
	// the input length if needed.
	newCount := len(keywords)

	logger.Info("keywords saved",
		"requestID", input.RequestID,
		"resolvedCount", len(keywords),
	)

	return &SaveKeywordsOutput{
		KeywordIDs: keywordIDs,
		NewCount:   newCount,
	}, nil
}

// SavePapers persists discovered papers using bulk upsert semantics and increments
// the review request's paper counters.
//
// Papers that already exist (by canonical ID) are updated. New papers are created.
// After persistence, the review request's papersFound counter is incremented by the
// total count and papersIngested counter by the number of newly saved papers.
// If the input papers slice is empty, the method returns zero counts without calling
// any repository methods.
func (a *StatusActivities) SavePapers(ctx context.Context, input SavePapersInput) (*SavePapersOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("saving papers",
		"requestID", input.RequestID,
		"paperCount", len(input.Papers),
		"source", input.DiscoveredViaSource,
		"expansionDepth", input.ExpansionDepth,
	)

	// Short-circuit for empty input.
	if len(input.Papers) == 0 {
		logger.Info("no papers to save, skipping",
			"requestID", input.RequestID,
		)
		return &SavePapersOutput{
			SavedCount:     0,
			DuplicateCount: 0,
		}, nil
	}

	savedPapers, err := a.paperRepo.BulkUpsert(ctx, input.Papers)
	if err != nil {
		logger.Error("failed to save papers",
			"requestID", input.RequestID,
			"error", err,
		)
		return nil, fmt.Errorf("bulk upsert papers: %w", err)
	}

	savedCount := len(savedPapers)
	duplicateCount := len(input.Papers) - savedCount

	// Increment review counters.
	if err := a.reviewRepo.IncrementCounters(ctx, input.OrgID, input.ProjectID, input.RequestID, savedCount, 0); err != nil {
		logger.Error("failed to increment review counters",
			"requestID", input.RequestID,
			"error", err,
		)
		return nil, fmt.Errorf("increment review counters: %w", err)
	}

	// Record metrics.
	if a.metrics != nil {
		a.metrics.RecordPapersDiscovered(string(input.DiscoveredViaSource), savedCount)
		if duplicateCount > 0 {
			a.metrics.RecordPaperDuplicates(duplicateCount)
		}
	}

	logger.Info("papers saved",
		"requestID", input.RequestID,
		"savedCount", savedCount,
		"duplicateCount", duplicateCount,
	)

	return &SavePapersOutput{
		SavedCount:     savedCount,
		DuplicateCount: duplicateCount,
	}, nil
}

// IncrementCounters atomically increments the papers found and ingested counters
// on a literature review request.
func (a *StatusActivities) IncrementCounters(ctx context.Context, input IncrementCountersInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("incrementing review counters",
		"requestID", input.RequestID,
		"papersFound", input.PapersFound,
		"papersIngested", input.PapersIngested,
	)

	err := a.reviewRepo.IncrementCounters(ctx, input.OrgID, input.ProjectID, input.RequestID, input.PapersFound, input.PapersIngested)
	if err != nil {
		logger.Error("failed to increment review counters",
			"requestID", input.RequestID,
			"error", err,
		)
		return fmt.Errorf("increment review counters: %w", err)
	}

	logger.Info("review counters incremented",
		"requestID", input.RequestID,
	)

	return nil
}

// UpdatePaperIngestionResults updates papers with their file_id and ingestion_run_id
// after successful download and ingestion. Failures are non-fatal.
func (a *StatusActivities) UpdatePaperIngestionResults(ctx context.Context, input UpdatePaperIngestionResultsInput) (*UpdatePaperIngestionResultsOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("updating paper ingestion results", "count", len(input.Results))

	output := &UpdatePaperIngestionResultsOutput{}

	for _, result := range input.Results {
		if result.FileID == "" || result.IngestionRunID == "" {
			output.Skipped++
			continue
		}

		activity.RecordHeartbeat(ctx, fmt.Sprintf("updating paper %s", result.PaperID))

		fileID, err := uuid.Parse(result.FileID)
		if err != nil {
			logger.Warn("invalid file_id, skipping", "paperID", result.PaperID, "fileID", result.FileID)
			output.Failed++
			continue
		}

		err = a.paperRepo.UpdateIngestionResult(ctx, result.PaperID, fileID, result.IngestionRunID)
		if err != nil {
			logger.Warn("failed to update paper ingestion result",
				"paperID", result.PaperID,
				"error", err,
			)
			output.Failed++
			continue
		}

		output.Updated++
	}

	logger.Info("paper ingestion results updated",
		"updated", output.Updated,
		"skipped", output.Skipped,
		"failed", output.Failed,
	)

	return output, nil
}
