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

	configJSON, err := json.Marshal(review.ConfigSnapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal config snapshot: %w", err)
	}

	sourceFiltersJSON, err := json.Marshal(review.SourceFilters)
	if err != nil {
		return fmt.Errorf("failed to marshal source filters: %w", err)
	}

	query := `
		INSERT INTO literature_review_requests (
			id, org_id, project_id, user_id, original_query,
			temporal_workflow_id, temporal_run_id, status,
			keywords_found_count, papers_found_count, papers_ingested_count, papers_failed_count,
			expansion_depth, config_snapshot, source_filters, date_from, date_to,
			created_at, updated_at, started_at, completed_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11, $12,
			$13, $14, $15, $16, $17,
			$18, $19, $20, $21
		)`

	_, err = r.db.Exec(ctx, query,
		review.ID, review.OrgID, review.ProjectID, review.UserID, review.OriginalQuery,
		nullString(review.TemporalWorkflowID), nullString(review.TemporalRunID), review.Status,
		review.KeywordsFoundCount, review.PapersFoundCount, review.PapersIngestedCount, review.PapersFailedCount,
		review.ExpansionDepth, configJSON, sourceFiltersJSON, review.DateFrom, review.DateTo,
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
		SELECT id, org_id, project_id, user_id, original_query,
			temporal_workflow_id, temporal_run_id, status,
			keywords_found_count, papers_found_count, papers_ingested_count, papers_failed_count,
			expansion_depth, config_snapshot, source_filters, date_from, date_to,
			created_at, updated_at, started_at, completed_at
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
func (r *PgReviewRepository) Update(ctx context.Context, orgID, projectID string, id uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
	// Use a transaction with SELECT FOR UPDATE to acquire a row lock.
	// If the caller passes a transaction via DBTX, we use that directly.
	// This approach requires the caller to manage the transaction when using DBTX.

	selectQuery := `
		SELECT id, org_id, project_id, user_id, original_query,
			temporal_workflow_id, temporal_run_id, status,
			keywords_found_count, papers_found_count, papers_ingested_count, papers_failed_count,
			expansion_depth, config_snapshot, source_filters, date_from, date_to,
			created_at, updated_at, started_at, completed_at
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
	configJSON, err := json.Marshal(review.ConfigSnapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal config snapshot: %w", err)
	}

	sourceFiltersJSON, err := json.Marshal(review.SourceFilters)
	if err != nil {
		return fmt.Errorf("failed to marshal source filters: %w", err)
	}

	updateQuery := `
		UPDATE literature_review_requests SET
			original_query = $1,
			temporal_workflow_id = $2,
			temporal_run_id = $3,
			status = $4,
			keywords_found_count = $5,
			papers_found_count = $6,
			papers_ingested_count = $7,
			papers_failed_count = $8,
			expansion_depth = $9,
			config_snapshot = $10,
			source_filters = $11,
			date_from = $12,
			date_to = $13,
			updated_at = $14,
			started_at = $15,
			completed_at = $16
		WHERE id = $17 AND org_id = $18 AND project_id = $19`

	_, err = r.db.Exec(ctx, updateQuery,
		review.OriginalQuery,
		nullString(review.TemporalWorkflowID),
		nullString(review.TemporalRunID),
		review.Status,
		review.KeywordsFoundCount,
		review.PapersFoundCount,
		review.PapersIngestedCount,
		review.PapersFailedCount,
		review.ExpansionDepth,
		configJSON,
		sourceFiltersJSON,
		review.DateFrom,
		review.DateTo,
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
		SELECT id, org_id, project_id, user_id, original_query,
			temporal_workflow_id, temporal_run_id, status,
			keywords_found_count, papers_found_count, papers_ingested_count, papers_failed_count,
			expansion_depth, config_snapshot, source_filters, date_from, date_to,
			created_at, updated_at, started_at, completed_at
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

	var reviews []*domain.LiteratureReviewRequest
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
		SELECT id, org_id, project_id, user_id, original_query,
			temporal_workflow_id, temporal_run_id, status,
			keywords_found_count, papers_found_count, papers_ingested_count, papers_failed_count,
			expansion_depth, config_snapshot, source_filters, date_from, date_to,
			created_at, updated_at, started_at, completed_at
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

// isValidStatusTransition validates that a status transition is allowed.
func isValidStatusTransition(from, to domain.ReviewStatus) bool {
	// Terminal states cannot transition to anything
	if from.IsTerminal() {
		return false
	}

	// Define valid transitions
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
			domain.ReviewStatusCompleted,
			domain.ReviewStatusPartial,
			domain.ReviewStatusFailed,
			domain.ReviewStatusCancelled,
		},
	}

	allowed, ok := validTransitions[from]
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

// isPgUniqueViolation checks if the error is a PostgreSQL unique constraint violation (error code 23505).
func isPgUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// scanReview scans a single row into a LiteratureReviewRequest.
func scanReview(row pgx.Row) (*domain.LiteratureReviewRequest, error) {
	var review domain.LiteratureReviewRequest
	var configJSON, sourceFiltersJSON []byte
	var temporalWorkflowID, temporalRunID *string

	err := row.Scan(
		&review.ID, &review.OrgID, &review.ProjectID, &review.UserID, &review.OriginalQuery,
		&temporalWorkflowID, &temporalRunID, &review.Status,
		&review.KeywordsFoundCount, &review.PapersFoundCount, &review.PapersIngestedCount, &review.PapersFailedCount,
		&review.ExpansionDepth, &configJSON, &sourceFiltersJSON, &review.DateFrom, &review.DateTo,
		&review.CreatedAt, &review.UpdatedAt, &review.StartedAt, &review.CompletedAt,
	)
	if err != nil {
		return nil, err
	}

	if temporalWorkflowID != nil {
		review.TemporalWorkflowID = *temporalWorkflowID
	}
	if temporalRunID != nil {
		review.TemporalRunID = *temporalRunID
	}

	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &review.ConfigSnapshot); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config snapshot: %w", err)
		}
	}

	if len(sourceFiltersJSON) > 0 {
		if err := json.Unmarshal(sourceFiltersJSON, &review.SourceFilters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal source filters: %w", err)
		}
	}

	return &review, nil
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
	var review domain.LiteratureReviewRequest
	var configJSON, sourceFiltersJSON []byte
	var temporalWorkflowID, temporalRunID *string

	err := rows.Scan(
		&review.ID, &review.OrgID, &review.ProjectID, &review.UserID, &review.OriginalQuery,
		&temporalWorkflowID, &temporalRunID, &review.Status,
		&review.KeywordsFoundCount, &review.PapersFoundCount, &review.PapersIngestedCount, &review.PapersFailedCount,
		&review.ExpansionDepth, &configJSON, &sourceFiltersJSON, &review.DateFrom, &review.DateTo,
		&review.CreatedAt, &review.UpdatedAt, &review.StartedAt, &review.CompletedAt,
	)
	if err != nil {
		return nil, err
	}

	if temporalWorkflowID != nil {
		review.TemporalWorkflowID = *temporalWorkflowID
	}
	if temporalRunID != nil {
		review.TemporalRunID = *temporalRunID
	}

	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &review.ConfigSnapshot); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config snapshot: %w", err)
		}
	}

	if len(sourceFiltersJSON) > 0 {
		if err := json.Unmarshal(sourceFiltersJSON, &review.SourceFilters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal source filters: %w", err)
		}
	}

	return &review, nil
}

// nullString returns a pointer to the string if non-empty, otherwise nil.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
