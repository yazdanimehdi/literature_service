package llm

import (
	"fmt"
	"time"
)

// FactoryConfig holds the parameters needed to create a KeywordExtractor.
// This is defined in the llm package to avoid importing the config package,
// keeping the llm package free of infrastructure dependencies.
type FactoryConfig struct {
	// Provider is the LLM provider name ("openai" or "anthropic").
	Provider string
	// Temperature is the LLM temperature setting.
	Temperature float64
	// Timeout is the timeout for LLM API calls.
	Timeout time.Duration
	// MaxRetries is the maximum number of retries for failed calls.
	MaxRetries int
	// OpenAI contains OpenAI-specific settings.
	OpenAI OpenAIConfig
	// Anthropic contains Anthropic-specific settings.
	Anthropic AnthropicConfig
}

// NewKeywordExtractor creates a KeywordExtractor based on the configuration.
// Supports "openai" and "anthropic" providers. Returns an error for unsupported
// or empty provider values.
func NewKeywordExtractor(cfg FactoryConfig) (KeywordExtractor, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIProvider(cfg.OpenAI, cfg.Temperature, cfg.Timeout, cfg.MaxRetries), nil
	case "anthropic":
		return NewAnthropicProvider(cfg.Anthropic, cfg.Temperature, cfg.Timeout, cfg.MaxRetries), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q", cfg.Provider)
	}
}
