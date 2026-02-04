package temporal

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/serviceerror"
)

func TestTemporalError(t *testing.T) {
	t.Run("Error includes all fields", func(t *testing.T) {
		err := &TemporalError{
			Op:         "StartWorkflow",
			Kind:       ErrWorkflowNotFound,
			WorkflowID: "wf-123",
			RunID:      "run-456",
			Err:        errors.New("underlying error"),
		}

		msg := err.Error()
		assert.Contains(t, msg, "StartWorkflow")
		assert.Contains(t, msg, "workflow not found")
		assert.Contains(t, msg, "wf-123")
		assert.Contains(t, msg, "run-456")
		assert.Contains(t, msg, "underlying error")
	})

	t.Run("Error without workflow IDs", func(t *testing.T) {
		err := &TemporalError{
			Op:   "Health",
			Kind: ErrConnectionFailed,
		}

		msg := err.Error()
		assert.Contains(t, msg, "Health")
		assert.Contains(t, msg, "connection failed")
		assert.NotContains(t, msg, "workflowID")
	})

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		underlying := errors.New("underlying")
		err := &TemporalError{
			Op:   "Test",
			Kind: ErrConnectionFailed,
			Err:  underlying,
		}

		assert.Equal(t, underlying, err.Unwrap())
	})

	t.Run("Is matches Kind", func(t *testing.T) {
		err := &TemporalError{
			Op:   "Test",
			Kind: ErrWorkflowNotFound,
		}

		assert.True(t, errors.Is(err, ErrWorkflowNotFound))
		assert.False(t, errors.Is(err, ErrConnectionFailed))
	})
}

func TestWrapTemporalError(t *testing.T) {
	t.Run("returns nil for nil error", func(t *testing.T) {
		result := wrapTemporalError("Test", nil, "", "")
		assert.Nil(t, result)
	})

	t.Run("wraps NotFound error", func(t *testing.T) {
		notFoundErr := serviceerror.NewNotFound("not found")
		result := wrapTemporalError("Test", notFoundErr, "wf-1", "run-1")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrWorkflowNotFound, te.Kind)
	})

	t.Run("wraps WorkflowExecutionAlreadyStarted error", func(t *testing.T) {
		alreadyStartedErr := serviceerror.NewWorkflowExecutionAlreadyStarted("already started", "", "")
		result := wrapTemporalError("Test", alreadyStartedErr, "wf-1", "")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrWorkflowAlreadyStarted, te.Kind)
	})

	t.Run("wraps context.DeadlineExceeded", func(t *testing.T) {
		result := wrapTemporalError("Test", context.DeadlineExceeded, "", "")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrDeadlineExceeded, te.Kind)
	})

	t.Run("wraps context.Canceled", func(t *testing.T) {
		result := wrapTemporalError("Test", context.Canceled, "", "")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrClientClosed, te.Kind)
	})

	t.Run("wraps unknown error as connection failed", func(t *testing.T) {
		unknownErr := errors.New("unknown error")
		result := wrapTemporalError("Test", unknownErr, "", "")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrConnectionFailed, te.Kind)
	})
}

func TestErrorCheckers(t *testing.T) {
	t.Run("IsWorkflowNotFound", func(t *testing.T) {
		err := &TemporalError{Kind: ErrWorkflowNotFound}
		assert.True(t, IsWorkflowNotFound(err))
		assert.False(t, IsWorkflowNotFound(errors.New("other")))
	})

	t.Run("IsWorkflowAlreadyStarted", func(t *testing.T) {
		err := &TemporalError{Kind: ErrWorkflowAlreadyStarted}
		assert.True(t, IsWorkflowAlreadyStarted(err))
		assert.False(t, IsWorkflowAlreadyStarted(errors.New("other")))
	})

	t.Run("IsQueryFailed", func(t *testing.T) {
		err := &TemporalError{Kind: ErrQueryFailed}
		assert.True(t, IsQueryFailed(err))
		assert.False(t, IsQueryFailed(errors.New("other")))
	})

	t.Run("IsConnectionFailed", func(t *testing.T) {
		err := &TemporalError{Kind: ErrConnectionFailed}
		assert.True(t, IsConnectionFailed(err))
		assert.False(t, IsConnectionFailed(errors.New("other")))
	})
}

func TestTLSConfig(t *testing.T) {
	t.Run("returns nil when not enabled", func(t *testing.T) {
		cfg := &TLSConfig{Enabled: false}
		tlsCfg, err := cfg.buildTLSConfig()
		require.NoError(t, err)
		assert.Nil(t, tlsCfg)
	})

	t.Run("builds config with basic settings", func(t *testing.T) {
		cfg := &TLSConfig{
			Enabled:            true,
			ServerName:         "test.example.com",
			InsecureSkipVerify: true,
		}
		tlsCfg, err := cfg.buildTLSConfig()
		require.NoError(t, err)
		require.NotNil(t, tlsCfg)
		assert.Equal(t, "test.example.com", tlsCfg.ServerName)
		assert.True(t, tlsCfg.InsecureSkipVerify)
	})

	t.Run("errors on invalid cert path", func(t *testing.T) {
		cfg := &TLSConfig{
			Enabled:  true,
			CertPath: "/nonexistent/cert.pem",
			KeyPath:  "/nonexistent/key.pem",
		}
		_, err := cfg.buildTLSConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "load client certificate")
	})

	t.Run("errors on invalid CA cert path", func(t *testing.T) {
		cfg := &TLSConfig{
			Enabled:    true,
			CACertPath: "/nonexistent/ca.pem",
		}
		_, err := cfg.buildTLSConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read CA certificate")
	})
}

func TestClientConfig(t *testing.T) {
	t.Run("stores all fields", func(t *testing.T) {
		cfg := ClientConfig{
			HostPort:  "localhost:7233",
			Namespace: "test-namespace",
			TaskQueue: "test-queue",
		}

		assert.Equal(t, "localhost:7233", cfg.HostPort)
		assert.Equal(t, "test-namespace", cfg.Namespace)
		assert.Equal(t, "test-queue", cfg.TaskQueue)
	})
}

func TestReviewWorkflowClient(t *testing.T) {
	t.Run("NewReviewWorkflowClient sets defaults", func(t *testing.T) {
		// We can't easily mock client.Client, so just test the struct creation
		rc := &ReviewWorkflowClient{
			taskQueue:          "test-queue",
			healthCheckTimeout: 0,
		}

		// Verify TaskQueue getter works
		assert.Equal(t, "test-queue", rc.TaskQueue())
	})

	t.Run("closed client returns error on Health", func(t *testing.T) {
		rc := &ReviewWorkflowClient{
			closed: true,
		}

		err := rc.Health(context.Background())
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrClientClosed))
	})

	t.Run("closed client returns error on CancelWorkflow", func(t *testing.T) {
		rc := &ReviewWorkflowClient{
			closed: true,
		}

		err := rc.CancelWorkflow(context.Background(), "wf-1", "run-1")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrClientClosed))
	})

	t.Run("closed client returns error on GetWorkflowResult", func(t *testing.T) {
		rc := &ReviewWorkflowClient{
			closed: true,
		}

		var result interface{}
		err := rc.GetWorkflowResult(context.Background(), "wf-1", "run-1", &result)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrClientClosed))
	})

	t.Run("closed client returns error on DescribeWorkflow", func(t *testing.T) {
		rc := &ReviewWorkflowClient{
			closed: true,
		}

		_, err := rc.DescribeWorkflow(context.Background(), "wf-1", "run-1")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrClientClosed))
	})

	t.Run("closed client returns error on SignalWorkflow", func(t *testing.T) {
		rc := &ReviewWorkflowClient{
			closed: true,
		}

		err := rc.SignalWorkflow(context.Background(), "wf-1", "run-1", "signal", nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrClientClosed))
	})

	t.Run("closed client returns error on QueryWorkflow", func(t *testing.T) {
		rc := &ReviewWorkflowClient{
			closed: true,
		}

		var result interface{}
		err := rc.QueryWorkflow(context.Background(), "wf-1", "run-1", "query", &result)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrClientClosed))
	})

	t.Run("closed client returns error on StartReviewWorkflow", func(t *testing.T) {
		rc := &ReviewWorkflowClient{
			closed: true,
		}

		req := ReviewWorkflowRequest{RequestID: "req-1"}
		_, _, err := rc.StartReviewWorkflow(context.Background(), req, nil, nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrClientClosed))
	})

	t.Run("Close is idempotent", func(t *testing.T) {
		rc := &ReviewWorkflowClient{
			closed: false,
			client: nil, // nil client means Close won't actually close anything
		}

		// With nil client, Close should not panic but also won't set closed=true
		// because the condition is "client != nil && !closed"
		rc.Close()

		// Second close should not panic
		rc.Close()
	})
}

func TestReviewWorkflowRequest(t *testing.T) {
	t.Run("stores all fields", func(t *testing.T) {
		req := ReviewWorkflowRequest{
			RequestID: "req-123",
			OrgID:     "org-456",
			ProjectID: "proj-789",
			Query:     "machine learning papers",
			MaxPapers: 100,
			MaxDepth:  3,
			Sources:   []string{"semantic_scholar", "openalex"},
		}

		assert.Equal(t, "req-123", req.RequestID)
		assert.Equal(t, "org-456", req.OrgID)
		assert.Equal(t, "proj-789", req.ProjectID)
		assert.Equal(t, "machine learning papers", req.Query)
		assert.Equal(t, 100, req.MaxPapers)
		assert.Equal(t, 3, req.MaxDepth)
		assert.Equal(t, []string{"semantic_scholar", "openalex"}, req.Sources)
	})
}

func TestWorkflowDescription(t *testing.T) {
	t.Run("stores all fields", func(t *testing.T) {
		desc := WorkflowDescription{
			WorkflowID: "wf-123",
			RunID:      "run-456",
			Status:     "RUNNING",
		}

		assert.Equal(t, "wf-123", desc.WorkflowID)
		assert.Equal(t, "run-456", desc.RunID)
		assert.Equal(t, "RUNNING", desc.Status)
		assert.Nil(t, desc.CloseTime)
	})
}
