// Package server implements the gRPC handlers for the Literature Review Service.
//
// The server layer translates between protobuf request/response types and internal
// domain types, delegates business operations to Temporal workflows, and queries
// repositories for read operations. All handlers enforce multi-tenancy via org_id
// and project_id validation, and map domain errors to appropriate gRPC status codes.
package server

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcauth "github.com/helixir/grpcauth"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
	"github.com/helixir/literature-review-service/internal/temporal"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
)

// Pagination constants for list endpoints.
const (
	defaultPageSize = 50
	maxPageSize     = 100
	maxQueryLength  = 10000
)

// WorkflowClient defines the interface for workflow operations used by the server.
// This decouples the server layer from the concrete temporal.ReviewWorkflowClient,
// enabling straightforward testing with mock implementations.
type WorkflowClient interface {
	StartReviewWorkflow(ctx context.Context, req temporal.ReviewWorkflowRequest, workflowFunc interface{}, input interface{}) (workflowID, runID string, err error)
	SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error
	QueryWorkflow(ctx context.Context, workflowID, runID, queryType string, result interface{}, args ...interface{}) error
}

// LiteratureReviewServer implements the gRPC LiteratureReviewService.
type LiteratureReviewServer struct {
	pb.UnimplementedLiteratureReviewServiceServer

	workflowClient WorkflowClient
	workflowFunc   interface{} // The Temporal workflow function reference.
	reviewRepo     repository.ReviewRepository
	paperRepo      repository.PaperRepository
	keywordRepo    repository.KeywordRepository
}

// NewLiteratureReviewServer creates a new LiteratureReviewServer with all required dependencies.
// workflowFunc is the Temporal workflow function reference (e.g., workflows.LiteratureReviewWorkflow)
// that will be passed to StartReviewWorkflow. This avoids a direct import of the workflows package.
func NewLiteratureReviewServer(
	workflowClient WorkflowClient,
	workflowFunc interface{},
	reviewRepo repository.ReviewRepository,
	paperRepo repository.PaperRepository,
	keywordRepo repository.KeywordRepository,
) *LiteratureReviewServer {
	return &LiteratureReviewServer{
		workflowClient: workflowClient,
		workflowFunc:   workflowFunc,
		reviewRepo:     reviewRepo,
		paperRepo:      paperRepo,
		keywordRepo:    keywordRepo,
	}
}

// domainErrToGRPC maps domain and temporal errors to appropriate gRPC status codes.
// It preserves the original error message in the gRPC status.
func domainErrToGRPC(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "resource not found")
	case errors.Is(err, domain.ErrInvalidInput):
		// Extract user-facing message from ValidationError if available;
		// otherwise use a generic message to avoid leaking internal details.
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			return status.Error(codes.InvalidArgument, ve.Error())
		}
		return status.Error(codes.InvalidArgument, "invalid input")
	case errors.Is(err, domain.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, "resource already exists")
	case errors.Is(err, domain.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, "unauthorized")
	case errors.Is(err, domain.ErrForbidden):
		return status.Error(codes.PermissionDenied, "forbidden")
	case errors.Is(err, domain.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "rate limit exceeded")
	case errors.Is(err, domain.ErrServiceUnavailable):
		return status.Error(codes.Unavailable, "service temporarily unavailable")
	case errors.Is(err, domain.ErrCancelled):
		return status.Error(codes.Canceled, "request cancelled")
	case errors.Is(err, temporal.ErrWorkflowNotFound):
		return status.Error(codes.NotFound, "resource not found")
	case errors.Is(err, temporal.ErrWorkflowAlreadyStarted):
		return status.Error(codes.AlreadyExists, "resource already exists")
	default:
		// Do not leak internal error details to clients.
		return status.Error(codes.Internal, "internal server error")
	}
}

// validateOrgProject validates that both org_id and project_id are non-empty.
func validateOrgProject(orgID, projectID string) error {
	if orgID == "" {
		return status.Error(codes.InvalidArgument, "org_id is required")
	}
	if projectID == "" {
		return status.Error(codes.InvalidArgument, "project_id is required")
	}
	return nil
}

// validateTenantAccess checks that the authenticated user has access to the
// requested org_id and project_id. When an auth context is present (auth is enabled),
// the user must be authenticated and have the correct org/project membership.
// When no auth context exists (auth disabled or service-to-service call), this is a no-op.
func validateTenantAccess(ctx context.Context, orgID, projectID string) error {
	authCtx, ok := grpcauth.AuthFromContext(ctx)
	if !ok || authCtx == nil {
		// No auth context — auth disabled or service-to-service call.
		return nil
	}

	// Auth context present but no user means authentication is enabled but
	// the request is unauthenticated — deny access.
	if authCtx.User == nil {
		return status.Error(codes.Unauthenticated, "authentication required")
	}

	user := authCtx.User

	if !user.HasOrgAccess(orgID) {
		return status.Error(codes.PermissionDenied, "permission denied")
	}

	if projectID != "" {
		if !user.IsOrgAdmin() && len(user.GetProjectRoles(projectID)) == 0 {
			return status.Error(codes.PermissionDenied, "permission denied")
		}
	}

	return nil
}

// validateUUID parses and validates a UUID string. Returns a descriptive error
// referencing fieldName if the string is not a valid UUID. The parse error
// details are not included to avoid echoing potentially malicious input.
func validateUUID(s, fieldName string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Errorf(codes.InvalidArgument, "%s must be a valid UUID", fieldName)
	}
	return id, nil
}

// parsePagination extracts limit and offset from page_size and page_token.
// It applies default and maximum bounds to the page size.
func parsePagination(pageSize int32, pageToken string) (limit, offset int) {
	limit = int(pageSize)
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	if pageToken != "" {
		decoded, err := base64.StdEncoding.DecodeString(pageToken)
		if err == nil {
			parsed, parseErr := strconv.Atoi(string(decoded))
			if parseErr == nil && parsed > 0 {
				offset = parsed
			}
		}
	}

	return limit, offset
}

// encodePageToken encodes the next offset as a base64 page token.
// Returns an empty string if there are no more results.
func encodePageToken(offset, limit int, totalCount int64) string {
	if offset+limit < int(totalCount) {
		return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset + limit)))
	}
	return ""
}
