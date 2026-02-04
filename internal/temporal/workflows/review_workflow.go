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
	"github.com/helixir/literature-review-service/internal/temporal/activities"
)

// Signal and query names for external interaction with the workflow.
const (
	// SignalCancel is the signal name used to request workflow cancellation.
	SignalCancel = "cancel"

	// QueryProgress is the query name used to retrieve workflow progress.
	QueryProgress = "progress"
)

// Activity timeout constants.
const (
	llmActivityTimeout    = 2 * time.Minute
	searchActivityTimeout = 5 * time.Minute
	statusActivityTimeout = 30 * time.Second
)

// maxPapersForExpansion is the maximum number of papers to use for keyword
// expansion in each round.
const maxPapersForExpansion = 5

// ReviewWorkflowInput contains the parameters for starting a literature review workflow.
type ReviewWorkflowInput struct {
	// RequestID is the unique identifier for this review request.
	RequestID uuid.UUID

	// OrgID is the organization identifier for multi-tenancy.
	OrgID string

	// ProjectID is the project identifier for multi-tenancy.
	ProjectID string

	// UserID is the user who initiated the review.
	UserID string

	// Query is the natural language research query.
	Query string

	// Config holds the review configuration parameters.
	Config domain.ReviewConfiguration
}

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
		maxKw = 10
	}

	var extractOutput activities.ExtractKeywordsOutput
	err = workflow.ExecuteActivity(llmCtx, llmAct.ExtractKeywords, activities.ExtractKeywordsInput{
		Text:        input.Query,
		Mode:        "query",
		MaxKeywords: maxKw,
		MinKeywords: 3,
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
		maxResults = 100
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
	// Phase 4: Complete
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
