package observability

import (
	"context"
)

// Context keys for observability data.
type contextKey string

const (
	requestIDKey  contextKey = "request_id"
	orgIDKey      contextKey = "org_id"
	projectIDKey  contextKey = "project_id"
	traceIDKey    contextKey = "trace_id"
	spanIDKey     contextKey = "span_id"
	workflowIDKey contextKey = "workflow_id"
	runIDKey      contextKey = "workflow_run_id"
)

// WithRequestID adds a request ID to the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromContext retrieves the request ID from context.
// Returns empty string if not present.
func RequestIDFromContext(ctx context.Context) string {
	if v := ctx.Value(requestIDKey); v != nil {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// WithOrgProject adds organization and project IDs to the context.
func WithOrgProject(ctx context.Context, orgID, projectID string) context.Context {
	ctx = context.WithValue(ctx, orgIDKey, orgID)
	ctx = context.WithValue(ctx, projectIDKey, projectID)
	return ctx
}

// OrgProjectFromContext retrieves organization and project IDs from context.
// Returns empty strings if not present.
func OrgProjectFromContext(ctx context.Context) (orgID, projectID string) {
	if v := ctx.Value(orgIDKey); v != nil {
		if id, ok := v.(string); ok {
			orgID = id
		}
	}
	if v := ctx.Value(projectIDKey); v != nil {
		if id, ok := v.(string); ok {
			projectID = id
		}
	}
	return orgID, projectID
}

// WithTraceSpan adds trace and span IDs to the context.
func WithTraceSpan(ctx context.Context, traceID, spanID string) context.Context {
	ctx = context.WithValue(ctx, traceIDKey, traceID)
	ctx = context.WithValue(ctx, spanIDKey, spanID)
	return ctx
}

// TraceSpanFromContext retrieves trace and span IDs from context.
// Returns empty strings if not present.
func TraceSpanFromContext(ctx context.Context) (traceID, spanID string) {
	if v := ctx.Value(traceIDKey); v != nil {
		if id, ok := v.(string); ok {
			traceID = id
		}
	}
	if v := ctx.Value(spanIDKey); v != nil {
		if id, ok := v.(string); ok {
			spanID = id
		}
	}
	return traceID, spanID
}

// WithWorkflow adds workflow ID and run ID to the context.
func WithWorkflow(ctx context.Context, workflowID, runID string) context.Context {
	ctx = context.WithValue(ctx, workflowIDKey, workflowID)
	ctx = context.WithValue(ctx, runIDKey, runID)
	return ctx
}

// WorkflowFromContext retrieves workflow ID and run ID from context.
// Returns empty strings if not present.
func WorkflowFromContext(ctx context.Context) (workflowID, runID string) {
	if v := ctx.Value(workflowIDKey); v != nil {
		if id, ok := v.(string); ok {
			workflowID = id
		}
	}
	if v := ctx.Value(runIDKey); v != nil {
		if id, ok := v.(string); ok {
			runID = id
		}
	}
	return workflowID, runID
}

// ReviewContext contains all the context data for a literature review.
type ReviewContext struct {
	RequestID  string
	OrgID      string
	ProjectID  string
	TraceID    string
	SpanID     string
	WorkflowID string
	RunID      string
}

// WithReviewContextFull adds all review context to the context.
func WithReviewContextFull(ctx context.Context, rc ReviewContext) context.Context {
	if rc.RequestID != "" {
		ctx = WithRequestID(ctx, rc.RequestID)
	}
	if rc.OrgID != "" || rc.ProjectID != "" {
		ctx = WithOrgProject(ctx, rc.OrgID, rc.ProjectID)
	}
	if rc.TraceID != "" || rc.SpanID != "" {
		ctx = WithTraceSpan(ctx, rc.TraceID, rc.SpanID)
	}
	if rc.WorkflowID != "" || rc.RunID != "" {
		ctx = WithWorkflow(ctx, rc.WorkflowID, rc.RunID)
	}
	return ctx
}

// ReviewContextFromContext extracts all review context from the context.
func ReviewContextFromContext(ctx context.Context) ReviewContext {
	orgID, projectID := OrgProjectFromContext(ctx)
	traceID, spanID := TraceSpanFromContext(ctx)
	workflowID, runID := WorkflowFromContext(ctx)

	return ReviewContext{
		RequestID:  RequestIDFromContext(ctx),
		OrgID:      orgID,
		ProjectID:  projectID,
		TraceID:    traceID,
		SpanID:     spanID,
		WorkflowID: workflowID,
		RunID:      runID,
	}
}
