package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"

	sharedllm "github.com/helixir/llm"

	"github.com/helixir/literature-review-service/internal/llm"
	"github.com/helixir/literature-review-service/internal/observability"
)

// BudgetUsageReporter emits budget usage events after successful LLM calls.
type BudgetUsageReporter interface {
	ReportUsage(ctx context.Context, params BudgetUsageParams) error
}

// BudgetUsageParams contains the data needed to emit a budget usage event.
type BudgetUsageParams struct {
	LeaseID      string
	OrgID        string
	ProjectID    string
	Model        string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CostUSD      float64
}

// LLMActivities provides Temporal activities for LLM-based operations.
// Methods on this struct are registered as Temporal activities via the worker.
type LLMActivities struct {
	extractor        llm.KeywordExtractor
	coverageAssessor llm.CoverageAssessor
	metrics          *observability.Metrics
	budgetReporter   BudgetUsageReporter // nil = budget reporting disabled
}

// LLMActivitiesOption configures optional LLMActivities dependencies.
type LLMActivitiesOption func(*LLMActivities)

// WithBudgetReporter attaches a budget usage reporter to the LLM activities.
func WithBudgetReporter(reporter BudgetUsageReporter) LLMActivitiesOption {
	return func(a *LLMActivities) { a.budgetReporter = reporter }
}

// WithCoverageAssessor attaches a coverage assessor to the LLM activities.
func WithCoverageAssessor(assessor llm.CoverageAssessor) LLMActivitiesOption {
	return func(a *LLMActivities) { a.coverageAssessor = assessor }
}

// NewLLMActivities creates a new LLMActivities instance with the given dependencies.
// The metrics parameter may be nil (metrics recording will be skipped).
func NewLLMActivities(extractor llm.KeywordExtractor, metrics *observability.Metrics, opts ...LLMActivitiesOption) *LLMActivities {
	a := &LLMActivities{
		extractor: extractor,
		metrics:   metrics,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// ExtractKeywords extracts research keywords from text using an LLM.
//
// This activity converts the Temporal-serializable input into an LLM extraction
// request, invokes the configured keyword extractor, and returns the results.
// Metrics are recorded for successful and failed requests.
func (a *LLMActivities) ExtractKeywords(ctx context.Context, input ExtractKeywordsInput) (*ExtractKeywordsOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("extracting keywords",
		"mode", input.Mode,
		"textLength", len(input.Text),
		"maxKeywords", input.MaxKeywords,
		"minKeywords", input.MinKeywords,
		"existingKeywords", len(input.ExistingKeywords),
	)

	req := llm.ExtractionRequest{
		Text:             input.Text,
		Mode:             llm.ExtractionMode(input.Mode),
		MaxKeywords:      input.MaxKeywords,
		MinKeywords:      input.MinKeywords,
		ExistingKeywords: input.ExistingKeywords,
		Context:          input.Context,
	}

	start := time.Now()
	result, err := a.extractor.ExtractKeywords(ctx, req)
	duration := time.Since(start).Seconds()

	if err != nil {
		logger.Error("keyword extraction failed",
			"error", err,
			"duration", duration,
		)

		if a.metrics != nil {
			a.metrics.RecordLLMRequestFailed("extract_keywords", a.extractor.Model(), errorType(err))
		}

		return nil, fmt.Errorf("keyword extraction failed: %w", err)
	}

	logger.Info("keywords extracted successfully",
		"keywordCount", len(result.Keywords),
		"model", result.Model,
		"inputTokens", result.InputTokens,
		"outputTokens", result.OutputTokens,
		"duration", duration,
	)

	if a.metrics != nil {
		a.metrics.RecordLLMRequest("extract_keywords", result.Model, duration, result.InputTokens, result.OutputTokens)
		a.metrics.RecordKeywordsExtracted(input.Mode, len(result.Keywords))
	}

	// Emit budget usage event if lease info is available.
	if a.budgetReporter != nil && input.LeaseID != "" {
		cost := sharedllm.EstimateCost(result.Model, result.InputTokens, result.OutputTokens)
		if reportErr := a.budgetReporter.ReportUsage(ctx, BudgetUsageParams{
			LeaseID:      input.LeaseID,
			OrgID:        input.OrgID,
			ProjectID:    input.ProjectID,
			Model:        result.Model,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
			TotalTokens:  result.InputTokens + result.OutputTokens,
			CostUSD:      cost,
		}); reportErr != nil {
			// Log but don't fail the activity - budget reporting is best-effort.
			logger.Warn("failed to report budget usage",
				"error", reportErr,
				"leaseID", input.LeaseID,
			)
		}
	}

	return &ExtractKeywordsOutput{
		Keywords:     result.Keywords,
		Reasoning:    result.Reasoning,
		Model:        result.Model,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	}, nil
}

// AssessCoverage evaluates the corpus coverage of a literature review using an LLM.
//
// This activity converts the Temporal-serializable input into an LLM coverage
// assessment request, invokes the configured coverage assessor, and returns the results.
// Metrics are recorded for successful and failed requests.
func (a *LLMActivities) AssessCoverage(ctx context.Context, input AssessCoverageInput) (*AssessCoverageOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("assessing corpus coverage",
		"title", input.Title,
		"totalPapers", input.TotalPapers,
		"keywordCount", len(input.AllKeywords),
		"paperSummaries", len(input.PaperSummaries),
	)

	if a.coverageAssessor == nil {
		return nil, fmt.Errorf("coverage assessor is not configured")
	}

	summaries := make([]llm.CoveragePaperSummary, len(input.PaperSummaries))
	for i, ps := range input.PaperSummaries {
		summaries[i] = llm.CoveragePaperSummary{Title: ps.Title, Abstract: ps.Abstract}
	}

	start := time.Now()
	result, err := a.coverageAssessor.AssessCoverage(ctx, llm.CoverageRequest{
		Title:           input.Title,
		Description:     input.Description,
		SeedKeywords:    input.SeedKeywords,
		AllKeywords:     input.AllKeywords,
		PaperSummaries:  summaries,
		TotalPapers:     input.TotalPapers,
		ExpansionRounds: input.ExpansionRounds,
	})
	duration := time.Since(start).Seconds()

	if err != nil {
		logger.Error("coverage assessment failed",
			"error", err,
			"duration", duration,
		)
		if a.metrics != nil {
			a.metrics.RecordLLMRequestFailed("assess_coverage", "", errorType(err))
		}
		return nil, fmt.Errorf("coverage assessment failed: %w", err)
	}

	logger.Info("coverage assessment completed",
		"score", result.CoverageScore,
		"isSufficient", result.IsSufficient,
		"gapTopics", len(result.GapTopics),
		"model", result.Model,
		"inputTokens", result.InputTokens,
		"outputTokens", result.OutputTokens,
		"duration", duration,
	)

	if a.metrics != nil {
		a.metrics.RecordLLMRequest("assess_coverage", result.Model, duration, result.InputTokens, result.OutputTokens)
	}

	// Emit budget usage event if lease info is available.
	if a.budgetReporter != nil && input.LeaseID != "" {
		cost := sharedllm.EstimateCost(result.Model, result.InputTokens, result.OutputTokens)
		if reportErr := a.budgetReporter.ReportUsage(ctx, BudgetUsageParams{
			LeaseID:      input.LeaseID,
			OrgID:        input.OrgID,
			ProjectID:    input.ProjectID,
			Model:        result.Model,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
			TotalTokens:  result.InputTokens + result.OutputTokens,
			CostUSD:      cost,
		}); reportErr != nil {
			logger.Warn("failed to report budget usage for coverage assessment",
				"error", reportErr,
				"leaseID", input.LeaseID,
			)
		}
	}

	return &AssessCoverageOutput{
		CoverageScore: result.CoverageScore,
		Reasoning:     result.Reasoning,
		GapTopics:     result.GapTopics,
		IsSufficient:  result.IsSufficient,
		Model:         result.Model,
		InputTokens:   result.InputTokens,
		OutputTokens:  result.OutputTokens,
	}, nil
}

// errorType classifies an error for metrics labeling.
// Uses errors.As to correctly unwrap wrapped errors.
func errorType(err error) string {
	// Check for shared llm.Error first (from shared providers via ResilientClient).
	var llmErr *sharedllm.Error
	if errors.As(err, &llmErr) {
		if llmErr.Kind != "" {
			return string(llmErr.Kind)
		}
		if llmErr.StatusCode > 0 {
			return fmt.Sprintf("http_%d", llmErr.StatusCode)
		}
		return "unknown"
	}

	// Fall back to legacy APIError (for backwards compatibility during transition).
	var apiErr *llm.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Type != "" {
			return apiErr.Type
		}
		return fmt.Sprintf("http_%d", apiErr.StatusCode)
	}

	return "unknown"
}
