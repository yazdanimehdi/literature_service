// Package main provides the entry point for the literature review Temporal worker.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	sharedllm "github.com/helixir/llm"
	"github.com/rs/zerolog"
	"go.temporal.io/sdk/client"

	"github.com/helixir/literature-review-service/internal/budget"
	"github.com/helixir/literature-review-service/internal/config"
	"github.com/helixir/literature-review-service/internal/database"
	"github.com/helixir/literature-review-service/internal/dedup"
	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/ingestion"
	"github.com/helixir/literature-review-service/internal/llm"
	"github.com/helixir/literature-review-service/internal/observability"
	"github.com/helixir/literature-review-service/internal/outbox"
	"github.com/helixir/literature-review-service/internal/papersources"
	"github.com/helixir/literature-review-service/internal/papersources/arxiv"
	"github.com/helixir/literature-review-service/internal/papersources/biorxiv"
	"github.com/helixir/literature-review-service/internal/papersources/openalex"
	"github.com/helixir/literature-review-service/internal/papersources/pubmed"
	"github.com/helixir/literature-review-service/internal/papersources/scopus"
	"github.com/helixir/literature-review-service/internal/papersources/semanticscholar"
	"github.com/helixir/literature-review-service/internal/pdf"
	"github.com/helixir/literature-review-service/internal/qdrant"
	"github.com/helixir/literature-review-service/internal/repository"
	"github.com/helixir/literature-review-service/internal/temporal"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
	"github.com/helixir/literature-review-service/internal/temporal/workflows"
	outboxpg "github.com/helixir/outbox/postgres"
)

// Compile-time check that batchEmbedderAdapter implements activities.Embedder.
var _ activities.Embedder = (*batchEmbedderAdapter)(nil)

// batchEmbedderAdapter adapts the shared llm.Embedder to the activities.Embedder
// interface for use in EmbeddingActivities.
type batchEmbedderAdapter struct {
	embedder sharedllm.Embedder
}

// EmbedBatch implements activities.Embedder by delegating to the underlying
// llm.Embedder which natively supports batch embedding.
func (a *batchEmbedderAdapter) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	resp, err := a.embedder.Embed(ctx, sharedllm.EmbedRequest{
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("batch embed: %w", err)
	}

	return resp.Embeddings, nil
}

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

	// Create PDF downloader.
	pdfDownloader := pdf.NewDownloader(pdf.Config{
		Timeout:   cfg.IngestionService.Timeout,
		MaxSize:   100 * 1024 * 1024, // 100MB
		UserAgent: "Helixir-LitReview/1.0",
	})
	logger.Info().Msg("PDF downloader created")

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
	})
	if err != nil {
		return fmt.Errorf("create embedder: %w", err)
	}

	// Create dedup checker.
	dedupAdapter := dedup.NewQdrantAdapter(qdrantClient, paperRepo)
	dedupEmbedderAdapter := dedup.NewEmbedderAdapter(embedder)
	dedupChecker := dedup.NewChecker(dedupAdapter, dedupEmbedderAdapter, dedup.CheckerConfig{
		SimilarityThreshold: cfg.Qdrant.SimilarityThreshold,
		AuthorThreshold:     cfg.Qdrant.AuthorThreshold,
		TopK:                cfg.Qdrant.TopK,
	})

	// Create batch embedder adapter for embedding activities.
	embeddingAdapter := &batchEmbedderAdapter{embedder: embedder}

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

	// Register workflows.
	manager.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
	manager.RegisterWorkflow(workflows.PaperProcessingWorkflow)

	// Create and register all activity structs.
	budgetReporter := activities.NewOutboxBudgetReporter(db.Pool())
	llmOpts := []activities.LLMActivitiesOption{
		activities.WithBudgetReporter(budgetReporter),
	}
	if ca, ok := extractor.(llm.CoverageAssessor); ok {
		llmOpts = append(llmOpts, activities.WithCoverageAssessor(ca))
		logger.Info().Msg("coverage assessor wired to LLM activities")
	} else {
		logger.Warn().Msg("extractor does not implement CoverageAssessor; coverage review will be unavailable")
	}
	llmActivities := activities.NewLLMActivities(extractor, metrics, llmOpts...)
	searchActivities := activities.NewSearchActivities(registry, metrics)
	statusActivities := activities.NewStatusActivities(reviewRepo, keywordRepo, paperRepo, metrics)
	ingestionActivities := activities.NewIngestionActivities(ingestionClient, pdfDownloader, ingestionClient, metrics)
	eventActivities := activities.NewEventActivities(publisher)
	dedupActivities := activities.NewDedupActivities(dedupChecker)
	embeddingActivities := activities.NewEmbeddingActivities(embeddingAdapter)

	manager.RegisterActivity(llmActivities)
	manager.RegisterActivity(searchActivities)
	manager.RegisterActivity(statusActivities)
	manager.RegisterActivity(ingestionActivities)
	manager.RegisterActivity(eventActivities)
	manager.RegisterActivity(dedupActivities)
	manager.RegisterActivity(embeddingActivities)

	// Start budget listener if enabled and Kafka is configured.
	if cfg.BudgetListener.Enabled && cfg.Kafka.Enabled {
		budgetListener := budget.NewListener(
			budget.Config{
				Brokers: cfg.Kafka.Brokers,
				Topic:   cfg.BudgetListener.Topic,
				GroupID: cfg.BudgetListener.GroupID,
			},
			temporalClient,
			reviewRepo,
			logger,
		)
		defer func() {
			if err := budgetListener.Close(); err != nil {
				logger.Error().Err(err).Msg("failed to close budget listener")
			}
		}()

		// Run budget listener in background goroutine.
		go func() {
			if err := budgetListener.Run(ctx); err != nil && ctx.Err() == nil {
				logger.Error().Err(err).Msg("budget listener error")
			}
		}()

		logger.Info().
			Str("topic", cfg.BudgetListener.Topic).
			Str("group_id", cfg.BudgetListener.GroupID).
			Msg("budget listener started")
	}

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

	// arXiv.
	if cfg.PaperSources.ArXiv.Enabled {
		axCfg := cfg.PaperSources.ArXiv
		axClient := arxiv.New(arxiv.Config{
			BaseURL:    axCfg.BaseURL,
			Timeout:    axCfg.Timeout,
			RateLimit:  axCfg.RateLimit,
			MaxResults: axCfg.MaxResults,
			Enabled:    true,
		})
		registry.Register(axClient)
		logger.Info().Msg("registered paper source: arXiv")
	}

	// Scopus (only if API key is provided).
	if cfg.PaperSources.Scopus.Enabled && cfg.PaperSources.Scopus.APIKey != "" {
		scCfg := cfg.PaperSources.Scopus
		scClient := scopus.New(scopus.Config{
			BaseURL:    scCfg.BaseURL,
			APIKey:     scCfg.APIKey,
			Timeout:    scCfg.Timeout,
			RateLimit:  scCfg.RateLimit,
			MaxResults: scCfg.MaxResults,
			Enabled:    true,
		})
		registry.Register(scClient)
		logger.Info().Msg("registered paper source: Scopus")
	}

	// bioRxiv (via Europe PMC).
	if cfg.PaperSources.BioRxiv.Enabled {
		brCfg := cfg.PaperSources.BioRxiv
		brClient := biorxiv.New(biorxiv.Config{
			BaseURL:    brCfg.BaseURL,
			Server:     "bioRxiv",
			SourceType: domain.SourceTypeBioRxiv,
			Timeout:    brCfg.Timeout,
			RateLimit:  brCfg.RateLimit,
			MaxResults: brCfg.MaxResults,
			Enabled:    true,
		})
		registry.Register(brClient)
		logger.Info().Msg("registered paper source: bioRxiv")
	}

	// medRxiv (via Europe PMC).
	if cfg.PaperSources.MedRxiv.Enabled {
		mrCfg := cfg.PaperSources.MedRxiv
		mrClient := biorxiv.New(biorxiv.Config{
			BaseURL:    mrCfg.BaseURL,
			Server:     "medRxiv",
			SourceType: domain.SourceTypeMedRxiv,
			Timeout:    mrCfg.Timeout,
			RateLimit:  mrCfg.RateLimit,
			MaxResults: mrCfg.MaxResults,
			Enabled:    true,
		})
		registry.Register(mrClient)
		logger.Info().Msg("registered paper source: medRxiv")
	}
}
