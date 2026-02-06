package server

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	grpcauth "github.com/helixir/grpcauth"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
	"github.com/helixir/literature-review-service/internal/temporal"

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

func (m *mockReviewRepo) FindPausedByReason(_ context.Context, _, _ string, _ domain.PauseReason) ([]*domain.LiteratureReviewRequest, error) {
	return nil, nil
}

func (m *mockReviewRepo) UpdatePauseState(_ context.Context, _, _ string, _ uuid.UUID, _ domain.ReviewStatus, _ domain.PauseReason, _ string) error {
	return nil
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
				Title: "CRISPR gene editing",
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
			Title:       "CRISPR",
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
			Title:       "mRNA vaccines",
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
			Title: "query",
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

// ---------------------------------------------------------------------------
// mockWorkflowClient implements WorkflowClient for handler tests.
// ---------------------------------------------------------------------------

type mockWorkflowClient struct {
	startFn  func(ctx context.Context, req temporal.ReviewWorkflowRequest, wfFunc interface{}, input interface{}) (string, string, error)
	signalFn func(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error
	queryFn  func(ctx context.Context, workflowID, runID, queryType string, result interface{}, args ...interface{}) error
}

func (m *mockWorkflowClient) StartReviewWorkflow(ctx context.Context, req temporal.ReviewWorkflowRequest, wfFunc interface{}, input interface{}) (string, string, error) {
	if m.startFn != nil {
		return m.startFn(ctx, req, wfFunc, input)
	}
	return "wf-test", "run-test", nil
}

func (m *mockWorkflowClient) SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error {
	if m.signalFn != nil {
		return m.signalFn(ctx, workflowID, runID, signalName, arg)
	}
	return nil
}

func (m *mockWorkflowClient) QueryWorkflow(ctx context.Context, workflowID, runID, queryType string, result interface{}, args ...interface{}) error {
	if m.queryFn != nil {
		return m.queryFn(ctx, workflowID, runID, queryType, result, args...)
	}
	return nil
}

func newTestServerWithWorkflow(
	wfClient WorkflowClient,
	reviewRepo repository.ReviewRepository,
	paperRepo repository.PaperRepository,
	keywordRepo repository.KeywordRepository,
) *LiteratureReviewServer {
	return NewLiteratureReviewServer(wfClient, nil, reviewRepo, paperRepo, keywordRepo)
}

// ---------------------------------------------------------------------------
// Tests: StartLiteratureReview success paths
// ---------------------------------------------------------------------------

func TestStartLiteratureReview_Success(t *testing.T) {
	var createdReview *domain.LiteratureReviewRequest

	repo := &mockReviewRepo{
		createFn: func(_ context.Context, review *domain.LiteratureReviewRequest) error {
			createdReview = review
			return nil
		},
		updateFn: func(_ context.Context, _, _ string, _ uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
			r := &domain.LiteratureReviewRequest{}
			return fn(r)
		},
	}

	wfClient := &mockWorkflowClient{
		startFn: func(_ context.Context, req temporal.ReviewWorkflowRequest, _ interface{}, _ interface{}) (string, string, error) {
			return "wf-review-" + req.RequestID, "run-abc123", nil
		},
	}

	srv := newTestServerWithWorkflow(wfClient, repo, &mockPaperRepo{}, &mockKeywordRepo{})
	resp, err := srv.StartLiteratureReview(context.Background(), &pb.StartLiteratureReviewRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		Query:     "CRISPR gene editing in cancer treatment",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ReviewId == "" {
		t.Error("expected review_id to be set")
	}
	if resp.WorkflowId == "" {
		t.Error("expected workflow_id to be set")
	}
	if resp.Status != pb.ReviewStatus_REVIEW_STATUS_PENDING {
		t.Errorf("expected PENDING status, got %v", resp.Status)
	}
	if resp.CreatedAt == nil {
		t.Error("expected created_at to be set")
	}
	if resp.Message == "" {
		t.Error("expected message to be set")
	}

	// Verify the created review has correct fields.
	if createdReview == nil {
		t.Fatal("expected createFn to be called")
	}
	if createdReview.OrgID != "org-1" {
		t.Errorf("expected org_id org-1, got %s", createdReview.OrgID)
	}
	if createdReview.Status != domain.ReviewStatusPending {
		t.Errorf("expected pending status, got %s", createdReview.Status)
	}
}

func TestStartLiteratureReview_CreateRepoError(t *testing.T) {
	repo := &mockReviewRepo{
		createFn: func(_ context.Context, _ *domain.LiteratureReviewRequest) error {
			return domain.ErrInternalError
		},
	}

	wfClient := &mockWorkflowClient{}
	srv := newTestServerWithWorkflow(wfClient, repo, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.StartLiteratureReview(context.Background(), &pb.StartLiteratureReviewRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		Query:     "some query",
	})
	assertGRPCCode(t, err, codes.Internal)
}

func TestStartLiteratureReview_WorkflowError(t *testing.T) {
	repo := &mockReviewRepo{
		createFn: func(_ context.Context, _ *domain.LiteratureReviewRequest) error {
			return nil
		},
	}

	wfClient := &mockWorkflowClient{
		startFn: func(_ context.Context, _ temporal.ReviewWorkflowRequest, _ interface{}, _ interface{}) (string, string, error) {
			return "", "", temporal.ErrConnectionFailed
		},
	}

	srv := newTestServerWithWorkflow(wfClient, repo, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.StartLiteratureReview(context.Background(), &pb.StartLiteratureReviewRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		Query:     "some query",
	})
	// ErrConnectionFailed is unmapped, falls through to Internal
	assertGRPCCode(t, err, codes.Internal)
}

func TestStartLiteratureReview_WithOptionalFields(t *testing.T) {
	repo := &mockReviewRepo{
		createFn: func(_ context.Context, _ *domain.LiteratureReviewRequest) error {
			return nil
		},
		updateFn: func(_ context.Context, _, _ string, _ uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
			return fn(&domain.LiteratureReviewRequest{})
		},
	}

	wfClient := &mockWorkflowClient{}

	srv := newTestServerWithWorkflow(wfClient, repo, &mockPaperRepo{}, &mockKeywordRepo{})

	now := time.Now()
	dateFrom := timestamppb.New(now.Add(-365 * 24 * time.Hour))
	dateTo := timestamppb.New(now)

	resp, err := srv.StartLiteratureReview(context.Background(), &pb.StartLiteratureReviewRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		Query:     "mRNA vaccine development",
		SourceFilters: []string{
			"semantic_scholar",
			"pubmed",
		},
		DateFrom: dateFrom,
		DateTo:   dateTo,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != pb.ReviewStatus_REVIEW_STATUS_PENDING {
		t.Errorf("expected PENDING status, got %v", resp.Status)
	}
}

// ---------------------------------------------------------------------------
// Tests: CancelLiteratureReview success path
// ---------------------------------------------------------------------------

func TestCancelLiteratureReview_Success(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	var signalledWorkflowID string
	var signalledSignalName string

	repo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:                 reviewID,
				OrgID:              orgID,
				ProjectID:          projectID,
				Status:             domain.ReviewStatusSearching,
				TemporalWorkflowID: "wf-review-123",
				TemporalRunID:      "run-abc",
				CreatedAt:          now,
				UpdatedAt:          now,
			}, nil
		},
	}

	wfClient := &mockWorkflowClient{
		signalFn: func(_ context.Context, workflowID, _, signalName string, _ interface{}) error {
			signalledWorkflowID = workflowID
			signalledSignalName = signalName
			return nil
		},
	}

	srv := newTestServerWithWorkflow(wfClient, repo, &mockPaperRepo{}, &mockKeywordRepo{})
	resp, err := srv.CancelLiteratureReview(context.Background(), &pb.CancelLiteratureReviewRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  reviewID.String(),
		Reason:    "no longer needed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
	if signalledWorkflowID != "wf-review-123" {
		t.Errorf("expected workflow ID wf-review-123, got %s", signalledWorkflowID)
	}
	if signalledSignalName != temporal.SignalCancel {
		t.Errorf("expected signal name %q, got %q", temporal.SignalCancel, signalledSignalName)
	}
}

func TestCancelLiteratureReview_SignalError(t *testing.T) {
	reviewID := uuid.New()

	repo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:                 reviewID,
				OrgID:              orgID,
				ProjectID:          projectID,
				Status:             domain.ReviewStatusSearching,
				TemporalWorkflowID: "wf-123",
				TemporalRunID:      "run-456",
			}, nil
		},
	}

	wfClient := &mockWorkflowClient{
		signalFn: func(_ context.Context, _, _, _ string, _ interface{}) error {
			return temporal.ErrWorkflowNotFound
		},
	}

	srv := newTestServerWithWorkflow(wfClient, repo, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.CancelLiteratureReview(context.Background(), &pb.CancelLiteratureReviewRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  reviewID.String(),
		Reason:    "test",
	})
	assertGRPCCode(t, err, codes.NotFound)
}

// ---------------------------------------------------------------------------
// Tests: ListLiteratureReviews with status/date filters
// ---------------------------------------------------------------------------

func TestListLiteratureReviews_WithStatusFilter(t *testing.T) {
	now := time.Now()
	reviews := []*domain.LiteratureReviewRequest{
		{
			ID:            uuid.New(),
			OrgID:         "org-1",
			ProjectID:     "proj-1",
			Title: "CRISPR",
			Status:        domain.ReviewStatusFailed,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}

	var capturedFilter repository.ReviewFilter
	repo := &mockReviewRepo{
		listFn: func(_ context.Context, filter repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
			capturedFilter = filter
			return reviews, 1, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})
	resp, err := srv.ListLiteratureReviews(context.Background(), &pb.ListLiteratureReviewsRequest{
		OrgId:        "org-1",
		ProjectId:    "proj-1",
		PageSize:     10,
		StatusFilter: pb.ReviewStatus_REVIEW_STATUS_FAILED,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(resp.Reviews))
	}
	// Verify the filter was properly constructed.
	if len(capturedFilter.Status) != 1 || capturedFilter.Status[0] != domain.ReviewStatusFailed {
		t.Errorf("expected status filter [failed], got %v", capturedFilter.Status)
	}
}

func TestListLiteratureReviews_WithDateFilters(t *testing.T) {
	now := time.Now()
	createdAfter := timestamppb.New(now.Add(-48 * time.Hour))
	createdBefore := timestamppb.New(now)

	var capturedFilter repository.ReviewFilter
	repo := &mockReviewRepo{
		listFn: func(_ context.Context, filter repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
			capturedFilter = filter
			return nil, 0, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.ListLiteratureReviews(context.Background(), &pb.ListLiteratureReviewsRequest{
		OrgId:         "org-1",
		ProjectId:     "proj-1",
		PageSize:      10,
		CreatedAfter:  createdAfter,
		CreatedBefore: createdBefore,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedFilter.CreatedAfter == nil {
		t.Error("expected CreatedAfter filter to be set")
	}
	if capturedFilter.CreatedBefore == nil {
		t.Error("expected CreatedBefore filter to be set")
	}
}

func TestListLiteratureReviews_RepoError(t *testing.T) {
	repo := &mockReviewRepo{
		listFn: func(_ context.Context, _ repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
			return nil, 0, domain.ErrInternalError
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})
	_, err := srv.ListLiteratureReviews(context.Background(), &pb.ListLiteratureReviewsRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
	})
	assertGRPCCode(t, err, codes.Internal)
}

// ---------------------------------------------------------------------------
// Tests: Tenant access validation
// ---------------------------------------------------------------------------

func TestStartLiteratureReview_TenantAccessDenied(t *testing.T) {
	repo := &mockReviewRepo{
		createFn: func(_ context.Context, _ *domain.LiteratureReviewRequest) error {
			return nil
		},
	}
	wfClient := &mockWorkflowClient{}
	srv := newTestServerWithWorkflow(wfClient, repo, &mockPaperRepo{}, &mockKeywordRepo{})

	// Create context with auth that does NOT have access to "org-1".
	authCtx := &grpcauth.AuthContext{
		User: &grpcauth.UserIdentity{
			Subject: "user-123",
			OrgIDs:  []string{"other-org"},
		},
	}
	ctx := grpcauth.ContextWithAuth(context.Background(), authCtx)

	_, err := srv.StartLiteratureReview(ctx, &pb.StartLiteratureReviewRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		Query:     "test query",
	})
	assertGRPCCode(t, err, codes.PermissionDenied)
}

func TestGetLiteratureReviewStatus_TenantAccessAllowed(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	repo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:            reviewID,
				OrgID:         orgID,
				ProjectID:     projectID,
				Title: "CRISPR",
				Status:        domain.ReviewStatusCompleted,
				Configuration: domain.DefaultReviewConfiguration(),
				CreatedAt:     now,
				UpdatedAt:     now,
			}, nil
		},
	}

	srv := newTestServer(repo, &mockPaperRepo{}, &mockKeywordRepo{})

	// Create context with auth that HAS access to "org-1".
	authCtx := &grpcauth.AuthContext{
		User: &grpcauth.UserIdentity{
			Subject:      "user-123",
			OrgIDs:       []string{"org-1"},
			ActiveOrgID:  "org-1",
			ProjectRoles: map[string][]string{"proj-1": {"editor"}},
		},
	}
	ctx := grpcauth.ContextWithAuth(context.Background(), authCtx)

	resp, err := srv.GetLiteratureReviewStatus(ctx, &pb.GetLiteratureReviewStatusRequest{
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
}
