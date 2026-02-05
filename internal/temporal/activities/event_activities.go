package activities

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"

	"github.com/helixir/literature-review-service/internal/outbox"
)

// EventPublisher is the interface used by EventActivities to publish events.
// This decouples the activity from the concrete outbox.Publisher implementation,
// enabling straightforward testing with mock implementations.
type EventPublisher interface {
	PublishNonTx(ctx context.Context, params outbox.EmitParams) error
}

// EventActivities provides Temporal activities for publishing domain events
// via the transactional outbox pattern. Events are inserted into the outbox
// table and asynchronously forwarded to Kafka by the outbox relay.
//
// Methods on this struct are registered as Temporal activities via the worker.
type EventActivities struct {
	publisher EventPublisher
}

// NewEventActivities creates a new EventActivities with the given publisher.
func NewEventActivities(publisher EventPublisher) *EventActivities {
	return &EventActivities{publisher: publisher}
}

// PublishEventInput is the serializable input for the PublishEvent activity.
type PublishEventInput struct {
	// EventType is the domain event type (e.g., "review.started", "review.completed").
	EventType string

	// RequestID is the literature review request identifier (used as aggregate ID).
	RequestID uuid.UUID

	// OrgID is the organization identifier for tenant scoping.
	OrgID string

	// ProjectID is the project identifier for tenant scoping.
	ProjectID string

	// Payload is the event payload that will be JSON-serialized.
	Payload map[string]interface{}
}

// PublishEvent publishes a domain event through the outbox.
//
// The event is inserted into the outbox table for asynchronous delivery to Kafka.
// This activity is designed to be called with fire-and-forget semantics from the
// workflow — event publishing failure should never fail the workflow.
func (a *EventActivities) PublishEvent(ctx context.Context, input PublishEventInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("publishing event",
		"eventType", input.EventType,
		"requestID", input.RequestID,
		"orgID", input.OrgID,
		"projectID", input.ProjectID,
	)

	err := a.publisher.PublishNonTx(ctx, outbox.EmitParams{
		RequestID: input.RequestID.String(),
		OrgID:     input.OrgID,
		ProjectID: input.ProjectID,
		EventType: input.EventType,
		Payload:   input.Payload,
	})
	if err != nil {
		logger.Error("failed to publish event",
			"eventType", input.EventType,
			"requestID", input.RequestID,
			"error", err,
		)
		return fmt.Errorf("publish event %s: %w", input.EventType, err)
	}

	logger.Info("event published",
		"eventType", input.EventType,
		"requestID", input.RequestID,
	)

	return nil
}
