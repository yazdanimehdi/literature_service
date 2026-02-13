package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/helixir/literature-review-service/internal/domain"
)

// KeywordRepository handles keyword persistence and search tracking.
// It manages the keyword registry with support for normalization, deduplication,
// and tracking of search operations across multiple academic source APIs.
type KeywordRepository interface {
	// GetOrCreate retrieves an existing keyword by its normalized form or creates a new one.
	// Keywords are normalized (lowercase, trimmed, collapsed whitespace) before storage.
	// Returns the keyword with its ID, whether existing or newly created.
	GetOrCreate(ctx context.Context, keyword string) (*domain.Keyword, error)

	// GetByID retrieves a keyword by its internal UUID.
	// Returns domain.ErrNotFound if no matching keyword exists.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Keyword, error)

	// GetByNormalized retrieves a keyword by its normalized form.
	// The normalized parameter should be pre-normalized using domain.NormalizeKeyword.
	// Returns domain.ErrNotFound if no matching keyword exists.
	GetByNormalized(ctx context.Context, normalized string) (*domain.Keyword, error)

	// BulkGetOrCreate retrieves or creates multiple keywords in a single transaction.
	// This is optimized for batch processing where many keywords are processed together.
	// Returns keywords in the same order as the input slice.
	// Empty or whitespace-only keywords are skipped.
	BulkGetOrCreate(ctx context.Context, keywords []string) ([]*domain.Keyword, error)

	// RecordSearch records a completed search operation for a keyword.
	// This is used for idempotency and audit tracking of search operations.
	// If a search with the same (keyword_id, source_api, search_window_hash) exists,
	// it updates the existing record.
	RecordSearch(ctx context.Context, search *domain.KeywordSearch) error

	// GetLastSearch retrieves the most recent search for a keyword from a specific source.
	// This is used to determine when a keyword was last searched in each source API.
	// Returns domain.ErrNotFound if no search has been recorded.
	GetLastSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI domain.SourceType) (*domain.KeywordSearch, error)

	// NeedsSearch determines if a keyword needs to be searched in a specific source API.
	// A keyword needs searching if it has never been searched, or if the last search
	// was completed more than maxAge ago, or if the last search failed.
	// Returns true if a search should be performed, false if recent results exist.
	NeedsSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI domain.SourceType, maxAge time.Duration) (bool, error)

	// ListSearches retrieves keyword search records matching the filter criteria.
	// Returns the matching searches and total count for pagination.
	ListSearches(ctx context.Context, filter SearchFilter) ([]*domain.KeywordSearch, int64, error)

	// AddPaperMapping creates a mapping between a keyword and a paper.
	// The mapping includes provenance information about how the relationship was discovered.
	// If the mapping already exists, this is a no-op (idempotent).
	AddPaperMapping(ctx context.Context, mapping *domain.KeywordPaperMapping) error

	// BulkAddPaperMappings creates multiple keyword-paper mappings in a single transaction.
	// Existing mappings are ignored (idempotent bulk insert).
	// This is optimized for batch processing when associating papers with keywords.
	BulkAddPaperMappings(ctx context.Context, mappings []*domain.KeywordPaperMapping) error

	// GetPapersForKeyword retrieves papers associated with a specific keyword
	// within a specific review. The reviewID ensures tenant isolation by joining
	// through request_keyword_mappings to verify the keyword belongs to the given review.
	// Papers are returned in order of association creation time (most recent first).
	// Returns the matching papers and total count for pagination.
	GetPapersForKeyword(ctx context.Context, reviewID uuid.UUID, keywordID uuid.UUID, limit, offset int) ([]*domain.Paper, int64, error)

	// GetPapersForKeywordAndSource retrieves papers associated with a specific keyword
	// and discovered via a specific source type, scoped to a specific review.
	// The reviewID ensures tenant isolation by joining through request_keyword_mappings.
	// This prevents cross-source and cross-review paper leakage
	// when checking search deduplication for a single source.
	// Papers are returned in order of association creation time (most recent first).
	GetPapersForKeywordAndSource(ctx context.Context, reviewID uuid.UUID, keywordID uuid.UUID, source domain.SourceType, limit, offset int) ([]*domain.Paper, int64, error)

	// List retrieves keywords matching the filter criteria.
	// Returns the matching keywords and total count for pagination.
	// The total count reflects all matching records regardless of limit/offset.
	List(ctx context.Context, filter KeywordFilter) ([]*domain.Keyword, int64, error)
}

// KeywordFilter specifies criteria for listing keywords via KeywordRepository.List.
// This filter operates on the keyword registry itself, allowing discovery of keywords
// based on their properties and search history.
//
// For filtering keyword search records (the history of searches performed for keywords),
// use SearchFilter with KeywordRepository.ListSearches instead.
type KeywordFilter struct {
	// ReviewID filters to keywords associated with a specific review request (optional).
	// When set, only keywords linked through request_keyword_mappings are returned.
	ReviewID *uuid.UUID

	// ExtractionRound filters to keywords from a specific extraction round (optional).
	// Applies only when ReviewID is also set, since extraction rounds are per-review.
	ExtractionRound *int

	// SourceType filters to keywords discovered via a specific source type (optional).
	// Applies to the source_type column in request_keyword_mappings.
	SourceType *string

	// NormalizedContains filters to keywords containing this substring (optional).
	// The search is performed on the normalized keyword form.
	NormalizedContains string

	// HasSearchInSource filters to keywords that have been searched in a specific source (optional).
	HasSearchInSource *domain.SourceType

	// NeedsSearchInSource filters to keywords that need to be searched in a specific source (optional).
	// This considers both whether a search exists and whether it has expired.
	NeedsSearchInSource *domain.SourceType

	// MaxSearchAge is the maximum age for a search to be considered valid (optional).
	// Used in conjunction with NeedsSearchInSource.
	MaxSearchAge *time.Duration

	// Limit specifies maximum number of results (default: 100, max: 1000).
	Limit int

	// Offset specifies the starting position for pagination.
	Offset int
}

// Validate checks if the filter has valid values and sets defaults.
func (f *KeywordFilter) Validate() error {
	applyPaginationDefaults(&f.Limit, &f.Offset)
	return nil
}

// SearchFilter specifies criteria for listing keyword search records via KeywordRepository.ListSearches.
// This filter operates on the search history, allowing queries about when and how keywords
// were searched across different source APIs.
//
// For filtering keywords themselves (the keyword registry), use KeywordFilter with
// KeywordRepository.List instead.
type SearchFilter struct {
	// KeywordID filters to searches for a specific keyword (optional).
	KeywordID *uuid.UUID

	// SourceAPI filters to searches from a specific source API (optional).
	SourceAPI *domain.SourceType

	// Status filters to searches with a specific status (optional).
	Status *domain.SearchStatus

	// SearchedAfter filters to searches performed after this timestamp (optional).
	SearchedAfter *time.Time

	// SearchedBefore filters to searches performed before this timestamp (optional).
	SearchedBefore *time.Time

	// Limit specifies maximum number of results (default: 100, max: 1000).
	Limit int

	// Offset specifies the starting position for pagination.
	Offset int
}

// Validate checks if the filter has valid values and sets defaults.
func (f *SearchFilter) Validate() error {
	applyPaginationDefaults(&f.Limit, &f.Offset)
	return nil
}
