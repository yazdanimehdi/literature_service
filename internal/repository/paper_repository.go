package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/helixir/literature-review-service/internal/domain"
)

// PaperRepository handles academic paper persistence and deduplication.
// It manages the central paper repository with support for multiple identifiers
// and source tracking for papers discovered from various academic databases.
type PaperRepository interface {
	// Create inserts a new paper or updates an existing one based on canonical_id.
	// If a paper with the same canonical_id exists, it is updated with the new data.
	// Returns the created or updated paper with its assigned ID.
	// Returns domain.ErrInvalidInput if the paper has no valid canonical ID.
	Create(ctx context.Context, paper *domain.Paper) (*domain.Paper, error)

	// GetByCanonicalID retrieves a paper by its canonical identifier.
	// The canonical ID is a normalized identifier derived from DOI, ArXiv ID, etc.
	// Returns domain.ErrNotFound if no matching paper exists.
	GetByCanonicalID(ctx context.Context, canonicalID string) (*domain.Paper, error)

	// GetByID retrieves a paper by its internal UUID.
	// Returns domain.ErrNotFound if no matching paper exists.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Paper, error)

	// GetByIDs retrieves multiple papers by their internal UUIDs.
	// Returns only the papers that were found; missing IDs are silently skipped.
	// Returns nil, nil if the input slice is empty.
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.Paper, error)

	// FindByIdentifier searches for a paper by any of its external identifiers.
	// The idType parameter specifies the identifier type (e.g., "doi", "arxiv_id").
	// The value parameter is the identifier value to search for.
	// Returns domain.ErrNotFound if no matching paper exists.
	FindByIdentifier(ctx context.Context, idType domain.IdentifierType, value string) (*domain.Paper, error)

	// UpsertIdentifier adds or updates an identifier for a paper.
	// If the identifier already exists for this paper, it updates the source API.
	// If the identifier exists for a different paper, returns domain.ErrAlreadyExists.
	// Returns domain.ErrNotFound if the paper does not exist.
	UpsertIdentifier(ctx context.Context, paperID uuid.UUID, idType domain.IdentifierType, value string, sourceAPI domain.SourceType) error

	// AddSource records that a paper was found in a specific source API.
	// The metadata parameter contains source-specific data (response fields, etc.).
	// If the source already exists for this paper, it updates the metadata.
	// Returns domain.ErrNotFound if the paper does not exist.
	AddSource(ctx context.Context, paperID uuid.UUID, sourceAPI domain.SourceType, metadata map[string]interface{}) error

	// List retrieves papers matching the filter criteria.
	// Returns the matching papers and total count for pagination.
	// The total count reflects all matching records regardless of limit/offset.
	List(ctx context.Context, filter PaperFilter) ([]*domain.Paper, int64, error)

	// MarkKeywordsExtracted updates a paper to indicate keywords have been extracted.
	// This is used to track which papers have been processed for keyword extraction.
	// Returns domain.ErrNotFound if the paper does not exist.
	MarkKeywordsExtracted(ctx context.Context, id uuid.UUID) error

	// BulkUpsert creates or updates multiple papers in a single transaction.
	// Papers are matched by canonical_id for upsert behavior.
	// Returns domain.ErrInvalidInput if any paper has no valid canonical ID.
	//
	// Return contract:
	//   - Returned papers are in the same order as the input slice.
	//   - Database-generated fields (ID, CreatedAt, UpdatedAt) are populated on all returned papers.
	//   - For newly created papers, the returned paper contains the generated ID and timestamps.
	//   - For existing papers (matched by canonical_id), the returned paper contains the merged/updated
	//     data reflecting the final database state after the upsert.
	BulkUpsert(ctx context.Context, papers []*domain.Paper) ([]*domain.Paper, error)

	// UpdateIngestionResult updates a paper's file_id and ingestion_run_id after
	// successful PDF download and ingestion submission.
	// Returns domain.ErrNotFound if the paper does not exist.
	UpdateIngestionResult(ctx context.Context, paperID uuid.UUID, fileID uuid.UUID, ingestionRunID string) error
}

// PaperFilter specifies criteria for listing papers.
type PaperFilter struct {
	// ReviewID filters to papers associated with a specific review request (optional).
	// When set, only papers linked to this review through request_paper_mappings are returned.
	ReviewID *uuid.UUID

	// KeywordID filters to papers associated with a specific keyword (optional).
	KeywordID *uuid.UUID

	// Source filters to papers from a specific source API (optional).
	Source *domain.SourceType

	// IngestionStatus filters to papers with a specific ingestion status (optional).
	// This applies to the request_paper_mappings ingestion_status column.
	IngestionStatus *domain.IngestionStatus

	// HasPDF filters to papers that have a PDF URL available (optional).
	// When true, only papers with PDFURL set are returned.
	// When false, only papers without PDFURL are returned.
	// When nil, no filtering is applied.
	HasPDF *bool

	// KeywordsExtracted filters by keyword extraction status (optional).
	// When true, only papers with extracted keywords are returned.
	// When false, only papers without extracted keywords are returned.
	// When nil, no filtering is applied.
	KeywordsExtracted *bool

	// Limit specifies maximum number of results (default: 100, max: 1000).
	Limit int

	// Offset specifies the starting position for pagination.
	Offset int
}

// Validate checks if the filter has valid values and sets defaults.
func (f *PaperFilter) Validate() error {
	applyPaginationDefaults(&f.Limit, &f.Offset)
	return nil
}
