package llm

import (
	"fmt"

	"github.com/helixir/literature-review-service/internal/config"
)

// NewKeywordExtractor creates a KeywordExtractor based on the configuration.
// Supports "openai" and "anthropic" providers. Returns an error for unsupported
// or empty provider values.
func NewKeywordExtractor(cfg config.LLMConfig) (KeywordExtractor, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIProvider(cfg.OpenAI, cfg.Temperature, cfg.Timeout, cfg.MaxRetries), nil
	case "anthropic":
		return NewAnthropicProvider(cfg.Anthropic, cfg.Temperature, cfg.Timeout, cfg.MaxRetries), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q", cfg.Provider)
	}
}
