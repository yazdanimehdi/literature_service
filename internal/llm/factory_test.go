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
	assert.Equal(t, "openai", extractor.Provider())
	assert.Equal(t, "gpt-4o", extractor.Model())
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
	assert.Equal(t, "anthropic", extractor.Provider())
	assert.Equal(t, "claude-3-sonnet-20240229", extractor.Model())
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

func TestNewKeywordExtractor_WithResilience(t *testing.T) {
	t.Parallel()

	cfg := FactoryConfig{
		Provider: "openai",
		Timeout:  30 * time.Second,
		OpenAI:   OpenAIConfig{APIKey: "sk-test", Model: "gpt-4o"},
		Resilience: &ResilienceConfig{
			RateLimitRPS:           10,
			RateLimitBurst:         20,
			RateLimitMinRPS:        1.0,
			RateLimitRecoverySec:   60,
			CBConsecutiveThreshold: 5,
			CBFailureRateThreshold: 0.5,
			CBWindowSize:           20,
			CBCooldownSec:          30,
			CBProbeCount:           3,
		},
	}

	extractor, err := NewKeywordExtractor(cfg)

	require.NoError(t, err)
	require.NotNil(t, extractor)
	assert.Equal(t, "openai", extractor.Provider())
	assert.Equal(t, "gpt-4o", extractor.Model())
}

func TestNewKeywordExtractor_WithoutResilience(t *testing.T) {
	t.Parallel()

	cfg := FactoryConfig{
		Provider: "anthropic",
		Timeout:  30 * time.Second,
		Anthropic: AnthropicConfig{
			APIKey: "sk-ant-test",
			Model:  "claude-3-sonnet-20240229",
		},
		Resilience: nil,
	}

	extractor, err := NewKeywordExtractor(cfg)

	require.NoError(t, err)
	require.NotNil(t, extractor)
	assert.Equal(t, "anthropic", extractor.Provider())
}
