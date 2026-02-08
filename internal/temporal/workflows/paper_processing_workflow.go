package workflows

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/helixir/literature-review-service/internal/temporal/activities"
)

// PaperProcessingInput is the input for the paper processing child workflow.
type PaperProcessingInput struct {
	// OrgID is the organization identifier.
	OrgID string `json:"org_id"`
	// ProjectID is the project identifier.
	ProjectID string `json:"project_id"`
	// RequestID is the parent review request ID.
	RequestID string `json:"request_id"`
	// Batch contains the paper IDs to process. Full paper details are fetched from DB.
	Batch PaperIDBatch `json:"batch"`
	// ParentWorkflowID is the parent workflow ID for signaling.
	ParentWorkflowID string `json:"parent_workflow_id"`
}

// PaperProcessingResult is the result of the paper processing child workflow.
type PaperProcessingResult struct {
	// BatchID is the batch identifier.
	BatchID string `json:"batch_id"`
	// Processed is total papers processed.
	Processed int `json:"processed"`
	// Duplicates is papers filtered as duplicates.
	Duplicates int `json:"duplicates"`
	// Ingested is papers successfully ingested.
	Ingested int `json:"ingested"`
	// Failed is papers that failed processing.
	Failed int `json:"failed"`
}

// PaperProcessingWorkflow processes a batch of papers through fetch -> embed -> dedup -> ingest.
//
// The workflow proceeds through the following stages:
//  0. Fetch: Retrieve full paper details from DB using paper IDs
//  1. Embed: Generate embeddings for paper abstracts
//  2. Dedup: Check papers against vector store for semantic duplicates
//  3. Ingest: Download and ingest non-duplicate papers with PDF URLs
//  4. Update: Save ingestion results to paper records
//
// The workflow signals the parent workflow upon completion with batch results.
// Dedup failure is non-fatal (workflow continues with all papers).
// Embedding failure is fatal (cannot process without embeddings).
func PaperProcessingWorkflow(ctx workflow.Context, input PaperProcessingInput) (*PaperProcessingResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("starting paper processing workflow",
		"batchID", input.Batch.BatchID,
		"paperCount", len(input.Batch.PaperIDs),
	)

	// Validate RequestID early to avoid wasting resources on embed/dedup.
	requestID, parseErr := uuid.Parse(input.RequestID)
	if parseErr != nil {
		logger.Error("invalid request ID", "requestID", input.RequestID, "error", parseErr)
		return &PaperProcessingResult{
			BatchID: input.Batch.BatchID,
			Failed:  len(input.Batch.PaperIDs),
		}, fmt.Errorf("parse request ID %q: %w", input.RequestID, parseErr)
	}

	result := &PaperProcessingResult{
		BatchID:   input.Batch.BatchID,
		Processed: len(input.Batch.PaperIDs),
	}

	// Activity pointers for method references
	var embeddingAct *activities.EmbeddingActivities
	var dedupAct *activities.DedupActivities
	var ingestionAct *activities.IngestionActivities
	var statusAct *activities.StatusActivities

	// Activity options
	embeddingCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		HeartbeatTimeout:    1 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    5,
		},
	})

	dedupCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 1 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    3,
		},
	})

	ingestionCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		HeartbeatTimeout:    2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    1 * time.Minute,
			MaximumAttempts:    5,
		},
	})

	statusCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    5,
		},
	})

	// =========================================================================
	// Stage 0: Fetch paper details from DB
	// =========================================================================
	logger.Info("stage 0: fetching papers from DB", "count", len(input.Batch.PaperIDs))

	var fetchOutput activities.FetchPaperBatchOutput
	err := workflow.ExecuteActivity(statusCtx, statusAct.FetchPaperBatch, activities.FetchPaperBatchInput{
		PaperIDs: input.Batch.PaperIDs,
	}).Get(ctx, &fetchOutput)
	if err != nil {
		logger.Error("failed to fetch papers from DB", "error", err)
		result.Failed = len(input.Batch.PaperIDs)
		return result, fmt.Errorf("fetch paper batch: %w", err)
	}

	papers := fetchOutput.Papers
	if len(papers) == 0 {
		logger.Warn("no papers found in DB for batch", "batchID", input.Batch.BatchID)
		return result, nil
	}

	result.Processed = len(papers)
	logger.Info("papers fetched from DB", "requested", len(input.Batch.PaperIDs), "found", len(papers))

	// =========================================================================
	// Stage 1: Embed paper abstracts
	// =========================================================================
	logger.Info("stage 1: embedding papers")

	papersForEmbedding := make([]activities.PaperForEmbedding, 0, len(papers))
	for _, p := range papers {
		papersForEmbedding = append(papersForEmbedding, activities.PaperForEmbedding{
			PaperID:     p.PaperID,
			CanonicalID: p.CanonicalID,
			Abstract:    p.Abstract,
		})
	}

	var embedOutput activities.EmbedPapersOutput
	err = workflow.ExecuteActivity(embeddingCtx, embeddingAct.EmbedPapers, activities.EmbedPapersInput{
		Papers: papersForEmbedding,
	}).Get(ctx, &embedOutput)
	if err != nil {
		logger.Error("embedding failed", "error", err)
		result.Failed = len(papers)
		return result, fmt.Errorf("embed papers: %w", err)
	}

	logger.Info("embedding completed", "embedded", len(embedOutput.Embeddings), "skipped", embedOutput.Skipped)

	// =========================================================================
	// Stage 2: Dedup against Qdrant
	// =========================================================================
	logger.Info("stage 2: deduplicating papers")

	// Build papers for dedup (only those with embeddings)
	var papersForDedup []*activities.PaperWithEmbedding
	paperMap := make(map[string]activities.PaperForProcessing)
	for _, p := range papers {
		paperMap[p.CanonicalID] = p
		if embedding, ok := embedOutput.Embeddings[p.CanonicalID]; ok {
			papersForDedup = append(papersForDedup, &activities.PaperWithEmbedding{
				PaperID:     p.PaperID,
				CanonicalID: p.CanonicalID,
				Embedding:   embedding,
			})
		}
	}

	var nonDuplicatePaperIDs []uuid.UUID
	if len(papersForDedup) > 0 {
		var dedupOutput activities.BatchDedupOutput
		err = workflow.ExecuteActivity(dedupCtx, dedupAct.BatchDedup, activities.BatchDedupInput{
			Papers: papersForDedup,
		}).Get(ctx, &dedupOutput)
		if err != nil {
			// Dedup failure is non-fatal: log and continue with all papers
			logger.Warn("dedup failed, continuing with all papers", "error", err)
			for _, p := range papers {
				nonDuplicatePaperIDs = append(nonDuplicatePaperIDs, p.PaperID)
			}
		} else {
			nonDuplicatePaperIDs = dedupOutput.NonDuplicateIDs
			result.Duplicates = dedupOutput.DuplicateCount
			logger.Info("dedup completed", "nonDuplicates", len(nonDuplicatePaperIDs), "duplicates", dedupOutput.DuplicateCount)
		}
	} else {
		// No embeddings, skip dedup
		for _, p := range papers {
			nonDuplicatePaperIDs = append(nonDuplicatePaperIDs, p.PaperID)
		}
	}

	// Build a map of paper ID -> canonical ID for lookups
	paperIDToCanonical := make(map[uuid.UUID]string)
	for _, p := range papers {
		paperIDToCanonical[p.PaperID] = p.CanonicalID
	}

	// =========================================================================
	// Stage 3: Download and ingest non-duplicate papers
	// =========================================================================
	logger.Info("stage 3: ingesting papers", "count", len(nonDuplicatePaperIDs))

	// Build papers for ingestion
	papersForIngestion := make([]activities.PaperForIngestion, 0)
	for _, paperID := range nonDuplicatePaperIDs {
		canonicalID := paperIDToCanonical[paperID]
		if p, ok := paperMap[canonicalID]; ok && p.PDFURL != "" {
			papersForIngestion = append(papersForIngestion, activities.PaperForIngestion{
				PaperID:     p.PaperID,
				PDFURL:      p.PDFURL,
				CanonicalID: p.CanonicalID,
			})
		}
	}

	var paperResults []PaperResult
	if len(papersForIngestion) > 0 {
		var ingestionOutput activities.DownloadAndIngestOutput
		err = workflow.ExecuteActivity(ingestionCtx, ingestionAct.DownloadAndIngestPapers, activities.DownloadAndIngestInput{
			OrgID:     input.OrgID,
			ProjectID: input.ProjectID,
			RequestID: requestID,
			Papers:    papersForIngestion,
		}).Get(ctx, &ingestionOutput)
		if err != nil {
			logger.Error("ingestion failed", "error", err)
			result.Failed += len(papersForIngestion)
		} else {
			result.Ingested = ingestionOutput.Successful
			result.Failed += ingestionOutput.Failed

			// Convert to paper results for DB update
			for _, r := range ingestionOutput.Results {
				paperResults = append(paperResults, PaperResult{
					PaperID:        r.PaperID,
					FileID:         r.FileID,
					IngestionRunID: r.IngestionRunID,
					Status:         r.Status,
					Error:          r.Error,
				})
			}
		}
	}

	// =========================================================================
	// Stage 4: Update paper records with ingestion results
	// =========================================================================
	if len(paperResults) > 0 {
		logger.Info("updating paper records", "count", len(paperResults))

		ingestionResults := make([]activities.PaperIngestionResult, 0, len(paperResults))
		for _, pr := range paperResults {
			ingestionResults = append(ingestionResults, activities.PaperIngestionResult{
				PaperID:        pr.PaperID,
				FileID:         pr.FileID,
				IngestionRunID: pr.IngestionRunID,
				Status:         pr.Status,
				Error:          pr.Error,
			})
		}

		var updateOutput activities.UpdatePaperIngestionResultsOutput
		err = workflow.ExecuteActivity(statusCtx, statusAct.UpdatePaperIngestionResults, activities.UpdatePaperIngestionResultsInput{
			Results: ingestionResults,
		}).Get(ctx, &updateOutput)
		if err != nil {
			logger.Warn("failed to update paper records", "error", err)
		}
	}

	// =========================================================================
	// Signal parent with completion
	// =========================================================================
	if input.ParentWorkflowID != "" {
		signal := BatchCompleteSignal{
			BatchID:    input.Batch.BatchID,
			Processed:  result.Processed,
			Duplicates: result.Duplicates,
			Ingested:   result.Ingested,
			Failed:     result.Failed,
			Results:    paperResults,
		}

		err = workflow.SignalExternalWorkflow(ctx, input.ParentWorkflowID, "", SignalBatchComplete, signal).Get(ctx, nil)
		if err != nil {
			logger.Warn("failed to signal parent workflow", "error", err)
		}
	}

	logger.Info("paper processing workflow completed",
		"batchID", input.Batch.BatchID,
		"processed", result.Processed,
		"duplicates", result.Duplicates,
		"ingested", result.Ingested,
		"failed", result.Failed,
	)

	return result, nil
}
