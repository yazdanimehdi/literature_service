package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEmbedder_OpenAI(t *testing.T) {
	t.Parallel()

	// Stand up a minimal httptest server that returns a valid embedding response.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"object": "list",
			"model":  "text-embedding-3-small",
			"data": []map[string]any{
				{"object": "embedding", "index": 0, "embedding": []float64{0.1, 0.2, 0.3}},
			},
			"usage": map[string]int{"prompt_tokens": 2, "total_tokens": 2},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := EmbedderFactoryConfig{
		Provider:   "openai",
		Timeout:    10 * time.Second,
		MaxRetries: 1,
		RetryDelay: 1 * time.Second,
		OpenAI: OpenAIConfig{
			APIKey:  "sk-test-key",
			Model:   "text-embedding-3-small",
			BaseURL: server.URL,
		},
	}

	embedder, err := NewEmbedder(context.Background(), cfg)

	require.NoError(t, err)
	require.NotNil(t, embedder)
	assert.Equal(t, "openai", embedder.Provider())
	assert.Equal(t, "text-embedding-3-small", embedder.EmbeddingModel())
}

func TestNewEmbedder_UnsupportedProvider(t *testing.T) {
	t.Parallel()

	cfg := EmbedderFactoryConfig{
		Provider: "cohere",
		OpenAI: OpenAIConfig{
			APIKey: "sk-test-key",
		},
	}

	embedder, err := NewEmbedder(context.Background(), cfg)

	require.Error(t, err)
	assert.Nil(t, embedder)
	assert.Contains(t, err.Error(), "unsupported embedding provider")
	assert.Contains(t, err.Error(), "cohere")
	assert.Contains(t, err.Error(), "only openai is currently supported")
}

func TestNewEmbedder_EmptyProvider(t *testing.T) {
	t.Parallel()

	cfg := EmbedderFactoryConfig{
		Provider: "",
		OpenAI: OpenAIConfig{
			APIKey: "sk-test-key",
		},
	}

	embedder, err := NewEmbedder(context.Background(), cfg)

	require.Error(t, err)
	assert.Nil(t, embedder)
	assert.Contains(t, err.Error(), "unsupported embedding provider")
}

func TestNewEmbedder_DefaultTimeouts(t *testing.T) {
	t.Parallel()

	// Create with zero Timeout and RetryDelay to verify defaults are applied
	// without error.
	cfg := EmbedderFactoryConfig{
		Provider: "openai",
		// Timeout, MaxRetries, RetryDelay all zero-valued
		OpenAI: OpenAIConfig{
			APIKey: "sk-test-key",
			Model:  "text-embedding-3-small",
		},
	}

	embedder, err := NewEmbedder(context.Background(), cfg)

	require.NoError(t, err)
	require.NotNil(t, embedder)
	assert.Equal(t, "openai", embedder.Provider())
}

func TestNewEmbedder_OpenAI_MissingAPIKey(t *testing.T) {
	t.Parallel()

	cfg := EmbedderFactoryConfig{
		Provider: "openai",
		OpenAI: OpenAIConfig{
			// APIKey intentionally empty
			Model: "text-embedding-3-small",
		},
	}

	embedder, err := NewEmbedder(context.Background(), cfg)

	require.Error(t, err)
	assert.Nil(t, embedder)
	assert.Contains(t, err.Error(), "API key")
}
