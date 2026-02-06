package server

import (
	"context"
	"fmt"
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

	if err := validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	// Validate query.
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}
	const minQueryLength = 3
	if len(req.Query) < minQueryLength {
		return nil, status.Errorf(codes.InvalidArgument, "query must be at least %d characters", minQueryLength)
	}
	if len(req.Query) > maxQueryLength {
		return nil, status.Errorf(codes.InvalidArgument, "query must be at most %d characters", maxQueryLength)
	}

	requestID := uuid.New()

	// Build configuration from defaults, overriding with request values.
	cfg := domain.DefaultReviewConfiguration()
	if req.InitialKeywordCount != nil {
		cfg.MaxKeywordsPerRound = int(req.InitialKeywordCount.Value)
	}
	if req.PaperKeywordCount != nil {
		cfg.PaperKeywordCount = int(req.PaperKeywordCount.Value)
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
	wfInput := temporal.ReviewWorkflowInput{
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
	}, s.workflowFunc, wfInput)
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

	if err := validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
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

	if err := validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
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

	err = s.workflowClient.SignalWorkflow(ctx, review.TemporalWorkflowID, review.TemporalRunID, temporal.SignalCancel, req.Reason)
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

	if err := validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	limit, offset := parsePagination(req.PageSize, req.PageToken)

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

	return &pb.ListLiteratureReviewsResponse{
		Reviews:       summaries,
		NextPageToken: encodePageToken(offset, limit, totalCount),
		TotalCount:    int32(totalCount),
	}, nil
}

// PauseReview requests the workflow to pause at the next checkpoint.
// The workflow will complete its current activity and then enter a paused state.
func (s *LiteratureReviewServer) PauseReview(ctx context.Context, req *pb.PauseReviewRequest) (*pb.PauseReviewResponse, error) {
	if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	if err := validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
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

	if review.Status.IsTerminal() || review.Status == domain.ReviewStatusPaused {
		return nil, status.Error(codes.FailedPrecondition,
			fmt.Sprintf("cannot pause review in %s status", review.Status))
	}

	err = s.workflowClient.SignalWorkflow(ctx, review.TemporalWorkflowID, review.TemporalRunID,
		temporal.SignalPause,
		workflows.PauseSignal{Reason: domain.PauseReasonUser},
	)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	return &pb.PauseReviewResponse{Success: true, Message: "Review pausing"}, nil
}

// ResumeReview requests the workflow to resume from a paused state.
// The workflow will continue from where it was paused.
func (s *LiteratureReviewServer) ResumeReview(ctx context.Context, req *pb.ResumeReviewRequest) (*pb.ResumeReviewResponse, error) {
	if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	if err := validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
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

	if review.Status != domain.ReviewStatusPaused {
		return nil, status.Error(codes.FailedPrecondition,
			fmt.Sprintf("cannot resume review in %s status", review.Status))
	}

	err = s.workflowClient.SignalWorkflow(ctx, review.TemporalWorkflowID, review.TemporalRunID,
		temporal.SignalResume,
		workflows.ResumeSignal{ResumedBy: "user"},
	)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	return &pb.ResumeReviewResponse{Success: true, Message: "Review resuming"}, nil
}

// StopReview requests graceful termination of the workflow with partial results.
// If the review is paused, it is first resumed before stopping.
func (s *LiteratureReviewServer) StopReview(ctx context.Context, req *pb.StopReviewRequest) (*pb.StopReviewResponse, error) {
	if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	if err := validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
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
		return nil, status.Error(codes.FailedPrecondition,
			fmt.Sprintf("cannot stop review in %s status", review.Status))
	}

	// If paused, resume first so the workflow can receive the stop signal.
	if review.Status == domain.ReviewStatusPaused {
		_ = s.workflowClient.SignalWorkflow(ctx, review.TemporalWorkflowID, review.TemporalRunID,
			temporal.SignalResume,
			workflows.ResumeSignal{ResumedBy: "stop_request"},
		)
	}

	err = s.workflowClient.SignalWorkflow(ctx, review.TemporalWorkflowID, review.TemporalRunID,
		temporal.SignalStop,
		workflows.StopSignal{Reason: "user_requested"},
	)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	return &pb.StopReviewResponse{Success: true, Message: "Review stopping"}, nil
}

// ListPausedReviews returns all paused reviews for the given org and project.
// Optionally filters by pause reason (user, budget_exhausted).
func (s *LiteratureReviewServer) ListPausedReviews(ctx context.Context, req *pb.ListPausedReviewsRequest) (*pb.ListPausedReviewsResponse, error) {
	if req.OrgId == "" {
		return nil, status.Error(codes.InvalidArgument, "org_id is required")
	}

	if err := validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	var reason domain.PauseReason
	if req.PauseReason != "" {
		if req.PauseReason != string(domain.PauseReasonUser) &&
			req.PauseReason != string(domain.PauseReasonBudgetExhausted) {
			return nil, status.Error(codes.InvalidArgument,
				fmt.Sprintf("invalid pause_reason: must be 'user' or 'budget_exhausted', got %q", req.PauseReason))
		}
		reason = domain.PauseReason(req.PauseReason)
	}

	reviews, err := s.reviewRepo.FindPausedByReason(ctx, req.OrgId, req.ProjectId, reason)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	pbReviews := make([]*pb.PausedReview, 0, len(reviews))
	for _, r := range reviews {
		pbReview := &pb.PausedReview{
			ReviewId:      r.ID.String(),
			ProjectId:     r.ProjectID,
			PauseReason:   string(r.PauseReason),
			PausedAtPhase: r.PausedAtPhase,
		}
		if r.PausedAt != nil {
			pbReview.PausedAt = timestamppb.New(*r.PausedAt)
		}
		pbReviews = append(pbReviews, pbReview)
	}

	return &pb.ListPausedReviewsResponse{Reviews: pbReviews}, nil
}
