// Package ingestion provides a gRPC client for the Ingestion Service.
package ingestion

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

	// TLS enables TLS for the gRPC connection using system CA certificates.
	// When false (the default), an insecure connection is used.
	TLS bool
}

// NewClient creates a new ingestion service client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("ingestion service address is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	var transportCreds credentials.TransportCredentials
	if cfg.TLS {
		transportCreds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		transportCreds = insecure.NewCredentials()
	}

	conn, err := grpc.NewClient(cfg.Address,
		grpc.WithTransportCredentials(transportCreds),
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

	// FileID is the file_service UUID (optional, returned by StartIngestionWithContent).
	FileID string
}

// StartIngestionWithContentRequest holds the parameters for streaming ingestion.
type StartIngestionWithContentRequest struct {
	// OrgID is the organization identifier.
	OrgID string

	// ProjectID is the project identifier.
	ProjectID string

	// IdempotencyKey ensures duplicate submissions are handled gracefully.
	IdempotencyKey string

	// MimeType is the content type (typically "application/pdf").
	MimeType string

	// ContentHash is the SHA-256 hex digest of the PDF content.
	ContentHash string

	// FileSize is the size of the content in bytes.
	FileSize int64

	// SourceKind identifies the source (typically "literature_review").
	SourceKind string

	// Filename is the original filename for file_service metadata.
	Filename string

	// Content is the PDF bytes to stream.
	Content []byte
}

// StartIngestionWithContentResult holds the result of streaming ingestion.
type StartIngestionWithContentResult struct {
	// RunID is the ingestion run identifier.
	RunID string

	// FileID is the file_service file UUID.
	FileID string

	// Status is the current status string.
	Status string

	// IsExisting is true if this was an idempotent hit.
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
		FileContentHash: hashURL(req.PDFURL),
		SourceKind:      "literature_review",
	})
	if err != nil {
		return nil, fmt.Errorf("start ingestion: %w", err)
	}

	return &StartIngestionResult{
		RunID:      resp.GetRunId(),
		Status:     resp.GetStatus().String(),
		IsExisting: resp.GetIsExisting(),
		FileID:     resp.GetFileId(),
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

// streamingChunkSize is the size of each chunk sent over the streaming RPC.
// 32KB is chosen to balance throughput and memory usage.
const streamingChunkSize = 32 * 1024

// streamingTimeout is the timeout for streaming uploads (5 minutes to handle large PDFs).
const streamingTimeout = 5 * time.Minute

// maxContentSize is the maximum allowed content size for streaming uploads (100MB).
const maxContentSize = 100 * 1024 * 1024

// StartIngestionWithContent initiates an ingestion run by streaming file content directly.
// This method uploads the PDF content to file_service via the ingestion service and returns
// the file_id along with the run details. The content is streamed in 32KB chunks.
//
// The first message sent contains metadata (org, project, idempotency key, etc.).
// Subsequent messages contain the file content in chunks.
func (c *Client) StartIngestionWithContent(ctx context.Context, req StartIngestionWithContentRequest) (*StartIngestionWithContentResult, error) {
	// Validate required fields
	if req.OrgID == "" {
		return nil, fmt.Errorf("OrgID is required")
	}
	if req.ProjectID == "" {
		return nil, fmt.Errorf("ProjectID is required")
	}
	if req.IdempotencyKey == "" {
		return nil, fmt.Errorf("IdempotencyKey is required")
	}
	if req.MimeType == "" {
		return nil, fmt.Errorf("MimeType is required")
	}
	if req.ContentHash == "" {
		return nil, fmt.Errorf("ContentHash is required")
	}

	// Validate ContentHash format (must be 64-char lowercase hex SHA-256)
	if len(req.ContentHash) != 64 {
		return nil, fmt.Errorf("ContentHash must be 64-character hex SHA-256 digest")
	}

	// Validate FileSize matches Content length
	if req.FileSize != int64(len(req.Content)) {
		return nil, fmt.Errorf("FileSize (%d) does not match Content length (%d)", req.FileSize, len(req.Content))
	}

	// Validate content size limit
	if len(req.Content) > maxContentSize {
		return nil, fmt.Errorf("Content exceeds maximum size of %d bytes", maxContentSize)
	}

	// Use a longer timeout for streaming uploads
	ctx, cancel := context.WithTimeout(ctx, streamingTimeout)
	defer cancel()

	md := metadata.Pairs(
		"x-org-id", req.OrgID,
		"x-project-id", req.ProjectID,
	)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// Open streaming RPC
	stream, err := c.client.StartIngestionWithContent(ctx)
	if err != nil {
		return nil, fmt.Errorf("open ingestion stream: %w", err)
	}

	// Send metadata message first
	metaMsg := &ingestionv1.StartIngestionWithContentRequest{
		Payload: &ingestionv1.StartIngestionWithContentRequest_Metadata{
			Metadata: &ingestionv1.StartIngestionContentMetadata{
				OrgId:           req.OrgID,
				ProjectId:       req.ProjectID,
				IdempotencyKey:  req.IdempotencyKey,
				MimeType:        req.MimeType,
				FileContentHash: req.ContentHash,
				FileSize:        req.FileSize,
				SourceKind:      req.SourceKind,
				Filename:        req.Filename,
			},
		},
	}
	if err := stream.Send(metaMsg); err != nil {
		return nil, fmt.Errorf("send metadata: %w", err)
	}

	// Stream content in chunks
	content := req.Content
	for len(content) > 0 {
		end := streamingChunkSize
		if end > len(content) {
			end = len(content)
		}
		chunk := content[:end]
		content = content[end:]

		chunkMsg := &ingestionv1.StartIngestionWithContentRequest{
			Payload: &ingestionv1.StartIngestionWithContentRequest_ChunkData{
				ChunkData: chunk,
			},
		}
		if err := stream.Send(chunkMsg); err != nil {
			return nil, fmt.Errorf("send chunk: %w", err)
		}
	}

	// Close send and receive response
	resp, err := stream.CloseAndRecv()
	if err != nil {
		return nil, fmt.Errorf("close and recv: %w", err)
	}

	return &StartIngestionWithContentResult{
		RunID:      resp.GetRunId(),
		FileID:     resp.GetFileId(),
		Status:     resp.GetStatus().String(),
		IsExisting: resp.GetIsExisting(),
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

// hashURL computes a deterministic SHA-256 hex digest of the given URL string.
// Note: this hashes the URL itself, not the content at that URL. It is used as a
// placeholder for FileContentHash when the actual file bytes are not yet available.
func hashURL(url string) string {
	h := sha256.Sum256([]byte(url))
	return fmt.Sprintf("%x", h)
}
