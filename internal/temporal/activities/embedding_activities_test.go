package activities

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

// mockEmbedder implements Embedder for testing.
type mockEmbedder struct {
	embeddings [][]float32
	err        error
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Return embeddings matching the number of texts.
	result := make([][]float32, len(texts))
	for i := range texts {
		if i < len(m.embeddings) {
			result[i] = m.embeddings[i]
		} else {
			result[i] = []float32{0.1, 0.2, 0.3}
		}
	}
	return result, nil
}

func TestEmbedPapers(t *testing.T) {
	t.Run("embeds papers successfully", func(t *testing.T) {
		embedder := &mockEmbedder{
			embeddings: [][]float32{{0.1, 0.2}, {0.3, 0.4}},
		}
		activities := NewEmbeddingActivities(embedder)

		input := EmbedPapersInput{
			Papers: []PaperForEmbedding{
				{PaperID: uuid.New(), CanonicalID: "doi:123", Abstract: "Abstract 1"},
				{PaperID: uuid.New(), CanonicalID: "doi:456", Abstract: "Abstract 2"},
			},
		}

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(activities.EmbedPapers)

		val, err := testEnv.ExecuteActivity(activities.EmbedPapers, input)
		require.NoError(t, err)

		var output EmbedPapersOutput
		require.NoError(t, val.Get(&output))

		assert.Len(t, output.Embeddings, 2)
		assert.Equal(t, 0, output.Skipped)
		assert.Contains(t, output.Embeddings, "doi:123")
		assert.Contains(t, output.Embeddings, "doi:456")
	})

	t.Run("skips papers without abstract", func(t *testing.T) {
		embedder := &mockEmbedder{
			embeddings: [][]float32{{0.1, 0.2}},
		}
		activities := NewEmbeddingActivities(embedder)

		input := EmbedPapersInput{
			Papers: []PaperForEmbedding{
				{PaperID: uuid.New(), CanonicalID: "doi:123", Abstract: "Has abstract"},
				{PaperID: uuid.New(), CanonicalID: "doi:456", Abstract: ""},
			},
		}

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(activities.EmbedPapers)

		val, err := testEnv.ExecuteActivity(activities.EmbedPapers, input)
		require.NoError(t, err)

		var output EmbedPapersOutput
		require.NoError(t, val.Get(&output))

		assert.Len(t, output.Embeddings, 1)
		assert.Equal(t, 1, output.Skipped)
	})

	t.Run("returns error when embedder fails", func(t *testing.T) {
		embedder := &mockEmbedder{err: fmt.Errorf("embedding API error")}
		activities := NewEmbeddingActivities(embedder)

		input := EmbedPapersInput{
			Papers: []PaperForEmbedding{
				{PaperID: uuid.New(), CanonicalID: "doi:123", Abstract: "Abstract"},
			},
		}

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(activities.EmbedPapers)

		_, err := testEnv.ExecuteActivity(activities.EmbedPapers, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "embed batch")
	})

	t.Run("returns error when embedder is nil", func(t *testing.T) {
		activities := NewEmbeddingActivities(nil)

		input := EmbedPapersInput{
			Papers: []PaperForEmbedding{
				{PaperID: uuid.New(), CanonicalID: "doi:123", Abstract: "Abstract"},
			},
		}

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(activities.EmbedPapers)

		_, err := testEnv.ExecuteActivity(activities.EmbedPapers, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "embedder is not configured")
	})

	t.Run("handles empty input", func(t *testing.T) {
		embedder := &mockEmbedder{}
		activities := NewEmbeddingActivities(embedder)

		input := EmbedPapersInput{
			Papers: []PaperForEmbedding{},
		}

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(activities.EmbedPapers)

		val, err := testEnv.ExecuteActivity(activities.EmbedPapers, input)
		require.NoError(t, err)

		var output EmbedPapersOutput
		require.NoError(t, val.Get(&output))

		assert.Len(t, output.Embeddings, 0)
		assert.Equal(t, 0, output.Skipped)
	})

	t.Run("handles all papers without abstracts", func(t *testing.T) {
		embedder := &mockEmbedder{}
		activities := NewEmbeddingActivities(embedder)

		input := EmbedPapersInput{
			Papers: []PaperForEmbedding{
				{PaperID: uuid.New(), CanonicalID: "doi:123", Abstract: ""},
				{PaperID: uuid.New(), CanonicalID: "doi:456", Abstract: ""},
			},
		}

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(activities.EmbedPapers)

		val, err := testEnv.ExecuteActivity(activities.EmbedPapers, input)
		require.NoError(t, err)

		var output EmbedPapersOutput
		require.NoError(t, val.Get(&output))

		assert.Len(t, output.Embeddings, 0)
		assert.Equal(t, 2, output.Skipped)
	})
}
