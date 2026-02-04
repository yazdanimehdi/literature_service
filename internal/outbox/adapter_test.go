package outbox

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	sharedoutbox "github.com/helixir/outbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockInserter is a mock implementation of the Inserter interface.
type MockInserter struct {
	mock.Mock
}

func (m *MockInserter) InsertEvent(ctx context.Context, tx sharedoutbox.Querier, event sharedoutbox.Event) error {
	args := m.Called(ctx, tx, event)
	return args.Error(0)
}

// MockQuerier is a mock implementation of sharedoutbox.Querier for testing.
type MockQuerier struct {
	mock.Mock
}

func (m *MockQuerier) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	args := m.Called(ctx, sql, arguments)
	return args.Get(0).(pgconn.CommandTag), args.Error(1)
}

func (m *MockQuerier) Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error) {
	args := m.Called(ctx, sql, arguments)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(pgx.Rows), args.Error(1)
}

func (m *MockQuerier) QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row {
	args := m.Called(ctx, sql, arguments)
	return args.Get(0).(pgx.Row)
}

func TestNewAdapter(t *testing.T) {
	mockRepo := new(MockInserter)
	adapter := NewAdapter(mockRepo)
	assert.NotNil(t, adapter)
	assert.Equal(t, mockRepo, adapter.repo)
}

func TestAdapter_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully inserts event with transaction", func(t *testing.T) {
		mockRepo := new(MockInserter)
		mockTx := new(MockQuerier)
		adapter := NewAdapter(mockRepo)

		event := sharedoutbox.Event{
			EventID:   "evt-123",
			EventType: "review.started",
		}

		mockRepo.On("InsertEvent", ctx, mockTx, event).Return(nil)

		err := adapter.Create(ctx, mockTx, event)
		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("successfully inserts event without transaction", func(t *testing.T) {
		mockRepo := new(MockInserter)
		adapter := NewAdapter(mockRepo)

		event := sharedoutbox.Event{
			EventID:   "evt-123",
			EventType: "review.started",
		}

		mockRepo.On("InsertEvent", ctx, nil, event).Return(nil)

		err := adapter.Create(ctx, nil, event)
		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("returns wrapped error on insert failure", func(t *testing.T) {
		mockRepo := new(MockInserter)
		adapter := NewAdapter(mockRepo)

		event := sharedoutbox.Event{
			EventID:   "evt-123",
			EventType: "review.started",
		}

		insertErr := errors.New("database error")
		mockRepo.On("InsertEvent", ctx, nil, event).Return(insertErr)

		err := adapter.Create(ctx, nil, event)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "outbox adapter: insert event")
		assert.ErrorIs(t, err, insertErr)
		mockRepo.AssertExpectations(t)
	})
}

func TestAdapter_CreateNonTx(t *testing.T) {
	ctx := context.Background()

	t.Run("calls Create with nil transaction", func(t *testing.T) {
		mockRepo := new(MockInserter)
		adapter := NewAdapter(mockRepo)

		event := sharedoutbox.Event{
			EventID:   "evt-123",
			EventType: "review.started",
		}

		mockRepo.On("InsertEvent", ctx, nil, event).Return(nil)

		err := adapter.CreateNonTx(ctx, event)
		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})
}

func TestNewPublisher(t *testing.T) {
	emitter := NewEmitter(EmitterConfig{ServiceName: "test-service"})
	mockRepo := new(MockInserter)
	adapter := NewAdapter(mockRepo)

	publisher := NewPublisher(emitter, adapter)
	assert.NotNil(t, publisher)
	assert.Equal(t, emitter, publisher.emitter)
	assert.Equal(t, adapter, publisher.adapter)
}

func TestPublisher_Publish(t *testing.T) {
	ctx := context.Background()
	emitter := NewEmitter(EmitterConfig{ServiceName: "test-service"})
	mockRepo := new(MockInserter)
	adapter := NewAdapter(mockRepo)
	publisher := NewPublisher(emitter, adapter)

	t.Run("successfully publishes event with transaction", func(t *testing.T) {
		mockTx := new(MockQuerier)
		params := EmitParams{
			RequestID: "req-123",
			OrgID:     "org-456",
			ProjectID: "proj-789",
			EventType: "review.started",
			Payload:   map[string]string{"key": "value"},
		}

		// Use mock.MatchedBy to match any event since EventID is generated
		mockRepo.On("InsertEvent", ctx, mockTx, mock.MatchedBy(func(e sharedoutbox.Event) bool {
			return e.AggregateID == "req-123" && e.EventType == "review.started"
		})).Return(nil)

		err := publisher.Publish(ctx, mockTx, params)
		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("successfully publishes event without transaction", func(t *testing.T) {
		params := EmitParams{
			RequestID: "req-123",
			EventType: "review.started",
			Payload:   map[string]string{},
		}

		mockRepo.On("InsertEvent", ctx, nil, mock.MatchedBy(func(e sharedoutbox.Event) bool {
			return e.AggregateID == "req-123" && e.EventType == "review.started"
		})).Return(nil)

		err := publisher.Publish(ctx, nil, params)
		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("returns error when emit fails", func(t *testing.T) {
		params := EmitParams{
			// Missing RequestID - will cause emit to fail
			EventType: "review.started",
			Payload:   map[string]string{},
		}

		err := publisher.Publish(ctx, nil, params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "emit event")
	})

	t.Run("returns error when adapter fails", func(t *testing.T) {
		mockRepo2 := new(MockInserter)
		adapter2 := NewAdapter(mockRepo2)
		publisher2 := NewPublisher(emitter, adapter2)

		params := EmitParams{
			RequestID: "req-123",
			EventType: "review.started",
			Payload:   map[string]string{},
		}

		insertErr := errors.New("database error")
		mockRepo2.On("InsertEvent", ctx, nil, mock.Anything).Return(insertErr)

		err := publisher2.Publish(ctx, nil, params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "store event")
		mockRepo2.AssertExpectations(t)
	})
}

func TestPublisher_PublishNonTx(t *testing.T) {
	ctx := context.Background()
	emitter := NewEmitter(EmitterConfig{ServiceName: "test-service"})
	mockRepo := new(MockInserter)
	adapter := NewAdapter(mockRepo)
	publisher := NewPublisher(emitter, adapter)

	t.Run("calls Publish with nil transaction", func(t *testing.T) {
		params := EmitParams{
			RequestID: "req-123",
			EventType: "review.started",
			Payload:   map[string]string{},
		}

		mockRepo.On("InsertEvent", ctx, nil, mock.MatchedBy(func(e sharedoutbox.Event) bool {
			return e.AggregateID == "req-123"
		})).Return(nil)

		err := publisher.PublishNonTx(ctx, params)
		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})
}

func TestPublisher_Accessors(t *testing.T) {
	emitter := NewEmitter(EmitterConfig{ServiceName: "test-service"})
	mockRepo := new(MockInserter)
	adapter := NewAdapter(mockRepo)
	publisher := NewPublisher(emitter, adapter)

	t.Run("Emitter returns the underlying emitter", func(t *testing.T) {
		assert.Equal(t, emitter, publisher.Emitter())
	})

	t.Run("Adapter returns the underlying adapter", func(t *testing.T) {
		assert.Equal(t, adapter, publisher.Adapter())
	})
}
