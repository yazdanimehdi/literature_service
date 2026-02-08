package workflows

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
)

// newTestInput returns a ReviewWorkflowInput configured for tests.
func newTestInput() ReviewWorkflowInput {
	return ReviewWorkflowInput{
		RequestID: uuid.New(),
		OrgID:     "org-1",
		ProjectID: "proj-1",
		UserID:    "user-1",
		Title:        "CRISPR gene editing therapeutic applications",
		Description:  "",
		SeedKeywords: nil,
		Config: domain.ReviewConfiguration{
			MaxPapers:           50,
			MaxExpansionDepth:   0,
			MaxKeywordsPerRound: 5,
			Sources:             []domain.SourceType{domain.SourceTypeSemanticScholar},
			IncludePreprints:    true,
			RequireOpenAccess:   false,
			MinCitations:        0,
		},
	}
}

func TestLiteratureReviewWorkflow_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	// Activity nil-pointer references matching the workflow pattern.
	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus - accept any input.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - return test keywords.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{"CRISPR", "gene therapy"},
			Reasoning: "test",
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

	// Mock SearchSingleSource - return papers (new concurrent activity).
	paperID := uuid.New()
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		&activities.SearchSingleSourceOutput{
			Source: domain.SourceTypeSemanticScholar,
			Papers: []*domain.Paper{
				{
					ID:          paperID,
					CanonicalID: "doi:10.1234/test",
					Title:       "Test Paper",
					Abstract:    "Test abstract about CRISPR",
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
			PaperIDs:       []uuid.UUID{paperID},
		}, nil,
	)

	// Register PaperProcessingWorkflow for child workflow execution.
	env.RegisterWorkflow(PaperProcessingWorkflow)

	// Mock activities for child workflow.
	var embeddingAct *activities.EmbeddingActivities
	var dedupAct *activities.DedupActivities
	var ingestionAct *activities.IngestionActivities

	env.OnActivity(statusAct.FetchPaperBatch, mock.Anything, mock.Anything).Return(
		&activities.FetchPaperBatchOutput{
			Papers: []activities.PaperForProcessing{
				{PaperID: paperID, CanonicalID: "doi:10.1234/test", Title: "Test Paper", Abstract: "Test abstract about CRISPR", PDFURL: "https://example.com/paper.pdf"},
			},
		}, nil,
	)

	env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).Return(
		&activities.EmbedPapersOutput{
			Embeddings: map[string][]float32{"doi:10.1234/test": make([]float32, 768)},
		}, nil,
	)

	env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).Return(
		&activities.BatchDedupOutput{
			NonDuplicateIDs: []uuid.UUID{paperID},
			DuplicateCount:  0,
		}, nil,
	)

	env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).Return(
		&activities.DownloadAndIngestOutput{
			Successful: 1,
			Skipped:    0,
			Failed:     0,
			Results: []activities.PaperIngestionResult{
				{
					PaperID:        paperID,
					FileID:         "file-123",
					IngestionRunID: "run-456",
					Status:         "completed",
				},
			},
		}, nil,
	)

	env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).Return(
		&activities.UpdatePaperIngestionResultsOutput{
			Updated: 1,
		}, nil,
	)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, input.RequestID, result.RequestID)
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	assert.Equal(t, 2, result.KeywordsFound)
	// With concurrent search: 2 keywords x 1 source = 2 searches, each returns 1 paper = 2 total
	assert.GreaterOrEqual(t, result.PapersFound, 1)
	assert.Equal(t, 0, result.ExpansionRounds)
	assert.GreaterOrEqual(t, result.Duration, 0.0)

	env.AssertExpectations(t)
}

func TestLiteratureReviewWorkflow_ExtractionFailure(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	var llmAct *activities.LLMActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus - succeed for all calls.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - return error.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		nil, assert.AnError,
	)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())

	err := env.GetWorkflowError()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extracting_keywords")
}

func TestLiteratureReviewWorkflow_WithExpansion(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()
	input.Config.MaxExpansionDepth = 1

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - returns keywords for both query and abstract modes.
	// The workflow calls this first for query extraction, then for each expansion paper.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{"CRISPR", "gene therapy"},
			Reasoning: "test extraction",
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

	// Mock SearchSingleSource - return papers with abstracts for expansion.
	paperID := uuid.New()
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		&activities.SearchSingleSourceOutput{
			Source: domain.SourceTypeSemanticScholar,
			Papers: []*domain.Paper{
				{
					ID:          paperID,
					CanonicalID: "doi:10.1234/test1",
					Title:       "CRISPR Paper 1",
					Abstract:    "This paper discusses CRISPR-Cas9 nuclease systems.",
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
			PaperIDs:       []uuid.UUID{paperID},
		}, nil,
	)

	// Register PaperProcessingWorkflow for child workflow execution.
	env.RegisterWorkflow(PaperProcessingWorkflow)

	// Mock activities for child workflow.
	var embeddingAct *activities.EmbeddingActivities
	var dedupAct *activities.DedupActivities
	var ingestionAct *activities.IngestionActivities

	env.OnActivity(statusAct.FetchPaperBatch, mock.Anything, mock.Anything).Return(
		&activities.FetchPaperBatchOutput{
			Papers: []activities.PaperForProcessing{
				{PaperID: paperID, CanonicalID: "doi:10.1234/test1", Title: "CRISPR Paper 1", Abstract: "This paper discusses CRISPR-Cas9 nuclease systems."},
			},
		}, nil,
	)

	env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).Return(
		&activities.EmbedPapersOutput{
			Embeddings: map[string][]float32{"doi:10.1234/test1": make([]float32, 768)},
		}, nil,
	)

	env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).Return(
		&activities.BatchDedupOutput{
			NonDuplicateIDs: []uuid.UUID{paperID},
			DuplicateCount:  0,
		}, nil,
	)

	env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).Return(
		&activities.DownloadAndIngestOutput{
			Successful: 0,
			Skipped:    0,
			Failed:     0,
		}, nil,
	)

	env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).Return(
		&activities.UpdatePaperIngestionResultsOutput{
			Updated: 0,
		}, nil,
	)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	// Initial: 2 keywords ("CRISPR", "gene therapy") + Expansion: 2 more = 4 total.
	assert.GreaterOrEqual(t, result.KeywordsFound, 2)
	assert.Equal(t, 1, result.ExpansionRounds)
}

func TestLiteratureReviewWorkflow_ProgressQuery(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{"CRISPR"},
			Reasoning: "test",
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

	// Mock SearchSingleSource - return empty results.
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		&activities.SearchSingleSourceOutput{
			Source:     domain.SourceTypeSemanticScholar,
			Papers:     []*domain.Paper{},
			TotalFound: 0,
		}, nil,
	)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Query progress after completion - query handlers remain registered.
	encoded, err := env.QueryWorkflow(QueryProgress)
	require.NoError(t, err)

	var queryProgress workflowProgress
	require.NoError(t, encoded.Get(&queryProgress))

	// The progress query should reflect the final state.
	assert.Equal(t, string(domain.ReviewStatusCompleted), queryProgress.Status)
	assert.Equal(t, "completed", queryProgress.Phase)
	assert.Equal(t, 0, queryProgress.MaxExpansionDepth)
	assert.Equal(t, 1, queryProgress.KeywordsFound)
}

func TestLiteratureReviewWorkflow_Cancellation(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	var llmAct *activities.LLMActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus - succeed for all calls.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - simulate cancellation by returning a cancellation error.
	// When the cancel signal fires during an activity, Temporal wraps it as a CanceledError.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		nil, temporal.NewCanceledError("workflow cancelled"),
	)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())

	err := env.GetWorkflowError()
	require.Error(t, err)
	// The error should indicate the workflow failed during keyword extraction.
	assert.Contains(t, err.Error(), "extracting_keywords")
}

func TestLiteratureReviewWorkflow_EmptySearchResults(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus - accept any input.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - return a single keyword.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{"CRISPR"},
			Reasoning: "test",
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

	// Mock SearchSingleSource - return empty results (no papers found).
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		&activities.SearchSingleSourceOutput{
			Source:     domain.SourceTypeSemanticScholar,
			Papers:     []*domain.Paper{},
			TotalFound: 0,
		}, nil,
	)

	// SavePapers is NOT mocked because the workflow skips it when allPapers is empty.

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, input.RequestID, result.RequestID)
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	assert.Equal(t, 1, result.KeywordsFound)
	assert.Equal(t, 0, result.PapersFound)
	assert.Equal(t, 0, result.PapersIngested)
	assert.Equal(t, 0, result.ExpansionRounds)

	env.AssertExpectations(t)
}

func TestLiteratureReviewWorkflow_LLMReturnsEmptyKeywords(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	var llmAct *activities.LLMActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus - accept any input.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - return empty keywords (LLM found nothing relevant).
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{},
			Reasoning: "no keywords found",
			Model:     "test-model",
		}, nil,
	)

	// Mock SaveKeywords - called with empty keyword list.
	env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).Return(
		&activities.SaveKeywordsOutput{
			KeywordIDs: []uuid.UUID{},
			NewCount:   0,
		}, nil,
	)

	// SearchSingleSource and SavePapers are NOT mocked because the search loop
	// iterates over extractOutput.Keywords which is empty, so no searches run.

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, input.RequestID, result.RequestID)
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	assert.Equal(t, 0, result.KeywordsFound)
	assert.Equal(t, 0, result.PapersFound)
	assert.Equal(t, 0, result.PapersIngested)
	assert.Equal(t, 0, result.ExpansionRounds)

	env.AssertExpectations(t)
}

func TestLiteratureReviewWorkflow_SearchActivityFails(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus - accept any input.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - return a single keyword.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{"CRISPR"},
			Reasoning: "test",
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

	// Mock SearchSingleSource - return an error (source failed).
	// Note: SearchSingleSource returns error in the output.Error field, not as an error.
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		&activities.SearchSingleSourceOutput{
			Source:     domain.SourceTypeSemanticScholar,
			Papers:     nil,
			TotalFound: 0,
			Error:      "source search failed",
		}, nil,
	)

	// SavePapers is NOT mocked because the workflow continues past search
	// failures and ends up with zero papers collected.

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	// The workflow does NOT fail when search returns an error because the
	// search loop logs a warning and continues.
	// With zero papers collected, SavePapers is skipped, and the workflow
	// completes successfully with PapersFound=0.
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, input.RequestID, result.RequestID)
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	assert.Equal(t, 1, result.KeywordsFound)
	assert.Equal(t, 0, result.PapersFound)
	assert.Equal(t, 0, result.PapersIngested)
	assert.Equal(t, 0, result.ExpansionRounds)

	env.AssertExpectations(t)
}

func TestLiteratureReviewWorkflow_BatchProcessing(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	// Activity nil-pointer references matching the workflow pattern.
	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus - accept any input.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - return a single keyword so search runs once.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{"CRISPR"},
			Reasoning: "test",
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

	// Two papers returned from search: one will be a duplicate, one will not.
	nonDupID := uuid.New()
	dupID := uuid.New()

	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		&activities.SearchSingleSourceOutput{
			Source: domain.SourceTypeSemanticScholar,
			Papers: []*domain.Paper{
				{
					ID:          nonDupID,
					CanonicalID: "doi:10.1234/original",
					Title:       "Original Paper",
					Abstract:    "Unique research about CRISPR",
					PDFURL:      "https://example.com/original.pdf",
				},
				{
					ID:          dupID,
					CanonicalID: "doi:10.1234/duplicate",
					Title:       "Duplicate Paper",
					Abstract:    "Near-duplicate research about CRISPR",
					PDFURL:      "https://example.com/duplicate.pdf",
				},
			},
			TotalFound: 2,
		}, nil,
	)

	// Mock SavePapers.
	env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).Return(
		&activities.SavePapersOutput{
			SavedCount:     2,
			DuplicateCount: 0,
			PaperIDs:       []uuid.UUID{nonDupID, dupID},
		}, nil,
	)

	// Register PaperProcessingWorkflow for child workflow execution.
	env.RegisterWorkflow(PaperProcessingWorkflow)

	// Mock activities for child workflow.
	var embeddingAct *activities.EmbeddingActivities
	var dedupAct *activities.DedupActivities
	var ingestionAct *activities.IngestionActivities

	env.OnActivity(statusAct.FetchPaperBatch, mock.Anything, mock.Anything).Return(
		&activities.FetchPaperBatchOutput{
			Papers: []activities.PaperForProcessing{
				{PaperID: nonDupID, CanonicalID: "doi:10.1234/original", Title: "Original Paper", Abstract: "Unique research about CRISPR", PDFURL: "https://example.com/original.pdf"},
				{PaperID: dupID, CanonicalID: "doi:10.1234/duplicate", Title: "Duplicate Paper", Abstract: "Near-duplicate research about CRISPR", PDFURL: "https://example.com/duplicate.pdf"},
			},
		}, nil,
	)

	env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).Return(
		&activities.EmbedPapersOutput{
			Embeddings: map[string][]float32{
				"doi:10.1234/original":  make([]float32, 768),
				"doi:10.1234/duplicate": make([]float32, 768),
			},
		}, nil,
	)

	// Mock BatchDedup - mark dupID as duplicate, only nonDupID is non-duplicate.
	env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).Return(
		&activities.BatchDedupOutput{
			NonDuplicateIDs: []uuid.UUID{nonDupID},
			DuplicateCount:  1,
		}, nil,
	)

	// Mock DownloadAndIngestPapers - expect only 1 paper (the non-duplicate).
	env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).Return(
		&activities.DownloadAndIngestOutput{
			Successful: 1,
			Skipped:    0,
			Failed:     0,
			Results: []activities.PaperIngestionResult{
				{
					PaperID:        nonDupID,
					FileID:         uuid.New().String(),
					IngestionRunID: "run-123",
					Status:         "RUN_STATUS_PENDING",
				},
			},
		}, nil,
	)

	// Mock UpdatePaperIngestionResults - called after successful ingestion.
	env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).Return(
		&activities.UpdatePaperIngestionResultsOutput{
			Updated: 1,
			Skipped: 0,
			Failed:  0,
		}, nil,
	)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, input.RequestID, result.RequestID)
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	assert.Equal(t, 1, result.KeywordsFound)
	assert.Equal(t, 2, result.PapersFound)
	// Ingested count comes from child workflow signals.
	assert.Equal(t, 1, result.PapersIngested)
	assert.Equal(t, 1, result.DuplicatesFound)
	assert.Equal(t, 0, result.ExpansionRounds)

	env.AssertExpectations(t)
}

func TestSelectPapersForExpansion(t *testing.T) {
	t.Run("returns papers with abstracts up to max", func(t *testing.T) {
		papers := []*domain.Paper{
			{Title: "Paper 1", Abstract: "Abstract 1"},
			{Title: "Paper 2", Abstract: ""},
			{Title: "Paper 3", Abstract: "Abstract 3"},
			{Title: "Paper 4", Abstract: "Abstract 4"},
		}

		selected := selectPapersForExpansion(papers, 2)
		assert.Len(t, selected, 2)
		assert.Equal(t, "Paper 1", selected[0].Title)
		assert.Equal(t, "Paper 3", selected[1].Title)
	})

	t.Run("returns empty for no abstracts", func(t *testing.T) {
		papers := []*domain.Paper{
			{Title: "Paper 1", Abstract: ""},
			{Title: "Paper 2", Abstract: ""},
		}

		selected := selectPapersForExpansion(papers, 5)
		assert.Empty(t, selected)
	})

	t.Run("handles nil papers in slice", func(t *testing.T) {
		papers := []*domain.Paper{
			nil,
			{Title: "Paper 1", Abstract: "Abstract 1"},
			nil,
		}

		selected := selectPapersForExpansion(papers, 5)
		assert.Len(t, selected, 1)
		assert.Equal(t, "Paper 1", selected[0].Title)
	})

	t.Run("handles empty slice", func(t *testing.T) {
		selected := selectPapersForExpansion(nil, 5)
		assert.Empty(t, selected)
	})
}

func TestCreateIDBatches(t *testing.T) {
	t.Run("creates batches of specified size", func(t *testing.T) {
		ids := make([]uuid.UUID, 12)
		for i := range ids {
			ids[i] = uuid.New()
		}

		batches := createIDBatches(ids, 5)
		assert.Len(t, batches, 3)
		assert.Len(t, batches[0], 5)
		assert.Len(t, batches[1], 5)
		assert.Len(t, batches[2], 2)
	})

	t.Run("handles empty input", func(t *testing.T) {
		batches := createIDBatches(nil, 5)
		assert.Nil(t, batches)

		batches = createIDBatches([]uuid.UUID{}, 5)
		assert.Nil(t, batches)
	})

	t.Run("handles zero or negative size", func(t *testing.T) {
		ids := []uuid.UUID{uuid.New()}
		assert.Nil(t, createIDBatches(ids, 0))
		assert.Nil(t, createIDBatches(ids, -1))
	})

	t.Run("handles single batch", func(t *testing.T) {
		ids := make([]uuid.UUID, 3)
		for i := range ids {
			ids[i] = uuid.New()
		}

		batches := createIDBatches(ids, 5)
		assert.Len(t, batches, 1)
		assert.Len(t, batches[0], 3)
	})

	t.Run("exact divisibility produces no empty trailing batch", func(t *testing.T) {
		ids := make([]uuid.UUID, 10)
		for i := range ids {
			ids[i] = uuid.New()
		}

		batches := createIDBatches(ids, 5)
		assert.Len(t, batches, 2)
		assert.Len(t, batches[0], 5)
		assert.Len(t, batches[1], 5)
	})
}

// ---------------------------------------------------------------------------
// Tests: concurrent stress
// ---------------------------------------------------------------------------

func TestLiteratureReviewWorkflow_ConcurrentStarts(t *testing.T) {
	const concurrency = 5
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			testSuite := &testsuite.WorkflowTestSuite{}
			env := testSuite.NewTestWorkflowEnvironment()

			input := newTestInput()
			input.RequestID = uuid.New()

			// Activity nil-pointer references matching the workflow pattern.
			var llmAct *activities.LLMActivities
			var searchAct *activities.SearchActivities
			var statusAct *activities.StatusActivities
			var eventAct *activities.EventActivities

			// Mock UpdateStatus - accept any input.
			env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

			// Mock PublishEvent - fire-and-forget.
			env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

			// Mock ExtractKeywords - return test keywords.
			env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
				&activities.ExtractKeywordsOutput{
					Keywords:  []string{"CRISPR", "gene therapy"},
					Reasoning: "test",
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

			// Mock SearchSingleSource - return some papers.
			paperID := uuid.New()
			env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
				&activities.SearchSingleSourceOutput{
					Source: domain.SourceTypeSemanticScholar,
					Papers: []*domain.Paper{
						{
							ID:          paperID,
							CanonicalID: "doi:10.1234/test",
							Title:       "Test Paper",
							Abstract:    "Test abstract about CRISPR",
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
					PaperIDs:       []uuid.UUID{paperID},
				}, nil,
			)

			// Register PaperProcessingWorkflow for child workflow execution.
			env.RegisterWorkflow(PaperProcessingWorkflow)

			// Mock activities for child workflow.
			var embeddingAct *activities.EmbeddingActivities
			var dedupAct *activities.DedupActivities
			var ingestionAct *activities.IngestionActivities

			env.OnActivity(statusAct.FetchPaperBatch, mock.Anything, mock.Anything).Return(
				&activities.FetchPaperBatchOutput{
					Papers: []activities.PaperForProcessing{
						{PaperID: paperID, CanonicalID: "doi:10.1234/test", Title: "Test Paper", Abstract: "Test abstract about CRISPR", PDFURL: "https://example.com/paper.pdf"},
					},
				}, nil,
			)

			env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).Return(
				&activities.EmbedPapersOutput{
					Embeddings: map[string][]float32{"doi:10.1234/test": make([]float32, 768)},
				}, nil,
			)

			env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).Return(
				&activities.BatchDedupOutput{
					NonDuplicateIDs: []uuid.UUID{paperID},
					DuplicateCount:  0,
				}, nil,
			)

			env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).Return(
				&activities.DownloadAndIngestOutput{
					Successful: 1,
					Skipped:    0,
					Failed:     0,
					Results: []activities.PaperIngestionResult{
						{
							PaperID:        paperID,
							FileID:         "file-123",
							IngestionRunID: "run-456",
							Status:         "completed",
						},
					},
				}, nil,
			)

			env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).Return(
				&activities.UpdatePaperIngestionResultsOutput{
					Updated: 1,
				}, nil,
			)

			env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

			if !env.IsWorkflowCompleted() {
				errs <- fmt.Errorf("workflow did not complete")
				return
			}
			errs <- env.GetWorkflowError()
		}()
	}

	for i := 0; i < concurrency; i++ {
		err := <-errs
		assert.NoError(t, err, "concurrent workflow %d failed", i)
	}
}

func TestLiteratureReviewWorkflow_RelevanceGate(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()
	input.Config.MaxExpansionDepth = 1
	input.Config.EnableRelevanceGate = true
	input.Config.RelevanceThreshold = 0.5

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities
	var embeddingAct *activities.EmbeddingActivities

	// Standard mocks.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).Return(
		&activities.SaveKeywordsOutput{KeywordIDs: []uuid.UUID{uuid.New()}, NewCount: 1}, nil)

	// Phase 1: Extract keywords.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.MatchedBy(func(in activities.ExtractKeywordsInput) bool {
		return in.Mode == "query"
	})).Return(&activities.ExtractKeywordsOutput{
		Keywords: []string{"CRISPR"}, Model: "test-model",
	}, nil).Once()

	// Phase 1: Embed query for relevance gate.
	env.OnActivity(embeddingAct.EmbedText, mock.Anything, mock.Anything).Return(
		&activities.EmbedTextOutput{Embedding: []float32{1.0, 0.0, 0.0}}, nil)

	// Phase 2: Search returns paper with abstract.
	paperID := uuid.New()
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		&activities.SearchSingleSourceOutput{
			Papers: []*domain.Paper{{
				ID: paperID, Title: "Test Paper", Abstract: "CRISPR mechanisms",
				CanonicalID: "doi:test",
			}},
			TotalFound: 1,
		}, nil)

	// Phase 3: Save papers.
	env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).Return(
		&activities.SavePapersOutput{SavedCount: 1, PaperIDs: []uuid.UUID{paperID}}, nil)

	// Register PaperProcessingWorkflow for child workflow execution.
	env.RegisterWorkflow(PaperProcessingWorkflow)

	// Mock activities for child workflow.
	var dedupAct *activities.DedupActivities
	var ingestionAct *activities.IngestionActivities

	env.OnActivity(statusAct.FetchPaperBatch, mock.Anything, mock.Anything).Return(
		&activities.FetchPaperBatchOutput{
			Papers: []activities.PaperForProcessing{
				{PaperID: paperID, CanonicalID: "doi:test", Title: "Test Paper", Abstract: "CRISPR mechanisms"},
			},
		}, nil)

	env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).Return(
		&activities.EmbedPapersOutput{
			Embeddings: map[string][]float32{"doi:test": make([]float32, 768)},
		}, nil)

	env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).Return(
		&activities.BatchDedupOutput{
			NonDuplicateIDs: []uuid.UUID{paperID},
			DuplicateCount:  0,
		}, nil)

	env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).Return(
		&activities.DownloadAndIngestOutput{
			Successful: 0, Skipped: 0, Failed: 0,
		}, nil)

	env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).Return(
		&activities.UpdatePaperIngestionResultsOutput{Updated: 0}, nil)

	// Phase 4: Extract expansion keywords.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.MatchedBy(func(in activities.ExtractKeywordsInput) bool {
		return in.Mode == "abstract"
	})).Return(&activities.ExtractKeywordsOutput{
		Keywords: []string{"relevant term", "off-topic term"}, Model: "test-model",
	}, nil)

	// Phase 4: Relevance gate — filter keywords.
	env.OnActivity(embeddingAct.ScoreKeywordRelevance, mock.Anything, mock.Anything).Return(
		&activities.ScoreKeywordRelevanceOutput{
			Accepted: []activities.ScoredKeyword{{Keyword: "relevant term", Score: 0.8}},
			Rejected: []activities.ScoredKeyword{{Keyword: "off-topic term", Score: 0.2}},
		}, nil)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, 1, result.KeywordsFilteredByRelevance)
}

func TestLiteratureReviewWorkflow_CoverageReview(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()
	input.Config.EnableCoverageReview = true
	input.Config.CoverageThreshold = 0.7

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).Return(
		&activities.SaveKeywordsOutput{KeywordIDs: []uuid.UUID{uuid.New()}, NewCount: 1}, nil)

	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{Keywords: []string{"CRISPR"}, Model: "test"}, nil)

	paperID := uuid.New()
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		&activities.SearchSingleSourceOutput{
			Papers:     []*domain.Paper{{ID: paperID, Title: "Paper", Abstract: "Abstract", CanonicalID: "doi:1"}},
			TotalFound: 1,
		}, nil)
	env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).Return(
		&activities.SavePapersOutput{SavedCount: 1, PaperIDs: []uuid.UUID{paperID}}, nil)

	// Register PaperProcessingWorkflow for child workflow execution.
	env.RegisterWorkflow(PaperProcessingWorkflow)

	// Mock activities for child workflow.
	var embeddingAct *activities.EmbeddingActivities
	var dedupAct *activities.DedupActivities
	var ingestionAct *activities.IngestionActivities

	env.OnActivity(statusAct.FetchPaperBatch, mock.Anything, mock.Anything).Return(
		&activities.FetchPaperBatchOutput{
			Papers: []activities.PaperForProcessing{
				{PaperID: paperID, CanonicalID: "doi:1", Title: "Paper", Abstract: "Abstract"},
			},
		}, nil)

	env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).Return(
		&activities.EmbedPapersOutput{
			Embeddings: map[string][]float32{"doi:1": make([]float32, 768)},
		}, nil)

	env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).Return(
		&activities.BatchDedupOutput{
			NonDuplicateIDs: []uuid.UUID{paperID},
			DuplicateCount:  0,
		}, nil)

	env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).Return(
		&activities.DownloadAndIngestOutput{
			Successful: 0, Skipped: 0, Failed: 0,
		}, nil)

	env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).Return(
		&activities.UpdatePaperIngestionResultsOutput{Updated: 0}, nil)

	// Coverage review returns high score -- no gap expansion.
	env.OnActivity(llmAct.AssessCoverage, mock.Anything, mock.Anything).Return(
		&activities.AssessCoverageOutput{
			CoverageScore: 0.85,
			Reasoning:     "Good coverage",
			IsSufficient:  true,
		}, nil)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.InDelta(t, 0.85, result.CoverageScore, 0.001)
	assert.Equal(t, "Good coverage", result.CoverageReasoning)
	assert.Empty(t, result.GapTopics)
}

func TestLiteratureReviewWorkflow_CoverageReview_GapExpansion(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()
	input.Config.EnableCoverageReview = true
	input.Config.CoverageThreshold = 0.7
	// MaxExpansionDepth=1 allows one expansion round. The expansion loop will
	// break early because the abstract-mode keyword extraction returns nothing,
	// leaving expansionRounds=0. This satisfies the gap expansion guard
	// (0 < 1) so the coverage gap round triggers.
	input.Config.MaxExpansionDepth = 1

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).Return(
		&activities.SaveKeywordsOutput{KeywordIDs: []uuid.UUID{uuid.New()}, NewCount: 1}, nil)

	// Phase 1: query-mode keyword extraction returns CRISPR.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.MatchedBy(func(in activities.ExtractKeywordsInput) bool {
		return in.Mode == "query"
	})).Return(
		&activities.ExtractKeywordsOutput{Keywords: []string{"CRISPR"}, Model: "test"}, nil).Once()

	// Phase 4: abstract-mode keyword extraction returns nothing so the
	// expansion loop breaks early (expansionRounds stays 0).
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.MatchedBy(func(in activities.ExtractKeywordsInput) bool {
		return in.Mode == "abstract"
	})).Return(
		&activities.ExtractKeywordsOutput{Keywords: []string{}, Model: "test"}, nil)

	paperID := uuid.New()
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).Return(
		&activities.SearchSingleSourceOutput{
			Papers:     []*domain.Paper{{ID: paperID, Title: "Paper", Abstract: "Abstract", CanonicalID: "doi:1"}},
			TotalFound: 1,
		}, nil)
	env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).Return(
		&activities.SavePapersOutput{SavedCount: 1, PaperIDs: []uuid.UUID{paperID}}, nil)

	// Register PaperProcessingWorkflow for child workflow execution.
	env.RegisterWorkflow(PaperProcessingWorkflow)

	// Mock activities for child workflow.
	var embeddingAct *activities.EmbeddingActivities
	var dedupAct *activities.DedupActivities
	var ingestionAct *activities.IngestionActivities

	env.OnActivity(statusAct.FetchPaperBatch, mock.Anything, mock.Anything).Return(
		&activities.FetchPaperBatchOutput{
			Papers: []activities.PaperForProcessing{
				{PaperID: paperID, CanonicalID: "doi:1", Title: "Paper", Abstract: "Abstract"},
			},
		}, nil)

	env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).Return(
		&activities.EmbedPapersOutput{
			Embeddings: map[string][]float32{"doi:1": make([]float32, 768)},
		}, nil)

	env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).Return(
		&activities.BatchDedupOutput{
			NonDuplicateIDs: []uuid.UUID{paperID},
			DuplicateCount:  0,
		}, nil)

	env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).Return(
		&activities.DownloadAndIngestOutput{
			Successful: 0, Skipped: 0, Failed: 0,
		}, nil)

	env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).Return(
		&activities.UpdatePaperIngestionResultsOutput{Updated: 0}, nil)

	// Coverage review returns low score with gap topics -- triggers gap expansion.
	env.OnActivity(llmAct.AssessCoverage, mock.Anything, mock.Anything).Return(
		&activities.AssessCoverageOutput{
			CoverageScore: 0.45,
			Reasoning:     "Missing delivery mechanisms",
			IsSufficient:  false,
			GapTopics:     []string{"lipid nanoparticle delivery"},
		}, nil)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.InDelta(t, 0.45, result.CoverageScore, 0.001)
	assert.Equal(t, "Missing delivery mechanisms", result.CoverageReasoning)
	assert.Equal(t, []string{"lipid nanoparticle delivery"}, result.GapTopics)
	// The expansion loop broke early (no new keywords), so expansionRounds was 0.
	// Gap expansion ran and incremented it to 1.
	assert.Equal(t, 1, result.ExpansionRounds)
	// Gap expansion added "lipid nanoparticle delivery" as a keyword, plus the
	// original "CRISPR".
	assert.Equal(t, 2, result.KeywordsFound)
}

func TestLiteratureReviewWorkflow_SearchDedup(t *testing.T) {
	// This test verifies incremental search deduplication:
	// When CheckSearchCompleted reports a cached search, the workflow:
	// 1. Uses cached papers from the previous search
	// 2. Launches a NEW forward search with DateFrom=searchedAt to find newer papers
	// 3. Records the new search result
	// This prevents the database from "freezing in time".
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()
	// Single source to simplify the test.
	input.Config.Sources = []domain.SourceType{domain.SourceTypeSemanticScholar}

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Phase 1: Extract keywords — returns two keywords.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords: []string{"CRISPR", "gene therapy"}, Model: "test-model",
		}, nil,
	)

	// SaveKeywords returns KeywordIDMap so dedup can look up keyword UUIDs.
	kwID1 := uuid.New()
	kwID2 := uuid.New()
	env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).Return(
		&activities.SaveKeywordsOutput{
			KeywordIDs: []uuid.UUID{kwID1, kwID2},
			KeywordIDMap: map[string]uuid.UUID{
				"CRISPR":       kwID1,
				"gene therapy": kwID2,
			},
			NewCount: 2,
		}, nil,
	)

	// CheckSearchCompleted — first keyword is cached, second is not.
	cachedSearchDate := "2026-02-05T10:00:00Z"
	cachedPaperID := uuid.New()
	env.OnActivity(statusAct.CheckSearchCompleted, mock.Anything, mock.MatchedBy(func(in activities.CheckSearchCompletedInput) bool {
		return in.KeywordID == kwID1
	})).Return(
		&activities.CheckSearchCompletedOutput{
			AlreadyCompleted: true,
			PreviouslyFoundPapers: []*domain.Paper{
				{
					ID:          cachedPaperID,
					CanonicalID: "doi:cached",
					Title:       "Cached Paper",
					Abstract:    "This was already found",
				},
			},
			PapersFoundCount: 1,
			SearchedAt:       cachedSearchDate,
		}, nil,
	)
	env.OnActivity(statusAct.CheckSearchCompleted, mock.Anything, mock.MatchedBy(func(in activities.CheckSearchCompletedInput) bool {
		return in.KeywordID == kwID2
	})).Return(
		&activities.CheckSearchCompletedOutput{AlreadyCompleted: false}, nil,
	)

	// SearchSingleSource — called for BOTH keywords:
	// - Keyword 1 (cached): forward search with DateFrom=cachedSearchDate
	// - Keyword 2 (not cached): full search without DateFrom
	forwardPaperID := uuid.New()
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.MatchedBy(func(in activities.SearchSingleSourceInput) bool {
		// Keyword 1 forward search: should have DateFrom set to cached search date.
		return in.DateFrom != nil && *in.DateFrom == cachedSearchDate
	})).Return(
		&activities.SearchSingleSourceOutput{
			Source: domain.SourceTypeSemanticScholar,
			Papers: []*domain.Paper{
				{
					ID:          forwardPaperID,
					CanonicalID: "doi:forward-new",
					Title:       "New Paper Since Cache",
					Abstract:    "Published after the cached search",
					PDFURL:      "https://example.com/forward.pdf",
				},
			},
			TotalFound: 1,
		}, nil,
	)

	newPaperID := uuid.New()
	env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.MatchedBy(func(in activities.SearchSingleSourceInput) bool {
		// Keyword 2 full search: should have no DateFrom.
		return in.DateFrom == nil
	})).Return(
		&activities.SearchSingleSourceOutput{
			Source: domain.SourceTypeSemanticScholar,
			Papers: []*domain.Paper{
				{
					ID:          newPaperID,
					CanonicalID: "doi:new",
					Title:       "New Paper",
					Abstract:    "Fresh result",
					PDFURL:      "https://example.com/new.pdf",
				},
			},
			TotalFound: 1,
		}, nil,
	)

	// RecordSearchResult — called for both searches.
	env.OnActivity(statusAct.RecordSearchResult, mock.Anything, mock.Anything).Return(nil)

	// SavePapers — 3 papers total: 1 cached + 1 forward + 1 new.
	env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).Return(
		&activities.SavePapersOutput{SavedCount: 3, DuplicateCount: 0, PaperIDs: []uuid.UUID{cachedPaperID, forwardPaperID, newPaperID}}, nil,
	)

	// BulkCreateKeywordPaperMappings — called after SavePapers when keyword IDs are mapped.
	env.OnActivity(statusAct.BulkCreateKeywordPaperMappings, mock.Anything, mock.Anything).Return(nil)

	// Child workflow mocks.
	env.RegisterWorkflow(PaperProcessingWorkflow)
	var embeddingAct *activities.EmbeddingActivities
	var dedupAct *activities.DedupActivities
	var ingestionAct *activities.IngestionActivities

	env.OnActivity(statusAct.FetchPaperBatch, mock.Anything, mock.Anything).Return(
		&activities.FetchPaperBatchOutput{
			Papers: []activities.PaperForProcessing{
				{PaperID: cachedPaperID, CanonicalID: "doi:cached", Title: "Cached Paper", Abstract: "This was already found"},
				{PaperID: forwardPaperID, CanonicalID: "doi:forward-new", Title: "New Paper Since Cache", Abstract: "Published after the cached search", PDFURL: "https://example.com/forward.pdf"},
				{PaperID: newPaperID, CanonicalID: "doi:new", Title: "New Paper", Abstract: "Fresh result", PDFURL: "https://example.com/new.pdf"},
			},
		}, nil)

	env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).Return(
		&activities.EmbedPapersOutput{Embeddings: map[string][]float32{"doi:new": make([]float32, 768)}}, nil)
	env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).Return(
		&activities.BatchDedupOutput{NonDuplicateIDs: []uuid.UUID{newPaperID}}, nil)
	env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).Return(
		&activities.DownloadAndIngestOutput{Successful: 0}, nil)
	env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).Return(
		&activities.UpdatePaperIngestionResultsOutput{}, nil)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	// Papers: 1 cached + 1 forward search + 1 new search = 3 total
	assert.Equal(t, 3, result.PapersFound)
	assert.Equal(t, 2, result.KeywordsFound)
}
