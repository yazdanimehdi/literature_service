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
	"github.com/helixir/literature-review-service/internal/ingestion"
	"github.com/helixir/literature-review-service/internal/llm"
	"github.com/helixir/literature-review-service/internal/observability"
	"github.com/helixir/literature-review-service/internal/outbox"
	"github.com/helixir/literature-review-service/internal/papersources"
	"github.com/helixir/literature-review-service/internal/papersources/openalex"
	"github.com/helixir/literature-review-service/internal/papersources/pubmed"
	"github.com/helixir/literature-review-service/internal/papersources/semanticscholar"
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

	// Create LLM keyword extractor.
	extractor, err := llm.NewKeywordExtractor(llm.FactoryConfig{
		Provider:    cfg.LLM.Provider,
		Temperature: cfg.LLM.Temperature,
		Timeout:     cfg.LLM.Timeout,
		MaxRetries:  cfg.LLM.MaxRetries,
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
	})
	if err != nil {
		return fmt.Errorf("create ingestion client: %w", err)
	}
	defer ingestionClient.Close()
	logger.Info().Str("address", cfg.IngestionService.Address).Msg("ingestion service client created")

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
		Logger:    newTemporalLogger(logger),
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
	llmActivities := activities.NewLLMActivities(extractor, metrics)
	searchActivities := activities.NewSearchActivities(registry, metrics)
	statusActivities := activities.NewStatusActivities(reviewRepo, keywordRepo, paperRepo, metrics)
	ingestionActivities := activities.NewIngestionActivities(ingestionClient, metrics)
	eventActivities := activities.NewEventActivities(publisher)

	manager.RegisterActivity(llmActivities)
	manager.RegisterActivity(searchActivities)
	manager.RegisterActivity(statusActivities)
	manager.RegisterActivity(ingestionActivities)
	manager.RegisterActivity(eventActivities)

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

// temporalLogger adapts zerolog to Temporal's log interface.
type temporalLogger struct {
	logger zerolog.Logger
}

func newTemporalLogger(logger zerolog.Logger) *temporalLogger {
	return &temporalLogger{logger: logger.With().Str("component", "temporal-sdk").Logger()}
}

func (l *temporalLogger) Debug(msg string, keyvals ...interface{}) {
	l.logger.Debug().Fields(keyvalToMap(keyvals)).Msg(msg)
}

func (l *temporalLogger) Info(msg string, keyvals ...interface{}) {
	l.logger.Info().Fields(keyvalToMap(keyvals)).Msg(msg)
}

func (l *temporalLogger) Warn(msg string, keyvals ...interface{}) {
	l.logger.Warn().Fields(keyvalToMap(keyvals)).Msg(msg)
}

func (l *temporalLogger) Error(msg string, keyvals ...interface{}) {
	l.logger.Error().Fields(keyvalToMap(keyvals)).Msg(msg)
}

// keyvalToMap converts key-value pairs to a map for zerolog fields.
func keyvalToMap(keyvals []interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(keyvals)/2)
	for i := 0; i+1 < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", keyvals[i])
		}
		m[key] = keyvals[i+1]
	}
	return m
}
