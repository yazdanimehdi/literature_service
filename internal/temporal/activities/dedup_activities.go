package activities

import (
	"context"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"

	"github.com/helixir/literature-review-service/internal/dedup"
)

// DedupActivities provides Temporal activities for semantic deduplication of papers.
// Methods on this struct are registered as Temporal activities via the worker.
type DedupActivities struct {
	checker *dedup.Checker
}

// NewDedupActivities creates a new DedupActivities instance with the given checker.
func NewDedupActivities(checker *dedup.Checker) *DedupActivities {
	return &DedupActivities{
		checker: checker,
	}
}

// DedupPapers checks a batch of papers for duplicates against the vector store.
//
// For each paper the activity:
//   - Skips papers without an abstract (counted as skipped, included in non-duplicates).
//   - Calls the checker to determine if the paper is a duplicate.
//   - If the check fails, logs a warning and treats the paper as non-duplicate (resilient).
//   - If the paper is a duplicate, counts it and excludes it from non-duplicates.
//   - Heartbeats every 10 papers to signal liveness to Temporal.
func (a *DedupActivities) DedupPapers(ctx context.Context, input DedupPapersInput) (*DedupPapersOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("starting dedup check", "paperCount", len(input.Papers))

	var nonDuplicateIDs []uuid.UUID
	var duplicateCount int
	var skippedCount int

	for i, paper := range input.Papers {
		// Heartbeat every 10 papers.
		if i > 0 && i%10 == 0 {
			activity.RecordHeartbeat(ctx, i)
		}

		// Papers without an abstract are skipped but included in non-duplicates.
		if paper.Abstract == "" {
			skippedCount++
			nonDuplicateIDs = append(nonDuplicateIDs, paper.ID)
			continue
		}

		result, err := a.checker.Check(ctx, paper)
		if err != nil {
			// Resilient: treat check failures as non-duplicate.
			logger.Warn("dedup check failed, treating as non-duplicate",
				"paperID", paper.ID,
				"error", err,
			)
			nonDuplicateIDs = append(nonDuplicateIDs, paper.ID)
			continue
		}

		if result.IsDuplicate {
			duplicateCount++
			logger.Info("duplicate detected",
				"paperID", paper.ID,
				"duplicateOf", result.DuplicateOf,
				"score", result.Score,
			)
			continue
		}

		nonDuplicateIDs = append(nonDuplicateIDs, paper.ID)
	}

	logger.Info("dedup check completed",
		"total", len(input.Papers),
		"nonDuplicates", len(nonDuplicateIDs),
		"duplicates", duplicateCount,
		"skipped", skippedCount,
	)

	return &DedupPapersOutput{
		NonDuplicateIDs: nonDuplicateIDs,
		DuplicateCount:  duplicateCount,
		SkippedCount:    skippedCount,
	}, nil
}
