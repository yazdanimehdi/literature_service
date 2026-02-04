// Package activities provides Temporal activity implementations for the
// literature review service pipeline.
//
// Activity inputs and outputs are defined as serializable structs that cross the
// Temporal serialization boundary. Each activity receives an input struct and
// returns an output struct (or error). All fields must be exported for JSON
// serialization by the Temporal SDK's default data converter.
package activities

import (
	"github.com/google/uuid"
	"github.com/helixir/literature-review-service/internal/domain"
)

// ExtractKeywordsInput contains the parameters for the keyword extraction activity.
type ExtractKeywordsInput struct {
	// Text is the input text to extract keywords from (query or abstract).
	Text string

	// Mode specifies the type of text being processed ("query" or "abstract").
	Mode string

	// MaxKeywords is the maximum number of keywords to extract.
	MaxKeywords int

	// MinKeywords is the minimum number of keywords to extract.
	MinKeywords int

	// ExistingKeywords are keywords already found (to avoid duplicates).
	ExistingKeywords []string

	// Context provides additional context about the research domain.
	Context string
}

// ExtractKeywordsOutput contains the results of the keyword extraction activity.
type ExtractKeywordsOutput struct {
	// Keywords is the list of extracted keywords/phrases.
	Keywords []string

	// Reasoning is the LLM's explanation of its keyword choices.
	Reasoning string

	// Model is the LLM model used for extraction.
	Model string

	// InputTokens is the number of input tokens consumed.
	InputTokens int

	// OutputTokens is the number of output tokens consumed.
	OutputTokens int
}

// SearchPapersInput contains the parameters for the paper search activity.
type SearchPapersInput struct {
	// Query is the search query string.
	Query string

	// Sources lists the paper sources to search.
	Sources []domain.SourceType

	// MaxResults is the maximum number of results to return per source.
	MaxResults int

	// IncludePreprints indicates whether to include preprint results.
	IncludePreprints bool

	// OpenAccessOnly indicates whether to restrict results to open access papers.
	OpenAccessOnly bool

	// MinCitations filters papers by minimum citation count.
	MinCitations int
}

// SearchPapersOutput contains the results of the paper search activity.
type SearchPapersOutput struct {
	// Papers is the list of papers found across all sources.
	Papers []*domain.Paper

	// TotalFound is the total number of papers found across all sources.
	TotalFound int

	// BySource maps each source to the count of papers found from that source.
	BySource map[domain.SourceType]int

	// Errors contains any errors encountered from individual sources.
	Errors []SourceError
}

// SourceError represents an error from a specific paper source during search.
type SourceError struct {
	// Source is the paper source that produced the error.
	Source domain.SourceType

	// Error is the error message from the source.
	Error string
}

// UpdateStatusInput contains the parameters for the review status update activity.
type UpdateStatusInput struct {
	// OrgID is the organization identifier.
	OrgID string

	// ProjectID is the project identifier.
	ProjectID string

	// RequestID is the review request identifier.
	RequestID uuid.UUID

	// Status is the new review status to set.
	Status domain.ReviewStatus

	// ErrorMsg contains an error message when transitioning to a failed state.
	ErrorMsg string
}

// SaveKeywordsInput contains the parameters for the keyword persistence activity.
type SaveKeywordsInput struct {
	// RequestID is the review request identifier.
	RequestID uuid.UUID

	// Keywords is the list of keyword strings to save.
	Keywords []string

	// ExtractionRound is the round number in which these keywords were extracted.
	ExtractionRound int

	// SourcePaperID is the ID of the paper from which keywords were extracted (nil for query-based extraction).
	SourcePaperID *uuid.UUID

	// SourceType indicates how the keywords were discovered (e.g., "query", "paper_keywords", "llm_extraction").
	SourceType string
}

// SaveKeywordsOutput contains the results of the keyword persistence activity.
type SaveKeywordsOutput struct {
	// KeywordIDs contains the UUIDs of the saved keyword records.
	KeywordIDs []uuid.UUID

	// NewCount is the number of newly created keywords (excludes duplicates).
	NewCount int
}

// SavePapersInput contains the parameters for the paper persistence activity.
type SavePapersInput struct {
	// RequestID is the review request identifier.
	RequestID uuid.UUID

	// OrgID is the organization identifier.
	OrgID string

	// ProjectID is the project identifier.
	ProjectID string

	// Papers is the list of papers to save.
	Papers []*domain.Paper

	// DiscoveredViaKeywordID is the keyword that led to discovering these papers.
	DiscoveredViaKeywordID *uuid.UUID

	// DiscoveredViaSource is the source that provided these papers.
	DiscoveredViaSource domain.SourceType

	// ExpansionDepth is the recursive expansion depth at which these papers were discovered.
	ExpansionDepth int
}

// SavePapersOutput contains the results of the paper persistence activity.
type SavePapersOutput struct {
	// SavedCount is the number of papers successfully saved.
	SavedCount int

	// DuplicateCount is the number of papers that were already present.
	DuplicateCount int
}

// IncrementCountersInput contains the parameters for the counter increment activity.
type IncrementCountersInput struct {
	// OrgID is the organization identifier.
	OrgID string

	// ProjectID is the project identifier.
	ProjectID string

	// RequestID is the review request identifier.
	RequestID uuid.UUID

	// PapersFound is the number of papers found to add to the counter.
	PapersFound int

	// PapersIngested is the number of papers ingested to add to the counter.
	PapersIngested int
}
