// Package observability provides logging, metrics, and tracing support for
// the literature review service.
//
// # Overview
//
// The observability package provides:
//
//   - Structured logging with zerolog
//   - Prometheus metrics for reviews, searches, and sources
//   - Context helpers for propagating observability data
//
// # Logging
//
// Create a logger from configuration:
//
//	cfg := observability.LoggingConfig{
//	    Level:     "info",
//	    Format:    "json",
//	    Output:    "stdout",
//	    AddSource: true,
//	}
//
//	logger := observability.NewLogger(cfg)
//	logger.Info().Str("request_id", reqID).Msg("review started")
//
// Add review context to logger:
//
//	logger = observability.WithReviewContext(logger, requestID, orgID, projectID)
//
// # Metrics
//
// Initialize metrics:
//
//	metrics := observability.NewMetrics("literature_review")
//	metrics.RegisterAll()
//
// Record metrics:
//
//	metrics.ReviewsStarted.Inc()
//	metrics.SearchesBySource.WithLabelValues("semantic_scholar").Inc()
//	metrics.PapersDiscovered.Add(42)
//
// # Context Helpers
//
// Store and retrieve request context:
//
//	ctx = observability.WithRequestID(ctx, requestID)
//	ctx = observability.WithOrgProject(ctx, orgID, projectID)
//
//	reqID := observability.RequestIDFromContext(ctx)
//	orgID, projectID := observability.OrgProjectFromContext(ctx)
//
// # Standard Fields
//
// Common fields used across the service:
//
//   - request_id: Literature review request identifier
//   - org_id: Organization identifier
//   - project_id: Project identifier
//   - query: User's research query
//   - source: Paper source (semantic_scholar, openalex, etc.)
//   - keyword: Search keyword
//   - paper_id: Paper identifier
//   - trace_id: Distributed trace identifier
//
// # Thread Safety
//
// All components are safe for concurrent use from multiple goroutines.
package observability
