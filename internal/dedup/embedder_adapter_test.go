package dedup

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	llm "github.com/helixir/llm"
	"github.com/helixir/llm/llmtest"
)

func TestEmbedderAdapter_EmbedText(t *testing.T) {
	t.Parallel()

	t.Run("returns correct vector and passes input", func(t *testing.T) {
		t.Parallel()

		expectedVec := []float32{0.1, 0.2, 0.3, 0.4}
		mock := llmtest.FixedEmbedding([][]float32{expectedVec})
		adapter := NewEmbedderAdapter(mock)

		vec, err := adapter.EmbedText(context.Background(), "hello world")
		require.NoError(t, err)
		assert.Equal(t, expectedVec, vec)

		// Verify the input was passed correctly.
		calls := mock.EmbedCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, []string{"hello world"}, calls[0].Input)
	})

	t.Run("propagates error from embedder", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("provider unavailable")
		mock := llmtest.FailingEmbedder(expectedErr)
		adapter := NewEmbedderAdapter(mock)

		vec, err := adapter.EmbedText(context.Background(), "some text")
		assert.Nil(t, vec)
		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("returns error when no embeddings in response", func(t *testing.T) {
		t.Parallel()

		// MockEmbedder with custom function that returns empty embeddings.
		mock := &llmtest.MockEmbedder{
			ProviderName:   "mock",
			EmbedModelName: "mock-model",
			EmbedFunc: func(ctx context.Context, req llm.EmbedRequest) (*llm.EmbedResponse, error) {
				return &llm.EmbedResponse{
					Embeddings: [][]float32{},
					Model:      "mock-model",
				}, nil
			},
		}
		adapter := NewEmbedderAdapter(mock)

		vec, err := adapter.EmbedText(context.Background(), "some text")
		assert.Nil(t, vec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no embeddings returned")
	})
}
