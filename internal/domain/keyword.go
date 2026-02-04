package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// whitespaceRegex matches one or more whitespace characters (spaces, tabs, newlines).
var whitespaceRegex = regexp.MustCompile(`\s+`)

// Keyword represents a unique keyword used for paper searches.
type Keyword struct {
	ID                uuid.UUID
	Keyword           string
	NormalizedKeyword string
	CreatedAt         time.Time
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

// KeywordSearch records a search operation for idempotency and audit.
type KeywordSearch struct {
	ID               uuid.UUID
	KeywordID        uuid.UUID
	SourceAPI        SourceType
	SearchedAt       time.Time
	DateFrom         *time.Time
	DateTo           *time.Time
	SearchWindowHash string
	PapersFound      int
	Status           SearchStatus
	ErrorMessage     string
}

// KeywordPaperMapping links keywords to papers with provenance information.
type KeywordPaperMapping struct {
	ID              uuid.UUID
	KeywordID       uuid.UUID
	PaperID         uuid.UUID
	MappingType     MappingType
	SourceType      SourceType
	ConfidenceScore *float64
	CreatedAt       time.Time
}
