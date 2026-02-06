package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/observability"
	"github.com/helixir/literature-review-service/internal/papersources"
	"github.com/helixir/literature-review-service/internal/temporal/resilience"
)

// PaperSearcher defines the interface for searching paper sources.
// This decouples the activity from the concrete papersources.Registry,
// enabling straightforward testing with mock implementations.
type PaperSearcher interface {
	SearchSources(ctx context.Context, params papersources.SearchParams, sourceTypes []domain.SourceType) []papersources.SourceResult
}

// SearchActivities provides Temporal activities for paper search operations.
// Methods on this struct are registered as Temporal activities via the worker.
type SearchActivities struct {
	registry PaperSearcher
	metrics  *observability.Metrics
	breakers *resilience.BreakerRegistry
}

// NewSearchActivities creates a new SearchActivities instance with the given dependencies.
// The metrics parameter may be nil (metrics recording will be skipped).
// The breakers parameter may be nil (circuit breaker protection will be skipped).
func NewSearchActivities(registry PaperSearcher, metrics *observability.Metrics, breakers ...*resilience.BreakerRegistry) *SearchActivities {
	a := &SearchActivities{
		registry: registry,
		metrics:  metrics,
	}
	if len(breakers) > 0 && breakers[0] != nil {
		a.breakers = breakers[0]
	}
	return a
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

// SearchSingleSource searches a single paper source.
// This activity is designed for rate-limited parallel execution where each source
// is searched independently with its own rate limiting.
// If a circuit breaker is configured and open for the source, the activity
// returns a non-retryable application error so the workflow can handle it.
func (a *SearchActivities) SearchSingleSource(ctx context.Context, input SearchSingleSourceInput) (*SearchSingleSourceOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("searching single source",
		"source", input.Source,
		"query", input.Query,
		"maxResults", input.MaxResults,
	)

	// Circuit breaker check.
	sourceName := string(input.Source)
	if a.breakers != nil {
		cb := a.breakers.Get(sourceName)
		if err := cb.Allow(); err != nil {
			logger.Warn("circuit breaker open for source",
				"source", input.Source,
				"error", err,
			)
			return nil, temporal.NewNonRetryableApplicationError(
				fmt.Sprintf("circuit_open: %s", sourceName),
				"circuit_open",
				err,
			)
		}
	}

	params := papersources.SearchParams{
		Query:            input.Query,
		MaxResults:       input.MaxResults,
		IncludePreprints: input.IncludePreprints,
		OpenAccessOnly:   input.OpenAccessOnly,
		MinCitations:     input.MinCitations,
	}

	// Parse optional date filters for incremental searches.
	if input.DateFrom != nil {
		if t, err := parseSearchDate(*input.DateFrom); err == nil {
			params.DateFrom = &t
		} else {
			logger.Warn("invalid DateFrom in search input, ignoring", "dateFrom", *input.DateFrom, "error", err)
		}
	}
	if input.DateTo != nil {
		if t, err := parseSearchDate(*input.DateTo); err == nil {
			params.DateTo = &t
		} else {
			logger.Warn("invalid DateTo in search input, ignoring", "dateTo", *input.DateTo, "error", err)
		}
	}

	if a.metrics != nil {
		a.metrics.RecordSearchStarted(sourceName)
	}

	start := time.Now()

	// Search single source
	results := a.registry.SearchSources(ctx, params, []domain.SourceType{input.Source})

	output := &SearchSingleSourceOutput{
		Source: input.Source,
	}

	if len(results) == 0 {
		output.Error = "no results returned from registry"
		if a.breakers != nil {
			a.breakers.Get(sourceName).RecordFailure()
		}
		return output, nil
	}

	sr := results[0]
	if sr.Error != nil {
		output.Error = sr.Error.Error()
		logger.Warn("source search failed",
			"source", input.Source,
			"error", sr.Error,
		)
		if a.breakers != nil {
			a.breakers.Get(sourceName).RecordFailure()
		}
		if a.metrics != nil {
			a.metrics.RecordSearchFailed(sourceName, time.Since(start).Seconds())
		}
		return output, nil // Non-fatal: return result with error field set
	}

	// Record success on the circuit breaker.
	if a.breakers != nil {
		a.breakers.Get(sourceName).RecordSuccess()
	}

	if sr.Result != nil {
		output.Papers = sr.Result.Papers
		output.TotalFound = len(sr.Result.Papers)
	}

	logger.Info("source search completed",
		"source", input.Source,
		"paperCount", output.TotalFound,
		"duration", time.Since(start).Seconds(),
	)

	if a.metrics != nil {
		a.metrics.RecordSearchCompleted(sourceName, output.TotalFound, time.Since(start).Seconds())
		a.metrics.RecordPapersDiscovered(sourceName, output.TotalFound)
	}

	return output, nil
}

// parseSearchDate parses a date string in either YYYY-MM-DD or RFC3339 format.
func parseSearchDate(s string) (time.Time, error) {
	if len(s) > 40 {
		return time.Time{}, fmt.Errorf("date string too long: %d characters", len(s))
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
