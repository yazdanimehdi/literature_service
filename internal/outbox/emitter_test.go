package outbox

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEmitter(t *testing.T) {
	t.Run("uses default service name when empty", func(t *testing.T) {
		emitter := NewEmitter(EmitterConfig{})
		assert.Equal(t, "literature-review-service", emitter.config.ServiceName)
	})

	t.Run("uses provided service name", func(t *testing.T) {
		emitter := NewEmitter(EmitterConfig{ServiceName: "custom-service"})
		assert.Equal(t, "custom-service", emitter.config.ServiceName)
	})
}

func TestEmitter_Emit(t *testing.T) {
	emitter := NewEmitter(EmitterConfig{ServiceName: "test-service"})

	t.Run("creates event with all fields", func(t *testing.T) {
		payload := map[string]string{"key": "value"}
		params := EmitParams{
			RequestID:     "req-123",
			OrgID:         "org-456",
			ProjectID:     "proj-789",
			EventType:     "review.started",
			Payload:       payload,
			CorrelationID: "corr-abc",
			TraceID:       "trace-xyz",
		}

		event, err := emitter.Emit(params)
		require.NoError(t, err)

		assert.NotEmpty(t, event.EventID)
		assert.Equal(t, "req-123", event.AggregateID)
		assert.Equal(t, AggregateTypeLiteratureReview, event.AggregateType)
		assert.Equal(t, "review.started", event.EventType)
		assert.Equal(t, "project", event.Scope)
		assert.NotNil(t, event.OwnerOrgID)
		assert.Equal(t, "org-456", *event.OwnerOrgID)
		assert.NotNil(t, event.OwnerProjectID)
		assert.Equal(t, "proj-789", *event.OwnerProjectID)
		assert.Equal(t, 5, event.MaxAttempts)

		// Verify payload
		var decoded map[string]string
		err = json.Unmarshal(event.Payload, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "value", decoded["key"])

		// Verify metadata contains expected fields
		var metadata map[string]interface{}
		err = json.Unmarshal(event.Metadata, &metadata)
		require.NoError(t, err)
		assert.Equal(t, "test-service", metadata["source"])
		assert.Equal(t, "corr-abc", metadata["correlation_id"])
		assert.Equal(t, "trace-xyz", metadata["trace_id"])
	})

	t.Run("uses default scope when not provided", func(t *testing.T) {
		params := EmitParams{
			RequestID: "req-123",
			EventType: "review.started",
			Payload:   map[string]string{},
		}

		event, err := emitter.Emit(params)
		require.NoError(t, err)
		assert.Equal(t, "project", event.Scope)
	})

	t.Run("uses custom scope when provided", func(t *testing.T) {
		params := EmitParams{
			RequestID: "req-123",
			EventType: "review.started",
			Payload:   map[string]string{},
			Scope:     "organization",
		}

		event, err := emitter.Emit(params)
		require.NoError(t, err)
		assert.Equal(t, "organization", event.Scope)
	})

	t.Run("handles nil org and project IDs", func(t *testing.T) {
		params := EmitParams{
			RequestID: "req-123",
			EventType: "review.started",
			Payload:   map[string]string{},
		}

		event, err := emitter.Emit(params)
		require.NoError(t, err)
		assert.Nil(t, event.OwnerOrgID)
		assert.Nil(t, event.OwnerProjectID)
	})

	t.Run("errors when request_id is empty", func(t *testing.T) {
		params := EmitParams{
			EventType: "review.started",
			Payload:   map[string]string{},
		}

		_, err := emitter.Emit(params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request_id is required")
	})

	t.Run("errors when event_type is empty", func(t *testing.T) {
		params := EmitParams{
			RequestID: "req-123",
			Payload:   map[string]string{},
		}

		_, err := emitter.Emit(params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "event_type is required")
	})

	t.Run("errors when payload cannot be marshaled", func(t *testing.T) {
		params := EmitParams{
			RequestID: "req-123",
			EventType: "review.started",
			Payload:   make(chan int), // channels cannot be JSON marshaled
		}

		_, err := emitter.Emit(params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "marshal payload")
	})

	t.Run("omits correlation_id from metadata when empty", func(t *testing.T) {
		params := EmitParams{
			RequestID: "req-123",
			EventType: "review.started",
			Payload:   map[string]string{},
		}

		event, err := emitter.Emit(params)
		require.NoError(t, err)

		var metadata map[string]interface{}
		err = json.Unmarshal(event.Metadata, &metadata)
		require.NoError(t, err)
		_, hasCorrelationID := metadata["correlation_id"]
		assert.False(t, hasCorrelationID, "correlation_id should not be present when empty")
	})

	t.Run("omits trace_id from metadata when empty", func(t *testing.T) {
		params := EmitParams{
			RequestID: "req-123",
			EventType: "review.started",
			Payload:   map[string]string{},
		}

		event, err := emitter.Emit(params)
		require.NoError(t, err)

		var metadata map[string]interface{}
		err = json.Unmarshal(event.Metadata, &metadata)
		require.NoError(t, err)
		_, hasTraceID := metadata["trace_id"]
		assert.False(t, hasTraceID, "trace_id should not be present when empty")
	})
}

func TestEmitter_ConvenienceMethods(t *testing.T) {
	emitter := NewEmitter(EmitterConfig{ServiceName: "test-service"})
	payload := map[string]string{"test": "data"}

	t.Run("EmitReviewStarted", func(t *testing.T) {
		event, err := emitter.EmitReviewStarted("req-1", "org-1", "proj-1", payload)
		require.NoError(t, err)
		assert.Equal(t, "review.started", event.EventType)
		assert.Equal(t, "req-1", event.AggregateID)
	})

	t.Run("EmitReviewCompleted", func(t *testing.T) {
		event, err := emitter.EmitReviewCompleted("req-2", "org-2", "proj-2", payload)
		require.NoError(t, err)
		assert.Equal(t, "review.completed", event.EventType)
		assert.Equal(t, "req-2", event.AggregateID)
	})

	t.Run("EmitReviewFailed", func(t *testing.T) {
		event, err := emitter.EmitReviewFailed("req-3", "org-3", "proj-3", payload)
		require.NoError(t, err)
		assert.Equal(t, "review.failed", event.EventType)
		assert.Equal(t, "req-3", event.AggregateID)
	})

	t.Run("EmitKeywordsExtracted", func(t *testing.T) {
		event, err := emitter.EmitKeywordsExtracted("req-4", "org-4", "proj-4", payload)
		require.NoError(t, err)
		assert.Equal(t, "review.keywords_extracted", event.EventType)
		assert.Equal(t, "req-4", event.AggregateID)
	})

	t.Run("EmitPapersDiscovered", func(t *testing.T) {
		event, err := emitter.EmitPapersDiscovered("req-5", "org-5", "proj-5", payload)
		require.NoError(t, err)
		assert.Equal(t, "review.papers_discovered", event.EventType)
		assert.Equal(t, "req-5", event.AggregateID)
	})

	t.Run("EmitProgressUpdated", func(t *testing.T) {
		event, err := emitter.EmitProgressUpdated("req-6", "org-6", "proj-6", payload)
		require.NoError(t, err)
		assert.Equal(t, "review.progress_updated", event.EventType)
		assert.Equal(t, "req-6", event.AggregateID)
	})
}
