package activities

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
)

// ---------------------------------------------------------------------------
// Mock: ReviewRepository
// ---------------------------------------------------------------------------

type mockReviewRepository struct {
	mock.Mock
}

func (m *mockReviewRepository) Create(ctx context.Context, review *domain.LiteratureReviewRequest) error {
	args := m.Called(ctx, review)
	return args.Error(0)
}

func (m *mockReviewRepository) Get(ctx context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
	args := m.Called(ctx, orgID, projectID, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.LiteratureReviewRequest), args.Error(1)
}

func (m *mockReviewRepository) Update(ctx context.Context, orgID, projectID string, id uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
	args := m.Called(ctx, orgID, projectID, id, fn)
	return args.Error(0)
}

func (m *mockReviewRepository) UpdateStatus(ctx context.Context, orgID, projectID string, id uuid.UUID, status domain.ReviewStatus, errorMsg string) error {
	args := m.Called(ctx, orgID, projectID, id, status, errorMsg)
	return args.Error(0)
}

func (m *mockReviewRepository) List(ctx context.Context, filter repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*domain.LiteratureReviewRequest), args.Get(1).(int64), args.Error(2)
}

func (m *mockReviewRepository) IncrementCounters(ctx context.Context, orgID, projectID string, id uuid.UUID, papersFound, papersIngested int) error {
	args := m.Called(ctx, orgID, projectID, id, papersFound, papersIngested)
	return args.Error(0)
}

func (m *mockReviewRepository) GetByWorkflowID(ctx context.Context, workflowID string) (*domain.LiteratureReviewRequest, error) {
	args := m.Called(ctx, workflowID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.LiteratureReviewRequest), args.Error(1)
}

// ---------------------------------------------------------------------------
// Mock: KeywordRepository
// ---------------------------------------------------------------------------

type mockKeywordRepository struct {
	mock.Mock
}

func (m *mockKeywordRepository) GetOrCreate(ctx context.Context, keyword string) (*domain.Keyword, error) {
	args := m.Called(ctx, keyword)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Keyword), args.Error(1)
}

func (m *mockKeywordRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Keyword, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Keyword), args.Error(1)
}

func (m *mockKeywordRepository) GetByNormalized(ctx context.Context, normalized string) (*domain.Keyword, error) {
	args := m.Called(ctx, normalized)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Keyword), args.Error(1)
}

func (m *mockKeywordRepository) BulkGetOrCreate(ctx context.Context, keywords []string) ([]*domain.Keyword, error) {
	args := m.Called(ctx, keywords)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Keyword), args.Error(1)
}

func (m *mockKeywordRepository) RecordSearch(ctx context.Context, search *domain.KeywordSearch) error {
	args := m.Called(ctx, search)
	return args.Error(0)
}

func (m *mockKeywordRepository) GetLastSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI domain.SourceType) (*domain.KeywordSearch, error) {
	args := m.Called(ctx, keywordID, sourceAPI)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.KeywordSearch), args.Error(1)
}

func (m *mockKeywordRepository) NeedsSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI domain.SourceType, maxAge time.Duration) (bool, error) {
	args := m.Called(ctx, keywordID, sourceAPI, maxAge)
	return args.Bool(0), args.Error(1)
}

func (m *mockKeywordRepository) ListSearches(ctx context.Context, filter repository.SearchFilter) ([]*domain.KeywordSearch, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*domain.KeywordSearch), args.Get(1).(int64), args.Error(2)
}

func (m *mockKeywordRepository) AddPaperMapping(ctx context.Context, mapping *domain.KeywordPaperMapping) error {
	args := m.Called(ctx, mapping)
	return args.Error(0)
}

func (m *mockKeywordRepository) BulkAddPaperMappings(ctx context.Context, mappings []*domain.KeywordPaperMapping) error {
	args := m.Called(ctx, mappings)
	return args.Error(0)
}

func (m *mockKeywordRepository) GetPapersForKeyword(ctx context.Context, keywordID uuid.UUID, limit, offset int) ([]*domain.Paper, int64, error) {
	args := m.Called(ctx, keywordID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*domain.Paper), args.Get(1).(int64), args.Error(2)
}

func (m *mockKeywordRepository) List(ctx context.Context, filter repository.KeywordFilter) ([]*domain.Keyword, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*domain.Keyword), args.Get(1).(int64), args.Error(2)
}

// ---------------------------------------------------------------------------
// Mock: PaperRepository
// ---------------------------------------------------------------------------

type mockPaperRepository struct {
	mock.Mock
}

func (m *mockPaperRepository) Create(ctx context.Context, paper *domain.Paper) (*domain.Paper, error) {
	args := m.Called(ctx, paper)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Paper), args.Error(1)
}

func (m *mockPaperRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*domain.Paper, error) {
	args := m.Called(ctx, canonicalID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Paper), args.Error(1)
}

func (m *mockPaperRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Paper, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Paper), args.Error(1)
}

func (m *mockPaperRepository) FindByIdentifier(ctx context.Context, idType domain.IdentifierType, value string) (*domain.Paper, error) {
	args := m.Called(ctx, idType, value)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Paper), args.Error(1)
}

func (m *mockPaperRepository) UpsertIdentifier(ctx context.Context, paperID uuid.UUID, idType domain.IdentifierType, value string, sourceAPI domain.SourceType) error {
	args := m.Called(ctx, paperID, idType, value, sourceAPI)
	return args.Error(0)
}

func (m *mockPaperRepository) AddSource(ctx context.Context, paperID uuid.UUID, sourceAPI domain.SourceType, metadata map[string]interface{}) error {
	args := m.Called(ctx, paperID, sourceAPI, metadata)
	return args.Error(0)
}

func (m *mockPaperRepository) List(ctx context.Context, filter repository.PaperFilter) ([]*domain.Paper, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*domain.Paper), args.Get(1).(int64), args.Error(2)
}

func (m *mockPaperRepository) MarkKeywordsExtracted(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockPaperRepository) BulkUpsert(ctx context.Context, papers []*domain.Paper) ([]*domain.Paper, error) {
	args := m.Called(ctx, papers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Paper), args.Error(1)
}

func (m *mockPaperRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.Paper, error) {
	args := m.Called(ctx, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Paper), args.Error(1)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestUpdateStatus_Success(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()

	reviewRepo.On("UpdateStatus", mock.Anything, "org-1", "proj-1", requestID, domain.ReviewStatusSearching, "").
		Return(nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdateStatus)

	input := UpdateStatusInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: requestID,
		Status:    domain.ReviewStatusSearching,
		ErrorMsg:  "",
	}

	_, err := env.ExecuteActivity(act.UpdateStatus, input)
	require.NoError(t, err)

	reviewRepo.AssertExpectations(t)
}

func TestSaveKeywords_Success(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()
	kw1ID := uuid.New()
	kw2ID := uuid.New()
	kw3ID := uuid.New()

	keywordRepo.On("BulkGetOrCreate", mock.Anything, []string{"CRISPR", "gene editing", "Cas9"}).
		Return([]*domain.Keyword{
			{ID: kw1ID, Keyword: "CRISPR", NormalizedKeyword: "crispr", CreatedAt: time.Now()},
			{ID: kw2ID, Keyword: "gene editing", NormalizedKeyword: "gene editing", CreatedAt: time.Now()},
			{ID: kw3ID, Keyword: "Cas9", NormalizedKeyword: "cas9", CreatedAt: time.Now()},
		}, nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.SaveKeywords)

	input := SaveKeywordsInput{
		RequestID:       requestID,
		Keywords:        []string{"CRISPR", "gene editing", "Cas9"},
		ExtractionRound: 1,
		SourceType:      "query",
	}

	result, err := env.ExecuteActivity(act.SaveKeywords, input)
	require.NoError(t, err)

	var output SaveKeywordsOutput
	require.NoError(t, result.Get(&output))

	assert.Len(t, output.KeywordIDs, 3)
	assert.Equal(t, kw1ID, output.KeywordIDs[0])
	assert.Equal(t, kw2ID, output.KeywordIDs[1])
	assert.Equal(t, kw3ID, output.KeywordIDs[2])
	assert.Equal(t, 3, output.NewCount)

	keywordRepo.AssertExpectations(t)
}

func TestSavePapers_Success(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()
	paper1ID := uuid.New()
	paper2ID := uuid.New()

	inputPapers := []*domain.Paper{
		{CanonicalID: "doi:10.1234/test1", Title: "Paper One"},
		{CanonicalID: "doi:10.1234/test2", Title: "Paper Two"},
	}

	savedPapers := []*domain.Paper{
		{ID: paper1ID, CanonicalID: "doi:10.1234/test1", Title: "Paper One", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: paper2ID, CanonicalID: "doi:10.1234/test2", Title: "Paper Two", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}

	paperRepo.On("BulkUpsert", mock.Anything, inputPapers).
		Return(savedPapers, nil)
	reviewRepo.On("IncrementCounters", mock.Anything, "org-1", "proj-1", requestID, 2, 0).
		Return(nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.SavePapers)

	input := SavePapersInput{
		RequestID:           requestID,
		OrgID:               "org-1",
		ProjectID:           "proj-1",
		Papers:              inputPapers,
		DiscoveredViaSource: domain.SourceTypeSemanticScholar,
		ExpansionDepth:      0,
	}

	result, err := env.ExecuteActivity(act.SavePapers, input)
	require.NoError(t, err)

	var output SavePapersOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, 2, output.SavedCount)
	assert.Equal(t, 0, output.DuplicateCount)

	paperRepo.AssertExpectations(t)
	reviewRepo.AssertExpectations(t)
}

func TestSavePapers_Empty(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.SavePapers)

	input := SavePapersInput{
		RequestID:           uuid.New(),
		OrgID:               "org-1",
		ProjectID:           "proj-1",
		Papers:              []*domain.Paper{},
		DiscoveredViaSource: domain.SourceTypeOpenAlex,
		ExpansionDepth:      1,
	}

	result, err := env.ExecuteActivity(act.SavePapers, input)
	require.NoError(t, err)

	var output SavePapersOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, 0, output.SavedCount)
	assert.Equal(t, 0, output.DuplicateCount)

	// Verify no repository methods were called.
	paperRepo.AssertNotCalled(t, "BulkUpsert", mock.Anything, mock.Anything)
	reviewRepo.AssertNotCalled(t, "IncrementCounters", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestUpdateStatus_Error(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()

	reviewRepo.On("UpdateStatus", mock.Anything, "org-1", "proj-1", requestID, domain.ReviewStatusFailed, "some error").
		Return(assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdateStatus)

	input := UpdateStatusInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: requestID,
		Status:    domain.ReviewStatusFailed,
		ErrorMsg:  "some error",
	}

	_, err := env.ExecuteActivity(act.UpdateStatus, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update review status")

	reviewRepo.AssertExpectations(t)
}

func TestUpdateStatus_FailedMetrics(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()

	reviewRepo.On("UpdateStatus", mock.Anything, "org-1", "proj-1", requestID, domain.ReviewStatusFailed, "workflow timed out").
		Return(nil)

	// nil metrics should not panic.
	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdateStatus)

	input := UpdateStatusInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: requestID,
		Status:    domain.ReviewStatusFailed,
		ErrorMsg:  "workflow timed out",
	}

	_, err := env.ExecuteActivity(act.UpdateStatus, input)
	require.NoError(t, err)

	reviewRepo.AssertExpectations(t)
}

func TestUpdateStatus_CancelledMetrics(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()

	reviewRepo.On("UpdateStatus", mock.Anything, "org-1", "proj-1", requestID, domain.ReviewStatusCancelled, "").
		Return(nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdateStatus)

	input := UpdateStatusInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: requestID,
		Status:    domain.ReviewStatusCancelled,
		ErrorMsg:  "",
	}

	_, err := env.ExecuteActivity(act.UpdateStatus, input)
	require.NoError(t, err)

	reviewRepo.AssertExpectations(t)
}

func TestSaveKeywords_Error(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	keywordRepo.On("BulkGetOrCreate", mock.Anything, []string{"CRISPR"}).
		Return(nil, assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.SaveKeywords)

	input := SaveKeywordsInput{
		RequestID:       uuid.New(),
		Keywords:        []string{"CRISPR"},
		ExtractionRound: 1,
		SourceType:      "query",
	}

	_, err := env.ExecuteActivity(act.SaveKeywords, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bulk get or create keywords")

	keywordRepo.AssertExpectations(t)
}

func TestSavePapers_BulkUpsertError(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	inputPapers := []*domain.Paper{
		{CanonicalID: "doi:10.1234/test1", Title: "Paper One"},
	}

	paperRepo.On("BulkUpsert", mock.Anything, inputPapers).
		Return(nil, assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.SavePapers)

	input := SavePapersInput{
		RequestID:           uuid.New(),
		OrgID:               "org-1",
		ProjectID:           "proj-1",
		Papers:              inputPapers,
		DiscoveredViaSource: domain.SourceTypeSemanticScholar,
		ExpansionDepth:      0,
	}

	_, err := env.ExecuteActivity(act.SavePapers, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bulk upsert papers")

	paperRepo.AssertExpectations(t)
}

func TestSavePapers_IncrementCountersError(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()
	inputPapers := []*domain.Paper{
		{CanonicalID: "doi:10.1234/test1", Title: "Paper One"},
	}

	savedPapers := []*domain.Paper{
		{ID: uuid.New(), CanonicalID: "doi:10.1234/test1", Title: "Paper One", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}

	paperRepo.On("BulkUpsert", mock.Anything, inputPapers).
		Return(savedPapers, nil)
	reviewRepo.On("IncrementCounters", mock.Anything, "org-1", "proj-1", requestID, 1, 0).
		Return(assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.SavePapers)

	input := SavePapersInput{
		RequestID:           requestID,
		OrgID:               "org-1",
		ProjectID:           "proj-1",
		Papers:              inputPapers,
		DiscoveredViaSource: domain.SourceTypePubMed,
		ExpansionDepth:      1,
	}

	_, err := env.ExecuteActivity(act.SavePapers, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "increment review counters")

	paperRepo.AssertExpectations(t)
	reviewRepo.AssertExpectations(t)
}

func TestIncrementCounters_Error(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()

	reviewRepo.On("IncrementCounters", mock.Anything, "org-1", "proj-1", requestID, 10, 5).
		Return(assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.IncrementCounters)

	input := IncrementCountersInput{
		OrgID:          "org-1",
		ProjectID:      "proj-1",
		RequestID:      requestID,
		PapersFound:    10,
		PapersIngested: 5,
	}

	_, err := env.ExecuteActivity(act.IncrementCounters, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "increment review counters")

	reviewRepo.AssertExpectations(t)
}

func TestIncrementCounters_Success(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()

	reviewRepo.On("IncrementCounters", mock.Anything, "org-1", "proj-1", requestID, 10, 5).
		Return(nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.IncrementCounters)

	input := IncrementCountersInput{
		OrgID:          "org-1",
		ProjectID:      "proj-1",
		RequestID:      requestID,
		PapersFound:    10,
		PapersIngested: 5,
	}

	_, err := env.ExecuteActivity(act.IncrementCounters, input)
	require.NoError(t, err)

	reviewRepo.AssertExpectations(t)
}
