package llm

import (
	"context"
	"fmt"
	"time"

	sharedllm "github.com/helixir/llm"
	"github.com/helixir/llm/anthropic"
	"github.com/helixir/llm/azure"
	"github.com/helixir/llm/bedrock"
	"github.com/helixir/llm/gemini"
	"github.com/helixir/llm/openai"
)

// FactoryConfig holds the parameters needed to create a KeywordExtractor.
// This is defined in the llm package to avoid importing the config package,
// keeping the llm package free of infrastructure dependencies.
type FactoryConfig struct {
	// Provider is the LLM provider name ("openai", "anthropic", "azure", "bedrock", "gemini", "vertex").
	Provider string
	// Temperature is the LLM temperature setting.
	Temperature float64
	// Timeout is the timeout for LLM API calls.
	Timeout time.Duration
	// MaxRetries is the maximum number of retries for failed calls.
	MaxRetries int
	// RetryDelay is the base delay between retries.
	RetryDelay time.Duration
	// OpenAI contains OpenAI-specific settings.
	OpenAI OpenAIConfig
	// Anthropic contains Anthropic-specific settings.
	Anthropic AnthropicConfig
	// Azure contains Azure OpenAI-specific settings.
	Azure AzureConfig
	// Bedrock contains AWS Bedrock-specific settings.
	Bedrock BedrockConfig
	// Gemini contains Google Gemini/Vertex AI-specific settings.
	Gemini GeminiConfig
	// Resilience holds optional rate limiter + circuit breaker configuration.
	// If nil or disabled, no resilience wrapper is applied.
	Resilience *ResilienceConfig
}

// ResilienceConfig holds rate limiter and circuit breaker settings for the factory.
type ResilienceConfig struct {
	// RateLimitRPS is the steady-state requests per second allowed.
	RateLimitRPS float64

	// RateLimitBurst is the maximum burst size above the steady-state rate.
	RateLimitBurst int

	// RateLimitMinRPS is the minimum RPS after adaptive backoff.
	RateLimitMinRPS float64

	// RateLimitRecoverySec is the seconds to recover from min to normal RPS.
	RateLimitRecoverySec int

	// CBConsecutiveThreshold is the consecutive failures before the circuit breaker opens.
	CBConsecutiveThreshold int

	// CBFailureRateThreshold is the failure rate (0.0-1.0) that triggers the circuit breaker.
	CBFailureRateThreshold float64

	// CBWindowSize is the number of recent calls tracked for failure rate calculation.
	CBWindowSize int

	// CBCooldownSec is the seconds the circuit breaker stays open before probing.
	CBCooldownSec int

	// CBProbeCount is the number of probe calls allowed during half-open state.
	CBProbeCount int
}

// OpenAIConfig holds OpenAI-specific settings.
type OpenAIConfig struct {
	// APIKey is the OpenAI API key.
	APIKey string
	// Model is the model identifier (e.g., "gpt-4-turbo").
	Model string
	// BaseURL is the API base URL (empty means default).
	BaseURL string
}

// AnthropicConfig holds Anthropic-specific settings.
type AnthropicConfig struct {
	// APIKey is the Anthropic API key.
	APIKey string
	// Model is the model identifier (e.g., "claude-3-sonnet-20240229").
	Model string
	// BaseURL is the API base URL.
	BaseURL string
}

// AzureConfig holds Azure OpenAI-specific settings.
type AzureConfig struct {
	ResourceName   string
	DeploymentName string
	APIKey         string
	APIVersion     string
	Model          string
}

// BedrockConfig holds AWS Bedrock-specific settings.
type BedrockConfig struct {
	Region string
	Model  string
}

// GeminiConfig holds Google Gemini/Vertex AI-specific settings.
type GeminiConfig struct {
	APIKey   string
	Project  string
	Location string
	Model    string
}

// NewKeywordExtractor creates a KeywordExtractor based on the configuration.
// All providers use the shared llm package. If Resilience config is provided,
// the client is wrapped with rate limiting and circuit breaking.
// The context is used during client initialization (e.g. Bedrock, Gemini SDK setup).
func NewKeywordExtractor(ctx context.Context, cfg FactoryConfig) (KeywordExtractor, error) {
	// Step 1: Create the base shared client for the selected provider.
	client, err := createClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Step 2: Optionally wrap with resilience (rate limiter + circuit breaker).
	if cfg.Resilience != nil {
		client = wrapWithResilience(client, cfg.Resilience)
	}

	// Step 3: Adapt shared Client to KeywordExtractor interface.
	return NewKeywordExtractorFromClient(client), nil
}

// sharedClientConfig adapts the FactoryConfig to the shared llm package ClientConfig.
func sharedClientConfig(cfg FactoryConfig) sharedllm.ClientConfig {
	retryDelay := cfg.RetryDelay
	if retryDelay == 0 {
		retryDelay = 2 * time.Second
	}
	return sharedllm.ClientConfig{
		Timeout:    cfg.Timeout,
		MaxRetries: cfg.MaxRetries,
		RetryDelay: retryDelay,
	}
}

// createClient creates a base LLM client for the selected provider without resilience wrapping.
func createClient(ctx context.Context, cfg FactoryConfig) (sharedllm.Client, error) {
	cc := sharedClientConfig(cfg)
	switch cfg.Provider {
	case "openai":
		return openai.New(openai.Config{
			APIKey:  cfg.OpenAI.APIKey,
			Model:   cfg.OpenAI.Model,
			BaseURL: cfg.OpenAI.BaseURL,
		}, cc)
	case "anthropic":
		return anthropic.New(anthropic.Config{
			APIKey:  cfg.Anthropic.APIKey,
			Model:   cfg.Anthropic.Model,
			BaseURL: cfg.Anthropic.BaseURL,
		}, cc)
	case "azure":
		return azure.New(azure.Config{
			ResourceName:   cfg.Azure.ResourceName,
			DeploymentName: cfg.Azure.DeploymentName,
			APIKey:         cfg.Azure.APIKey,
			APIVersion:     cfg.Azure.APIVersion,
			Model:          cfg.Azure.Model,
		}, cc)
	case "bedrock":
		return bedrock.New(ctx, bedrock.Config{
			Region: cfg.Bedrock.Region,
			Model:  cfg.Bedrock.Model,
		}, cc)
	case "gemini":
		return createGeminiClient(ctx, cfg, false)
	case "vertex":
		return createGeminiClient(ctx, cfg, true)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q", cfg.Provider)
	}
}

// createGeminiClient creates a Gemini or Vertex AI client, auto-detecting mode based on configuration.
func createGeminiClient(ctx context.Context, cfg FactoryConfig, forceVertex bool) (sharedllm.Client, error) {
	geminiCfg := gemini.Config{Model: cfg.Gemini.Model}
	if forceVertex || (cfg.Gemini.Project != "" && cfg.Gemini.APIKey == "") {
		geminiCfg.Project = cfg.Gemini.Project
		geminiCfg.Location = cfg.Gemini.Location
	} else {
		geminiCfg.APIKey = cfg.Gemini.APIKey
	}
	return gemini.New(ctx, geminiCfg, sharedClientConfig(cfg))
}

// wrapWithResilience wraps a client with rate limiter and circuit breaker middleware.
func wrapWithResilience(client sharedllm.Client, cfg *ResilienceConfig) sharedllm.Client {
	sharedCfg := &sharedllm.Config{
		RateLimitRPS:           cfg.RateLimitRPS,
		RateLimitBurst:         cfg.RateLimitBurst,
		RateLimitMinRPS:        cfg.RateLimitMinRPS,
		RateLimitRecoverySec:   cfg.RateLimitRecoverySec,
		CBConsecutiveThreshold: cfg.CBConsecutiveThreshold,
		CBFailureRateThreshold: cfg.CBFailureRateThreshold,
		CBWindowSize:           cfg.CBWindowSize,
		CBCooldownSec:          cfg.CBCooldownSec,
		CBProbeCount:           cfg.CBProbeCount,
		BudgetEnabled:          false, // budget handled at activity layer
	}
	return sharedllm.NewResilientClientFromConfig(client, sharedCfg, nil, sharedllm.BudgetKey{})
}
