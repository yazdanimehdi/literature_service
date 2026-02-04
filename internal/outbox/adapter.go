package outbox

import (
	"context"
	"fmt"

	sharedoutbox "github.com/helixir/outbox"
)

// Inserter is the subset of sharedoutbox.Repository needed by the adapter.
// This interface allows for easy mocking in tests.
type Inserter interface {
	InsertEvent(ctx context.Context, tx sharedoutbox.Querier, event sharedoutbox.Event) error
}

// Adapter bridges the shared outbox repository to the service interfaces.
// It provides a simplified interface for inserting outbox events.
type Adapter struct {
	repo Inserter
}

// NewAdapter creates a new Adapter wrapping the given Inserter.
func NewAdapter(repo Inserter) *Adapter {
	return &Adapter{repo: repo}
}

// Create inserts an event into the outbox.
// If tx is nil, the shared repository handles non-transactional inserts.
// For transactional inserts within a database transaction, pass the transaction as tx.
func (a *Adapter) Create(ctx context.Context, tx sharedoutbox.Querier, event sharedoutbox.Event) error {
	if err := a.repo.InsertEvent(ctx, tx, event); err != nil {
		return fmt.Errorf("outbox adapter: insert event: %w", err)
	}
	return nil
}

// CreateNonTx inserts an event into the outbox without a transaction.
// This is a convenience method when not operating within a database transaction.
func (a *Adapter) CreateNonTx(ctx context.Context, event sharedoutbox.Event) error {
	return a.Create(ctx, nil, event)
}

// Publisher combines the Emitter and Adapter for a complete event publishing workflow.
// It provides a high-level interface for emitting and storing events.
type Publisher struct {
	emitter *Emitter
	adapter *Adapter
}

// NewPublisher creates a new Publisher with the given emitter and adapter.
func NewPublisher(emitter *Emitter, adapter *Adapter) *Publisher {
	return &Publisher{
		emitter: emitter,
		adapter: adapter,
	}
}

// Publish emits an event and inserts it into the outbox within a transaction.
// This is the primary method for publishing events with transactional guarantees.
func (p *Publisher) Publish(ctx context.Context, tx sharedoutbox.Querier, params EmitParams) error {
	event, err := p.emitter.Emit(params)
	if err != nil {
		return fmt.Errorf("emit event: %w", err)
	}

	if err := p.adapter.Create(ctx, tx, event); err != nil {
		return fmt.Errorf("store event: %w", err)
	}

	return nil
}

// PublishNonTx emits an event and inserts it into the outbox without a transaction.
// Use this when not operating within a database transaction context.
func (p *Publisher) PublishNonTx(ctx context.Context, params EmitParams) error {
	return p.Publish(ctx, nil, params)
}

// Emitter returns the underlying emitter for direct event creation.
func (p *Publisher) Emitter() *Emitter {
	return p.emitter
}

// Adapter returns the underlying adapter for direct event insertion.
func (p *Publisher) Adapter() *Adapter {
	return p.adapter
}
