package server

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
	"github.com/helixir/literature-review-service/internal/temporal"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
)

// LiteratureReviewServer implements the gRPC LiteratureReviewService.
type LiteratureReviewServer struct {
	pb.UnimplementedLiteratureReviewServiceServer

	workflowClient *temporal.ReviewWorkflowClient
	reviewRepo     repository.ReviewRepository
	paperRepo      repository.PaperRepository
	keywordRepo    repository.KeywordRepository
}

// NewLiteratureReviewServer creates a new LiteratureReviewServer with all required dependencies.
func NewLiteratureReviewServer(
	workflowClient *temporal.ReviewWorkflowClient,
	reviewRepo repository.ReviewRepository,
	paperRepo repository.PaperRepository,
	keywordRepo repository.KeywordRepository,
) *LiteratureReviewServer {
	return &LiteratureReviewServer{
		workflowClient: workflowClient,
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
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, domain.ErrServiceUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, domain.ErrCancelled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, temporal.ErrWorkflowNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, temporal.ErrWorkflowAlreadyStarted):
		return status.Error(codes.AlreadyExists, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
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

// validateUUID parses and validates a UUID string. Returns a descriptive error
// referencing fieldName if the string is not a valid UUID.
func validateUUID(s, fieldName string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid %s: %v", fieldName, err))
	}
	return id, nil
}
