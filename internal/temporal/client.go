package temporal

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"

	"github.com/helixir/literature-review-service/internal/domain"
)

// =============================================================================
// Signal and Query Names
// =============================================================================

// Signal and query names for external interaction with review workflows.
// These are defined here (not in the workflows package) so that both the
// server layer and the workflow implementation can reference them without
// creating a dependency from server -> workflows.
const (
	// SignalCancel is the signal name used to request workflow cancellation.
	SignalCancel = "cancel"

	// QueryProgress is the query name used to retrieve workflow progress.
	QueryProgress = "progress"
)

// Default timeout constants for workflow execution and health checks.
const (
	// DefaultWorkflowExecutionTimeout is the maximum time a review workflow is allowed to run.
	DefaultWorkflowExecutionTimeout = 4 * time.Hour

	// DefaultHealthCheckTimeout is the timeout for Temporal server health checks.
	DefaultHealthCheckTimeout = 5 * time.Second
)

// =============================================================================
// Sentinel Errors
// =============================================================================

var (
	// ErrWorkflowNotFound indicates the workflow execution was not found.
	ErrWorkflowNotFound = errors.New("workflow not found")

	// ErrWorkflowAlreadyStarted indicates a workflow with the same ID is already running.
	ErrWorkflowAlreadyStarted = errors.New("workflow already started")

	// ErrWorkflowAlreadyCompleted indicates the workflow has already completed.
	ErrWorkflowAlreadyCompleted = errors.New("workflow already completed")

	// ErrQueryFailed indicates the workflow query failed.
	ErrQueryFailed = errors.New("query failed")

	// ErrSignalFailed indicates the workflow signal failed.
	ErrSignalFailed = errors.New("signal failed")

	// ErrClientClosed indicates the client has been closed.
	ErrClientClosed = errors.New("client closed")

	// ErrConnectionFailed indicates a connection failure to the Temporal server.
	ErrConnectionFailed = errors.New("connection failed")

	// ErrNamespaceNotFound indicates the namespace does not exist.
	ErrNamespaceNotFound = errors.New("namespace not found")

	// ErrPermissionDenied indicates insufficient permissions.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrInvalidArgument indicates an invalid argument was provided.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrResourceExhausted indicates resource limits have been reached.
	ErrResourceExhausted = errors.New("resource exhausted")

	// ErrDeadlineExceeded indicates the operation deadline was exceeded.
	ErrDeadlineExceeded = errors.New("deadline exceeded")
)

// =============================================================================
// Error Helpers
// =============================================================================

// TemporalError wraps a Temporal error with additional context.
type TemporalError struct {
	Op         string // Operation that failed
	Kind       error  // Category of error (sentinel)
	WorkflowID string // Workflow ID (if applicable)
	RunID      string // Run ID (if applicable)
	Err        error  // Underlying error
}

// Error returns the error message.
func (e *TemporalError) Error() string {
	msg := fmt.Sprintf("%s: %s", e.Op, e.Kind)
	if e.WorkflowID != "" {
		msg += fmt.Sprintf(" [workflowID=%s", e.WorkflowID)
		if e.RunID != "" {
			msg += fmt.Sprintf(", runID=%s", e.RunID)
		}
		msg += "]"
	}
	if e.Err != nil {
		msg += fmt.Sprintf(": %v", e.Err)
	}
	return msg
}

// Unwrap returns the underlying error.
func (e *TemporalError) Unwrap() error {
	return e.Err
}

// Is reports whether target matches this error's Kind.
func (e *TemporalError) Is(target error) bool {
	return errors.Is(e.Kind, target)
}

// wrapTemporalError converts a Temporal SDK error to a TemporalError.
func wrapTemporalError(op string, err error, workflowID, runID string) error {
	if err == nil {
		return nil
	}

	te := &TemporalError{
		Op:         op,
		WorkflowID: workflowID,
		RunID:      runID,
		Err:        err,
	}

	// Map Temporal service errors to sentinel errors
	var notFoundErr *serviceerror.NotFound
	var alreadyStartedErr *serviceerror.WorkflowExecutionAlreadyStarted
	var namespaceNotFoundErr *serviceerror.NamespaceNotFound
	var permissionDeniedErr *serviceerror.PermissionDenied
	var invalidArgumentErr *serviceerror.InvalidArgument
	var resourceExhaustedErr *serviceerror.ResourceExhausted
	var deadlineExceededErr *serviceerror.DeadlineExceeded
	var queryFailedErr *serviceerror.QueryFailed
	var unavailableErr *serviceerror.Unavailable

	switch {
	case errors.As(err, &notFoundErr):
		te.Kind = ErrWorkflowNotFound
	case errors.As(err, &alreadyStartedErr):
		te.Kind = ErrWorkflowAlreadyStarted
	case errors.As(err, &namespaceNotFoundErr):
		te.Kind = ErrNamespaceNotFound
	case errors.As(err, &permissionDeniedErr):
		te.Kind = ErrPermissionDenied
	case errors.As(err, &invalidArgumentErr):
		te.Kind = ErrInvalidArgument
	case errors.As(err, &resourceExhaustedErr):
		te.Kind = ErrResourceExhausted
	case errors.As(err, &deadlineExceededErr):
		te.Kind = ErrDeadlineExceeded
	case errors.As(err, &queryFailedErr):
		te.Kind = ErrQueryFailed
	case errors.As(err, &unavailableErr):
		te.Kind = ErrConnectionFailed
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			te.Kind = ErrDeadlineExceeded
		} else if errors.Is(err, context.Canceled) {
			te.Kind = ErrClientClosed
		} else {
			te.Kind = ErrConnectionFailed
		}
	}

	return te
}

// IsWorkflowNotFound checks if the error indicates a workflow was not found.
func IsWorkflowNotFound(err error) bool {
	return errors.Is(err, ErrWorkflowNotFound)
}

// IsWorkflowAlreadyStarted checks if the error indicates a workflow already started.
func IsWorkflowAlreadyStarted(err error) bool {
	return errors.Is(err, ErrWorkflowAlreadyStarted)
}

// IsQueryFailed checks if the error indicates a query failure.
func IsQueryFailed(err error) bool {
	return errors.Is(err, ErrQueryFailed)
}

// IsConnectionFailed checks if the error indicates a connection failure.
func IsConnectionFailed(err error) bool {
	return errors.Is(err, ErrConnectionFailed)
}

// =============================================================================
// TLS Configuration
// =============================================================================

// TLSConfig contains TLS configuration for the Temporal client.
type TLSConfig struct {
	// Enabled enables TLS for the connection.
	Enabled bool

	// CertPath is the path to the client certificate file (PEM format).
	CertPath string

	// KeyPath is the path to the client private key file (PEM format).
	KeyPath string

	// CACertPath is the path to the CA certificate file (PEM format).
	CACertPath string

	// ServerName is the expected server name for certificate verification.
	ServerName string

	// InsecureSkipVerify disables certificate verification.
	// WARNING: This should only be used for testing/development.
	InsecureSkipVerify bool
}

// buildTLSConfig creates a *tls.Config from TLSConfig.
func (t *TLSConfig) buildTLSConfig() (*tls.Config, error) {
	if !t.Enabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: t.InsecureSkipVerify,
		ServerName:         t.ServerName,
		MinVersion:         tls.VersionTLS12,
	}

	// Load client certificate if provided
	if t.CertPath != "" && t.KeyPath != "" {
		cert, err := tls.LoadX509KeyPair(t.CertPath, t.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate if provided
	if t.CACertPath != "" {
		caCert, err := os.ReadFile(t.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// =============================================================================
// Client Configuration
// =============================================================================

// ClientConfig contains configuration for the Temporal client.
type ClientConfig struct {
	// HostPort is the Temporal server address (e.g., "localhost:7233").
	HostPort string

	// Namespace is the Temporal namespace to use.
	Namespace string

	// TaskQueue is the default task queue for starting workflows.
	TaskQueue string

	// TLS contains optional TLS configuration.
	TLS *TLSConfig

	// ConnectionTimeout is the timeout for establishing the connection.
	// Defaults to 10 seconds if not set.
	ConnectionTimeout time.Duration

	// HealthCheckTimeout is the timeout for health check operations.
	// Defaults to 5 seconds if not set.
	HealthCheckTimeout time.Duration
}

// NewClient creates a new Temporal client with the given configuration.
func NewClient(cfg ClientConfig) (client.Client, error) {
	options := client.Options{
		HostPort:  cfg.HostPort,
		Namespace: cfg.Namespace,
	}

	// Configure TLS if enabled
	if cfg.TLS != nil && cfg.TLS.Enabled {
		tlsConfig, err := cfg.TLS.buildTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("configure TLS: %w", err)
		}
		options.ConnectionOptions = client.ConnectionOptions{
			TLS: tlsConfig,
		}
	}

	c, err := client.Dial(options)
	if err != nil {
		return nil, fmt.Errorf("create Temporal client: %w", err)
	}

	return c, nil
}

// =============================================================================
// Shared Workflow Input Types
// =============================================================================

// ReviewWorkflowInput contains the parameters for starting a literature review workflow.
// This type is defined in the temporal package (not in workflows) so that
// the server layer can construct workflow inputs without importing the workflows package.
type ReviewWorkflowInput struct {
	// RequestID is the unique identifier for this review request.
	RequestID uuid.UUID

	// OrgID is the organization identifier for multi-tenancy.
	OrgID string

	// ProjectID is the project identifier for multi-tenancy.
	ProjectID string

	// UserID is the user who initiated the review.
	UserID string

	// Title is the research topic title (required).
	Title string

	// Description is an expanded description of the review scope (optional).
	Description string

	// SeedKeywords are user-provided starting keywords (optional).
	SeedKeywords []string

	// Config holds the review configuration parameters.
	Config domain.ReviewConfiguration
}

// =============================================================================
// Literature Review Workflow Client
// =============================================================================

// ReviewWorkflowClient provides methods for starting and managing review workflows.
type ReviewWorkflowClient struct {
	mu                 sync.RWMutex
	client             client.Client
	taskQueue          string
	healthCheckTimeout time.Duration
	closed             bool
}

// NewReviewWorkflowClient creates a new ReviewWorkflowClient.
func NewReviewWorkflowClient(c client.Client, taskQueue string) *ReviewWorkflowClient {
	return &ReviewWorkflowClient{
		client:             c,
		taskQueue:          taskQueue,
		healthCheckTimeout: DefaultHealthCheckTimeout,
	}
}

// NewReviewWorkflowClientWithConfig creates a new ReviewWorkflowClient with full configuration.
func NewReviewWorkflowClientWithConfig(c client.Client, cfg ClientConfig) *ReviewWorkflowClient {
	healthTimeout := cfg.HealthCheckTimeout
	if healthTimeout == 0 {
		healthTimeout = DefaultHealthCheckTimeout
	}

	return &ReviewWorkflowClient{
		client:             c,
		taskQueue:          cfg.TaskQueue,
		healthCheckTimeout: healthTimeout,
	}
}

// Close closes the underlying Temporal client connection.
func (c *ReviewWorkflowClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil && !c.closed {
		c.client.Close()
		c.closed = true
	}
}

// isClosed returns whether the client has been closed. It is safe for concurrent use.
func (c *ReviewWorkflowClient) isClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// Health checks the connection health to the Temporal server.
func (c *ReviewWorkflowClient) Health(ctx context.Context) error {
	if c.isClosed() {
		return &TemporalError{
			Op:   "Health",
			Kind: ErrClientClosed,
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, c.healthCheckTimeout)
	defer cancel()

	_, err := c.client.CheckHealth(checkCtx, &client.CheckHealthRequest{})
	if err != nil {
		return wrapTemporalError("Health", err, "", "")
	}

	return nil
}

// ReviewWorkflowRequest contains the parameters for starting a review workflow.
type ReviewWorkflowRequest struct {
	// RequestID is the literature review request ID.
	RequestID string
	// OrgID is the organization identifier.
	OrgID string
	// ProjectID is the project identifier.
	ProjectID string
	// Title is the user's research topic title.
	Title string
	// MaxPapers is the maximum number of papers to retrieve.
	MaxPapers int
	// MaxDepth is the maximum recursive search depth.
	MaxDepth int
	// Sources is the list of paper sources to search.
	Sources []string
}

// StartReviewWorkflow starts a new literature review workflow.
// The workflow function must be registered with the worker separately.
func (c *ReviewWorkflowClient) StartReviewWorkflow(ctx context.Context, req ReviewWorkflowRequest, workflowFunc interface{}, input interface{}) (workflowID, runID string, err error) {
	if c.isClosed() {
		return "", "", &TemporalError{
			Op:   "StartReviewWorkflow",
			Kind: ErrClientClosed,
		}
	}

	workflowID = fmt.Sprintf("review-%s", req.RequestID)
	options := client.StartWorkflowOptions{
		ID:                       workflowID,
		TaskQueue:                c.taskQueue,
		WorkflowExecutionTimeout: DefaultWorkflowExecutionTimeout,
	}

	run, err := c.client.ExecuteWorkflow(ctx, options, workflowFunc, input)
	if err != nil {
		return "", "", wrapTemporalError("StartReviewWorkflow", err, workflowID, "")
	}

	return workflowID, run.GetRunID(), nil
}

// CancelWorkflow cancels a running workflow.
func (c *ReviewWorkflowClient) CancelWorkflow(ctx context.Context, workflowID, runID string) error {
	if c.isClosed() {
		return &TemporalError{
			Op:         "CancelWorkflow",
			Kind:       ErrClientClosed,
			WorkflowID: workflowID,
			RunID:      runID,
		}
	}

	err := c.client.CancelWorkflow(ctx, workflowID, runID)
	if err != nil {
		return wrapTemporalError("CancelWorkflow", err, workflowID, runID)
	}
	return nil
}

// GetWorkflowResult waits for a workflow to complete and returns the result.
func (c *ReviewWorkflowClient) GetWorkflowResult(ctx context.Context, workflowID, runID string, result interface{}) error {
	if c.isClosed() {
		return &TemporalError{
			Op:         "GetWorkflowResult",
			Kind:       ErrClientClosed,
			WorkflowID: workflowID,
			RunID:      runID,
		}
	}

	run := c.client.GetWorkflow(ctx, workflowID, runID)

	if err := run.Get(ctx, result); err != nil {
		return wrapTemporalError("GetWorkflowResult", err, workflowID, runID)
	}

	return nil
}

// WorkflowDescription contains information about a workflow execution.
type WorkflowDescription struct {
	// WorkflowID is the workflow identifier.
	WorkflowID string
	// RunID is the workflow run identifier.
	RunID string
	// Status is the workflow execution status.
	Status string
	// StartTime is when the workflow started.
	StartTime time.Time
	// CloseTime is when the workflow completed (nil if still running).
	CloseTime *time.Time
}

// DescribeWorkflow returns information about a workflow execution.
func (c *ReviewWorkflowClient) DescribeWorkflow(ctx context.Context, workflowID, runID string) (*WorkflowDescription, error) {
	if c.isClosed() {
		return nil, &TemporalError{
			Op:         "DescribeWorkflow",
			Kind:       ErrClientClosed,
			WorkflowID: workflowID,
			RunID:      runID,
		}
	}

	resp, err := c.client.DescribeWorkflowExecution(ctx, workflowID, runID)
	if err != nil {
		return nil, wrapTemporalError("DescribeWorkflow", err, workflowID, runID)
	}

	desc := &WorkflowDescription{
		WorkflowID: workflowID,
		RunID:      resp.WorkflowExecutionInfo.Execution.RunId,
		Status:     resp.WorkflowExecutionInfo.Status.String(),
		StartTime:  resp.WorkflowExecutionInfo.StartTime.AsTime(),
	}

	if resp.WorkflowExecutionInfo.CloseTime != nil {
		closeTime := resp.WorkflowExecutionInfo.CloseTime.AsTime()
		desc.CloseTime = &closeTime
	}

	return desc, nil
}

// SignalWorkflow sends a signal to a running workflow.
func (c *ReviewWorkflowClient) SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error {
	if c.isClosed() {
		return &TemporalError{
			Op:         "SignalWorkflow",
			Kind:       ErrClientClosed,
			WorkflowID: workflowID,
			RunID:      runID,
		}
	}

	err := c.client.SignalWorkflow(ctx, workflowID, runID, signalName, arg)
	if err != nil {
		return wrapTemporalError("SignalWorkflow", err, workflowID, runID)
	}

	return nil
}

// QueryWorkflow queries a running workflow's state.
func (c *ReviewWorkflowClient) QueryWorkflow(ctx context.Context, workflowID, runID, queryType string, result interface{}, args ...interface{}) error {
	if c.isClosed() {
		return &TemporalError{
			Op:         "QueryWorkflow",
			Kind:       ErrClientClosed,
			WorkflowID: workflowID,
			RunID:      runID,
		}
	}

	resp, err := c.client.QueryWorkflow(ctx, workflowID, runID, queryType, args...)
	if err != nil {
		return wrapTemporalError("QueryWorkflow", err, workflowID, runID)
	}

	if result != nil {
		if err := resp.Get(result); err != nil {
			return &TemporalError{
				Op:         "QueryWorkflow",
				Kind:       ErrQueryFailed,
				WorkflowID: workflowID,
				RunID:      runID,
				Err:        fmt.Errorf("decode query result: %w", err),
			}
		}
	}

	return nil
}

// Client returns the underlying Temporal client for advanced operations.
func (c *ReviewWorkflowClient) Client() client.Client {
	return c.client
}

// TaskQueue returns the configured task queue name.
func (c *ReviewWorkflowClient) TaskQueue() string {
	return c.taskQueue
}
