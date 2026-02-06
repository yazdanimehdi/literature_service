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

func TestEmbedText(t *testing.T) {
	t.Run("embeds text successfully", func(t *testing.T) {
		embedder := &mockEmbedder{
			embeddings: [][]float32{{0.1, 0.2, 0.3}},
		}
		act := NewEmbeddingActivities(embedder)

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(act.EmbedText)

		val, err := testEnv.ExecuteActivity(act.EmbedText, EmbedTextInput{Text: "CRISPR gene editing"})
		require.NoError(t, err)

		var output EmbedTextOutput
		require.NoError(t, val.Get(&output))
		assert.Equal(t, []float32{0.1, 0.2, 0.3}, output.Embedding)
	})

	t.Run("returns error for empty text", func(t *testing.T) {
		embedder := &mockEmbedder{}
		act := NewEmbeddingActivities(embedder)

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(act.EmbedText)

		_, err := testEnv.ExecuteActivity(act.EmbedText, EmbedTextInput{Text: ""})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "text is required")
	})

	t.Run("returns error when embedder fails", func(t *testing.T) {
		embedder := &mockEmbedder{err: fmt.Errorf("API error")}
		act := NewEmbeddingActivities(embedder)

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(act.EmbedText)

		_, err := testEnv.ExecuteActivity(act.EmbedText, EmbedTextInput{Text: "some text"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "embed text")
	})

	t.Run("returns error when embedder is nil", func(t *testing.T) {
		act := NewEmbeddingActivities(nil)

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(act.EmbedText)

		_, err := testEnv.ExecuteActivity(act.EmbedText, EmbedTextInput{Text: "some text"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "embedder is not configured")
	})
}

func TestScoreKeywordRelevance(t *testing.T) {
	t.Run("filters keywords by threshold", func(t *testing.T) {
		embedder := &mockEmbedder{
			embeddings: [][]float32{
				{0.9, 0.1, 0.0}, // "relevant" — high similarity
				{0.0, 0.0, 1.0}, // "irrelevant" — low similarity
			},
		}
		act := NewEmbeddingActivities(embedder)

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(act.ScoreKeywordRelevance)

		val, err := testEnv.ExecuteActivity(act.ScoreKeywordRelevance, ScoreKeywordRelevanceInput{
			Keywords:       []string{"relevant term", "irrelevant term"},
			QueryEmbedding: []float32{1.0, 0.0, 0.0},
			Threshold:      0.5,
		})
		require.NoError(t, err)

		var output ScoreKeywordRelevanceOutput
		require.NoError(t, val.Get(&output))

		assert.Len(t, output.Accepted, 1)
		assert.Equal(t, "relevant term", output.Accepted[0].Keyword)
		assert.Len(t, output.Rejected, 1)
		assert.Equal(t, "irrelevant term", output.Rejected[0].Keyword)
	})

	t.Run("accepts all keywords when all above threshold", func(t *testing.T) {
		embedder := &mockEmbedder{
			embeddings: [][]float32{{0.9, 0.1, 0.0}, {0.8, 0.2, 0.0}},
		}
		act := NewEmbeddingActivities(embedder)

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(act.ScoreKeywordRelevance)

		val, err := testEnv.ExecuteActivity(act.ScoreKeywordRelevance, ScoreKeywordRelevanceInput{
			Keywords:       []string{"kw1", "kw2"},
			QueryEmbedding: []float32{1.0, 0.0, 0.0},
			Threshold:      0.3,
		})
		require.NoError(t, err)

		var output ScoreKeywordRelevanceOutput
		require.NoError(t, val.Get(&output))
		assert.Len(t, output.Accepted, 2)
		assert.Empty(t, output.Rejected)
	})

	t.Run("returns empty when no keywords", func(t *testing.T) {
		embedder := &mockEmbedder{}
		act := NewEmbeddingActivities(embedder)

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(act.ScoreKeywordRelevance)

		val, err := testEnv.ExecuteActivity(act.ScoreKeywordRelevance, ScoreKeywordRelevanceInput{
			Keywords:       []string{},
			QueryEmbedding: []float32{1.0, 0.0, 0.0},
			Threshold:      0.3,
		})
		require.NoError(t, err)

		var output ScoreKeywordRelevanceOutput
		require.NoError(t, val.Get(&output))
		assert.Empty(t, output.Accepted)
		assert.Empty(t, output.Rejected)
	})

	t.Run("returns error when embedder fails", func(t *testing.T) {
		embedder := &mockEmbedder{err: fmt.Errorf("API error")}
		act := NewEmbeddingActivities(embedder)

		suite := &testsuite.WorkflowTestSuite{}
		testEnv := suite.NewTestActivityEnvironment()
		testEnv.RegisterActivity(act.ScoreKeywordRelevance)

		_, err := testEnv.ExecuteActivity(act.ScoreKeywordRelevance, ScoreKeywordRelevanceInput{
			Keywords:       []string{"kw1"},
			QueryEmbedding: []float32{1.0, 0.0, 0.0},
			Threshold:      0.3,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "embed keywords")
	})
}

func TestCosineSimilarity(t *testing.T) {
	t.Run("identical vectors", func(t *testing.T) {
		score := cosineSimilarity([]float32{1, 0, 0}, []float32{1, 0, 0})
		assert.InDelta(t, 1.0, score, 0.001)
	})

	t.Run("orthogonal vectors", func(t *testing.T) {
		score := cosineSimilarity([]float32{1, 0, 0}, []float32{0, 1, 0})
		assert.InDelta(t, 0.0, score, 0.001)
	})

	t.Run("opposite vectors", func(t *testing.T) {
		score := cosineSimilarity([]float32{1, 0, 0}, []float32{-1, 0, 0})
		assert.InDelta(t, -1.0, score, 0.001)
	})

	t.Run("zero vector returns 0", func(t *testing.T) {
		score := cosineSimilarity([]float32{0, 0, 0}, []float32{1, 0, 0})
		assert.Equal(t, 0.0, score)
	})

	t.Run("different lengths returns 0", func(t *testing.T) {
		score := cosineSimilarity([]float32{1, 0}, []float32{1, 0, 0})
		assert.Equal(t, 0.0, score)
	})

	t.Run("empty vectors returns 0", func(t *testing.T) {
		score := cosineSimilarity([]float32{}, []float32{})
		assert.Equal(t, 0.0, score)
	})
}
