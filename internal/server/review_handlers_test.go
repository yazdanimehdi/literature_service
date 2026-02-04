package server

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
)

// ---------------------------------------------------------------------------
// mockReviewRepo is a mock ReviewRepository used by review and stream handler tests.
// ---------------------------------------------------------------------------

type mockReviewRepo struct {
	createFn           func(ctx context.Context, review *domain.LiteratureReviewRequest) error
	getFn              func(ctx context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error)
	updateFn           func(ctx context.Context, orgID, projectID string, id uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error
	listFn             func(ctx context.Context, filter repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error)
	updateStatusFn     func(ctx context.Context, orgID, projectID string, id uuid.UUID, status domain.ReviewStatus, errorMsg string) error
	incrementCounterFn func(ctx context.Context, orgID, projectID string, id uuid.UUID, papersFound, papersIngested int) error
	getByWorkflowIDFn  func(ctx context.Context, workflowID string) (*domain.LiteratureReviewRequest, error)
}

func (m *mockReviewRepo) Create(ctx context.Context, review *domain.LiteratureReviewRequest) error {
	if m.createFn != nil {
		return m.createFn(ctx, review)
	}
	return nil
}

func (m *mockReviewRepo) Get(ctx context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
	if m.getFn != nil {
		return m.getFn(ctx, orgID, projectID, id)
	}
	return nil, domain.ErrNotFound
}

func (m *mockReviewRepo) Update(ctx context.Context, orgID, projectID string, id uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, orgID, projectID, id, fn)
	}
	return nil
}

func (m *mockReviewRepo) UpdateStatus(ctx context.Context, orgID, projectID string, id uuid.UUID, st domain.ReviewStatus, errorMsg string) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, orgID, projectID, id, st, errorMsg)
	}
	return nil
}

func (m *mockReviewRepo) List(ctx context.Context, filter repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
	if m.listFn != nil {
		return m.listFn(ctx, filter)
	}
	return nil, 0, nil
}

func (m *mockReviewRepo) IncrementCounters(ctx context.Context, orgID, projectID string, id uuid.UUID, papersFound, papersIngested int) error {
	if m.incrementCounterFn != nil {
		return m.incrementCounterFn(ctx, orgID, projectID, id, papersFound, papersIngested)
	}
	return nil
}

func (m *mockReviewRepo) GetByWorkflowID(ctx context.Context, workflowID string) (*domain.LiteratureReviewRequest, error) {
	if m.getByWorkflowIDFn != nil {
		return m.getByWorkflowIDFn(ctx, workflowID)
	}
	return nil, domain.ErrNotFound
}

// ---------------------------------------------------------------------------
// mockPaperRepo is a no-op PaperRepository mock used by review and stream handler tests.
// ---------------------------------------------------------------------------

type mockPaperRepo struct {
	paperTestPaperRepo
}

// ---------------------------------------------------------------------------
// mockKeywordRepo is a no-op KeywordRepository mock used by review and stream handler tests.
// ---------------------------------------------------------------------------

type mockKeywordRepo struct {
	paperTestKeywordRepo
}

// ---------------------------------------------------------------------------
// Tests: StartLiteratureReview
// ---------------------------------------------------------------------------

func TestStartLiteratureReview_ValidationError_EmptyQuery(t *testing.T) {
	srv := newTestServer(&mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.StartLiteratureReview(context.Background(), &pb.StartLiteratureReviewRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		Query:     "",
	})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestStartLiteratureReview_ValidationError_EmptyOrgID(t *testing.T) {
	srv := newTestServer(&mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.StartLiteratureReview(context.Background(), &pb.StartLiteratureReviewRequest{
		OrgId:     "",
		ProjectId: "proj-1",
		Query:     "some query",
	})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestStartLiteratureReview_ValidationError_QueryTooLong(t *testing.T) {
	srv := newTestServer(&mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})
	longQuery := make([]byte, 10001)
	for i := range longQuery {
		longQuery[i] = 'a'
	}
	_, err := srv.StartLiteratureReview(context.Background(), &pb.StartLiteratureReviewRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		Query:     string(longQuery),
	})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

// ---------------------------------------------------------------------------
// Tests: GetLiteratureReviewStatus
// ---------------------------------------------------------------------------

func TestGetLiteratureReviewStatus_Success(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	repo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:            reviewID,
				OrgID:         orgID,
				ProjectID:     projectID,
				OriginalQuery: "CRISPR gene editing",
				Status:        domain.ReviewStatusSearching,
				Configuration: domain.DefaultReviewConfiguration(),
				CreatedAt:     now,
				UpdatedAt:     now,
			}, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})
	resp, err := srv.GetLiteratureReviewStatus(context.Background(), &pb.GetLiteratureReviewStatusRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  reviewID.String(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ReviewId != reviewID.String() {
		t.Errorf("expected review_id %s, got %s", reviewID.String(), resp.ReviewId)
	}
	if resp.Status != pb.ReviewStatus_REVIEW_STATUS_SEARCHING {
		t.Errorf("expected status SEARCHING, got %v", resp.Status)
	}
	if resp.CreatedAt == nil {
		t.Error("expected created_at to be set")
	}
	if resp.Configuration == nil {
		t.Error("expected configuration to be set")
	}
}

func TestGetLiteratureReviewStatus_NotFound(t *testing.T) {
	repo := &mockReviewRepo{
		getFn: func(_ context.Context, _ string, _ string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return nil, domain.NewNotFoundError("review", id.String())
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.GetLiteratureReviewStatus(context.Background(), &pb.GetLiteratureReviewStatusRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  uuid.New().String(),
	})
	assertGRPCCode(t, err, codes.NotFound)
}

func TestGetLiteratureReviewStatus_InvalidUUID(t *testing.T) {
	srv := newTestServer(&mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.GetLiteratureReviewStatus(context.Background(), &pb.GetLiteratureReviewStatusRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  "not-a-uuid",
	})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

// ---------------------------------------------------------------------------
// Tests: CancelLiteratureReview
// ---------------------------------------------------------------------------

func TestCancelLiteratureReview_AlreadyCompleted(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	repo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:        reviewID,
				OrgID:     orgID,
				ProjectID: projectID,
				Status:    domain.ReviewStatusCompleted,
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.CancelLiteratureReview(context.Background(), &pb.CancelLiteratureReviewRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  reviewID.String(),
		Reason:    "no longer needed",
	})
	assertGRPCCode(t, err, codes.FailedPrecondition)
}

func TestCancelLiteratureReview_ValidationError_EmptyOrgID(t *testing.T) {
	srv := newTestServer(&mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.CancelLiteratureReview(context.Background(), &pb.CancelLiteratureReviewRequest{
		OrgId:     "",
		ProjectId: "proj-1",
		ReviewId:  uuid.New().String(),
	})
	assertGRPCCode(t, err, codes.InvalidArgument)
}

// ---------------------------------------------------------------------------
// Tests: ListLiteratureReviews
// ---------------------------------------------------------------------------

func TestListLiteratureReviews_Success(t *testing.T) {
	now := time.Now()
	reviews := []*domain.LiteratureReviewRequest{
		{
			ID:                  uuid.New(),
			OrgID:               "org-1",
			ProjectID:           "proj-1",
			OriginalQuery:       "CRISPR",
			Status:              domain.ReviewStatusCompleted,
			PapersFoundCount:    42,
			PapersIngestedCount: 38,
			KeywordsFoundCount:  10,
			Configuration:       domain.DefaultReviewConfiguration(),
			CreatedAt:           now.Add(-2 * time.Hour),
			UpdatedAt:           now,
		},
		{
			ID:                  uuid.New(),
			OrgID:               "org-1",
			ProjectID:           "proj-1",
			OriginalQuery:       "mRNA vaccines",
			Status:              domain.ReviewStatusSearching,
			PapersFoundCount:    15,
			PapersIngestedCount: 0,
			KeywordsFoundCount:  5,
			Configuration:       domain.DefaultReviewConfiguration(),
			CreatedAt:           now.Add(-1 * time.Hour),
			UpdatedAt:           now,
		},
	}

	repo := &mockReviewRepo{
		listFn: func(_ context.Context, filter repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
			if filter.OrgID != "org-1" || filter.ProjectID != "proj-1" {
				t.Errorf("unexpected filter org/project: %s/%s", filter.OrgID, filter.ProjectID)
			}
			return reviews, 2, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})
	resp, err := srv.ListLiteratureReviews(context.Background(), &pb.ListLiteratureReviewsRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Reviews) != 2 {
		t.Fatalf("expected 2 reviews, got %d", len(resp.Reviews))
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected total_count 2, got %d", resp.TotalCount)
	}
	if resp.NextPageToken != "" {
		t.Errorf("expected empty next_page_token, got %q", resp.NextPageToken)
	}

	// Verify first review summary fields.
	s0 := resp.Reviews[0]
	if s0.ReviewId != reviews[0].ID.String() {
		t.Errorf("expected review_id %s, got %s", reviews[0].ID.String(), s0.ReviewId)
	}
	if s0.OriginalQuery != "CRISPR" {
		t.Errorf("expected original_query CRISPR, got %s", s0.OriginalQuery)
	}
	if s0.PapersFound != 42 {
		t.Errorf("expected papers_found 42, got %d", s0.PapersFound)
	}
	if s0.PapersIngested != 38 {
		t.Errorf("expected papers_ingested 38, got %d", s0.PapersIngested)
	}
	if s0.KeywordsUsed != 10 {
		t.Errorf("expected keywords_used 10, got %d", s0.KeywordsUsed)
	}
}

func TestListLiteratureReviews_Pagination(t *testing.T) {
	now := time.Now()
	allReviews := make([]*domain.LiteratureReviewRequest, 3)
	for i := range allReviews {
		allReviews[i] = &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			OrgID:         "org-1",
			ProjectID:     "proj-1",
			OriginalQuery: "query",
			Status:        domain.ReviewStatusCompleted,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
	}

	repo := &mockReviewRepo{
		listFn: func(_ context.Context, filter repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
			end := filter.Offset + filter.Limit
			if end > len(allReviews) {
				end = len(allReviews)
			}
			start := filter.Offset
			if start > len(allReviews) {
				start = len(allReviews)
			}
			return allReviews[start:end], 3, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})
	resp, err := srv.ListLiteratureReviews(context.Background(), &pb.ListLiteratureReviewsRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		PageSize:  2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Reviews) != 2 {
		t.Fatalf("expected 2 reviews on first page, got %d", len(resp.Reviews))
	}
	if resp.NextPageToken == "" {
		t.Fatal("expected non-empty next_page_token for paginated results")
	}
	if resp.TotalCount != 3 {
		t.Errorf("expected total_count 3, got %d", resp.TotalCount)
	}
}

func TestListLiteratureReviews_Empty(t *testing.T) {
	repo := &mockReviewRepo{
		listFn: func(_ context.Context, _ repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
			return nil, 0, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})
	resp, err := srv.ListLiteratureReviews(context.Background(), &pb.ListLiteratureReviewsRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(resp.Reviews))
	}
	if resp.TotalCount != 0 {
		t.Errorf("expected total_count 0, got %d", resp.TotalCount)
	}
	if resp.NextPageToken != "" {
		t.Errorf("expected empty next_page_token, got %q", resp.NextPageToken)
	}
}

func TestListLiteratureReviews_ValidationError_EmptyOrgID(t *testing.T) {
	srv := newTestServer(&mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.ListLiteratureReviews(context.Background(), &pb.ListLiteratureReviewsRequest{
		OrgId:     "",
		ProjectId: "proj-1",
	})
	assertGRPCCode(t, err, codes.InvalidArgument)
}
