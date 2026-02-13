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

// txBeginner is an interface for types that can begin a transaction (e.g., *pgxpool.Pool, *database.DB).
// Used by Update to automatically wrap SELECT FOR UPDATE + UPDATE in a transaction
// when the underlying DBTX is a pool rather than an existing transaction.
type txBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PostgreSQL error codes used for constraint violation detection.
const (
	pgUniqueViolation     = "23505" // unique_violation
	pgForeignKeyViolation = "23503" // foreign_key_violation
)

// validStatusTransitions defines the allowed status transitions for review requests.
// This is a package-level variable to avoid re-allocating on every call.
var validStatusTransitions = map[domain.ReviewStatus][]domain.ReviewStatus{
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
		domain.ReviewStatusPartial,
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
		domain.ReviewStatusReviewing,
		domain.ReviewStatusCompleted,
		domain.ReviewStatusPartial,
		domain.ReviewStatusFailed,
		domain.ReviewStatusCancelled,
	},
	domain.ReviewStatusReviewing: {
		domain.ReviewStatusCompleted,
		domain.ReviewStatusPartial,
		domain.ReviewStatusFailed,
		domain.ReviewStatusCancelled,
	},
}

// Compile-time interface verification.
var _ ReviewRepository = (*PgReviewRepository)(nil)

// PgReviewRepository is a PostgreSQL implementation of ReviewRepository.
type PgReviewRepository struct {
	db DBTX
}

// NewPgReviewRepository creates a new PostgreSQL review repository.
func NewPgReviewRepository(db DBTX) *PgReviewRepository {
	return &PgReviewRepository{db: db}
}

// Create inserts a new literature review request.
func (r *PgReviewRepository) Create(ctx context.Context, review *domain.LiteratureReviewRequest) error {
	if review == nil {
		return domain.NewValidationError("review", "review cannot be nil")
	}
	if review.ID == uuid.Nil {
		return domain.NewValidationError("id", "review ID is required")
	}
	if review.OrgID == "" {
		return domain.NewValidationError("org_id", "organization ID is required")
	}
	if review.ProjectID == "" {
		return domain.NewValidationError("project_id", "project ID is required")
	}
	if review.UserID == "" {
		return domain.NewValidationError("user_id", "user ID is required")
	}

	configJSON, err := json.Marshal(review.Configuration)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	sourceFiltersJSON, err := json.Marshal(review.SourceFilters)
	if err != nil {
		return fmt.Errorf("failed to marshal source filters: %w", err)
	}

	seedKeywordsJSON, err := json.Marshal(review.SeedKeywords)
	if err != nil {
		return fmt.Errorf("failed to marshal seed keywords: %w", err)
	}

	query := `
		INSERT INTO literature_review_requests (
			id, org_id, project_id, user_id, title,
			description, seed_keywords,
			temporal_workflow_id, temporal_run_id, status,
			keywords_found_count, papers_found_count, papers_ingested_count, papers_failed_count,
			config_snapshot, source_filters,
			coverage_score, coverage_reasoning,
			created_at, updated_at, started_at, completed_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7,
			$8, $9, $10,
			$11, $12, $13, $14,
			$15, $16,
			$17, $18,
			$19, $20, $21, $22
		)`

	_, err = r.db.Exec(ctx, query,
		review.ID, review.OrgID, review.ProjectID, review.UserID, review.Title,
		review.Description, seedKeywordsJSON,
		nullString(review.TemporalWorkflowID), nullString(review.TemporalRunID), review.Status,
		review.KeywordsFoundCount, review.PapersFoundCount, review.PapersIngestedCount, review.PapersFailedCount,
		configJSON, sourceFiltersJSON,
		review.CoverageScore, nullString(review.CoverageReasoning),
		review.CreatedAt, review.UpdatedAt, review.StartedAt, review.CompletedAt,
	)

	if err != nil {
		if isPgUniqueViolation(err) {
			return domain.NewAlreadyExistsError("review", review.ID.String())
		}
		return fmt.Errorf("failed to create review: %w", err)
	}

	return nil
}

// Get retrieves a literature review request by its ID within a tenant context.
func (r *PgReviewRepository) Get(ctx context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
	query := `
		SELECT id, org_id, project_id, user_id, title,
			description, seed_keywords,
			temporal_workflow_id, temporal_run_id, status,
			keywords_found_count, papers_found_count, papers_ingested_count, papers_failed_count,
			config_snapshot, source_filters,
			coverage_score, coverage_reasoning,
			created_at, updated_at, started_at, completed_at,
			pause_reason, paused_at, paused_at_phase
		FROM literature_review_requests
		WHERE id = $1 AND org_id = $2 AND project_id = $3`

	row := r.db.QueryRow(ctx, query, id, orgID, projectID)
	review, err := scanReview(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("review", id.String())
		}
		return nil, fmt.Errorf("failed to get review: %w", err)
	}

	return review, nil
}

// Update performs an optimistic update on a literature review request using SELECT FOR UPDATE.
//
// Transaction Management:
// This method uses SELECT FOR UPDATE which requires a transaction for correct locking.
// If the underlying DBTX is a connection pool (supports Begin), the method automatically
// wraps the SELECT FOR UPDATE + UPDATE in an explicit transaction. If the underlying
// DBTX is already a transaction, it executes within that existing transaction.
//
// Callers may still provide their own transaction if they need to include additional
// operations in the same atomic unit:
//
//	tx, err := pool.Begin(ctx)
//	if err != nil { return err }
//	defer tx.Rollback(ctx)
//
//	repo := NewPgReviewRepository(tx)
//	err = repo.Update(ctx, orgID, projectID, id, func(r *domain.LiteratureReviewRequest) error {
//	    r.Status = domain.ReviewStatusSearching
//	    return nil
//	})
//	if err != nil { return err }
//
//	return tx.Commit(ctx)
func (r *PgReviewRepository) Update(ctx context.Context, orgID, projectID string, id uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
	// If the underlying DBTX supports Begin (i.e., it's a pool, not already a transaction),
	// wrap the SELECT FOR UPDATE + UPDATE in an explicit transaction to prevent lost updates.
	if beginner, ok := r.db.(txBeginner); ok {
		tx, err := beginner.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction for update: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		txRepo := &PgReviewRepository{db: tx}
		if err := txRepo.updateInTx(ctx, orgID, projectID, id, fn); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	// Already running within a transaction — execute directly.
	return r.updateInTx(ctx, orgID, projectID, id, fn)
}

// updateInTx performs the actual SELECT FOR UPDATE + UPDATE within the current DBTX.
// This must be called within a transaction for correct row-level locking.
func (r *PgReviewRepository) updateInTx(ctx context.Context, orgID, projectID string, id uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
	selectQuery := `
		SELECT id, org_id, project_id, user_id, title,
			description, seed_keywords,
			temporal_workflow_id, temporal_run_id, status,
			keywords_found_count, papers_found_count, papers_ingested_count, papers_failed_count,
			config_snapshot, source_filters,
			coverage_score, coverage_reasoning,
			created_at, updated_at, started_at, completed_at,
			pause_reason, paused_at, paused_at_phase
		FROM literature_review_requests
		WHERE id = $1 AND org_id = $2 AND project_id = $3
		FOR UPDATE`

	rows, err := r.db.Query(ctx, selectQuery, id, orgID, projectID)
	if err != nil {
		return fmt.Errorf("failed to query review for update: %w", err)
	}

	review, err := scanReviewRows(rows)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NewNotFoundError("review", id.String())
		}
		return fmt.Errorf("failed to scan review: %w", err)
	}

	// Apply the update function
	if err := fn(review); err != nil {
		return err
	}

	// Update the timestamp
	review.UpdatedAt = time.Now().UTC()

	// Persist the updated review
	configJSON, err := json.Marshal(review.Configuration)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	sourceFiltersJSON, err := json.Marshal(review.SourceFilters)
	if err != nil {
		return fmt.Errorf("failed to marshal source filters: %w", err)
	}

	seedKeywordsJSON, err := json.Marshal(review.SeedKeywords)
	if err != nil {
		return fmt.Errorf("failed to marshal seed keywords: %w", err)
	}

	updateQuery := `
		UPDATE literature_review_requests SET
			title = $1,
			description = $2,
			seed_keywords = $3,
			temporal_workflow_id = $4,
			temporal_run_id = $5,
			status = $6,
			keywords_found_count = $7,
			papers_found_count = $8,
			papers_ingested_count = $9,
			papers_failed_count = $10,
			config_snapshot = $11,
			source_filters = $12,
			coverage_score = $13,
			coverage_reasoning = $14,
			updated_at = $15,
			started_at = $16,
			completed_at = $17
		WHERE id = $18 AND org_id = $19 AND project_id = $20`

	_, err = r.db.Exec(ctx, updateQuery,
		review.Title,
		review.Description,
		seedKeywordsJSON,
		nullString(review.TemporalWorkflowID),
		nullString(review.TemporalRunID),
		review.Status,
		review.KeywordsFoundCount,
		review.PapersFoundCount,
		review.PapersIngestedCount,
		review.PapersFailedCount,
		configJSON,
		sourceFiltersJSON,
		review.CoverageScore,
		nullString(review.CoverageReasoning),
		review.UpdatedAt,
		review.StartedAt,
		review.CompletedAt,
		id, orgID, projectID,
	)

	if err != nil {
		return fmt.Errorf("failed to update review: %w", err)
	}

	return nil
}

// UpdateStatus updates the status of a literature review request with optional error message.
func (r *PgReviewRepository) UpdateStatus(ctx context.Context, orgID, projectID string, id uuid.UUID, status domain.ReviewStatus, errorMsg string) error {
	return r.Update(ctx, orgID, projectID, id, func(review *domain.LiteratureReviewRequest) error {
		// Validate status transition
		if !isValidStatusTransition(review.Status, status) {
			return fmt.Errorf("invalid status transition from %s to %s: %w",
				review.Status, status, domain.ErrInvalidInput)
		}

		review.Status = status

		// Set timestamps based on status
		now := time.Now().UTC()
		if status == domain.ReviewStatusExtractingKeywords && review.StartedAt == nil {
			review.StartedAt = &now
		}
		if status.IsTerminal() && review.CompletedAt == nil {
			review.CompletedAt = &now
		}

		return nil
	})
}

// List retrieves literature review requests matching the filter criteria.
func (r *PgReviewRepository) List(ctx context.Context, filter ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
	if err := filter.Validate(); err != nil {
		return nil, 0, err
	}

	// Build dynamic WHERE clause
	conditions := []string{"org_id = $1"}
	args := []interface{}{filter.OrgID}
	argIndex := 2

	if filter.ProjectID != "" {
		conditions = append(conditions, fmt.Sprintf("project_id = $%d", argIndex))
		args = append(args, filter.ProjectID)
		argIndex++
	}

	if len(filter.Status) > 0 {
		placeholders := make([]string, len(filter.Status))
		for i, s := range filter.Status {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, s)
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ", ")))
	}

	if filter.CreatedAfter != nil {
		conditions = append(conditions, fmt.Sprintf("created_at > $%d", argIndex))
		args = append(args, *filter.CreatedAfter)
		argIndex++
	}

	if filter.CreatedBefore != nil {
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", argIndex))
		args = append(args, *filter.CreatedBefore)
		argIndex++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total matching records
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM literature_review_requests WHERE %s", whereClause)
	var totalCount int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to count reviews: %w", err)
	}

	// Query with pagination
	selectQuery := fmt.Sprintf(`
		SELECT id, org_id, project_id, user_id, title,
			description, seed_keywords,
			temporal_workflow_id, temporal_run_id, status,
			keywords_found_count, papers_found_count, papers_ingested_count, papers_failed_count,
			config_snapshot, source_filters,
			coverage_score, coverage_reasoning,
			created_at, updated_at, started_at, completed_at,
			pause_reason, paused_at, paused_at_phase
		FROM literature_review_requests
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`,
		whereClause, argIndex, argIndex+1)

	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list reviews: %w", err)
	}
	defer rows.Close()

	reviews := make([]*domain.LiteratureReviewRequest, 0, filter.Limit)
	for rows.Next() {
		review, err := scanReviewFromRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan review: %w", err)
		}
		reviews = append(reviews, review)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating reviews: %w", err)
	}

	return reviews, totalCount, nil
}

// IncrementCounters atomically increments the papers found and ingested counters.
func (r *PgReviewRepository) IncrementCounters(ctx context.Context, orgID, projectID string, id uuid.UUID, papersFound, papersIngested int) error {
	query := `
		UPDATE literature_review_requests
		SET papers_found_count = papers_found_count + $1,
			papers_ingested_count = papers_ingested_count + $2,
			updated_at = $3
		WHERE id = $4 AND org_id = $5 AND project_id = $6`

	result, err := r.db.Exec(ctx, query,
		papersFound,
		papersIngested,
		time.Now().UTC(),
		id, orgID, projectID,
	)

	if err != nil {
		return fmt.Errorf("failed to increment counters: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.NewNotFoundError("review", id.String())
	}

	return nil
}

// GetByWorkflowID retrieves a literature review request by its Temporal workflow ID.
func (r *PgReviewRepository) GetByWorkflowID(ctx context.Context, workflowID string) (*domain.LiteratureReviewRequest, error) {
	if workflowID == "" {
		return nil, domain.NewValidationError("workflow_id", "workflow ID is required")
	}

	query := `
		SELECT id, org_id, project_id, user_id, title,
			description, seed_keywords,
			temporal_workflow_id, temporal_run_id, status,
			keywords_found_count, papers_found_count, papers_ingested_count, papers_failed_count,
			config_snapshot, source_filters,
			coverage_score, coverage_reasoning,
			created_at, updated_at, started_at, completed_at,
			pause_reason, paused_at, paused_at_phase
		FROM literature_review_requests
		WHERE temporal_workflow_id = $1`

	row := r.db.QueryRow(ctx, query, workflowID)
	review, err := scanReview(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewNotFoundError("review", workflowID)
		}
		return nil, fmt.Errorf("failed to get review by workflow ID: %w", err)
	}

	return review, nil
}

// FindPausedByReason returns all paused reviews matching the given org, project, and reason.
func (r *PgReviewRepository) FindPausedByReason(ctx context.Context, orgID, projectID string, reason domain.PauseReason) ([]*domain.LiteratureReviewRequest, error) {
	if orgID == "" {
		return nil, domain.NewValidationError("org_id", "organization ID is required")
	}

	query := `
		SELECT id, org_id, project_id, user_id, title,
			description, seed_keywords,
			temporal_workflow_id, temporal_run_id, status,
			keywords_found_count, papers_found_count, papers_ingested_count, papers_failed_count,
			config_snapshot, source_filters,
			coverage_score, coverage_reasoning,
			created_at, updated_at, started_at, completed_at,
			pause_reason, paused_at, paused_at_phase
		FROM literature_review_requests
		WHERE org_id = $1
		  AND ($2 = '' OR project_id = $2)
		  AND status = 'paused'
		  AND ($3 = '' OR pause_reason = $3)
		ORDER BY paused_at ASC`

	rows, err := r.db.Query(ctx, query, orgID, projectID, string(reason))
	if err != nil {
		return nil, fmt.Errorf("failed to query paused reviews: %w", err)
	}
	defer rows.Close()

	var reviews []*domain.LiteratureReviewRequest
	for rows.Next() {
		review, err := scanReviewFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan paused review: %w", err)
		}
		reviews = append(reviews, review)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating paused reviews: %w", err)
	}

	return reviews, nil
}

// isValidStatusTransition validates that a status transition is allowed.
func isValidStatusTransition(from, to domain.ReviewStatus) bool {
	// Terminal states cannot transition to anything.
	if from.IsTerminal() {
		return false
	}

	allowed, ok := validStatusTransitions[from]
	if !ok {
		return false
	}

	for _, s := range allowed {
		if s == to {
			return true
		}
	}

	return false
}

// isPgUniqueViolation checks if the error is a PostgreSQL unique constraint violation.
func isPgUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolation
	}
	return false
}

// reviewScanDest holds the destination pointers for scanning a LiteratureReviewRequest row.
// This eliminates code duplication between pgx.Row and pgx.Rows scanning.
type reviewScanDest struct {
	review             domain.LiteratureReviewRequest
	configJSON         []byte
	sourceFiltersJSON  []byte
	seedKeywordsJSON   []byte
	temporalWorkflowID *string
	temporalRunID      *string
	coverageReasoning  *string
	pauseReason        *string
	pausedAtPhase      *string
}

// destinations returns the slice of pointers for Scan operations.
func (d *reviewScanDest) destinations() []interface{} {
	return []interface{}{
		&d.review.ID, &d.review.OrgID, &d.review.ProjectID, &d.review.UserID, &d.review.Title,
		&d.review.Description, &d.seedKeywordsJSON,
		&d.temporalWorkflowID, &d.temporalRunID, &d.review.Status,
		&d.review.KeywordsFoundCount, &d.review.PapersFoundCount, &d.review.PapersIngestedCount, &d.review.PapersFailedCount,
		&d.configJSON, &d.sourceFiltersJSON,
		&d.review.CoverageScore, &d.coverageReasoning,
		&d.review.CreatedAt, &d.review.UpdatedAt, &d.review.StartedAt, &d.review.CompletedAt,
		&d.pauseReason, &d.review.PausedAt, &d.pausedAtPhase,
	}
}

// finalize performs post-scan processing: sets nullable string fields and unmarshals JSON.
func (d *reviewScanDest) finalize() (*domain.LiteratureReviewRequest, error) {
	if d.temporalWorkflowID != nil {
		d.review.TemporalWorkflowID = *d.temporalWorkflowID
	}
	if d.temporalRunID != nil {
		d.review.TemporalRunID = *d.temporalRunID
	}
	if d.coverageReasoning != nil {
		d.review.CoverageReasoning = *d.coverageReasoning
	}
	if d.pauseReason != nil {
		d.review.PauseReason = domain.PauseReason(*d.pauseReason)
	}
	if d.pausedAtPhase != nil {
		d.review.PausedAtPhase = *d.pausedAtPhase
	}

	if len(d.configJSON) > 0 {
		if err := json.Unmarshal(d.configJSON, &d.review.Configuration); err != nil {
			return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
		}
	}

	if len(d.sourceFiltersJSON) > 0 {
		var sourceFilters domain.SourceFilters
		if err := json.Unmarshal(d.sourceFiltersJSON, &sourceFilters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal source filters: %w", err)
		}
		d.review.SourceFilters = &sourceFilters
	}

	if len(d.seedKeywordsJSON) > 0 {
		if err := json.Unmarshal(d.seedKeywordsJSON, &d.review.SeedKeywords); err != nil {
			return nil, fmt.Errorf("failed to unmarshal seed keywords: %w", err)
		}
	}

	return &d.review, nil
}

// scanReview scans a single row into a LiteratureReviewRequest.
func scanReview(row pgx.Row) (*domain.LiteratureReviewRequest, error) {
	var dest reviewScanDest
	if err := row.Scan(dest.destinations()...); err != nil {
		return nil, err
	}
	return dest.finalize()
}

// scanReviewRows scans a single row from pgx.Rows into a LiteratureReviewRequest.
// This is used with SELECT FOR UPDATE which returns Rows instead of Row.
func scanReviewRows(rows pgx.Rows) (*domain.LiteratureReviewRequest, error) {
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, pgx.ErrNoRows
	}

	return scanReviewFromRows(rows)
}

// scanReviewFromRows scans the current row from pgx.Rows into a LiteratureReviewRequest.
func scanReviewFromRows(rows pgx.Rows) (*domain.LiteratureReviewRequest, error) {
	var dest reviewScanDest
	if err := rows.Scan(dest.destinations()...); err != nil {
		return nil, err
	}
	return dest.finalize()
}

// UpdatePauseState updates the pause-related fields of a review request.
func (r *PgReviewRepository) UpdatePauseState(
	ctx context.Context,
	orgID, projectID string,
	requestID uuid.UUID,
	status domain.ReviewStatus,
	pauseReason domain.PauseReason,
	pausedAtPhase string,
) error {
	query := `
		UPDATE literature_review_requests
		SET status = $1,
			pause_reason = $2,
			paused_at = CURRENT_TIMESTAMP,
			paused_at_phase = $3,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $4 AND org_id = $5 AND project_id = $6`

	result, err := r.db.Exec(ctx, query,
		string(status), string(pauseReason), pausedAtPhase,
		requestID, orgID, projectID,
	)
	if err != nil {
		return fmt.Errorf("update pause state: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.NewNotFoundError("review", requestID.String())
	}

	return nil
}

// nullString returns a pointer to the string if non-empty, otherwise nil.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
