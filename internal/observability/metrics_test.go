package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: prometheus/promauto registers metrics globally, so we need to use
// unique namespaces per test to avoid registration conflicts.

func TestNewMetrics(t *testing.T) {
	// Use unique namespace to avoid conflicts with other tests
	m := NewMetrics("test_literature_review_new")

	assert.NotNil(t, m.ReviewsStarted)
	assert.NotNil(t, m.ReviewsCompleted)
	assert.NotNil(t, m.ReviewsFailed)
	assert.NotNil(t, m.ReviewsCancelled)
	assert.NotNil(t, m.ReviewDuration)
	assert.NotNil(t, m.KeywordsExtracted)
	assert.NotNil(t, m.KeywordExtractions)
	assert.NotNil(t, m.SearchesStarted)
	assert.NotNil(t, m.SearchesCompleted)
	assert.NotNil(t, m.SearchesFailed)
	assert.NotNil(t, m.PapersDiscovered)
	assert.NotNil(t, m.PapersBySource)
	assert.NotNil(t, m.SourceRequestsTotal)
	assert.NotNil(t, m.SourceRequestsFailed)
	assert.NotNil(t, m.LLMRequestsTotal)
	assert.NotNil(t, m.LLMTokensUsed)
}

func TestRecordReviewStarted(t *testing.T) {
	m := NewMetrics("test_review_started")

	initial := testutil.ToFloat64(m.ReviewsStarted)
	m.RecordReviewStarted()
	assert.Equal(t, initial+1, testutil.ToFloat64(m.ReviewsStarted))
}

func TestRecordReviewCompleted(t *testing.T) {
	m := NewMetrics("test_review_completed")

	initial := testutil.ToFloat64(m.ReviewsCompleted)
	m.RecordReviewCompleted(5.5)
	assert.Equal(t, initial+1, testutil.ToFloat64(m.ReviewsCompleted))

	// Check histogram
	histCount, err := getHistogramSampleCount(m.ReviewDuration)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), histCount)
}

func TestRecordReviewFailed(t *testing.T) {
	m := NewMetrics("test_review_failed")

	initial := testutil.ToFloat64(m.ReviewsFailed)
	m.RecordReviewFailed(3.0)
	assert.Equal(t, initial+1, testutil.ToFloat64(m.ReviewsFailed))
}

func TestRecordReviewCancelled(t *testing.T) {
	m := NewMetrics("test_review_cancelled")

	initial := testutil.ToFloat64(m.ReviewsCancelled)
	m.RecordReviewCancelled()
	assert.Equal(t, initial+1, testutil.ToFloat64(m.ReviewsCancelled))
}

func TestRecordKeywordsExtracted(t *testing.T) {
	m := NewMetrics("test_keywords_extracted")

	initial := testutil.ToFloat64(m.KeywordsExtracted)
	m.RecordKeywordsExtracted("query", 5)
	assert.Equal(t, initial+5, testutil.ToFloat64(m.KeywordsExtracted))
	assert.Equal(t, float64(1), testutil.ToFloat64(m.KeywordExtractions.WithLabelValues("query")))
}

func TestRecordSearchStarted(t *testing.T) {
	m := NewMetrics("test_search_started")

	m.RecordSearchStarted("semantic_scholar")
	assert.Equal(t, float64(1), testutil.ToFloat64(m.SearchesStarted.WithLabelValues("semantic_scholar")))
}

func TestRecordSearchCompleted(t *testing.T) {
	m := NewMetrics("test_search_completed")

	m.RecordSearchCompleted("openalex", 42, 2.5)
	assert.Equal(t, float64(1), testutil.ToFloat64(m.SearchesCompleted.WithLabelValues("openalex")))
}

func TestRecordSearchFailed(t *testing.T) {
	m := NewMetrics("test_search_failed")

	m.RecordSearchFailed("pubmed", 1.0)
	assert.Equal(t, float64(1), testutil.ToFloat64(m.SearchesFailed.WithLabelValues("pubmed")))
}

func TestRecordPapersDiscovered(t *testing.T) {
	m := NewMetrics("test_papers_discovered")

	initial := testutil.ToFloat64(m.PapersDiscovered)
	m.RecordPapersDiscovered("semantic_scholar", 25)
	assert.Equal(t, initial+25, testutil.ToFloat64(m.PapersDiscovered))
	assert.Equal(t, float64(25), testutil.ToFloat64(m.PapersBySource.WithLabelValues("semantic_scholar")))
}

func TestRecordPaperIngested(t *testing.T) {
	m := NewMetrics("test_paper_ingested")

	initial := testutil.ToFloat64(m.PapersIngested)
	m.RecordPaperIngested()
	assert.Equal(t, initial+1, testutil.ToFloat64(m.PapersIngested))
}

func TestRecordPaperSkipped(t *testing.T) {
	m := NewMetrics("test_paper_skipped")

	initial := testutil.ToFloat64(m.PapersSkipped)
	m.RecordPaperSkipped()
	assert.Equal(t, initial+1, testutil.ToFloat64(m.PapersSkipped))
}

func TestRecordPaperDuplicate(t *testing.T) {
	m := NewMetrics("test_paper_duplicate")

	initial := testutil.ToFloat64(m.PapersDuplicate)
	m.RecordPaperDuplicate()
	assert.Equal(t, initial+1, testutil.ToFloat64(m.PapersDuplicate))
}

func TestRecordSourceRequest(t *testing.T) {
	m := NewMetrics("test_source_request")

	m.RecordSourceRequest("semantic_scholar", "search", 0.5)
	assert.Equal(t, float64(1), testutil.ToFloat64(m.SourceRequestsTotal.WithLabelValues("semantic_scholar", "search")))
}

func TestRecordSourceRequestFailed(t *testing.T) {
	m := NewMetrics("test_source_request_failed")

	m.RecordSourceRequestFailed("openalex", "search", "timeout")
	assert.Equal(t, float64(1), testutil.ToFloat64(m.SourceRequestsFailed.WithLabelValues("openalex", "search", "timeout")))
}

func TestRecordSourceRateLimited(t *testing.T) {
	m := NewMetrics("test_source_rate_limited")

	m.RecordSourceRateLimited("pubmed")
	assert.Equal(t, float64(1), testutil.ToFloat64(m.SourceRateLimited.WithLabelValues("pubmed")))
}

func TestRecordIngestionStarted(t *testing.T) {
	m := NewMetrics("test_ingestion_started")

	initial := testutil.ToFloat64(m.IngestionRequestsStarted)
	m.RecordIngestionStarted()
	assert.Equal(t, initial+1, testutil.ToFloat64(m.IngestionRequestsStarted))
}

func TestRecordIngestionCompleted(t *testing.T) {
	m := NewMetrics("test_ingestion_completed")

	initial := testutil.ToFloat64(m.IngestionRequestsCompleted)
	m.RecordIngestionCompleted()
	assert.Equal(t, initial+1, testutil.ToFloat64(m.IngestionRequestsCompleted))
}

func TestRecordIngestionFailed(t *testing.T) {
	m := NewMetrics("test_ingestion_failed")

	initial := testutil.ToFloat64(m.IngestionRequestsFailed)
	m.RecordIngestionFailed()
	assert.Equal(t, initial+1, testutil.ToFloat64(m.IngestionRequestsFailed))
}

func TestRecordLLMRequest(t *testing.T) {
	m := NewMetrics("test_llm_request")

	m.RecordLLMRequest("keyword_extraction", "gpt-4", 2.5, 100, 50)
	assert.Equal(t, float64(1), testutil.ToFloat64(m.LLMRequestsTotal.WithLabelValues("keyword_extraction", "gpt-4")))
	assert.Equal(t, float64(100), testutil.ToFloat64(m.LLMTokensUsed.WithLabelValues("keyword_extraction", "gpt-4", "input")))
	assert.Equal(t, float64(50), testutil.ToFloat64(m.LLMTokensUsed.WithLabelValues("keyword_extraction", "gpt-4", "output")))
}

func TestRecordLLMRequestFailed(t *testing.T) {
	m := NewMetrics("test_llm_request_failed")

	m.RecordLLMRequestFailed("keyword_extraction", "gpt-4", "rate_limit")
	assert.Equal(t, float64(1), testutil.ToFloat64(m.LLMRequestsFailed.WithLabelValues("keyword_extraction", "gpt-4", "rate_limit")))
}

// Helper to get histogram sample count
func getHistogramSampleCount(h prometheus.Histogram) (uint64, error) {
	ch := make(chan prometheus.Metric, 1)
	h.Collect(ch)
	close(ch)

	var m prometheus.Metric
	for m = range ch {
		break
	}

	var dto = &dto.Metric{}
	if err := m.Write(dto); err != nil {
		return 0, err
	}

	return dto.Histogram.GetSampleCount(), nil
}
