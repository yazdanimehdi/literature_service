package server

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

// paperTestReviewRepo is a mock ReviewRepository for paper handler tests.
type paperTestReviewRepo struct {
	getFunc func(ctx context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error)
}

func (m *paperTestReviewRepo) Create(_ context.Context, _ *domain.LiteratureReviewRequest) error {
	return nil
}

func (m *paperTestReviewRepo) Get(ctx context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, orgID, projectID, id)
	}
	return nil, nil
}

func (m *paperTestReviewRepo) Update(_ context.Context, _, _ string, _ uuid.UUID, _ func(*domain.LiteratureReviewRequest) error) error {
	return nil
}

func (m *paperTestReviewRepo) UpdateStatus(_ context.Context, _, _ string, _ uuid.UUID, _ domain.ReviewStatus, _ string) error {
	return nil
}

func (m *paperTestReviewRepo) List(_ context.Context, _ repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
	return nil, 0, nil
}

func (m *paperTestReviewRepo) IncrementCounters(_ context.Context, _, _ string, _ uuid.UUID, _, _ int) error {
	return nil
}

func (m *paperTestReviewRepo) GetByWorkflowID(_ context.Context, _ string) (*domain.LiteratureReviewRequest, error) {
	return nil, nil
}

func (m *paperTestReviewRepo) FindPausedByReason(_ context.Context, _, _ string, _ domain.PauseReason) ([]*domain.LiteratureReviewRequest, error) {
	return nil, nil
}

func (m *paperTestReviewRepo) UpdatePauseState(_ context.Context, _, _ string, _ uuid.UUID, _ domain.ReviewStatus, _ domain.PauseReason, _ string) error {
	return nil
}

// paperTestPaperRepo is a mock PaperRepository for paper handler tests.
type paperTestPaperRepo struct {
	listFunc func(ctx context.Context, filter repository.PaperFilter) ([]*domain.Paper, int64, error)
}

func (m *paperTestPaperRepo) Create(_ context.Context, _ *domain.Paper) (*domain.Paper, error) {
	return nil, nil
}

func (m *paperTestPaperRepo) GetByCanonicalID(_ context.Context, _ string) (*domain.Paper, error) {
	return nil, nil
}

func (m *paperTestPaperRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.Paper, error) {
	return nil, nil
}

func (m *paperTestPaperRepo) FindByIdentifier(_ context.Context, _ domain.IdentifierType, _ string) (*domain.Paper, error) {
	return nil, nil
}

func (m *paperTestPaperRepo) UpsertIdentifier(_ context.Context, _ uuid.UUID, _ domain.IdentifierType, _ string, _ domain.SourceType) error {
	return nil
}

func (m *paperTestPaperRepo) AddSource(_ context.Context, _ uuid.UUID, _ domain.SourceType, _ map[string]interface{}) error {
	return nil
}

func (m *paperTestPaperRepo) List(ctx context.Context, filter repository.PaperFilter) ([]*domain.Paper, int64, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, filter)
	}
	return nil, 0, nil
}

func (m *paperTestPaperRepo) MarkKeywordsExtracted(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (m *paperTestPaperRepo) BulkUpsert(_ context.Context, _ []*domain.Paper) ([]*domain.Paper, error) {
	return nil, nil
}

func (m *paperTestPaperRepo) GetByIDs(_ context.Context, _ []uuid.UUID) ([]*domain.Paper, error) {
	return nil, nil
}

func (m *paperTestPaperRepo) UpdateIngestionResult(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string) error {
	return nil
}

// paperTestKeywordRepo is a mock KeywordRepository for paper handler tests.
type paperTestKeywordRepo struct {
	listFunc func(ctx context.Context, filter repository.KeywordFilter) ([]*domain.Keyword, int64, error)
}

func (m *paperTestKeywordRepo) GetOrCreate(_ context.Context, _ string) (*domain.Keyword, error) {
	return nil, nil
}

func (m *paperTestKeywordRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.Keyword, error) {
	return nil, nil
}

func (m *paperTestKeywordRepo) GetByNormalized(_ context.Context, _ string) (*domain.Keyword, error) {
	return nil, nil
}

func (m *paperTestKeywordRepo) BulkGetOrCreate(_ context.Context, _ []string) ([]*domain.Keyword, error) {
	return nil, nil
}

func (m *paperTestKeywordRepo) RecordSearch(_ context.Context, _ *domain.KeywordSearch) error {
	return nil
}

func (m *paperTestKeywordRepo) GetLastSearch(_ context.Context, _ uuid.UUID, _ domain.SourceType) (*domain.KeywordSearch, error) {
	return nil, nil
}

func (m *paperTestKeywordRepo) NeedsSearch(_ context.Context, _ uuid.UUID, _ domain.SourceType, _ time.Duration) (bool, error) {
	return false, nil
}

func (m *paperTestKeywordRepo) ListSearches(_ context.Context, _ repository.SearchFilter) ([]*domain.KeywordSearch, int64, error) {
	return nil, 0, nil
}

func (m *paperTestKeywordRepo) AddPaperMapping(_ context.Context, _ *domain.KeywordPaperMapping) error {
	return nil
}

func (m *paperTestKeywordRepo) BulkAddPaperMappings(_ context.Context, _ []*domain.KeywordPaperMapping) error {
	return nil
}

func (m *paperTestKeywordRepo) GetPapersForKeyword(_ context.Context, _ uuid.UUID, _, _ int) ([]*domain.Paper, int64, error) {
	return nil, 0, nil
}

func (m *paperTestKeywordRepo) GetPapersForKeywordAndSource(_ context.Context, _ uuid.UUID, _ domain.SourceType, _, _ int) ([]*domain.Paper, int64, error) {
	return nil, 0, nil
}

func (m *paperTestKeywordRepo) List(ctx context.Context, filter repository.KeywordFilter) ([]*domain.Keyword, int64, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, filter)
	}
	return nil, 0, nil
}

// ---------------------------------------------------------------------------
// Test helper
// ---------------------------------------------------------------------------

func assertGRPCCode(t *testing.T, err error, expected codes.Code) {
	t.Helper()
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != expected {
		t.Errorf("expected code %v, got %v: %s", expected, st.Code(), st.Message())
	}
}

// ---------------------------------------------------------------------------
// Helpers to build a test server
// ---------------------------------------------------------------------------

func newTestReview() *domain.LiteratureReviewRequest {
	now := time.Now()
	return &domain.LiteratureReviewRequest{
		ID:            uuid.New(),
		OrgID:         "org-1",
		ProjectID:     "proj-1",
		UserID:        "user-1",
		Title: "CRISPR gene editing",
		Status:        domain.ReviewStatusCompleted,
		CreatedAt:     now,
		UpdatedAt:     now,
		Configuration: domain.DefaultReviewConfiguration(),
	}
}

func newTestServer(
	reviewRepo repository.ReviewRepository,
	paperRepo repository.PaperRepository,
	keywordRepo repository.KeywordRepository,
) *LiteratureReviewServer {
	return NewLiteratureReviewServer(nil, nil, reviewRepo, paperRepo, keywordRepo)
}

// ---------------------------------------------------------------------------
// GetLiteratureReviewPapers tests
// ---------------------------------------------------------------------------

func TestGetLiteratureReviewPapers_Success(t *testing.T) {
	review := newTestReview()

	reviewRepo := &paperTestReviewRepo{
		getFunc: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			if orgID != review.OrgID || projectID != review.ProjectID || id != review.ID {
				return nil, domain.NewNotFoundError("review", id.String())
			}
			return review, nil
		},
	}

	paper1 := &domain.Paper{
		ID:              uuid.New(),
		CanonicalID:     "doi:10.1234/test1",
		Title:           "Paper One",
		Abstract:        "Abstract one",
		Authors:         []domain.Author{{Name: "Author A"}},
		PublicationYear: 2023,
		CitationCount:   10,
	}
	paper2 := &domain.Paper{
		ID:              uuid.New(),
		CanonicalID:     "doi:10.1234/test2",
		Title:           "Paper Two",
		Abstract:        "Abstract two",
		Authors:         []domain.Author{{Name: "Author B"}},
		PublicationYear: 2024,
		CitationCount:   5,
	}

	paperRepo := &paperTestPaperRepo{
		listFunc: func(_ context.Context, _ repository.PaperFilter) ([]*domain.Paper, int64, error) {
			return []*domain.Paper{paper1, paper2}, 5, nil
		},
	}

	srv := newTestServer(reviewRepo, paperRepo, &paperTestKeywordRepo{})

	resp, err := srv.GetLiteratureReviewPapers(context.Background(), &pb.GetLiteratureReviewPapersRequest{
		OrgId:     review.OrgID,
		ProjectId: review.ProjectID,
		ReviewId:  review.ID.String(),
		PageSize:  2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Papers) != 2 {
		t.Errorf("expected 2 papers, got %d", len(resp.Papers))
	}
	if resp.TotalCount != 5 {
		t.Errorf("expected total_count=5, got %d", resp.TotalCount)
	}
	if resp.NextPageToken == "" {
		t.Error("expected next_page_token to be set when more results exist")
	}
	if resp.Papers[0].Title != "Paper One" {
		t.Errorf("expected first paper title 'Paper One', got %q", resp.Papers[0].Title)
	}
	if resp.Papers[1].Title != "Paper Two" {
		t.Errorf("expected second paper title 'Paper Two', got %q", resp.Papers[1].Title)
	}
}

func TestGetLiteratureReviewPapers_ReviewNotFound(t *testing.T) {
	reviewRepo := &paperTestReviewRepo{
		getFunc: func(_ context.Context, _ string, _ string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return nil, domain.NewNotFoundError("review", id.String())
		},
	}

	srv := newTestServer(reviewRepo, &paperTestPaperRepo{}, &paperTestKeywordRepo{})

	reviewID := uuid.New()
	_, err := srv.GetLiteratureReviewPapers(context.Background(), &pb.GetLiteratureReviewPapersRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  reviewID.String(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertGRPCCode(t, err, codes.NotFound)
}

func TestGetLiteratureReviewPapers_ValidationError(t *testing.T) {
	srv := newTestServer(&paperTestReviewRepo{}, &paperTestPaperRepo{}, &paperTestKeywordRepo{})

	_, err := srv.GetLiteratureReviewPapers(context.Background(), &pb.GetLiteratureReviewPapersRequest{
		OrgId:     "",
		ProjectId: "proj-1",
		ReviewId:  uuid.New().String(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertGRPCCode(t, err, codes.InvalidArgument)
}

// ---------------------------------------------------------------------------
// GetLiteratureReviewKeywords tests
// ---------------------------------------------------------------------------

func TestGetLiteratureReviewKeywords_Success(t *testing.T) {
	review := newTestReview()

	reviewRepo := &paperTestReviewRepo{
		getFunc: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			if orgID != review.OrgID || projectID != review.ProjectID || id != review.ID {
				return nil, domain.NewNotFoundError("review", id.String())
			}
			return review, nil
		},
	}

	kw1 := &domain.Keyword{
		ID:                uuid.New(),
		Keyword:           "CRISPR",
		NormalizedKeyword: "crispr",
		CreatedAt:         time.Now(),
	}
	kw2 := &domain.Keyword{
		ID:                uuid.New(),
		Keyword:           "Gene Editing",
		NormalizedKeyword: "gene editing",
		CreatedAt:         time.Now(),
	}

	keywordRepo := &paperTestKeywordRepo{
		listFunc: func(_ context.Context, _ repository.KeywordFilter) ([]*domain.Keyword, int64, error) {
			return []*domain.Keyword{kw1, kw2}, 2, nil
		},
	}

	srv := newTestServer(reviewRepo, &paperTestPaperRepo{}, keywordRepo)

	resp, err := srv.GetLiteratureReviewKeywords(context.Background(), &pb.GetLiteratureReviewKeywordsRequest{
		OrgId:     review.OrgID,
		ProjectId: review.ProjectID,
		ReviewId:  review.ID.String(),
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Keywords) != 2 {
		t.Errorf("expected 2 keywords, got %d", len(resp.Keywords))
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected total_count=2, got %d", resp.TotalCount)
	}
	if resp.NextPageToken != "" {
		t.Errorf("expected no next_page_token when all results fit, got %q", resp.NextPageToken)
	}
	if resp.Keywords[0].Keyword != "CRISPR" {
		t.Errorf("expected first keyword 'CRISPR', got %q", resp.Keywords[0].Keyword)
	}
	if resp.Keywords[1].NormalizedKeyword != "gene editing" {
		t.Errorf("expected second normalized keyword 'gene editing', got %q", resp.Keywords[1].NormalizedKeyword)
	}
}

func TestGetLiteratureReviewKeywords_ValidationError_EmptyOrgID(t *testing.T) {
	srv := newTestServer(&paperTestReviewRepo{}, &paperTestPaperRepo{}, &paperTestKeywordRepo{})

	_, err := srv.GetLiteratureReviewKeywords(context.Background(), &pb.GetLiteratureReviewKeywordsRequest{
		OrgId:     "",
		ProjectId: "proj-1",
		ReviewId:  uuid.New().String(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestGetLiteratureReviewKeywords_InvalidUUID(t *testing.T) {
	srv := newTestServer(&paperTestReviewRepo{}, &paperTestPaperRepo{}, &paperTestKeywordRepo{})

	_, err := srv.GetLiteratureReviewKeywords(context.Background(), &pb.GetLiteratureReviewKeywordsRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  "not-a-valid-uuid",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertGRPCCode(t, err, codes.InvalidArgument)
}

func TestGetLiteratureReviewKeywords_ReviewNotFound(t *testing.T) {
	reviewRepo := &paperTestReviewRepo{
		getFunc: func(_ context.Context, _ string, _ string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return nil, domain.NewNotFoundError("review", id.String())
		},
	}

	srv := newTestServer(reviewRepo, &paperTestPaperRepo{}, &paperTestKeywordRepo{})

	_, err := srv.GetLiteratureReviewKeywords(context.Background(), &pb.GetLiteratureReviewKeywordsRequest{
		OrgId:     "org-1",
		ProjectId: "proj-1",
		ReviewId:  uuid.New().String(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertGRPCCode(t, err, codes.NotFound)
}

func TestGetLiteratureReviewKeywords_RepoError(t *testing.T) {
	review := newTestReview()

	reviewRepo := &paperTestReviewRepo{
		getFunc: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			if orgID != review.OrgID || projectID != review.ProjectID || id != review.ID {
				return nil, domain.NewNotFoundError("review", id.String())
			}
			return review, nil
		},
	}

	keywordRepo := &paperTestKeywordRepo{
		listFunc: func(_ context.Context, _ repository.KeywordFilter) ([]*domain.Keyword, int64, error) {
			return nil, 0, domain.ErrInternalError
		},
	}

	srv := newTestServer(reviewRepo, &paperTestPaperRepo{}, keywordRepo)

	_, err := srv.GetLiteratureReviewKeywords(context.Background(), &pb.GetLiteratureReviewKeywordsRequest{
		OrgId:     review.OrgID,
		ProjectId: review.ProjectID,
		ReviewId:  review.ID.String(),
		PageSize:  10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertGRPCCode(t, err, codes.Internal)
}

func TestGetLiteratureReviewPapers_RepoError(t *testing.T) {
	review := newTestReview()

	reviewRepo := &paperTestReviewRepo{
		getFunc: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			if orgID != review.OrgID || projectID != review.ProjectID || id != review.ID {
				return nil, domain.NewNotFoundError("review", id.String())
			}
			return review, nil
		},
	}

	paperRepo := &paperTestPaperRepo{
		listFunc: func(_ context.Context, _ repository.PaperFilter) ([]*domain.Paper, int64, error) {
			return nil, 0, domain.ErrInternalError
		},
	}

	srv := newTestServer(reviewRepo, paperRepo, &paperTestKeywordRepo{})

	_, err := srv.GetLiteratureReviewPapers(context.Background(), &pb.GetLiteratureReviewPapersRequest{
		OrgId:     review.OrgID,
		ProjectId: review.ProjectID,
		ReviewId:  review.ID.String(),
		PageSize:  10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertGRPCCode(t, err, codes.Internal)
}

func TestGetLiteratureReviewPapers_WithSourceFilter(t *testing.T) {
	review := newTestReview()

	reviewRepo := &paperTestReviewRepo{
		getFunc: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return review, nil
		},
	}

	var capturedFilter repository.PaperFilter
	paperRepo := &paperTestPaperRepo{
		listFunc: func(_ context.Context, filter repository.PaperFilter) ([]*domain.Paper, int64, error) {
			capturedFilter = filter
			return []*domain.Paper{}, 0, nil
		},
	}

	srv := newTestServer(reviewRepo, paperRepo, &paperTestKeywordRepo{})

	_, err := srv.GetLiteratureReviewPapers(context.Background(), &pb.GetLiteratureReviewPapersRequest{
		OrgId:        review.OrgID,
		ProjectId:    review.ProjectID,
		ReviewId:     review.ID.String(),
		PageSize:     10,
		SourceFilter: "semantic_scholar",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedFilter.Source == nil {
		t.Error("expected source filter to be set")
	} else if *capturedFilter.Source != domain.SourceTypeSemanticScholar {
		t.Errorf("expected source filter semantic_scholar, got %s", *capturedFilter.Source)
	}
}

func TestGetLiteratureReviewKeywords_Empty(t *testing.T) {
	review := newTestReview()

	reviewRepo := &paperTestReviewRepo{
		getFunc: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			if orgID != review.OrgID || projectID != review.ProjectID || id != review.ID {
				return nil, domain.NewNotFoundError("review", id.String())
			}
			return review, nil
		},
	}

	keywordRepo := &paperTestKeywordRepo{
		listFunc: func(_ context.Context, _ repository.KeywordFilter) ([]*domain.Keyword, int64, error) {
			return []*domain.Keyword{}, 0, nil
		},
	}

	srv := newTestServer(reviewRepo, &paperTestPaperRepo{}, keywordRepo)

	resp, err := srv.GetLiteratureReviewKeywords(context.Background(), &pb.GetLiteratureReviewKeywordsRequest{
		OrgId:     review.OrgID,
		ProjectId: review.ProjectID,
		ReviewId:  review.ID.String(),
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Keywords) != 0 {
		t.Errorf("expected 0 keywords, got %d", len(resp.Keywords))
	}
	if resp.TotalCount != 0 {
		t.Errorf("expected total_count=0, got %d", resp.TotalCount)
	}
	if resp.NextPageToken != "" {
		t.Errorf("expected empty next_page_token, got %q", resp.NextPageToken)
	}
}
