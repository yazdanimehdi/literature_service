// Package main provides the entry point for the literature review Temporal worker.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"go.temporal.io/sdk/client"

	"github.com/helixir/literature-review-service/internal/config"
	"github.com/helixir/literature-review-service/internal/database"
	"github.com/helixir/literature-review-service/internal/dedup"
	"github.com/helixir/literature-review-service/internal/ingestion"
	"github.com/helixir/literature-review-service/internal/llm"
	"github.com/helixir/literature-review-service/internal/observability"
	"github.com/helixir/literature-review-service/internal/outbox"
	"github.com/helixir/literature-review-service/internal/papersources"
	"github.com/helixir/literature-review-service/internal/papersources/openalex"
	"github.com/helixir/literature-review-service/internal/papersources/pubmed"
	"github.com/helixir/literature-review-service/internal/papersources/semanticscholar"
	"github.com/helixir/literature-review-service/internal/qdrant"
	"github.com/helixir/literature-review-service/internal/repository"
	"github.com/helixir/literature-review-service/internal/temporal"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
	"github.com/helixir/literature-review-service/internal/temporal/workflows"
	outboxpg "github.com/helixir/outbox/postgres"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Set up structured logging.
	logger := observability.NewLogger(observability.LoggingConfig{
		Level:      cfg.Logging.Level,
		Format:     cfg.Logging.Format,
		Output:     cfg.Logging.Output,
		AddSource:  cfg.Logging.AddSource,
		TimeFormat: cfg.Logging.TimeFormat,
	})
	logger = logger.With().Str("component", "worker").Logger()
	logger.Info().Msg("literature-review-service worker starting")

	// Set up context with graceful shutdown via OS signals.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Connect to PostgreSQL.
	db, err := database.New(ctx, &cfg.Database, logger)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()
	logger.Info().Msg("database connection established")

	// Create repositories.
	reviewRepo := repository.NewPgReviewRepository(db)
	paperRepo := repository.NewPgPaperRepository(db)
	keywordRepo := repository.NewPgKeywordRepository(db)

	// Build resilience config if enabled.
	var resilienceCfg *llm.ResilienceConfig
	if cfg.LLM.Resilience.Enabled {
		resilienceCfg = &llm.ResilienceConfig{
			RateLimitRPS:           cfg.LLM.Resilience.RateLimitRPS,
			RateLimitBurst:         cfg.LLM.Resilience.RateLimitBurst,
			RateLimitMinRPS:        cfg.LLM.Resilience.RateLimitMinRPS,
			RateLimitRecoverySec:   cfg.LLM.Resilience.RateLimitRecoverySec,
			CBConsecutiveThreshold: cfg.LLM.Resilience.CBConsecutiveThreshold,
			CBFailureRateThreshold: cfg.LLM.Resilience.CBFailureRateThreshold,
			CBWindowSize:           cfg.LLM.Resilience.CBWindowSize,
			CBCooldownSec:          cfg.LLM.Resilience.CBCooldownSec,
			CBProbeCount:           cfg.LLM.Resilience.CBProbeCount,
		}
	}

	// Create LLM keyword extractor.
	extractor, err := llm.NewKeywordExtractor(ctx, llm.FactoryConfig{
		Provider:    cfg.LLM.Provider,
		Temperature: cfg.LLM.Temperature,
		Timeout:     cfg.LLM.Timeout,
		MaxRetries:  cfg.LLM.MaxRetries,
		RetryDelay:  cfg.LLM.RetryDelay,
		OpenAI: llm.OpenAIConfig{
			APIKey:  cfg.LLM.OpenAI.APIKey,
			Model:   cfg.LLM.OpenAI.Model,
			BaseURL: cfg.LLM.OpenAI.BaseURL,
		},
		Anthropic: llm.AnthropicConfig{
			APIKey:  cfg.LLM.Anthropic.APIKey,
			Model:   cfg.LLM.Anthropic.Model,
			BaseURL: cfg.LLM.Anthropic.BaseURL,
		},
		Azure: llm.AzureConfig{
			ResourceName:   cfg.LLM.Azure.ResourceName,
			DeploymentName: cfg.LLM.Azure.DeploymentName,
			APIKey:         cfg.LLM.Azure.APIKey,
			APIVersion:     cfg.LLM.Azure.APIVersion,
			Model:          cfg.LLM.Azure.Model,
		},
		Bedrock: llm.BedrockConfig{
			Region: cfg.LLM.Bedrock.Region,
			Model:  cfg.LLM.Bedrock.Model,
		},
		Gemini: llm.GeminiConfig{
			APIKey:   cfg.LLM.Gemini.APIKey,
			Project:  cfg.LLM.Gemini.Project,
			Location: cfg.LLM.Gemini.Location,
			Model:    cfg.LLM.Gemini.Model,
		},
		Resilience: resilienceCfg,
	})
	if err != nil {
		return fmt.Errorf("create LLM extractor: %w", err)
	}

	// Create paper source registry and register enabled sources.
	registry := papersources.NewRegistry()
	registerPaperSources(registry, cfg, logger)

	// Create ingestion client.
	ingestionClient, err := ingestion.NewClient(ingestion.Config{
		Address: cfg.IngestionService.Address,
		Timeout: cfg.IngestionService.Timeout,
		TLS:     cfg.IngestionService.TLS,
	})
	if err != nil {
		return fmt.Errorf("create ingestion client: %w", err)
	}
	defer ingestionClient.Close()
	logger.Info().Str("address", cfg.IngestionService.Address).Msg("ingestion service client created")

	// Create Qdrant client for dedup.
	qdrantClient, err := qdrant.NewClient(qdrant.Config{
		Address:        cfg.Qdrant.Address,
		CollectionName: cfg.Qdrant.CollectionName,
		VectorSize:     cfg.Qdrant.VectorSize,
	})
	if err != nil {
		return fmt.Errorf("create qdrant client: %w", err)
	}
	defer qdrantClient.Close()

	if err := qdrantClient.EnsureCollection(ctx); err != nil {
		return fmt.Errorf("ensure qdrant collection: %w", err)
	}
	logger.Info().Str("address", cfg.Qdrant.Address).Str("collection", cfg.Qdrant.CollectionName).Msg("qdrant client connected")

	// Create LLM embedder.
	embedder, err := llm.NewEmbedder(ctx, llm.EmbedderFactoryConfig{
		Provider:   cfg.LLM.EmbeddingProvider,
		Timeout:    cfg.LLM.Timeout,
		MaxRetries: cfg.LLM.MaxRetries,
		RetryDelay: cfg.LLM.RetryDelay,
		OpenAI: llm.OpenAIConfig{
			APIKey:  cfg.LLM.OpenAI.APIKey,
			Model:   cfg.LLM.EmbeddingModel,
			BaseURL: cfg.LLM.OpenAI.BaseURL,
		},
	})
	if err != nil {
		return fmt.Errorf("create embedder: %w", err)
	}

	// Create dedup checker.
	dedupAdapter := dedup.NewQdrantAdapter(qdrantClient, paperRepo)
	embedderAdapter := dedup.NewEmbedderAdapter(embedder)
	dedupChecker := dedup.NewChecker(dedupAdapter, embedderAdapter, dedup.CheckerConfig{
		SimilarityThreshold: cfg.Qdrant.SimilarityThreshold,
		AuthorThreshold:     cfg.Qdrant.AuthorThreshold,
		TopK:                cfg.Qdrant.TopK,
	})

	// Create outbox publisher for event activities.
	outboxRepo := outboxpg.NewRepository(db.Pool(), nil)
	emitter := outbox.NewEmitter(outbox.EmitterConfig{
		ServiceName: "literature-review-service",
	})
	adapter := outbox.NewAdapter(outboxRepo)
	publisher := outbox.NewPublisher(emitter, adapter)

	// Create metrics (optional).
	metrics := observability.NewMetrics("literature_review")

	// Create Temporal client.
	temporalClient, err := client.Dial(client.Options{
		HostPort:  cfg.Temporal.HostPort,
		Namespace: cfg.Temporal.Namespace,
		Logger:    observability.NewTemporalLogger(logger),
	})
	if err != nil {
		return fmt.Errorf("connect to temporal: %w", err)
	}
	defer temporalClient.Close()
	logger.Info().
		Str("host_port", cfg.Temporal.HostPort).
		Str("namespace", cfg.Temporal.Namespace).
		Msg("temporal client connected")

	// Create WorkerManager.
	workerConfig := temporal.DefaultWorkerConfig(cfg.Temporal.TaskQueue)
	manager, err := temporal.NewWorkerManager(temporalClient, workerConfig)
	if err != nil {
		return fmt.Errorf("create worker manager: %w", err)
	}

	// Register the workflow.
	manager.RegisterWorkflow(workflows.LiteratureReviewWorkflow)

	// Create and register all activity structs.
	budgetReporter := activities.NewOutboxBudgetReporter(db.Pool())
	llmActivities := activities.NewLLMActivities(extractor, metrics,
		activities.WithBudgetReporter(budgetReporter),
	)
	searchActivities := activities.NewSearchActivities(registry, metrics)
	statusActivities := activities.NewStatusActivities(reviewRepo, keywordRepo, paperRepo, metrics)
	ingestionActivities := activities.NewIngestionActivities(ingestionClient, metrics)
	eventActivities := activities.NewEventActivities(publisher)
	dedupActivities := activities.NewDedupActivities(dedupChecker)

	manager.RegisterActivity(llmActivities)
	manager.RegisterActivity(searchActivities)
	manager.RegisterActivity(statusActivities)
	manager.RegisterActivity(ingestionActivities)
	manager.RegisterActivity(eventActivities)
	manager.RegisterActivity(dedupActivities)

	logger.Info().
		Str("task_queue", cfg.Temporal.TaskQueue).
		Msg("starting temporal worker")

	// Start the worker and block until context is cancelled.
	if err := manager.Start(ctx); err != nil {
		if ctx.Err() != nil {
			logger.Info().Msg("worker stopped via signal")
			return nil
		}
		return fmt.Errorf("worker error: %w", err)
	}

	return nil
}

// registerPaperSources registers all enabled paper sources with the registry.
func registerPaperSources(registry *papersources.Registry, cfg *config.Config, logger zerolog.Logger) {
	// Semantic Scholar.
	if cfg.PaperSources.SemanticScholar.Enabled {
		ssCfg := cfg.PaperSources.SemanticScholar
		ssClient := semanticscholar.NewClient(semanticscholar.Config{
			BaseURL:    ssCfg.BaseURL,
			APIKey:     ssCfg.APIKey,
			Timeout:    ssCfg.Timeout,
			RateLimit:  ssCfg.RateLimit,
			MaxResults: ssCfg.MaxResults,
			Enabled:    true,
		}, nil)
		registry.Register(ssClient)
		logger.Info().Msg("registered paper source: Semantic Scholar")
	}

	// OpenAlex.
	if cfg.PaperSources.OpenAlex.Enabled {
		oaCfg := cfg.PaperSources.OpenAlex
		oaClient := openalex.New(openalex.Config{
			BaseURL:    oaCfg.BaseURL,
			Timeout:    oaCfg.Timeout,
			RateLimit:  oaCfg.RateLimit,
			MaxResults: oaCfg.MaxResults,
			Enabled:    true,
		})
		registry.Register(oaClient)
		logger.Info().Msg("registered paper source: OpenAlex")
	}

	// PubMed.
	if cfg.PaperSources.PubMed.Enabled {
		pmCfg := cfg.PaperSources.PubMed
		pmClient := pubmed.New(pubmed.Config{
			BaseURL:    pmCfg.BaseURL,
			APIKey:     pmCfg.APIKey,
			Timeout:    pmCfg.Timeout,
			RateLimit:  pmCfg.RateLimit,
			MaxResults: pmCfg.MaxResults,
			Enabled:    true,
		})
		registry.Register(pmClient)
		logger.Info().Msg("registered paper source: PubMed")
	}
}
