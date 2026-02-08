package llm

import (
	"context"
	"fmt"
	"time"

	sharedllm "github.com/helixir/llm"
	"github.com/helixir/llm/azure"
	"github.com/helixir/llm/bedrock"
	"github.com/helixir/llm/gemini"
	"github.com/helixir/llm/openai"
)

// EmbedderFactoryConfig holds the parameters needed to create an Embedder.
type EmbedderFactoryConfig struct {
	// Provider is the embedding provider name ("openai", "azure", "bedrock", "gemini", "vertex").
	Provider string
	// Timeout is the timeout for embedding API calls.
	Timeout time.Duration
	// MaxRetries is the maximum number of retries for failed calls.
	MaxRetries int
	// RetryDelay is the base delay between retries.
	RetryDelay time.Duration
	// OpenAI contains OpenAI-specific settings.
	OpenAI OpenAIConfig
	// Azure contains Azure OpenAI-specific settings.
	Azure AzureConfig
	// Bedrock contains AWS Bedrock-specific settings.
	Bedrock BedrockConfig
	// Gemini contains Google Gemini/Vertex AI-specific settings.
	Gemini GeminiConfig
}

// NewEmbedder creates an Embedder based on the factory configuration.
// The context is used by providers that require it during initialization
// (e.g. AWS credential loading, Google Cloud client creation).
func NewEmbedder(ctx context.Context, cfg EmbedderFactoryConfig) (sharedllm.Embedder, error) {
	cc := embedderClientConfig(cfg)

	switch cfg.Provider {
	case "openai":
		return openai.NewEmbedder(openai.EmbedderConfig{
			APIKey:  cfg.OpenAI.APIKey,
			Model:   cfg.OpenAI.Model,
			BaseURL: cfg.OpenAI.BaseURL,
		}, cc)
	case "azure":
		deploymentName := cfg.Azure.EmbeddingDeploymentName
		if deploymentName == "" {
			deploymentName = cfg.Azure.DeploymentName
		}
		model := cfg.Azure.EmbeddingModel
		if model == "" {
			model = deploymentName
		}
		return azure.NewEmbedder(azure.EmbedderConfig{
			ResourceName:   cfg.Azure.ResourceName,
			DeploymentName: deploymentName,
			APIKey:         cfg.Azure.APIKey,
			APIVersion:     cfg.Azure.APIVersion,
			Model:          model,
		}, cc)
	case "bedrock":
		return bedrock.NewEmbedder(ctx, bedrock.EmbedderConfig{
			Region: cfg.Bedrock.Region,
			Model:  cfg.Bedrock.Model,
		}, cc)
	case "gemini", "vertex":
		return gemini.NewEmbedder(ctx, gemini.EmbedderConfig{
			APIKey:   cfg.Gemini.APIKey,
			Project:  cfg.Gemini.Project,
			Location: cfg.Gemini.Location,
			Model:    cfg.Gemini.Model,
		}, cc)
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %q (supported: openai, azure, bedrock, gemini, vertex)", cfg.Provider)
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
