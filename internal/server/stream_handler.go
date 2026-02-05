package server

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
)

const (
	// streamPollInterval is how frequently the server polls the database for review progress updates
	// when streaming progress events to the client.
	streamPollInterval = 2 * time.Second

	// streamMaxDuration is the maximum total time a progress stream may remain open.
	// After this duration, the stream is closed with a DeadlineExceeded error.
	streamMaxDuration = 4 * time.Hour
)

// StreamLiteratureReviewProgress streams real-time progress updates for a literature review.
// It polls the review repository at regular intervals and sends progress events to the client
// until the review reaches a terminal state or the stream duration limit is exceeded.
func (s *LiteratureReviewServer) StreamLiteratureReviewProgress(req *pb.StreamLiteratureReviewProgressRequest, stream grpc.ServerStreamingServer[pb.LiteratureReviewProgressEvent]) error {
	if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
		return err
	}

	if err := validateTenantAccess(stream.Context(), req.OrgId, req.ProjectId); err != nil {
		return err
	}

	reviewID, err := validateUUID(req.ReviewId, "review_id")
	if err != nil {
		return err
	}

	review, err := s.reviewRepo.Get(stream.Context(), req.OrgId, req.ProjectId, reviewID)
	if err != nil {
		return domainErrToGRPC(err)
	}

	// If the review is already in a terminal state, send one final event and return.
	if review.Status.IsTerminal() {
		event := &pb.LiteratureReviewProgressEvent{
			ReviewId:  review.ID.String(),
			EventType: "completed",
			Status:    reviewStatusToProto(review.Status),
			Message:   "review is in terminal state: " + string(review.Status),
			Timestamp: timestamppb.Now(),
			Progress: &pb.ReviewProgress{
				PapersFound:    int32(review.PapersFoundCount),
				PapersIngested: int32(review.PapersIngestedCount),
				PapersFailed:   int32(review.PapersFailedCount),
			},
		}
		if err := stream.Send(event); err != nil {
			return err
		}
		return nil
	}

	// A workflow ID is required to stream progress for an active review.
	if review.TemporalWorkflowID == "" {
		return status.Error(codes.FailedPrecondition, "review has no associated workflow")
	}

	// Poll loop with deadline.
	ctx := stream.Context()
	deadlineTimer := time.NewTimer(streamMaxDuration)
	defer deadlineTimer.Stop()
	ticker := time.NewTicker(streamPollInterval)
	defer ticker.Stop()

	// Send initial event immediately.
	initialEvent := &pb.LiteratureReviewProgressEvent{
		ReviewId:  review.ID.String(),
		EventType: "stream_started",
		Status:    reviewStatusToProto(review.Status),
		Message:   "progress stream started",
		Timestamp: timestamppb.Now(),
	}
	if err := stream.Send(initialEvent); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadlineTimer.C:
			return status.Error(codes.DeadlineExceeded, "stream max duration exceeded")
		case <-ticker.C:
			// Re-fetch review status from repo.
			currentReview, err := s.reviewRepo.Get(ctx, req.OrgId, req.ProjectId, reviewID)
			if err != nil {
				return domainErrToGRPC(err)
			}

			event := &pb.LiteratureReviewProgressEvent{
				ReviewId:  currentReview.ID.String(),
				EventType: "progress_update",
				Status:    reviewStatusToProto(currentReview.Status),
				Message:   "status: " + string(currentReview.Status),
				Timestamp: timestamppb.Now(),
				Progress: &pb.ReviewProgress{
					PapersFound:    int32(currentReview.PapersFoundCount),
					PapersIngested: int32(currentReview.PapersIngestedCount),
					PapersFailed:   int32(currentReview.PapersFailedCount),
				},
			}

			if err := stream.Send(event); err != nil {
				return err
			}

			if currentReview.Status.IsTerminal() {
				// Send final event.
				finalEvent := &pb.LiteratureReviewProgressEvent{
					ReviewId:  currentReview.ID.String(),
					EventType: "completed",
					Status:    reviewStatusToProto(currentReview.Status),
					Message:   "review completed with status: " + string(currentReview.Status),
					Timestamp: timestamppb.Now(),
					Progress: &pb.ReviewProgress{
						PapersFound:    int32(currentReview.PapersFoundCount),
						PapersIngested: int32(currentReview.PapersIngestedCount),
						PapersFailed:   int32(currentReview.PapersFailedCount),
					},
				}
				_ = stream.Send(finalEvent)
				return nil
			}
		}
	}
}
