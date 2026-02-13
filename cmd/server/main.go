// Package main provides the entry point for the literature review service gRPC server.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/helixir/grpcauth/chiauth"
	"github.com/helixir/grpcauth/env"
	"github.com/helixir/grpcauth/interceptor"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
	"github.com/helixir/literature-review-service/internal/config"
	"github.com/helixir/literature-review-service/internal/database"
	"github.com/helixir/literature-review-service/internal/observability"
	"github.com/helixir/literature-review-service/internal/repository"
	"github.com/helixir/literature-review-service/internal/server"
	httpserver "github.com/helixir/literature-review-service/internal/server/http"
	"github.com/helixir/literature-review-service/internal/temporal"
	"github.com/helixir/literature-review-service/internal/temporal/workflows"
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
	logger = logger.With().Str("component", "server").Logger()
	logger.Info().Msg("literature-review-service server starting")

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

	// Run migrations if configured.
	if cfg.Database.MigrationAutoRun {
		migrator, err := database.NewMigrator(db, cfg.Database.MigrationPath, logger)
		if err != nil {
			return fmt.Errorf("create migrator: %w", err)
		}
		defer func() {
			if closeErr := migrator.Close(); closeErr != nil {
				logger.Error().Err(closeErr).Msg("failed to close migrator")
			}
		}()

		if err := migrator.Up(); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
	}

	// Create repositories.
	reviewRepo := repository.NewPgReviewRepository(db)
	paperRepo := repository.NewPgPaperRepository(db)
	keywordRepo := repository.NewPgKeywordRepository(db)

	// Create Temporal client.
	temporalClient, err := client.Dial(client.Options{
		HostPort:  cfg.Temporal.HostPort,
		Namespace: cfg.Temporal.Namespace,
		Logger:    observability.NewTemporalLogger(logger),
	})
	if err != nil {
		return fmt.Errorf("connect to temporal: %w", err)
	}
	logger.Info().
		Str("host_port", cfg.Temporal.HostPort).
		Str("namespace", cfg.Temporal.Namespace).
		Msg("temporal client connected")

	// Configure authentication using shared grpcauth package.
	authConfig, err := env.FromEnvironment()
	if err != nil {
		return fmt.Errorf("load auth config: %w", err)
	}
	authConfig.SkipMethods = []string{
		"/grpc.health.v1.Health/Check",
		"/grpc.health.v1.Health/Watch",
		"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
	}

	serverInterceptor, err := interceptor.NewServerInterceptor(authConfig)
	if err != nil {
		return fmt.Errorf("create auth interceptor: %w", err)
	}
	defer serverInterceptor.Close()
	logger.Info().Msg("grpcauth interceptor initialized")

	// Create the ReviewWorkflowClient for the gRPC server.
	workflowClient := temporal.NewReviewWorkflowClient(temporalClient, cfg.Temporal.TaskQueue)

	// Create the gRPC LiteratureReviewServer.
	litServer := server.NewLiteratureReviewServer(
		workflowClient,
		workflows.LiteratureReviewWorkflow,
		reviewRepo,
		paperRepo,
		keywordRepo,
	)

	// Create gRPC server with keepalive, size limits, and auth interceptors.
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(16*1024*1024), // 16MB
		grpc.MaxSendMsgSize(16*1024*1024), // 16MB
		grpc.MaxConcurrentStreams(100),
		grpc.ChainUnaryInterceptor(
			serverInterceptor.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			serverInterceptor.StreamServerInterceptor(),
		),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Minute,
			MaxConnectionAge:      30 * time.Minute,
			MaxConnectionAgeGrace: 5 * time.Minute,
			Time:                  5 * time.Minute,
			Timeout:               1 * time.Minute,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Minute,
			PermitWithoutStream: true,
		}),
	)

	// Register services.
	pb.RegisterLiteratureReviewServiceServer(grpcServer, litServer)

	// Register gRPC health check.
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus(
		"literaturereview.v1.LiteratureReviewService",
		healthpb.HealthCheckResponse_SERVING,
	)

	// Enable reflection for debugging.
	reflection.Register(grpcServer)

	// Start gRPC listener.
	grpcAddr := cfg.Server.GRPCAddress()
	grpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("listen on gRPC port: %w", err)
	}

	// Create HTTP REST API server with chiauth middleware.
	chiMiddleware, err := chiauth.NewMiddleware(authConfig)
	if err != nil {
		return fmt.Errorf("create HTTP auth middleware: %w", err)
	}
	defer chiMiddleware.Close()
	httpAuthMiddleware := chiMiddleware.Handler()

	httpCfg := httpserver.Config{
		Address:         cfg.Server.HTTPAddress(),
		ReadTimeout:     cfg.Server.ReadTimeout,
		WriteTimeout:    5 * time.Minute, // Long timeout for SSE streaming.
		IdleTimeout:     2 * time.Minute,
		ShutdownTimeout: cfg.Server.ShutdownTimeout,
	}

	httpSrv := httpserver.NewServer(
		httpCfg,
		workflowClient,
		workflows.LiteratureReviewWorkflow,
		reviewRepo,
		paperRepo,
		keywordRepo,
		db,
		logger,
		httpAuthMiddleware,
	)

	// Set up Prometheus metrics handler on a separate port if configured.
	var metricsServer *http.Server
	if cfg.Metrics.Enabled {
		metricsMux := http.NewServeMux()
		metricsMux.Handle(cfg.Metrics.Path, promhttp.Handler())
		metricsServer = &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.MetricsPort),
			Handler:      metricsMux,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
		}
	}

	// Channel to collect server errors.
	errCh := make(chan error, 3)

	// Start gRPC server in background.
	go func() {
		logger.Info().
			Str("address", grpcAddr).
			Msg("gRPC server starting")
		if err := grpcServer.Serve(grpcListener); err != nil {
			errCh <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	// Start HTTP REST API server in background.
	go func() {
		logger.Info().
			Str("address", httpCfg.Address).
			Msg("HTTP REST API server starting")
		if err := httpSrv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// Start metrics server if configured.
	if metricsServer != nil {
		go func() {
			logger.Info().
				Str("address", metricsServer.Addr).
				Msg("metrics server starting")
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("metrics server error: %w", err)
			}
		}()
	}

	readyLog := logger.Info().
		Str("grpc_address", grpcAddr).
		Str("http_address", httpCfg.Address)
	if metricsServer != nil {
		readyLog = readyLog.Str("metrics_address", metricsServer.Addr)
	}
	readyLog.Msg("literature-review-service is ready")

	// Wait for shutdown signal or server error.
	select {
	case <-ctx.Done():
		logger.Info().Msg("received shutdown signal")
	case err := <-errCh:
		logger.Error().Err(err).Msg("server error")
		return err
	}

	// Graceful shutdown.
	logger.Info().Msg("shutting down literature-review-service")

	// Mark health as not serving.
	healthServer.SetServingStatus(
		"literaturereview.v1.LiteratureReviewService",
		healthpb.HealthCheckResponse_NOT_SERVING,
	)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// Shut down HTTP REST API server with timeout.
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("HTTP server shutdown error")
	}

	// Shut down metrics server if running.
	if metricsServer != nil {
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("metrics server shutdown error")
		}
	}

	// Gracefully stop gRPC server with timeout.
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		logger.Info().Msg("gRPC server stopped gracefully")
	case <-shutdownCtx.Done():
		logger.Warn().Msg("gRPC server forced shutdown due to timeout")
		grpcServer.Stop()
	}

	// Close workflow client.
	workflowClient.Close()

	logger.Info().Msg("literature-review-service shutdown complete")
	return nil
}
