package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
)

// ---------------------------------------------------------------------------
// Tests: getLiteratureReviewPapers
// ---------------------------------------------------------------------------

func TestGetLiteratureReviewPapers_Success(t *testing.T) {
	reviewID := uuid.New()
	paperID1 := uuid.New()
	paperID2 := uuid.New()
	now := time.Now()

	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			if id != reviewID {
				return nil, domain.NewNotFoundError("review", id.String())
			}
			return &domain.LiteratureReviewRequest{
				ID:            reviewID,
				OrgID:         orgID,
				ProjectID:     projectID,
				OriginalQuery: "CRISPR gene editing",
				Status:        domain.ReviewStatusCompleted,
				Configuration: domain.DefaultReviewConfiguration(),
				CreatedAt:     now,
				UpdatedAt:     now,
			}, nil
		},
	}

	pubDate := now.Add(-30 * 24 * time.Hour)
	papers := []*domain.Paper{
		{
			ID:              paperID1,
			CanonicalID:     "doi:10.1234/paper1",
			Title:           "CRISPR Advances in Oncology",
			Abstract:        "A review of CRISPR applications in cancer treatment.",
			Authors:         []domain.Author{{Name: "Jane Doe", Affiliation: "MIT", ORCID: "0000-0001-2345-6789"}},
			PublicationDate: &pubDate,
			PublicationYear: 2024,
			Venue:           "Nature",
			Journal:         "Nature Reviews Cancer",
			CitationCount:   150,
			PDFURL:          "https://example.com/paper1.pdf",
			OpenAccess:      true,
		},
		{
			ID:              paperID2,
			CanonicalID:     "doi:10.1234/paper2",
			Title:           "Gene Editing Techniques",
			Abstract:        "An overview of gene editing techniques.",
			Authors:         []domain.Author{{Name: "John Smith"}},
			PublicationYear: 2023,
			CitationCount:   42,
			OpenAccess:      false,
		},
	}

	var capturedFilter repository.PaperFilter
	paperRepo := &mockPaperRepo{
		listFn: func(_ context.Context, filter repository.PaperFilter) ([]*domain.Paper, int64, error) {
			capturedFilter = filter
			return papers, 2, nil
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, paperRepo, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", "/"+reviewID.String()+"/papers?page_size=10"), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp listPapersResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Papers) != 2 {
		t.Fatalf("expected 2 papers, got %d", len(resp.Papers))
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected total_count 2, got %d", resp.TotalCount)
	}
	if resp.NextPageToken != "" {
		t.Errorf("expected empty next_page_token, got %q", resp.NextPageToken)
	}

	// Verify first paper response fields.
	p0 := resp.Papers[0]
	if p0.ID != paperID1.String() {
		t.Errorf("expected paper id %s, got %s", paperID1.String(), p0.ID)
	}
	if p0.CanonicalID != "doi:10.1234/paper1" {
		t.Errorf("expected canonical_id doi:10.1234/paper1, got %s", p0.CanonicalID)
	}
	if p0.Title != "CRISPR Advances in Oncology" {
		t.Errorf("expected title 'CRISPR Advances in Oncology', got %s", p0.Title)
	}
	if len(p0.Authors) != 1 {
		t.Fatalf("expected 1 author, got %d", len(p0.Authors))
	}
	if p0.Authors[0].Name != "Jane Doe" {
		t.Errorf("expected author name 'Jane Doe', got %s", p0.Authors[0].Name)
	}
	if p0.Authors[0].Affiliation != "MIT" {
		t.Errorf("expected affiliation 'MIT', got %s", p0.Authors[0].Affiliation)
	}
	if p0.CitationCount != 150 {
		t.Errorf("expected citation_count 150, got %d", p0.CitationCount)
	}
	if !p0.OpenAccess {
		t.Error("expected open_access true for first paper")
	}

	// Verify the filter was constructed correctly.
	if capturedFilter.ReviewID == nil || *capturedFilter.ReviewID != reviewID {
		t.Errorf("expected filter review_id %s, got %v", reviewID, capturedFilter.ReviewID)
	}
	if capturedFilter.Limit != 10 {
		t.Errorf("expected filter limit 10, got %d", capturedFilter.Limit)
	}
}

func TestGetLiteratureReviewPapers_WithFilters(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:            reviewID,
				OrgID:         orgID,
				ProjectID:     projectID,
				Status:        domain.ReviewStatusCompleted,
				Configuration: domain.DefaultReviewConfiguration(),
				CreatedAt:     now,
				UpdatedAt:     now,
			}, nil
		},
	}

	var capturedFilter repository.PaperFilter
	paperRepo := &mockPaperRepo{
		listFn: func(_ context.Context, filter repository.PaperFilter) ([]*domain.Paper, int64, error) {
			capturedFilter = filter
			return nil, 0, nil
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, paperRepo, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet,
		buildPath("org-1", "proj-1", "/"+reviewID.String()+"/papers?source=semantic_scholar&ingestion_status=completed"),
		nil,
	)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if capturedFilter.Source == nil || *capturedFilter.Source != domain.SourceType("semantic_scholar") {
		t.Errorf("expected source filter semantic_scholar, got %v", capturedFilter.Source)
	}
	if capturedFilter.IngestionStatus == nil || *capturedFilter.IngestionStatus != domain.IngestionStatus("completed") {
		t.Errorf("expected ingestion_status filter completed, got %v", capturedFilter.IngestionStatus)
	}
}

func TestGetLiteratureReviewPapers_ReviewNotFound(t *testing.T) {
	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, _, _ string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return nil, domain.NewNotFoundError("review", id.String())
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet,
		buildPath("org-1", "proj-1", "/"+uuid.New().String()+"/papers"),
		nil,
	)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if resp["error"] != "resource not found" {
		t.Errorf("expected error 'resource not found', got %q", resp["error"])
	}
}

func TestGetLiteratureReviewPapers_InvalidUUID(t *testing.T) {
	srv := newTestHTTPServer(&mockWorkflowClient{}, &mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet,
		buildPath("org-1", "proj-1", "/not-a-uuid/papers"),
		nil,
	)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetLiteratureReviewPapers_RepoError(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:            reviewID,
				OrgID:         orgID,
				ProjectID:     projectID,
				Status:        domain.ReviewStatusCompleted,
				Configuration: domain.DefaultReviewConfiguration(),
				CreatedAt:     now,
				UpdatedAt:     now,
			}, nil
		},
	}

	paperRepo := &mockPaperRepo{
		listFn: func(_ context.Context, _ repository.PaperFilter) ([]*domain.Paper, int64, error) {
			return nil, 0, domain.ErrInternalError
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, paperRepo, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet,
		buildPath("org-1", "proj-1", "/"+reviewID.String()+"/papers"),
		nil,
	)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetLiteratureReviewPapers_Pagination(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:            reviewID,
				OrgID:         orgID,
				ProjectID:     projectID,
				Status:        domain.ReviewStatusCompleted,
				Configuration: domain.DefaultReviewConfiguration(),
				CreatedAt:     now,
				UpdatedAt:     now,
			}, nil
		},
	}

	allPapers := make([]*domain.Paper, 5)
	for i := range allPapers {
		allPapers[i] = &domain.Paper{
			ID:    uuid.New(),
			Title: "Paper",
		}
	}

	paperRepo := &mockPaperRepo{
		listFn: func(_ context.Context, filter repository.PaperFilter) ([]*domain.Paper, int64, error) {
			end := filter.Offset + filter.Limit
			if end > len(allPapers) {
				end = len(allPapers)
			}
			start := filter.Offset
			if start > len(allPapers) {
				start = len(allPapers)
			}
			return allPapers[start:end], 5, nil
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, paperRepo, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet,
		buildPath("org-1", "proj-1", "/"+reviewID.String()+"/papers?page_size=2"),
		nil,
	)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp listPapersResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Papers) != 2 {
		t.Fatalf("expected 2 papers on first page, got %d", len(resp.Papers))
	}
	if resp.NextPageToken == "" {
		t.Fatal("expected non-empty next_page_token for paginated results")
	}
	if resp.TotalCount != 5 {
		t.Errorf("expected total_count 5, got %d", resp.TotalCount)
	}
}

// ---------------------------------------------------------------------------
// Tests: getLiteratureReviewKeywords
// ---------------------------------------------------------------------------

func TestGetLiteratureReviewKeywords_Success(t *testing.T) {
	reviewID := uuid.New()
	kwID1 := uuid.New()
	kwID2 := uuid.New()
	now := time.Now()

	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			if id != reviewID {
				return nil, domain.NewNotFoundError("review", id.String())
			}
			return &domain.LiteratureReviewRequest{
				ID:            reviewID,
				OrgID:         orgID,
				ProjectID:     projectID,
				OriginalQuery: "CRISPR gene editing",
				Status:        domain.ReviewStatusCompleted,
				Configuration: domain.DefaultReviewConfiguration(),
				CreatedAt:     now,
				UpdatedAt:     now,
			}, nil
		},
	}

	keywords := []*domain.Keyword{
		{
			ID:                kwID1,
			Keyword:           "CRISPR-Cas9",
			NormalizedKeyword: "crispr-cas9",
			CreatedAt:         now,
		},
		{
			ID:                kwID2,
			Keyword:           "Gene Therapy",
			NormalizedKeyword: "gene therapy",
			CreatedAt:         now,
		},
	}

	var capturedFilter repository.KeywordFilter
	keywordRepo := &mockKeywordRepo{
		listFn: func(_ context.Context, filter repository.KeywordFilter) ([]*domain.Keyword, int64, error) {
			capturedFilter = filter
			return keywords, 2, nil
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, &mockPaperRepo{}, keywordRepo)

	req := httptest.NewRequest(http.MethodGet,
		buildPath("org-1", "proj-1", "/"+reviewID.String()+"/keywords?page_size=25"),
		nil,
	)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp listKeywordsResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Keywords) != 2 {
		t.Fatalf("expected 2 keywords, got %d", len(resp.Keywords))
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected total_count 2, got %d", resp.TotalCount)
	}
	if resp.NextPageToken != "" {
		t.Errorf("expected empty next_page_token, got %q", resp.NextPageToken)
	}

	// Verify first keyword response fields.
	k0 := resp.Keywords[0]
	if k0.ID != kwID1.String() {
		t.Errorf("expected keyword id %s, got %s", kwID1.String(), k0.ID)
	}
	if k0.Keyword != "CRISPR-Cas9" {
		t.Errorf("expected keyword 'CRISPR-Cas9', got %s", k0.Keyword)
	}
	if k0.NormalizedKeyword != "crispr-cas9" {
		t.Errorf("expected normalized_keyword 'crispr-cas9', got %s", k0.NormalizedKeyword)
	}

	// Verify filter was constructed correctly.
	if capturedFilter.ReviewID == nil || *capturedFilter.ReviewID != reviewID {
		t.Errorf("expected filter review_id %s, got %v", reviewID, capturedFilter.ReviewID)
	}
	if capturedFilter.Limit != 25 {
		t.Errorf("expected filter limit 25, got %d", capturedFilter.Limit)
	}
}

func TestGetLiteratureReviewKeywords_WithFilters(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:            reviewID,
				OrgID:         orgID,
				ProjectID:     projectID,
				Status:        domain.ReviewStatusCompleted,
				Configuration: domain.DefaultReviewConfiguration(),
				CreatedAt:     now,
				UpdatedAt:     now,
			}, nil
		},
	}

	var capturedFilter repository.KeywordFilter
	keywordRepo := &mockKeywordRepo{
		listFn: func(_ context.Context, filter repository.KeywordFilter) ([]*domain.Keyword, int64, error) {
			capturedFilter = filter
			return nil, 0, nil
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, &mockPaperRepo{}, keywordRepo)

	req := httptest.NewRequest(http.MethodGet,
		buildPath("org-1", "proj-1", "/"+reviewID.String()+"/keywords?extraction_round=2&source_type=llm"),
		nil,
	)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if capturedFilter.ExtractionRound == nil || *capturedFilter.ExtractionRound != 2 {
		t.Errorf("expected extraction_round filter 2, got %v", capturedFilter.ExtractionRound)
	}
	if capturedFilter.SourceType == nil || *capturedFilter.SourceType != "llm" {
		t.Errorf("expected source_type filter 'llm', got %v", capturedFilter.SourceType)
	}
}

func TestGetLiteratureReviewKeywords_ReviewNotFound(t *testing.T) {
	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, _, _ string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return nil, domain.NewNotFoundError("review", id.String())
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet,
		buildPath("org-1", "proj-1", "/"+uuid.New().String()+"/keywords"),
		nil,
	)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if resp["error"] != "resource not found" {
		t.Errorf("expected error 'resource not found', got %q", resp["error"])
	}
}

func TestGetLiteratureReviewKeywords_InvalidUUID(t *testing.T) {
	srv := newTestHTTPServer(&mockWorkflowClient{}, &mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet,
		buildPath("org-1", "proj-1", "/not-a-uuid/keywords"),
		nil,
	)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetLiteratureReviewKeywords_RepoError(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, _ uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return &domain.LiteratureReviewRequest{
				ID:            reviewID,
				OrgID:         orgID,
				ProjectID:     projectID,
				Status:        domain.ReviewStatusCompleted,
				Configuration: domain.DefaultReviewConfiguration(),
				CreatedAt:     now,
				UpdatedAt:     now,
			}, nil
		},
	}

	keywordRepo := &mockKeywordRepo{
		listFn: func(_ context.Context, _ repository.KeywordFilter) ([]*domain.Keyword, int64, error) {
			return nil, 0, domain.ErrInternalError
		},
	}

	srv := newTestHTTPServer(&mockWorkflowClient{}, reviewRepo, &mockPaperRepo{}, keywordRepo)

	req := httptest.NewRequest(http.MethodGet,
		buildPath("org-1", "proj-1", "/"+reviewID.String()+"/keywords"),
		nil,
	)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}
