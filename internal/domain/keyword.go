package domain

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// whitespaceRegex matches one or more whitespace characters (spaces, tabs, newlines).
var whitespaceRegex = regexp.MustCompile(`\s+`)

// Keyword represents a unique keyword used for paper searches.
// Keywords are deduplicated by their normalized form.
type Keyword struct {
	// ID is the primary key for this keyword.
	ID uuid.UUID

	// Keyword is the original keyword text as provided by the source.
	Keyword string

	// NormalizedKeyword is the lowercase, whitespace-collapsed form used for deduplication.
	NormalizedKeyword string

	// CreatedAt records when the keyword was first created.
	CreatedAt time.Time
}

// NormalizeKeyword normalizes a keyword string by:
// - Converting to lowercase
// - Trimming leading/trailing whitespace
// - Collapsing multiple whitespace characters into a single space
func NormalizeKeyword(s string) string {
	// Trim leading and trailing whitespace
	s = strings.TrimSpace(s)

	// Return early for empty strings
	if s == "" {
		return ""
	}

	// Convert to lowercase
	s = strings.ToLower(s)

	// Collapse multiple whitespace characters into single space
	s = whitespaceRegex.ReplaceAllString(s, " ")

	return s
}

// NewKeyword creates a new Keyword with a generated ID and normalized form.
func NewKeyword(keyword string) *Keyword {
	return &Keyword{
		ID:                uuid.New(),
		Keyword:           keyword,
		NormalizedKeyword: NormalizeKeyword(keyword),
		CreatedAt:         time.Now(),
	}
}

// ComputeSearchWindowHash computes a deterministic SHA-256 hash for a
// keyword+source+dateRange combination. This is the unique key stored in
// keyword_searches.search_window_hash for deduplication.
func ComputeSearchWindowHash(keywordID uuid.UUID, source SourceType, dateFrom, dateTo *string) string {
	dfStr := ""
	if dateFrom != nil {
		dfStr = *dateFrom
	}
	dtStr := ""
	if dateTo != nil {
		dtStr = *dateTo
	}
	raw := fmt.Sprintf("%s|%s|%s|%s", keywordID, source, dfStr, dtStr)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", hash)
}

// KeywordSearch records a search operation for idempotency and audit.
// Each combination of keyword, source, and date window produces a unique search record,
// preventing duplicate API calls during recursive expansion rounds.
type KeywordSearch struct {
	// ID is the primary key for this search record.
	ID uuid.UUID

	// KeywordID references the keyword that was searched.
	KeywordID uuid.UUID

	// SourceAPI identifies which paper source API was queried.
	SourceAPI SourceType

	// SearchedAt records when the search was executed.
	SearchedAt time.Time

	// DateFrom is the start of the publication date filter window, if set.
	DateFrom *time.Time

	// DateTo is the end of the publication date filter window, if set.
	DateTo *time.Time

	// SearchWindowHash is a deterministic hash of the search parameters used for idempotency checks.
	SearchWindowHash string

	// PapersFound is the number of papers returned by this search.
	PapersFound int

	// Status is the current state of this search operation.
	Status SearchStatus

	// ErrorMessage contains the error details if the search failed.
	ErrorMessage string
}

// KeywordPaperMapping links keywords to papers with provenance information.
// This many-to-many relationship tracks how keywords and papers were associated.
type KeywordPaperMapping struct {
	// ID is the primary key for this mapping record.
	ID uuid.UUID

	// KeywordID references the keyword in the mapping.
	KeywordID uuid.UUID

	// PaperID references the paper in the mapping.
	PaperID uuid.UUID

	// MappingType indicates how the keyword-paper association was established
	// (e.g., author keyword, MeSH term, LLM extraction, or query match).
	MappingType MappingType

	// SourceType identifies which paper source API provided the association.
	SourceType SourceType

	// ConfidenceScore is an optional relevance score for the keyword-paper relationship.
	// Nil when the source does not provide confidence information.
	ConfidenceScore *float64

	// CreatedAt records when this mapping was created.
	CreatedAt time.Time
}
