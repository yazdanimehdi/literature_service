package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	keywordIDMap := make(map[string]uuid.UUID, len(keywords)*2)
	for _, kw := range keywords {
		keywordIDs = append(keywordIDs, kw.ID)
		// Store by both original keyword and normalized form so callers
		// can look up by either the original or normalized string.
		keywordIDMap[kw.Keyword] = kw.ID
		keywordIDMap[kw.NormalizedKeyword] = kw.ID
	}
	// Also map back from the input keywords (which may differ in case/whitespace).
	for i, kw := range keywords {
		if i < len(input.Keywords) {
			keywordIDMap[input.Keywords[i]] = kw.ID
		}
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
		KeywordIDs:   keywordIDs,
		KeywordIDMap: keywordIDMap,
		NewCount:     newCount,
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

	// Collect paper IDs from the upserted results so the workflow can
	// reference them for child workflow batches. Papers arrive from search
	// sources with uuid.Nil; BulkUpsert assigns/returns the real DB IDs.
	paperIDs := make([]uuid.UUID, 0, len(savedPapers))
	for _, p := range savedPapers {
		if p != nil {
			paperIDs = append(paperIDs, p.ID)
		}
	}

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
		PaperIDs:       paperIDs,
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

// UpdatePauseState updates the pause state of a review request.
// This is used when a workflow needs to pause (e.g., due to budget exhaustion or user request).
func (a *StatusActivities) UpdatePauseState(ctx context.Context, input UpdatePauseStateInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("updating pause state",
		"requestID", input.RequestID,
		"status", input.Status,
		"pauseReason", input.PauseReason,
		"phase", input.PausedAtPhase,
	)

	err := a.reviewRepo.UpdatePauseState(ctx, input.OrgID, input.ProjectID, input.RequestID,
		input.Status, input.PauseReason, input.PausedAtPhase)
	if err != nil {
		logger.Error("failed to update pause state",
			"requestID", input.RequestID,
			"error", err,
		)
		return fmt.Errorf("update pause state: %w", err)
	}

	logger.Info("pause state updated",
		"requestID", input.RequestID,
		"status", input.Status,
		"pauseReason", input.PauseReason,
	)

	return nil
}

// CheckSearchCompleted checks if a keyword+source search was already done.
// This enables search deduplication — when a workflow is resumed or a keyword
// appears in multiple expansion rounds, we skip searches that already have results.
func (a *StatusActivities) CheckSearchCompleted(ctx context.Context, input CheckSearchCompletedInput) (*CheckSearchCompletedOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("checking search completion",
		"keywordID", input.KeywordID,
		"keyword", input.Keyword,
		"source", input.Source,
	)

	// Look up the most recent search for this keyword+source.
	lastSearch, err := a.keywordRepo.GetLastSearch(ctx, input.KeywordID, input.Source)
	if err != nil {
		// Not found = never searched.
		if errors.Is(err, domain.ErrNotFound) {
			return &CheckSearchCompletedOutput{AlreadyCompleted: false}, nil
		}
		return nil, fmt.Errorf("check search completed: %w", err)
	}

	// Only consider completed searches as deduplicated.
	if lastSearch.Status != domain.SearchStatusCompleted {
		logger.Info("previous search not completed, will re-search",
			"keyword", input.Keyword,
			"source", input.Source,
			"lastStatus", lastSearch.Status,
		)
		return &CheckSearchCompletedOutput{AlreadyCompleted: false}, nil
	}

	// Fetch cached papers from keyword_paper_mappings, filtered by source
	// to avoid cross-source paper leakage.
	// Limit to 200 papers to prevent workflow history bloat — each paper is ~7KB,
	// so 200 papers ≈ 1.4MB per cached search in Temporal serialization.
	const maxCachedPapers = 200
	papers, _, err := a.keywordRepo.GetPapersForKeywordAndSource(ctx, input.KeywordID, input.Source, maxCachedPapers, 0)
	if err != nil {
		logger.Warn("failed to fetch cached papers, will re-search",
			"keyword", input.Keyword,
			"source", input.Source,
			"error", err,
		)
		return &CheckSearchCompletedOutput{AlreadyCompleted: false}, nil
	}

	logger.Info("search already completed, reusing cached results",
		"keyword", input.Keyword,
		"source", input.Source,
		"searchedAt", lastSearch.SearchedAt,
		"cachedPapers", len(papers),
	)

	return &CheckSearchCompletedOutput{
		AlreadyCompleted:      true,
		PreviouslyFoundPapers: papers,
		PapersFoundCount:      lastSearch.PapersFound,
		SearchedAt:            lastSearch.SearchedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// RecordSearchResult records a completed search and its paper mappings.
// This persists the search record for future deduplication and links
// the discovered papers to the keyword.
func (a *StatusActivities) RecordSearchResult(ctx context.Context, input RecordSearchResultInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("recording search result",
		"keywordID", input.KeywordID,
		"source", input.Source,
		"papersFound", input.PapersFound,
		"status", input.Status,
	)

	// Compute search window hash for idempotency.
	hash := domain.ComputeSearchWindowHash(input.KeywordID, input.Source, input.DateFrom, input.DateTo)

	status := input.Status
	if status == "" {
		status = domain.SearchStatusCompleted
	}

	search := &domain.KeywordSearch{
		KeywordID:        input.KeywordID,
		SourceAPI:        input.Source,
		PapersFound:      input.PapersFound,
		Status:           status,
		ErrorMessage:     input.ErrorMessage,
		SearchWindowHash: hash,
	}

	// Parse optional date range.
	if input.DateFrom != nil {
		t, err := parseDate(*input.DateFrom)
		if err != nil {
			logger.Warn("invalid DateFrom format, ignoring",
				"dateFrom", *input.DateFrom,
				"error", err,
			)
		} else {
			search.DateFrom = &t
		}
	}
	if input.DateTo != nil {
		t, err := parseDate(*input.DateTo)
		if err != nil {
			logger.Warn("invalid DateTo format, ignoring",
				"dateTo", *input.DateTo,
				"error", err,
			)
		} else {
			search.DateTo = &t
		}
	}

	if err := a.keywordRepo.RecordSearch(ctx, search); err != nil {
		logger.Error("failed to record search", "error", err)
		return fmt.Errorf("record search: %w", err)
	}

	// Create keyword-paper mappings for the found papers.
	if len(input.PaperIDs) > 0 {
		mappings := make([]*domain.KeywordPaperMapping, 0, len(input.PaperIDs))
		for _, paperID := range input.PaperIDs {
			mappings = append(mappings, &domain.KeywordPaperMapping{
				KeywordID:   input.KeywordID,
				PaperID:     paperID,
				MappingType: domain.MappingTypeQueryMatch,
				SourceType:  input.Source,
			})
		}

		if err := a.keywordRepo.BulkAddPaperMappings(ctx, mappings); err != nil {
			logger.Warn("failed to record keyword-paper mappings",
				"keywordID", input.KeywordID,
				"error", err,
			)
			// Non-fatal: the search record is saved, mappings are best-effort.
		}
	}

	logger.Info("search result recorded",
		"keywordID", input.KeywordID,
		"source", input.Source,
		"papersFound", input.PapersFound,
		"mappings", len(input.PaperIDs),
	)

	return nil
}

// FetchPaperBatch fetches full paper details from the database by their IDs.
// This is used by child workflows to retrieve paper data without passing large payloads
// through Temporal's serialization boundary.
func (a *StatusActivities) FetchPaperBatch(ctx context.Context, input FetchPaperBatchInput) (*FetchPaperBatchOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("fetching paper batch", "count", len(input.PaperIDs))

	if len(input.PaperIDs) == 0 {
		return &FetchPaperBatchOutput{Papers: []PaperForProcessing{}}, nil
	}

	papers, err := a.paperRepo.GetByIDs(ctx, input.PaperIDs)
	if err != nil {
		logger.Error("failed to fetch papers by IDs", "error", err)
		return nil, fmt.Errorf("fetch papers by IDs: %w", err)
	}

	result := make([]PaperForProcessing, 0, len(papers))
	for _, p := range papers {
		if p == nil {
			continue
		}
		authors := make([]string, 0, len(p.Authors))
		for _, author := range p.Authors {
			if author.Name != "" {
				authors = append(authors, author.Name)
			}
		}
		result = append(result, PaperForProcessing{
			PaperID:     p.ID,
			CanonicalID: p.CanonicalID,
			Title:       p.Title,
			Abstract:    p.Abstract,
			PDFURL:      p.PDFURL,
			Authors:     authors,
			OpenAccess:  p.OpenAccess,
		})
	}

	logger.Info("paper batch fetched", "requested", len(input.PaperIDs), "found", len(result))

	return &FetchPaperBatchOutput{Papers: result}, nil
}

// BulkCreateKeywordPaperMappings creates keyword-paper mappings in bulk after papers
// have been persisted to the database. This defers mapping creation until after SavePapers
// to avoid FK constraint violations on the keyword_paper_mappings table.
func (a *StatusActivities) BulkCreateKeywordPaperMappings(ctx context.Context, input BulkCreateKeywordPaperMappingsInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("bulk creating keyword-paper mappings", "entryCount", len(input.Entries))

	// Flatten all entries into a single slice to minimize DB round trips.
	totalMappings := 0
	for _, entry := range input.Entries {
		totalMappings += len(entry.PaperIDs)
	}

	if totalMappings == 0 {
		logger.Info("no keyword-paper mappings to create")
		return nil
	}

	allMappings := make([]*domain.KeywordPaperMapping, 0, totalMappings)
	for _, entry := range input.Entries {
		for _, paperID := range entry.PaperIDs {
			allMappings = append(allMappings, &domain.KeywordPaperMapping{
				KeywordID:   entry.KeywordID,
				PaperID:     paperID,
				MappingType: domain.MappingTypeQueryMatch,
				SourceType:  entry.Source,
			})
		}
	}

	activity.RecordHeartbeat(ctx, fmt.Sprintf("inserting %d mappings", len(allMappings)))

	if err := a.keywordRepo.BulkAddPaperMappings(ctx, allMappings); err != nil {
		logger.Error("failed to create keyword-paper mappings",
			"mappingCount", len(allMappings),
			"error", err,
		)
		return fmt.Errorf("bulk add keyword-paper mappings: %w", err)
	}

	logger.Info("keyword-paper mappings created",
		"entryCount", len(input.Entries),
		"mappingCount", len(allMappings),
	)

	return nil
}

// maxDateStringLen is the maximum length of a date string we accept for parsing.
// RFC3339 with timezone offset is at most ~35 chars ("2006-01-02T15:04:05.999999999Z07:00").
const maxDateStringLen = 40

// parseDate parses a date string in YYYY-MM-DD or RFC3339 format.
// RFC3339 support is needed because SearchedAt timestamps are formatted as RFC3339
// and flow through as DateFrom values for forward searches.
func parseDate(s string) (time.Time, error) {
	if len(s) > maxDateStringLen {
		return time.Time{}, fmt.Errorf("date string too long: %d characters (max %d)", len(s), maxDateStringLen)
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
