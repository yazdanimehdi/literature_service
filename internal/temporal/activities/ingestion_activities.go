package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	"github.com/helixir/literature-review-service/internal/ingestion"
	"github.com/helixir/literature-review-service/internal/observability"
	"github.com/helixir/literature-review-service/internal/pdf"
	"github.com/helixir/literature-review-service/internal/temporal/resilience"
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

// PDFDownloader defines the interface for downloading PDFs.
type PDFDownloader interface {
	Download(ctx context.Context, url string) (*pdf.DownloadResult, error)
}

// StreamingIngestionClient defines the interface for streaming PDF content to ingestion.
type StreamingIngestionClient interface {
	StartIngestionWithContent(ctx context.Context, req ingestion.StartIngestionWithContentRequest) (*ingestion.StartIngestionWithContentResult, error)
}

// IngestionActivities provides Temporal activities for paper ingestion operations.
// Methods on this struct are registered as Temporal activities via the worker.
type IngestionActivities struct {
	client          IngestionClient
	downloader      PDFDownloader
	streamingClient StreamingIngestionClient
	metrics         *observability.Metrics
	breakers        *resilience.BreakerRegistry
}

// NewIngestionActivities creates a new IngestionActivities instance.
// The metrics parameter may be nil (metrics recording will be skipped).
// The downloader and streamingClient parameters may be nil if DownloadAndIngestPapers
// activity will not be used.
// The breakers parameter may be nil (circuit breaker protection will be skipped).
func NewIngestionActivities(
	client IngestionClient,
	downloader PDFDownloader,
	streamingClient StreamingIngestionClient,
	metrics *observability.Metrics,
	breakers ...*resilience.BreakerRegistry,
) *IngestionActivities {
	a := &IngestionActivities{
		client:          client,
		downloader:      downloader,
		streamingClient: streamingClient,
		metrics:         metrics,
	}
	if len(breakers) > 0 && breakers[0] != nil {
		a.breakers = breakers[0]
	}
	return a
}

// SubmitPaperForIngestion submits a single paper to the ingestion service for processing.
func (a *IngestionActivities) SubmitPaperForIngestion(ctx context.Context, input SubmitPaperForIngestionInput) (*SubmitPaperForIngestionOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("submitting paper for ingestion",
		"paperID", input.PaperID,
		"requestID", input.RequestID,
	)

	// Circuit breaker check.
	if a.breakers != nil {
		cb := a.breakers.Get("ingestion")
		if cbErr := cb.Allow(); cbErr != nil {
			logger.Warn("ingestion circuit breaker open", "error", cbErr)
			return nil, temporal.NewNonRetryableApplicationError(
				"circuit_open: ingestion",
				"circuit_open",
				cbErr,
			)
		}
	}

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
		if a.breakers != nil {
			a.breakers.Get("ingestion").RecordFailure()
		}
		return nil, fmt.Errorf("submit paper %s for ingestion: %w", input.PaperID, err)
	}

	// Record success on the circuit breaker.
	if a.breakers != nil {
		a.breakers.Get("ingestion").RecordSuccess()
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

	// Circuit breaker check — fail the entire batch early if ingestion is down.
	if a.breakers != nil {
		cb := a.breakers.Get("ingestion")
		if cbErr := cb.Allow(); cbErr != nil {
			logger.Warn("ingestion circuit breaker open", "error", cbErr)
			return nil, temporal.NewNonRetryableApplicationError(
				"circuit_open: ingestion",
				"circuit_open",
				cbErr,
			)
		}
	}

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
			if a.breakers != nil {
				a.breakers.Get("ingestion").RecordFailure()
			}
			result.Failed++
			continue
		}

		if a.breakers != nil {
			a.breakers.Get("ingestion").RecordSuccess()
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

// DownloadAndIngestPapers downloads PDFs for papers and streams them to the ingestion service.
// This activity processes papers one at a time to bound memory usage.
// Papers without PDF URLs are skipped. Failures are non-fatal (processing continues).
func (a *IngestionActivities) DownloadAndIngestPapers(ctx context.Context, input DownloadAndIngestInput) (*DownloadAndIngestOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("starting download and ingest",
		"requestID", input.RequestID,
		"paperCount", len(input.Papers),
	)

	if a.downloader == nil {
		return nil, fmt.Errorf("downloader is not configured")
	}
	if a.streamingClient == nil {
		return nil, fmt.Errorf("streaming client is not configured")
	}

	// Circuit breaker check — fail the entire batch early if ingestion is down.
	if a.breakers != nil {
		cb := a.breakers.Get("ingestion")
		if cbErr := cb.Allow(); cbErr != nil {
			logger.Warn("ingestion circuit breaker open for download+ingest", "error", cbErr)
			return nil, temporal.NewNonRetryableApplicationError(
				"circuit_open: ingestion",
				"circuit_open",
				cbErr,
			)
		}
	}

	output := &DownloadAndIngestOutput{
		Results: make([]PaperIngestionResult, 0, len(input.Papers)),
	}

	for i, paper := range input.Papers {
		// Record heartbeat for long-running activity
		activity.RecordHeartbeat(ctx, fmt.Sprintf("processing paper %d/%d: %s", i+1, len(input.Papers), paper.PaperID))

		// Check if context was cancelled
		if ctx.Err() != nil {
			logger.Info("activity cancelled", "processed", i, "total", len(input.Papers))
			return output, ctx.Err()
		}

		result := PaperIngestionResult{
			PaperID: paper.PaperID,
		}

		// Skip papers without PDF URL
		if paper.PDFURL == "" {
			output.Skipped++
			result.Error = "no PDF URL"
			output.Results = append(output.Results, result)
			continue
		}

		// Download the PDF
		downloadResult, err := a.downloader.Download(ctx, paper.PDFURL)
		if err != nil {
			logger.Warn("failed to download PDF",
				"paperID", paper.PaperID,
				"url", paper.PDFURL,
				"error", err,
			)
			output.Failed++
			result.Error = fmt.Sprintf("download failed: %v", err)
			output.Results = append(output.Results, result)
			continue
		}

		// Build idempotency key
		idempotencyKey := fmt.Sprintf("%s/%s/%s", idempotencyKeyPrefix, input.RequestID, paper.PaperID)

		// Derive filename from canonical ID or paper ID
		filename := paper.CanonicalID
		if filename == "" {
			filename = paper.PaperID.String()
		}
		filename = filename + ".pdf"

		// Stream to ingestion service
		ingestionResult, err := a.streamingClient.StartIngestionWithContent(ctx, ingestion.StartIngestionWithContentRequest{
			OrgID:          input.OrgID,
			ProjectID:      input.ProjectID,
			IdempotencyKey: idempotencyKey,
			MimeType:       mimeTypePDF,
			ContentHash:    downloadResult.ContentHash,
			FileSize:       downloadResult.SizeBytes,
			SourceKind:     "literature_review",
			Filename:       filename,
			Content:        downloadResult.Content,
		})
		if err != nil {
			logger.Warn("failed to submit to ingestion",
				"paperID", paper.PaperID,
				"error", err,
			)
			if a.breakers != nil {
				a.breakers.Get("ingestion").RecordFailure()
			}
			output.Failed++
			result.Error = fmt.Sprintf("ingestion failed: %v", err)
			output.Results = append(output.Results, result)
			continue
		}

		if a.breakers != nil {
			a.breakers.Get("ingestion").RecordSuccess()
		}
		result.FileID = ingestionResult.FileID
		result.IngestionRunID = ingestionResult.RunID
		result.Status = ingestionResult.Status
		output.Successful++
		output.Results = append(output.Results, result)

		logger.Info("paper downloaded and ingested",
			"paperID", paper.PaperID,
			"fileID", ingestionResult.FileID,
			"runID", ingestionResult.RunID,
		)
	}

	logger.Info("download and ingest completed",
		"successful", output.Successful,
		"failed", output.Failed,
		"skipped", output.Skipped,
	)

	return output, nil
}
