package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics contains all Prometheus metrics for the literature review service.
// Metrics are organized by subsystem: reviews, keywords, searches, papers, sources,
// ingestion, and LLM operations. All counters and histograms are registered via promauto
// for automatic registration with the default Prometheus registry.
type Metrics struct {
	// ReviewsStarted counts the total number of literature reviews initiated.
	ReviewsStarted prometheus.Counter

	// ReviewsCompleted counts the total number of reviews that finished successfully.
	ReviewsCompleted prometheus.Counter

	// ReviewsFailed counts the total number of reviews that ended in failure.
	ReviewsFailed prometheus.Counter

	// ReviewsCancelled counts the total number of reviews cancelled by user or system.
	ReviewsCancelled prometheus.Counter

	// ReviewDuration observes the end-to-end duration of reviews in seconds.
	ReviewDuration prometheus.Histogram

	// KeywordsExtracted counts the total number of keywords extracted across all reviews.
	KeywordsExtracted prometheus.Counter

	// KeywordExtractions counts extraction operations by source label (e.g., "query", "paper").
	KeywordExtractions *prometheus.CounterVec

	// KeywordsPerReview observes the distribution of keyword counts per review.
	KeywordsPerReview prometheus.Histogram

	// SearchesStarted counts searches initiated, labeled by paper source.
	SearchesStarted *prometheus.CounterVec

	// SearchesCompleted counts successful searches, labeled by paper source.
	SearchesCompleted *prometheus.CounterVec

	// SearchesFailed counts failed searches, labeled by paper source.
	SearchesFailed *prometheus.CounterVec

	// SearchDuration observes search duration in seconds, labeled by paper source.
	SearchDuration *prometheus.HistogramVec

	// PapersPerSearch observes the distribution of papers returned per search, labeled by source.
	PapersPerSearch *prometheus.HistogramVec

	// PapersDiscovered counts the total number of unique papers discovered.
	PapersDiscovered prometheus.Counter

	// PapersIngested counts the total number of papers submitted for ingestion.
	PapersIngested prometheus.Counter

	// PapersSkipped counts the total number of papers skipped (e.g., already processed).
	PapersSkipped prometheus.Counter

	// PapersDuplicate counts the total number of duplicate papers detected during deduplication.
	PapersDuplicate prometheus.Counter

	// PapersBySource counts papers discovered, labeled by paper source.
	PapersBySource *prometheus.CounterVec

	// SourceRequestsTotal counts HTTP requests to paper source APIs, labeled by source and endpoint.
	SourceRequestsTotal *prometheus.CounterVec

	// SourceRequestsFailed counts failed HTTP requests to paper source APIs, labeled by source, endpoint, and error type.
	SourceRequestsFailed *prometheus.CounterVec

	// SourceRequestDuration observes HTTP request duration to paper source APIs in seconds.
	SourceRequestDuration *prometheus.HistogramVec

	// SourceRateLimited counts rate-limited responses from paper source APIs, labeled by source.
	SourceRateLimited *prometheus.CounterVec

	// IngestionRequestsStarted counts ingestion requests initiated.
	IngestionRequestsStarted prometheus.Counter

	// IngestionRequestsCompleted counts ingestion requests that completed successfully.
	IngestionRequestsCompleted prometheus.Counter

	// IngestionRequestsFailed counts ingestion requests that failed.
	IngestionRequestsFailed prometheus.Counter

	// LLMRequestsTotal counts LLM API requests, labeled by operation and model.
	LLMRequestsTotal *prometheus.CounterVec

	// LLMRequestsFailed counts failed LLM API requests, labeled by operation, model, and error type.
	LLMRequestsFailed *prometheus.CounterVec

	// LLMRequestDuration observes LLM API request duration in seconds, labeled by operation and model.
	LLMRequestDuration *prometheus.HistogramVec

	// LLMTokensUsed counts tokens consumed by LLM operations, labeled by operation, model, and token type.
	LLMTokensUsed *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance with all metrics initialized.
// The namespace is used as a prefix for all metric names.
func NewMetrics(namespace string) *Metrics {
	return &Metrics{
		// Reviews
		ReviewsStarted: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "reviews_started_total",
			Help:      "Total number of literature reviews started",
		}),
		ReviewsCompleted: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "reviews_completed_total",
			Help:      "Total number of literature reviews completed successfully",
		}),
		ReviewsFailed: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "reviews_failed_total",
			Help:      "Total number of literature reviews that failed",
		}),
		ReviewsCancelled: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "reviews_cancelled_total",
			Help:      "Total number of literature reviews cancelled",
		}),
		ReviewDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "review_duration_seconds",
			Help:      "Duration of literature reviews in seconds",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1200, 1800, 3600},
		}),

		// Keywords
		KeywordsExtracted: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "keywords_extracted_total",
			Help:      "Total number of keywords extracted",
		}),
		KeywordExtractions: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "keyword_extractions_total",
			Help:      "Total number of keyword extraction operations by source",
		}, []string{"source"}),
		KeywordsPerReview: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "keywords_per_review",
			Help:      "Number of keywords extracted per review",
			Buckets:   []float64{1, 2, 5, 10, 20, 50, 100},
		}),

		// Searches
		SearchesStarted: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "searches_started_total",
			Help:      "Total number of paper searches started by source",
		}, []string{"source"}),
		SearchesCompleted: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "searches_completed_total",
			Help:      "Total number of paper searches completed by source",
		}, []string{"source"}),
		SearchesFailed: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "searches_failed_total",
			Help:      "Total number of paper searches that failed by source",
		}, []string{"source"}),
		SearchDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "search_duration_seconds",
			Help:      "Duration of paper searches in seconds by source",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		}, []string{"source"}),
		PapersPerSearch: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "papers_per_search",
			Help:      "Number of papers returned per search by source",
			Buckets:   []float64{0, 1, 5, 10, 25, 50, 100, 200, 500},
		}, []string{"source"}),

		// Papers
		PapersDiscovered: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "papers_discovered_total",
			Help:      "Total number of papers discovered",
		}),
		PapersIngested: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "papers_ingested_total",
			Help:      "Total number of papers sent for ingestion",
		}),
		PapersSkipped: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "papers_skipped_total",
			Help:      "Total number of papers skipped (already processed)",
		}),
		PapersDuplicate: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "papers_duplicate_total",
			Help:      "Total number of duplicate papers found",
		}),
		PapersBySource: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "papers_by_source_total",
			Help:      "Total number of papers discovered by source",
		}, []string{"source"}),

		// Sources
		SourceRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "source_requests_total",
			Help:      "Total number of requests to paper sources",
		}, []string{"source", "endpoint"}),
		SourceRequestsFailed: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "source_requests_failed_total",
			Help:      "Total number of failed requests to paper sources",
		}, []string{"source", "endpoint", "error_type"}),
		SourceRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "source_request_duration_seconds",
			Help:      "Duration of requests to paper sources in seconds",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"source", "endpoint"}),
		SourceRateLimited: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "source_rate_limited_total",
			Help:      "Total number of rate limit responses from paper sources",
		}, []string{"source"}),

		// Ingestion
		IngestionRequestsStarted: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ingestion_requests_started_total",
			Help:      "Total number of ingestion requests started",
		}),
		IngestionRequestsCompleted: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ingestion_requests_completed_total",
			Help:      "Total number of ingestion requests completed",
		}),
		IngestionRequestsFailed: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ingestion_requests_failed_total",
			Help:      "Total number of ingestion requests failed",
		}),

		// LLM
		LLMRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "llm_requests_total",
			Help:      "Total number of LLM requests by operation",
		}, []string{"operation", "model"}),
		LLMRequestsFailed: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "llm_requests_failed_total",
			Help:      "Total number of failed LLM requests by operation",
		}, []string{"operation", "model", "error_type"}),
		LLMRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "llm_request_duration_seconds",
			Help:      "Duration of LLM requests in seconds",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		}, []string{"operation", "model"}),
		LLMTokensUsed: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "llm_tokens_used_total",
			Help:      "Total number of tokens used by LLM operations",
		}, []string{"operation", "model", "token_type"}),
	}
}

// RecordReviewStarted records that a review has started.
func (m *Metrics) RecordReviewStarted() {
	m.ReviewsStarted.Inc()
}

// RecordReviewCompleted records that a review has completed.
func (m *Metrics) RecordReviewCompleted(durationSeconds float64) {
	m.ReviewsCompleted.Inc()
	m.ReviewDuration.Observe(durationSeconds)
}

// RecordReviewFailed records that a review has failed.
func (m *Metrics) RecordReviewFailed(durationSeconds float64) {
	m.ReviewsFailed.Inc()
	m.ReviewDuration.Observe(durationSeconds)
}

// RecordReviewCancelled records that a review has been cancelled.
func (m *Metrics) RecordReviewCancelled() {
	m.ReviewsCancelled.Inc()
}

// RecordKeywordsExtracted records keyword extraction results.
func (m *Metrics) RecordKeywordsExtracted(source string, count int) {
	m.KeywordsExtracted.Add(float64(count))
	m.KeywordExtractions.WithLabelValues(source).Inc()
	m.KeywordsPerReview.Observe(float64(count))
}

// RecordSearchStarted records that a search has started.
func (m *Metrics) RecordSearchStarted(source string) {
	m.SearchesStarted.WithLabelValues(source).Inc()
}

// RecordSearchCompleted records that a search has completed.
func (m *Metrics) RecordSearchCompleted(source string, paperCount int, durationSeconds float64) {
	m.SearchesCompleted.WithLabelValues(source).Inc()
	m.SearchDuration.WithLabelValues(source).Observe(durationSeconds)
	m.PapersPerSearch.WithLabelValues(source).Observe(float64(paperCount))
}

// RecordSearchFailed records that a search has failed.
func (m *Metrics) RecordSearchFailed(source string, durationSeconds float64) {
	m.SearchesFailed.WithLabelValues(source).Inc()
	m.SearchDuration.WithLabelValues(source).Observe(durationSeconds)
}

// RecordPapersDiscovered records papers discovered from a source.
func (m *Metrics) RecordPapersDiscovered(source string, count int) {
	m.PapersDiscovered.Add(float64(count))
	m.PapersBySource.WithLabelValues(source).Add(float64(count))
}

// RecordPaperIngested records a paper sent for ingestion.
func (m *Metrics) RecordPaperIngested() {
	m.PapersIngested.Inc()
}

// RecordPaperSkipped records a paper that was skipped.
func (m *Metrics) RecordPaperSkipped() {
	m.PapersSkipped.Inc()
}

// RecordPaperDuplicate records a duplicate paper.
func (m *Metrics) RecordPaperDuplicate() {
	m.PapersDuplicate.Inc()
}

// RecordPaperDuplicates records multiple duplicate papers in a single call,
// avoiding the overhead of incrementing the counter one at a time.
func (m *Metrics) RecordPaperDuplicates(count int) {
	m.PapersDuplicate.Add(float64(count))
}

// RecordSourceRequest records a request to a paper source.
func (m *Metrics) RecordSourceRequest(source, endpoint string, durationSeconds float64) {
	m.SourceRequestsTotal.WithLabelValues(source, endpoint).Inc()
	m.SourceRequestDuration.WithLabelValues(source, endpoint).Observe(durationSeconds)
}

// RecordSourceRequestFailed records a failed request to a paper source.
func (m *Metrics) RecordSourceRequestFailed(source, endpoint, errorType string) {
	m.SourceRequestsFailed.WithLabelValues(source, endpoint, errorType).Inc()
}

// RecordSourceRateLimited records a rate limit response from a source.
func (m *Metrics) RecordSourceRateLimited(source string) {
	m.SourceRateLimited.WithLabelValues(source).Inc()
}

// RecordIngestionStarted records that an ingestion request has started.
func (m *Metrics) RecordIngestionStarted() {
	m.IngestionRequestsStarted.Inc()
}

// RecordIngestionCompleted records that an ingestion request has completed.
func (m *Metrics) RecordIngestionCompleted() {
	m.IngestionRequestsCompleted.Inc()
}

// RecordIngestionFailed records that an ingestion request has failed.
func (m *Metrics) RecordIngestionFailed() {
	m.IngestionRequestsFailed.Inc()
}

// RecordLLMRequest records an LLM request.
func (m *Metrics) RecordLLMRequest(operation, model string, durationSeconds float64, inputTokens, outputTokens int) {
	m.LLMRequestsTotal.WithLabelValues(operation, model).Inc()
	m.LLMRequestDuration.WithLabelValues(operation, model).Observe(durationSeconds)
	m.LLMTokensUsed.WithLabelValues(operation, model, "input").Add(float64(inputTokens))
	m.LLMTokensUsed.WithLabelValues(operation, model, "output").Add(float64(outputTokens))
}

// RecordLLMRequestFailed records a failed LLM request.
func (m *Metrics) RecordLLMRequestFailed(operation, model, errorType string) {
	m.LLMRequestsFailed.WithLabelValues(operation, model, errorType).Inc()
}
