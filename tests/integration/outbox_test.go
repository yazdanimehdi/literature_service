//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serviceoutbox "github.com/helixir/literature-review-service/internal/outbox"
	sharedoutbox "github.com/helixir/outbox"
	outboxpg "github.com/helixir/outbox/postgres"
)

func TestOutboxWriteAndRead(t *testing.T) {
	cleanTable(t, "outbox_events")
	ctx := context.Background()

	// Create the shared outbox PG repository (no dead letter).
	repo := outboxpg.NewRepository(testPool, nil)

	// Create the service's adapter and emitter.
	adapter := serviceoutbox.NewAdapter(repo)
	emitter := serviceoutbox.NewEmitter(serviceoutbox.EmitterConfig{
		ServiceName: "literature-review-service-test",
	})

	// Emit and insert an event.
	event, err := emitter.Emit(serviceoutbox.EmitParams{
		RequestID: "test-request-001",
		OrgID:     "org-outbox-test",
		ProjectID: "proj-outbox-test",
		EventType: "review.started",
		Payload:   map[string]string{"query": "test query"},
	})
	require.NoError(t, err)

	// Insert using the adapter (pass pool as querier for non-tx insert).
	err = adapter.Create(ctx, testPool, event)
	require.NoError(t, err)

	// Verify the event was written by listing from the repository.
	events, err := repo.List(ctx, sharedoutbox.EventFilter{
		AggregateID: "test-request-001",
		Limit:       10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, events, "expected at least one outbox event")

	found := false
	for _, e := range events {
		if e.AggregateID == "test-request-001" {
			found = true
			assert.Equal(t, "review.started", e.EventType)
			assert.Equal(t, "literature_review", e.AggregateType)
		}
	}
	assert.True(t, found, "inserted event should appear in list results")
}
