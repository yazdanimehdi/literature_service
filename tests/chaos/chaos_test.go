// Package chaos provides fault injection tests for the LiteratureReviewWorkflow.
//
// These tests verify that the workflow handles various failure scenarios
// correctly, including transient LLM failures, search source unavailability,
// ingestion failures, and status update failures. All tests use the Temporal
// test environment with mocked activities (no external services required).
package chaos

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
	"github.com/helixir/literature-review-service/internal/temporal/workflows"
)

// newChaosInput returns a ReviewWorkflowInput configured for chaos tests.
func newChaosInput() workflows.ReviewWorkflowInput {
	return workflows.ReviewWorkflowInput{
		RequestID: uuid.New(),
		OrgID:     "org-chaos",
		ProjectID: "proj-chaos",
		UserID:    "user-chaos",
		Title:     "chaos test query",
		Config: domain.ReviewConfiguration{
			MaxPapers:           10,
			MaxExpansionDepth:   0,
			MaxKeywordsPerRound: 3,
			Sources:             []domain.SourceType{domain.SourceTypeSemanticScholar},
		},
	}
}

// TestChaos_LLMFailsThenRecovers verifies that the workflow completes
// successfully when the LLM keyword extraction activity fails on the first
// two invocations with retryable errors, then succeeds on the third.
//
// The Temporal test environment collapses retry policies: each OnActivity call
// represents the final outcome after all retries are exhausted. To simulate
// transient failures followed by success, we use a closure-based mock with
// an atomic counter. The first two calls return a retryable ApplicationError;
// the third call returns valid keywords.
func TestChaos_LLMFailsThenRecovers(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register workflows for child workflow support.
	env.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
	env.RegisterWorkflow(workflows.PaperProcessingWorkflow)

	input := newChaosInput()

	// Activity nil-pointer references for method-based registration.
	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities
	var embeddingAct *activities.EmbeddingActivities
	var dedupAct *activities.DedupActivities
	var ingestionAct *activities.IngestionActivities

	// Mock UpdateStatus -- accept all calls.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent -- fire-and-forget, always succeed.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords with a closure that tracks call count.
	// First 2 calls: return retryable application error.
	// 3rd call: return valid keywords.
	var llmCallCount int32
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ activities.ExtractKeywordsInput) (*activities.ExtractKeywordsOutput, error) {
			n := atomic.AddInt32(&llmCallCount, 1)
			if n <= 2 {
				return nil, temporal.NewApplicationError(
					"LLM service temporarily unavailable",
					"LLM_TRANSIENT",
				)
			}
			return &activities.ExtractKeywordsOutput{
				Keywords:  []string{"chaos", "resilience"},
				Reasoning: "recovered after transient failures",
				Model:     "test-model",
			}, nil
		},
	)

	// Mock SaveKeywords.
	env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).Return(
		&activities.SaveKeywordsOutput{
			KeywordIDs: []uuid.UUID{uuid.New(), uuid.New()},
			NewCount:   2,
		}, nil,
	)

	// Mock SearchSingleSource -- return a paper for each keyword.
	paperID := uuid.New()
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		&activities.SearchSingleSourceOutput{
			Source: domain.SourceTypeSemanticScholar,
			Papers: []*domain.Paper{
				{
					ID:          paperID,
					CanonicalID: "doi:10.9999/chaos",
					Title:       "Chaos Resilience Paper",
					Abstract:    "Testing fault tolerance in distributed systems.",
				},
			},
			TotalFound: 1,
		}, nil,
	)

	// Mock SavePapers.
	env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).Return(
		&activities.SavePapersOutput{
			SavedCount:     1,
			DuplicateCount: 0,
		}, nil,
	)

	// Mock child workflow activities: EmbedPapers, BatchDedup, DownloadAndIngestPapers.
	env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).Return(
		&activities.EmbedPapersOutput{
			Embeddings: map[string][]float32{
				"doi:10.9999/chaos": make([]float32, 768),
			},
		}, nil,
	)

	env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).Return(
		&activities.BatchDedupOutput{
			NonDuplicateIDs: []uuid.UUID{paperID},
			DuplicateCount:  0,
		}, nil,
	)

	// Papers don't have PDF URLs, so ingestion will be skipped.
	// But mock it anyway in case the workflow calls it.
	env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).Return(
		&activities.DownloadAndIngestOutput{
			Successful: 0,
			Failed:     0,
			Skipped:    1,
			Results:    nil,
		}, nil,
	).Maybe()

	env.ExecuteWorkflow(workflows.LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result workflows.ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, input.RequestID, result.RequestID)
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	assert.Equal(t, 2, result.KeywordsFound, "should have 2 keywords from the recovered LLM call")
	assert.GreaterOrEqual(t, result.PapersFound, 1, "should have found papers after recovery")

	env.AssertExpectations(t)
}

// TestChaos_SearchAllSourcesFail verifies workflow behavior when every
// SearchPapers call returns a non-retryable error.
//
// The workflow iterates over each extracted keyword and calls SearchPapers.
// When search fails for a keyword, the workflow logs a warning and continues
// to the next keyword (review_workflow.go lines 343-344). As a result, even
// when ALL searches fail, the workflow completes successfully with zero
// papers found. The search loop is resilient by design.
func TestChaos_SearchAllSourcesFail(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newChaosInput()

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus -- accept all calls.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent -- fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords -- succeed with keywords.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{"chaos", "failure"},
			Reasoning: "test keywords for chaos scenario",
			Model:     "test-model",
		}, nil,
	)

	// Mock SaveKeywords.
	env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).Return(
		&activities.SaveKeywordsOutput{
			KeywordIDs: []uuid.UUID{uuid.New(), uuid.New()},
			NewCount:   2,
		}, nil,
	)

	// Mock SearchSingleSource -- return non-retryable error for all calls.
	// This simulates all paper sources being unavailable.
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		nil, temporal.NewNonRetryableApplicationError(
			"all sources unavailable",
			"SEARCH_FAILED",
			nil, // cause
		),
	)

	// SavePapers is NOT mocked because the workflow skips it when no papers
	// are collected (allPapers is empty).

	// Register workflows for child workflow support.
	env.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
	env.RegisterWorkflow(workflows.PaperProcessingWorkflow)

	env.ExecuteWorkflow(workflows.LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())

	// The workflow completes successfully even when all searches fail.
	// Search failures are non-fatal: the loop logs a warning and continues
	// for each keyword. With zero papers, SavePapers is skipped and the
	// workflow proceeds to the completion phase.
	require.NoError(t, env.GetWorkflowError())

	var result workflows.ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, input.RequestID, result.RequestID)
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	assert.Equal(t, 2, result.KeywordsFound, "keywords were extracted before search failures")
	assert.Equal(t, 0, result.PapersFound, "no papers should be found when all searches fail")
	assert.Equal(t, 0, result.PapersIngested, "no papers to ingest")

	env.AssertExpectations(t)
}

// TestChaos_IngestionNonFatal verifies that the workflow completes
// successfully when DownloadAndIngestPapers fails entirely.
//
// The ingestion phase is explicitly non-fatal: the child workflow catches the error,
// logs a warning, and proceeds to completion. This test confirms that paper discovery
// results are preserved even when PDF download/ingestion fails.
func TestChaos_IngestionNonFatal(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register workflows for child workflow support.
	env.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
	env.RegisterWorkflow(workflows.PaperProcessingWorkflow)

	input := newChaosInput()

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var ingestionAct *activities.IngestionActivities
	var eventAct *activities.EventActivities
	var dedupAct *activities.DedupActivities
	var embeddingAct *activities.EmbeddingActivities

	// Mock UpdateStatus -- accept all calls.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent -- fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords -- succeed.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{"ingestion-chaos"},
			Reasoning: "testing ingestion failure path",
			Model:     "test-model",
		}, nil,
	)

	// Mock SaveKeywords.
	env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).Return(
		&activities.SaveKeywordsOutput{
			KeywordIDs: []uuid.UUID{uuid.New()},
			NewCount:   1,
		}, nil,
	)

	// Mock SearchSingleSource -- return papers WITH PDF URLs to trigger ingestion.
	paperID := uuid.New()
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		&activities.SearchSingleSourceOutput{
			Source: domain.SourceTypeSemanticScholar,
			Papers: []*domain.Paper{
				{
					ID:          paperID,
					CanonicalID: "doi:10.9999/ingestion-chaos",
					Title:       "Paper With PDF",
					Abstract:    "This paper has a PDF URL for ingestion testing.",
					PDFURL:      "https://example.com/paper.pdf",
				},
			},
			TotalFound: 1,
		}, nil,
	)

	// Mock SavePapers.
	env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).Return(
		&activities.SavePapersOutput{
			SavedCount:     1,
			DuplicateCount: 0,
		}, nil,
	)

	// Mock EmbedPapers -- required by child workflow.
	env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).Return(
		&activities.EmbedPapersOutput{
			Embeddings: map[string][]float32{
				"doi:10.9999/ingestion-chaos": make([]float32, 768),
			},
		}, nil,
	)

	// Mock BatchDedup -- all papers are non-duplicates.
	env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).Return(
		&activities.BatchDedupOutput{
			NonDuplicateIDs: []uuid.UUID{paperID},
			DuplicateCount:  0,
		}, nil,
	)

	// Mock DownloadAndIngestPapers -- return a non-retryable error.
	// This simulates the ingestion service being completely unavailable.
	env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).Return(
		nil, temporal.NewNonRetryableApplicationError(
			"ingestion service unavailable",
			"INGESTION_FAILED",
			nil, // cause
		),
	)

	env.ExecuteWorkflow(workflows.LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())

	// Ingestion failure is non-fatal: the workflow should still complete.
	require.NoError(t, env.GetWorkflowError())

	var result workflows.ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, input.RequestID, result.RequestID)
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	assert.Equal(t, 1, result.KeywordsFound)
	assert.Equal(t, 1, result.PapersFound)
	// When ingestion fails completely, PapersIngested is 0 because no papers
	// were actually ingested. PapersFailed should be 1.
	assert.Equal(t, 0, result.PapersIngested, "no papers ingested due to ingestion failure")
	assert.Equal(t, 1, result.PapersFailed, "paper failed due to ingestion error")

	env.AssertExpectations(t)
}

// TestChaos_StatusUpdateFailsRepeatedly verifies that the workflow fails
// when the initial UpdateStatus call returns a non-retryable error.
//
// Status updates are on the critical path: when updateStatus fails, the
// workflow calls handleFailure which wraps the error and returns it as the
// workflow error (review_workflow.go lines 256-258). The handleFailure
// function also attempts a fire-and-forget status update to "failed" and
// publishes a "review.failed" event, both of which may also fail without
// affecting the returned error.
func TestChaos_StatusUpdateFailsRepeatedly(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newChaosInput()

	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus -- return non-retryable error for all calls.
	// This means the very first updateStatus (to "extracting_keywords") will fail.
	// The handleFailure path also calls UpdateStatus (to set "failed"), which
	// will also fail, but that error is ignored.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(
		temporal.NewNonRetryableApplicationError(
			"database connection lost",
			"DB_UNAVAILABLE",
			nil, // cause
		),
	)

	// Mock PublishEvent -- the handleFailure path publishes "review.failed".
	// Allow it to succeed (fire-and-forget).
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	env.ExecuteWorkflow(workflows.LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())

	err := env.GetWorkflowError()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update status to extracting_keywords",
		"error should indicate which status transition failed")
}
