package workflows

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
)

const (
	// maxConcurrentBatches limits child workflow concurrency to avoid
	// overwhelming downstream services (embed, dedup, ingest).
	maxConcurrentBatches = 20

	// maxCandidatePapers bounds the papers retained for expansion/coverage.
	// Only papers with non-empty abstracts are kept.
	maxCandidatePapers = 50

	// savePapersBatchSize limits papers per SavePapers activity call to stay
	// within Temporal's ~2 MB payload size limit. Each domain.Paper includes
	// title, abstract, authors (JSONB), and raw_metadata (JSONB), so 50
	// papers is a safe upper bound.
	savePapersBatchSize = 50
)

// searchFutureEntry replaces the 3 locally-defined identical structs used in
// each search phase (initial, expansion, gap).
type searchFutureEntry struct {
	source    domain.SourceType
	keyword   string
	keywordID uuid.UUID
	dateFrom  *string
	future    workflow.Future
}

// searchExecutor bundles workflow-level shared state for search helpers.
type searchExecutor struct {
	cancelCtx            workflow.Context
	statusCtx            workflow.Context
	searchAct            *activities.SearchActivities
	statusAct            *activities.StatusActivities
	logger               log.Logger
	progress             *workflowProgress
	input                ReviewWorkflowInput
	wfInfo               *workflow.Info
	searchActivityTimeout time.Duration // Configurable search activity timeout.
}

// searchPhaseParams contains phase-specific parameters.
type searchPhaseParams struct {
	keywords         []string
	keywordIDMap     map[string]uuid.UUID
	sources          []domain.SourceType
	resultsPerSource int
	logPrefix        string // "initial", "expansion round 1", "gap"
}

// searchPhaseResult contains the output of a search phase.
type searchPhaseResult struct {
	papers              []*domain.Paper
	keywordPaperEntries []activities.KeywordPaperMappingEntry
	totalFound          int
}

// saveAndSpawnParams contains parameters for saving papers and spawning batches.
type saveAndSpawnParams struct {
	papers              []*domain.Paper
	keywordPaperEntries []activities.KeywordPaperMappingEntry
	discoveredVia       domain.SourceType
	expansionDepth      int
	batchIDPrefix       string // e.g. "{wfID}", "{wfID}-exp1", "{wfID}-gap"
	fatalOnSaveError    bool   // initial+expansion: true, gap: false
	childFutures        *[]workflow.ChildWorkflowFuture
}

// executeSearch runs the keyword x source search loop used by all three search
// phases. It returns discovered papers, keyword-paper mapping entries, and the
// total papers found count. This is a regular Go function (not a Temporal
// activity) that calls workflow.ExecuteActivity internally.
func (e *searchExecutor) executeSearch(params searchPhaseParams) searchPhaseResult {
	var result searchPhaseResult
	var futures []searchFutureEntry

	for _, keyword := range params.keywords {
		keyword := keyword // capture
		kwID := params.keywordIDMap[keyword]

		for idx, source := range params.sources {
			source := source // capture

			// Search deduplication: check if this keyword+source was already searched.
			var forwardDateFrom *string
			if kwID != uuid.Nil {
				var checkOutput activities.CheckSearchCompletedOutput
				checkErr := workflow.ExecuteActivity(e.statusCtx, e.statusAct.CheckSearchCompleted, activities.CheckSearchCompletedInput{
					ReviewID:  e.input.RequestID,
					KeywordID: kwID,
					Keyword:   keyword,
					Source:    source,
				}).Get(e.cancelCtx, &checkOutput)

				if checkErr == nil && checkOutput.AlreadyCompleted {
					e.logger.Info(params.logPrefix+" search cached — using cached results + searching forward",
						"keyword", keyword,
						"source", source,
						"cachedPapers", checkOutput.PapersFoundCount,
					)
					if len(checkOutput.PreviouslyFoundPapers) > 0 {
						result.papers = append(result.papers, checkOutput.PreviouslyFoundPapers...)
						result.totalFound += checkOutput.PapersFoundCount
						e.progress.PapersFound += checkOutput.PapersFoundCount
					}
					if checkOutput.SearchedAt != "" {
						forwardDateFrom = &checkOutput.SearchedAt
					}
				}
			}

			actCtx := workflow.WithActivityOptions(e.cancelCtx, workflow.ActivityOptions{
				StartToCloseTimeout: e.searchActivityTimeout,
				RetryPolicy: &temporal.RetryPolicy{
					InitialInterval:    2 * time.Second,
					BackoffCoefficient: 2.0,
					MaximumInterval:    1 * time.Minute,
					MaximumAttempts:    3,
				},
			})

			// Rate limiting: stagger requests to avoid overwhelming sources.
			if idx > 0 {
				_ = workflow.Sleep(e.cancelCtx, time.Duration(idx)*searchRateLimitDelay)
			}

			future := workflow.ExecuteActivity(actCtx, e.searchAct.SearchSingleSource, activities.SearchSingleSourceInput{
				Source:           source,
				Query:            keyword,
				MaxResults:       params.resultsPerSource,
				IncludePreprints: e.input.Config.IncludePreprints,
				OpenAccessOnly:   e.input.Config.RequireOpenAccess,
				MinCitations:     e.input.Config.MinCitations,
				DateFrom:         forwardDateFrom,
			})

			futures = append(futures, searchFutureEntry{
				source:    source,
				keyword:   keyword,
				keywordID: kwID,
				dateFrom:  forwardDateFrom,
				future:    future,
			})
		}
	}

	// Process results in deterministic order.
	for _, sf := range futures {
		var output activities.SearchSingleSourceOutput
		err := sf.future.Get(e.cancelCtx, &output)
		if err != nil {
			e.logger.Warn(params.logPrefix+" search failed",
				"source", sf.source,
				"keyword", sf.keyword,
				"error", err,
			)
			if sf.keywordID != uuid.Nil {
				_ = workflow.ExecuteActivity(e.statusCtx, e.statusAct.RecordSearchResult, activities.RecordSearchResultInput{
					KeywordID:    sf.keywordID,
					Source:       sf.source,
					DateFrom:     sf.dateFrom,
					PapersFound:  0,
					Status:       domain.SearchStatusFailed,
					ErrorMessage: err.Error(),
				}).Get(e.cancelCtx, nil)
			}
			continue
		}

		if output.Error != "" {
			e.logger.Warn(params.logPrefix+" search returned error",
				"source", sf.source,
				"keyword", sf.keyword,
				"error", output.Error,
			)
			if sf.keywordID != uuid.Nil {
				_ = workflow.ExecuteActivity(e.statusCtx, e.statusAct.RecordSearchResult, activities.RecordSearchResultInput{
					KeywordID:    sf.keywordID,
					Source:       sf.source,
					DateFrom:     sf.dateFrom,
					PapersFound:  0,
					Status:       domain.SearchStatusFailed,
					ErrorMessage: output.Error,
				}).Get(e.cancelCtx, nil)
			}
			continue
		}

		if len(output.Papers) > 0 {
			result.papers = append(result.papers, output.Papers...)
			result.totalFound += output.TotalFound
			e.progress.PapersFound += output.TotalFound
			e.logger.Info(params.logPrefix+" search completed",
				"source", sf.source,
				"keyword", sf.keyword,
				"papers", len(output.Papers),
			)
		}

		if sf.keywordID != uuid.Nil {
			_ = workflow.ExecuteActivity(e.statusCtx, e.statusAct.RecordSearchResult, activities.RecordSearchResultInput{
				KeywordID:   sf.keywordID,
				Source:      sf.source,
				DateFrom:    sf.dateFrom,
				PapersFound: output.TotalFound,
				Status:      domain.SearchStatusCompleted,
			}).Get(e.cancelCtx, nil)

			if pids := extractPaperIDs(output.Papers); len(pids) > 0 {
				result.keywordPaperEntries = append(result.keywordPaperEntries, activities.KeywordPaperMappingEntry{
					KeywordID: sf.keywordID,
					Source:    sf.source,
					PaperIDs:  pids,
				})
			}
		}
	}

	e.logger.Info(params.logPrefix+" all searches completed", "totalPapers", len(result.papers))
	return result
}

// saveAndSpawnBatches saves papers to DB, creates keyword-paper mappings, then
// batches paper IDs and spawns child workflows with bounded concurrency.
// Returns nil on empty papers (no-op).
func (e *searchExecutor) saveAndSpawnBatches(params saveAndSpawnParams) error {
	if len(params.papers) == 0 {
		return nil
	}

	// Batch SavePapers calls to stay within Temporal's payload size limit.
	// Collect all DB-assigned paper IDs from the activity outputs, since papers
	// from search sources arrive with uuid.Nil and get real IDs during BulkUpsert.
	var totalSaved, totalDuplicates int
	var allPaperIDs []uuid.UUID
	for start := 0; start < len(params.papers); start += savePapersBatchSize {
		end := start + savePapersBatchSize
		if end > len(params.papers) {
			end = len(params.papers)
		}
		chunk := params.papers[start:end]

		var savePapersOutput activities.SavePapersOutput
		err := workflow.ExecuteActivity(e.statusCtx, e.statusAct.SavePapers, activities.SavePapersInput{
			RequestID:           e.input.RequestID,
			OrgID:               e.input.OrgID,
			ProjectID:           e.input.ProjectID,
			Papers:              chunk,
			DiscoveredViaSource: params.discoveredVia,
			ExpansionDepth:      params.expansionDepth,
		}).Get(e.cancelCtx, &savePapersOutput)
		if err != nil {
			if params.fatalOnSaveError {
				return err
			}
			e.logger.Warn("failed to save papers batch", "error", err, "batchStart", start, "batchSize", len(chunk))
			return nil
		}
		totalSaved += savePapersOutput.SavedCount
		totalDuplicates += savePapersOutput.DuplicateCount
		allPaperIDs = append(allPaperIDs, savePapersOutput.PaperIDs...)
	}
	e.logger.Info("papers saved", "saved", totalSaved, "duplicates", totalDuplicates)

	// Create keyword-paper mappings now that papers exist in DB.
	if len(params.keywordPaperEntries) > 0 {
		_ = workflow.ExecuteActivity(e.statusCtx, e.statusAct.BulkCreateKeywordPaperMappings, activities.BulkCreateKeywordPaperMappingsInput{
			Entries: params.keywordPaperEntries,
		}).Get(e.cancelCtx, nil)
	}

	// Batch and spawn child workflows using the DB-assigned paper IDs.
	if len(allPaperIDs) == 0 {
		return nil
	}

	idBatches := createIDBatches(allPaperIDs, batchSize)
	return e.spawnBatchesWithWindow(idBatches, params.batchIDPrefix, params.childFutures)
}

// spawnBatchesWithWindow spawns child workflows using a sliding window to
// limit concurrency. It uses progress.BatchesSpawned and
// progress.BatchesCompleted as cumulative counters: inflight =
// spawned - completed.
func (e *searchExecutor) spawnBatchesWithWindow(batches [][]uuid.UUID, batchIDPrefix string, childFutures *[]workflow.ChildWorkflowFuture) error {
	for i, batch := range batches {
		// Sliding window: wait if too many batches are in-flight.
		if e.progress.BatchesSpawned-e.progress.BatchesCompleted >= maxConcurrentBatches {
			err := workflow.Await(e.cancelCtx, func() bool {
				return e.progress.BatchesSpawned-e.progress.BatchesCompleted < maxConcurrentBatches
			})
			if err != nil {
				return fmt.Errorf("await batch window slot: %w", err)
			}
		}

		batchID := fmt.Sprintf("%s-batch-%d", batchIDPrefix, i)

		childCtx := workflow.WithChildOptions(e.cancelCtx, workflow.ChildWorkflowOptions{
			WorkflowID: batchID,
			TaskQueue:  e.wfInfo.TaskQueueName,
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval:    2 * time.Second,
				BackoffCoefficient: 2.0,
				MaximumInterval:    1 * time.Minute,
				MaximumAttempts:    3,
			},
		})

		future := workflow.ExecuteChildWorkflow(childCtx, PaperProcessingWorkflow, PaperProcessingInput{
			OrgID:            e.input.OrgID,
			ProjectID:        e.input.ProjectID,
			RequestID:        e.input.RequestID.String(),
			Batch:            PaperIDBatch{BatchID: batchID, PaperIDs: batch},
			ParentWorkflowID: e.wfInfo.WorkflowExecution.ID,
			Timeouts: ChildWorkflowTimeouts{
				EmbeddingActivity: e.input.Timeouts.EmbeddingActivity,
				DedupActivity:     e.input.Timeouts.DedupActivity,
				IngestionActivity: e.input.Timeouts.IngestionActivity,
				StatusActivity:    e.input.Timeouts.StatusActivity,
			},
		})
		*childFutures = append(*childFutures, future)

		e.progress.BatchesSpawned++
		e.logger.Info("spawned batch workflow",
			"batchID", batchID,
			"paperCount", len(batch),
		)
	}

	// Wait for all spawned batches to complete.
	if e.progress.BatchesSpawned > 0 {
		err := workflow.Await(e.cancelCtx, func() bool {
			return e.progress.BatchesCompleted >= e.progress.BatchesSpawned
		})
		if err != nil {
			return fmt.Errorf("await batch completion: %w", err)
		}
	}

	return nil
}

// appendCandidates adds papers with non-empty abstracts to candidates up to max.
func appendCandidates(candidates *[]*domain.Paper, papers []*domain.Paper, max int) {
	for _, p := range papers {
		if len(*candidates) >= max {
			return
		}
		if p != nil && p.Abstract != "" {
			*candidates = append(*candidates, p)
		}
	}
}
