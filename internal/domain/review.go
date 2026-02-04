package domain

import (
	"time"

	"github.com/google/uuid"
)

// LiteratureReviewRequest represents a user's request for a literature review.
type LiteratureReviewRequest struct {
	ID uuid.UUID

	// Tenant context (multi-tenancy)
	OrgID     string
	ProjectID string
	UserID    string

	// Request details
	OriginalQuery string

	// Temporal workflow tracking
	TemporalWorkflowID string
	TemporalRunID      string

	// Status and progress
	Status              ReviewStatus
	KeywordsFoundCount  int
	PapersFoundCount    int
	PapersIngestedCount int
	PapersFailedCount   int

	// Configuration
	ExpansionDepth int
	ConfigSnapshot map[string]interface{}
	SourceFilters  map[string]interface{}
	DateFrom       *time.Time
	DateTo         *time.Time

	// Timestamps
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// Duration returns the duration of the review request.
// Returns zero if the request has not started.
// Returns elapsed time from start if still running.
// Returns total duration if completed.
func (r *LiteratureReviewRequest) Duration() time.Duration {
	if r.StartedAt == nil {
		return 0
	}

	if r.CompletedAt != nil {
		return r.CompletedAt.Sub(*r.StartedAt)
	}

	return time.Since(*r.StartedAt)
}

// IsActive returns true if the review request is still in progress.
func (r *LiteratureReviewRequest) IsActive() bool {
	return !r.Status.IsTerminal()
}

// GetTenant returns the tenant context for this request.
func (r *LiteratureReviewRequest) GetTenant() Tenant {
	return Tenant{
		OrgID:     r.OrgID,
		ProjectID: r.ProjectID,
		UserID:    r.UserID,
	}
}

// RequestKeywordMapping links a review request to a discovered keyword.
type RequestKeywordMapping struct {
	ID              uuid.UUID
	RequestID       uuid.UUID
	KeywordID       uuid.UUID
	ExtractionRound int
	SourcePaperID   *uuid.UUID
	SourceType      string // "query", "paper_keywords", "llm_extraction"
	CreatedAt       time.Time
}

// RequestPaperMapping links a review request to a discovered paper.
type RequestPaperMapping struct {
	ID                     uuid.UUID
	RequestID              uuid.UUID
	PaperID                uuid.UUID
	DiscoveredViaKeywordID *uuid.UUID
	DiscoveredViaSource    SourceType
	ExpansionDepth         int

	// Ingestion tracking
	IngestionStatus IngestionStatus
	IngestionJobID  string
	IngestionError  string

	// Timestamps
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ReviewProgressEvent represents a real-time progress event for UI updates.
type ReviewProgressEvent struct {
	ID        uuid.UUID
	RequestID uuid.UUID
	EventType string
	EventData map[string]interface{}
	CreatedAt time.Time
}

// ReviewProgress represents the current progress of a literature review.
type ReviewProgress struct {
	RequestID        uuid.UUID
	Status           ReviewStatus
	CurrentPhase     string
	KeywordsFound    int
	KeywordsSearched int
	PapersFound      int
	PapersIngested   int
	PapersFailed     int
	SourceProgress   map[SourceType]*SourceProgress
	StartedAt        time.Time
	EstimatedEndAt   *time.Time
	LastUpdatedAt    time.Time
}

// SourceProgress represents the progress of searches for a specific source.
type SourceProgress struct {
	Source       SourceType
	Searched     int
	PapersFound  int
	Status       SearchStatus
	ErrorMessage string
}
