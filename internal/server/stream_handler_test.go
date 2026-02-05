package server

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
	"github.com/helixir/literature-review-service/internal/domain"
)

// ---------------------------------------------------------------------------
// Mock stream server for progress events
// ---------------------------------------------------------------------------

// mockProgressStreamServer mocks the gRPC server-streaming server for progress events.
type mockProgressStreamServer struct {
	grpc.ServerStreamingServer[pb.LiteratureReviewProgressEvent]
	ctx    context.Context
	events []*pb.LiteratureReviewProgressEvent
}

func (m *mockProgressStreamServer) Send(event *pb.LiteratureReviewProgressEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockProgressStreamServer) Context() context.Context {
	return m.ctx
}

// ---------------------------------------------------------------------------
// Tests: StreamLiteratureReviewProgress
// ---------------------------------------------------------------------------

func TestStreamProgress_ValidationError(t *testing.T) {
	srv := &LiteratureReviewServer{}

	tests := []struct {
		name string
		req  *pb.StreamLiteratureReviewProgressRequest
	}{
		{
			name: "empty org_id",
			req: &pb.StreamLiteratureReviewProgressRequest{
				OrgId:     "",
				ProjectId: "proj-1",
				ReviewId:  uuid.New().String(),
			},
		},
		{
			name: "empty project_id",
			req: &pb.StreamLiteratureReviewProgressRequest{
				OrgId:     "org-1",
				ProjectId: "",
				ReviewId:  uuid.New().String(),
			},
		},
		{
			name: "invalid review_id",
			req: &pb.StreamLiteratureReviewProgressRequest{
				OrgId:     "org-1",
				ProjectId: "proj-1",
				ReviewId:  "not-a-uuid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := &mockProgressStreamServer{ctx: context.Background()}
			err := srv.StreamLiteratureReviewProgress(tt.req, stream)
			assertGRPCCode(t, err, codes.InvalidArgument)
			assert.Empty(t, stream.events, "no events should be sent on validation error")
		})
	}
}

func TestStreamProgress_ReviewNotFound(t *testing.T) {
	reviewID := uuid.New()
	repo := &mockReviewRepo{
		getFn: func(_ context.Context, _, _ string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return nil, domain.NewNotFoundError("review", reviewID.String())
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})

	stream := &mockProgressStreamServer{ctx: context.Background()}
	req := &pb.StreamLiteratureReviewProgressRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  reviewID.String(),
	}

	err := srv.StreamLiteratureReviewProgress(req, stream)
	assertGRPCCode(t, err, codes.NotFound)
	assert.Empty(t, stream.events, "no events should be sent when review is not found")
}

func TestStreamProgress_TerminalReview(t *testing.T) {
	reviewID := uuid.New()

	terminalStatuses := []domain.ReviewStatus{
		domain.ReviewStatusCompleted,
		domain.ReviewStatusFailed,
		domain.ReviewStatusCancelled,
		domain.ReviewStatusPartial,
	}

	for _, terminalStatus := range terminalStatuses {
		t.Run(string(terminalStatus), func(t *testing.T) {
			repo := &mockReviewRepo{
				getFn: func(_ context.Context, _, _ string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
					return &domain.LiteratureReviewRequest{
						ID:                  reviewID,
						OrgID:               "org-1",
						ProjectID:           "proj-1",
						Status:              terminalStatus,
						PapersFoundCount:    42,
						PapersIngestedCount: 38,
						PapersFailedCount:   4,
					}, nil
				},
			}

			srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})
			stream := &mockProgressStreamServer{ctx: context.Background()}
			req := &pb.StreamLiteratureReviewProgressRequest{
				OrgId:     "org-1",
				ProjectId: "proj-1",
				ReviewId:  reviewID.String(),
			}

			err := srv.StreamLiteratureReviewProgress(req, stream)
			require.NoError(t, err)

			require.Len(t, stream.events, 1, "exactly one event should be sent for terminal review")
			event := stream.events[0]
			assert.Equal(t, reviewID.String(), event.ReviewId)
			assert.Equal(t, "completed", event.EventType)
			assert.Equal(t, reviewStatusToProto(terminalStatus), event.Status)
			assert.Contains(t, event.Message, string(terminalStatus))
			assert.NotNil(t, event.Timestamp)
			assert.NotNil(t, event.Progress)
			assert.Equal(t, int32(42), event.Progress.PapersFound)
			assert.Equal(t, int32(38), event.Progress.PapersIngested)
			assert.Equal(t, int32(4), event.Progress.PapersFailed)
		})
	}
}

func TestStreamProgress_NoWorkflowID(t *testing.T) {
	reviewID := uuid.New()
	repo := &mockReviewRepo{
		getFn: func(_ context.Context, _, _ string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:                 reviewID,
				OrgID:              "org-1",
				ProjectID:          "proj-1",
				Status:             domain.ReviewStatusPending,
				TemporalWorkflowID: "",
			}, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})
	stream := &mockProgressStreamServer{ctx: context.Background()}
	req := &pb.StreamLiteratureReviewProgressRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  reviewID.String(),
	}

	err := srv.StreamLiteratureReviewProgress(req, stream)
	assertGRPCCode(t, err, codes.FailedPrecondition)
	assert.Empty(t, stream.events, "no events should be sent when workflow ID is missing")
}

func TestStreamProgress_ContextCancellation(t *testing.T) {
	reviewID := uuid.New()

	repo := &mockReviewRepo{
		getFn: func(_ context.Context, _, _ string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:                 reviewID,
				OrgID:              "org-1",
				ProjectID:          "proj-1",
				Status:             domain.ReviewStatusSearching,
				TemporalWorkflowID: "wf-123",
				TemporalRunID:      "run-456",
			}, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})

	// Create a context that is immediately cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stream := &mockProgressStreamServer{ctx: ctx}
	req := &pb.StreamLiteratureReviewProgressRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  reviewID.String(),
	}

	err := srv.StreamLiteratureReviewProgress(req, stream)
	// When context is cancelled, the initial "stream_started" event is sent,
	// but then the select picks up ctx.Done() and returns ctx.Err().
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)

	// The initial "stream_started" event should have been sent.
	require.GreaterOrEqual(t, len(stream.events), 1)
	assert.Equal(t, "stream_started", stream.events[0].EventType)
}

func TestStreamProgress_PollLoopCompletesOnTerminalStatus(t *testing.T) {
	reviewID := uuid.New()
	callCount := 0

	repo := &mockReviewRepo{
		getFn: func(_ context.Context, _, _ string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			callCount++
			if callCount == 1 {
				// First call: initial check (active review).
				return &domain.LiteratureReviewRequest{
					ID:                  reviewID,
					OrgID:               "org-1",
					ProjectID:           "proj-1",
					Status:              domain.ReviewStatusSearching,
					TemporalWorkflowID:  "wf-123",
					PapersFoundCount:    10,
					PapersIngestedCount: 5,
				}, nil
			}
			// Subsequent calls from poll loop: return terminal status.
			return &domain.LiteratureReviewRequest{
				ID:                  reviewID,
				OrgID:               "org-1",
				ProjectID:           "proj-1",
				Status:              domain.ReviewStatusCompleted,
				TemporalWorkflowID:  "wf-123",
				PapersFoundCount:    42,
				PapersIngestedCount: 38,
				PapersFailedCount:   4,
			}, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream := &mockProgressStreamServer{ctx: ctx}
	req := &pb.StreamLiteratureReviewProgressRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  reviewID.String(),
	}

	err := srv.StreamLiteratureReviewProgress(req, stream)
	require.NoError(t, err)

	// Should have: stream_started + progress_update + completed
	require.GreaterOrEqual(t, len(stream.events), 2)

	// First event is stream_started.
	assert.Equal(t, "stream_started", stream.events[0].EventType)

	// Last event is completed.
	lastEvent := stream.events[len(stream.events)-1]
	assert.Equal(t, "completed", lastEvent.EventType)
	assert.Equal(t, int32(42), lastEvent.Progress.PapersFound)
	assert.Equal(t, int32(38), lastEvent.Progress.PapersIngested)
	assert.Equal(t, int32(4), lastEvent.Progress.PapersFailed)
}

func TestStreamProgress_PollLoopRepoError(t *testing.T) {
	reviewID := uuid.New()
	callCount := 0

	repo := &mockReviewRepo{
		getFn: func(_ context.Context, _, _ string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			callCount++
			if callCount == 1 {
				// Initial check succeeds.
				return &domain.LiteratureReviewRequest{
					ID:                 reviewID,
					OrgID:              "org-1",
					ProjectID:          "proj-1",
					Status:             domain.ReviewStatusSearching,
					TemporalWorkflowID: "wf-123",
				}, nil
			}
			// Poll loop: repo returns error.
			return nil, domain.NewNotFoundError("review", reviewID.String())
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream := &mockProgressStreamServer{ctx: ctx}
	req := &pb.StreamLiteratureReviewProgressRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  reviewID.String(),
	}

	err := srv.StreamLiteratureReviewProgress(req, stream)
	assertGRPCCode(t, err, codes.NotFound)

	// stream_started event should have been sent before the error.
	require.GreaterOrEqual(t, len(stream.events), 1)
	assert.Equal(t, "stream_started", stream.events[0].EventType)
}
