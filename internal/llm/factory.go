package llm

import (
	"context"
	"fmt"
	"time"

	sharedllm "github.com/helixir/llm"
	sharedanthropic "github.com/helixir/llm/anthropic"
	"github.com/helixir/llm/azure"
	"github.com/helixir/llm/bedrock"
	"github.com/helixir/llm/gemini"
	sharedopenai "github.com/helixir/llm/openai"
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
// Supports "openai", "anthropic", "azure", "bedrock", "gemini", and "vertex" providers.
// Returns an error for unsupported or empty provider values.
func NewKeywordExtractor(cfg FactoryConfig) (KeywordExtractor, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIProvider(cfg.OpenAI, cfg.Temperature, cfg.Timeout, cfg.MaxRetries), nil
	case "anthropic":
		return NewAnthropicProvider(cfg.Anthropic, cfg.Temperature, cfg.Timeout, cfg.MaxRetries), nil
	case "azure":
		return newAzureExtractor(cfg)
	case "bedrock":
		return newBedrockExtractor(cfg)
	case "gemini":
		return newGeminiExtractor(cfg, false)
	case "vertex":
		return newGeminiExtractor(cfg, true)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q", cfg.Provider)
	}
}

func sharedClientConfig(cfg FactoryConfig) sharedllm.ClientConfig {
	return sharedllm.ClientConfig{
		Timeout:    cfg.Timeout,
		MaxRetries: cfg.MaxRetries,
		RetryDelay: 2 * time.Second,
	}
}

func newAzureExtractor(cfg FactoryConfig) (KeywordExtractor, error) {
	client, err := azure.New(azure.Config{
		ResourceName:   cfg.Azure.ResourceName,
		DeploymentName: cfg.Azure.DeploymentName,
		APIKey:         cfg.Azure.APIKey,
		APIVersion:     cfg.Azure.APIVersion,
		Model:          cfg.Azure.Model,
	}, sharedClientConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure OpenAI client: %w", err)
	}
	return NewKeywordExtractorFromClient(client), nil
}

func newBedrockExtractor(cfg FactoryConfig) (KeywordExtractor, error) {
	client, err := bedrock.New(context.Background(), bedrock.Config{
		Region: cfg.Bedrock.Region,
		Model:  cfg.Bedrock.Model,
	}, sharedClientConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create Bedrock client: %w", err)
	}
	return NewKeywordExtractorFromClient(client), nil
}

func newGeminiExtractor(cfg FactoryConfig, forceVertex bool) (KeywordExtractor, error) {
	geminiCfg := gemini.Config{
		Model: cfg.Gemini.Model,
	}

	if forceVertex || (cfg.Gemini.Project != "" && cfg.Gemini.APIKey == "") {
		geminiCfg.Project = cfg.Gemini.Project
		geminiCfg.Location = cfg.Gemini.Location
	} else {
		geminiCfg.APIKey = cfg.Gemini.APIKey
	}

	client, err := gemini.New(context.Background(), geminiCfg, sharedClientConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	return NewKeywordExtractorFromClient(client), nil
}

// newSharedOpenAIExtractor creates a KeywordExtractor using the shared OpenAI client.
// This is available as an alternative to the built-in OpenAI provider.
func newSharedOpenAIExtractor(cfg FactoryConfig) (KeywordExtractor, error) {
	client, err := sharedopenai.New(sharedopenai.Config{
		APIKey:  cfg.OpenAI.APIKey,
		Model:   cfg.OpenAI.Model,
		BaseURL: cfg.OpenAI.BaseURL,
	}, sharedClientConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create shared OpenAI client: %w", err)
	}
	return NewKeywordExtractorFromClient(client), nil
}

// newSharedAnthropicExtractor creates a KeywordExtractor using the shared Anthropic client.
// This is available as an alternative to the built-in Anthropic provider.
func newSharedAnthropicExtractor(cfg FactoryConfig) (KeywordExtractor, error) {
	client, err := sharedanthropic.New(sharedanthropic.Config{
		APIKey:  cfg.Anthropic.APIKey,
		Model:   cfg.Anthropic.Model,
		BaseURL: cfg.Anthropic.BaseURL,
	}, sharedClientConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create shared Anthropic client: %w", err)
	}
	return NewKeywordExtractorFromClient(client), nil
}
