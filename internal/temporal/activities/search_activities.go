package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/activity"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/observability"
	"github.com/helixir/literature-review-service/internal/papersources"
)

// SearchActivities provides Temporal activities for paper search operations.
// Methods on this struct are registered as Temporal activities via the worker.
type SearchActivities struct {
	registry *papersources.Registry
	metrics  *observability.Metrics
}

// NewSearchActivities creates a new SearchActivities instance with the given dependencies.
// The metrics parameter may be nil (metrics recording will be skipped).
func NewSearchActivities(registry *papersources.Registry, metrics *observability.Metrics) *SearchActivities {
	return &SearchActivities{
		registry: registry,
		metrics:  metrics,
	}
}

// SearchPapers searches multiple academic paper sources concurrently and aggregates the results.
//
// This activity converts the Temporal-serializable input into papersource search params,
// invokes the registry to search across the requested sources, and aggregates all results.
// If at least one source returns papers, the activity succeeds with partial results.
// If all sources fail (zero papers returned and at least one error), the activity returns an error.
// Metrics are recorded for each source's search outcome.
func (a *SearchActivities) SearchPapers(ctx context.Context, input SearchPapersInput) (*SearchPapersOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("starting paper search",
		"query", input.Query,
		"sources", formatSourceTypes(input.Sources),
		"maxResults", input.MaxResults,
		"includePreprints", input.IncludePreprints,
		"openAccessOnly", input.OpenAccessOnly,
		"minCitations", input.MinCitations,
	)

	// Convert activity input to papersources search params.
	params := papersources.SearchParams{
		Query:            input.Query,
		MaxResults:       input.MaxResults,
		IncludePreprints: input.IncludePreprints,
		OpenAccessOnly:   input.OpenAccessOnly,
		MinCitations:     input.MinCitations,
	}

	// Record search started for each source.
	if a.metrics != nil {
		for _, src := range input.Sources {
			a.metrics.RecordSearchStarted(string(src))
		}
	}

	start := time.Now()

	// Execute concurrent searches across all requested sources.
	results := a.registry.SearchSources(ctx, params, input.Sources)

	// Aggregate results from all sources.
	var allPapers []*domain.Paper
	bySource := make(map[domain.SourceType]int)
	var sourceErrors []SourceError
	var errorCount int

	for _, sr := range results {
		sourceName := string(sr.Source)
		if sr.Error != nil {
			errorCount++
			sourceErrors = append(sourceErrors, SourceError{
				Source: sr.Source,
				Error:  sr.Error.Error(),
			})

			logger.Warn("source search failed",
				"source", sourceName,
				"error", sr.Error,
			)

			if a.metrics != nil {
				a.metrics.RecordSearchFailed(sourceName, time.Since(start).Seconds())
			}

			continue
		}

		// Successful source result.
		paperCount := 0
		if sr.Result != nil {
			paperCount = len(sr.Result.Papers)
			allPapers = append(allPapers, sr.Result.Papers...)
		}
		bySource[sr.Source] = paperCount

		searchDuration := 0.0
		if sr.Result != nil {
			searchDuration = sr.Result.SearchDuration.Seconds()
		}

		logger.Info("source search completed",
			"source", sourceName,
			"paperCount", paperCount,
			"searchDuration", searchDuration,
		)

		if a.metrics != nil {
			a.metrics.RecordSearchCompleted(sourceName, paperCount, searchDuration)
			a.metrics.RecordPapersDiscovered(sourceName, paperCount)
		}
	}

	// If all sources failed and we have zero papers, return an error.
	if len(allPapers) == 0 && errorCount > 0 {
		errMsgs := make([]string, 0, len(sourceErrors))
		for _, se := range sourceErrors {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %s", se.Source, se.Error))
		}
		return nil, fmt.Errorf("all paper sources failed: %s", strings.Join(errMsgs, "; "))
	}

	logger.Info("paper search completed",
		"totalPapers", len(allPapers),
		"sourceCount", len(bySource),
		"errorCount", errorCount,
		"duration", time.Since(start).Seconds(),
	)

	return &SearchPapersOutput{
		Papers:     allPapers,
		TotalFound: len(allPapers),
		BySource:   bySource,
		Errors:     sourceErrors,
	}, nil
}

// formatSourceTypes formats a slice of SourceType values as a comma-separated string for logging.
func formatSourceTypes(sources []domain.SourceType) string {
	strs := make([]string, len(sources))
	for i, s := range sources {
		strs[i] = string(s)
	}
	return strings.Join(strs, ", ")
}
