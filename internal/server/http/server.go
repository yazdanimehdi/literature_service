// Package httpserver provides the HTTP REST API server for the literature review service.
package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/helixir/literature-review-service/internal/database"
	"github.com/helixir/literature-review-service/internal/repository"
	servergrpc "github.com/helixir/literature-review-service/internal/server"
)

// WorkflowClient defines the interface for workflow operations used by the HTTP server.
// This is the same interface as the gRPC server uses.
type WorkflowClient = servergrpc.WorkflowClient

// Server is the HTTP REST API server.
type Server struct {
	router         chi.Router
	httpServer     *http.Server
	workflowClient WorkflowClient
	workflowFunc   interface{} // The Temporal workflow function reference.
	reviewRepo     repository.ReviewRepository
	paperRepo      repository.PaperRepository
	keywordRepo    repository.KeywordRepository
	db             *database.DB
	pool           *pgxpool.Pool
	logger         zerolog.Logger
	authMiddleware func(http.Handler) http.Handler
}

// Config holds HTTP server configuration.
type Config struct {
	Address         string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// NewServer creates a new HTTP server with all dependencies.
// workflowFunc is the Temporal workflow function reference (e.g., workflows.LiteratureReviewWorkflow)
// that will be passed to StartReviewWorkflow.
func NewServer(
	cfg Config,
	workflowClient WorkflowClient,
	workflowFunc interface{},
	reviewRepo repository.ReviewRepository,
	paperRepo repository.PaperRepository,
	keywordRepo repository.KeywordRepository,
	db *database.DB,
	logger zerolog.Logger,
	authMiddleware func(http.Handler) http.Handler,
) *Server {
	s := &Server{
		workflowClient: workflowClient,
		workflowFunc:   workflowFunc,
		reviewRepo:     reviewRepo,
		paperRepo:      paperRepo,
		keywordRepo:    keywordRepo,
		db:             db,
		pool:           db.Pool(),
		logger:         logger.With().Str("component", "http-server").Logger(),
		authMiddleware: authMiddleware,
	}

	s.router = s.buildRouter()

	s.httpServer = &http.Server{
		Addr:         cfg.Address,
		Handler:      s.router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return s
}

// buildRouter creates the chi router with all middleware and routes.
func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(correlationIDMiddleware)
	r.Use(jsonContentTypeMiddleware)

	// Health endpoints (no auth)
	r.Get("/healthz", s.healthHandler)
	r.Get("/readyz", s.readinessHandler)

	// API routes with auth + tenant context
	r.Route("/api/v1/orgs/{orgID}/projects/{projectID}", func(r chi.Router) {
		if s.authMiddleware != nil {
			r.Use(s.authMiddleware)
		}
		r.Use(tenantContextMiddleware)

		r.Post("/literature-reviews", s.startLiteratureReview)
		r.Get("/literature-reviews", s.listLiteratureReviews)
		r.Get("/literature-reviews/{reviewID}", s.getLiteratureReviewStatus)
		r.Delete("/literature-reviews/{reviewID}", s.cancelLiteratureReview)
		r.Get("/literature-reviews/{reviewID}/papers", s.getLiteratureReviewPapers)
		r.Get("/literature-reviews/{reviewID}/keywords", s.getLiteratureReviewKeywords)
		r.Get("/literature-reviews/{reviewID}/progress", s.streamProgress)
	})

	return r
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	s.logger.Info().Str("address", s.httpServer.Addr).Msg("HTTP server starting")
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen on HTTP address: %w", err)
	}
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// healthHandler returns basic liveness status.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	health := s.db.Health(r.Context())
	if health.Status == "healthy" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": health.Status})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{
		"status":   "unhealthy",
		"database": health.Status,
		"error":    health.Error,
	})
}

// readinessHandler returns readiness status including Temporal connectivity.
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	health := s.db.Health(r.Context())
	if health.Status != "healthy" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status":   "not_ready",
			"database": health.Status,
			"error":    health.Error,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "ready",
		"database": "healthy",
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Best-effort log; headers already sent.
		_ = err
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{
		"error": message,
	})
}
