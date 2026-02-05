package workflows

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/temporal/activities"
)

func TestPaperProcessingWorkflow(t *testing.T) {
	t.Run("processes batch successfully", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		paperID := uuid.New()

		// Activity nil-pointer references matching the workflow pattern.
		var embeddingAct *activities.EmbeddingActivities
		var dedupAct *activities.DedupActivities
		var ingestionAct *activities.IngestionActivities
		var statusAct *activities.StatusActivities

		// Mock activities
		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{
					"doi:123": {0.1, 0.2, 0.3},
				},
			}, nil)

		env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).
			Return(&activities.BatchDedupOutput{
				NonDuplicateIDs: []uuid.UUID{paperID},
				DuplicateCount:  0,
			}, nil)

		env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).
			Return(&activities.DownloadAndIngestOutput{
				Successful: 1,
				Failed:     0,
				Results: []activities.PaperIngestionResult{
					{PaperID: paperID, FileID: "file-123", IngestionRunID: "run-456", Status: "completed"},
				},
			}, nil)

		env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).
			Return(&activities.UpdatePaperIngestionResultsOutput{Updated: 1}, nil)

		input := PaperProcessingInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New().String(),
			Batch: PaperBatch{
				BatchID: "batch-1",
				Papers: []PaperForProcessing{
					{
						PaperID:     paperID,
						CanonicalID: "doi:123",
						Title:       "Test Paper",
						Abstract:    "Test abstract",
						PDFURL:      "https://example.com/paper.pdf",
					},
				},
			},
		}

		env.ExecuteWorkflow(PaperProcessingWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		require.NoError(t, env.GetWorkflowError())

		var result PaperProcessingResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, "batch-1", result.BatchID)
		assert.Equal(t, 1, result.Processed)
		assert.Equal(t, 1, result.Ingested)
		assert.Equal(t, 0, result.Failed)
		assert.Equal(t, 0, result.Duplicates)
	})

	t.Run("handles embedding failure", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		var embeddingAct *activities.EmbeddingActivities

		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("embedding service unavailable"))

		input := PaperProcessingInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New().String(),
			Batch: PaperBatch{
				BatchID: "batch-1",
				Papers: []PaperForProcessing{
					{
						PaperID:     uuid.New(),
						CanonicalID: "doi:123",
						Title:       "Test Paper",
						Abstract:    "Test abstract",
						PDFURL:      "https://example.com/paper.pdf",
					},
				},
			},
		}

		env.ExecuteWorkflow(PaperProcessingWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		require.Error(t, env.GetWorkflowError())
	})

	t.Run("continues when dedup fails", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		paperID := uuid.New()

		var embeddingAct *activities.EmbeddingActivities
		var dedupAct *activities.DedupActivities
		var ingestionAct *activities.IngestionActivities
		var statusAct *activities.StatusActivities

		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{
					"doi:123": {0.1, 0.2, 0.3},
				},
			}, nil)

		env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("qdrant unavailable"))

		env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).
			Return(&activities.DownloadAndIngestOutput{
				Successful: 1,
				Failed:     0,
				Results: []activities.PaperIngestionResult{
					{PaperID: paperID, FileID: "file-123", IngestionRunID: "run-456", Status: "completed"},
				},
			}, nil)

		env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).
			Return(&activities.UpdatePaperIngestionResultsOutput{Updated: 1}, nil)

		input := PaperProcessingInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New().String(),
			Batch: PaperBatch{
				BatchID: "batch-1",
				Papers: []PaperForProcessing{
					{
						PaperID:     paperID,
						CanonicalID: "doi:123",
						Title:       "Test Paper",
						Abstract:    "Test abstract",
						PDFURL:      "https://example.com/paper.pdf",
					},
				},
			},
		}

		env.ExecuteWorkflow(PaperProcessingWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		require.NoError(t, env.GetWorkflowError()) // Dedup failure is non-fatal

		var result PaperProcessingResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, 1, result.Ingested) // Should still ingest
	})

	t.Run("filters duplicates from ingestion", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		paperID1 := uuid.New()
		paperID2 := uuid.New()

		var embeddingAct *activities.EmbeddingActivities
		var dedupAct *activities.DedupActivities
		var ingestionAct *activities.IngestionActivities
		var statusAct *activities.StatusActivities

		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{
					"doi:123": {0.1, 0.2, 0.3},
					"doi:456": {0.4, 0.5, 0.6},
				},
			}, nil)

		// Only paperID1 passes dedup, paperID2 is marked as duplicate
		env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).
			Return(&activities.BatchDedupOutput{
				NonDuplicateIDs: []uuid.UUID{paperID1},
				DuplicateCount:  1,
			}, nil)

		// Ingestion should only receive 1 paper (the non-duplicate)
		env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything,
			mock.MatchedBy(func(input activities.DownloadAndIngestInput) bool {
				return len(input.Papers) == 1 && input.Papers[0].PaperID == paperID1
			})).
			Return(&activities.DownloadAndIngestOutput{
				Successful: 1,
				Failed:     0,
				Results: []activities.PaperIngestionResult{
					{PaperID: paperID1, FileID: "file-123", IngestionRunID: "run-456", Status: "completed"},
				},
			}, nil)

		env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).
			Return(&activities.UpdatePaperIngestionResultsOutput{Updated: 1}, nil)

		input := PaperProcessingInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New().String(),
			Batch: PaperBatch{
				BatchID: "batch-1",
				Papers: []PaperForProcessing{
					{
						PaperID:     paperID1,
						CanonicalID: "doi:123",
						Title:       "Test Paper 1",
						Abstract:    "Test abstract 1",
						PDFURL:      "https://example.com/paper1.pdf",
					},
					{
						PaperID:     paperID2,
						CanonicalID: "doi:456",
						Title:       "Test Paper 2",
						Abstract:    "Test abstract 2",
						PDFURL:      "https://example.com/paper2.pdf",
					},
				},
			},
		}

		env.ExecuteWorkflow(PaperProcessingWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		require.NoError(t, env.GetWorkflowError())

		var result PaperProcessingResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, 2, result.Processed)
		assert.Equal(t, 1, result.Duplicates)
		assert.Equal(t, 1, result.Ingested)
		assert.Equal(t, 0, result.Failed)
	})

	t.Run("skips papers without PDF URL", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		paperID := uuid.New()

		var embeddingAct *activities.EmbeddingActivities
		var dedupAct *activities.DedupActivities

		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{
					"doi:123": {0.1, 0.2, 0.3},
				},
			}, nil)

		env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).
			Return(&activities.BatchDedupOutput{
				NonDuplicateIDs: []uuid.UUID{paperID},
				DuplicateCount:  0,
			}, nil)

		// DownloadAndIngestPapers should NOT be called since there's no PDF URL

		input := PaperProcessingInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New().String(),
			Batch: PaperBatch{
				BatchID: "batch-1",
				Papers: []PaperForProcessing{
					{
						PaperID:     paperID,
						CanonicalID: "doi:123",
						Title:       "Test Paper",
						Abstract:    "Test abstract",
						PDFURL:      "", // No PDF URL
					},
				},
			},
		}

		env.ExecuteWorkflow(PaperProcessingWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		require.NoError(t, env.GetWorkflowError())

		var result PaperProcessingResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, 1, result.Processed)
		assert.Equal(t, 0, result.Duplicates)
		assert.Equal(t, 0, result.Ingested)
		assert.Equal(t, 0, result.Failed)
	})

	t.Run("signals parent on completion", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		paperID := uuid.New()
		parentWorkflowID := "parent-workflow-123"

		var embeddingAct *activities.EmbeddingActivities
		var dedupAct *activities.DedupActivities
		var ingestionAct *activities.IngestionActivities
		var statusAct *activities.StatusActivities

		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{
					"doi:123": {0.1, 0.2, 0.3},
				},
			}, nil)

		env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).
			Return(&activities.BatchDedupOutput{
				NonDuplicateIDs: []uuid.UUID{paperID},
				DuplicateCount:  0,
			}, nil)

		env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).
			Return(&activities.DownloadAndIngestOutput{
				Successful: 1,
				Failed:     0,
				Results: []activities.PaperIngestionResult{
					{PaperID: paperID, FileID: "file-123", IngestionRunID: "run-456", Status: "completed"},
				},
			}, nil)

		env.OnActivity(statusAct.UpdatePaperIngestionResults, mock.Anything, mock.Anything).
			Return(&activities.UpdatePaperIngestionResultsOutput{Updated: 1}, nil)

		// Mock the signal to parent workflow
		env.OnSignalExternalWorkflow(mock.Anything, parentWorkflowID, "", SignalBatchComplete, mock.Anything).Return(nil)

		input := PaperProcessingInput{
			OrgID:            "org-1",
			ProjectID:        "proj-1",
			RequestID:        uuid.New().String(),
			ParentWorkflowID: parentWorkflowID,
			Batch: PaperBatch{
				BatchID: "batch-1",
				Papers: []PaperForProcessing{
					{
						PaperID:     paperID,
						CanonicalID: "doi:123",
						Title:       "Test Paper",
						Abstract:    "Test abstract",
						PDFURL:      "https://example.com/paper.pdf",
					},
				},
			},
		}

		env.ExecuteWorkflow(PaperProcessingWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		require.NoError(t, env.GetWorkflowError())

		var result PaperProcessingResult
		require.NoError(t, env.GetWorkflowResult(&result))

		// Verify workflow completed successfully with parent workflow ID set
		assert.Equal(t, 1, result.Ingested)
	})

	t.Run("handles ingestion failure gracefully", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		paperID := uuid.New()

		var embeddingAct *activities.EmbeddingActivities
		var dedupAct *activities.DedupActivities
		var ingestionAct *activities.IngestionActivities

		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{
					"doi:123": {0.1, 0.2, 0.3},
				},
			}, nil)

		env.OnActivity(dedupAct.BatchDedup, mock.Anything, mock.Anything).
			Return(&activities.BatchDedupOutput{
				NonDuplicateIDs: []uuid.UUID{paperID},
				DuplicateCount:  0,
			}, nil)

		env.OnActivity(ingestionAct.DownloadAndIngestPapers, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("ingestion service unavailable"))

		input := PaperProcessingInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New().String(),
			Batch: PaperBatch{
				BatchID: "batch-1",
				Papers: []PaperForProcessing{
					{
						PaperID:     paperID,
						CanonicalID: "doi:123",
						Title:       "Test Paper",
						Abstract:    "Test abstract",
						PDFURL:      "https://example.com/paper.pdf",
					},
				},
			},
		}

		env.ExecuteWorkflow(PaperProcessingWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		// Workflow should complete even when ingestion fails
		require.NoError(t, env.GetWorkflowError())

		var result PaperProcessingResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, 1, result.Processed)
		assert.Equal(t, 0, result.Ingested)
		assert.Equal(t, 1, result.Failed)
	})

	t.Run("handles empty batch", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		var embeddingAct *activities.EmbeddingActivities

		env.OnActivity(embeddingAct.EmbedPapers, mock.Anything, mock.Anything).
			Return(&activities.EmbedPapersOutput{
				Embeddings: map[string][]float32{},
			}, nil)

		// BatchDedup should not be called when no embeddings

		input := PaperProcessingInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New().String(),
			Batch: PaperBatch{
				BatchID: "batch-empty",
				Papers:  []PaperForProcessing{},
			},
		}

		env.ExecuteWorkflow(PaperProcessingWorkflow, input)

		require.True(t, env.IsWorkflowCompleted())
		require.NoError(t, env.GetWorkflowError())

		var result PaperProcessingResult
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, "batch-empty", result.BatchID)
		assert.Equal(t, 0, result.Processed)
		assert.Equal(t, 0, result.Duplicates)
		assert.Equal(t, 0, result.Ingested)
		assert.Equal(t, 0, result.Failed)
	})
}
