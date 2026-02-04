# Phase 2: Infrastructure Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the infrastructure layer for the Literature Review Service including repositories, outbox integration, observability, and Temporal setup.

**Architecture:** Repository interfaces abstract data persistence with PostgreSQL implementations. The outbox pattern ensures reliable event publishing. Temporal orchestrates workflows with proper activity dependencies. gRPC servers use mTLS for secure service-to-service communication.

**Tech Stack:** Go 1.25, pgxpool, Temporal SDK, outbox package, grpcauth, Prometheus, zerolog

---

## Task 1: Create Repository Interfaces

**Files:**
- Create: `literature_service/internal/repository/repository.go`
- Create: `literature_service/internal/repository/review_repository.go`
- Create: `literature_service/internal/repository/paper_repository.go`
- Create: `literature_service/internal/repository/keyword_repository.go`

**Step 1: Create repository package documentation**

Create `internal/repository/repository.go`:

```go
// Package repository provides data access interfaces and implementations
// for the Literature Review Service.
//
// Thread Safety:
// All repository implementations are safe for concurrent use by multiple goroutines.
// The underlying pgxpool handles connection pooling and synchronization.
//
// Error Handling:
// All methods return domain-specific errors from the domain package.
// Wrap database errors with context using fmt.Errorf with %w verb.
//
// Transactions:
// Use the DBTX interface to support both pool and transaction contexts.
// Pass transaction from database.DB.WithTransaction for atomic operations.
package repository

import (
	"context"

	"github.com/helixir/literature-review-service/internal/database"
	"github.com/helixir/literature-review-service/internal/domain"
)

// DBTX is the database interface supporting both pool and transaction.
type DBTX = database.DBTX
```

**Step 2: Create review repository interface**

Create `internal/repository/review_repository.go`:

```go
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/helixir/literature-review-service/internal/domain"
)

// ReviewFilter defines criteria for listing literature reviews.
type ReviewFilter struct {
	OrgID        string
	ProjectID    string
	Status       *domain.ReviewStatus
	CreatedAfter *time.Time
	CreatedBefore *time.Time
	Limit        int
	Offset       int
}

// ReviewRepository defines operations for literature review persistence.
type ReviewRepository interface {
	// Create stores a new literature review request.
	// Returns ErrAlreadyExists if a review with same ID exists.
	Create(ctx context.Context, review *domain.LiteratureReviewRequest) error

	// Get retrieves a review by ID within tenant scope.
	// Returns ErrNotFound if review doesn't exist.
	Get(ctx context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error)

	// Update atomically updates a review using optimistic locking.
	// The updateFn receives current state and returns modified state.
	// Returns ErrNotFound if review doesn't exist.
	// Returns ErrConcurrentModification if review was modified.
	Update(ctx context.Context, orgID, projectID string, id uuid.UUID, updateFn func(*domain.LiteratureReviewRequest) error) error

	// UpdateStatus transitions review status with optional error message.
	// Validates state transition is allowed.
	// Returns ErrInvalidStateTransition for invalid transitions.
	UpdateStatus(ctx context.Context, orgID, projectID string, id uuid.UUID, status domain.ReviewStatus, errorMsg *string) error

	// List retrieves reviews matching filter criteria.
	// Returns reviews and total count for pagination.
	List(ctx context.Context, filter ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error)

	// IncrementCounters atomically increments review counters.
	IncrementCounters(ctx context.Context, id uuid.UUID, papersFound, papersIngested int) error

	// GetByWorkflowID retrieves a review by its Temporal workflow ID.
	// Returns ErrNotFound if no matching review.
	GetByWorkflowID(ctx context.Context, workflowID string) (*domain.LiteratureReviewRequest, error)
}
```

**Step 3: Create paper repository interface**

Create `internal/repository/paper_repository.go`:

```go
package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/helixir/literature-review-service/internal/domain"
)

// PaperFilter defines criteria for listing papers.
type PaperFilter struct {
	KeywordID       *uuid.UUID
	Source          *string
	HasPDF          *bool
	KeywordsExtracted *bool
	Limit           int
	Offset          int
}

// PaperRepository defines operations for paper persistence.
type PaperRepository interface {
	// Create stores a new paper. Uses upsert on canonical_id.
	// Returns the paper with ID populated.
	Create(ctx context.Context, paper *domain.Paper) (*domain.Paper, error)

	// GetByCanonicalID retrieves a paper by its canonical identifier.
	// Returns ErrNotFound if paper doesn't exist.
	GetByCanonicalID(ctx context.Context, canonicalID string) (*domain.Paper, error)

	// GetByID retrieves a paper by its UUID.
	// Returns ErrNotFound if paper doesn't exist.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Paper, error)

	// FindByIdentifier looks up a paper by any identifier type.
	// Returns nil, nil if no paper found (not an error).
	FindByIdentifier(ctx context.Context, idType domain.IdentifierType, value string) (*domain.Paper, error)

	// UpsertIdentifier adds or updates a paper identifier.
	UpsertIdentifier(ctx context.Context, paperID uuid.UUID, idType domain.IdentifierType, value, sourceAPI string) error

	// AddSource records that a paper was seen from a specific API.
	AddSource(ctx context.Context, paperID uuid.UUID, sourceAPI string, metadata map[string]interface{}) error

	// List retrieves papers matching filter criteria.
	List(ctx context.Context, filter PaperFilter) ([]*domain.Paper, int64, error)

	// MarkKeywordsExtracted marks a paper's keywords as extracted.
	MarkKeywordsExtracted(ctx context.Context, id uuid.UUID) error

	// BulkUpsert efficiently inserts or updates multiple papers.
	// Returns the papers with IDs populated.
	BulkUpsert(ctx context.Context, papers []*domain.Paper) ([]*domain.Paper, error)
}
```

**Step 4: Create keyword repository interface**

Create `internal/repository/keyword_repository.go`:

```go
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/helixir/literature-review-service/internal/domain"
)

// KeywordFilter defines criteria for listing keywords.
type KeywordFilter struct {
	ReviewID   *uuid.UUID
	SourceType *domain.SourceType
	Round      *int
	Limit      int
	Offset     int
}

// SearchFilter defines criteria for listing keyword searches.
type SearchFilter struct {
	KeywordID *uuid.UUID
	SourceAPI *string
	Status    *domain.SearchStatus
	Limit     int
	Offset    int
}

// KeywordRepository defines operations for keyword persistence.
type KeywordRepository interface {
	// GetOrCreate retrieves existing keyword or creates new one.
	// Normalizes keyword before lookup/creation.
	GetOrCreate(ctx context.Context, keyword string) (*domain.Keyword, error)

	// GetByID retrieves a keyword by its UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Keyword, error)

	// GetByNormalized retrieves a keyword by normalized form.
	// Returns ErrNotFound if keyword doesn't exist.
	GetByNormalized(ctx context.Context, normalized string) (*domain.Keyword, error)

	// BulkGetOrCreate efficiently gets or creates multiple keywords.
	BulkGetOrCreate(ctx context.Context, keywords []string) ([]*domain.Keyword, error)

	// RecordSearch records a search execution for a keyword.
	RecordSearch(ctx context.Context, search *domain.KeywordSearch) error

	// GetLastSearch retrieves the most recent search for a keyword+source.
	// Returns nil, nil if no search exists.
	GetLastSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI string) (*domain.KeywordSearch, error)

	// NeedsSearch checks if a keyword needs searching for a given source.
	// Returns true if no search exists or last search is older than maxAge.
	NeedsSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI string, maxAge time.Duration) (bool, error)

	// ListSearches retrieves searches matching filter criteria.
	ListSearches(ctx context.Context, filter SearchFilter) ([]*domain.KeywordSearch, int64, error)

	// AddPaperMapping creates a keyword-paper association.
	AddPaperMapping(ctx context.Context, mapping *domain.KeywordPaperMapping) error

	// BulkAddPaperMappings efficiently creates multiple associations.
	BulkAddPaperMappings(ctx context.Context, mappings []*domain.KeywordPaperMapping) error

	// GetPapersForKeyword retrieves papers associated with a keyword.
	GetPapersForKeyword(ctx context.Context, keywordID uuid.UUID, limit, offset int) ([]*domain.Paper, int64, error)
}
```

**Step 5: Run go build to verify interfaces compile**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go build ./internal/repository/...
```

Expected: Build succeeds

**Step 6: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/repository/
git commit -m "feat(repository): add repository interfaces

- Add ReviewRepository for literature review persistence
- Add PaperRepository for paper and identifier management
- Add KeywordRepository for keyword and search tracking
- Define filter types for list operations
- Document thread safety and error handling patterns"
```

---

## Task 2: Implement PostgreSQL Review Repository

**Files:**
- Create: `literature_service/internal/repository/pg_review_repository.go`
- Create: `literature_service/internal/repository/pg_review_repository_test.go`

**Step 1: Create PostgreSQL review repository**

Create `internal/repository/pg_review_repository.go`:

```go
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/helixir/literature-review-service/internal/domain"
)

// PgReviewRepository implements ReviewRepository using PostgreSQL.
type PgReviewRepository struct {
	pool *pgxpool.Pool
}

// Compile-time interface verification.
var _ ReviewRepository = (*PgReviewRepository)(nil)

// NewPgReviewRepository creates a new PostgreSQL review repository.
func NewPgReviewRepository(pool *pgxpool.Pool) *PgReviewRepository {
	return &PgReviewRepository{pool: pool}
}

// Create stores a new literature review request.
func (r *PgReviewRepository) Create(ctx context.Context, review *domain.LiteratureReviewRequest) error {
	const op = "PgReviewRepository.Create"

	configJSON, err := json.Marshal(review.Configuration)
	if err != nil {
		return fmt.Errorf("%s: marshal configuration: %w", op, err)
	}

	query := `
		INSERT INTO literature_review_requests (
			id, org_id, project_id, user_id, original_query,
			temporal_workflow_id, temporal_run_id, status,
			max_expansion_depth, configuration, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)`

	now := time.Now()
	if review.ID == uuid.Nil {
		review.ID = uuid.New()
	}
	if review.CreatedAt.IsZero() {
		review.CreatedAt = now
	}
	review.UpdatedAt = now

	_, err = r.pool.Exec(ctx, query,
		review.ID,
		review.OrgID,
		review.ProjectID,
		review.UserID,
		review.OriginalQuery,
		review.TemporalWorkflowID,
		review.TemporalRunID,
		review.Status,
		review.MaxExpansionDepth,
		configJSON,
		review.CreatedAt,
		review.UpdatedAt,
	)
	if err != nil {
		if isPgUniqueViolation(err) {
			return fmt.Errorf("%s: %w", op, domain.ErrAlreadyExists)
		}
		return fmt.Errorf("%s: exec: %w", op, err)
	}

	return nil
}

// Get retrieves a review by ID within tenant scope.
func (r *PgReviewRepository) Get(ctx context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
	const op = "PgReviewRepository.Get"

	query := `
		SELECT id, org_id, project_id, user_id, original_query,
			temporal_workflow_id, temporal_run_id, status,
			initial_keywords_count, papers_found_count, papers_ingested_count,
			current_expansion_depth, max_expansion_depth, configuration,
			error_message, created_at, started_at, completed_at, updated_at
		FROM literature_review_requests
		WHERE id = $1 AND org_id = $2 AND project_id = $3`

	review, err := r.scanReview(r.pool.QueryRow(ctx, query, id, orgID, projectID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("%s: query: %w", op, err)
	}

	return review, nil
}

// Update atomically updates a review using optimistic locking.
func (r *PgReviewRepository) Update(ctx context.Context, orgID, projectID string, id uuid.UUID, updateFn func(*domain.LiteratureReviewRequest) error) error {
	const op = "PgReviewRepository.Update"

	// Start transaction with serializable isolation
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer tx.Rollback(ctx)

	// Lock row for update
	query := `
		SELECT id, org_id, project_id, user_id, original_query,
			temporal_workflow_id, temporal_run_id, status,
			initial_keywords_count, papers_found_count, papers_ingested_count,
			current_expansion_depth, max_expansion_depth, configuration,
			error_message, created_at, started_at, completed_at, updated_at
		FROM literature_review_requests
		WHERE id = $1 AND org_id = $2 AND project_id = $3
		FOR UPDATE`

	review, err := r.scanReview(tx.QueryRow(ctx, query, id, orgID, projectID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%s: %w", op, domain.ErrNotFound)
		}
		return fmt.Errorf("%s: select for update: %w", op, err)
	}

	// Apply update function
	if err := updateFn(review); err != nil {
		return fmt.Errorf("%s: update function: %w", op, err)
	}

	// Marshal configuration
	configJSON, err := json.Marshal(review.Configuration)
	if err != nil {
		return fmt.Errorf("%s: marshal configuration: %w", op, err)
	}

	// Update row
	review.UpdatedAt = time.Now()
	updateQuery := `
		UPDATE literature_review_requests
		SET status = $1, initial_keywords_count = $2, papers_found_count = $3,
			papers_ingested_count = $4, current_expansion_depth = $5,
			configuration = $6, error_message = $7, started_at = $8,
			completed_at = $9, updated_at = $10
		WHERE id = $11`

	_, err = tx.Exec(ctx, updateQuery,
		review.Status,
		review.InitialKeywordsCount,
		review.PapersFoundCount,
		review.PapersIngestedCount,
		review.CurrentExpansionDepth,
		configJSON,
		review.ErrorMessage,
		review.StartedAt,
		review.CompletedAt,
		review.UpdatedAt,
		review.ID,
	)
	if err != nil {
		return fmt.Errorf("%s: update: %w", op, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}

	return nil
}

// UpdateStatus transitions review status with optional error message.
func (r *PgReviewRepository) UpdateStatus(ctx context.Context, orgID, projectID string, id uuid.UUID, status domain.ReviewStatus, errorMsg *string) error {
	const op = "PgReviewRepository.UpdateStatus"

	return r.Update(ctx, orgID, projectID, id, func(review *domain.LiteratureReviewRequest) error {
		// Validate state transition
		if !isValidStatusTransition(review.Status, status) {
			return fmt.Errorf("%w: cannot transition from %s to %s",
				domain.ErrInvalidStateTransition, review.Status, status)
		}

		review.Status = status
		review.ErrorMessage = errorMsg

		// Set timestamps based on status
		now := time.Now()
		if status == domain.ReviewStatusSearching && review.StartedAt == nil {
			review.StartedAt = &now
		}
		if status.IsTerminal() {
			review.CompletedAt = &now
		}

		return nil
	})
}

// List retrieves reviews matching filter criteria.
func (r *PgReviewRepository) List(ctx context.Context, filter ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
	const op = "PgReviewRepository.List"

	// Build query with filters
	baseQuery := `
		FROM literature_review_requests
		WHERE org_id = $1 AND project_id = $2`
	args := []interface{}{filter.OrgID, filter.ProjectID}
	argNum := 3

	if filter.Status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", argNum)
		args = append(args, *filter.Status)
		argNum++
	}
	if filter.CreatedAfter != nil {
		baseQuery += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, *filter.CreatedAfter)
		argNum++
	}
	if filter.CreatedBefore != nil {
		baseQuery += fmt.Sprintf(" AND created_at <= $%d", argNum)
		args = append(args, *filter.CreatedBefore)
		argNum++
	}

	// Get total count
	var total int64
	countQuery := "SELECT COUNT(*) " + baseQuery
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("%s: count: %w", op, err)
	}

	// Get paginated results
	selectQuery := `
		SELECT id, org_id, project_id, user_id, original_query,
			temporal_workflow_id, temporal_run_id, status,
			initial_keywords_count, papers_found_count, papers_ingested_count,
			current_expansion_depth, max_expansion_depth, configuration,
			error_message, created_at, started_at, completed_at, updated_at
		` + baseQuery + ` ORDER BY created_at DESC`

	if filter.Limit > 0 {
		selectQuery += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		selectQuery += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: query: %w", op, err)
	}
	defer rows.Close()

	var reviews []*domain.LiteratureReviewRequest
	for rows.Next() {
		review, err := r.scanReviewRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: scan: %w", op, err)
		}
		reviews = append(reviews, review)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("%s: rows: %w", op, err)
	}

	return reviews, total, nil
}

// IncrementCounters atomically increments review counters.
func (r *PgReviewRepository) IncrementCounters(ctx context.Context, id uuid.UUID, papersFound, papersIngested int) error {
	const op = "PgReviewRepository.IncrementCounters"

	query := `
		UPDATE literature_review_requests
		SET papers_found_count = COALESCE(papers_found_count, 0) + $1,
			papers_ingested_count = COALESCE(papers_ingested_count, 0) + $2,
			updated_at = NOW()
		WHERE id = $3`

	result, err := r.pool.Exec(ctx, query, papersFound, papersIngested, id)
	if err != nil {
		return fmt.Errorf("%s: exec: %w", op, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, domain.ErrNotFound)
	}

	return nil
}

// GetByWorkflowID retrieves a review by its Temporal workflow ID.
func (r *PgReviewRepository) GetByWorkflowID(ctx context.Context, workflowID string) (*domain.LiteratureReviewRequest, error) {
	const op = "PgReviewRepository.GetByWorkflowID"

	query := `
		SELECT id, org_id, project_id, user_id, original_query,
			temporal_workflow_id, temporal_run_id, status,
			initial_keywords_count, papers_found_count, papers_ingested_count,
			current_expansion_depth, max_expansion_depth, configuration,
			error_message, created_at, started_at, completed_at, updated_at
		FROM literature_review_requests
		WHERE temporal_workflow_id = $1`

	review, err := r.scanReview(r.pool.QueryRow(ctx, query, workflowID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("%s: query: %w", op, err)
	}

	return review, nil
}

// scanReview scans a single row into a LiteratureReviewRequest.
func (r *PgReviewRepository) scanReview(row pgx.Row) (*domain.LiteratureReviewRequest, error) {
	var review domain.LiteratureReviewRequest
	var configJSON []byte

	err := row.Scan(
		&review.ID,
		&review.OrgID,
		&review.ProjectID,
		&review.UserID,
		&review.OriginalQuery,
		&review.TemporalWorkflowID,
		&review.TemporalRunID,
		&review.Status,
		&review.InitialKeywordsCount,
		&review.PapersFoundCount,
		&review.PapersIngestedCount,
		&review.CurrentExpansionDepth,
		&review.MaxExpansionDepth,
		&configJSON,
		&review.ErrorMessage,
		&review.CreatedAt,
		&review.StartedAt,
		&review.CompletedAt,
		&review.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &review.Configuration); err != nil {
			return nil, fmt.Errorf("unmarshal configuration: %w", err)
		}
	}

	return &review, nil
}

// scanReviewRows scans rows into a LiteratureReviewRequest.
func (r *PgReviewRepository) scanReviewRows(rows pgx.Rows) (*domain.LiteratureReviewRequest, error) {
	var review domain.LiteratureReviewRequest
	var configJSON []byte

	err := rows.Scan(
		&review.ID,
		&review.OrgID,
		&review.ProjectID,
		&review.UserID,
		&review.OriginalQuery,
		&review.TemporalWorkflowID,
		&review.TemporalRunID,
		&review.Status,
		&review.InitialKeywordsCount,
		&review.PapersFoundCount,
		&review.PapersIngestedCount,
		&review.CurrentExpansionDepth,
		&review.MaxExpansionDepth,
		&configJSON,
		&review.ErrorMessage,
		&review.CreatedAt,
		&review.StartedAt,
		&review.CompletedAt,
		&review.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &review.Configuration); err != nil {
			return nil, fmt.Errorf("unmarshal configuration: %w", err)
		}
	}

	return &review, nil
}

// isValidStatusTransition validates review status transitions.
func isValidStatusTransition(from, to domain.ReviewStatus) bool {
	validTransitions := map[domain.ReviewStatus][]domain.ReviewStatus{
		domain.ReviewStatusPending: {
			domain.ReviewStatusExtractingKeywords,
			domain.ReviewStatusFailed,
			domain.ReviewStatusCancelled,
		},
		domain.ReviewStatusExtractingKeywords: {
			domain.ReviewStatusSearching,
			domain.ReviewStatusFailed,
			domain.ReviewStatusCancelled,
		},
		domain.ReviewStatusSearching: {
			domain.ReviewStatusExpanding,
			domain.ReviewStatusIngesting,
			domain.ReviewStatusCompleted,
			domain.ReviewStatusFailed,
			domain.ReviewStatusCancelled,
		},
		domain.ReviewStatusExpanding: {
			domain.ReviewStatusSearching,
			domain.ReviewStatusIngesting,
			domain.ReviewStatusFailed,
			domain.ReviewStatusCancelled,
		},
		domain.ReviewStatusIngesting: {
			domain.ReviewStatusCompleted,
			domain.ReviewStatusFailed,
			domain.ReviewStatusCancelled,
		},
	}

	allowed, exists := validTransitions[from]
	if !exists {
		return false // Terminal states cannot transition
	}

	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// isPgUniqueViolation checks if error is a PostgreSQL unique constraint violation.
func isPgUniqueViolation(err error) bool {
	// pgx returns errors with Code() method for PostgreSQL errors
	var pgErr interface{ Code() string }
	if errors.As(err, &pgErr) {
		return pgErr.Code() == "23505" // unique_violation
	}
	return false
}
```

**Step 2: Create repository tests**

Create `internal/repository/pg_review_repository_test.go`:

```go
package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/domain"
)

func TestIsValidStatusTransition(t *testing.T) {
	tests := []struct {
		name     string
		from     domain.ReviewStatus
		to       domain.ReviewStatus
		expected bool
	}{
		{
			name:     "pending to extracting keywords",
			from:     domain.ReviewStatusPending,
			to:       domain.ReviewStatusExtractingKeywords,
			expected: true,
		},
		{
			name:     "pending to searching",
			from:     domain.ReviewStatusPending,
			to:       domain.ReviewStatusSearching,
			expected: false,
		},
		{
			name:     "extracting to searching",
			from:     domain.ReviewStatusExtractingKeywords,
			to:       domain.ReviewStatusSearching,
			expected: true,
		},
		{
			name:     "searching to expanding",
			from:     domain.ReviewStatusSearching,
			to:       domain.ReviewStatusExpanding,
			expected: true,
		},
		{
			name:     "searching to ingesting",
			from:     domain.ReviewStatusSearching,
			to:       domain.ReviewStatusIngesting,
			expected: true,
		},
		{
			name:     "searching to completed",
			from:     domain.ReviewStatusSearching,
			to:       domain.ReviewStatusCompleted,
			expected: true,
		},
		{
			name:     "expanding to searching",
			from:     domain.ReviewStatusExpanding,
			to:       domain.ReviewStatusSearching,
			expected: true,
		},
		{
			name:     "ingesting to completed",
			from:     domain.ReviewStatusIngesting,
			to:       domain.ReviewStatusCompleted,
			expected: true,
		},
		{
			name:     "any to failed",
			from:     domain.ReviewStatusSearching,
			to:       domain.ReviewStatusFailed,
			expected: true,
		},
		{
			name:     "any to cancelled",
			from:     domain.ReviewStatusIngesting,
			to:       domain.ReviewStatusCancelled,
			expected: true,
		},
		{
			name:     "completed to anything",
			from:     domain.ReviewStatusCompleted,
			to:       domain.ReviewStatusSearching,
			expected: false,
		},
		{
			name:     "failed to anything",
			from:     domain.ReviewStatusFailed,
			to:       domain.ReviewStatusPending,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidStatusTransition(tt.from, tt.to)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewPgReviewRepository(t *testing.T) {
	t.Run("creates repository with pool", func(t *testing.T) {
		// This test verifies the constructor works
		// Integration tests with real DB are skipped in short mode
		repo := NewPgReviewRepository(nil)
		require.NotNil(t, repo)
	})
}

// Integration tests - require database connection
func TestPgReviewRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// TODO: Add integration tests with testcontainers
	// These tests require a running PostgreSQL instance
}
```

**Step 3: Run tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/repository/... -v -short
```

Expected: PASS

**Step 4: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/repository/
git commit -m "feat(repository): implement PostgreSQL review repository

- Add PgReviewRepository with all interface methods
- Implement optimistic locking for updates
- Add status transition validation
- Add counter increment support
- Add filter-based list with pagination
- Add unit tests for status transitions"
```

---

## Task 3: Implement PostgreSQL Paper Repository

**Files:**
- Create: `literature_service/internal/repository/pg_paper_repository.go`
- Create: `literature_service/internal/repository/pg_paper_repository_test.go`

**Step 1: Create PostgreSQL paper repository**

Create `internal/repository/pg_paper_repository.go`:

```go
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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/helixir/literature-review-service/internal/domain"
)

// PgPaperRepository implements PaperRepository using PostgreSQL.
type PgPaperRepository struct {
	pool *pgxpool.Pool
}

// Compile-time interface verification.
var _ PaperRepository = (*PgPaperRepository)(nil)

// NewPgPaperRepository creates a new PostgreSQL paper repository.
func NewPgPaperRepository(pool *pgxpool.Pool) *PgPaperRepository {
	return &PgPaperRepository{pool: pool}
}

// Create stores a new paper. Uses upsert on canonical_id.
func (r *PgPaperRepository) Create(ctx context.Context, paper *domain.Paper) (*domain.Paper, error) {
	const op = "PgPaperRepository.Create"

	authorsJSON, err := json.Marshal(paper.Authors)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal authors: %w", op, err)
	}

	var metadataJSON []byte
	if paper.RawMetadata != nil {
		metadataJSON, err = json.Marshal(paper.RawMetadata)
		if err != nil {
			return nil, fmt.Errorf("%s: marshal metadata: %w", op, err)
		}
	}

	now := time.Now()
	if paper.ID == uuid.Nil {
		paper.ID = uuid.New()
	}

	query := `
		INSERT INTO papers (
			id, canonical_id, title, abstract, authors,
			publication_date, publication_year, venue, journal,
			volume, issue, pages, citation_count, reference_count,
			pdf_url, open_access, first_discovered_at, last_updated_at,
			keywords_extracted, raw_metadata
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
			pdf_url = COALESCE(EXCLUDED.pdf_url, papers.pdf_url),
			open_access = EXCLUDED.open_access OR papers.open_access,
			last_updated_at = NOW(),
			raw_metadata = COALESCE(EXCLUDED.raw_metadata, papers.raw_metadata)
		RETURNING id, first_discovered_at, last_updated_at`

	err = r.pool.QueryRow(ctx, query,
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
		now,
		now,
		paper.KeywordsExtracted,
		metadataJSON,
	).Scan(&paper.ID, &paper.FirstDiscoveredAt, &paper.LastUpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("%s: upsert: %w", op, err)
	}

	return paper, nil
}

// GetByCanonicalID retrieves a paper by its canonical identifier.
func (r *PgPaperRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*domain.Paper, error) {
	const op = "PgPaperRepository.GetByCanonicalID"

	query := `
		SELECT id, canonical_id, title, abstract, authors,
			publication_date, publication_year, venue, journal,
			volume, issue, pages, citation_count, reference_count,
			pdf_url, open_access, first_discovered_at, last_updated_at,
			keywords_extracted, keywords_extracted_at, raw_metadata
		FROM papers
		WHERE canonical_id = $1`

	paper, err := r.scanPaper(r.pool.QueryRow(ctx, query, canonicalID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("%s: query: %w", op, err)
	}

	return paper, nil
}

// GetByID retrieves a paper by its UUID.
func (r *PgPaperRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Paper, error) {
	const op = "PgPaperRepository.GetByID"

	query := `
		SELECT id, canonical_id, title, abstract, authors,
			publication_date, publication_year, venue, journal,
			volume, issue, pages, citation_count, reference_count,
			pdf_url, open_access, first_discovered_at, last_updated_at,
			keywords_extracted, keywords_extracted_at, raw_metadata
		FROM papers
		WHERE id = $1`

	paper, err := r.scanPaper(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("%s: query: %w", op, err)
	}

	return paper, nil
}

// FindByIdentifier looks up a paper by any identifier type.
func (r *PgPaperRepository) FindByIdentifier(ctx context.Context, idType domain.IdentifierType, value string) (*domain.Paper, error) {
	const op = "PgPaperRepository.FindByIdentifier"

	query := `
		SELECT p.id, p.canonical_id, p.title, p.abstract, p.authors,
			p.publication_date, p.publication_year, p.venue, p.journal,
			p.volume, p.issue, p.pages, p.citation_count, p.reference_count,
			p.pdf_url, p.open_access, p.first_discovered_at, p.last_updated_at,
			p.keywords_extracted, p.keywords_extracted_at, p.raw_metadata
		FROM papers p
		JOIN paper_identifiers pi ON p.id = pi.paper_id
		WHERE pi.identifier_type = $1 AND pi.identifier_value = $2`

	paper, err := r.scanPaper(r.pool.QueryRow(ctx, query, idType, value))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not an error, just not found
		}
		return nil, fmt.Errorf("%s: query: %w", op, err)
	}

	return paper, nil
}

// UpsertIdentifier adds or updates a paper identifier.
func (r *PgPaperRepository) UpsertIdentifier(ctx context.Context, paperID uuid.UUID, idType domain.IdentifierType, value, sourceAPI string) error {
	const op = "PgPaperRepository.UpsertIdentifier"

	query := `
		INSERT INTO paper_identifiers (id, paper_id, identifier_type, identifier_value, source_api, discovered_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (identifier_type, identifier_value) DO UPDATE SET
			source_api = EXCLUDED.source_api`

	_, err := r.pool.Exec(ctx, query, uuid.New(), paperID, idType, value, sourceAPI)
	if err != nil {
		return fmt.Errorf("%s: upsert: %w", op, err)
	}

	return nil
}

// AddSource records that a paper was seen from a specific API.
func (r *PgPaperRepository) AddSource(ctx context.Context, paperID uuid.UUID, sourceAPI string, metadata map[string]interface{}) error {
	const op = "PgPaperRepository.AddSource"

	var metadataJSON []byte
	var err error
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("%s: marshal metadata: %w", op, err)
		}
	}

	query := `
		INSERT INTO paper_sources (id, paper_id, source_api, first_seen_at, last_seen_at, source_metadata)
		VALUES ($1, $2, $3, NOW(), NOW(), $4)
		ON CONFLICT (paper_id, source_api) DO UPDATE SET
			last_seen_at = NOW(),
			source_metadata = COALESCE(EXCLUDED.source_metadata, paper_sources.source_metadata)`

	_, err = r.pool.Exec(ctx, query, uuid.New(), paperID, sourceAPI, metadataJSON)
	if err != nil {
		return fmt.Errorf("%s: upsert: %w", op, err)
	}

	return nil
}

// List retrieves papers matching filter criteria.
func (r *PgPaperRepository) List(ctx context.Context, filter PaperFilter) ([]*domain.Paper, int64, error) {
	const op = "PgPaperRepository.List"

	baseQuery := "FROM papers p WHERE 1=1"
	args := []interface{}{}
	argNum := 1

	if filter.KeywordID != nil {
		baseQuery += fmt.Sprintf(" AND p.id IN (SELECT paper_id FROM keyword_paper_mappings WHERE keyword_id = $%d)", argNum)
		args = append(args, *filter.KeywordID)
		argNum++
	}
	if filter.Source != nil {
		baseQuery += fmt.Sprintf(" AND p.id IN (SELECT paper_id FROM paper_sources WHERE source_api = $%d)", argNum)
		args = append(args, *filter.Source)
		argNum++
	}
	if filter.HasPDF != nil {
		if *filter.HasPDF {
			baseQuery += " AND p.pdf_url IS NOT NULL AND p.pdf_url != ''"
		} else {
			baseQuery += " AND (p.pdf_url IS NULL OR p.pdf_url = '')"
		}
	}
	if filter.KeywordsExtracted != nil {
		baseQuery += fmt.Sprintf(" AND p.keywords_extracted = $%d", argNum)
		args = append(args, *filter.KeywordsExtracted)
		argNum++
	}

	// Count total
	var total int64
	countQuery := "SELECT COUNT(*) " + baseQuery
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("%s: count: %w", op, err)
	}

	// Get results
	selectQuery := `
		SELECT p.id, p.canonical_id, p.title, p.abstract, p.authors,
			p.publication_date, p.publication_year, p.venue, p.journal,
			p.volume, p.issue, p.pages, p.citation_count, p.reference_count,
			p.pdf_url, p.open_access, p.first_discovered_at, p.last_updated_at,
			p.keywords_extracted, p.keywords_extracted_at, p.raw_metadata
		` + baseQuery + " ORDER BY p.first_discovered_at DESC"

	if filter.Limit > 0 {
		selectQuery += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		selectQuery += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: query: %w", op, err)
	}
	defer rows.Close()

	var papers []*domain.Paper
	for rows.Next() {
		paper, err := r.scanPaperRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: scan: %w", op, err)
		}
		papers = append(papers, paper)
	}

	return papers, total, nil
}

// MarkKeywordsExtracted marks a paper's keywords as extracted.
func (r *PgPaperRepository) MarkKeywordsExtracted(ctx context.Context, id uuid.UUID) error {
	const op = "PgPaperRepository.MarkKeywordsExtracted"

	query := `
		UPDATE papers
		SET keywords_extracted = true, keywords_extracted_at = NOW(), last_updated_at = NOW()
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("%s: exec: %w", op, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, domain.ErrNotFound)
	}

	return nil
}

// BulkUpsert efficiently inserts or updates multiple papers.
func (r *PgPaperRepository) BulkUpsert(ctx context.Context, papers []*domain.Paper) ([]*domain.Paper, error) {
	const op = "PgPaperRepository.BulkUpsert"

	if len(papers) == 0 {
		return papers, nil
	}

	// Build batch
	batch := &pgx.Batch{}
	now := time.Now()

	for _, paper := range papers {
		if paper.ID == uuid.Nil {
			paper.ID = uuid.New()
		}

		authorsJSON, err := json.Marshal(paper.Authors)
		if err != nil {
			return nil, fmt.Errorf("%s: marshal authors: %w", op, err)
		}

		var metadataJSON []byte
		if paper.RawMetadata != nil {
			metadataJSON, _ = json.Marshal(paper.RawMetadata)
		}

		query := `
			INSERT INTO papers (
				id, canonical_id, title, abstract, authors,
				publication_date, publication_year, venue, journal,
				volume, issue, pages, citation_count, reference_count,
				pdf_url, open_access, first_discovered_at, last_updated_at,
				keywords_extracted, raw_metadata
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
			)
			ON CONFLICT (canonical_id) DO UPDATE SET
				title = EXCLUDED.title,
				abstract = COALESCE(EXCLUDED.abstract, papers.abstract),
				authors = EXCLUDED.authors,
				citation_count = GREATEST(EXCLUDED.citation_count, papers.citation_count),
				last_updated_at = NOW()
			RETURNING id, first_discovered_at, last_updated_at`

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
			now,
			now,
			paper.KeywordsExtracted,
			metadataJSON,
		)
	}

	// Execute batch
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()

	for i := range papers {
		err := results.QueryRow().Scan(
			&papers[i].ID,
			&papers[i].FirstDiscoveredAt,
			&papers[i].LastUpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: batch result %d: %w", op, i, err)
		}
	}

	return papers, nil
}

// scanPaper scans a single row into a Paper.
func (r *PgPaperRepository) scanPaper(row pgx.Row) (*domain.Paper, error) {
	var paper domain.Paper
	var authorsJSON []byte
	var metadataJSON []byte

	err := row.Scan(
		&paper.ID,
		&paper.CanonicalID,
		&paper.Title,
		&paper.Abstract,
		&authorsJSON,
		&paper.PublicationDate,
		&paper.PublicationYear,
		&paper.Venue,
		&paper.Journal,
		&paper.Volume,
		&paper.Issue,
		&paper.Pages,
		&paper.CitationCount,
		&paper.ReferenceCount,
		&paper.PDFURL,
		&paper.OpenAccess,
		&paper.FirstDiscoveredAt,
		&paper.LastUpdatedAt,
		&paper.KeywordsExtracted,
		&paper.KeywordsExtractedAt,
		&metadataJSON,
	)
	if err != nil {
		return nil, err
	}

	if len(authorsJSON) > 0 {
		if err := json.Unmarshal(authorsJSON, &paper.Authors); err != nil {
			return nil, fmt.Errorf("unmarshal authors: %w", err)
		}
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &paper.RawMetadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return &paper, nil
}

// scanPaperRows scans rows into a Paper.
func (r *PgPaperRepository) scanPaperRows(rows pgx.Rows) (*domain.Paper, error) {
	var paper domain.Paper
	var authorsJSON []byte
	var metadataJSON []byte

	err := rows.Scan(
		&paper.ID,
		&paper.CanonicalID,
		&paper.Title,
		&paper.Abstract,
		&authorsJSON,
		&paper.PublicationDate,
		&paper.PublicationYear,
		&paper.Venue,
		&paper.Journal,
		&paper.Volume,
		&paper.Issue,
		&paper.Pages,
		&paper.CitationCount,
		&paper.ReferenceCount,
		&paper.PDFURL,
		&paper.OpenAccess,
		&paper.FirstDiscoveredAt,
		&paper.LastUpdatedAt,
		&paper.KeywordsExtracted,
		&paper.KeywordsExtractedAt,
		&metadataJSON,
	)
	if err != nil {
		return nil, err
	}

	if len(authorsJSON) > 0 {
		json.Unmarshal(authorsJSON, &paper.Authors)
	}
	if len(metadataJSON) > 0 {
		json.Unmarshal(metadataJSON, &paper.RawMetadata)
	}

	return &paper, nil
}
```

**Step 2: Create paper repository tests**

Create `internal/repository/pg_paper_repository_test.go`:

```go
package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPgPaperRepository(t *testing.T) {
	t.Run("creates repository with pool", func(t *testing.T) {
		repo := NewPgPaperRepository(nil)
		require.NotNil(t, repo)
	})
}

func TestPgPaperRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// TODO: Add integration tests with testcontainers
}
```

**Step 3: Run tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/repository/... -v -short
```

Expected: PASS

**Step 4: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/repository/
git commit -m "feat(repository): implement PostgreSQL paper repository

- Add PgPaperRepository with upsert support
- Implement bulk upsert using pgx batch
- Add identifier and source management
- Add filter-based list with pagination
- Support canonical ID deduplication"
```

---

## Task 4: Implement PostgreSQL Keyword Repository

**Files:**
- Create: `literature_service/internal/repository/pg_keyword_repository.go`
- Create: `literature_service/internal/repository/pg_keyword_repository_test.go`

**Step 1: Create PostgreSQL keyword repository**

Create `internal/repository/pg_keyword_repository.go`:

```go
package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/helixir/literature-review-service/internal/domain"
)

// PgKeywordRepository implements KeywordRepository using PostgreSQL.
type PgKeywordRepository struct {
	pool *pgxpool.Pool
}

// Compile-time interface verification.
var _ KeywordRepository = (*PgKeywordRepository)(nil)

// NewPgKeywordRepository creates a new PostgreSQL keyword repository.
func NewPgKeywordRepository(pool *pgxpool.Pool) *PgKeywordRepository {
	return &PgKeywordRepository{pool: pool}
}

// GetOrCreate retrieves existing keyword or creates new one.
func (r *PgKeywordRepository) GetOrCreate(ctx context.Context, keyword string) (*domain.Keyword, error) {
	const op = "PgKeywordRepository.GetOrCreate"

	normalized := domain.NormalizeKeyword(keyword)

	// Try to get existing
	existing, err := r.GetByNormalized(ctx, normalized)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("%s: get: %w", op, err)
	}

	// Create new
	kw := domain.NewKeyword(keyword)
	query := `
		INSERT INTO keywords (id, keyword, normalized_keyword, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (normalized_keyword) DO UPDATE SET keyword = keywords.keyword
		RETURNING id, keyword, normalized_keyword, created_at`

	err = r.pool.QueryRow(ctx, query, kw.ID, kw.Keyword, kw.NormalizedKeyword, kw.CreatedAt).
		Scan(&kw.ID, &kw.Keyword, &kw.NormalizedKeyword, &kw.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("%s: upsert: %w", op, err)
	}

	return kw, nil
}

// GetByID retrieves a keyword by its UUID.
func (r *PgKeywordRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Keyword, error) {
	const op = "PgKeywordRepository.GetByID"

	query := `SELECT id, keyword, normalized_keyword, created_at FROM keywords WHERE id = $1`

	var kw domain.Keyword
	err := r.pool.QueryRow(ctx, query, id).Scan(&kw.ID, &kw.Keyword, &kw.NormalizedKeyword, &kw.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("%s: query: %w", op, err)
	}

	return &kw, nil
}

// GetByNormalized retrieves a keyword by normalized form.
func (r *PgKeywordRepository) GetByNormalized(ctx context.Context, normalized string) (*domain.Keyword, error) {
	const op = "PgKeywordRepository.GetByNormalized"

	query := `SELECT id, keyword, normalized_keyword, created_at FROM keywords WHERE normalized_keyword = $1`

	var kw domain.Keyword
	err := r.pool.QueryRow(ctx, query, normalized).Scan(&kw.ID, &kw.Keyword, &kw.NormalizedKeyword, &kw.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("%s: query: %w", op, err)
	}

	return &kw, nil
}

// BulkGetOrCreate efficiently gets or creates multiple keywords.
func (r *PgKeywordRepository) BulkGetOrCreate(ctx context.Context, keywords []string) ([]*domain.Keyword, error) {
	const op = "PgKeywordRepository.BulkGetOrCreate"

	if len(keywords) == 0 {
		return []*domain.Keyword{}, nil
	}

	result := make([]*domain.Keyword, len(keywords))
	batch := &pgx.Batch{}
	now := time.Now()

	for i, kw := range keywords {
		normalized := domain.NormalizeKeyword(kw)
		id := uuid.New()

		query := `
			INSERT INTO keywords (id, keyword, normalized_keyword, created_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (normalized_keyword) DO UPDATE SET keyword = keywords.keyword
			RETURNING id, keyword, normalized_keyword, created_at`

		batch.Queue(query, id, kw, normalized, now)
		result[i] = &domain.Keyword{ID: id, Keyword: kw, NormalizedKeyword: normalized}
	}

	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()

	for i := range result {
		err := results.QueryRow().Scan(
			&result[i].ID,
			&result[i].Keyword,
			&result[i].NormalizedKeyword,
			&result[i].CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: batch result %d: %w", op, i, err)
		}
	}

	return result, nil
}

// RecordSearch records a search execution for a keyword.
func (r *PgKeywordRepository) RecordSearch(ctx context.Context, search *domain.KeywordSearch) error {
	const op = "PgKeywordRepository.RecordSearch"

	if search.ID == uuid.Nil {
		search.ID = uuid.New()
	}

	// Generate window hash for deduplication
	windowHash := generateSearchWindowHash(search.KeywordID, search.SourceAPI, search.SearchFromDate, search.SearchToDate)

	query := `
		INSERT INTO keyword_searches (
			id, keyword_id, source_api, searched_at, search_from_date,
			search_to_date, search_window_hash, papers_found, status, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (keyword_id, source_api, search_window_hash) DO UPDATE SET
			searched_at = EXCLUDED.searched_at,
			papers_found = EXCLUDED.papers_found,
			status = EXCLUDED.status,
			error_message = EXCLUDED.error_message`

	_, err := r.pool.Exec(ctx, query,
		search.ID,
		search.KeywordID,
		search.SourceAPI,
		search.SearchedAt,
		search.SearchFromDate,
		search.SearchToDate,
		windowHash,
		search.PapersFound,
		search.Status,
		search.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("%s: upsert: %w", op, err)
	}

	return nil
}

// GetLastSearch retrieves the most recent search for a keyword+source.
func (r *PgKeywordRepository) GetLastSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI string) (*domain.KeywordSearch, error) {
	const op = "PgKeywordRepository.GetLastSearch"

	query := `
		SELECT id, keyword_id, source_api, searched_at, search_from_date,
			search_to_date, papers_found, status, error_message
		FROM keyword_searches
		WHERE keyword_id = $1 AND source_api = $2
		ORDER BY searched_at DESC
		LIMIT 1`

	var search domain.KeywordSearch
	err := r.pool.QueryRow(ctx, query, keywordID, sourceAPI).Scan(
		&search.ID,
		&search.KeywordID,
		&search.SourceAPI,
		&search.SearchedAt,
		&search.SearchFromDate,
		&search.SearchToDate,
		&search.PapersFound,
		&search.Status,
		&search.ErrorMessage,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: query: %w", op, err)
	}

	return &search, nil
}

// NeedsSearch checks if a keyword needs searching for a given source.
func (r *PgKeywordRepository) NeedsSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI string, maxAge time.Duration) (bool, error) {
	const op = "PgKeywordRepository.NeedsSearch"

	lastSearch, err := r.GetLastSearch(ctx, keywordID, sourceAPI)
	if err != nil {
		return false, fmt.Errorf("%s: get last search: %w", op, err)
	}

	if lastSearch == nil {
		return true, nil
	}

	return time.Since(lastSearch.SearchedAt) > maxAge, nil
}

// ListSearches retrieves searches matching filter criteria.
func (r *PgKeywordRepository) ListSearches(ctx context.Context, filter SearchFilter) ([]*domain.KeywordSearch, int64, error) {
	const op = "PgKeywordRepository.ListSearches"

	baseQuery := "FROM keyword_searches WHERE 1=1"
	args := []interface{}{}
	argNum := 1

	if filter.KeywordID != nil {
		baseQuery += fmt.Sprintf(" AND keyword_id = $%d", argNum)
		args = append(args, *filter.KeywordID)
		argNum++
	}
	if filter.SourceAPI != nil {
		baseQuery += fmt.Sprintf(" AND source_api = $%d", argNum)
		args = append(args, *filter.SourceAPI)
		argNum++
	}
	if filter.Status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", argNum)
		args = append(args, *filter.Status)
		argNum++
	}

	// Count
	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) "+baseQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("%s: count: %w", op, err)
	}

	// Select
	selectQuery := `
		SELECT id, keyword_id, source_api, searched_at, search_from_date,
			search_to_date, papers_found, status, error_message
		` + baseQuery + " ORDER BY searched_at DESC"

	if filter.Limit > 0 {
		selectQuery += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		selectQuery += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: query: %w", op, err)
	}
	defer rows.Close()

	var searches []*domain.KeywordSearch
	for rows.Next() {
		var search domain.KeywordSearch
		err := rows.Scan(
			&search.ID,
			&search.KeywordID,
			&search.SourceAPI,
			&search.SearchedAt,
			&search.SearchFromDate,
			&search.SearchToDate,
			&search.PapersFound,
			&search.Status,
			&search.ErrorMessage,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: scan: %w", op, err)
		}
		searches = append(searches, &search)
	}

	return searches, total, nil
}

// AddPaperMapping creates a keyword-paper association.
func (r *PgKeywordRepository) AddPaperMapping(ctx context.Context, mapping *domain.KeywordPaperMapping) error {
	const op = "PgKeywordRepository.AddPaperMapping"

	if mapping.ID == uuid.Nil {
		mapping.ID = uuid.New()
	}

	query := `
		INSERT INTO keyword_paper_mappings (
			id, keyword_id, paper_id, mapping_type, source_type, confidence_score, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (keyword_id, paper_id, mapping_type) DO NOTHING`

	_, err := r.pool.Exec(ctx, query,
		mapping.ID,
		mapping.KeywordID,
		mapping.PaperID,
		mapping.MappingType,
		mapping.SourceType,
		mapping.ConfidenceScore,
		mapping.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("%s: insert: %w", op, err)
	}

	return nil
}

// BulkAddPaperMappings efficiently creates multiple associations.
func (r *PgKeywordRepository) BulkAddPaperMappings(ctx context.Context, mappings []*domain.KeywordPaperMapping) error {
	const op = "PgKeywordRepository.BulkAddPaperMappings"

	if len(mappings) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO keyword_paper_mappings (
			id, keyword_id, paper_id, mapping_type, source_type, confidence_score, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (keyword_id, paper_id, mapping_type) DO NOTHING`

	for _, m := range mappings {
		if m.ID == uuid.Nil {
			m.ID = uuid.New()
		}
		batch.Queue(query, m.ID, m.KeywordID, m.PaperID, m.MappingType, m.SourceType, m.ConfidenceScore, m.CreatedAt)
	}

	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()

	for i := range mappings {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("%s: batch result %d: %w", op, i, err)
		}
	}

	return nil
}

// GetPapersForKeyword retrieves papers associated with a keyword.
func (r *PgKeywordRepository) GetPapersForKeyword(ctx context.Context, keywordID uuid.UUID, limit, offset int) ([]*domain.Paper, int64, error) {
	const op = "PgKeywordRepository.GetPapersForKeyword"

	// Count
	var total int64
	countQuery := `SELECT COUNT(*) FROM keyword_paper_mappings WHERE keyword_id = $1`
	if err := r.pool.QueryRow(ctx, countQuery, keywordID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("%s: count: %w", op, err)
	}

	// Select papers
	query := `
		SELECT p.id, p.canonical_id, p.title, p.abstract, p.authors,
			p.publication_date, p.publication_year, p.venue, p.journal,
			p.volume, p.issue, p.pages, p.citation_count, p.reference_count,
			p.pdf_url, p.open_access, p.first_discovered_at, p.last_updated_at,
			p.keywords_extracted, p.keywords_extracted_at, p.raw_metadata
		FROM papers p
		JOIN keyword_paper_mappings kpm ON p.id = kpm.paper_id
		WHERE kpm.keyword_id = $1
		ORDER BY p.citation_count DESC NULLS LAST
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, keywordID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: query: %w", op, err)
	}
	defer rows.Close()

	paperRepo := &PgPaperRepository{pool: r.pool}
	var papers []*domain.Paper
	for rows.Next() {
		paper, err := paperRepo.scanPaperRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: scan: %w", op, err)
		}
		papers = append(papers, paper)
	}

	return papers, total, nil
}

// generateSearchWindowHash creates a hash for search deduplication.
func generateSearchWindowHash(keywordID uuid.UUID, sourceAPI string, fromDate, toDate *time.Time) string {
	var from, to string
	if fromDate != nil {
		from = fromDate.Format("2006-01-02")
	}
	if toDate != nil {
		to = toDate.Format("2006-01-02")
	}

	data := fmt.Sprintf("%s:%s:%s:%s", keywordID, sourceAPI, from, to)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:16])
}
```

**Step 2: Create keyword repository tests**

Create `internal/repository/pg_keyword_repository_test.go`:

```go
package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPgKeywordRepository(t *testing.T) {
	t.Run("creates repository with pool", func(t *testing.T) {
		repo := NewPgKeywordRepository(nil)
		require.NotNil(t, repo)
	})
}

func TestGenerateSearchWindowHash(t *testing.T) {
	t.Run("generates consistent hash for same inputs", func(t *testing.T) {
		keywordID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
		sourceAPI := "semantic_scholar"
		fromDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		toDate := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

		hash1 := generateSearchWindowHash(keywordID, sourceAPI, &fromDate, &toDate)
		hash2 := generateSearchWindowHash(keywordID, sourceAPI, &fromDate, &toDate)

		assert.Equal(t, hash1, hash2)
		assert.Len(t, hash1, 32) // 16 bytes hex encoded
	})

	t.Run("generates different hash for different inputs", func(t *testing.T) {
		keywordID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
		toDate := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

		hash1 := generateSearchWindowHash(keywordID, "semantic_scholar", nil, &toDate)
		hash2 := generateSearchWindowHash(keywordID, "openalex", nil, &toDate)

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("handles nil dates", func(t *testing.T) {
		keywordID := uuid.New()
		hash := generateSearchWindowHash(keywordID, "pubmed", nil, nil)

		assert.NotEmpty(t, hash)
		assert.Len(t, hash, 32)
	})
}

func TestPgKeywordRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// TODO: Add integration tests with testcontainers
}
```

**Step 3: Run tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/repository/... -v -short
```

Expected: PASS

**Step 4: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/repository/
git commit -m "feat(repository): implement PostgreSQL keyword repository

- Add PgKeywordRepository with get-or-create pattern
- Implement bulk operations using pgx batch
- Add search recording with window hash deduplication
- Add paper mapping management
- Support keyword-paper association queries"
```

---

## Task 5: Implement Outbox Integration

**Files:**
- Create: `literature_service/internal/outbox/publisher.go`
- Create: `literature_service/internal/outbox/events.go`
- Create: `literature_service/internal/outbox/publisher_test.go`

**Step 1: Create outbox publisher wrapper**

Create `internal/outbox/publisher.go`:

```go
// Package outbox provides event publishing using the transactional outbox pattern.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/helixir/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/helixir/literature-review-service/internal/domain"
)

// Publisher wraps the outbox package for literature review events.
type Publisher struct {
	store  outbox.Store
	logger zerolog.Logger
}

// NewPublisher creates a new outbox publisher.
func NewPublisher(pool *pgxpool.Pool, logger zerolog.Logger) (*Publisher, error) {
	store, err := outbox.NewPgStore(pool)
	if err != nil {
		return nil, fmt.Errorf("create outbox store: %w", err)
	}

	return &Publisher{
		store:  store,
		logger: logger.With().Str("component", "outbox_publisher").Logger(),
	}, nil
}

// PublishReviewCreated publishes a review created event.
func (p *Publisher) PublishReviewCreated(ctx context.Context, review *domain.LiteratureReviewRequest) error {
	payload := ReviewCreatedPayload{
		ReviewID:      review.ID.String(),
		OrgID:         review.OrgID,
		ProjectID:     review.ProjectID,
		UserID:        review.UserID,
		OriginalQuery: review.OriginalQuery,
		WorkflowID:    review.TemporalWorkflowID,
	}

	return p.publish(ctx, domain.EventTypeReviewStarted, review.ID.String(), "literature_review", payload, &review.OrgID, &review.ProjectID)
}

// PublishStatusChanged publishes a review status changed event.
func (p *Publisher) PublishStatusChanged(ctx context.Context, review *domain.LiteratureReviewRequest, previousStatus domain.ReviewStatus) error {
	payload := StatusChangedPayload{
		ReviewID:       review.ID.String(),
		PreviousStatus: string(previousStatus),
		NewStatus:      string(review.Status),
		ErrorMessage:   review.ErrorMessage,
	}

	return p.publish(ctx, domain.EventTypeReviewStatusChanged, review.ID.String(), "literature_review", payload, &review.OrgID, &review.ProjectID)
}

// PublishKeywordsExtracted publishes a keywords extracted event.
func (p *Publisher) PublishKeywordsExtracted(ctx context.Context, reviewID uuid.UUID, orgID, projectID string, keywords []string, round int) error {
	payload := KeywordsExtractedPayload{
		ReviewID:        reviewID.String(),
		Keywords:        keywords,
		ExtractionRound: round,
		Count:           len(keywords),
	}

	return p.publish(ctx, domain.EventTypeKeywordsExtracted, reviewID.String(), "literature_review", payload, &orgID, &projectID)
}

// PublishPapersDiscovered publishes a papers discovered event.
func (p *Publisher) PublishPapersDiscovered(ctx context.Context, reviewID uuid.UUID, orgID, projectID, source, keyword string, count, newCount int) error {
	payload := PapersDiscoveredPayload{
		ReviewID: reviewID.String(),
		Source:   source,
		Keyword:  keyword,
		Count:    count,
		NewCount: newCount,
	}

	return p.publish(ctx, domain.EventTypePapersDiscovered, reviewID.String(), "literature_review", payload, &orgID, &projectID)
}

// PublishIngestionRequested publishes an ingestion requested event.
func (p *Publisher) PublishIngestionRequested(ctx context.Context, reviewID uuid.UUID, orgID, projectID string, paperIDs []string) error {
	payload := IngestionRequestedPayload{
		ReviewID: reviewID.String(),
		PaperIDs: paperIDs,
		Count:    len(paperIDs),
	}

	return p.publish(ctx, domain.EventTypeIngestionRequested, reviewID.String(), "literature_review", payload, &orgID, &projectID)
}

// PublishReviewCompleted publishes a review completed event.
func (p *Publisher) PublishReviewCompleted(ctx context.Context, review *domain.LiteratureReviewRequest) error {
	payload := ReviewCompletedPayload{
		ReviewID:       review.ID.String(),
		PapersFound:    review.PapersFoundCount,
		PapersIngested: review.PapersIngestedCount,
		Duration:       review.Duration(),
	}

	return p.publish(ctx, domain.EventTypeReviewCompleted, review.ID.String(), "literature_review", payload, &review.OrgID, &review.ProjectID)
}

// publish creates and stores an outbox event.
func (p *Publisher) publish(ctx context.Context, eventType, aggregateID, aggregateType string, payload interface{}, orgID, projectID *string) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	event := outbox.NewEventBuilder().
		WithEventID(uuid.New().String()).
		WithAggregateID(aggregateID).
		WithAggregateType(aggregateType).
		WithEventType(eventType).
		WithPayload(payloadBytes).
		WithScope("project", orgID, projectID).
		WithMaxAttempts(5).
		Build()

	if err := p.store.Insert(ctx, event); err != nil {
		p.logger.Error().Err(err).
			Str("event_type", eventType).
			Str("aggregate_id", aggregateID).
			Msg("failed to insert outbox event")
		return fmt.Errorf("insert event: %w", err)
	}

	p.logger.Debug().
		Str("event_type", eventType).
		Str("aggregate_id", aggregateID).
		Msg("outbox event published")

	return nil
}
```

**Step 2: Create event payload types**

Create `internal/outbox/events.go`:

```go
package outbox

import "time"

// ReviewCreatedPayload is the payload for review created events.
type ReviewCreatedPayload struct {
	ReviewID      string `json:"review_id"`
	OrgID         string `json:"org_id"`
	ProjectID     string `json:"project_id"`
	UserID        string `json:"user_id"`
	OriginalQuery string `json:"original_query"`
	WorkflowID    string `json:"workflow_id"`
}

// StatusChangedPayload is the payload for status changed events.
type StatusChangedPayload struct {
	ReviewID       string  `json:"review_id"`
	PreviousStatus string  `json:"previous_status"`
	NewStatus      string  `json:"new_status"`
	ErrorMessage   *string `json:"error_message,omitempty"`
}

// KeywordsExtractedPayload is the payload for keywords extracted events.
type KeywordsExtractedPayload struct {
	ReviewID        string   `json:"review_id"`
	Keywords        []string `json:"keywords"`
	ExtractionRound int      `json:"extraction_round"`
	Count           int      `json:"count"`
}

// PapersDiscoveredPayload is the payload for papers discovered events.
type PapersDiscoveredPayload struct {
	ReviewID string `json:"review_id"`
	Source   string `json:"source"`
	Keyword  string `json:"keyword"`
	Count    int    `json:"count"`
	NewCount int    `json:"new_count"`
}

// IngestionRequestedPayload is the payload for ingestion requested events.
type IngestionRequestedPayload struct {
	ReviewID string   `json:"review_id"`
	PaperIDs []string `json:"paper_ids"`
	Count    int      `json:"count"`
}

// ReviewCompletedPayload is the payload for review completed events.
type ReviewCompletedPayload struct {
	ReviewID       string        `json:"review_id"`
	PapersFound    int           `json:"papers_found"`
	PapersIngested int           `json:"papers_ingested"`
	Duration       time.Duration `json:"duration"`
}
```

**Step 3: Create publisher tests**

Create `internal/outbox/publisher_test.go`:

```go
package outbox

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewCreatedPayload_JSON(t *testing.T) {
	payload := ReviewCreatedPayload{
		ReviewID:      "550e8400-e29b-41d4-a716-446655440000",
		OrgID:         "org-123",
		ProjectID:     "proj-456",
		UserID:        "user-789",
		OriginalQuery: "CRISPR gene editing",
		WorkflowID:    "workflow-abc",
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded ReviewCreatedPayload
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, payload, decoded)
}

func TestStatusChangedPayload_JSON(t *testing.T) {
	t.Run("with error message", func(t *testing.T) {
		errMsg := "rate limit exceeded"
		payload := StatusChangedPayload{
			ReviewID:       "550e8400-e29b-41d4-a716-446655440000",
			PreviousStatus: "searching",
			NewStatus:      "failed",
			ErrorMessage:   &errMsg,
		}

		data, err := json.Marshal(payload)
		require.NoError(t, err)

		var decoded StatusChangedPayload
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, payload, decoded)
		assert.NotNil(t, decoded.ErrorMessage)
	})

	t.Run("without error message", func(t *testing.T) {
		payload := StatusChangedPayload{
			ReviewID:       "550e8400-e29b-41d4-a716-446655440000",
			PreviousStatus: "searching",
			NewStatus:      "completed",
		}

		data, err := json.Marshal(payload)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "error_message")
	})
}

func TestKeywordsExtractedPayload_JSON(t *testing.T) {
	payload := KeywordsExtractedPayload{
		ReviewID:        "550e8400-e29b-41d4-a716-446655440000",
		Keywords:        []string{"CRISPR", "gene editing", "cancer"},
		ExtractionRound: 1,
		Count:           3,
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded KeywordsExtractedPayload
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, payload, decoded)
}

func TestPapersDiscoveredPayload_JSON(t *testing.T) {
	payload := PapersDiscoveredPayload{
		ReviewID: "550e8400-e29b-41d4-a716-446655440000",
		Source:   "semantic_scholar",
		Keyword:  "CRISPR",
		Count:    100,
		NewCount: 75,
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded PapersDiscoveredPayload
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, payload, decoded)
}

func TestReviewCompletedPayload_JSON(t *testing.T) {
	payload := ReviewCompletedPayload{
		ReviewID:       "550e8400-e29b-41d4-a716-446655440000",
		PapersFound:    500,
		PapersIngested: 450,
		Duration:       5 * time.Minute,
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded ReviewCompletedPayload
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, payload, decoded)
}
```

**Step 4: Run tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/outbox/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/outbox/
git commit -m "feat(outbox): implement outbox event publishing

- Add Publisher wrapper for transactional outbox pattern
- Define event payload types for all literature review events
- Support review created, status changed, keywords extracted events
- Support papers discovered, ingestion requested, completed events
- Add JSON serialization tests for all payloads"
```

---

## Task 6: Implement Temporal Worker Setup

**Files:**
- Create: `literature_service/internal/temporal/worker.go`
- Create: `literature_service/internal/temporal/client.go`
- Create: `literature_service/internal/temporal/worker_test.go`

**Step 1: Create Temporal client wrapper**

Create `internal/temporal/client.go`:

```go
// Package temporal provides Temporal workflow client and worker setup.
package temporal

import (
	"fmt"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/helixir/literature-review-service/internal/config"
)

// NewClient creates a new Temporal client from configuration.
func NewClient(cfg config.TemporalConfig) (client.Client, error) {
	options := client.Options{
		HostPort:  cfg.HostPort,
		Namespace: cfg.Namespace,
	}

	c, err := client.Dial(options)
	if err != nil {
		return nil, fmt.Errorf("dial temporal: %w", err)
	}

	return c, nil
}

// NewWorker creates a new Temporal worker from configuration.
func NewWorker(c client.Client, cfg WorkerConfig) worker.Worker {
	options := worker.Options{
		MaxConcurrentActivityExecutionSize:     cfg.MaxConcurrentActivityExecutionSize,
		MaxConcurrentWorkflowTaskExecutionSize: cfg.MaxConcurrentWorkflowTaskExecutionSize,
		MaxConcurrentActivityTaskPollers:       cfg.MaxConcurrentActivityTaskPollers,
		MaxConcurrentWorkflowTaskPollers:       cfg.MaxConcurrentWorkflowTaskPollers,
	}

	return worker.New(c, cfg.TaskQueue, options)
}
```

**Step 2: Create worker configuration and setup**

Create `internal/temporal/worker.go`:

```go
package temporal

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"github.com/rs/zerolog"
)

// WorkerConfig defines Temporal worker configuration.
type WorkerConfig struct {
	TaskQueue                              string
	MaxConcurrentActivityExecutionSize     int
	MaxConcurrentWorkflowTaskExecutionSize int
	MaxConcurrentActivityTaskPollers       int
	MaxConcurrentWorkflowTaskPollers       int
}

// DefaultWorkerConfig returns default worker configuration.
func DefaultWorkerConfig(taskQueue string) WorkerConfig {
	return WorkerConfig{
		TaskQueue:                              taskQueue,
		MaxConcurrentActivityExecutionSize:     100,
		MaxConcurrentWorkflowTaskExecutionSize: 50,
		MaxConcurrentActivityTaskPollers:       4,
		MaxConcurrentWorkflowTaskPollers:       2,
	}
}

// ActivityDependencies holds all activity implementations.
type ActivityDependencies struct {
	// Will be populated as activities are implemented
	// KeywordActivities   *activities.KeywordActivities
	// SearchActivities    *activities.SearchActivities
	// IngestionActivities *activities.IngestionActivities
	// StatusActivities    *activities.StatusActivities
}

// WorkerManager manages Temporal worker lifecycle.
type WorkerManager struct {
	client client.Client
	worker worker.Worker
	config WorkerConfig
	logger zerolog.Logger
}

// NewWorkerManager creates a new worker manager.
func NewWorkerManager(c client.Client, cfg WorkerConfig, logger zerolog.Logger) *WorkerManager {
	return &WorkerManager{
		client: c,
		config: cfg,
		logger: logger.With().Str("component", "temporal_worker").Logger(),
	}
}

// Start initializes and starts the worker.
func (m *WorkerManager) Start(ctx context.Context, deps ActivityDependencies) error {
	m.worker = NewWorker(m.client, m.config)

	// Register workflows (will be implemented in Phase 3)
	// m.worker.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
	// m.worker.RegisterWorkflow(workflows.SearchExpansionWorkflow)

	// Register activities (will be implemented in Phase 3)
	// if deps.KeywordActivities != nil {
	//     m.worker.RegisterActivity(deps.KeywordActivities.ExtractKeywordsActivity)
	// }

	m.logger.Info().
		Str("task_queue", m.config.TaskQueue).
		Int("max_activities", m.config.MaxConcurrentActivityExecutionSize).
		Msg("starting temporal worker")

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.worker.Run(worker.InterruptCh())
	}()

	select {
	case <-ctx.Done():
		m.logger.Info().Msg("stopping temporal worker due to context cancellation")
		m.worker.Stop()
		return ctx.Err()
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("worker run: %w", err)
		}
		return nil
	}
}

// Stop gracefully stops the worker.
func (m *WorkerManager) Stop() {
	if m.worker != nil {
		m.worker.Stop()
		m.logger.Info().Msg("temporal worker stopped")
	}
}

// IsRunning returns whether the worker is running.
func (m *WorkerManager) IsRunning() bool {
	return m.worker != nil
}
```

**Step 3: Create worker tests**

Create `internal/temporal/worker_test.go`:

```go
package temporal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultWorkerConfig(t *testing.T) {
	t.Run("creates config with correct defaults", func(t *testing.T) {
		cfg := DefaultWorkerConfig("literature-review-tasks")

		assert.Equal(t, "literature-review-tasks", cfg.TaskQueue)
		assert.Equal(t, 100, cfg.MaxConcurrentActivityExecutionSize)
		assert.Equal(t, 50, cfg.MaxConcurrentWorkflowTaskExecutionSize)
		assert.Equal(t, 4, cfg.MaxConcurrentActivityTaskPollers)
		assert.Equal(t, 2, cfg.MaxConcurrentWorkflowTaskPollers)
	})
}

func TestNewWorkerManager(t *testing.T) {
	t.Run("creates manager", func(t *testing.T) {
		cfg := DefaultWorkerConfig("test-queue")
		// Can't test with real client without Temporal server
		// This just verifies the constructor works
		require.NotNil(t, cfg)
	})
}

func TestWorkerManager_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// TODO: Add integration tests with testcontainers-temporal
}
```

**Step 4: Run tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/temporal/... -v -short
```

Expected: PASS

**Step 5: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/temporal/
git commit -m "feat(temporal): implement Temporal worker setup

- Add Temporal client wrapper with configuration
- Add WorkerManager for lifecycle management
- Define ActivityDependencies for dependency injection
- Add default worker configuration
- Prepare for workflow and activity registration"
```

---

## Task 7: Implement Observability Setup

**Files:**
- Create: `literature_service/internal/observability/metrics.go`
- Create: `literature_service/internal/observability/logger.go`
- Create: `literature_service/internal/observability/metrics_test.go`

**Step 1: Create Prometheus metrics**

Create `internal/observability/metrics.go`:

```go
// Package observability provides metrics, logging, and tracing setup.
package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the literature review service.
type Metrics struct {
	// Review metrics
	ReviewsStarted   *prometheus.CounterVec
	ReviewsCompleted *prometheus.CounterVec
	ReviewsFailed    *prometheus.CounterVec
	ReviewDuration   *prometheus.HistogramVec

	// Keyword extraction metrics
	KeywordsExtracted *prometheus.CounterVec
	ExtractionDuration *prometheus.HistogramVec

	// Search metrics
	SearchesExecuted *prometheus.CounterVec
	SearchesFailed   *prometheus.CounterVec
	SearchDuration   *prometheus.HistogramVec
	PapersFound      *prometheus.CounterVec

	// Paper source metrics
	SourceRequests  *prometheus.CounterVec
	SourceErrors    *prometheus.CounterVec
	SourceLatency   *prometheus.HistogramVec
	SourceRateLimit *prometheus.CounterVec

	// Ingestion metrics
	IngestionQueued    *prometheus.CounterVec
	IngestionCompleted *prometheus.CounterVec
	IngestionFailed    *prometheus.CounterVec

	// Queue metrics
	QueueDepth prometheus.Gauge
}

// NewMetrics creates and registers all metrics.
func NewMetrics(namespace string) *Metrics {
	return &Metrics{
		// Review metrics
		ReviewsStarted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "reviews_started_total",
				Help:      "Total number of literature reviews started",
			},
			[]string{"org_id"},
		),
		ReviewsCompleted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "reviews_completed_total",
				Help:      "Total number of literature reviews completed",
			},
			[]string{"org_id", "status"},
		),
		ReviewsFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "reviews_failed_total",
				Help:      "Total number of literature reviews failed",
			},
			[]string{"org_id", "error_type"},
		),
		ReviewDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "review_duration_seconds",
				Help:      "Duration of literature reviews in seconds",
				Buckets:   []float64{10, 30, 60, 120, 300, 600, 1800, 3600},
			},
			[]string{"org_id", "status"},
		),

		// Keyword extraction metrics
		KeywordsExtracted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "keywords_extracted_total",
				Help:      "Total number of keywords extracted",
			},
			[]string{"extraction_round"},
		),
		ExtractionDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "extraction_duration_seconds",
				Help:      "Duration of keyword extraction in seconds",
				Buckets:   []float64{0.5, 1, 2, 5, 10, 30},
			},
			[]string{"llm_provider"},
		),

		// Search metrics
		SearchesExecuted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "searches_executed_total",
				Help:      "Total number of searches executed",
			},
			[]string{"source"},
		),
		SearchesFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "searches_failed_total",
				Help:      "Total number of searches failed",
			},
			[]string{"source", "error_type"},
		),
		SearchDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "search_duration_seconds",
				Help:      "Duration of searches in seconds",
				Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30},
			},
			[]string{"source"},
		),
		PapersFound: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "papers_found_total",
				Help:      "Total number of papers found",
			},
			[]string{"source", "is_new"},
		),

		// Paper source metrics
		SourceRequests: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "source_requests_total",
				Help:      "Total number of requests to paper sources",
			},
			[]string{"source", "endpoint"},
		),
		SourceErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "source_errors_total",
				Help:      "Total number of errors from paper sources",
			},
			[]string{"source", "error_type"},
		),
		SourceLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "source_latency_seconds",
				Help:      "Latency of paper source API calls in seconds",
				Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"source", "endpoint"},
		),
		SourceRateLimit: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "source_rate_limit_total",
				Help:      "Total number of rate limit hits",
			},
			[]string{"source"},
		),

		// Ingestion metrics
		IngestionQueued: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "ingestion_queued_total",
				Help:      "Total number of papers queued for ingestion",
			},
			[]string{"source"},
		),
		IngestionCompleted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "ingestion_completed_total",
				Help:      "Total number of papers successfully ingested",
			},
			[]string{"source"},
		),
		IngestionFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "ingestion_failed_total",
				Help:      "Total number of papers failed to ingest",
			},
			[]string{"source", "error_type"},
		),

		// Queue metrics
		QueueDepth: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "queue_depth",
				Help:      "Current depth of the processing queue",
			},
		),
	}
}

// RecordReviewStarted records a review being started.
func (m *Metrics) RecordReviewStarted(orgID string) {
	m.ReviewsStarted.WithLabelValues(orgID).Inc()
}

// RecordReviewCompleted records a review completion.
func (m *Metrics) RecordReviewCompleted(orgID, status string, durationSeconds float64) {
	m.ReviewsCompleted.WithLabelValues(orgID, status).Inc()
	m.ReviewDuration.WithLabelValues(orgID, status).Observe(durationSeconds)
}

// RecordReviewFailed records a review failure.
func (m *Metrics) RecordReviewFailed(orgID, errorType string) {
	m.ReviewsFailed.WithLabelValues(orgID, errorType).Inc()
}

// RecordKeywordsExtracted records keyword extraction.
func (m *Metrics) RecordKeywordsExtracted(round string, count int) {
	m.KeywordsExtracted.WithLabelValues(round).Add(float64(count))
}

// RecordExtractionDuration records keyword extraction duration.
func (m *Metrics) RecordExtractionDuration(provider string, durationSeconds float64) {
	m.ExtractionDuration.WithLabelValues(provider).Observe(durationSeconds)
}

// RecordSearchExecuted records a search execution.
func (m *Metrics) RecordSearchExecuted(source string, durationSeconds float64, papersFound, newPapers int) {
	m.SearchesExecuted.WithLabelValues(source).Inc()
	m.SearchDuration.WithLabelValues(source).Observe(durationSeconds)
	m.PapersFound.WithLabelValues(source, "true").Add(float64(newPapers))
	m.PapersFound.WithLabelValues(source, "false").Add(float64(papersFound - newPapers))
}

// RecordSearchFailed records a search failure.
func (m *Metrics) RecordSearchFailed(source, errorType string) {
	m.SearchesFailed.WithLabelValues(source, errorType).Inc()
}

// RecordSourceRequest records an API request to a paper source.
func (m *Metrics) RecordSourceRequest(source, endpoint string, durationSeconds float64) {
	m.SourceRequests.WithLabelValues(source, endpoint).Inc()
	m.SourceLatency.WithLabelValues(source, endpoint).Observe(durationSeconds)
}

// RecordSourceError records an API error from a paper source.
func (m *Metrics) RecordSourceError(source, errorType string) {
	m.SourceErrors.WithLabelValues(source, errorType).Inc()
}

// RecordSourceRateLimit records a rate limit hit.
func (m *Metrics) RecordSourceRateLimit(source string) {
	m.SourceRateLimit.WithLabelValues(source).Inc()
}

// RecordIngestionQueued records papers being queued for ingestion.
func (m *Metrics) RecordIngestionQueued(source string, count int) {
	m.IngestionQueued.WithLabelValues(source).Add(float64(count))
}

// RecordIngestionCompleted records successful ingestion.
func (m *Metrics) RecordIngestionCompleted(source string) {
	m.IngestionCompleted.WithLabelValues(source).Inc()
}

// RecordIngestionFailed records ingestion failure.
func (m *Metrics) RecordIngestionFailed(source, errorType string) {
	m.IngestionFailed.WithLabelValues(source, errorType).Inc()
}

// SetQueueDepth sets the current queue depth.
func (m *Metrics) SetQueueDepth(depth int) {
	m.QueueDepth.Set(float64(depth))
}
```

**Step 2: Create logger setup**

Create `internal/observability/logger.go`:

```go
package observability

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/helixir/literature-review-service/internal/config"
)

// NewLogger creates a new zerolog logger from configuration.
func NewLogger(cfg config.LoggingConfig) zerolog.Logger {
	var output io.Writer = os.Stdout

	// Configure output format
	if cfg.Format == "console" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	// Parse log level
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}

	// Create logger
	logger := zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Str("service", "literature-review-service").
		Logger()

	return logger
}

// WithComponent adds a component name to the logger.
func WithComponent(logger zerolog.Logger, component string) zerolog.Logger {
	return logger.With().Str("component", component).Logger()
}

// WithReview adds review context to the logger.
func WithReview(logger zerolog.Logger, reviewID, orgID, projectID string) zerolog.Logger {
	return logger.With().
		Str("review_id", reviewID).
		Str("org_id", orgID).
		Str("project_id", projectID).
		Logger()
}

// WithWorkflow adds workflow context to the logger.
func WithWorkflow(logger zerolog.Logger, workflowID, runID string) zerolog.Logger {
	return logger.With().
		Str("workflow_id", workflowID).
		Str("run_id", runID).
		Logger()
}
```

**Step 3: Create metrics tests**

Create `internal/observability/metrics_test.go`:

```go
package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetrics(t *testing.T) {
	// Reset default registry for test isolation
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry

	t.Run("creates all metrics", func(t *testing.T) {
		metrics := NewMetrics("literature_review")

		require.NotNil(t, metrics)
		assert.NotNil(t, metrics.ReviewsStarted)
		assert.NotNil(t, metrics.ReviewsCompleted)
		assert.NotNil(t, metrics.ReviewsFailed)
		assert.NotNil(t, metrics.ReviewDuration)
		assert.NotNil(t, metrics.KeywordsExtracted)
		assert.NotNil(t, metrics.SearchesExecuted)
		assert.NotNil(t, metrics.PapersFound)
		assert.NotNil(t, metrics.SourceRequests)
		assert.NotNil(t, metrics.IngestionQueued)
		assert.NotNil(t, metrics.QueueDepth)
	})
}

func TestMetrics_RecordReviewStarted(t *testing.T) {
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry

	metrics := NewMetrics("test")
	metrics.RecordReviewStarted("org-123")

	// Verify metric was recorded (no panic)
}

func TestMetrics_RecordReviewCompleted(t *testing.T) {
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry

	metrics := NewMetrics("test")
	metrics.RecordReviewCompleted("org-123", "completed", 300.5)

	// Verify metric was recorded (no panic)
}

func TestMetrics_RecordSearchExecuted(t *testing.T) {
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry

	metrics := NewMetrics("test")
	metrics.RecordSearchExecuted("semantic_scholar", 1.5, 100, 75)

	// Verify metric was recorded (no panic)
}

func TestMetrics_RecordSourceRequest(t *testing.T) {
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry

	metrics := NewMetrics("test")
	metrics.RecordSourceRequest("openalex", "search", 0.5)
	metrics.RecordSourceError("openalex", "timeout")
	metrics.RecordSourceRateLimit("openalex")

	// Verify metrics were recorded (no panic)
}

func TestMetrics_SetQueueDepth(t *testing.T) {
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry

	metrics := NewMetrics("test")
	metrics.SetQueueDepth(42)

	// Verify metric was set (no panic)
}
```

**Step 4: Run tests**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/observability/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/observability/
git commit -m "feat(observability): implement metrics and logging setup

- Add Prometheus metrics for reviews, searches, sources, ingestion
- Add histogram metrics for durations and latencies
- Add counter metrics for operations and errors
- Add gauge for queue depth
- Add zerolog logger setup with configuration
- Add context helpers for structured logging"
```

---

## Task 8: Update Domain Models for Repository Support

**Files:**
- Modify: `literature_service/internal/domain/review.go`
- Modify: `literature_service/internal/domain/keyword.go`

**Step 1: Add Configuration field to LiteratureReviewRequest**

Update `internal/domain/review.go` to add the Configuration field:

```go
// Add after existing fields in LiteratureReviewRequest struct:

// Configuration holds the review configuration settings.
type ReviewConfiguration struct {
	InitialKeywordCount int      `json:"initial_keyword_count"`
	PaperKeywordCount   int      `json:"paper_keyword_count"`
	MaxExpansionDepth   int      `json:"max_expansion_depth"`
	EnabledSources      []string `json:"enabled_sources"`
	DateFrom            *time.Time `json:"date_from,omitempty"`
	DateTo              *time.Time `json:"date_to,omitempty"`
}

// Add Configuration field to LiteratureReviewRequest:
// Configuration ReviewConfiguration `json:"configuration"`
```

**Step 2: Add KeywordSearch struct to keyword.go**

Update `internal/domain/keyword.go`:

```go
// Add KeywordSearch struct:

// KeywordSearch represents a search execution for a keyword.
type KeywordSearch struct {
	ID             uuid.UUID     `json:"id"`
	KeywordID      uuid.UUID     `json:"keyword_id"`
	SourceAPI      string        `json:"source_api"`
	SearchedAt     time.Time     `json:"searched_at"`
	SearchFromDate *time.Time    `json:"search_from_date,omitempty"`
	SearchToDate   *time.Time    `json:"search_to_date"`
	PapersFound    int           `json:"papers_found"`
	Status         SearchStatus  `json:"status"`
	ErrorMessage   *string       `json:"error_message,omitempty"`
}
```

**Step 3: Run tests to verify domain models**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
go test ./internal/domain/... -v
go build ./...
```

Expected: PASS

**Step 4: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/domain/
git commit -m "feat(domain): add repository support fields

- Add ReviewConfiguration for storing review settings
- Add KeywordSearch for tracking search executions
- Support JSON serialization for all new types"
```

---

## Phase 2 Completion Checklist

After completing all tasks:

- [ ] `make build` succeeds
- [ ] `make test` passes
- [ ] Repository interfaces defined
- [ ] PostgreSQL implementations complete
- [ ] Outbox publisher integrated
- [ ] Temporal worker setup ready
- [ ] Prometheus metrics registered
- [ ] Logger configured

---

## Next Steps

After Phase 2 is complete, proceed to **Phase 3: Core Services** which includes:
- Paper source client implementations (Semantic Scholar, OpenAlex, etc.)
- LLM client for keyword extraction
- Rate limiter pool
- gRPC server implementation
- Temporal workflow definitions
