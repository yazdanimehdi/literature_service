package llm

import (
	"context"
	"fmt"
	"time"

	sharedllm "github.com/helixir/llm"
	"github.com/helixir/llm/openai"
)

// EmbedderFactoryConfig holds the parameters needed to create an Embedder.
// It mirrors the structure of FactoryConfig but is tailored for embedding
// providers. Currently only OpenAI is supported.
type EmbedderFactoryConfig struct {
	// Provider is the embedding provider name (currently only "openai").
	Provider string
	// Timeout is the timeout for embedding API calls.
	Timeout time.Duration
	// MaxRetries is the maximum number of retries for failed calls.
	MaxRetries int
	// RetryDelay is the base delay between retries.
	RetryDelay time.Duration
	// OpenAI contains OpenAI-specific settings (reused from FactoryConfig).
	OpenAI OpenAIConfig
}

// NewEmbedder creates an Embedder based on the factory configuration.
// The context is reserved for future providers that may require it during
// initialization (e.g. credential exchange).
func NewEmbedder(_ context.Context, cfg EmbedderFactoryConfig) (sharedllm.Embedder, error) {
	cc := embedderClientConfig(cfg)

	switch cfg.Provider {
	case "openai":
		return openai.NewEmbedder(openai.EmbedderConfig{
			APIKey:  cfg.OpenAI.APIKey,
			Model:   cfg.OpenAI.Model,
			BaseURL: cfg.OpenAI.BaseURL,
		}, cc)
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %q (only openai is currently supported)", cfg.Provider)
	}
}

// embedderClientConfig adapts the EmbedderFactoryConfig to a shared llm.ClientConfig,
// applying sensible defaults for zero values.
func embedderClientConfig(cfg EmbedderFactoryConfig) sharedllm.ClientConfig {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	retryDelay := cfg.RetryDelay
	if retryDelay == 0 {
		retryDelay = 2 * time.Second
	}
	return sharedllm.ClientConfig{
		Timeout:    timeout,
		MaxRetries: cfg.MaxRetries,
		RetryDelay: retryDelay,
	}
}
