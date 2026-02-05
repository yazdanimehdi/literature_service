package outbox

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	sharedoutbox "github.com/helixir/outbox"
)

const (
	// AggregateTypeLiteratureReview is the aggregate type for literature review events.
	AggregateTypeLiteratureReview = "literature_review"

	// defaultMaxAttempts is the default maximum number of delivery attempts for outbox events.
	defaultMaxAttempts = 5

	// defaultScope is the default scope for tenant-scoped events.
	defaultScope = "project"
)

// EmitterConfig configures the EventEmitter with service context.
type EmitterConfig struct {
	// ServiceName identifies the source service.
	ServiceName string
}

// EmitParams contains the parameters for emitting an event.
type EmitParams struct {
	// RequestID is the literature review request ID (aggregate ID).
	RequestID string
	// OrgID is the organization ID for scoping.
	OrgID string
	// ProjectID is the project ID for scoping.
	ProjectID string
	// EventType is the type of event (e.g., "review.started").
	EventType string
	// Payload is the event payload that will be JSON-serialized.
	Payload interface{}
	// CorrelationID for request tracing (optional).
	CorrelationID string
	// TraceID for distributed tracing (optional).
	TraceID string
	// Scope overrides the default "project" scope (optional).
	Scope string
}

// Emitter creates shared outbox events enriched with literature review context.
type Emitter struct {
	config EmitterConfig
}

// NewEmitter creates a new Emitter with the given service configuration.
func NewEmitter(config EmitterConfig) *Emitter {
	if config.ServiceName == "" {
		config.ServiceName = "literature-review-service"
	}
	return &Emitter{config: config}
}

// Emit creates a shared outbox Event from the given parameters.
// The event is ready to be inserted into the outbox table.
func (e *Emitter) Emit(params EmitParams) (sharedoutbox.Event, error) {
	if params.RequestID == "" {
		return sharedoutbox.Event{}, fmt.Errorf("request_id is required")
	}
	if params.EventType == "" {
		return sharedoutbox.Event{}, fmt.Errorf("event_type is required")
	}

	payloadBytes, err := json.Marshal(params.Payload)
	if err != nil {
		return sharedoutbox.Event{}, fmt.Errorf("marshal payload: %w", err)
	}

	// Build metadata with service context and tracing
	metaBuilder := sharedoutbox.NewMetadataBuilder().
		WithSource(e.config.ServiceName)

	if params.CorrelationID != "" {
		metaBuilder = metaBuilder.WithCorrelationID(params.CorrelationID)
	}
	if params.TraceID != "" {
		metaBuilder = metaBuilder.WithTraceID(params.TraceID)
	}

	metadata := metaBuilder.Build()

	// Default scope to "project" for tenant-scoped events
	scope := params.Scope
	if scope == "" {
		scope = defaultScope
	}

	// Prepare org/project pointers for scope
	var orgPtr, projectPtr *string
	if params.OrgID != "" {
		orgPtr = &params.OrgID
	}
	if params.ProjectID != "" {
		projectPtr = &params.ProjectID
	}

	builder := sharedoutbox.NewEventBuilder().
		WithEventID(uuid.New().String()).
		WithAggregateID(params.RequestID).
		WithAggregateType(AggregateTypeLiteratureReview).
		WithEventType(params.EventType).
		WithPayload(payloadBytes).
		WithScope(scope, orgPtr, projectPtr).
		WithMetadata(metadata).
		WithMaxAttempts(defaultMaxAttempts)

	event := builder.Build()
	return event, nil
}

// EmitReviewStarted is a convenience method for emitting review.started events.
func (e *Emitter) EmitReviewStarted(requestID, orgID, projectID string, payload interface{}) (sharedoutbox.Event, error) {
	return e.Emit(EmitParams{
		RequestID: requestID,
		OrgID:     orgID,
		ProjectID: projectID,
		EventType: "review.started",
		Payload:   payload,
	})
}

// EmitReviewCompleted is a convenience method for emitting review.completed events.
func (e *Emitter) EmitReviewCompleted(requestID, orgID, projectID string, payload interface{}) (sharedoutbox.Event, error) {
	return e.Emit(EmitParams{
		RequestID: requestID,
		OrgID:     orgID,
		ProjectID: projectID,
		EventType: "review.completed",
		Payload:   payload,
	})
}

// EmitReviewFailed is a convenience method for emitting review.failed events.
func (e *Emitter) EmitReviewFailed(requestID, orgID, projectID string, payload interface{}) (sharedoutbox.Event, error) {
	return e.Emit(EmitParams{
		RequestID: requestID,
		OrgID:     orgID,
		ProjectID: projectID,
		EventType: "review.failed",
		Payload:   payload,
	})
}

// EmitKeywordsExtracted is a convenience method for emitting review.keywords_extracted events.
func (e *Emitter) EmitKeywordsExtracted(requestID, orgID, projectID string, payload interface{}) (sharedoutbox.Event, error) {
	return e.Emit(EmitParams{
		RequestID: requestID,
		OrgID:     orgID,
		ProjectID: projectID,
		EventType: "review.keywords_extracted",
		Payload:   payload,
	})
}

// EmitPapersDiscovered is a convenience method for emitting review.papers_discovered events.
func (e *Emitter) EmitPapersDiscovered(requestID, orgID, projectID string, payload interface{}) (sharedoutbox.Event, error) {
	return e.Emit(EmitParams{
		RequestID: requestID,
		OrgID:     orgID,
		ProjectID: projectID,
		EventType: "review.papers_discovered",
		Payload:   payload,
	})
}

// EmitProgressUpdated is a convenience method for emitting review.progress_updated events.
func (e *Emitter) EmitProgressUpdated(requestID, orgID, projectID string, payload interface{}) (sharedoutbox.Event, error) {
	return e.Emit(EmitParams{
		RequestID: requestID,
		OrgID:     orgID,
		ProjectID: projectID,
		EventType: "review.progress_updated",
		Payload:   payload,
	})
}
