// Package ingestion provides a gRPC client for the Ingestion Service.
package ingestion

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	ingestionv1 "github.com/helixir/ingestion-service/api/gen/ingestion/v1"
)

// Client wraps the ingestion service gRPC client with convenience methods.
type Client struct {
	conn    *grpc.ClientConn
	client  ingestionv1.IngestionServiceClient
	timeout time.Duration
}

// Config holds ingestion client configuration.
type Config struct {
	// Address is the gRPC target address of the ingestion service (e.g., "localhost:50051").
	Address string

	// Timeout is the per-request timeout for gRPC calls to the ingestion service.
	// Defaults to 30 seconds if zero.
	Timeout time.Duration
}

// NewClient creates a new ingestion service client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("ingestion service address is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	conn, err := grpc.NewClient(cfg.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to ingestion service: %w", err)
	}

	return &Client{
		conn:    conn,
		client:  ingestionv1.NewIngestionServiceClient(conn),
		timeout: cfg.Timeout,
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// StartIngestionRequest holds the parameters for starting an ingestion run.
type StartIngestionRequest struct {
	// OrgID is the organization identifier for multi-tenant isolation.
	OrgID string

	// ProjectID is the project identifier for multi-tenant isolation.
	ProjectID string

	// IdempotencyKey ensures duplicate submissions for the same paper are handled gracefully.
	// Typically formatted as "litreview/{requestID}/{paperID}".
	IdempotencyKey string

	// PDFURL is the URL to the paper's PDF file for download and processing.
	PDFURL string

	// MimeType is the content type of the file (typically "application/pdf").
	MimeType string
}

// StartIngestionResult holds the result of starting an ingestion run.
type StartIngestionResult struct {
	// RunID is the unique identifier for the created or existing ingestion run.
	RunID string

	// Status is the current status of the ingestion run (e.g., "PENDING", "PROCESSING").
	Status string

	// IsExisting is true if the idempotency key matched a previously created run.
	IsExisting bool
}

// StartIngestion initiates a new ingestion run or returns an existing run if the
// idempotency key matches a previous request. Auth metadata headers (x-org-id,
// x-project-id) are attached automatically.
func (c *Client) StartIngestion(ctx context.Context, req StartIngestionRequest) (*StartIngestionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	md := metadata.Pairs(
		"x-org-id", req.OrgID,
		"x-project-id", req.ProjectID,
	)
	ctx = metadata.NewOutgoingContext(ctx, md)

	resp, err := c.client.StartIngestion(ctx, &ingestionv1.StartIngestionRequest{
		OrgId:           req.OrgID,
		ProjectId:       req.ProjectID,
		IdempotencyKey:  req.IdempotencyKey,
		MimeType:        req.MimeType,
		FileContentHash: hashFromURL(req.PDFURL),
		SourceKind:      "literature_review",
	})
	if err != nil {
		return nil, fmt.Errorf("start ingestion: %w", err)
	}

	return &StartIngestionResult{
		RunID:      resp.GetRunId(),
		Status:     resp.GetStatus().String(),
		IsExisting: resp.GetIsExisting(),
	}, nil
}

// RunStatus holds the status details of an ingestion run.
type RunStatus struct {
	// RunID is the unique identifier of the ingestion run.
	RunID string

	// Status is the current status string of the ingestion run.
	Status string

	// IsTerminal is true if the run has reached a final state (completed, failed, cancelled, or timed out).
	IsTerminal bool

	// ErrorMessage contains error details if the run failed; empty otherwise.
	ErrorMessage string
}

// GetRunStatus retrieves the current status of an ingestion run by its ID.
func (c *Client) GetRunStatus(ctx context.Context, runID string) (*RunStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.GetRun(ctx, &ingestionv1.GetRunRequest{
		RunId: runID,
	})
	if err != nil {
		return nil, fmt.Errorf("get ingestion run: %w", err)
	}

	run := resp.GetRun()
	return &RunStatus{
		RunID:        run.GetId(),
		Status:       run.GetStatus().String(),
		IsTerminal:   isTerminalStatus(run.GetStatus()),
		ErrorMessage: run.GetErrorMessage(),
	}, nil
}

// isTerminalStatus returns true if the given run status represents a final state
// from which no further transitions are expected.
func isTerminalStatus(s ingestionv1.RunStatus) bool {
	switch s {
	case ingestionv1.RunStatus_RUN_STATUS_COMPLETED,
		ingestionv1.RunStatus_RUN_STATUS_PARTIAL,
		ingestionv1.RunStatus_RUN_STATUS_FAILED,
		ingestionv1.RunStatus_RUN_STATUS_CANCELLED,
		ingestionv1.RunStatus_RUN_STATUS_TIMEOUT:
		return true
	default:
		return false
	}
}

// hashFromURL generates a deterministic SHA-256 hash from a URL string.
// Used as file_content_hash placeholder when only a URL is available.
func hashFromURL(url string) string {
	h := sha256.Sum256([]byte(url))
	return fmt.Sprintf("%x", h)
}
