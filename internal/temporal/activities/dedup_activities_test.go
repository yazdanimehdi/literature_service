package activities

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/dedup"
	"github.com/helixir/literature-review-service/internal/domain"
)

// dedupMockVectorStore implements dedup.VectorSearcher for activity tests.
type dedupMockVectorStore struct {
	searchResults []dedup.SimilarPaper
	upserted      []dedup.PaperEmbedding
}

func (m *dedupMockVectorStore) Search(_ context.Context, _ []float32, _ uint64) ([]dedup.SimilarPaper, error) {
	return m.searchResults, nil
}

func (m *dedupMockVectorStore) Upsert(_ context.Context, pe dedup.PaperEmbedding) error {
	m.upserted = append(m.upserted, pe)
	return nil
}

// dedupMockEmbedder implements dedup.TextEmbedder for activity tests.
type dedupMockEmbedder struct {
	vector []float32
}

func (m *dedupMockEmbedder) EmbedText(_ context.Context, _ string) ([]float32, error) {
	return m.vector, nil
}

func TestDedupPapers_MixedPapers(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	existingID := uuid.New()

	store := &dedupMockVectorStore{
		searchResults: []dedup.SimilarPaper{
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
	embedder := &dedupMockEmbedder{
		vector: []float32{0.1, 0.2, 0.3},
	}
	cfg := dedup.CheckerConfig{
		SimilarityThreshold: 0.95,
		AuthorThreshold:     0.5,
		TopK:                10,
	}

	checker := dedup.NewChecker(store, embedder, cfg)
	activities := NewDedupActivities(checker)
	env.RegisterActivity(activities.DedupPapers)

	// Create a mix of papers:
	// - Paper 1: has abstract, will be detected as duplicate (matching authors + high similarity)
	// - Paper 2: has abstract, unique ID so different from existing, but same mock returns same results
	//            (will also be duplicate since mock always returns same high-similarity result)
	// - Paper 3: no abstract, should be skipped
	paper1 := &domain.Paper{
		ID:       uuid.New(),
		Title:    "Duplicate Paper",
		Abstract: "This paper has an abstract and will match the existing paper.",
		Authors: []domain.Author{
			{Name: "John Smith"},
			{Name: "Jane Doe"},
		},
	}
	paper2 := &domain.Paper{
		ID:       uuid.New(),
		Title:    "Another Duplicate Paper",
		Abstract: "This paper also has an abstract and will match.",
		Authors: []domain.Author{
			{Name: "John Smith"},
			{Name: "Jane Doe"},
		},
	}
	paper3 := &domain.Paper{
		ID:       uuid.New(),
		Title:    "Paper Without Abstract",
		Abstract: "",
		Authors: []domain.Author{
			{Name: "Alice Johnson"},
		},
	}

	input := DedupPapersInput{
		Papers: []*domain.Paper{paper1, paper2, paper3},
	}

	result, err := env.ExecuteActivity(activities.DedupPapers, input)
	require.NoError(t, err)

	var output DedupPapersOutput
	require.NoError(t, result.Get(&output))

	// Paper 3 has no abstract -> skipped, included in non-duplicates.
	// Paper 1 and 2 have abstracts, mock returns high-similarity match with matching authors -> duplicates.
	assert.Equal(t, 2, output.DuplicateCount, "papers 1 and 2 should be detected as duplicates")
	assert.Equal(t, 1, output.SkippedCount, "paper 3 should be skipped (no abstract)")
	assert.Len(t, output.NonDuplicateIDs, 1, "only paper 3 should be a non-duplicate")
	assert.Equal(t, paper3.ID, output.NonDuplicateIDs[0])
}

func TestDedupPapers_AllUnique(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	store := &dedupMockVectorStore{
		searchResults: []dedup.SimilarPaper{}, // No similar papers.
	}
	embedder := &dedupMockEmbedder{
		vector: []float32{0.1, 0.2, 0.3},
	}
	cfg := dedup.CheckerConfig{
		SimilarityThreshold: 0.95,
		AuthorThreshold:     0.5,
		TopK:                10,
	}

	checker := dedup.NewChecker(store, embedder, cfg)
	activities := NewDedupActivities(checker)
	env.RegisterActivity(activities.DedupPapers)

	paper1 := &domain.Paper{
		ID:       uuid.New(),
		Title:    "Unique Paper 1",
		Abstract: "A unique abstract for paper 1.",
		Authors:  []domain.Author{{Name: "Author A"}},
	}
	paper2 := &domain.Paper{
		ID:       uuid.New(),
		Title:    "Unique Paper 2",
		Abstract: "A unique abstract for paper 2.",
		Authors:  []domain.Author{{Name: "Author B"}},
	}

	input := DedupPapersInput{
		Papers: []*domain.Paper{paper1, paper2},
	}

	result, err := env.ExecuteActivity(activities.DedupPapers, input)
	require.NoError(t, err)

	var output DedupPapersOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, 0, output.DuplicateCount)
	assert.Equal(t, 0, output.SkippedCount)
	assert.Len(t, output.NonDuplicateIDs, 2)
}

func TestDedupPapers_EmptyInput(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	store := &dedupMockVectorStore{}
	embedder := &dedupMockEmbedder{
		vector: []float32{0.1, 0.2, 0.3},
	}
	cfg := dedup.CheckerConfig{
		SimilarityThreshold: 0.95,
		AuthorThreshold:     0.5,
		TopK:                10,
	}

	checker := dedup.NewChecker(store, embedder, cfg)
	activities := NewDedupActivities(checker)
	env.RegisterActivity(activities.DedupPapers)

	input := DedupPapersInput{
		Papers: []*domain.Paper{},
	}

	result, err := env.ExecuteActivity(activities.DedupPapers, input)
	require.NoError(t, err)

	var output DedupPapersOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, 0, output.DuplicateCount)
	assert.Equal(t, 0, output.SkippedCount)
	assert.Empty(t, output.NonDuplicateIDs)
}
