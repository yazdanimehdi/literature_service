package temporal

import (
	"context"
	"errors"
	"testing"
	"time"

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

	t.Run("Error with workflowID but no runID", func(t *testing.T) {
		err := &TemporalError{
			Op:         "StartReviewWorkflow",
			Kind:       ErrWorkflowAlreadyStarted,
			WorkflowID: "wf-only",
		}

		msg := err.Error()
		assert.Contains(t, msg, "StartReviewWorkflow")
		assert.Contains(t, msg, "workflow already started")
		assert.Contains(t, msg, "wf-only")
		assert.NotContains(t, msg, "runID")
	})

	t.Run("Error with nil underlying error", func(t *testing.T) {
		err := &TemporalError{
			Op:   "Health",
			Kind: ErrClientClosed,
			Err:  nil,
		}

		msg := err.Error()
		assert.Contains(t, msg, "Health")
		assert.Contains(t, msg, "client closed")
		assert.NotContains(t, msg, ": <nil>")
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

	t.Run("wraps NamespaceNotFound error", func(t *testing.T) {
		nsErr := serviceerror.NewNamespaceNotFound("test-namespace")
		result := wrapTemporalError("Test", nsErr, "", "")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrNamespaceNotFound, te.Kind)
		assert.Equal(t, "Test", te.Op)
		assert.ErrorIs(t, te, ErrNamespaceNotFound)
	})

	t.Run("wraps PermissionDenied error", func(t *testing.T) {
		permErr := serviceerror.NewPermissionDenied("access denied", "missing role")
		result := wrapTemporalError("Test", permErr, "wf-1", "")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrPermissionDenied, te.Kind)
		assert.Equal(t, "wf-1", te.WorkflowID)
	})

	t.Run("wraps QueryFailed error", func(t *testing.T) {
		queryErr := serviceerror.NewQueryFailed("query failed")
		result := wrapTemporalError("QueryWorkflow", queryErr, "wf-1", "run-1")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrQueryFailed, te.Kind)
		assert.Equal(t, "QueryWorkflow", te.Op)
		assert.Equal(t, "wf-1", te.WorkflowID)
		assert.Equal(t, "run-1", te.RunID)
	})

	t.Run("wraps ResourceExhausted error", func(t *testing.T) {
		resErr := &serviceerror.ResourceExhausted{
			Message: "rate limit exceeded",
		}
		result := wrapTemporalError("Test", resErr, "", "")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrResourceExhausted, te.Kind)
	})

	t.Run("wraps InvalidArgument error", func(t *testing.T) {
		invalidErr := serviceerror.NewInvalidArgument("bad argument")
		result := wrapTemporalError("Test", invalidErr, "", "")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrInvalidArgument, te.Kind)
	})

	t.Run("wraps DeadlineExceeded service error", func(t *testing.T) {
		dlErr := serviceerror.NewDeadlineExceeded("timed out")
		result := wrapTemporalError("Test", dlErr, "", "")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrDeadlineExceeded, te.Kind)
	})

	t.Run("wraps Unavailable error as connection failed", func(t *testing.T) {
		unavailErr := serviceerror.NewUnavailable("server unavailable")
		result := wrapTemporalError("Test", unavailErr, "", "")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, ErrConnectionFailed, te.Kind)
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

	t.Run("preserves workflow ID and run ID", func(t *testing.T) {
		err := errors.New("some error")
		result := wrapTemporalError("StartReviewWorkflow", err, "wf-abc", "run-xyz")

		var te *TemporalError
		require.True(t, errors.As(result, &te))
		assert.Equal(t, "StartReviewWorkflow", te.Op)
		assert.Equal(t, "wf-abc", te.WorkflowID)
		assert.Equal(t, "run-xyz", te.RunID)
		assert.Equal(t, err, te.Err)
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

func TestNewReviewWorkflowClient(t *testing.T) {
	t.Run("sets task queue and default health check timeout", func(t *testing.T) {
		rc := NewReviewWorkflowClient(nil, "my-task-queue")

		require.NotNil(t, rc)
		assert.Equal(t, "my-task-queue", rc.taskQueue)
		assert.Equal(t, 5*time.Second, rc.healthCheckTimeout)
		assert.Nil(t, rc.client)
		assert.False(t, rc.closed)
	})

	t.Run("TaskQueue returns configured value", func(t *testing.T) {
		rc := NewReviewWorkflowClient(nil, "literature-review-queue")

		assert.Equal(t, "literature-review-queue", rc.TaskQueue())
	})

	t.Run("Client returns underlying client", func(t *testing.T) {
		rc := NewReviewWorkflowClient(nil, "queue")

		assert.Nil(t, rc.Client())
	})
}

func TestNewReviewWorkflowClientWithConfig(t *testing.T) {
	t.Run("uses config values", func(t *testing.T) {
		cfg := ClientConfig{
			TaskQueue:          "custom-queue",
			HealthCheckTimeout: 10 * time.Second,
		}
		rc := NewReviewWorkflowClientWithConfig(nil, cfg)

		require.NotNil(t, rc)
		assert.Equal(t, "custom-queue", rc.taskQueue)
		assert.Equal(t, 10*time.Second, rc.healthCheckTimeout)
		assert.Nil(t, rc.client)
		assert.False(t, rc.closed)
	})

	t.Run("defaults health check timeout to 5s when zero", func(t *testing.T) {
		cfg := ClientConfig{
			TaskQueue:          "another-queue",
			HealthCheckTimeout: 0,
		}
		rc := NewReviewWorkflowClientWithConfig(nil, cfg)

		require.NotNil(t, rc)
		assert.Equal(t, 5*time.Second, rc.healthCheckTimeout)
	})

	t.Run("TaskQueue returns configured value from config", func(t *testing.T) {
		cfg := ClientConfig{
			TaskQueue: "config-queue",
		}
		rc := NewReviewWorkflowClientWithConfig(nil, cfg)

		assert.Equal(t, "config-queue", rc.TaskQueue())
	})

	t.Run("Client returns the provided client", func(t *testing.T) {
		cfg := ClientConfig{
			TaskQueue: "queue",
		}
		rc := NewReviewWorkflowClientWithConfig(nil, cfg)

		assert.Nil(t, rc.Client())
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
