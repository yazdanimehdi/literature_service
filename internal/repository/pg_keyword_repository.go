package repository

import (
	"context"
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
var _ KeywordRepository = (*PgKeywordRepository)(nil)

// PgKeywordRepository is a PostgreSQL implementation of KeywordRepository.
type PgKeywordRepository struct {
	db DBTX
}

// NewPgKeywordRepository creates a new PostgreSQL keyword repository.
func NewPgKeywordRepository(db DBTX) *PgKeywordRepository {
	return &PgKeywordRepository{db: db}
}

// GetOrCreate retrieves an existing keyword by its normalized form or creates a new one.
// Uses a single INSERT...ON CONFLICT...RETURNING query to avoid two roundtrips.
func (r *PgKeywordRepository) GetOrCreate(ctx context.Context, keyword string) (*domain.Keyword, error) {
	normalized := domain.NormalizeKeyword(keyword)
	if normalized == "" {
		return nil, domain.NewValidationError("keyword", "keyword cannot be empty or whitespace-only")
	}

	kw := domain.NewKeyword(keyword)
	query := `
		INSERT INTO keywords (id, keyword, normalized_keyword, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (normalized_keyword) DO UPDATE SET
			normalized_keyword = keywords.normalized_keyword
		RETURNING id, keyword, normalized_keyword, created_at`

	err := r.db.QueryRow(ctx, query, kw.ID, keyword, normalized, kw.CreatedAt).
		Scan(&kw.ID, &kw.Keyword, &kw.NormalizedKeyword, &kw.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create keyword: %w", err)
	}

	return kw, nil
}

// GetByID retrieves a keyword by its internal UUID.
func (r *PgKeywordRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Keyword, error) {
	query := `
		SELECT id, keyword, normalized_keyword, created_at
		FROM keywords
		WHERE id = $1`

	row := r.db.QueryRow(ctx, query, id)
	kw, err := scanKeyword(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("keyword", id.String())
		}
		return nil, fmt.Errorf("failed to get keyword by ID: %w", err)
	}

	return kw, nil
}

// GetByNormalized retrieves a keyword by its normalized form.
func (r *PgKeywordRepository) GetByNormalized(ctx context.Context, normalized string) (*domain.Keyword, error) {
	if normalized == "" {
		return nil, domain.NewValidationError("normalized", "normalized keyword is required")
	}

	query := `
		SELECT id, keyword, normalized_keyword, created_at
		FROM keywords
		WHERE normalized_keyword = $1`

	row := r.db.QueryRow(ctx, query, normalized)
	kw, err := scanKeyword(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("keyword", normalized)
		}
		return nil, fmt.Errorf("failed to get keyword by normalized form: %w", err)
	}

	return kw, nil
}

// BulkGetOrCreate retrieves or creates multiple keywords in a single transaction.
func (r *PgKeywordRepository) BulkGetOrCreate(ctx context.Context, keywords []string) ([]*domain.Keyword, error) {
	if len(keywords) == 0 {
		return []*domain.Keyword{}, nil
	}

	// Normalize and filter out empty keywords
	type keywordPair struct {
		original   string
		normalized string
	}
	var pairs []keywordPair
	seen := make(map[string]bool)

	for _, kw := range keywords {
		normalized := domain.NormalizeKeyword(kw)
		if normalized == "" {
			continue
		}
		// Deduplicate by normalized form
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		pairs = append(pairs, keywordPair{original: kw, normalized: normalized})
	}

	if len(pairs) == 0 {
		return []*domain.Keyword{}, nil
	}

	// Build bulk insert with ON CONFLICT
	var valueStrings []string
	var args []interface{}
	now := time.Now().UTC()

	for i, pair := range pairs {
		id := uuid.New()
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d)", i*4+1, i*4+2, i*4+3, i*4+4))
		args = append(args, id, pair.original, pair.normalized, now)
	}

	query := fmt.Sprintf(`
		INSERT INTO keywords (id, keyword, normalized_keyword, created_at)
		VALUES %s
		ON CONFLICT (normalized_keyword) DO UPDATE SET
			normalized_keyword = keywords.normalized_keyword
		RETURNING id, keyword, normalized_keyword, created_at`,
		strings.Join(valueStrings, ", "))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to bulk get or create keywords: %w", err)
	}
	defer rows.Close()

	// Collect results into a map by normalized form for ordering
	resultMap := make(map[string]*domain.Keyword)
	for rows.Next() {
		kw, err := scanKeywordFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan keyword: %w", err)
		}
		resultMap[kw.NormalizedKeyword] = kw
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating keywords: %w", err)
	}

	// Return results in original order (by normalized form)
	results := make([]*domain.Keyword, 0, len(pairs))
	for _, pair := range pairs {
		if kw, ok := resultMap[pair.normalized]; ok {
			results = append(results, kw)
		}
	}

	return results, nil
}

// RecordSearch records a completed search operation for a keyword.
func (r *PgKeywordRepository) RecordSearch(ctx context.Context, search *domain.KeywordSearch) error {
	if search == nil {
		return domain.NewValidationError("search", "search cannot be nil")
	}

	if search.ID == uuid.Nil {
		search.ID = uuid.New()
	}
	if search.SearchedAt.IsZero() {
		search.SearchedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO keyword_searches (
			id, keyword_id, source_api, searched_at, date_from, date_to,
			search_window_hash, papers_found, status, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (search_window_hash) DO UPDATE SET
			searched_at = EXCLUDED.searched_at,
			papers_found = EXCLUDED.papers_found,
			status = EXCLUDED.status,
			error_message = EXCLUDED.error_message`

	result, err := r.db.Exec(ctx, query,
		search.ID,
		search.KeywordID,
		search.SourceAPI,
		search.SearchedAt,
		search.DateFrom,
		search.DateTo,
		search.SearchWindowHash,
		search.PapersFound,
		search.Status,
		search.ErrorMessage,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		// Foreign key violation indicates the keyword doesn't exist
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			return domain.NewNotFoundError("keyword", search.KeywordID.String())
		}
		return fmt.Errorf("failed to record search: %w", err)
	}

	// Note: ON CONFLICT DO UPDATE always affects at least one row (insert or update),
	// so RowsAffected check is not needed here. FK constraint handles missing keywords.
	_ = result // Explicitly ignore result to avoid unused variable warning

	return nil
}

// GetLastSearch retrieves the most recent search for a keyword from a specific source.
func (r *PgKeywordRepository) GetLastSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI domain.SourceType) (*domain.KeywordSearch, error) {
	query := `
		SELECT id, keyword_id, source_api, searched_at, date_from, date_to,
			search_window_hash, papers_found, status, error_message
		FROM keyword_searches
		WHERE keyword_id = $1 AND source_api = $2
		ORDER BY searched_at DESC
		LIMIT 1`

	row := r.db.QueryRow(ctx, query, keywordID, sourceAPI)
	search, err := scanKeywordSearch(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("search", fmt.Sprintf("%s:%s", keywordID, sourceAPI))
		}
		return nil, fmt.Errorf("failed to get last search: %w", err)
	}

	return search, nil
}

// NeedsSearch determines if a keyword needs to be searched in a specific source API.
func (r *PgKeywordRepository) NeedsSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI domain.SourceType, maxAge time.Duration) (bool, error) {
	lastSearch, err := r.GetLastSearch(ctx, keywordID, sourceAPI)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Never searched before
			return true, nil
		}
		return false, err
	}

	// Check if the last search failed
	if lastSearch.Status == domain.SearchStatusFailed || lastSearch.Status == domain.SearchStatusRateLimited {
		return true, nil
	}

	// Check if the last search is too old
	if time.Since(lastSearch.SearchedAt) > maxAge {
		return true, nil
	}

	return false, nil
}

// ListSearches retrieves keyword search records matching the filter criteria.
func (r *PgKeywordRepository) ListSearches(ctx context.Context, filter SearchFilter) ([]*domain.KeywordSearch, int64, error) {
	if err := filter.Validate(); err != nil {
		return nil, 0, err
	}

	// Build dynamic WHERE clause
	var conditions []string
	var args []interface{}
	argIndex := 1

	if filter.KeywordID != nil {
		conditions = append(conditions, fmt.Sprintf("keyword_id = $%d", argIndex))
		args = append(args, *filter.KeywordID)
		argIndex++
	}

	if filter.SourceAPI != nil {
		conditions = append(conditions, fmt.Sprintf("source_api = $%d", argIndex))
		args = append(args, *filter.SourceAPI)
		argIndex++
	}

	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, *filter.Status)
		argIndex++
	}

	if filter.SearchedAfter != nil {
		conditions = append(conditions, fmt.Sprintf("searched_at > $%d", argIndex))
		args = append(args, *filter.SearchedAfter)
		argIndex++
	}

	if filter.SearchedBefore != nil {
		conditions = append(conditions, fmt.Sprintf("searched_at < $%d", argIndex))
		args = append(args, *filter.SearchedBefore)
		argIndex++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching records
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM keyword_searches %s", whereClause)
	var totalCount int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to count searches: %w", err)
	}

	// Query with pagination
	selectQuery := fmt.Sprintf(`
		SELECT id, keyword_id, source_api, searched_at, date_from, date_to,
			search_window_hash, papers_found, status, error_message
		FROM keyword_searches
		%s
		ORDER BY searched_at DESC
		LIMIT $%d OFFSET $%d`,
		whereClause, argIndex, argIndex+1)

	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list searches: %w", err)
	}
	defer rows.Close()

	searches := make([]*domain.KeywordSearch, 0, filter.Limit)
	for rows.Next() {
		search, err := scanKeywordSearchFromRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan search: %w", err)
		}
		searches = append(searches, search)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating searches: %w", err)
	}

	return searches, totalCount, nil
}

// AddPaperMapping creates a mapping between a keyword and a paper.
func (r *PgKeywordRepository) AddPaperMapping(ctx context.Context, mapping *domain.KeywordPaperMapping) error {
	if mapping == nil {
		return domain.NewValidationError("mapping", "mapping cannot be nil")
	}

	if mapping.ID == uuid.Nil {
		mapping.ID = uuid.New()
	}
	if mapping.CreatedAt.IsZero() {
		mapping.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO keyword_paper_mappings (
			id, keyword_id, paper_id, mapping_type, source_type, confidence_score, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (keyword_id, paper_id, mapping_type) DO NOTHING`

	_, err := r.db.Exec(ctx, query,
		mapping.ID,
		mapping.KeywordID,
		mapping.PaperID,
		mapping.MappingType,
		mapping.SourceType,
		mapping.ConfidenceScore,
		mapping.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			return domain.NewNotFoundError("keyword or paper", fmt.Sprintf("keyword=%s, paper=%s", mapping.KeywordID, mapping.PaperID))
		}
		return fmt.Errorf("failed to add paper mapping: %w", err)
	}

	return nil
}

// BulkAddPaperMappings creates multiple keyword-paper mappings in a single transaction.
func (r *PgKeywordRepository) BulkAddPaperMappings(ctx context.Context, mappings []*domain.KeywordPaperMapping) error {
	if len(mappings) == 0 {
		return nil
	}

	// Build bulk insert with ON CONFLICT
	var valueStrings []string
	var args []interface{}
	now := time.Now().UTC()

	for i, m := range mappings {
		if m == nil {
			return domain.NewValidationError("mapping", fmt.Sprintf("mapping at index %d is nil", i))
		}

		id := m.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		createdAt := m.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}

		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			i*7+1, i*7+2, i*7+3, i*7+4, i*7+5, i*7+6, i*7+7))
		args = append(args, id, m.KeywordID, m.PaperID, m.MappingType, m.SourceType, m.ConfidenceScore, createdAt)
	}

	query := fmt.Sprintf(`
		INSERT INTO keyword_paper_mappings (
			id, keyword_id, paper_id, mapping_type, source_type, confidence_score, created_at
		) VALUES %s
		ON CONFLICT (keyword_id, paper_id, mapping_type) DO NOTHING`,
		strings.Join(valueStrings, ", "))

	_, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			return domain.NewNotFoundError("keyword or paper", "foreign key constraint violation")
		}
		return fmt.Errorf("failed to bulk add paper mappings: %w", err)
	}

	return nil
}

// GetPapersForKeyword retrieves papers associated with a specific keyword.
func (r *PgKeywordRepository) GetPapersForKeyword(ctx context.Context, keywordID uuid.UUID, limit, offset int) ([]*domain.Paper, int64, error) {
	applyPaginationDefaults(&limit, &offset)

	// Count total matching records
	countQuery := `SELECT COUNT(*) FROM keyword_paper_mappings WHERE keyword_id = $1`
	var totalCount int64
	if err := r.db.QueryRow(ctx, countQuery, keywordID).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to count papers: %w", err)
	}

	// Query with pagination
	selectQuery := `
		SELECT p.id, p.canonical_id, p.title, p.abstract, p.authors,
			p.publication_date, p.publication_year, p.venue, p.journal,
			p.volume, p.issue, p.pages, p.citation_count, p.reference_count,
			p.pdf_url, p.open_access, p.keywords_extracted,
			p.file_id, p.ingestion_run_id,
			p.raw_metadata, p.created_at, p.updated_at
		FROM papers p
		INNER JOIN keyword_paper_mappings kpm ON p.id = kpm.paper_id
		WHERE kpm.keyword_id = $1
		ORDER BY kpm.created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, selectQuery, keywordID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get papers for keyword: %w", err)
	}
	defer rows.Close()

	papers := make([]*domain.Paper, 0, limit)
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

// List retrieves keywords matching the filter criteria.
func (r *PgKeywordRepository) List(ctx context.Context, filter KeywordFilter) ([]*domain.Keyword, int64, error) {
	if err := filter.Validate(); err != nil {
		return nil, 0, err
	}

	// Build dynamic WHERE clause
	var conditions []string
	var args []interface{}
	argIndex := 1

	if filter.NormalizedContains != "" {
		conditions = append(conditions, fmt.Sprintf("normalized_keyword ILIKE $%d", argIndex))
		// Escape LIKE special characters to prevent pattern injection.
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(filter.NormalizedContains)
		args = append(args, "%"+escaped+"%")
		argIndex++
	}

	if filter.HasSearchInSource != nil {
		conditions = append(conditions, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM keyword_searches ks WHERE ks.keyword_id = k.id AND ks.source_api = $%d)", argIndex))
		args = append(args, *filter.HasSearchInSource)
		argIndex++
	}

	if filter.NeedsSearchInSource != nil {
		maxAge := 24 * time.Hour // Default max age
		if filter.MaxSearchAge != nil {
			maxAge = *filter.MaxSearchAge
		}

		// Keywords that either:
		// 1. Have never been searched in this source
		// 2. Have a failed/rate-limited search as the most recent
		// 3. Have a completed search older than maxAge
		conditions = append(conditions, fmt.Sprintf(`(
			NOT EXISTS (
				SELECT 1 FROM keyword_searches ks
				WHERE ks.keyword_id = k.id AND ks.source_api = $%d
			)
			OR EXISTS (
				SELECT 1 FROM keyword_searches ks
				WHERE ks.keyword_id = k.id
					AND ks.source_api = $%d
					AND (
						ks.status IN ('failed', 'rate_limited')
						OR (ks.status = 'completed' AND ks.searched_at < $%d)
					)
					AND ks.searched_at = (
						SELECT MAX(ks2.searched_at)
						FROM keyword_searches ks2
						WHERE ks2.keyword_id = k.id AND ks2.source_api = $%d
					)
			)
		)`, argIndex, argIndex, argIndex+1, argIndex))
		args = append(args, *filter.NeedsSearchInSource, time.Now().UTC().Add(-maxAge))
		argIndex += 2
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching records
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM keywords k %s", whereClause)
	var totalCount int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to count keywords: %w", err)
	}

	// Query with pagination
	selectQuery := fmt.Sprintf(`
		SELECT k.id, k.keyword, k.normalized_keyword, k.created_at
		FROM keywords k
		%s
		ORDER BY k.created_at DESC
		LIMIT $%d OFFSET $%d`,
		whereClause, argIndex, argIndex+1)

	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list keywords: %w", err)
	}
	defer rows.Close()

	keywords := make([]*domain.Keyword, 0, filter.Limit)
	for rows.Next() {
		kw, err := scanKeywordFromRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan keyword: %w", err)
		}
		keywords = append(keywords, kw)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating keywords: %w", err)
	}

	return keywords, totalCount, nil
}

// keywordScanDest holds the destination pointers for scanning a Keyword row.
type keywordScanDest struct {
	keyword domain.Keyword
}

// destinations returns the slice of pointers for Scan operations.
func (d *keywordScanDest) destinations() []interface{} {
	return []interface{}{
		&d.keyword.ID, &d.keyword.Keyword, &d.keyword.NormalizedKeyword, &d.keyword.CreatedAt,
	}
}

// finalize performs post-scan processing (no-op for keywords as there's no JSON).
func (d *keywordScanDest) finalize() (*domain.Keyword, error) {
	return &d.keyword, nil
}

// scanKeyword scans a single row into a Keyword.
func scanKeyword(row pgx.Row) (*domain.Keyword, error) {
	var dest keywordScanDest
	if err := row.Scan(dest.destinations()...); err != nil {
		return nil, err
	}
	return dest.finalize()
}

// scanKeywordFromRows scans the current row from pgx.Rows into a Keyword.
func scanKeywordFromRows(rows pgx.Rows) (*domain.Keyword, error) {
	var dest keywordScanDest
	if err := rows.Scan(dest.destinations()...); err != nil {
		return nil, err
	}
	return dest.finalize()
}

// keywordSearchScanDest holds the destination pointers for scanning a KeywordSearch row.
type keywordSearchScanDest struct {
	search domain.KeywordSearch
}

// destinations returns the slice of pointers for Scan operations.
func (d *keywordSearchScanDest) destinations() []interface{} {
	return []interface{}{
		&d.search.ID, &d.search.KeywordID, &d.search.SourceAPI, &d.search.SearchedAt,
		&d.search.DateFrom, &d.search.DateTo, &d.search.SearchWindowHash,
		&d.search.PapersFound, &d.search.Status, &d.search.ErrorMessage,
	}
}

// finalize performs post-scan processing (no-op for keyword searches).
func (d *keywordSearchScanDest) finalize() (*domain.KeywordSearch, error) {
	return &d.search, nil
}

// scanKeywordSearch scans a single row into a KeywordSearch.
func scanKeywordSearch(row pgx.Row) (*domain.KeywordSearch, error) {
	var dest keywordSearchScanDest
	if err := row.Scan(dest.destinations()...); err != nil {
		return nil, err
	}
	return dest.finalize()
}

// scanKeywordSearchFromRows scans the current row from pgx.Rows into a KeywordSearch.
func scanKeywordSearchFromRows(rows pgx.Rows) (*domain.KeywordSearch, error) {
	var dest keywordSearchScanDest
	if err := rows.Scan(dest.destinations()...); err != nil {
		return nil, err
	}
	return dest.finalize()
}
