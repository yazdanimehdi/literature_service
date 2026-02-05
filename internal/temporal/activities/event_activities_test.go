package activities

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/outbox"
)

// ---------------------------------------------------------------------------
// Mock: EventPublisher
// ---------------------------------------------------------------------------

// mockEventPublisher is a manual test double for the EventPublisher interface.
type mockEventPublisher struct {
	published []outbox.EmitParams
	err       error
}

func (m *mockEventPublisher) PublishNonTx(_ context.Context, params outbox.EmitParams) error {
	if m.err != nil {
		return m.err
	}
	m.published = append(m.published, params)
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestEventActivities_PublishEvent(t *testing.T) {
	t.Run("publishes event via outbox", func(t *testing.T) {
		suite := &testsuite.WorkflowTestSuite{}
		env := suite.NewTestActivityEnvironment()

		pub := &mockEventPublisher{}
		act := NewEventActivities(pub)
		env.RegisterActivity(act.PublishEvent)

		requestID := uuid.New()
		input := PublishEventInput{
			EventType: "review.started",
			RequestID: requestID,
			OrgID:     "org-1",
			ProjectID: "proj-1",
			Payload: map[string]interface{}{
				"query": "CRISPR gene editing",
			},
		}

		_, err := env.ExecuteActivity(act.PublishEvent, input)
		require.NoError(t, err)

		require.Len(t, pub.published, 1)
		assert.Equal(t, "review.started", pub.published[0].EventType)
		assert.Equal(t, requestID.String(), pub.published[0].RequestID)
		assert.Equal(t, "org-1", pub.published[0].OrgID)
		assert.Equal(t, "proj-1", pub.published[0].ProjectID)
	})

	t.Run("returns error from publisher", func(t *testing.T) {
		suite := &testsuite.WorkflowTestSuite{}
		env := suite.NewTestActivityEnvironment()

		pub := &mockEventPublisher{err: fmt.Errorf("outbox connection refused")}
		act := NewEventActivities(pub)
		env.RegisterActivity(act.PublishEvent)

		input := PublishEventInput{
			EventType: "review.started",
			RequestID: uuid.New(),
			OrgID:     "org-1",
			ProjectID: "proj-1",
		}

		_, err := env.ExecuteActivity(act.PublishEvent, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "publish event review.started")
	})

	t.Run("maps all fields to EmitParams", func(t *testing.T) {
		suite := &testsuite.WorkflowTestSuite{}
		env := suite.NewTestActivityEnvironment()

		pub := &mockEventPublisher{}
		act := NewEventActivities(pub)
		env.RegisterActivity(act.PublishEvent)

		requestID := uuid.New()
		input := PublishEventInput{
			EventType: "review.completed",
			RequestID: requestID,
			OrgID:     "org-42",
			ProjectID: "proj-99",
			Payload: map[string]interface{}{
				"papers_found":    float64(150),
				"keywords_found":  float64(12),
				"papers_ingested": float64(75),
			},
		}

		_, err := env.ExecuteActivity(act.PublishEvent, input)
		require.NoError(t, err)

		require.Len(t, pub.published, 1)
		emitted := pub.published[0]
		assert.Equal(t, "review.completed", emitted.EventType)
		assert.Equal(t, requestID.String(), emitted.RequestID)
		assert.Equal(t, "org-42", emitted.OrgID)
		assert.Equal(t, "proj-99", emitted.ProjectID)

		// Verify payload was passed through.
		payload, ok := emitted.Payload.(map[string]interface{})
		require.True(t, ok, "payload should be a map[string]interface{}")
		assert.Equal(t, float64(150), payload["papers_found"])
		assert.Equal(t, float64(12), payload["keywords_found"])
	})

	t.Run("publishes failure event", func(t *testing.T) {
		suite := &testsuite.WorkflowTestSuite{}
		env := suite.NewTestActivityEnvironment()

		pub := &mockEventPublisher{}
		act := NewEventActivities(pub)
		env.RegisterActivity(act.PublishEvent)

		input := PublishEventInput{
			EventType: "review.failed",
			RequestID: uuid.New(),
			OrgID:     "org-1",
			ProjectID: "proj-1",
			Payload: map[string]interface{}{
				"error": "LLM timeout exceeded",
			},
		}

		_, err := env.ExecuteActivity(act.PublishEvent, input)
		require.NoError(t, err)

		require.Len(t, pub.published, 1)
		assert.Equal(t, "review.failed", pub.published[0].EventType)
	})
}
