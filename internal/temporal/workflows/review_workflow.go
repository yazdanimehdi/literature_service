// Package workflows defines Temporal workflow implementations for the
// literature review service pipeline.
package workflows

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/helixir/literature-review-service/internal/domain"
	litemporal "github.com/helixir/literature-review-service/internal/temporal"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
)

// Re-export signal/query name constants from the parent temporal package for
// convenience. These are defined in the parent package so the server layer can
// reference them without depending on the workflows package.
const (
	SignalCancel  = litemporal.SignalCancel
	QueryProgress = litemporal.QueryProgress
)

// Activity timeout constants.
const (
	llmActivityTimeout    = 2 * time.Minute
	searchActivityTimeout = 5 * time.Minute
	statusActivityTimeout = 30 * time.Second

	ingestionSubmitTimeout = 5 * time.Minute
	ingestionPollInterval  = 30 * time.Second
	ingestionMaxPollTime   = 30 * time.Minute
)

// Workflow defaults for keyword extraction and paper search.
const (
	// maxPapersForExpansion is the maximum number of papers to use for keyword
	// expansion in each round.
	maxPapersForExpansion = 5

	// defaultMaxKeywordsPerRound is the default maximum keywords extracted per round
	// when not specified in the configuration.
	defaultMaxKeywordsPerRound = 10

	// defaultMinKeywordsForQuery is the minimum number of keywords to extract from the
	// initial user query.
	defaultMinKeywordsForQuery = 3

	// defaultMaxPapers is the default maximum number of papers to retrieve
	// when not specified in the configuration.
	defaultMaxPapers = 100
)

// Pipeline constants for concurrent processing.
const (
	// batchSize is the number of papers to collect before spawning a child workflow.
	batchSize = 5

	// searchRateLimitDelay is the delay between source searches to avoid rate limiting.
	searchRateLimitDelay = 500 * time.Millisecond
)

// ReviewWorkflowInput is an alias for the shared input type defined in the
// parent temporal package. This allows the workflow function signature to
// remain unchanged while the type is importable from either location.
type ReviewWorkflowInput = litemporal.ReviewWorkflowInput

// ReviewWorkflowResult contains the final results of a literature review workflow.
type ReviewWorkflowResult struct {
	// RequestID is the review request identifier.
	RequestID uuid.UUID

	// Status is the final status of the review.
	Status string

	// KeywordsFound is the total number of unique keywords extracted.
	KeywordsFound int

	// PapersFound is the total number of papers discovered.
	PapersFound int

	// PapersIngested is the total number of papers successfully ingested.
	PapersIngested int

	// DuplicatesFound is the number of papers filtered as duplicates.
	DuplicatesFound int

	// PapersFailed is the number of papers that failed processing.
	PapersFailed int

	// ExpansionRounds is the number of expansion rounds completed.
	ExpansionRounds int

	// Duration is the total workflow execution time in seconds.
	Duration float64
}

// workflowProgress tracks the internal progress state of the workflow, exposed
// via the QueryProgress query handler.
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
}

// LiteratureReviewWorkflow orchestrates an automated literature review using
// a concurrent two-phase pipeline architecture.
//
// The workflow proceeds through the following phases:
//  1. Extract keywords from the user query using an LLM
//  2. Search academic databases concurrently with rate limiting (Phase 1)
//  3. Batch papers (5 papers or 5s timeout) and spawn child workflows (Phase 2)
//  4. Track progress via signals from child workflows
//  5. Optionally expand the search by extracting keywords from discovered papers
//  6. Save all results and update the review status
//
// The workflow supports cancellation via the "cancel" signal and progress
// queries via the "progress" query type.
func LiteratureReviewWorkflow(ctx workflow.Context, input ReviewWorkflowInput) (*ReviewWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	startTime := workflow.Now(ctx)
	workflowInfo := workflow.GetInfo(ctx)

	// Track progress for query handler.
	progress := &workflowProgress{
		Status:            string(domain.ReviewStatusPending),
		Phase:             "initializing",
		MaxExpansionDepth: input.Config.MaxExpansionDepth,
	}

	// Register query handler for progress reporting.
	err := workflow.SetQueryHandler(ctx, QueryProgress, func() (*workflowProgress, error) {
		return progress, nil
	})
	if err != nil {
		logger.Error("failed to register progress query handler", "error", err)
		return nil, fmt.Errorf("register query handler: %w", err)
	}

	// Set up cancellation signal handling.
	cancelCtx, cancelFunc := workflow.WithCancel(ctx)
	signalCh := workflow.GetSignalChannel(ctx, SignalCancel)
	workflow.Go(ctx, func(gCtx workflow.Context) {
		signalCh.Receive(gCtx, nil)
		logger.Info("received cancel signal")
		cancelFunc()
	})

	// Set up batch completion signal handling.
	batchCompleteCh := workflow.GetSignalChannel(ctx, SignalBatchComplete)
	workflow.Go(ctx, func(gCtx workflow.Context) {
		for {
			var signal BatchCompleteSignal
			if !batchCompleteCh.Receive(gCtx, &signal) {
				return // Channel closed
			}
			progress.BatchesCompleted++
			progress.PapersIngested += signal.Ingested
			progress.DuplicatesFound += signal.Duplicates
			progress.PapersFailed += signal.Failed
			logger.Info("batch completed",
				"batchID", signal.BatchID,
				"processed", signal.Processed,
				"duplicates", signal.Duplicates,
				"ingested", signal.Ingested,
				"failed", signal.Failed,
			)
		}
	})

	// Activity nil-pointer variables for method references.
	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Build activity option contexts with retry policies.
	llmCtx := workflow.WithActivityOptions(cancelCtx, workflow.ActivityOptions{
		StartToCloseTimeout: llmActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	})

	statusCtx := workflow.WithActivityOptions(cancelCtx, workflow.ActivityOptions{
		StartToCloseTimeout: statusActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    5,
		},
	})

	eventCtx := workflow.WithActivityOptions(cancelCtx, workflow.ActivityOptions{
		StartToCloseTimeout: statusActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    5,
		},
	})

	// Helper to update status and track in progress.
	updateStatus := func(status domain.ReviewStatus, phase string, errMsg string) error {
		progress.Status = string(status)
		progress.Phase = phase
		return workflow.ExecuteActivity(statusCtx, statusAct.UpdateStatus, activities.UpdateStatusInput{
			OrgID:     input.OrgID,
			ProjectID: input.ProjectID,
			RequestID: input.RequestID,
			Status:    status,
			ErrorMsg:  errMsg,
		}).Get(cancelCtx, nil)
	}

	// handleFailure updates status to failed and returns the original error.
	handleFailure := func(originalErr error) (*ReviewWorkflowResult, error) {
		logger.Error("workflow failed", "error", originalErr)

		// Use the root context for failure status update to avoid cancelled context issues.
		failCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: statusActivityTimeout,
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval:    500 * time.Millisecond,
				BackoffCoefficient: 2.0,
				MaximumInterval:    10 * time.Second,
				MaximumAttempts:    5,
			},
		})
		_ = workflow.ExecuteActivity(failCtx, statusAct.UpdateStatus, activities.UpdateStatusInput{
			OrgID:     input.OrgID,
			ProjectID: input.ProjectID,
			RequestID: input.RequestID,
			Status:    domain.ReviewStatusFailed,
			ErrorMsg:  originalErr.Error(),
		}).Get(ctx, nil)

		// Fire-and-forget: publish review.failed event using root context.
		_ = workflow.ExecuteActivity(failCtx, eventAct.PublishEvent, activities.PublishEventInput{
			EventType: "review.failed",
			RequestID: input.RequestID,
			OrgID:     input.OrgID,
			ProjectID: input.ProjectID,
			Payload: map[string]interface{}{
				"error": originalErr.Error(),
			},
		}).Get(ctx, nil)

		return nil, originalErr
	}

	// Accumulate counters for the result.
	var totalKeywords int
	var totalPapersFound int
	var allKeywords []string

	// =========================================================================
	// Phase 1: Extract keywords from the user query
	// =========================================================================

	logger.Info("starting keyword extraction from query", "query", input.Query)
	if err := updateStatus(domain.ReviewStatusExtractingKeywords, "extracting_keywords", ""); err != nil {
		return handleFailure(fmt.Errorf("update status to extracting_keywords: %w", err))
	}

	maxKw := input.Config.MaxKeywordsPerRound
	if maxKw == 0 {
		maxKw = defaultMaxKeywordsPerRound
	}

	var extractOutput activities.ExtractKeywordsOutput
	err = workflow.ExecuteActivity(llmCtx, llmAct.ExtractKeywords, activities.ExtractKeywordsInput{
		Text:        input.Query,
		Mode:        "query",
		MaxKeywords: maxKw,
		MinKeywords: defaultMinKeywordsForQuery,
	}).Get(cancelCtx, &extractOutput)
	if err != nil {
		return handleFailure(fmt.Errorf("extract_keywords: %w", err))
	}

	allKeywords = append(allKeywords, extractOutput.Keywords...)
	totalKeywords += len(extractOutput.Keywords)
	progress.KeywordsFound = totalKeywords

	logger.Info("keywords extracted", "count", len(extractOutput.Keywords), "keywords", extractOutput.Keywords)

	// Save initial keywords.
	var saveKwOutput activities.SaveKeywordsOutput
	err = workflow.ExecuteActivity(statusCtx, statusAct.SaveKeywords, activities.SaveKeywordsInput{
		RequestID:       input.RequestID,
		Keywords:        extractOutput.Keywords,
		ExtractionRound: 0,
		SourceType:      "query",
	}).Get(cancelCtx, &saveKwOutput)
	if err != nil {
		return handleFailure(fmt.Errorf("save_keywords: %w", err))
	}

	// Fire-and-forget: publish review.started event.
	_ = workflow.ExecuteActivity(eventCtx, eventAct.PublishEvent, activities.PublishEventInput{
		EventType: "review.started",
		RequestID: input.RequestID,
		OrgID:     input.OrgID,
		ProjectID: input.ProjectID,
		Payload: map[string]interface{}{
			"query":         input.Query,
			"keywords":      extractOutput.Keywords,
			"keyword_count": len(extractOutput.Keywords),
		},
	}).Get(cancelCtx, nil)

	// =========================================================================
	// Phase 2: Concurrent paper search with rate limiting
	// =========================================================================

	logger.Info("starting concurrent paper search", "keywordCount", len(extractOutput.Keywords))
	if err := updateStatus(domain.ReviewStatusSearching, "searching", ""); err != nil {
		return handleFailure(fmt.Errorf("update status to searching: %w", err))
	}

	sources := input.Config.Sources
	if len(sources) == 0 {
		sources = []domain.SourceType{
			domain.SourceTypeSemanticScholar,
			domain.SourceTypeOpenAlex,
			domain.SourceTypePubMed,
		}
	}

	maxResults := input.Config.MaxPapers
	if maxResults == 0 {
		maxResults = defaultMaxPapers
	}

	// Calculate results per source to stay within total limit.
	resultsPerSource := maxResults / len(sources)
	if resultsPerSource < 10 {
		resultsPerSource = 10
	}

	// Collect papers from searches.
	var allPapers []*domain.Paper

	// Use a deterministic approach with futures to avoid non-determinism from goroutine
	// completion order. Collect all search futures first, then process results in order.
	type searchFuture struct {
		source  domain.SourceType
		keyword string
		future  workflow.Future
	}
	var searchFutures []searchFuture

	progress.Phase = "searching"
	for _, keyword := range extractOutput.Keywords {
		keyword := keyword // capture
		for idx, source := range sources {
			source := source // capture

			actCtx := workflow.WithActivityOptions(cancelCtx, workflow.ActivityOptions{
				StartToCloseTimeout: searchActivityTimeout,
				RetryPolicy: &temporal.RetryPolicy{
					InitialInterval:    2 * time.Second,
					BackoffCoefficient: 2.0,
					MaximumInterval:    1 * time.Minute,
					MaximumAttempts:    3,
				},
			})

			// Rate limiting: stagger requests to avoid overwhelming sources.
			if idx > 0 {
				_ = workflow.Sleep(cancelCtx, time.Duration(idx)*searchRateLimitDelay)
			}

			future := workflow.ExecuteActivity(actCtx, searchAct.SearchSingleSource, activities.SearchSingleSourceInput{
				Source:           source,
				Query:            keyword,
				MaxResults:       resultsPerSource,
				IncludePreprints: input.Config.IncludePreprints,
				OpenAccessOnly:   input.Config.RequireOpenAccess,
				MinCitations:     input.Config.MinCitations,
			})

			searchFutures = append(searchFutures, searchFuture{source: source, keyword: keyword, future: future})
		}
	}

	// Process results in deterministic order.
	for _, sf := range searchFutures {
		var output activities.SearchSingleSourceOutput
		err := sf.future.Get(cancelCtx, &output)
		if err != nil {
			logger.Warn("search failed for source",
				"source", sf.source,
				"keyword", sf.keyword,
				"error", err,
			)
			continue
		}

		if output.Error != "" {
			logger.Warn("search returned error",
				"source", sf.source,
				"keyword", sf.keyword,
				"error", output.Error,
			)
			continue
		}

		if len(output.Papers) > 0 {
			allPapers = append(allPapers, output.Papers...)
			totalPapersFound += output.TotalFound
			progress.PapersFound = totalPapersFound
			logger.Info("search completed",
				"source", sf.source,
				"keyword", sf.keyword,
				"papers", len(output.Papers),
			)
		}
	}
	logger.Info("all searches completed", "totalPapers", len(allPapers))

	// Save discovered papers.
	if len(allPapers) > 0 {
		var savePapersOutput activities.SavePapersOutput
		err = workflow.ExecuteActivity(statusCtx, statusAct.SavePapers, activities.SavePapersInput{
			RequestID:           input.RequestID,
			OrgID:               input.OrgID,
			ProjectID:           input.ProjectID,
			Papers:              allPapers,
			DiscoveredViaSource: sources[0],
			ExpansionDepth:      0,
		}).Get(cancelCtx, &savePapersOutput)
		if err != nil {
			return handleFailure(fmt.Errorf("save_papers: %w", err))
		}
		logger.Info("papers saved", "saved", savePapersOutput.SavedCount, "duplicates", savePapersOutput.DuplicateCount)
	}

	// =========================================================================
	// Phase 3: Batch processing via child workflows
	// =========================================================================

	logger.Info("starting batch processing", "paperCount", len(allPapers))
	if err := updateStatus(domain.ReviewStatusIngesting, "processing", ""); err != nil {
		return handleFailure(fmt.Errorf("update status to processing: %w", err))
	}

	// Convert papers to processing format.
	papersForProcessing := make([]PaperForProcessing, 0, len(allPapers))
	for _, p := range allPapers {
		if p == nil {
			continue
		}
		papersForProcessing = append(papersForProcessing, PaperForProcessing{
			PaperID:     p.ID,
			CanonicalID: p.CanonicalID,
			Title:       p.Title,
			Abstract:    p.Abstract,
			PDFURL:      p.PDFURL,
			Authors:     extractAuthors(p),
		})
	}

	// Spawn child workflows for batches.
	// Capture all child workflow futures to avoid memory leaks.
	var childFutures []workflow.ChildWorkflowFuture
	progress.Phase = "batching"
	if len(papersForProcessing) > 0 {
		batches := createBatches(papersForProcessing, batchSize)
		for i, batch := range batches {
			batchID := fmt.Sprintf("%s-batch-%d", workflowInfo.WorkflowExecution.ID, i)

			childCtx := workflow.WithChildOptions(cancelCtx, workflow.ChildWorkflowOptions{
				WorkflowID: batchID,
				TaskQueue:  workflowInfo.TaskQueueName,
				RetryPolicy: &temporal.RetryPolicy{
					InitialInterval:    2 * time.Second,
					BackoffCoefficient: 2.0,
					MaximumInterval:    1 * time.Minute,
					MaximumAttempts:    3,
				},
			})

			future := workflow.ExecuteChildWorkflow(childCtx, PaperProcessingWorkflow, PaperProcessingInput{
				OrgID:            input.OrgID,
				ProjectID:        input.ProjectID,
				RequestID:        input.RequestID.String(),
				Batch:            PaperBatch{BatchID: batchID, Papers: batch},
				ParentWorkflowID: workflowInfo.WorkflowExecution.ID,
			})
			childFutures = append(childFutures, future)

			progress.BatchesSpawned++
			logger.Info("spawned batch workflow",
				"batchID", batchID,
				"paperCount", len(batch),
			)
		}
	}

	// Wait for all batches to complete.
	if progress.BatchesSpawned > 0 {
		logger.Info("waiting for batch completion",
			"batchesSpawned", progress.BatchesSpawned,
		)
		err = workflow.Await(cancelCtx, func() bool {
			return progress.BatchesCompleted >= progress.BatchesSpawned
		})
		if err != nil {
			return handleFailure(fmt.Errorf("await batch completion: %w", err))
		}
		logger.Info("all batches completed",
			"ingested", progress.PapersIngested,
			"duplicates", progress.DuplicatesFound,
			"failed", progress.PapersFailed,
		)
	}

	// =========================================================================
	// Phase 4: Expansion rounds (optional)
	// =========================================================================

	expansionRounds := 0
	for round := 1; round <= input.Config.MaxExpansionDepth; round++ {
		// Check if we have reached the paper limit.
		if totalPapersFound >= input.Config.MaxPapers && input.Config.MaxPapers > 0 {
			logger.Info("paper limit reached, stopping expansion",
				"totalPapersFound", totalPapersFound,
				"maxPapers", input.Config.MaxPapers,
			)
			break
		}

		logger.Info("starting expansion round", "round", round)
		if err := updateStatus(domain.ReviewStatusExpanding, "expanding", ""); err != nil {
			return handleFailure(fmt.Errorf("update status to expanding: %w", err))
		}

		progress.ExpansionRound = round
		progress.Phase = "expanding"

		// Select top papers with abstracts for keyword expansion.
		expansionPapers := selectPapersForExpansion(allPapers, maxPapersForExpansion)
		if len(expansionPapers) == 0 {
			logger.Info("no papers with abstracts available for expansion, stopping")
			break
		}

		// Extract keywords from each expansion paper.
		var newKeywords []string
		for _, paper := range expansionPapers {
			var expExtractOutput activities.ExtractKeywordsOutput
			err = workflow.ExecuteActivity(llmCtx, llmAct.ExtractKeywords, activities.ExtractKeywordsInput{
				Text:             paper.Abstract,
				Mode:             "abstract",
				MaxKeywords:      maxKw,
				MinKeywords:      1,
				ExistingKeywords: allKeywords,
				Context:          input.Query,
			}).Get(cancelCtx, &expExtractOutput)
			if err != nil {
				logger.Warn("expansion keyword extraction failed, skipping paper",
					"paperTitle", paper.Title,
					"error", err,
				)
				continue
			}

			newKeywords = append(newKeywords, expExtractOutput.Keywords...)
		}

		if len(newKeywords) == 0 {
			logger.Info("no new keywords from expansion, stopping", "round", round)
			break
		}

		allKeywords = append(allKeywords, newKeywords...)
		totalKeywords += len(newKeywords)
		progress.KeywordsFound = totalKeywords

		// Save expansion keywords.
		var expSaveKwOutput activities.SaveKeywordsOutput
		err = workflow.ExecuteActivity(statusCtx, statusAct.SaveKeywords, activities.SaveKeywordsInput{
			RequestID:       input.RequestID,
			Keywords:        newKeywords,
			ExtractionRound: round,
			SourceType:      "llm_extraction",
		}).Get(cancelCtx, &expSaveKwOutput)
		if err != nil {
			return handleFailure(fmt.Errorf("save_expansion_keywords round %d: %w", round, err))
		}

		// Use deterministic approach with futures to avoid non-determinism from goroutine
		// completion order. Collect all expansion search futures first, then process results in order.
		type expSearchFuture struct {
			source  domain.SourceType
			keyword string
			future  workflow.Future
		}
		var expSearchFutures []expSearchFuture

		for _, keyword := range newKeywords {
			keyword := keyword
			for idx, source := range sources {
				source := source

				actCtx := workflow.WithActivityOptions(cancelCtx, workflow.ActivityOptions{
					StartToCloseTimeout: searchActivityTimeout,
					RetryPolicy: &temporal.RetryPolicy{
						InitialInterval:    2 * time.Second,
						BackoffCoefficient: 2.0,
						MaximumInterval:    1 * time.Minute,
						MaximumAttempts:    3,
					},
				})

				// Rate limiting: stagger requests to avoid overwhelming sources.
				if idx > 0 {
					_ = workflow.Sleep(cancelCtx, time.Duration(idx)*searchRateLimitDelay)
				}

				future := workflow.ExecuteActivity(actCtx, searchAct.SearchSingleSource, activities.SearchSingleSourceInput{
					Source:           source,
					Query:            keyword,
					MaxResults:       resultsPerSource,
					IncludePreprints: input.Config.IncludePreprints,
					OpenAccessOnly:   input.Config.RequireOpenAccess,
					MinCitations:     input.Config.MinCitations,
				})

				expSearchFutures = append(expSearchFutures, expSearchFuture{source: source, keyword: keyword, future: future})
			}
		}

		// Process expansion results in deterministic order.
		var expansionPapersFound []*domain.Paper
		for _, sf := range expSearchFutures {
			var output activities.SearchSingleSourceOutput
			err := sf.future.Get(cancelCtx, &output)
			if err != nil || output.Error != "" {
				continue
			}

			if len(output.Papers) > 0 {
				expansionPapersFound = append(expansionPapersFound, output.Papers...)
				totalPapersFound += output.TotalFound
				progress.PapersFound = totalPapersFound
			}
		}

		// Save expansion papers.
		if len(expansionPapersFound) > 0 {
			var expSavePapersOutput activities.SavePapersOutput
			err = workflow.ExecuteActivity(statusCtx, statusAct.SavePapers, activities.SavePapersInput{
				RequestID:           input.RequestID,
				OrgID:               input.OrgID,
				ProjectID:           input.ProjectID,
				Papers:              expansionPapersFound,
				DiscoveredViaSource: sources[0],
				ExpansionDepth:      round,
			}).Get(cancelCtx, &expSavePapersOutput)
			if err != nil {
				return handleFailure(fmt.Errorf("save_expansion_papers round %d: %w", round, err))
			}

			// Spawn batch processing for expansion papers.
			expPapersForProcessing := make([]PaperForProcessing, 0, len(expansionPapersFound))
			for _, p := range expansionPapersFound {
				if p == nil {
					continue
				}
				expPapersForProcessing = append(expPapersForProcessing, PaperForProcessing{
					PaperID:     p.ID,
					CanonicalID: p.CanonicalID,
					Title:       p.Title,
					Abstract:    p.Abstract,
					PDFURL:      p.PDFURL,
					Authors:     extractAuthors(p),
				})
			}

			expBatches := createBatches(expPapersForProcessing, batchSize)
			for i, batch := range expBatches {
				batchID := fmt.Sprintf("%s-exp%d-batch-%d", workflowInfo.WorkflowExecution.ID, round, i)

				childCtx := workflow.WithChildOptions(cancelCtx, workflow.ChildWorkflowOptions{
					WorkflowID: batchID,
					TaskQueue:  workflowInfo.TaskQueueName,
					RetryPolicy: &temporal.RetryPolicy{
						InitialInterval:    2 * time.Second,
						BackoffCoefficient: 2.0,
						MaximumInterval:    1 * time.Minute,
						MaximumAttempts:    3,
					},
				})

				future := workflow.ExecuteChildWorkflow(childCtx, PaperProcessingWorkflow, PaperProcessingInput{
					OrgID:            input.OrgID,
					ProjectID:        input.ProjectID,
					RequestID:        input.RequestID.String(),
					Batch:            PaperBatch{BatchID: batchID, Papers: batch},
					ParentWorkflowID: workflowInfo.WorkflowExecution.ID,
				})
				childFutures = append(childFutures, future)

				progress.BatchesSpawned++
			}

			// Wait for expansion batch completion.
			if len(expBatches) > 0 {
				err = workflow.Await(cancelCtx, func() bool {
					return progress.BatchesCompleted >= progress.BatchesSpawned
				})
				if err != nil {
					return handleFailure(fmt.Errorf("await expansion batch completion: %w", err))
				}
			}
		}

		// Append expansion papers for subsequent rounds.
		allPapers = append(allPapers, expansionPapersFound...)
		expansionRounds = round
	}

	// =========================================================================
	// Phase 5: Complete
	// =========================================================================

	if err := updateStatus(domain.ReviewStatusCompleted, "completed", ""); err != nil {
		return handleFailure(fmt.Errorf("update status to completed: %w", err))
	}

	duration := workflow.Now(ctx).Sub(startTime).Seconds()

	result := &ReviewWorkflowResult{
		RequestID:       input.RequestID,
		Status:          string(domain.ReviewStatusCompleted),
		KeywordsFound:   totalKeywords,
		PapersFound:     totalPapersFound,
		PapersIngested:  progress.PapersIngested,
		DuplicatesFound: progress.DuplicatesFound,
		PapersFailed:    progress.PapersFailed,
		ExpansionRounds: expansionRounds,
		Duration:        duration,
	}

	// Fire-and-forget: publish review.completed event.
	_ = workflow.ExecuteActivity(eventCtx, eventAct.PublishEvent, activities.PublishEventInput{
		EventType: "review.completed",
		RequestID: input.RequestID,
		OrgID:     input.OrgID,
		ProjectID: input.ProjectID,
		Payload: map[string]interface{}{
			"keywords_found":   totalKeywords,
			"papers_found":     totalPapersFound,
			"papers_ingested":  progress.PapersIngested,
			"duplicates_found": progress.DuplicatesFound,
			"papers_failed":    progress.PapersFailed,
			"expansion_rounds": expansionRounds,
			"duration":         duration,
		},
	}).Get(cancelCtx, nil)

	logger.Info("literature review workflow completed",
		"requestID", input.RequestID,
		"keywordsFound", totalKeywords,
		"papersFound", totalPapersFound,
		"papersIngested", progress.PapersIngested,
		"duplicatesFound", progress.DuplicatesFound,
		"papersFailed", progress.PapersFailed,
		"expansionRounds", expansionRounds,
		"duration", duration,
	)

	return result, nil
}

// selectPapersForExpansion returns up to max papers that have non-empty abstracts,
// suitable for keyword extraction during expansion rounds.
func selectPapersForExpansion(papers []*domain.Paper, max int) []*domain.Paper {
	var selected []*domain.Paper
	for _, p := range papers {
		if len(selected) >= max {
			break
		}
		if p != nil && p.Abstract != "" {
			selected = append(selected, p)
		}
	}
	return selected
}

// createBatches splits papers into batches of the specified size.
func createBatches(papers []PaperForProcessing, size int) [][]PaperForProcessing {
	if len(papers) == 0 {
		return nil
	}

	var batches [][]PaperForProcessing
	for i := 0; i < len(papers); i += size {
		end := i + size
		if end > len(papers) {
			end = len(papers)
		}
		batches = append(batches, papers[i:end])
	}
	return batches
}

// extractAuthors extracts author names from a paper.
func extractAuthors(p *domain.Paper) []string {
	if p == nil || len(p.Authors) == 0 {
		return nil
	}
	names := make([]string, 0, len(p.Authors))
	for _, a := range p.Authors {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return names
}
