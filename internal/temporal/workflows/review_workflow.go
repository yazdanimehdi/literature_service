// Package workflows defines Temporal workflow implementations for the
// literature review service pipeline.
package workflows

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/helixir/literature-review-service/internal/domain"
	litemporal "github.com/helixir/literature-review-service/internal/temporal"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
	"github.com/helixir/literature-review-service/internal/temporal/resilience"
)

// Re-export signal/query name constants from the parent temporal package for
// convenience. These are defined in the parent package so the server layer can
// reference them without depending on the workflows package.
const (
	SignalCancel  = litemporal.SignalCancel
	SignalPause   = litemporal.SignalPause
	SignalResume  = litemporal.SignalResume
	SignalStop    = litemporal.SignalStop
	QueryProgress = litemporal.QueryProgress
)

// Activity timeout defaults. These are used when the workflow input does not
// provide explicit timeout values (i.e., the timeout fields are zero-valued).
const (
	defaultLLMActivityTimeout    = 2 * time.Minute
	defaultSearchActivityTimeout = 5 * time.Minute
	defaultStatusActivityTimeout = 30 * time.Second

	defaultIngestionSubmitTimeout = 5 * time.Minute
	ingestionPollInterval         = 30 * time.Second
	ingestionMaxPollTime          = 30 * time.Minute
)

// resolveTimeouts returns effective timeout values, using the input overrides
// when non-zero and falling back to defaults otherwise.
func resolveTimeouts(input ReviewWorkflowInput) (llmTimeout, searchTimeout, statusTimeout, ingestionSubmitTimeout time.Duration) {
	llmTimeout = defaultLLMActivityTimeout
	if input.Timeouts.LLMActivity > 0 {
		llmTimeout = input.Timeouts.LLMActivity
	}
	searchTimeout = defaultSearchActivityTimeout
	if input.Timeouts.SearchActivity > 0 {
		searchTimeout = input.Timeouts.SearchActivity
	}
	statusTimeout = defaultStatusActivityTimeout
	if input.Timeouts.StatusActivity > 0 {
		statusTimeout = input.Timeouts.StatusActivity
	}
	ingestionSubmitTimeout = defaultIngestionSubmitTimeout
	if input.Timeouts.IngestionSubmitActivity > 0 {
		ingestionSubmitTimeout = input.Timeouts.IngestionSubmitActivity
	}
	return
}

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

// queryText builds a combined text from title and description for LLM extraction and logging.
func queryText(title, description string) string {
	if description != "" {
		return title + "\n" + description
	}
	return title
}

// extractPaperIDs collects non-nil paper UUIDs from a slice of papers.
func extractPaperIDs(papers []*domain.Paper) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(papers))
	for _, p := range papers {
		if p != nil {
			ids = append(ids, p.ID)
		}
	}
	return ids
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

	// PapersIngested is the total number of papers successfully ingested.
	PapersIngested int

	// DuplicatesFound is the number of papers filtered as duplicates.
	DuplicatesFound int

	// PapersFailed is the number of papers that failed processing.
	PapersFailed int

	// ExpansionRounds is the number of expansion rounds completed.
	ExpansionRounds int

	// KeywordsFilteredByRelevance is the count of keywords dropped by the relevance gate.
	KeywordsFilteredByRelevance int

	// CoverageScore is the LLM-assessed coverage score (0.0-1.0).
	CoverageScore float64

	// CoverageReasoning is the LLM's explanation of the coverage assessment.
	CoverageReasoning string

	// GapTopics are research subtopics the LLM identified as under-represented.
	GapTopics []string

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
	BatchesSpawned              int
	BatchesCompleted            int
	ExpansionRound              int
	MaxExpansionDepth           int
	KeywordsFilteredByRelevance int
	CoverageScore               float64
	CoverageReasoning           string

	// Pause state
	IsPaused      bool
	PauseReason   domain.PauseReason
	PausedAt      time.Time
	PausedAtPhase string

	// Retry state (populated by resilience.ExecutePhase)
	RetryAttempt   int
	RetryPhase     string
	LastRetryError string

	// Degradation state
	IsDegraded    bool
	DegradedPhase string
}

// LiteratureReviewWorkflow orchestrates an automated literature review using
// a concurrent two-phase pipeline architecture.
//
// The workflow proceeds through the following phases:
//  1. Extract keywords from the research title/description using an LLM
//  2. Embed the query text for relevance gating (if enabled)
//  3. Search academic databases concurrently with rate limiting
//  4. Batch papers and spawn child workflows for dedup, ingestion, and embedding
//  5. Optionally expand the search by extracting keywords from discovered papers,
//     filtering expansion keywords via embedding-based relevance gate
//  6. Assess corpus coverage via LLM (if enabled), triggering gap-based expansion
//     rounds when coverage is below threshold
//  7. Save all results and update the review status
//
// The workflow supports pause/resume/stop/cancel signals and progress queries.
// Budget exhaustion during LLM calls triggers automatic pause with resume on refill.
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

	// Set up pause/resume/stop signal channels.
	pauseCh := workflow.GetSignalChannel(ctx, SignalPause)
	resumeCh := workflow.GetSignalChannel(ctx, SignalResume)
	stopCh := workflow.GetSignalChannel(ctx, SignalStop)

	var stopRequested bool

	// Pause signal handler goroutine - listens for pause signals.
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

	// Stop signal handler goroutine - listens for stop signals.
	workflow.Go(ctx, func(gCtx workflow.Context) {
		var signal StopSignal
		if !stopCh.Receive(gCtx, &signal) {
			return
		}
		stopRequested = true
		logger.Info("stop requested, will complete current phase", "reason", signal.Reason)
	})

	// Activity nil-pointer variables for method references.
	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities
	var embeddingAct *activities.EmbeddingActivities

	// Resolve configurable timeouts from input, falling back to defaults.
	llmActivityTimeout, searchActivityTimeout, statusActivityTimeout, _ := resolveTimeouts(input)

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

	// llmPhaseCtx disables Temporal-level retries for LLM activities wrapped by
	// resilience.ExecutePhase, which handles its own retry loop. This prevents
	// multiplicative retry amplification (Temporal retries × phase retries).
	llmPhaseCtx := workflow.WithActivityOptions(cancelCtx, workflow.ActivityOptions{
		StartToCloseTimeout: llmActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
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

	// Build search executor for shared search helpers.
	executor := &searchExecutor{
		cancelCtx:             cancelCtx,
		statusCtx:             statusCtx,
		searchAct:             searchAct,
		statusAct:             statusAct,
		logger:                logger,
		progress:              progress,
		input:                 input,
		wfInfo:                workflowInfo,
		searchActivityTimeout: searchActivityTimeout,
	}

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
	// Error messages stored in DB and published to events are sanitized to avoid
	// leaking internal details (stack traces, connection strings, file paths).
	handleFailure := func(originalErr error) (*ReviewWorkflowResult, error) {
		logger.Error("workflow failed", "error", originalErr)

		// Sanitize: use a generic message for persistence and events.
		// The detailed error is already logged above and returned to Temporal.
		sanitizedMsg := "workflow failed"

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
			ErrorMsg:  sanitizedMsg,
		}).Get(ctx, nil)

		// Fire-and-forget: publish review.failed event using root context.
		_ = workflow.ExecuteActivity(failCtx, eventAct.PublishEvent, activities.PublishEventInput{
			EventType: "review.failed",
			RequestID: input.RequestID,
			OrgID:     input.OrgID,
			ProjectID: input.ProjectID,
			Payload: map[string]interface{}{
				"error": sanitizedMsg,
			},
		}).Get(ctx, nil)

		return nil, originalErr
	}

	// Accumulate counters for the result.
	var totalKeywords int
	var totalPapersFound int
	var allKeywords []string

	// Phase configurations for resilience.
	phaseConfigs := resilience.DefaultPhaseConfigs()

	// Retry progress bridge — maps resilience.Progress to workflowProgress fields.
	retryProgress := &resilience.Progress{}
	syncRetryProgress := func() {
		progress.RetryAttempt = retryProgress.RetryAttempt
		progress.RetryPhase = retryProgress.RetryPhase
		progress.LastRetryError = retryProgress.LastRetryError
	}

	// handleDegradation marks the workflow as degraded/partial and continues.
	handleDegradation := func(phase string, err error) {
		progress.IsDegraded = true
		progress.DegradedPhase = phase
		logger.Warn("phase degraded, continuing with partial results",
			"phase", phase,
			"error", err,
		)
	}

	// Track keyword string → DB UUID for search deduplication.
	keywordIDMap := make(map[string]uuid.UUID)

	// =========================================================================
	// Phase 1: Extract keywords from the user query
	// =========================================================================

	logger.Info("starting keyword extraction", "title", input.Title)
	if err := updateStatus(domain.ReviewStatusExtractingKeywords, "extracting_keywords", ""); err != nil {
		return handleFailure(fmt.Errorf("update status to extracting_keywords: %w", err))
	}

	// Add user-provided seed keywords first.
	if len(input.SeedKeywords) > 0 {
		allKeywords = append(allKeywords, input.SeedKeywords...)
		totalKeywords += len(input.SeedKeywords)
		progress.KeywordsFound = totalKeywords

		// Save seed keywords.
		var seedKwOutput activities.SaveKeywordsOutput
		err = workflow.ExecuteActivity(statusCtx, statusAct.SaveKeywords, activities.SaveKeywordsInput{
			RequestID:       input.RequestID,
			Keywords:        input.SeedKeywords,
			ExtractionRound: 0,
			SourceType:      "user_provided",
		}).Get(cancelCtx, &seedKwOutput)
		if err != nil {
			return handleFailure(fmt.Errorf("save_seed_keywords: %w", err))
		}
		for k, v := range seedKwOutput.KeywordIDMap {
			keywordIDMap[k] = v
		}
	}

	maxKw := input.Config.MaxKeywordsPerRound
	if maxKw == 0 {
		maxKw = defaultMaxKeywordsPerRound
	}

	var extractOutput activities.ExtractKeywordsOutput
	phaseResult := resilience.ExecutePhase(cancelCtx, phaseConfigs["extracting_keywords"], retryProgress, func() error {
		return workflow.ExecuteActivity(llmPhaseCtx, llmAct.ExtractKeywords, activities.ExtractKeywordsInput{
			Text:             queryText(input.Title, input.Description),
			Mode:             "query",
			MaxKeywords:      maxKw,
			MinKeywords:      defaultMinKeywordsForQuery,
			ExistingKeywords: input.SeedKeywords,
		}).Get(cancelCtx, &extractOutput)
	})
	syncRetryProgress()
	if phaseResult.PausedForBudget {
		if err := checkPausePoint(ctx, progress, resumeCh, statusAct, input, logger); err != nil {
			return handleFailure(err)
		}
		// Re-run after budget resume — recursive call through the same phase executor.
		phaseResult = resilience.ExecutePhase(cancelCtx, phaseConfigs["extracting_keywords"], retryProgress, func() error {
			return workflow.ExecuteActivity(llmPhaseCtx, llmAct.ExtractKeywords, activities.ExtractKeywordsInput{
				Text:             queryText(input.Title, input.Description),
				Mode:             "query",
				MaxKeywords:      maxKw,
				MinKeywords:      defaultMinKeywordsForQuery,
				ExistingKeywords: input.SeedKeywords,
			}).Get(cancelCtx, &extractOutput)
		})
		syncRetryProgress()
	}
	if phaseResult.Failed {
		return handleFailure(phaseResult.Err)
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
	for k, v := range saveKwOutput.KeywordIDMap {
		keywordIDMap[k] = v
	}

	// Embed query for relevance gate.
	var queryEmbedding []float32
	if input.Config.EnableRelevanceGate {
		var embedOutput activities.EmbedTextOutput
		err = workflow.ExecuteActivity(llmCtx, embeddingAct.EmbedText, activities.EmbedTextInput{
			Text: queryText(input.Title, input.Description),
		}).Get(cancelCtx, &embedOutput)
		if err != nil {
			logger.Warn("failed to embed query for relevance gate, disabling for this run", "error", err)
			input.Config.EnableRelevanceGate = false
		} else {
			queryEmbedding = embedOutput.Embedding
		}
	}

	// Fire-and-forget: publish review.started event.
	_ = workflow.ExecuteActivity(eventCtx, eventAct.PublishEvent, activities.PublishEventInput{
		EventType: "review.started",
		RequestID: input.RequestID,
		OrgID:     input.OrgID,
		ProjectID: input.ProjectID,
		Payload: map[string]interface{}{
			"title":         input.Title,
			"keywords":      extractOutput.Keywords,
			"keyword_count": len(extractOutput.Keywords),
		},
	}).Get(cancelCtx, nil)

	// Check for pause/stop after keyword extraction.
	if err := checkPausePoint(ctx, progress, resumeCh, statusAct, input, logger); err != nil {
		return handleFailure(err)
	}
	if shouldReturn, result, _ := checkStopPoint(ctx, stopRequested, progress, input, startTime, totalKeywords, totalPapersFound, 0, statusAct, eventAct, logger); shouldReturn {
		return result, nil
	}

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

	// Bounded candidate papers for expansion/coverage selection.
	var candidatePapers []*domain.Paper

	progress.Phase = "searching"
	searchResult := executor.executeSearch(searchPhaseParams{
		keywords:         extractOutput.Keywords,
		keywordIDMap:     keywordIDMap,
		sources:          sources,
		resultsPerSource: resultsPerSource,
		logPrefix:        "initial",
	})
	totalPapersFound += searchResult.totalFound

	// Check for pause/stop after searches complete.
	if err := checkPausePoint(ctx, progress, resumeCh, statusAct, input, logger); err != nil {
		return handleFailure(err)
	}
	if shouldReturn, result, _ := checkStopPoint(ctx, stopRequested, progress, input, startTime, totalKeywords, totalPapersFound, 0, statusAct, eventAct, logger); shouldReturn {
		return result, nil
	}

	// =========================================================================
	// Phase 3: Save papers and batch processing via child workflows
	// =========================================================================

	logger.Info("starting batch processing", "paperCount", len(searchResult.papers))
	if err := updateStatus(domain.ReviewStatusIngesting, "processing", ""); err != nil {
		return handleFailure(fmt.Errorf("update status to processing: %w", err))
	}

	// Capture all child workflow futures to avoid memory leaks.
	var childFutures []workflow.ChildWorkflowFuture
	progress.Phase = "batching"

	discoveredVia := sources[0]
	if len(sources) > 1 {
		discoveredVia = "multiple"
	}
	if err := executor.saveAndSpawnBatches(saveAndSpawnParams{
		papers:              searchResult.papers,
		keywordPaperEntries: searchResult.keywordPaperEntries,
		discoveredVia:       discoveredVia,
		expansionDepth:      0,
		batchIDPrefix:       workflowInfo.WorkflowExecution.ID,
		fatalOnSaveError:    true,
		childFutures:        &childFutures,
	}); err != nil {
		return handleFailure(fmt.Errorf("save_papers: %w", err))
	}
	appendCandidates(&candidatePapers, searchResult.papers, maxCandidatePapers)

	// Check for pause/stop after batch processing.
	if err := checkPausePoint(ctx, progress, resumeCh, statusAct, input, logger); err != nil {
		return handleFailure(err)
	}
	if shouldReturn, result, _ := checkStopPoint(ctx, stopRequested, progress, input, startTime, totalKeywords, totalPapersFound, 0, statusAct, eventAct, logger); shouldReturn {
		return result, nil
	}

	// =========================================================================
	// Phase 4: Expansion rounds (optional)
	// =========================================================================

	expansionRounds := 0
	for round := 1; round <= input.Config.MaxExpansionDepth; round++ {
		// Check for pause/stop at start of expansion round.
		if err := checkPausePoint(ctx, progress, resumeCh, statusAct, input, logger); err != nil {
			return handleFailure(err)
		}
		if shouldReturn, result, _ := checkStopPoint(ctx, stopRequested, progress, input, startTime, totalKeywords, totalPapersFound, expansionRounds, statusAct, eventAct, logger); shouldReturn {
			return result, nil
		}

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
		expansionPapers := selectPapersForExpansion(candidatePapers, maxPapersForExpansion)
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
				Context:          queryText(input.Title, input.Description),
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

		// Relevance gate: filter drifting keywords.
		if input.Config.EnableRelevanceGate && len(queryEmbedding) > 0 && len(newKeywords) > 0 {
			var scoreOutput activities.ScoreKeywordRelevanceOutput
			err = workflow.ExecuteActivity(llmCtx, embeddingAct.ScoreKeywordRelevance, activities.ScoreKeywordRelevanceInput{
				Keywords:       newKeywords,
				QueryEmbedding: queryEmbedding,
				Threshold:      input.Config.RelevanceThreshold,
			}).Get(cancelCtx, &scoreOutput)
			if err != nil {
				logger.Warn("relevance scoring failed, using all keywords", "round", round, "error", err)
			} else {
				filtered := make([]string, 0, len(scoreOutput.Accepted))
				for _, kw := range scoreOutput.Accepted {
					filtered = append(filtered, kw.Keyword)
				}
				logger.Info("relevance gate applied",
					"round", round,
					"accepted", len(scoreOutput.Accepted),
					"rejected", len(scoreOutput.Rejected),
				)
				progress.KeywordsFilteredByRelevance += len(scoreOutput.Rejected)
				newKeywords = filtered

				if len(newKeywords) == 0 {
					logger.Info("all expansion keywords filtered by relevance gate, stopping", "round", round)
					break
				}
			}
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
		for k, v := range expSaveKwOutput.KeywordIDMap {
			keywordIDMap[k] = v
		}

		expSearchResult := executor.executeSearch(searchPhaseParams{
			keywords:         newKeywords,
			keywordIDMap:     keywordIDMap,
			sources:          sources,
			resultsPerSource: resultsPerSource,
			logPrefix:        fmt.Sprintf("expansion round %d", round),
		})
		totalPapersFound += expSearchResult.totalFound

		expDiscoveredVia := sources[0]
		if len(sources) > 1 {
			expDiscoveredVia = "multiple"
		}
		if err := executor.saveAndSpawnBatches(saveAndSpawnParams{
			papers:              expSearchResult.papers,
			keywordPaperEntries: expSearchResult.keywordPaperEntries,
			discoveredVia:       expDiscoveredVia,
			expansionDepth:      round,
			batchIDPrefix:       fmt.Sprintf("%s-exp%d", workflowInfo.WorkflowExecution.ID, round),
			fatalOnSaveError:    true,
			childFutures:        &childFutures,
		}); err != nil {
			return handleFailure(fmt.Errorf("save_expansion_papers round %d: %w", round, err))
		}
		appendCandidates(&candidatePapers, expSearchResult.papers, maxCandidatePapers)

		expansionRounds = round
	}

	// =========================================================================
	// Phase 5: Coverage Review (optional)
	// =========================================================================

	var coverageScore float64
	var coverageReasoning string
	var gapTopics []string
	var gapExpansionTriggered bool

	if input.Config.EnableCoverageReview {
		if err := checkPausePoint(ctx, progress, resumeCh, statusAct, input, logger); err != nil {
			return handleFailure(err)
		}
		if shouldReturn, result, _ := checkStopPoint(ctx, stopRequested, progress, input, startTime, totalKeywords, totalPapersFound, expansionRounds, statusAct, eventAct, logger); shouldReturn {
			return result, nil
		}

		logger.Info("starting coverage review")
		if err := updateStatus(domain.ReviewStatusReviewing, "reviewing", ""); err != nil {
			return handleFailure(fmt.Errorf("update status to reviewing: %w", err))
		}
		progress.Phase = "reviewing"

		// Select up to 20 papers for assessment.
		assessmentPapers := selectPapersForExpansion(candidatePapers, 20)
		summaries := make([]activities.PaperSummary, 0, len(assessmentPapers))
		for _, p := range assessmentPapers {
			abstract := p.Abstract
			if len(abstract) > 500 {
				abstract = abstract[:500]
			}
			summaries = append(summaries, activities.PaperSummary{
				Title:    p.Title,
				Abstract: abstract,
			})
		}

		var coverageOutput activities.AssessCoverageOutput
		coveragePhaseResult := resilience.ExecutePhase(cancelCtx, phaseConfigs["reviewing"], retryProgress, func() error {
			return workflow.ExecuteActivity(llmPhaseCtx, llmAct.AssessCoverage, activities.AssessCoverageInput{
				Title:           input.Title,
				Description:     input.Description,
				SeedKeywords:    input.SeedKeywords,
				AllKeywords:     allKeywords,
				PaperSummaries:  summaries,
				TotalPapers:     totalPapersFound,
				ExpansionRounds: expansionRounds,
			}).Get(cancelCtx, &coverageOutput)
		})
		syncRetryProgress()

		if coveragePhaseResult.Failed || coveragePhaseResult.Degraded || coveragePhaseResult.Skipped {
			// Non-fatal: complete without score (reviewing is NonCritical).
			handleDegradation("reviewing", coveragePhaseResult.Err)
		} else {
			coverageScore = coverageOutput.CoverageScore
			coverageReasoning = coverageOutput.Reasoning
			gapTopics = coverageOutput.GapTopics
			progress.CoverageScore = coverageScore
			progress.CoverageReasoning = coverageReasoning

			logger.Info("coverage assessment completed",
				"score", coverageScore,
				"isSufficient", coverageOutput.IsSufficient,
				"gapTopics", gapTopics,
			)

			// Auto-trigger gap expansion if below threshold and depth available.
			if coverageScore < input.Config.CoverageThreshold &&
				!coverageOutput.IsSufficient &&
				len(gapTopics) > 0 &&
				expansionRounds < input.Config.MaxExpansionDepth {

				logger.Info("coverage below threshold, triggering gap expansion",
					"score", coverageScore,
					"threshold", input.Config.CoverageThreshold,
					"gapTopics", gapTopics,
				)
				gapExpansionTriggered = true

				if err := updateStatus(domain.ReviewStatusExpanding, "expanding", ""); err != nil {
					return handleFailure(fmt.Errorf("update status to expanding for gap: %w", err))
				}

				// Save gap topics as keywords.
				var gapKwOutput activities.SaveKeywordsOutput
				err = workflow.ExecuteActivity(statusCtx, statusAct.SaveKeywords, activities.SaveKeywordsInput{
					RequestID:       input.RequestID,
					Keywords:        gapTopics,
					ExtractionRound: expansionRounds + 1,
					SourceType:      "coverage_gap",
				}).Get(cancelCtx, &gapKwOutput)
				if err != nil {
					logger.Warn("failed to save gap keywords", "error", err)
					// Non-fatal, continue with search.
				}

				allKeywords = append(allKeywords, gapTopics...)
				totalKeywords += len(gapTopics)

				// Populate keywordIDMap from gap keyword save output.
				for k, v := range gapKwOutput.KeywordIDMap {
					keywordIDMap[k] = v
				}

				// Search with gap topics.
				gapSearchResult := executor.executeSearch(searchPhaseParams{
					keywords:         gapTopics,
					keywordIDMap:     keywordIDMap,
					sources:          sources,
					resultsPerSource: resultsPerSource,
					logPrefix:        "gap",
				})
				totalPapersFound += gapSearchResult.totalFound

				gapDiscoveredVia := sources[0]
				if len(sources) > 1 {
					gapDiscoveredVia = "multiple"
				}
				if err := executor.saveAndSpawnBatches(saveAndSpawnParams{
					papers:              gapSearchResult.papers,
					keywordPaperEntries: gapSearchResult.keywordPaperEntries,
					discoveredVia:       gapDiscoveredVia,
					expansionDepth:      expansionRounds + 1,
					batchIDPrefix:       fmt.Sprintf("%s-gap", workflowInfo.WorkflowExecution.ID),
					fatalOnSaveError:    false, // non-fatal for gap phase
					childFutures:        &childFutures,
				}); err != nil {
					// unreachable when fatalOnSaveError=false, but handle defensively
					logger.Warn("gap save and spawn failed", "error", err)
				}
				appendCandidates(&candidatePapers, gapSearchResult.papers, maxCandidatePapers)

				expansionRounds++
			}
		}
	}

	// =========================================================================
	// Phase 6: Complete
	// =========================================================================

	finalStatus := domain.ReviewStatusCompleted
	if progress.IsDegraded {
		finalStatus = domain.ReviewStatusPartial
	}

	if err := updateStatus(finalStatus, "completed", ""); err != nil {
		return handleFailure(fmt.Errorf("update status to %s: %w", finalStatus, err))
	}

	duration := workflow.Now(ctx).Sub(startTime).Seconds()

	result := &ReviewWorkflowResult{
		RequestID:                   input.RequestID,
		Status:                      string(finalStatus),
		KeywordsFound:               totalKeywords,
		PapersFound:                 totalPapersFound,
		PapersIngested:              progress.PapersIngested,
		DuplicatesFound:             progress.DuplicatesFound,
		PapersFailed:                progress.PapersFailed,
		ExpansionRounds:             expansionRounds,
		KeywordsFilteredByRelevance: progress.KeywordsFilteredByRelevance,
		CoverageScore:               coverageScore,
		CoverageReasoning:           coverageReasoning,
		GapTopics:                   gapTopics,
		Duration:                    duration,
	}

	// Fire-and-forget: publish review.completed event.
	_ = workflow.ExecuteActivity(eventCtx, eventAct.PublishEvent, activities.PublishEventInput{
		EventType: "review.completed",
		RequestID: input.RequestID,
		OrgID:     input.OrgID,
		ProjectID: input.ProjectID,
		Payload: map[string]interface{}{
			"keywords_found":                 totalKeywords,
			"papers_found":                   totalPapersFound,
			"papers_ingested":                progress.PapersIngested,
			"duplicates_found":               progress.DuplicatesFound,
			"papers_failed":                  progress.PapersFailed,
			"expansion_rounds":               expansionRounds,
			"duration":                       duration,
			"coverage_score":                 coverageScore,
			"coverage_reasoning":             coverageReasoning,
			"gap_topics":                     gapTopics,
			"gap_expansion_triggered":        gapExpansionTriggered,
			"keywords_filtered_by_relevance": progress.KeywordsFilteredByRelevance,
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

// createIDBatches splits paper IDs into batches of the specified size.
// Used for lightweight child workflow input to avoid Temporal payload size limits.
func createIDBatches(ids []uuid.UUID, size int) [][]uuid.UUID {
	if len(ids) == 0 || size <= 0 {
		return nil
	}

	var batches [][]uuid.UUID
	for i := 0; i < len(ids); i += size {
		end := i + size
		if end > len(ids) {
			end = len(ids)
		}
		batches = append(batches, ids[i:end])
	}
	return batches
}

// checkPausePoint checks if the workflow is paused and waits for resume if so.
// Returns an error if the context is cancelled while waiting.
func checkPausePoint(
	ctx workflow.Context,
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

	// Update status to paused in database.
	statusCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: defaultStatusActivityTimeout,
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
		// Continue anyway - don't fail the workflow for status update failure.
	}

	// Wait for resume signal.
	var resumeSignal ResumeSignal
	resumeCh.Receive(ctx, &resumeSignal)

	logger.Info("workflow resumed", "by", resumeSignal.ResumedBy)

	// Clear pause state.
	progress.IsPaused = false
	progress.PauseReason = ""

	// Update status back to running.
	err = workflow.ExecuteActivity(statusCtx, statusAct.UpdateStatus, activities.UpdateStatusInput{
		OrgID:     input.OrgID,
		ProjectID: input.ProjectID,
		RequestID: input.RequestID,
		Status:    phaseToStatus(progress.Phase),
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
	case "reviewing":
		return domain.ReviewStatusReviewing
	default:
		return domain.ReviewStatusPending
	}
}

// checkStopPoint checks if stop was requested and returns partial results if so.
// Returns (shouldReturn, result, error). If shouldReturn is true, the workflow
// should return immediately with the provided result.
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

	// Update status to partial using root context to avoid cancelled context issues.
	statusCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: defaultStatusActivityTimeout,
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

	// Publish stopped event (fire-and-forget).
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
