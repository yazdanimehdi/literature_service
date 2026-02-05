package dedup

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/domain"
)

// mockVectorStore implements VectorSearcher for testing.
type mockVectorStore struct {
	searchResults []SimilarPaper
	searchErr     error
	upserted      []PaperEmbedding
	upsertErr     error
}

func (m *mockVectorStore) Search(_ context.Context, _ []float32, _ uint64) ([]SimilarPaper, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.searchResults, nil
}

func (m *mockVectorStore) Upsert(_ context.Context, pe PaperEmbedding) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.upserted = append(m.upserted, pe)
	return nil
}

// mockEmbedder implements TextEmbedder for testing.
type mockEmbedder struct {
	vector    []float32
	embedErr  error
	callCount int
}

func (m *mockEmbedder) EmbedText(_ context.Context, _ string) ([]float32, error) {
	m.callCount++
	if m.embedErr != nil {
		return nil, m.embedErr
	}
	return m.vector, nil
}

func TestChecker_NoDuplicate(t *testing.T) {
	t.Parallel()

	paperID := uuid.New()
	paper := &domain.Paper{
		ID:       paperID,
		Title:    "A Novel Method for Gene Editing",
		Abstract: "We present a new approach to CRISPR-based gene editing.",
		Authors: []domain.Author{
			{Name: "John Smith"},
			{Name: "Jane Doe"},
		},
	}

	store := &mockVectorStore{
		searchResults: []SimilarPaper{}, // No similar papers found.
	}
	embedder := &mockEmbedder{
		vector: []float32{0.1, 0.2, 0.3},
	}
	cfg := CheckerConfig{
		SimilarityThreshold: 0.95,
		AuthorThreshold:     0.5,
		TopK:                10,
	}

	checker := NewChecker(store, embedder, cfg)
	result, err := checker.Check(context.Background(), paper)

	require.NoError(t, err)
	assert.False(t, result.IsDuplicate)
	assert.Equal(t, uuid.Nil, result.DuplicateOf)
	assert.Equal(t, float32(0), result.Score)

	// Embedding should have been stored.
	require.Len(t, store.upserted, 1)
	assert.Equal(t, paperID, store.upserted[0].PaperID)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, store.upserted[0].Embedding)
}

func TestChecker_DuplicateFound(t *testing.T) {
	t.Parallel()

	paperID := uuid.New()
	existingID := uuid.New()

	paper := &domain.Paper{
		ID:       paperID,
		Title:    "A Novel Method for Gene Editing",
		Abstract: "We present a new approach to CRISPR-based gene editing.",
		Authors: []domain.Author{
			{Name: "John Smith"},
			{Name: "Jane Doe"},
		},
	}

	store := &mockVectorStore{
		searchResults: []SimilarPaper{
			{
				PaperID: existingID,
				Score:   0.98,
				Authors: []domain.Author{
					{Name: "John Smith"},
					{Name: "Jane Doe"},
				},
			},
		},
	}
	embedder := &mockEmbedder{
		vector: []float32{0.1, 0.2, 0.3},
	}
	cfg := CheckerConfig{
		SimilarityThreshold: 0.95,
		AuthorThreshold:     0.5,
		TopK:                10,
	}

	checker := NewChecker(store, embedder, cfg)
	result, err := checker.Check(context.Background(), paper)

	require.NoError(t, err)
	assert.True(t, result.IsDuplicate)
	assert.Equal(t, existingID, result.DuplicateOf)
	assert.Equal(t, float32(0.98), result.Score)

	// Embedding should still have been stored.
	require.Len(t, store.upserted, 1)
	assert.Equal(t, paperID, store.upserted[0].PaperID)
}

func TestChecker_SimilarButDifferentAuthors(t *testing.T) {
	t.Parallel()

	paperID := uuid.New()
	existingID := uuid.New()

	paper := &domain.Paper{
		ID:       paperID,
		Title:    "A Novel Method for Gene Editing",
		Abstract: "We present a new approach to CRISPR-based gene editing.",
		Authors: []domain.Author{
			{Name: "John Smith"},
			{Name: "Jane Doe"},
		},
	}

	store := &mockVectorStore{
		searchResults: []SimilarPaper{
			{
				PaperID: existingID,
				Score:   0.97,
				Authors: []domain.Author{
					// Completely different authors.
					{Name: "Alice Johnson"},
					{Name: "Bob Williams"},
				},
			},
		},
	}
	embedder := &mockEmbedder{
		vector: []float32{0.1, 0.2, 0.3},
	}
	cfg := CheckerConfig{
		SimilarityThreshold: 0.95,
		AuthorThreshold:     0.5,
		TopK:                10,
	}

	checker := NewChecker(store, embedder, cfg)
	result, err := checker.Check(context.Background(), paper)

	require.NoError(t, err)
	assert.False(t, result.IsDuplicate, "similar embeddings but different authors should NOT be duplicate")
	assert.Equal(t, uuid.Nil, result.DuplicateOf)
	assert.Equal(t, float32(0), result.Score)

	// Embedding should still have been stored.
	require.Len(t, store.upserted, 1)
}

func TestChecker_SkipsPaperWithoutAbstract(t *testing.T) {
	t.Parallel()

	paperID := uuid.New()
	paper := &domain.Paper{
		ID:       paperID,
		Title:    "A Paper Without an Abstract",
		Abstract: "",
		Authors: []domain.Author{
			{Name: "John Smith"},
		},
	}

	store := &mockVectorStore{}
	embedder := &mockEmbedder{
		vector: []float32{0.1, 0.2, 0.3},
	}
	cfg := CheckerConfig{
		SimilarityThreshold: 0.95,
		AuthorThreshold:     0.5,
		TopK:                10,
	}

	checker := NewChecker(store, embedder, cfg)
	result, err := checker.Check(context.Background(), paper)

	require.NoError(t, err)
	assert.False(t, result.IsDuplicate)
	assert.Equal(t, uuid.Nil, result.DuplicateOf)

	// No embedding should have been stored because no abstract.
	assert.Empty(t, store.upserted)
	// Embedder should not have been called.
	assert.Equal(t, 0, embedder.callCount)
}

func TestChecker_SkipsSelf(t *testing.T) {
	t.Parallel()

	paperID := uuid.New()
	paper := &domain.Paper{
		ID:       paperID,
		Title:    "A Novel Method for Gene Editing",
		Abstract: "We present a new approach to CRISPR-based gene editing.",
		Authors: []domain.Author{
			{Name: "John Smith"},
			{Name: "Jane Doe"},
		},
	}

	// Vector store returns the paper itself as a match.
	store := &mockVectorStore{
		searchResults: []SimilarPaper{
			{
				PaperID: paperID, // Same as the input paper.
				Score:   1.0,
				Authors: []domain.Author{
					{Name: "John Smith"},
					{Name: "Jane Doe"},
				},
			},
		},
	}
	embedder := &mockEmbedder{
		vector: []float32{0.1, 0.2, 0.3},
	}
	cfg := CheckerConfig{
		SimilarityThreshold: 0.95,
		AuthorThreshold:     0.5,
		TopK:                10,
	}

	checker := NewChecker(store, embedder, cfg)
	result, err := checker.Check(context.Background(), paper)

	require.NoError(t, err)
	assert.False(t, result.IsDuplicate, "paper should not be detected as duplicate of itself")
	assert.Equal(t, uuid.Nil, result.DuplicateOf)
	assert.Equal(t, float32(0), result.Score)

	// Embedding should still have been stored.
	require.Len(t, store.upserted, 1)
}

func TestChecker_BelowThreshold(t *testing.T) {
	t.Parallel()

	paperID := uuid.New()
	existingID := uuid.New()

	paper := &domain.Paper{
		ID:       paperID,
		Title:    "A Novel Method for Gene Editing",
		Abstract: "We present a new approach to CRISPR-based gene editing.",
		Authors: []domain.Author{
			{Name: "John Smith"},
			{Name: "Jane Doe"},
		},
	}

	// Similarity below threshold (0.90 < 0.95).
	store := &mockVectorStore{
		searchResults: []SimilarPaper{
			{
				PaperID: existingID,
				Score:   0.90,
				Authors: []domain.Author{
					{Name: "John Smith"},
					{Name: "Jane Doe"},
				},
			},
		},
	}
	embedder := &mockEmbedder{
		vector: []float32{0.1, 0.2, 0.3},
	}
	cfg := CheckerConfig{
		SimilarityThreshold: 0.95,
		AuthorThreshold:     0.5,
		TopK:                10,
	}

	checker := NewChecker(store, embedder, cfg)
	result, err := checker.Check(context.Background(), paper)

	require.NoError(t, err)
	assert.False(t, result.IsDuplicate, "similarity below threshold should NOT be duplicate")
	assert.Equal(t, uuid.Nil, result.DuplicateOf)
	assert.Equal(t, float32(0), result.Score)

	// Embedding should still have been stored.
	require.Len(t, store.upserted, 1)
}
