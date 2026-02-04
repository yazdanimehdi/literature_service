package server

import (
	"context"
	"encoding/base64"
	"strconv"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
	"github.com/helixir/literature-review-service/internal/temporal"
	"github.com/helixir/literature-review-service/internal/temporal/workflows"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
)

// StartLiteratureReview initiates a new literature review by creating a review
// record and starting the Temporal workflow that orchestrates the review pipeline.
func (s *LiteratureReviewServer) StartLiteratureReview(ctx context.Context, req *pb.StartLiteratureReviewRequest) (*pb.StartLiteratureReviewResponse, error) {
	if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	// Validate query.
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}
	if len(req.Query) > 10000 {
		return nil, status.Error(codes.InvalidArgument, "query must be at most 10000 characters")
	}

	requestID := uuid.New()

	// Build configuration from defaults, overriding with request values.
	cfg := domain.DefaultReviewConfiguration()
	if req.InitialKeywordCount != nil {
		cfg.MaxKeywordsPerRound = int(req.InitialKeywordCount.Value)
	}
	if req.MaxExpansionDepth != nil {
		cfg.MaxExpansionDepth = int(req.MaxExpansionDepth.Value)
	}
	if len(req.SourceFilters) > 0 {
		sources := make([]domain.SourceType, len(req.SourceFilters))
		for i, sf := range req.SourceFilters {
			sources[i] = domain.SourceType(sf)
		}
		cfg.Sources = sources
	}
	if req.DateFrom != nil {
		t := req.DateFrom.AsTime()
		cfg.DateFrom = &t
	}
	if req.DateTo != nil {
		t := req.DateTo.AsTime()
		cfg.DateTo = &t
	}

	now := time.Now()
	review := &domain.LiteratureReviewRequest{
		ID:            requestID,
		OrgID:         req.OrgId,
		ProjectID:     req.ProjectId,
		OriginalQuery: req.Query,
		Status:        domain.ReviewStatusPending,
		Configuration: cfg,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.reviewRepo.Create(ctx, review); err != nil {
		return nil, domainErrToGRPC(err)
	}

	// Prepare and start the Temporal workflow.
	wfInput := workflows.ReviewWorkflowInput{
		RequestID: requestID,
		OrgID:     req.OrgId,
		ProjectID: req.ProjectId,
		Query:     req.Query,
		Config:    cfg,
	}

	workflowID, runID, err := s.workflowClient.StartReviewWorkflow(ctx, temporal.ReviewWorkflowRequest{
		RequestID: requestID.String(),
		OrgID:     req.OrgId,
		ProjectID: req.ProjectId,
		Query:     req.Query,
	}, workflows.LiteratureReviewWorkflow, wfInput)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	// Best-effort update of workflow tracking IDs on the review record.
	_ = s.reviewRepo.Update(ctx, req.OrgId, req.ProjectId, requestID, func(r *domain.LiteratureReviewRequest) error {
		r.TemporalWorkflowID = workflowID
		r.TemporalRunID = runID
		return nil
	})

	return &pb.StartLiteratureReviewResponse{
		ReviewId:   requestID.String(),
		WorkflowId: workflowID,
		Status:     pb.ReviewStatus_REVIEW_STATUS_PENDING,
		CreatedAt:  timestamppb.New(now),
		Message:    "literature review started",
	}, nil
}

// GetLiteratureReviewStatus retrieves the current status and details of a
// literature review request.
func (s *LiteratureReviewServer) GetLiteratureReviewStatus(ctx context.Context, req *pb.GetLiteratureReviewStatusRequest) (*pb.GetLiteratureReviewStatusResponse, error) {
	if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	reviewID, err := validateUUID(req.ReviewId, "review_id")
	if err != nil {
		return nil, err
	}

	review, err := s.reviewRepo.Get(ctx, req.OrgId, req.ProjectId, reviewID)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	return reviewToStatusResponseProto(review), nil
}

// CancelLiteratureReview requests cancellation of a running literature review
// by signalling the Temporal workflow.
func (s *LiteratureReviewServer) CancelLiteratureReview(ctx context.Context, req *pb.CancelLiteratureReviewRequest) (*pb.CancelLiteratureReviewResponse, error) {
	if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	reviewID, err := validateUUID(req.ReviewId, "review_id")
	if err != nil {
		return nil, err
	}

	review, err := s.reviewRepo.Get(ctx, req.OrgId, req.ProjectId, reviewID)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	if review.Status.IsTerminal() {
		return nil, status.Error(codes.FailedPrecondition, "review is already in terminal state")
	}

	err = s.workflowClient.SignalWorkflow(ctx, review.TemporalWorkflowID, review.TemporalRunID, workflows.SignalCancel, req.Reason)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	return &pb.CancelLiteratureReviewResponse{
		Success:     true,
		Message:     "cancellation requested",
		FinalStatus: reviewStatusToProto(review.Status),
	}, nil
}

// ListLiteratureReviews returns a paginated list of literature review summaries
// for a given organization and project, with optional status and date filters.
func (s *LiteratureReviewServer) ListLiteratureReviews(ctx context.Context, req *pb.ListLiteratureReviewsRequest) (*pb.ListLiteratureReviewsResponse, error) {
	if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	limit := int(req.PageSize)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	offset := 0
	if req.PageToken != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.PageToken)
		if err == nil {
			offset, _ = strconv.Atoi(string(decoded))
		}
	}

	filter := repository.ReviewFilter{
		OrgID:     req.OrgId,
		ProjectID: req.ProjectId,
		Limit:     limit,
		Offset:    offset,
	}

	if req.StatusFilter != pb.ReviewStatus_REVIEW_STATUS_UNSPECIFIED {
		filter.Status = []domain.ReviewStatus{protoToReviewStatus(req.StatusFilter)}
	}
	if req.CreatedAfter != nil {
		t := req.CreatedAfter.AsTime()
		filter.CreatedAfter = &t
	}
	if req.CreatedBefore != nil {
		t := req.CreatedBefore.AsTime()
		filter.CreatedBefore = &t
	}

	reviews, totalCount, err := s.reviewRepo.List(ctx, filter)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	summaries := make([]*pb.LiteratureReviewSummary, len(reviews))
	for i, r := range reviews {
		summaries[i] = reviewToSummaryProto(r)
	}

	nextPageToken := ""
	if offset+limit < int(totalCount) {
		nextPageToken = base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset + limit)))
	}

	return &pb.ListLiteratureReviewsResponse{
		Reviews:       summaries,
		NextPageToken: nextPageToken,
		TotalCount:    int32(totalCount),
	}, nil
}
