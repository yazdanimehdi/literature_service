package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/helixir/literature-review-service/internal/domain"
)

// Compile-time interface verification.
var _ PaperRepository = (*PgPaperRepository)(nil)

// PgPaperRepository is a PostgreSQL implementation of PaperRepository.
type PgPaperRepository struct {
	db DBTX
}

// NewPgPaperRepository creates a new PostgreSQL paper repository.
func NewPgPaperRepository(db DBTX) *PgPaperRepository {
	return &PgPaperRepository{db: db}
}

// Create inserts a new paper or updates an existing one based on canonical_id.
func (r *PgPaperRepository) Create(ctx context.Context, paper *domain.Paper) (*domain.Paper, error) {
	if paper == nil {
		return nil, domain.NewValidationError("paper", "paper cannot be nil")
	}
	if paper.CanonicalID == "" {
		return nil, domain.NewValidationError("canonical_id", "canonical ID is required")
	}

	authorsJSON, err := json.Marshal(paper.Authors)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal authors: %w", err)
	}

	var metadataJSON []byte
	if paper.RawMetadata != nil {
		metadataJSON, err = json.Marshal(paper.RawMetadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	now := time.Now().UTC()
	if paper.ID == uuid.Nil {
		paper.ID = uuid.New()
	}

	query := `
		INSERT INTO papers (
			id, canonical_id, title, abstract, authors,
			publication_date, publication_year, venue, journal,
			volume, issue, pages, citation_count, reference_count,
			pdf_url, open_access, keywords_extracted, raw_metadata,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
		ON CONFLICT (canonical_id) DO UPDATE SET
			title = EXCLUDED.title,
			abstract = COALESCE(EXCLUDED.abstract, papers.abstract),
			authors = EXCLUDED.authors,
			publication_date = COALESCE(EXCLUDED.publication_date, papers.publication_date),
			publication_year = COALESCE(EXCLUDED.publication_year, papers.publication_year),
			venue = COALESCE(EXCLUDED.venue, papers.venue),
			journal = COALESCE(EXCLUDED.journal, papers.journal),
			citation_count = GREATEST(EXCLUDED.citation_count, papers.citation_count),
			reference_count = GREATEST(EXCLUDED.reference_count, papers.reference_count),
			pdf_url = COALESCE(EXCLUDED.pdf_url, papers.pdf_url),
			open_access = EXCLUDED.open_access OR papers.open_access,
			raw_metadata = COALESCE(EXCLUDED.raw_metadata, papers.raw_metadata),
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

	err = r.db.QueryRow(ctx, query,
		paper.ID,
		paper.CanonicalID,
		paper.Title,
		paper.Abstract,
		authorsJSON,
		paper.PublicationDate,
		paper.PublicationYear,
		paper.Venue,
		paper.Journal,
		paper.Volume,
		paper.Issue,
		paper.Pages,
		paper.CitationCount,
		paper.ReferenceCount,
		paper.PDFURL,
		paper.OpenAccess,
		paper.KeywordsExtracted,
		metadataJSON,
		now,
		now,
	).Scan(&paper.ID, &paper.CreatedAt, &paper.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to upsert paper: %w", err)
	}

	return paper, nil
}

// GetByCanonicalID retrieves a paper by its canonical identifier.
func (r *PgPaperRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*domain.Paper, error) {
	if canonicalID == "" {
		return nil, domain.NewValidationError("canonical_id", "canonical ID is required")
	}

	query := `
		SELECT id, canonical_id, title, abstract, authors,
			publication_date, publication_year, venue, journal,
			volume, issue, pages, citation_count, reference_count,
			pdf_url, open_access, keywords_extracted, raw_metadata,
			created_at, updated_at
		FROM papers
		WHERE canonical_id = $1`

	row := r.db.QueryRow(ctx, query, canonicalID)
	paper, err := scanPaper(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("paper", canonicalID)
		}
		return nil, fmt.Errorf("failed to get paper by canonical ID: %w", err)
	}

	return paper, nil
}

// GetByID retrieves a paper by its UUID.
func (r *PgPaperRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Paper, error) {
	query := `
		SELECT id, canonical_id, title, abstract, authors,
			publication_date, publication_year, venue, journal,
			volume, issue, pages, citation_count, reference_count,
			pdf_url, open_access, keywords_extracted, raw_metadata,
			created_at, updated_at
		FROM papers
		WHERE id = $1`

	row := r.db.QueryRow(ctx, query, id)
	paper, err := scanPaper(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("paper", id.String())
		}
		return nil, fmt.Errorf("failed to get paper by ID: %w", err)
	}

	return paper, nil
}

// FindByIdentifier searches for a paper by any of its external identifiers.
func (r *PgPaperRepository) FindByIdentifier(ctx context.Context, idType domain.IdentifierType, value string) (*domain.Paper, error) {
	if value == "" {
		return nil, domain.NewValidationError("value", "identifier value is required")
	}

	query := `
		SELECT p.id, p.canonical_id, p.title, p.abstract, p.authors,
			p.publication_date, p.publication_year, p.venue, p.journal,
			p.volume, p.issue, p.pages, p.citation_count, p.reference_count,
			p.pdf_url, p.open_access, p.keywords_extracted, p.raw_metadata,
			p.created_at, p.updated_at
		FROM papers p
		INNER JOIN paper_identifiers pi ON p.id = pi.paper_id
		WHERE pi.identifier_type = $1 AND pi.identifier_value = $2`

	row := r.db.QueryRow(ctx, query, idType, value)
	paper, err := scanPaper(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("paper", fmt.Sprintf("%s:%s", idType, value))
		}
		return nil, fmt.Errorf("failed to find paper by identifier: %w", err)
	}

	return paper, nil
}

// UpsertIdentifier adds or updates an identifier for a paper.
func (r *PgPaperRepository) UpsertIdentifier(ctx context.Context, paperID uuid.UUID, idType domain.IdentifierType, value string, sourceAPI domain.SourceType) error {
	if value == "" {
		return domain.NewValidationError("value", "identifier value is required")
	}

	query := `
		INSERT INTO paper_identifiers (paper_id, identifier_type, identifier_value, source_api, discovered_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (identifier_type, identifier_value) DO UPDATE SET
			source_api = EXCLUDED.source_api
		WHERE paper_identifiers.paper_id = EXCLUDED.paper_id`

	now := time.Now().UTC()
	result, err := r.db.Exec(ctx, query, paperID, idType, value, sourceAPI, now)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			// Check for foreign key violation (paper doesn't exist)
			if pgErr.Code == "23503" {
				return domain.NewNotFoundError("paper", paperID.String())
			}
			// Check for unique constraint violation (identifier belongs to different paper)
			if pgErr.Code == "23505" {
				return domain.NewAlreadyExistsError("identifier", fmt.Sprintf("%s:%s", idType, value))
			}
		}
		return fmt.Errorf("failed to upsert identifier: %w", err)
	}

	if result.RowsAffected() == 0 {
		// The WHERE clause prevented the update because the identifier belongs to a different paper
		return domain.NewAlreadyExistsError("identifier", fmt.Sprintf("%s:%s", idType, value))
	}

	return nil
}

// AddSource records that a paper was found in a specific source API.
func (r *PgPaperRepository) AddSource(ctx context.Context, paperID uuid.UUID, sourceAPI domain.SourceType, metadata map[string]interface{}) error {
	var metadataJSON []byte
	var err error
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO paper_sources (paper_id, source_api, source_metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (paper_id, source_api) DO UPDATE SET
			source_metadata = COALESCE(EXCLUDED.source_metadata, paper_sources.source_metadata),
			updated_at = NOW()`

	now := time.Now().UTC()
	result, err := r.db.Exec(ctx, query, paperID, sourceAPI, metadataJSON, now)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return domain.NewNotFoundError("paper", paperID.String())
		}
		return fmt.Errorf("failed to add source: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.NewNotFoundError("paper", paperID.String())
	}

	return nil
}

// List retrieves papers matching the filter criteria.
func (r *PgPaperRepository) List(ctx context.Context, filter PaperFilter) ([]*domain.Paper, int64, error) {
	if err := filter.Validate(); err != nil {
		return nil, 0, err
	}

	// Build dynamic WHERE clause
	var conditions []string
	var args []interface{}
	argIndex := 1

	if filter.Source != nil {
		conditions = append(conditions, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM paper_sources ps WHERE ps.paper_id = p.id AND ps.source_api = $%d)", argIndex))
		args = append(args, *filter.Source)
		argIndex++
	}

	if filter.HasPDF != nil {
		if *filter.HasPDF {
			conditions = append(conditions, "p.pdf_url IS NOT NULL AND p.pdf_url != ''")
		} else {
			conditions = append(conditions, "(p.pdf_url IS NULL OR p.pdf_url = '')")
		}
	}

	if filter.KeywordsExtracted != nil {
		conditions = append(conditions, fmt.Sprintf("p.keywords_extracted = $%d", argIndex))
		args = append(args, *filter.KeywordsExtracted)
		argIndex++
	}

	if filter.KeywordID != nil {
		conditions = append(conditions, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM paper_keywords pk WHERE pk.paper_id = p.id AND pk.keyword_id = $%d)", argIndex))
		args = append(args, *filter.KeywordID)
		argIndex++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching records
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM papers p %s", whereClause)
	var totalCount int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to count papers: %w", err)
	}

	// Query with pagination
	selectQuery := fmt.Sprintf(`
		SELECT p.id, p.canonical_id, p.title, p.abstract, p.authors,
			p.publication_date, p.publication_year, p.venue, p.journal,
			p.volume, p.issue, p.pages, p.citation_count, p.reference_count,
			p.pdf_url, p.open_access, p.keywords_extracted, p.raw_metadata,
			p.created_at, p.updated_at
		FROM papers p
		%s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d`,
		whereClause, argIndex, argIndex+1)

	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list papers: %w", err)
	}
	defer rows.Close()

	papers := make([]*domain.Paper, 0, filter.Limit)
	for rows.Next() {
		paper, err := scanPaperFromRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan paper: %w", err)
		}
		papers = append(papers, paper)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating papers: %w", err)
	}

	return papers, totalCount, nil
}

// MarkKeywordsExtracted updates a paper to indicate keywords have been extracted.
func (r *PgPaperRepository) MarkKeywordsExtracted(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE papers
		SET keywords_extracted = true, updated_at = $1
		WHERE id = $2`

	result, err := r.db.Exec(ctx, query, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to mark keywords extracted: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.NewNotFoundError("paper", id.String())
	}

	return nil
}

// BulkUpsert creates or updates multiple papers in a single transaction.
// Uses pgx.Batch to send all upserts in a single network roundtrip,
// dramatically reducing latency compared to individual queries.
func (r *PgPaperRepository) BulkUpsert(ctx context.Context, papers []*domain.Paper) ([]*domain.Paper, error) {
	if len(papers) == 0 {
		return []*domain.Paper{}, nil
	}

	// Validate all papers have canonical IDs
	for i, paper := range papers {
		if paper == nil {
			return nil, domain.NewValidationError("paper", fmt.Sprintf("paper at index %d is nil", i))
		}
		if paper.CanonicalID == "" {
			return nil, domain.NewValidationError("canonical_id", fmt.Sprintf("paper at index %d has no canonical ID", i))
		}
	}

	query := `
		INSERT INTO papers (
			id, canonical_id, title, abstract, authors,
			publication_date, publication_year, venue, journal,
			volume, issue, pages, citation_count, reference_count,
			pdf_url, open_access, keywords_extracted, raw_metadata,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
		ON CONFLICT (canonical_id) DO UPDATE SET
			title = EXCLUDED.title,
			abstract = COALESCE(EXCLUDED.abstract, papers.abstract),
			authors = EXCLUDED.authors,
			publication_date = COALESCE(EXCLUDED.publication_date, papers.publication_date),
			publication_year = COALESCE(EXCLUDED.publication_year, papers.publication_year),
			venue = COALESCE(EXCLUDED.venue, papers.venue),
			journal = COALESCE(EXCLUDED.journal, papers.journal),
			citation_count = GREATEST(EXCLUDED.citation_count, papers.citation_count),
			reference_count = GREATEST(EXCLUDED.reference_count, papers.reference_count),
			pdf_url = COALESCE(EXCLUDED.pdf_url, papers.pdf_url),
			open_access = EXCLUDED.open_access OR papers.open_access,
			raw_metadata = COALESCE(EXCLUDED.raw_metadata, papers.raw_metadata),
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

	now := time.Now().UTC()
	batch := &pgx.Batch{}

	for _, paper := range papers {
		authorsJSON, err := json.Marshal(paper.Authors)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal authors: %w", err)
		}

		var metadataJSON []byte
		if paper.RawMetadata != nil {
			metadataJSON, err = json.Marshal(paper.RawMetadata)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal metadata: %w", err)
			}
		}

		if paper.ID == uuid.Nil {
			paper.ID = uuid.New()
		}

		batch.Queue(query,
			paper.ID,
			paper.CanonicalID,
			paper.Title,
			paper.Abstract,
			authorsJSON,
			paper.PublicationDate,
			paper.PublicationYear,
			paper.Venue,
			paper.Journal,
			paper.Volume,
			paper.Issue,
			paper.Pages,
			paper.CitationCount,
			paper.ReferenceCount,
			paper.PDFURL,
			paper.OpenAccess,
			paper.KeywordsExtracted,
			metadataJSON,
			now,
			now,
		)
	}

	br := r.db.SendBatch(ctx, batch)
	defer br.Close()

	results := make([]*domain.Paper, len(papers))
	for i, paper := range papers {
		err := br.QueryRow().Scan(&paper.ID, &paper.CreatedAt, &paper.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to upsert paper at index %d: %w", i, err)
		}
		results[i] = paper
	}

	return results, nil
}

// paperScanDest holds the destination pointers for scanning a Paper row.
type paperScanDest struct {
	paper         domain.Paper
	authorsJSON   []byte
	metadataJSON  []byte
}

// destinations returns the slice of pointers for Scan operations.
func (d *paperScanDest) destinations() []interface{} {
	return []interface{}{
		&d.paper.ID, &d.paper.CanonicalID, &d.paper.Title, &d.paper.Abstract, &d.authorsJSON,
		&d.paper.PublicationDate, &d.paper.PublicationYear, &d.paper.Venue, &d.paper.Journal,
		&d.paper.Volume, &d.paper.Issue, &d.paper.Pages, &d.paper.CitationCount, &d.paper.ReferenceCount,
		&d.paper.PDFURL, &d.paper.OpenAccess, &d.paper.KeywordsExtracted, &d.metadataJSON,
		&d.paper.CreatedAt, &d.paper.UpdatedAt,
	}
}

// finalize performs post-scan processing: unmarshals JSON fields.
func (d *paperScanDest) finalize() (*domain.Paper, error) {
	if len(d.authorsJSON) > 0 {
		if err := json.Unmarshal(d.authorsJSON, &d.paper.Authors); err != nil {
			return nil, fmt.Errorf("failed to unmarshal authors: %w", err)
		}
	}

	if len(d.metadataJSON) > 0 {
		if err := json.Unmarshal(d.metadataJSON, &d.paper.RawMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &d.paper, nil
}

// scanPaper scans a single row into a Paper.
func scanPaper(row pgx.Row) (*domain.Paper, error) {
	var dest paperScanDest
	if err := row.Scan(dest.destinations()...); err != nil {
		return nil, err
	}
	return dest.finalize()
}

// scanPaperFromRows scans the current row from pgx.Rows into a Paper.
func scanPaperFromRows(rows pgx.Rows) (*domain.Paper, error) {
	var dest paperScanDest
	if err := rows.Scan(dest.destinations()...); err != nil {
		return nil, err
	}
	return dest.finalize()
}
