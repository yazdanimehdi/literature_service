package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Event type constants for outbox events.
const (
	EventTypeReviewStarted      = "review.started"
	EventTypeReviewCompleted    = "review.completed"
	EventTypeReviewFailed       = "review.failed"
	EventTypeReviewCancelled    = "review.cancelled"
	EventTypeKeywordsExtracted  = "review.keywords_extracted"
	EventTypePapersDiscovered   = "review.papers_discovered"
	EventTypeSearchCompleted    = "review.search_completed"
	EventTypeIngestionStarted   = "review.ingestion_started"
	EventTypeIngestionCompleted = "review.ingestion_completed"
	EventTypeProgressUpdated    = "review.progress_updated"
)

// OutboxEvent represents an event to be published via the outbox pattern.
type OutboxEvent struct {
	EventID       string
	EventVersion  int
	AggregateID   string
	AggregateType string
	EventType     string
	Payload       []byte
	Scope         string
	OrgID         string
	ProjectID     string
	Metadata      map[string]interface{}
	CreatedAt     time.Time
}

// NewOutboxEvent creates a new outbox event with the given parameters.
// The payload is JSON-serialized automatically.
func NewOutboxEvent(eventType, aggregateID, aggregateType string, payload interface{}) (*OutboxEvent, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &OutboxEvent{
		EventID:       uuid.New().String(),
		EventVersion:  1,
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		EventType:     eventType,
		Payload:       payloadBytes,
		Scope:         "project",
		CreatedAt:     time.Now(),
	}, nil
}

// WithTenant sets the tenant information on the event.
func (e *OutboxEvent) WithTenant(orgID, projectID string) *OutboxEvent {
	e.OrgID = orgID
	e.ProjectID = projectID
	return e
}

// WithMetadata sets the metadata on the event.
func (e *OutboxEvent) WithMetadata(metadata map[string]interface{}) *OutboxEvent {
	e.Metadata = metadata
	return e
}

// ReviewStartedPayload is the payload for review.started events.
type ReviewStartedPayload struct {
	RequestID      uuid.UUID `json:"request_id"`
	OrgID          string    `json:"org_id"`
	ProjectID      string    `json:"project_id"`
	UserID         string    `json:"user_id"`
	Query          string    `json:"query"`
	ExpansionDepth int       `json:"expansion_depth"`
}

// ReviewCompletedPayload is the payload for review.completed events.
type ReviewCompletedPayload struct {
	RequestID      uuid.UUID     `json:"request_id"`
	OrgID          string        `json:"org_id"`
	ProjectID      string        `json:"project_id"`
	KeywordsFound  int           `json:"keywords_found"`
	PapersFound    int           `json:"papers_found"`
	PapersIngested int           `json:"papers_ingested"`
	PapersFailed   int           `json:"papers_failed"`
	Duration       time.Duration `json:"duration_ns"`
}

// ReviewFailedPayload is the payload for review.failed events.
type ReviewFailedPayload struct {
	RequestID uuid.UUID `json:"request_id"`
	OrgID     string    `json:"org_id"`
	ProjectID string    `json:"project_id"`
	Error     string    `json:"error"`
	Phase     string    `json:"phase"`
}

// ReviewCancelledPayload is the payload for review.cancelled events.
type ReviewCancelledPayload struct {
	RequestID   uuid.UUID `json:"request_id"`
	OrgID       string    `json:"org_id"`
	ProjectID   string    `json:"project_id"`
	CancelledBy string    `json:"cancelled_by"`
	Reason      string    `json:"reason,omitempty"`
}

// KeywordsExtractedPayload is the payload for review.keywords_extracted events.
type KeywordsExtractedPayload struct {
	RequestID       uuid.UUID  `json:"request_id"`
	OrgID           string     `json:"org_id"`
	ProjectID       string     `json:"project_id"`
	Keywords        []string   `json:"keywords"`
	Count           int        `json:"count"`
	ExtractionRound int        `json:"extraction_round"`
	SourcePaperID   *uuid.UUID `json:"source_paper_id,omitempty"`
}

// PapersDiscoveredPayload is the payload for review.papers_discovered events.
type PapersDiscoveredPayload struct {
	RequestID uuid.UUID   `json:"request_id"`
	OrgID     string      `json:"org_id"`
	ProjectID string      `json:"project_id"`
	KeywordID uuid.UUID   `json:"keyword_id"`
	Source    SourceType  `json:"source"`
	PaperIDs  []uuid.UUID `json:"paper_ids"`
	Count     int         `json:"count"`
}

// SearchCompletedPayload is the payload for review.search_completed events.
type SearchCompletedPayload struct {
	RequestID   uuid.UUID     `json:"request_id"`
	OrgID       string        `json:"org_id"`
	ProjectID   string        `json:"project_id"`
	KeywordID   uuid.UUID     `json:"keyword_id"`
	Source      SourceType    `json:"source"`
	PapersFound int           `json:"papers_found"`
	Duration    time.Duration `json:"duration_ns"`
}

// IngestionStartedPayload is the payload for review.ingestion_started events.
type IngestionStartedPayload struct {
	RequestID  uuid.UUID `json:"request_id"`
	OrgID      string    `json:"org_id"`
	ProjectID  string    `json:"project_id"`
	PaperCount int       `json:"paper_count"`
}

// IngestionCompletedPayload is the payload for review.ingestion_completed events.
type IngestionCompletedPayload struct {
	RequestID      uuid.UUID `json:"request_id"`
	OrgID          string    `json:"org_id"`
	ProjectID      string    `json:"project_id"`
	PapersIngested int       `json:"papers_ingested"`
	PapersFailed   int       `json:"papers_failed"`
	PapersSkipped  int       `json:"papers_skipped"`
}

// ProgressUpdatedPayload is the payload for review.progress_updated events.
type ProgressUpdatedPayload struct {
	RequestID        uuid.UUID    `json:"request_id"`
	OrgID            string       `json:"org_id"`
	ProjectID        string       `json:"project_id"`
	Status           ReviewStatus `json:"status"`
	CurrentPhase     string       `json:"current_phase"`
	KeywordsFound    int          `json:"keywords_found"`
	KeywordsSearched int          `json:"keywords_searched"`
	PapersFound      int          `json:"papers_found"`
	PapersIngested   int          `json:"papers_ingested"`
	PapersFailed     int          `json:"papers_failed"`
}
