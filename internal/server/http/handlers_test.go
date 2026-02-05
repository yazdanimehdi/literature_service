package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
	"github.com/helixir/literature-review-service/internal/temporal"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

// mockReviewRepo implements repository.ReviewRepository for HTTP handler tests.
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

// mockPaperRepo implements repository.PaperRepository for HTTP handler tests.
type mockPaperRepo struct {
	listFn func(ctx context.Context, filter repository.PaperFilter) ([]*domain.Paper, int64, error)
}

func (m *mockPaperRepo) Create(_ context.Context, _ *domain.Paper) (*domain.Paper, error)       { return nil, nil }
func (m *mockPaperRepo) GetByCanonicalID(_ context.Context, _ string) (*domain.Paper, error)     { return nil, nil }
func (m *mockPaperRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.Paper, error)           { return nil, nil }
func (m *mockPaperRepo) FindByIdentifier(_ context.Context, _ domain.IdentifierType, _ string) (*domain.Paper, error) {
	return nil, nil
}
func (m *mockPaperRepo) UpsertIdentifier(_ context.Context, _ uuid.UUID, _ domain.IdentifierType, _ string, _ domain.SourceType) error {
	return nil
}
func (m *mockPaperRepo) AddSource(_ context.Context, _ uuid.UUID, _ domain.SourceType, _ map[string]interface{}) error {
	return nil
}
func (m *mockPaperRepo) List(ctx context.Context, filter repository.PaperFilter) ([]*domain.Paper, int64, error) {
	if m.listFn != nil {
		return m.listFn(ctx, filter)
	}
	return nil, 0, nil
}
func (m *mockPaperRepo) MarkKeywordsExtracted(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockPaperRepo) BulkUpsert(_ context.Context, _ []*domain.Paper) ([]*domain.Paper, error) {
	return nil, nil
}

// mockKeywordRepo implements repository.KeywordRepository for HTTP handler tests.
type mockKeywordRepo struct {
	listFn func(ctx context.Context, filter repository.KeywordFilter) ([]*domain.Keyword, int64, error)
}

func (m *mockKeywordRepo) GetOrCreate(_ context.Context, _ string) (*domain.Keyword, error)  { return nil, nil }
func (m *mockKeywordRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.Keyword, error)   { return nil, nil }
func (m *mockKeywordRepo) GetByNormalized(_ context.Context, _ string) (*domain.Keyword, error) {
	return nil, nil
}
func (m *mockKeywordRepo) BulkGetOrCreate(_ context.Context, _ []string) ([]*domain.Keyword, error) {
	return nil, nil
}
func (m *mockKeywordRepo) RecordSearch(_ context.Context, _ *domain.KeywordSearch) error { return nil }
func (m *mockKeywordRepo) GetLastSearch(_ context.Context, _ uuid.UUID, _ domain.SourceType) (*domain.KeywordSearch, error) {
	return nil, nil
}
func (m *mockKeywordRepo) NeedsSearch(_ context.Context, _ uuid.UUID, _ domain.SourceType, _ time.Duration) (bool, error) {
	return false, nil
}
func (m *mockKeywordRepo) ListSearches(_ context.Context, _ repository.SearchFilter) ([]*domain.KeywordSearch, int64, error) {
	return nil, 0, nil
}
func (m *mockKeywordRepo) AddPaperMapping(_ context.Context, _ *domain.KeywordPaperMapping) error {
	return nil
}
func (m *mockKeywordRepo) BulkAddPaperMappings(_ context.Context, _ []*domain.KeywordPaperMapping) error {
	return nil
}
func (m *mockKeywordRepo) GetPapersForKeyword(_ context.Context, _ uuid.UUID, _, _ int) ([]*domain.Paper, int64, error) {
	return nil, 0, nil
}
func (m *mockKeywordRepo) List(ctx context.Context, filter repository.KeywordFilter) ([]*domain.Keyword, int64, error) {
	if m.listFn != nil {
		return m.listFn(ctx, filter)
	}
	return nil, 0, nil
}

// mockWorkflowClient implements WorkflowClient for HTTP handler tests.
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

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestHTTPServer creates a Server configured for testing with mocked dependencies.
func newTestHTTPServer(
	wfClient WorkflowClient,
	reviewRepo repository.ReviewRepository,
	paperRepo repository.PaperRepository,
	keywordRepo repository.KeywordRepository,
) *Server {
	logger := zerolog.Nop()
	s := &Server{
		workflowClient: wfClient,
		reviewRepo:     reviewRepo,
		paperRepo:      paperRepo,
		keywordRepo:    keywordRepo,
		logger:         logger,
	}
	s.router = s.buildRouter()
	return s
}

// serveHTTP dispatches a request through the test server's router and returns the recorder.
func serveHTTP(s *Server, r *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, r)
	return rr
}

// buildPath returns the full API path for a literature review endpoint.
func buildPath(orgID, projectID, suffix string) string {
	return "/api/v1/orgs/" + orgID + "/projects/" + projectID + "/literature-reviews" + suffix
}

// injectTenantContext creates a chi URL context with orgID and projectID params
// and injects tenant context values.
func injectTenantContext(r *http.Request, orgID, projectID string) *http.Request {
	ctx := context.WithValue(r.Context(), ctxKeyOrgID, orgID)
	ctx = context.WithValue(ctx, ctxKeyProjectID, projectID)
	return r.WithContext(ctx)
}

// decodeJSON decodes a JSON response body into the given target.
func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, target interface{}) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(target); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: startLiteratureReview
// ---------------------------------------------------------------------------

func TestStartLiteratureReview_Success(t *testing.T) {
	var createdReview *domain.LiteratureReviewRequest

	reviewRepo := &mockReviewRepo{
		createFn: func(_ context.Context, review *domain.LiteratureReviewRequest) error {
			createdReview = review
			return nil
		},
		updateFn: func(_ context.Context, _, _ string, _ uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
			return fn(&domain.LiteratureReviewRequest{})
		},
	}

	var capturedReq temporal.ReviewWorkflowRequest
	wfClient := &mockWorkflowClient{
		startFn: func(_ context.Context, req temporal.ReviewWorkflowRequest, _ interface{}, _ interface{}) (string, string, error) {
			capturedReq = req
			return "wf-review-" + req.RequestID, "run-abc123", nil
		},
	}

	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	body := `{"query":"CRISPR gene editing in cancer treatment"}`
	req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp startReviewResponse
	decodeJSON(t, rr, &resp)

	if resp.ReviewID == "" {
		t.Error("expected review_id to be set")
	}
	if resp.WorkflowID == "" {
		t.Error("expected workflow_id to be set")
	}
	if resp.Status != string(domain.ReviewStatusPending) {
		t.Errorf("expected status %q, got %q", domain.ReviewStatusPending, resp.Status)
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
	if createdReview.ProjectID != "proj-1" {
		t.Errorf("expected project_id proj-1, got %s", createdReview.ProjectID)
	}
	if createdReview.Status != domain.ReviewStatusPending {
		t.Errorf("expected pending status, got %s", createdReview.Status)
	}
	if createdReview.OriginalQuery != "CRISPR gene editing in cancer treatment" {
		t.Errorf("expected query to match, got %s", createdReview.OriginalQuery)
	}

	// Verify the workflow request was properly constructed.
	if capturedReq.OrgID != "org-1" {
		t.Errorf("expected workflow req org_id org-1, got %s", capturedReq.OrgID)
	}
	if capturedReq.Query != "CRISPR gene editing in cancer treatment" {
		t.Errorf("expected workflow req query to match, got %s", capturedReq.Query)
	}
}

func TestStartLiteratureReview_MissingQuery(t *testing.T) {
	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, &mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})

	body := `{"query":""}`
	req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if resp["error"] != "query is required" {
		t.Errorf("expected error message 'query is required', got %q", resp["error"])
	}
}

func TestStartLiteratureReview_QueryTooLong(t *testing.T) {
	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, &mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})

	longQuery := make([]byte, maxQueryLength+1)
	for i := range longQuery {
		longQuery[i] = 'a'
	}
	bodyMap := map[string]string{"query": string(longQuery)}
	bodyBytes, _ := json.Marshal(bodyMap)
	req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStartLiteratureReview_InvalidJSON(t *testing.T) {
	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, &mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBufferString("{invalid json"))
	req.Header.Set("Content-Type", "application/json")

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStartLiteratureReview_CreateRepoError(t *testing.T) {
	reviewRepo := &mockReviewRepo{
		createFn: func(_ context.Context, _ *domain.LiteratureReviewRequest) error {
			return domain.ErrInternalError
		},
	}
	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	body := `{"query":"some query"}`
	req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStartLiteratureReview_WorkflowError(t *testing.T) {
	reviewRepo := &mockReviewRepo{
		createFn: func(_ context.Context, _ *domain.LiteratureReviewRequest) error {
			return nil
		},
	}
	wfClient := &mockWorkflowClient{
		startFn: func(_ context.Context, _ temporal.ReviewWorkflowRequest, _ interface{}, _ interface{}) (string, string, error) {
			return "", "", temporal.ErrConnectionFailed
		},
	}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	body := `{"query":"some query"}`
	req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestStartLiteratureReview_WithOptionalFields(t *testing.T) {
	var createdReview *domain.LiteratureReviewRequest

	reviewRepo := &mockReviewRepo{
		createFn: func(_ context.Context, review *domain.LiteratureReviewRequest) error {
			createdReview = review
			return nil
		},
		updateFn: func(_ context.Context, _, _ string, _ uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
			return fn(&domain.LiteratureReviewRequest{})
		},
	}

	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	body := `{
		"query": "mRNA vaccine development",
		"initial_keyword_count": 15,
		"paper_keyword_count": 5,
		"max_expansion_depth": 3,
		"source_filters": ["semantic_scholar", "pubmed"],
		"date_from": "2023-01-01T00:00:00Z",
		"date_to": "2024-01-01T00:00:00Z"
	}`
	req := httptest.NewRequest(http.MethodPost, buildPath("org-1", "proj-1", ""), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	if createdReview == nil {
		t.Fatal("expected createFn to be called")
	}
	if createdReview.Configuration.MaxKeywordsPerRound != 15 {
		t.Errorf("expected MaxKeywordsPerRound 15, got %d", createdReview.Configuration.MaxKeywordsPerRound)
	}
	if createdReview.Configuration.PaperKeywordCount != 5 {
		t.Errorf("expected PaperKeywordCount 5, got %d", createdReview.Configuration.PaperKeywordCount)
	}
	if createdReview.Configuration.MaxExpansionDepth != 3 {
		t.Errorf("expected MaxExpansionDepth 3, got %d", createdReview.Configuration.MaxExpansionDepth)
	}
	if len(createdReview.Configuration.Sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(createdReview.Configuration.Sources))
	}
	if createdReview.Configuration.DateFrom == nil {
		t.Error("expected DateFrom to be set")
	}
	if createdReview.Configuration.DateTo == nil {
		t.Error("expected DateTo to be set")
	}
}

// ---------------------------------------------------------------------------
// Tests: getLiteratureReviewStatus
// ---------------------------------------------------------------------------

func TestGetLiteratureReviewStatus_Success(t *testing.T) {
	reviewID := uuid.New()
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
				Status:        domain.ReviewStatusSearching,
				Configuration: domain.DefaultReviewConfiguration(),
				CreatedAt:     now,
				UpdatedAt:     now,
			}, nil
		},
	}

	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", "/"+reviewID.String()), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp reviewStatusResponse
	decodeJSON(t, rr, &resp)

	if resp.ReviewID != reviewID.String() {
		t.Errorf("expected review_id %s, got %s", reviewID.String(), resp.ReviewID)
	}
	if resp.Status != string(domain.ReviewStatusSearching) {
		t.Errorf("expected status %q, got %q", domain.ReviewStatusSearching, resp.Status)
	}
	if resp.Config == nil {
		t.Error("expected configuration to be set")
	}
}

func TestGetLiteratureReviewStatus_NotFound(t *testing.T) {
	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, _, _ string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return nil, domain.NewNotFoundError("review", id.String())
		},
	}

	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", "/"+uuid.New().String()), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetLiteratureReviewStatus_InvalidUUID(t *testing.T) {
	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, &mockReviewRepo{}, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", "/not-a-uuid"), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Tests: cancelLiteratureReview
// ---------------------------------------------------------------------------

func TestCancelLiteratureReview_Success(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	var signalledWorkflowID string
	var signalledSignalName string

	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			if id != reviewID {
				return nil, domain.NewNotFoundError("review", id.String())
			}
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

	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	body := `{"reason":"no longer needed"}`
	req := httptest.NewRequest(http.MethodDelete, buildPath("org-1", "proj-1", "/"+reviewID.String()), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp cancelReviewResponse
	decodeJSON(t, rr, &resp)

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

func TestCancelLiteratureReview_TerminalState(t *testing.T) {
	reviewID := uuid.New()
	now := time.Now()

	reviewRepo := &mockReviewRepo{
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

	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodDelete, buildPath("org-1", "proj-1", "/"+reviewID.String()), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCancelLiteratureReview_NotFound(t *testing.T) {
	reviewRepo := &mockReviewRepo{
		getFn: func(_ context.Context, _, _ string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
			return nil, domain.NewNotFoundError("review", id.String())
		},
	}

	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodDelete, buildPath("org-1", "proj-1", "/"+uuid.New().String()), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCancelLiteratureReview_SignalError(t *testing.T) {
	reviewID := uuid.New()

	reviewRepo := &mockReviewRepo{
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

	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodDelete, buildPath("org-1", "proj-1", "/"+reviewID.String()), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Tests: listLiteratureReviews
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

	reviewRepo := &mockReviewRepo{
		listFn: func(_ context.Context, filter repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
			if filter.OrgID != "org-1" || filter.ProjectID != "proj-1" {
				t.Errorf("unexpected filter org/project: %s/%s", filter.OrgID, filter.ProjectID)
			}
			return reviews, 2, nil
		},
	}

	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", "?page_size=10"), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp listReviewsResponse
	decodeJSON(t, rr, &resp)

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
	if s0.ReviewID != reviews[0].ID.String() {
		t.Errorf("expected review_id %s, got %s", reviews[0].ID.String(), s0.ReviewID)
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

func TestListLiteratureReviews_Empty(t *testing.T) {
	reviewRepo := &mockReviewRepo{
		listFn: func(_ context.Context, _ repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
			return nil, 0, nil
		},
	}

	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", ""), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp listReviewsResponse
	decodeJSON(t, rr, &resp)

	if resp.TotalCount != 0 {
		t.Errorf("expected total_count 0, got %d", resp.TotalCount)
	}
	if resp.NextPageToken != "" {
		t.Errorf("expected empty next_page_token, got %q", resp.NextPageToken)
	}
}

func TestListLiteratureReviews_WithStatusFilter(t *testing.T) {
	now := time.Now()
	reviews := []*domain.LiteratureReviewRequest{
		{
			ID:            uuid.New(),
			OrgID:         "org-1",
			ProjectID:     "proj-1",
			OriginalQuery: "CRISPR",
			Status:        domain.ReviewStatusFailed,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}

	var capturedFilter repository.ReviewFilter
	reviewRepo := &mockReviewRepo{
		listFn: func(_ context.Context, filter repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
			capturedFilter = filter
			return reviews, 1, nil
		},
	}

	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", "?status=failed"), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if len(capturedFilter.Status) != 1 || capturedFilter.Status[0] != domain.ReviewStatusFailed {
		t.Errorf("expected status filter [failed], got %v", capturedFilter.Status)
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

	reviewRepo := &mockReviewRepo{
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

	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", "?page_size=2"), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp listReviewsResponse
	decodeJSON(t, rr, &resp)

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

func TestListLiteratureReviews_RepoError(t *testing.T) {
	reviewRepo := &mockReviewRepo{
		listFn: func(_ context.Context, _ repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
			return nil, 0, domain.ErrInternalError
		},
	}

	wfClient := &mockWorkflowClient{}
	srv := newTestHTTPServer(wfClient, reviewRepo, &mockPaperRepo{}, &mockKeywordRepo{})

	req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", ""), nil)

	rr := serveHTTP(srv, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Tests: helper functions
// ---------------------------------------------------------------------------

func TestWriteDomainError_Mappings(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
	}{
		{"not found", domain.ErrNotFound, http.StatusNotFound},
		{"not found wrapped", domain.NewNotFoundError("review", "123"), http.StatusNotFound},
		{"invalid input", domain.ErrInvalidInput, http.StatusBadRequest},
		{"already exists", domain.ErrAlreadyExists, http.StatusConflict},
		{"unauthorized", domain.ErrUnauthorized, http.StatusUnauthorized},
		{"forbidden", domain.ErrForbidden, http.StatusForbidden},
		{"rate limited", domain.ErrRateLimited, http.StatusTooManyRequests},
		{"service unavailable", domain.ErrServiceUnavailable, http.StatusServiceUnavailable},
		{"cancelled", domain.ErrCancelled, http.StatusConflict},
		{"workflow not found", temporal.ErrWorkflowNotFound, http.StatusNotFound},
		{"workflow already started", temporal.ErrWorkflowAlreadyStarted, http.StatusConflict},
		{"internal error", domain.ErrInternalError, http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeDomainError(rr, tc.err)
			if rr.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rr.Code)
			}
		})
	}
}

func TestParsePaginationParams_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	limit, offset := parsePaginationParams(req)
	if limit != defaultPageSize {
		t.Errorf("expected default limit %d, got %d", defaultPageSize, limit)
	}
	if offset != 0 {
		t.Errorf("expected offset 0, got %d", offset)
	}
}

func TestParsePaginationParams_Custom(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?page_size=25", nil)
	limit, offset := parsePaginationParams(req)
	if limit != 25 {
		t.Errorf("expected limit 25, got %d", limit)
	}
	if offset != 0 {
		t.Errorf("expected offset 0, got %d", offset)
	}
}

func TestParsePaginationParams_MaxPageSize(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?page_size=500", nil)
	limit, _ := parsePaginationParams(req)
	if limit != maxPageSize {
		t.Errorf("expected max limit %d, got %d", maxPageSize, limit)
	}
}

func TestEncodeHTTPPageToken(t *testing.T) {
	// More results available.
	token := encodeHTTPPageToken(0, 10, 25)
	if token == "" {
		t.Error("expected non-empty token when more results available")
	}

	// No more results.
	token = encodeHTTPPageToken(0, 10, 5)
	if token != "" {
		t.Errorf("expected empty token when no more results, got %q", token)
	}

	// Exactly at boundary.
	token = encodeHTTPPageToken(0, 10, 10)
	if token != "" {
		t.Errorf("expected empty token at exact boundary, got %q", token)
	}
}

func TestParsePaginationParams_EdgeCases(t *testing.T) {
	t.Run("invalid non-numeric page_size uses default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test?page_size=abc", nil)
		limit, offset := parsePaginationParams(req)
		if limit != defaultPageSize {
			t.Errorf("expected default limit %d for non-numeric page_size, got %d", defaultPageSize, limit)
		}
		if offset != 0 {
			t.Errorf("expected offset 0, got %d", offset)
		}
	})

	t.Run("negative page_size uses default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test?page_size=-5", nil)
		limit, offset := parsePaginationParams(req)
		if limit != defaultPageSize {
			t.Errorf("expected default limit %d for negative page_size, got %d", defaultPageSize, limit)
		}
		if offset != 0 {
			t.Errorf("expected offset 0, got %d", offset)
		}
	})

	t.Run("valid page_token decodes to correct offset", func(t *testing.T) {
		// Encode offset 75 as base64, simulating what encodeHTTPPageToken produces.
		encodedToken := base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(75)))
		req := httptest.NewRequest(http.MethodGet, "/test?page_token="+encodedToken, nil)
		limit, offset := parsePaginationParams(req)
		if limit != defaultPageSize {
			t.Errorf("expected default limit %d, got %d", defaultPageSize, limit)
		}
		if offset != 75 {
			t.Errorf("expected offset 75 from decoded page_token, got %d", offset)
		}
	})

	t.Run("invalid page_token keeps offset at zero", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test?page_token=not-valid-base64!!!", nil)
		limit, offset := parsePaginationParams(req)
		if limit != defaultPageSize {
			t.Errorf("expected default limit %d, got %d", defaultPageSize, limit)
		}
		if offset != 0 {
			t.Errorf("expected offset 0 for invalid page_token, got %d", offset)
		}
	})
}

// ---------------------------------------------------------------------------
// Tests: concurrent stress
// ---------------------------------------------------------------------------

func TestListReviews_ConcurrentRequests(t *testing.T) {
	repo := &mockReviewRepo{
		listFn: func(_ context.Context, _ repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
			return []*domain.LiteratureReviewRequest{}, 0, nil
		},
	}
	srv := newTestHTTPServer(nil, repo, &mockPaperRepo{}, &mockKeywordRepo{})

	const concurrency = 50
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, buildPath("org-1", "proj-1", ""), nil)
			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				errs <- fmt.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
				return
			}
			errs <- nil
		}()
	}

	for i := 0; i < concurrency; i++ {
		if err := <-errs; err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}
