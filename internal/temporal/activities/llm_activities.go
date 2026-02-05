package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"

	"github.com/helixir/literature-review-service/internal/llm"
	"github.com/helixir/literature-review-service/internal/observability"
)

// LLMActivities provides Temporal activities for LLM-based operations.
// Methods on this struct are registered as Temporal activities via the worker.
type LLMActivities struct {
	extractor llm.KeywordExtractor
	metrics   *observability.Metrics
}

// NewLLMActivities creates a new LLMActivities instance with the given dependencies.
// The metrics parameter may be nil (metrics recording will be skipped).
func NewLLMActivities(extractor llm.KeywordExtractor, metrics *observability.Metrics) *LLMActivities {
	return &LLMActivities{
		extractor: extractor,
		metrics:   metrics,
	}
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

	return &ExtractKeywordsOutput{
		Keywords:     result.Keywords,
		Reasoning:    result.Reasoning,
		Model:        result.Model,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	}, nil
}

// errorType classifies an error for metrics labeling.
// Uses errors.As to correctly unwrap wrapped errors.
func errorType(err error) string {
	var apiErr *llm.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Type != "" {
			return apiErr.Type
		}
		return fmt.Sprintf("http_%d", apiErr.StatusCode)
	}
	return "unknown"
}
