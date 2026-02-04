package observability

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestIDContext(t *testing.T) {
	t.Run("stores and retrieves request ID", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithRequestID(ctx, "req-123")

		result := RequestIDFromContext(ctx)
		assert.Equal(t, "req-123", result)
	})

	t.Run("returns empty string when not set", func(t *testing.T) {
		ctx := context.Background()
		result := RequestIDFromContext(ctx)
		assert.Equal(t, "", result)
	})
}

func TestOrgProjectContext(t *testing.T) {
	t.Run("stores and retrieves org and project IDs", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithOrgProject(ctx, "org-456", "proj-789")

		orgID, projectID := OrgProjectFromContext(ctx)
		assert.Equal(t, "org-456", orgID)
		assert.Equal(t, "proj-789", projectID)
	})

	t.Run("returns empty strings when not set", func(t *testing.T) {
		ctx := context.Background()
		orgID, projectID := OrgProjectFromContext(ctx)
		assert.Equal(t, "", orgID)
		assert.Equal(t, "", projectID)
	})

	t.Run("handles partial values", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithOrgProject(ctx, "org-only", "")

		orgID, projectID := OrgProjectFromContext(ctx)
		assert.Equal(t, "org-only", orgID)
		assert.Equal(t, "", projectID)
	})
}

func TestTraceSpanContext(t *testing.T) {
	t.Run("stores and retrieves trace and span IDs", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithTraceSpan(ctx, "trace-abc", "span-xyz")

		traceID, spanID := TraceSpanFromContext(ctx)
		assert.Equal(t, "trace-abc", traceID)
		assert.Equal(t, "span-xyz", spanID)
	})

	t.Run("returns empty strings when not set", func(t *testing.T) {
		ctx := context.Background()
		traceID, spanID := TraceSpanFromContext(ctx)
		assert.Equal(t, "", traceID)
		assert.Equal(t, "", spanID)
	})
}

func TestWorkflowContext(t *testing.T) {
	t.Run("stores and retrieves workflow and run IDs", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithWorkflow(ctx, "wf-123", "run-456")

		workflowID, runID := WorkflowFromContext(ctx)
		assert.Equal(t, "wf-123", workflowID)
		assert.Equal(t, "run-456", runID)
	})

	t.Run("returns empty strings when not set", func(t *testing.T) {
		ctx := context.Background()
		workflowID, runID := WorkflowFromContext(ctx)
		assert.Equal(t, "", workflowID)
		assert.Equal(t, "", runID)
	})
}

func TestReviewContextFull(t *testing.T) {
	t.Run("stores and retrieves full review context", func(t *testing.T) {
		ctx := context.Background()
		rc := ReviewContext{
			RequestID:  "req-123",
			OrgID:      "org-456",
			ProjectID:  "proj-789",
			TraceID:    "trace-abc",
			SpanID:     "span-xyz",
			WorkflowID: "wf-123",
			RunID:      "run-456",
		}

		ctx = WithReviewContextFull(ctx, rc)
		result := ReviewContextFromContext(ctx)

		assert.Equal(t, rc.RequestID, result.RequestID)
		assert.Equal(t, rc.OrgID, result.OrgID)
		assert.Equal(t, rc.ProjectID, result.ProjectID)
		assert.Equal(t, rc.TraceID, result.TraceID)
		assert.Equal(t, rc.SpanID, result.SpanID)
		assert.Equal(t, rc.WorkflowID, result.WorkflowID)
		assert.Equal(t, rc.RunID, result.RunID)
	})

	t.Run("handles partial context", func(t *testing.T) {
		ctx := context.Background()
		rc := ReviewContext{
			RequestID: "req-only",
		}

		ctx = WithReviewContextFull(ctx, rc)
		result := ReviewContextFromContext(ctx)

		assert.Equal(t, "req-only", result.RequestID)
		assert.Equal(t, "", result.OrgID)
		assert.Equal(t, "", result.ProjectID)
	})

	t.Run("returns empty context when nothing set", func(t *testing.T) {
		ctx := context.Background()
		result := ReviewContextFromContext(ctx)

		assert.Equal(t, ReviewContext{}, result)
	})
}

func TestContextChaining(t *testing.T) {
	ctx := context.Background()

	// Chain multiple context additions
	ctx = WithRequestID(ctx, "req-1")
	ctx = WithOrgProject(ctx, "org-1", "proj-1")
	ctx = WithTraceSpan(ctx, "trace-1", "span-1")
	ctx = WithWorkflow(ctx, "wf-1", "run-1")

	// All values should be retrievable
	assert.Equal(t, "req-1", RequestIDFromContext(ctx))

	orgID, projectID := OrgProjectFromContext(ctx)
	assert.Equal(t, "org-1", orgID)
	assert.Equal(t, "proj-1", projectID)

	traceID, spanID := TraceSpanFromContext(ctx)
	assert.Equal(t, "trace-1", traceID)
	assert.Equal(t, "span-1", spanID)

	workflowID, runID := WorkflowFromContext(ctx)
	assert.Equal(t, "wf-1", workflowID)
	assert.Equal(t, "run-1", runID)
}

func TestContextOverwrite(t *testing.T) {
	ctx := context.Background()

	// Set initial values
	ctx = WithRequestID(ctx, "req-1")

	// Overwrite with new values
	ctx = WithRequestID(ctx, "req-2")

	// Should have new value
	assert.Equal(t, "req-2", RequestIDFromContext(ctx))
}
