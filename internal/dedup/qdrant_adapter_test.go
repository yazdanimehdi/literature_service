package dedup

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/qdrant"
)

// mockQdrantStore implements qdrant.VectorStore for testing.
type mockQdrantStore struct {
	searchResults []qdrant.SearchResult
	searchErr     error
	upsertedPoint *qdrant.PaperPoint
	upsertErr     error
}

func (m *mockQdrantStore) EnsureCollection(_ context.Context) error { return nil }
func (m *mockQdrantStore) Close() error                             { return nil }

func (m *mockQdrantStore) Search(_ context.Context, _ []float32, _ uint64) ([]qdrant.SearchResult, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.searchResults, nil
}

func (m *mockQdrantStore) Upsert(_ context.Context, point qdrant.PaperPoint) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.upsertedPoint = &point
	return nil
}

// mockPaperLookup implements PaperLookup for testing.
type mockPaperLookup struct {
	papers    []*domain.Paper
	lookupErr error
	calledIDs []uuid.UUID
}

func (m *mockPaperLookup) GetByIDs(_ context.Context, ids []uuid.UUID) ([]*domain.Paper, error) {
	m.calledIDs = ids
	if m.lookupErr != nil {
		return nil, m.lookupErr
	}
	return m.papers, nil
}

func TestQdrantAdapter_Search(t *testing.T) {
	t.Parallel()

	t.Run("returns enriched results with authors", func(t *testing.T) {
		t.Parallel()

		id1 := uuid.New()
		id2 := uuid.New()

		store := &mockQdrantStore{
			searchResults: []qdrant.SearchResult{
				{PaperID: id1, Score: 0.98},
				{PaperID: id2, Score: 0.91},
			},
		}
		lookup := &mockPaperLookup{
			papers: []*domain.Paper{
				{
					ID: id1,
					Authors: []domain.Author{
						{Name: "Alice Smith"},
						{Name: "Bob Jones"},
					},
				},
				{
					ID: id2,
					Authors: []domain.Author{
						{Name: "Carol White"},
					},
				},
			},
		}

		adapter := NewQdrantAdapter(store, lookup)
		results, err := adapter.Search(context.Background(), []float32{0.1, 0.2}, 10)

		require.NoError(t, err)
		require.Len(t, results, 2)

		// Verify first result.
		assert.Equal(t, id1, results[0].PaperID)
		assert.Equal(t, float32(0.98), results[0].Score)
		require.Len(t, results[0].Authors, 2)
		assert.Equal(t, "Alice Smith", results[0].Authors[0].Name)
		assert.Equal(t, "Bob Jones", results[0].Authors[1].Name)

		// Verify second result.
		assert.Equal(t, id2, results[1].PaperID)
		assert.Equal(t, float32(0.91), results[1].Score)
		require.Len(t, results[1].Authors, 1)
		assert.Equal(t, "Carol White", results[1].Authors[0].Name)

		// Verify correct IDs were passed to lookup.
		assert.Equal(t, []uuid.UUID{id1, id2}, lookup.calledIDs)
	})

	t.Run("returns empty slice when no results", func(t *testing.T) {
		t.Parallel()

		store := &mockQdrantStore{
			searchResults: []qdrant.SearchResult{},
		}
		lookup := &mockPaperLookup{}

		adapter := NewQdrantAdapter(store, lookup)
		results, err := adapter.Search(context.Background(), []float32{0.1, 0.2}, 10)

		require.NoError(t, err)
		assert.Empty(t, results)
		// Lookup should not have been called when there are no results.
		assert.Nil(t, lookup.calledIDs)
	})

	t.Run("handles missing paper in lookup gracefully", func(t *testing.T) {
		t.Parallel()

		id1 := uuid.New()

		store := &mockQdrantStore{
			searchResults: []qdrant.SearchResult{
				{PaperID: id1, Score: 0.95},
			},
		}
		// Lookup returns no papers (e.g., paper was deleted between search and lookup).
		lookup := &mockPaperLookup{
			papers: []*domain.Paper{},
		}

		adapter := NewQdrantAdapter(store, lookup)
		results, err := adapter.Search(context.Background(), []float32{0.1, 0.2}, 10)

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, id1, results[0].PaperID)
		assert.Equal(t, float32(0.95), results[0].Score)
		assert.Nil(t, results[0].Authors) // No authors because paper wasn't found.
	})

	t.Run("propagates search error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("qdrant connection failed")
		store := &mockQdrantStore{searchErr: expectedErr}
		lookup := &mockPaperLookup{}

		adapter := NewQdrantAdapter(store, lookup)
		results, err := adapter.Search(context.Background(), []float32{0.1, 0.2}, 10)

		assert.Nil(t, results)
		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("propagates lookup error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("database connection failed")
		store := &mockQdrantStore{
			searchResults: []qdrant.SearchResult{
				{PaperID: uuid.New(), Score: 0.9},
			},
		}
		lookup := &mockPaperLookup{lookupErr: expectedErr}

		adapter := NewQdrantAdapter(store, lookup)
		results, err := adapter.Search(context.Background(), []float32{0.1, 0.2}, 10)

		assert.Nil(t, results)
		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestQdrantAdapter_Upsert(t *testing.T) {
	t.Parallel()

	t.Run("converts PaperEmbedding to PaperPoint and upserts", func(t *testing.T) {
		t.Parallel()

		paperID := uuid.New()
		embedding := []float32{0.5, 0.6, 0.7}

		store := &mockQdrantStore{}
		lookup := &mockPaperLookup{}

		adapter := NewQdrantAdapter(store, lookup)
		err := adapter.Upsert(context.Background(), PaperEmbedding{
			PaperID:   paperID,
			Embedding: embedding,
		})

		require.NoError(t, err)
		require.NotNil(t, store.upsertedPoint)
		assert.Equal(t, paperID, store.upsertedPoint.PaperID)
		assert.Equal(t, embedding, store.upsertedPoint.Embedding)
	})

	t.Run("propagates upsert error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("qdrant write failed")
		store := &mockQdrantStore{upsertErr: expectedErr}
		lookup := &mockPaperLookup{}

		adapter := NewQdrantAdapter(store, lookup)
		err := adapter.Upsert(context.Background(), PaperEmbedding{
			PaperID:   uuid.New(),
			Embedding: []float32{0.1},
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
	})
}
