package dedup

import (
	"context"
	"errors"
	"fmt"
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

func TestChecker_CheckWithEmbedding(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name          string
		paperID       uuid.UUID
		embedding     []float32
		store         *mockVectorStore
		cfg           CheckerConfig
		wantDup       bool
		wantErr       bool
		wantErrSubstr string
		// wantUpserted indicates whether we expect the embedding to have been upserted.
		wantUpserted bool
	}

	paperID := uuid.New()
	otherID := uuid.New()
	selfID := uuid.New()

	defaultCfg := CheckerConfig{
		SimilarityThreshold: 0.95,
		AuthorThreshold:     0.5,
		TopK:                10,
	}

	tests := []testCase{
		{
			name:      "empty embedding returns false",
			paperID:   paperID,
			embedding: nil,
			store:     &mockVectorStore{},
			cfg:       defaultCfg,
			wantDup:   false,
			wantErr:   false,
			// Search and Upsert should not be called; upserted stays empty.
			wantUpserted: false,
		},
		{
			name:      "no duplicates found",
			paperID:   paperID,
			embedding: []float32{0.1, 0.2, 0.3},
			store: &mockVectorStore{
				searchResults: []SimilarPaper{},
			},
			cfg:          defaultCfg,
			wantDup:      false,
			wantErr:      false,
			wantUpserted: true,
		},
		{
			name:      "duplicate above threshold",
			paperID:   paperID,
			embedding: []float32{0.1, 0.2, 0.3},
			store: &mockVectorStore{
				searchResults: []SimilarPaper{
					{PaperID: otherID, Score: 0.98},
				},
			},
			cfg:          defaultCfg,
			wantDup:      true,
			wantErr:      false,
			wantUpserted: true,
		},
		{
			name:      "below threshold is not duplicate",
			paperID:   paperID,
			embedding: []float32{0.1, 0.2, 0.3},
			store: &mockVectorStore{
				searchResults: []SimilarPaper{
					{PaperID: otherID, Score: 0.90},
				},
			},
			cfg:          defaultCfg,
			wantDup:      false,
			wantErr:      false,
			wantUpserted: true,
		},
		{
			name:      "skips self-match",
			paperID:   selfID,
			embedding: []float32{0.1, 0.2, 0.3},
			store: &mockVectorStore{
				searchResults: []SimilarPaper{
					{PaperID: selfID, Score: 1.0},
				},
			},
			cfg:          defaultCfg,
			wantDup:      false,
			wantErr:      false,
			wantUpserted: true,
		},
		{
			name:      "search error",
			paperID:   paperID,
			embedding: []float32{0.1, 0.2, 0.3},
			store: &mockVectorStore{
				searchErr: errors.New("connection refused"),
			},
			cfg:           defaultCfg,
			wantDup:       false,
			wantErr:       true,
			wantErrSubstr: fmt.Sprintf("searching vector store for paper %s", paperID),
			// Upsert happens after Search, so it should NOT be called on search error.
			wantUpserted: false,
		},
		{
			name:      "upsert error",
			paperID:   paperID,
			embedding: []float32{0.1, 0.2, 0.3},
			store: &mockVectorStore{
				searchResults: []SimilarPaper{},
				upsertErr:     errors.New("disk full"),
			},
			cfg:           defaultCfg,
			wantDup:       false,
			wantErr:       true,
			wantErrSubstr: fmt.Sprintf("upserting embedding for paper %s", paperID),
			wantUpserted:  false,
		},
		{
			name:      "multiple candidates picks first above threshold",
			paperID:   paperID,
			embedding: []float32{0.1, 0.2, 0.3},
			store: &mockVectorStore{
				searchResults: []SimilarPaper{
					{PaperID: uuid.New(), Score: 0.80},
					{PaperID: uuid.New(), Score: 0.96},
					{PaperID: uuid.New(), Score: 0.99},
				},
			},
			cfg:          defaultCfg,
			wantDup:      true,
			wantErr:      false,
			wantUpserted: true,
		},
		{
			name:      "exact threshold boundary",
			paperID:   paperID,
			embedding: []float32{0.1, 0.2, 0.3},
			store: &mockVectorStore{
				searchResults: []SimilarPaper{
					{PaperID: otherID, Score: 0.95},
				},
			},
			// Use float64(float32(0.95)) as threshold so the >= comparison
			// is truly at the boundary. A raw float64(0.95) is slightly
			// larger than float64(float32(0.95)) due to precision loss in
			// the float32-to-float64 widening, which would cause a false
			// negative at the boundary.
			cfg: CheckerConfig{
				SimilarityThreshold: float64(float32(0.95)),
				AuthorThreshold:     0.5,
				TopK:                10,
			},
			wantDup:      true,
			wantErr:      false,
			wantUpserted: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			embedder := &mockEmbedder{}
			checker := NewChecker(tc.store, embedder, tc.cfg)

			gotDup, err := checker.CheckWithEmbedding(context.Background(), tc.paperID, tc.embedding)

			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrSubstr)
				assert.False(t, gotDup, "should return false on error")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantDup, gotDup)
			}

			if tc.wantUpserted {
				require.Len(t, tc.store.upserted, 1)
				assert.Equal(t, tc.paperID, tc.store.upserted[0].PaperID)
				assert.Equal(t, tc.embedding, tc.store.upserted[0].Embedding)
			} else {
				assert.Empty(t, tc.store.upserted)
			}

			// Embedder should never be called by CheckWithEmbedding (embedding is pre-computed).
			assert.Equal(t, 0, embedder.callCount, "embedder should not be called by CheckWithEmbedding")
		})
	}
}
