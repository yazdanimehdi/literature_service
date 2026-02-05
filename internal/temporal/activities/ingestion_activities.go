package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"

	"github.com/helixir/literature-review-service/internal/ingestion"
	"github.com/helixir/literature-review-service/internal/observability"
)

const (
	// mimeTypePDF is the MIME type used when submitting papers for ingestion.
	mimeTypePDF = "application/pdf"

	// idempotencyKeyPrefix is the prefix for ingestion idempotency keys.
	idempotencyKeyPrefix = "litreview"
)

// IngestionClient defines the interface for interacting with the ingestion service.
// This decouples the activity from the concrete ingestion.Client implementation,
// enabling straightforward testing with mock implementations.
type IngestionClient interface {
	StartIngestion(ctx context.Context, req ingestion.StartIngestionRequest) (*ingestion.StartIngestionResult, error)
	GetRunStatus(ctx context.Context, runID string) (*ingestion.RunStatus, error)
}

// IngestionActivities provides Temporal activities for paper ingestion operations.
// Methods on this struct are registered as Temporal activities via the worker.
type IngestionActivities struct {
	client  IngestionClient
	metrics *observability.Metrics
}

// NewIngestionActivities creates a new IngestionActivities instance.
// The metrics parameter may be nil (metrics recording will be skipped).
func NewIngestionActivities(client IngestionClient, metrics *observability.Metrics) *IngestionActivities {
	return &IngestionActivities{
		client:  client,
		metrics: metrics,
	}
}

// SubmitPaperForIngestion submits a single paper to the ingestion service for processing.
func (a *IngestionActivities) SubmitPaperForIngestion(ctx context.Context, input SubmitPaperForIngestionInput) (*SubmitPaperForIngestionOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("submitting paper for ingestion",
		"paperID", input.PaperID,
		"requestID", input.RequestID,
	)

	idempotencyKey := input.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("%s/%s/%s", idempotencyKeyPrefix, input.RequestID, input.PaperID)
	}

	result, err := a.client.StartIngestion(ctx, ingestion.StartIngestionRequest{
		OrgID:          input.OrgID,
		ProjectID:      input.ProjectID,
		IdempotencyKey: idempotencyKey,
		PDFURL:         input.PDFURL,
		MimeType:       mimeTypePDF,
	})
	if err != nil {
		logger.Error("failed to submit paper for ingestion",
			"paperID", input.PaperID,
			"error", err,
		)
		return nil, fmt.Errorf("submit paper %s for ingestion: %w", input.PaperID, err)
	}

	logger.Info("paper submitted for ingestion",
		"paperID", input.PaperID,
		"runID", result.RunID,
		"isExisting", result.IsExisting,
	)

	return &SubmitPaperForIngestionOutput{
		RunID:      result.RunID,
		IsExisting: result.IsExisting,
		Status:     result.Status,
	}, nil
}

// CheckIngestionStatus checks the current status of an ingestion run.
func (a *IngestionActivities) CheckIngestionStatus(ctx context.Context, input CheckIngestionStatusInput) (*CheckIngestionStatusOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Debug("checking ingestion status", "runID", input.RunID)

	status, err := a.client.GetRunStatus(ctx, input.RunID)
	if err != nil {
		return nil, fmt.Errorf("check ingestion status for run %s: %w", input.RunID, err)
	}

	return &CheckIngestionStatusOutput{
		RunID:        status.RunID,
		Status:       status.Status,
		IsTerminal:   status.IsTerminal,
		ErrorMessage: status.ErrorMessage,
	}, nil
}

// SubmitPapersForIngestion submits a batch of papers for ingestion.
// Papers without PDF URLs are skipped.
func (a *IngestionActivities) SubmitPapersForIngestion(ctx context.Context, input SubmitPapersForIngestionInput) (*SubmitPapersForIngestionOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("submitting papers for ingestion",
		"requestID", input.RequestID,
		"paperCount", len(input.Papers),
	)

	result := &SubmitPapersForIngestionOutput{
		RunIDs: make(map[string]string),
	}

	for _, paper := range input.Papers {
		if paper.PDFURL == "" {
			result.Skipped++
			continue
		}

		activity.RecordHeartbeat(ctx, fmt.Sprintf("submitting paper %s", paper.PaperID))

		idempotencyKey := fmt.Sprintf("%s/%s/%s", idempotencyKeyPrefix, input.RequestID, paper.PaperID)

		res, err := a.client.StartIngestion(ctx, ingestion.StartIngestionRequest{
			OrgID:          input.OrgID,
			ProjectID:      input.ProjectID,
			IdempotencyKey: idempotencyKey,
			PDFURL:         paper.PDFURL,
			MimeType:       mimeTypePDF,
		})
		if err != nil {
			logger.Warn("failed to submit paper for ingestion",
				"paperID", paper.PaperID,
				"error", err,
			)
			result.Failed++
			continue
		}

		result.RunIDs[paper.PaperID.String()] = res.RunID
		result.Submitted++
	}

	logger.Info("batch ingestion submission completed",
		"submitted", result.Submitted,
		"skipped", result.Skipped,
		"failed", result.Failed,
	)

	return result, nil
}
