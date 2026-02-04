// Package papersources provides interfaces and types for academic paper source clients.
//
// This package defines the foundational abstractions that all paper source implementations
// must follow. Each academic database (Semantic Scholar, OpenAlex, PubMed, etc.) implements
// the PaperSource interface, allowing the literature review service to search multiple
// sources concurrently with a unified API.
//
// Example usage:
//
//	source := semanticscholar.New(cfg, httpClient)
//	params := papersources.SearchParams{
//		Query:      "CRISPR gene editing",
//		MaxResults: 100,
//	}
//	result, err := source.Search(ctx, params)
package papersources

import (
	"context"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
)

// SearchParams defines the parameters for searching academic papers.
// All fields except Query are optional and support filtering the search results.
type SearchParams struct {
	// Query is the search query string (required).
	// The format may vary by source - some support boolean operators,
	// field-specific searches, or semantic search.
	Query string

	// DateFrom filters papers published on or after this date.
	// If nil, no lower date bound is applied.
	DateFrom *time.Time

	// DateTo filters papers published on or before this date.
	// If nil, no upper date bound is applied.
	DateTo *time.Time

	// MaxResults limits the number of papers returned in a single request.
	// Sources may have their own maximum limits that override this value.
	// A value of 0 uses the source's default limit.
	MaxResults int

	// Offset specifies the starting position for paginated results.
	// Used in conjunction with MaxResults for pagination.
	Offset int

	// IncludePreprints includes preprint versions of papers when true.
	// When false, only peer-reviewed publications are returned.
	IncludePreprints bool

	// OpenAccessOnly filters results to only include open access papers.
	OpenAccessOnly bool

	// MinCitations filters papers to only include those with at least
	// this many citations. A value of 0 applies no citation filter.
	MinCitations int
}

// SearchResult contains the results from a paper source search operation.
type SearchResult struct {
	// Papers contains the papers returned by the search.
	// May be empty if no papers match the search criteria.
	Papers []*domain.Paper

	// TotalResults is the total number of papers matching the query,
	// regardless of pagination limits. This value is provided by the
	// source API and may be an estimate for large result sets.
	TotalResults int

	// HasMore indicates whether additional results are available
	// beyond the current page.
	HasMore bool

	// NextOffset is the offset value to use for fetching the next page
	// of results. Only meaningful when HasMore is true.
	NextOffset int

	// Source identifies which paper source provided these results.
	Source domain.SourceType

	// SearchDuration is the time taken to execute the search,
	// including network latency and response parsing.
	SearchDuration time.Duration
}

// PaperSource defines the interface that all paper source clients must implement.
// Each academic database or API (Semantic Scholar, OpenAlex, PubMed, etc.)
// provides its own implementation of this interface.
type PaperSource interface {
	// Search queries the paper source for papers matching the given parameters.
	// Returns a SearchResult containing the matching papers and pagination info.
	// The context should be used for cancellation and deadline propagation.
	//
	// Implementations should:
	//   - Respect context cancellation
	//   - Apply rate limiting as needed
	//   - Transform source-specific responses to domain.Paper
	//   - Include appropriate error wrapping with source context
	Search(ctx context.Context, params SearchParams) (*SearchResult, error)

	// GetByID retrieves a specific paper by its source-specific identifier.
	// Returns the paper if found, or an error if not found or on failure.
	// The id format is source-specific (e.g., DOI, Semantic Scholar ID).
	//
	// Returns domain.ErrNotFound if the paper does not exist.
	GetByID(ctx context.Context, id string) (*domain.Paper, error)

	// SourceType returns the type identifier for this paper source.
	// Used for attribution, deduplication, and routing.
	SourceType() domain.SourceType

	// Name returns a human-readable name for this paper source.
	// Used for logging, metrics, and display purposes.
	Name() string

	// IsEnabled returns whether this paper source is currently enabled
	// and available for searches. A source may be disabled due to
	// configuration, missing API keys, or temporary outages.
	IsEnabled() bool
}
