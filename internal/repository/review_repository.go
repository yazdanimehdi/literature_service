package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/helixir/literature-review-service/internal/domain"
)

// ReviewRepository handles literature review request persistence and lifecycle management.
// It provides methods for creating, retrieving, updating, and listing review requests
// with support for multi-tenancy through organization and project isolation.
type ReviewRepository interface {
	// Create inserts a new literature review request.
	// The review must have a valid ID, OrgID, ProjectID, and UserID.
	// Returns domain.ErrAlreadyExists if a review with the same ID already exists.
	// Returns domain.ErrInvalidInput if required fields are missing.
	Create(ctx context.Context, review *domain.LiteratureReviewRequest) error

	// Get retrieves a literature review request by its ID within a tenant context.
	// The orgID and projectID parameters enforce tenant isolation.
	// Returns domain.ErrNotFound if no matching review exists.
	Get(ctx context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error)

	// Update performs an optimistic update on a literature review request using SELECT FOR UPDATE.
	// The provided function receives the current review state and should return an error
	// if the update should be aborted. Changes made to the review in the function are persisted.
	// Returns domain.ErrNotFound if no matching review exists.
	//
	// Concurrent update behavior:
	//   - If the row lock cannot be acquired before context deadline, returns context.DeadlineExceeded.
	//   - If the provided function returns an error, the transaction is rolled back and that error is returned.
	//   - Callers should use a context with an appropriate timeout to avoid long waits on lock contention.
	Update(ctx context.Context, orgID, projectID string, id uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error

	// UpdateStatus updates the status of a literature review request with optional error message.
	// This is a convenience method for status-only updates without requiring the full Update flow.
	// The errorMsg parameter is stored only when transitioning to a failed or error state.
	// Returns domain.ErrNotFound if no matching review exists.
	UpdateStatus(ctx context.Context, orgID, projectID string, id uuid.UUID, status domain.ReviewStatus, errorMsg string) error

	// List retrieves literature review requests matching the filter criteria.
	// Returns the matching reviews and total count for pagination.
	// The total count reflects all matching records regardless of limit/offset.
	List(ctx context.Context, filter ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error)

	// IncrementCounters atomically increments the papers found and ingested counters.
	// This is used during the review process to track progress without requiring full updates.
	// The orgID and projectID parameters enforce tenant isolation.
	// Negative values are allowed to decrement counters if needed.
	// Returns domain.ErrNotFound if no matching review exists.
	IncrementCounters(ctx context.Context, orgID, projectID string, id uuid.UUID, papersFound, papersIngested int) error

	// GetByWorkflowID retrieves a literature review request by its Temporal workflow ID.
	// This is useful for correlating workflow events back to review requests.
	// Returns domain.ErrNotFound if no matching review exists.
	GetByWorkflowID(ctx context.Context, workflowID string) (*domain.LiteratureReviewRequest, error)
}

// ReviewFilter specifies criteria for listing literature review requests.
type ReviewFilter struct {
	// OrgID filters by organization ID (required for tenant isolation).
	OrgID string

	// ProjectID filters by project ID (optional, narrows within organization).
	ProjectID string

	// Status filters by one or more review statuses (optional).
	// When multiple statuses are provided, reviews matching any status are returned.
	Status []domain.ReviewStatus

	// CreatedAfter filters to reviews created after this timestamp (optional).
	CreatedAfter *time.Time

	// CreatedBefore filters to reviews created before this timestamp (optional).
	CreatedBefore *time.Time

	// Limit specifies maximum number of results (default: 100, max: 1000).
	Limit int

	// Offset specifies the starting position for pagination.
	Offset int
}

// Validate checks if the filter has valid values and sets defaults.
// Returns domain.ErrInvalidInput if OrgID is empty.
func (f *ReviewFilter) Validate() error {
	if f.OrgID == "" {
		return domain.NewValidationError("org_id", "organization ID is required")
	}

	// Apply defaults
	if f.Limit <= 0 {
		f.Limit = 100
	}
	if f.Limit > 1000 {
		f.Limit = 1000
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	return nil
}
