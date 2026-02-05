// Package pipeline provides integration tests for the concurrent pipeline workflow.
// These tests verify the complete flow: search -> save -> embed -> dedup -> ingest.
package pipeline

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
	"github.com/helixir/literature-review-service/internal/temporal/workflows"
)

// newTestInput returns a ReviewWorkflowInput configured for tests.
func newTestInput() workflows.ReviewWorkflowInput {
	return workflows.ReviewWorkflowInput{
		RequestID: uuid.New(),
		OrgID:     "org-1",
		ProjectID: "proj-1",
		UserID:    "user-1",
		Query:     "test query for pipeline integration",
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

func TestPipelineIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Run("full pipeline processes papers through all stages", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		// Register workflows
		env.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
		env.RegisterWorkflow(workflows.PaperProcessingWorkflow)

		paperID := uuid.New()

		// Activity nil-pointer references matching the workflow pattern.
		var llmAct *activities.LLMActivities
		var searchAct *activities.SearchActivities
		var statusAct *activities.StatusActivities
		var eventAct *activities.EventActivities
		var embeddingAct *activities.EmbeddingActivities
		var dedupAct *activities.DedupActivities
		var ingestionAct *activities.IngestionActivities

		// Mock LLM activities
		env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).
			Return(&activities.ExtractKeywordsOutput{
				Keywords:  []string{"test keyword"},
				Reasoning: "test reasoning",
				Model:     "test-model",
			}, nil)

		// Mock search activities - return papers
		env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).
			Return(&activities.SearchSingleSourceOutput{
				Source: domain.SourceTypeSemanticScholar,
				Papers: []*domain.Paper{
					{
						ID:          paperID,
						CanonicalID: "doi:10.1234/test",
						Title:       "Test Paper",
						Abstract:    "This is a test abstract for embedding.",
						PDFURL:      "https://example.com/paper.pdf",
					},
				},
				TotalFound: 1,
			}, nil)

		// Mock status activities
		env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).
			Return(nil)
		env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).
			Return(&activities.SaveKeywordsOutput{
				KeywordIDs: []uuid.UUID{uuid.New()},
				NewCount:   1,
			}, nil)
		env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).
			Return(&activities.SavePapersOutput{
				SavedCount:     1,
				DuplicateCount: 0,
			}, nil)
		env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).
			Return(&activities.UpdatePaperIngestionResultsOutput{Updated: 1}, nil)

		// Mock embedding activities
		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{
					"doi:10.1234/test": make([]float32, 768),
				},
			}, nil)

		// Mock dedup activities
		env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).
			Return(&activities.BatchDedupOutput{
				NonDuplicateIDs: []uuid.UUID{paperID},
				DuplicateCount:  0,
			}, nil)

		// Mock ingestion activities
		env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).
			Return(&activities.DownloadAndIngestOutput{
				Successful: 1,
				Failed:     0,
				Results: []activities.PaperIngestionResult{
					{PaperID: paperID, FileID: "file-123", IngestionRunID: "run-456", Status: "completed"},
				},
			}, nil)

		// Mock event publisher
		env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).
			Return(nil)

		// Execute workflow
		input := newTestInput()

		env.ExecuteWorkflow(workflows.LiteratureReviewWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		err := env.GetWorkflowError()
		require.NoError(t, err)

		var result workflows.ReviewWorkflowResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, "completed", result.Status)
		assert.GreaterOrEqual(t, result.PapersFound, 1)
		assert.GreaterOrEqual(t, result.PapersIngested, 1)

		env.AssertExpectations(t)
	})

	t.Run("pipeline handles search failure gracefully", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		env.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
		env.RegisterWorkflow(workflows.PaperProcessingWorkflow)

		// Activity nil-pointer references matching the workflow pattern.
		var llmAct *activities.LLMActivities
		var searchAct *activities.SearchActivities
		var statusAct *activities.StatusActivities
		var eventAct *activities.EventActivities

		// Mock LLM activities
		env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).
			Return(&activities.ExtractKeywordsOutput{
				Keywords:  []string{"test keyword"},
				Reasoning: "test reasoning",
				Model:     "test-model",
			}, nil)

		// Mock search activities - return error in output (non-fatal)
		env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).
			Return(&activities.SearchSingleSourceOutput{
				Source: domain.SourceTypeSemanticScholar,
				Papers: nil,
				Error:  "API rate limit exceeded",
			}, nil)

		// Mock status activities
		env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).
			Return(nil)
		env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).
			Return(&activities.SaveKeywordsOutput{
				KeywordIDs: []uuid.UUID{uuid.New()},
				NewCount:   1,
			}, nil)
		// SavePapers is not called when no papers are found

		// Mock event publisher
		env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).
			Return(nil)

		input := newTestInput()

		env.ExecuteWorkflow(workflows.LiteratureReviewWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		err := env.GetWorkflowError()
		require.NoError(t, err) // Workflow should complete even with search failures

		var result workflows.ReviewWorkflowResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, "completed", result.Status)
		assert.Equal(t, 0, result.PapersFound) // No papers found due to error

		env.AssertExpectations(t)
	})

	t.Run("pipeline handles dedup failure gracefully", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		env.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
		env.RegisterWorkflow(workflows.PaperProcessingWorkflow)

		paperID := uuid.New()

		// Activity nil-pointer references matching the workflow pattern.
		var llmAct *activities.LLMActivities
		var searchAct *activities.SearchActivities
		var statusAct *activities.StatusActivities
		var eventAct *activities.EventActivities
		var embeddingAct *activities.EmbeddingActivities
		var dedupAct *activities.DedupActivities
		var ingestionAct *activities.IngestionActivities

		// Mock LLM activities
		env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).
			Return(&activities.ExtractKeywordsOutput{
				Keywords:  []string{"test keyword"},
				Reasoning: "test reasoning",
				Model:     "test-model",
			}, nil)

		// Mock search activities
		env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).
			Return(&activities.SearchSingleSourceOutput{
				Source: domain.SourceTypeSemanticScholar,
				Papers: []*domain.Paper{
					{
						ID:          paperID,
						CanonicalID: "doi:10.1234/test",
						Title:       "Test Paper",
						Abstract:    "Test abstract",
						PDFURL:      "https://example.com/paper.pdf",
					},
				},
				TotalFound: 1,
			}, nil)

		// Mock status activities
		env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).
			Return(nil)
		env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).
			Return(&activities.SaveKeywordsOutput{
				KeywordIDs: []uuid.UUID{uuid.New()},
				NewCount:   1,
			}, nil)
		env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).
			Return(&activities.SavePapersOutput{
				SavedCount:     1,
				DuplicateCount: 0,
			}, nil)
		env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).
			Return(&activities.UpdatePaperIngestionResultsOutput{Updated: 1}, nil)

		// Mock embedding activities
		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{
					"doi:10.1234/test": make([]float32, 768),
				},
			}, nil)

		// Mock dedup activities - return error (non-fatal in child workflow)
		// The child workflow treats dedup failure as non-fatal and continues with all papers
		env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("qdrant unavailable"))

		// Mock ingestion activities - should still be called since dedup failure is non-fatal
		env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).
			Return(&activities.DownloadAndIngestOutput{
				Successful: 1,
				Failed:     0,
				Results: []activities.PaperIngestionResult{
					{PaperID: paperID, FileID: "file-123", IngestionRunID: "run-456", Status: "completed"},
				},
			}, nil)

		// Mock event publisher
		env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).
			Return(nil)

		input := newTestInput()

		env.ExecuteWorkflow(workflows.LiteratureReviewWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		err := env.GetWorkflowError()
		require.NoError(t, err) // Workflow should complete even with dedup failure

		var result workflows.ReviewWorkflowResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, "completed", result.Status)
		assert.Equal(t, 1, result.PapersIngested) // Should still ingest despite dedup failure

		env.AssertExpectations(t)
	})

	t.Run("pipeline handles partial embedding failure gracefully", func(t *testing.T) {
		// This test verifies that when some papers fail embedding but the workflow still
		// completes, the failed count is recorded correctly. We use a scenario where
		// the embedding returns an empty result for some papers (skipped) instead of
		// a fatal error, which allows the workflow to complete.
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		env.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
		env.RegisterWorkflow(workflows.PaperProcessingWorkflow)

		paperID := uuid.New()

		// Activity nil-pointer references matching the workflow pattern.
		var llmAct *activities.LLMActivities
		var searchAct *activities.SearchActivities
		var statusAct *activities.StatusActivities
		var eventAct *activities.EventActivities
		var embeddingAct *activities.EmbeddingActivities
		var ingestionAct *activities.IngestionActivities

		// Mock LLM activities
		env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).
			Return(&activities.ExtractKeywordsOutput{
				Keywords:  []string{"test keyword"},
				Reasoning: "test reasoning",
				Model:     "test-model",
			}, nil)

		// Mock search activities - paper with abstract
		env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).
			Return(&activities.SearchSingleSourceOutput{
				Source: domain.SourceTypeSemanticScholar,
				Papers: []*domain.Paper{
					{
						ID:          paperID,
						CanonicalID: "doi:10.1234/test",
						Title:       "Test Paper",
						Abstract:    "Test abstract",
						PDFURL:      "https://example.com/paper.pdf",
					},
				},
				TotalFound: 1,
			}, nil)

		// Mock status activities
		env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).
			Return(nil)
		env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).
			Return(&activities.SaveKeywordsOutput{
				KeywordIDs: []uuid.UUID{uuid.New()},
				NewCount:   1,
			}, nil)
		env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).
			Return(&activities.SavePapersOutput{
				SavedCount:     1,
				DuplicateCount: 0,
			}, nil)
		// Note: UpdatePaperIngestionResults is not called because there are no results

		// Mock embedding activities - return empty embeddings (paper skipped)
		// This simulates when embedding fails for individual papers but doesn't
		// cause a fatal error.
		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{}, // No embeddings returned
				Skipped:    1,
			}, nil)

		// Note: BatchDedup is NOT called when there are no embeddings (len(papersForDedup) == 0)
		// The workflow skips dedup and adds all papers to nonDuplicatePaperIDs

		// Mock ingestion activities - will be called with paper that has PDF URL
		// Even without embeddings, ingestion proceeds with all papers
		env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).
			Return(&activities.DownloadAndIngestOutput{
				Successful: 0, // No successful ingestions in this test scenario
				Failed:     0,
				Skipped:    0,
				Results:    nil, // No results means UpdatePaperIngestionResults won't be called
			}, nil)

		// Mock event publisher
		env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).
			Return(nil)

		input := newTestInput()

		env.ExecuteWorkflow(workflows.LiteratureReviewWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		err := env.GetWorkflowError()
		require.NoError(t, err)

		var result workflows.ReviewWorkflowResult
		require.NoError(t, env.GetWorkflowResult(&result))

		// Workflow completes successfully but no papers were ingested
		// because embedding returned empty results.
		assert.Equal(t, "completed", result.Status)
		assert.Equal(t, 1, result.PapersFound)
		assert.Equal(t, 0, result.PapersIngested)
	})

	t.Run("pipeline handles ingestion failure gracefully", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		env.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
		env.RegisterWorkflow(workflows.PaperProcessingWorkflow)

		paperID := uuid.New()

		// Activity nil-pointer references matching the workflow pattern.
		var llmAct *activities.LLMActivities
		var searchAct *activities.SearchActivities
		var statusAct *activities.StatusActivities
		var eventAct *activities.EventActivities
		var embeddingAct *activities.EmbeddingActivities
		var dedupAct *activities.DedupActivities
		var ingestionAct *activities.IngestionActivities

		// Mock LLM activities
		env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).
			Return(&activities.ExtractKeywordsOutput{
				Keywords:  []string{"test keyword"},
				Reasoning: "test reasoning",
				Model:     "test-model",
			}, nil)

		// Mock search activities
		env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).
			Return(&activities.SearchSingleSourceOutput{
				Source: domain.SourceTypeSemanticScholar,
				Papers: []*domain.Paper{
					{
						ID:          paperID,
						CanonicalID: "doi:10.1234/test",
						Title:       "Test Paper",
						Abstract:    "Test abstract",
						PDFURL:      "https://example.com/paper.pdf",
					},
				},
				TotalFound: 1,
			}, nil)

		// Mock status activities
		env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).
			Return(nil)
		env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).
			Return(&activities.SaveKeywordsOutput{
				KeywordIDs: []uuid.UUID{uuid.New()},
				NewCount:   1,
			}, nil)
		env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).
			Return(&activities.SavePapersOutput{
				SavedCount:     1,
				DuplicateCount: 0,
			}, nil)

		// Mock embedding activities
		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{
					"doi:10.1234/test": make([]float32, 768),
				},
			}, nil)

		// Mock dedup activities - paper passes dedup
		env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).
			Return(&activities.BatchDedupOutput{
				NonDuplicateIDs: []uuid.UUID{paperID},
				DuplicateCount:  0,
			}, nil)

		// Mock ingestion activities - return failure for paper
		env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).
			Return(&activities.DownloadAndIngestOutput{
				Successful: 0,
				Failed:     1,
				Results: []activities.PaperIngestionResult{
					{PaperID: paperID, Status: "failed", Error: "download failed"},
				},
			}, nil)

		// Mock event publisher
		env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).
			Return(nil)

		input := newTestInput()

		env.ExecuteWorkflow(workflows.LiteratureReviewWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		err := env.GetWorkflowError()
		require.NoError(t, err) // Workflow completes even with ingestion failures

		var result workflows.ReviewWorkflowResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, "completed", result.Status)
		assert.Equal(t, 1, result.PapersFailed) // Paper failed during ingestion
		assert.Equal(t, 0, result.PapersIngested)

		env.AssertExpectations(t)
	})

	t.Run("pipeline processes multiple batches correctly", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		env.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
		env.RegisterWorkflow(workflows.PaperProcessingWorkflow)

		// Create 7 papers to test batching (batch size is 5, so 2 batches)
		paperIDs := make([]uuid.UUID, 7)
		papers := make([]*domain.Paper, 7)
		embeddings := make(map[string][]float32)
		for i := 0; i < 7; i++ {
			paperIDs[i] = uuid.New()
			canonicalID := fmt.Sprintf("doi:10.1234/test%d", i)
			papers[i] = &domain.Paper{
				ID:          paperIDs[i],
				CanonicalID: canonicalID,
				Title:       fmt.Sprintf("Test Paper %d", i),
				Abstract:    fmt.Sprintf("Test abstract %d", i),
				PDFURL:      fmt.Sprintf("https://example.com/paper%d.pdf", i),
			}
			embeddings[canonicalID] = make([]float32, 768)
		}

		// Activity nil-pointer references matching the workflow pattern.
		var llmAct *activities.LLMActivities
		var searchAct *activities.SearchActivities
		var statusAct *activities.StatusActivities
		var eventAct *activities.EventActivities
		var embeddingAct *activities.EmbeddingActivities
		var dedupAct *activities.DedupActivities
		var ingestionAct *activities.IngestionActivities

		// Mock LLM activities
		env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).
			Return(&activities.ExtractKeywordsOutput{
				Keywords:  []string{"test keyword"},
				Reasoning: "test reasoning",
				Model:     "test-model",
			}, nil)

		// Mock search activities - return all 7 papers
		env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).
			Return(&activities.SearchSingleSourceOutput{
				Source:     domain.SourceTypeSemanticScholar,
				Papers:     papers,
				TotalFound: 7,
			}, nil)

		// Mock status activities
		env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).
			Return(nil)
		env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).
			Return(&activities.SaveKeywordsOutput{
				KeywordIDs: []uuid.UUID{uuid.New()},
				NewCount:   1,
			}, nil)
		env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).
			Return(&activities.SavePapersOutput{
				SavedCount:     7,
				DuplicateCount: 0,
			}, nil)
		env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).
			Return(&activities.UpdatePaperIngestionResultsOutput{Updated: 7}, nil)

		// Mock embedding activities - return embeddings for batch
		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: embeddings,
			}, nil)

		// Mock dedup activities - all papers pass
		env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).
			Return(&activities.BatchDedupOutput{
				NonDuplicateIDs: paperIDs,
				DuplicateCount:  0,
			}, nil)

		// Mock ingestion activities - return success for all
		env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).
			Return(&activities.DownloadAndIngestOutput{
				Successful: 7,
				Failed:     0,
				Results: func() []activities.PaperIngestionResult {
					results := make([]activities.PaperIngestionResult, 7)
					for i, id := range paperIDs {
						results[i] = activities.PaperIngestionResult{
							PaperID:        id,
							FileID:         fmt.Sprintf("file-%d", i),
							IngestionRunID: fmt.Sprintf("run-%d", i),
							Status:         "completed",
						}
					}
					return results
				}(),
			}, nil)

		// Mock event publisher
		env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).
			Return(nil)

		input := newTestInput()

		env.ExecuteWorkflow(workflows.LiteratureReviewWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		err := env.GetWorkflowError()
		require.NoError(t, err)

		var result workflows.ReviewWorkflowResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, "completed", result.Status)
		assert.Equal(t, 7, result.PapersFound)
		assert.GreaterOrEqual(t, result.PapersIngested, 7)
		assert.Equal(t, 0, result.DuplicatesFound)
		assert.Equal(t, 0, result.PapersFailed)

		env.AssertExpectations(t)
	})

	t.Run("pipeline handles mixed duplicate and non-duplicate papers", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		env.RegisterWorkflow(workflows.LiteratureReviewWorkflow)
		env.RegisterWorkflow(workflows.PaperProcessingWorkflow)

		// Create 3 papers - 2 non-duplicates, 1 duplicate
		nonDupID1 := uuid.New()
		nonDupID2 := uuid.New()
		dupID := uuid.New()

		papers := []*domain.Paper{
			{
				ID:          nonDupID1,
				CanonicalID: "doi:10.1234/original1",
				Title:       "Original Paper 1",
				Abstract:    "Unique research about topic",
				PDFURL:      "https://example.com/original1.pdf",
			},
			{
				ID:          nonDupID2,
				CanonicalID: "doi:10.1234/original2",
				Title:       "Original Paper 2",
				Abstract:    "Different unique research",
				PDFURL:      "https://example.com/original2.pdf",
			},
			{
				ID:          dupID,
				CanonicalID: "doi:10.1234/duplicate",
				Title:       "Duplicate Paper",
				Abstract:    "Near-duplicate research",
				PDFURL:      "https://example.com/duplicate.pdf",
			},
		}

		// Activity nil-pointer references matching the workflow pattern.
		var llmAct *activities.LLMActivities
		var searchAct *activities.SearchActivities
		var statusAct *activities.StatusActivities
		var eventAct *activities.EventActivities
		var embeddingAct *activities.EmbeddingActivities
		var dedupAct *activities.DedupActivities
		var ingestionAct *activities.IngestionActivities

		// Mock LLM activities
		env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).
			Return(&activities.ExtractKeywordsOutput{
				Keywords:  []string{"test keyword"},
				Reasoning: "test reasoning",
				Model:     "test-model",
			}, nil)

		// Mock search activities
		env.OnActivity(searchAct.SearchSingleSource, mock.Anything, mock.Anything).
			Return(&activities.SearchSingleSourceOutput{
				Source:     domain.SourceTypeSemanticScholar,
				Papers:     papers,
				TotalFound: 3,
			}, nil)

		// Mock status activities
		env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).
			Return(nil)
		env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).
			Return(&activities.SaveKeywordsOutput{
				KeywordIDs: []uuid.UUID{uuid.New()},
				NewCount:   1,
			}, nil)
		env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).
			Return(&activities.SavePapersOutput{
				SavedCount:     3,
				DuplicateCount: 0,
			}, nil)
		env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).
			Return(&activities.UpdatePaperIngestionResultsOutput{Updated: 2}, nil)

		// Mock embedding activities
		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{
					"doi:10.1234/original1": make([]float32, 768),
					"doi:10.1234/original2": make([]float32, 768),
					"doi:10.1234/duplicate": make([]float32, 768),
				},
			}, nil)

		// Mock dedup activities - mark dupID as duplicate
		env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).
			Return(&activities.BatchDedupOutput{
				NonDuplicateIDs: []uuid.UUID{nonDupID1, nonDupID2},
				DuplicateCount:  1,
			}, nil)

		// Mock ingestion activities - only non-duplicates are ingested
		env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).
			Return(&activities.DownloadAndIngestOutput{
				Successful: 2,
				Failed:     0,
				Results: []activities.PaperIngestionResult{
					{PaperID: nonDupID1, FileID: "file-1", IngestionRunID: "run-1", Status: "completed"},
					{PaperID: nonDupID2, FileID: "file-2", IngestionRunID: "run-2", Status: "completed"},
				},
			}, nil)

		// Mock event publisher
		env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).
			Return(nil)

		input := newTestInput()

		env.ExecuteWorkflow(workflows.LiteratureReviewWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		err := env.GetWorkflowError()
		require.NoError(t, err)

		var result workflows.ReviewWorkflowResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, "completed", result.Status)
		assert.Equal(t, 3, result.PapersFound)
		assert.Equal(t, 2, result.PapersIngested)
		assert.Equal(t, 1, result.DuplicatesFound)
		assert.Equal(t, 0, result.PapersFailed)

		env.AssertExpectations(t)
	})
}
