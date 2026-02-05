package llm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewKeywordExtractor_OpenAI(t *testing.T) {
	t.Parallel()

	cfg := FactoryConfig{
		Provider:    "openai",
		Timeout:     30 * time.Second,
		MaxRetries:  3,
		Temperature: 0.7,
		OpenAI: OpenAIConfig{
			APIKey:  "sk-test-key",
			Model:   "gpt-4o",
			BaseURL: "https://api.openai.com/v1",
		},
	}

	extractor, err := NewKeywordExtractor(cfg)

	require.NoError(t, err)
	require.NotNil(t, extractor)

	// Verify the returned extractor is an OpenAIProvider.
	openaiProvider, ok := extractor.(*OpenAIProvider)
	require.True(t, ok, "expected *OpenAIProvider, got %T", extractor)

	assert.Equal(t, "openai", openaiProvider.Provider())
	assert.Equal(t, "gpt-4o", openaiProvider.Model())
	assert.Equal(t, 0.7, openaiProvider.temperature)
	assert.Equal(t, 3, openaiProvider.maxRetries)
	assert.Equal(t, "sk-test-key", openaiProvider.apiKey)
}

func TestNewKeywordExtractor_Anthropic(t *testing.T) {
	t.Parallel()

	cfg := FactoryConfig{
		Provider:    "anthropic",
		Timeout:     45 * time.Second,
		MaxRetries:  2,
		Temperature: 0.5,
		Anthropic: AnthropicConfig{
			APIKey:  "sk-ant-test-key",
			Model:   "claude-3-sonnet-20240229",
			BaseURL: "https://api.anthropic.com",
		},
	}

	extractor, err := NewKeywordExtractor(cfg)

	require.NoError(t, err)
	require.NotNil(t, extractor)

	// Verify the returned extractor is an AnthropicProvider.
	anthropicProvider, ok := extractor.(*AnthropicProvider)
	require.True(t, ok, "expected *AnthropicProvider, got %T", extractor)

	assert.Equal(t, "anthropic", anthropicProvider.Provider())
	assert.Equal(t, "claude-3-sonnet-20240229", anthropicProvider.Model())
	assert.Equal(t, 0.5, anthropicProvider.temperature)
	assert.Equal(t, 2, anthropicProvider.maxRetries)
	assert.Equal(t, "sk-ant-test-key", anthropicProvider.apiKey)
}

func TestNewKeywordExtractor_Unknown(t *testing.T) {
	t.Parallel()

	cfg := FactoryConfig{
		Provider: "unknown-provider",
	}

	extractor, err := NewKeywordExtractor(cfg)

	require.Error(t, err)
	assert.Nil(t, extractor)
	assert.Contains(t, err.Error(), "unsupported LLM provider")
	assert.Contains(t, err.Error(), "unknown-provider")
}

func TestNewKeywordExtractor_EmptyProvider(t *testing.T) {
	t.Parallel()

	cfg := FactoryConfig{
		Provider: "",
	}

	extractor, err := NewKeywordExtractor(cfg)

	require.Error(t, err)
	assert.Nil(t, extractor)
	assert.Contains(t, err.Error(), "unsupported LLM provider")
}
