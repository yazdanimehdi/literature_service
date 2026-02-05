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

	// PapersIngested is the total number of papers saved.
	PapersIngested int

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
	ExpansionRound    int
	MaxExpansionDepth int
}

// LiteratureReviewWorkflow orchestrates an automated literature review.
//
// The workflow proceeds through the following phases:
//  1. Extract keywords from the user query using an LLM
//  2. Search academic databases for papers matching the keywords
//  3. Optionally expand the search by extracting keywords from discovered papers
//  4. Save all results and update the review status
//
// The workflow supports cancellation via the "cancel" signal and progress
// queries via the "progress" query type.
func LiteratureReviewWorkflow(ctx workflow.Context, input ReviewWorkflowInput) (*ReviewWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	startTime := workflow.Now(ctx)

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

	// Activity nil-pointer variables for method references.
	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var ingestionAct *activities.IngestionActivities
	var eventAct *activities.EventActivities
	var dedupAct *activities.DedupActivities

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

	searchCtx := workflow.WithActivityOptions(cancelCtx, workflow.ActivityOptions{
		StartToCloseTimeout: searchActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    1 * time.Minute,
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

	ingestionCtx := workflow.WithActivityOptions(cancelCtx, workflow.ActivityOptions{
		StartToCloseTimeout: ingestionSubmitTimeout,
		HeartbeatTimeout:    2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    1 * time.Minute,
			MaximumAttempts:    3,
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
	var totalPapersSaved int
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
	// Phase 2: Search for papers using extracted keywords
	// =========================================================================

	logger.Info("starting paper search", "keywordCount", len(extractOutput.Keywords))
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

	// Search for each keyword.
	var allPapers []*domain.Paper
	for _, keyword := range extractOutput.Keywords {
		var searchOutput activities.SearchPapersOutput
		err = workflow.ExecuteActivity(searchCtx, searchAct.SearchPapers, activities.SearchPapersInput{
			Query:            keyword,
			Sources:          sources,
			MaxResults:       maxResults,
			IncludePreprints: input.Config.IncludePreprints,
			OpenAccessOnly:   input.Config.RequireOpenAccess,
			MinCitations:     input.Config.MinCitations,
		}).Get(cancelCtx, &searchOutput)
		if err != nil {
			logger.Warn("search failed for keyword, continuing", "keyword", keyword, "error", err)
			continue
		}

		allPapers = append(allPapers, searchOutput.Papers...)
		totalPapersFound += searchOutput.TotalFound
		progress.PapersFound = totalPapersFound
	}

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
		totalPapersSaved += savePapersOutput.SavedCount
	}

	// =========================================================================
	// Phase 3: Expansion rounds
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

		// Search with new keywords.
		var expansionPapersFound []*domain.Paper
		for _, keyword := range newKeywords {
			var expSearchOutput activities.SearchPapersOutput
			err = workflow.ExecuteActivity(searchCtx, searchAct.SearchPapers, activities.SearchPapersInput{
				Query:            keyword,
				Sources:          sources,
				MaxResults:       maxResults,
				IncludePreprints: input.Config.IncludePreprints,
				OpenAccessOnly:   input.Config.RequireOpenAccess,
				MinCitations:     input.Config.MinCitations,
			}).Get(cancelCtx, &expSearchOutput)
			if err != nil {
				logger.Warn("expansion search failed for keyword, continuing",
					"keyword", keyword,
					"round", round,
					"error", err,
				)
				continue
			}

			expansionPapersFound = append(expansionPapersFound, expSearchOutput.Papers...)
			totalPapersFound += expSearchOutput.TotalFound
			progress.PapersFound = totalPapersFound
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
			totalPapersSaved += expSavePapersOutput.SavedCount
		}

		// Append expansion papers for subsequent rounds.
		allPapers = append(allPapers, expansionPapersFound...)
		expansionRounds = round
	}

	// =========================================================================
	// Phase 3.5: Semantic Deduplication
	// =========================================================================

	logger.Info("starting semantic deduplication", "paperCount", len(allPapers))

	// Filter out papers without title (can't meaningfully dedup).
	var papersToDedup []*domain.Paper
	for _, p := range allPapers {
		if p != nil && p.Title != "" {
			papersToDedup = append(papersToDedup, p)
		}
	}

	nonDuplicateIDs := make(map[uuid.UUID]bool)
	if len(papersToDedup) > 0 {
		dedupCtx := workflow.WithActivityOptions(cancelCtx, workflow.ActivityOptions{
			StartToCloseTimeout: 10 * time.Minute,
			HeartbeatTimeout:    2 * time.Minute,
			RetryPolicy: &temporal.RetryPolicy{
				MaximumAttempts: 3,
			},
		})

		var dedupOutput activities.DedupPapersOutput
		err = workflow.ExecuteActivity(dedupCtx, dedupAct.DedupPapers, activities.DedupPapersInput{
			Papers: papersToDedup,
		}).Get(cancelCtx, &dedupOutput)
		if err != nil {
			// Dedup failure is non-fatal: log and treat all papers as non-duplicate.
			logger.Warn("semantic deduplication failed, proceeding without dedup",
				"error", err,
			)
			for _, p := range papersToDedup {
				nonDuplicateIDs[p.ID] = true
			}
		} else {
			for _, id := range dedupOutput.NonDuplicateIDs {
				nonDuplicateIDs[id] = true
			}
			logger.Info("semantic deduplication completed",
				"duplicates", dedupOutput.DuplicateCount,
				"nonDuplicates", len(dedupOutput.NonDuplicateIDs),
				"skipped", dedupOutput.SkippedCount,
			)
		}
	} else {
		// No papers to dedup — all are "non-duplicate".
		for _, p := range allPapers {
			if p != nil {
				nonDuplicateIDs[p.ID] = true
			}
		}
	}

	// =========================================================================
	// Phase 4: Ingestion — Submit papers with PDFs to the ingestion service
	// =========================================================================

	// Collect papers with PDF URLs for ingestion, filtered by dedup results.
	var papersForIngestion []activities.PaperForIngestion
	for _, p := range allPapers {
		if p != nil && p.PDFURL != "" && nonDuplicateIDs[p.ID] {
			papersForIngestion = append(papersForIngestion, activities.PaperForIngestion{
				PaperID:     p.ID,
				PDFURL:      p.PDFURL,
				CanonicalID: p.CanonicalID,
			})
		}
	}

	if len(papersForIngestion) > 0 {
		logger.Info("starting paper ingestion", "paperCount", len(papersForIngestion))
		if err := updateStatus(domain.ReviewStatusIngesting, "ingesting", ""); err != nil {
			return handleFailure(fmt.Errorf("update status to ingesting: %w", err))
		}

		var submitOutput activities.SubmitPapersForIngestionOutput
		err = workflow.ExecuteActivity(ingestionCtx, ingestionAct.SubmitPapersForIngestion, activities.SubmitPapersForIngestionInput{
			OrgID:     input.OrgID,
			ProjectID: input.ProjectID,
			RequestID: input.RequestID,
			Papers:    papersForIngestion,
		}).Get(cancelCtx, &submitOutput)
		if err != nil {
			// Ingestion failure is non-fatal: log it and continue to completion.
			logger.Warn("paper ingestion submission failed, completing with partial results",
				"error", err,
			)
		} else {
			totalPapersSaved += submitOutput.Submitted
			progress.PapersIngested = submitOutput.Submitted
			progress.PapersFailed = submitOutput.Failed

			logger.Info("paper ingestion completed",
				"submitted", submitOutput.Submitted,
				"skipped", submitOutput.Skipped,
				"failed", submitOutput.Failed,
			)
		}
	} else {
		logger.Info("no papers with PDF URLs found, skipping ingestion phase")
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
		PapersIngested:  totalPapersSaved,
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
			"papers_ingested":  totalPapersSaved,
			"expansion_rounds": expansionRounds,
			"duration":         duration,
		},
	}).Get(cancelCtx, nil)

	logger.Info("literature review workflow completed",
		"requestID", input.RequestID,
		"keywordsFound", totalKeywords,
		"papersFound", totalPapersFound,
		"papersIngested", totalPapersSaved,
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
