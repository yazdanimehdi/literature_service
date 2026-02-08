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

func (m *mockReviewRepository) FindPausedByReason(ctx context.Context, orgID, projectID string, reason domain.PauseReason) ([]*domain.LiteratureReviewRequest, error) {
	args := m.Called(ctx, orgID, projectID, reason)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.LiteratureReviewRequest), args.Error(1)
}

func (m *mockReviewRepository) UpdatePauseState(ctx context.Context, orgID, projectID string, requestID uuid.UUID, status domain.ReviewStatus, pauseReason domain.PauseReason, pausedAtPhase string) error {
	args := m.Called(ctx, orgID, projectID, requestID, status, pauseReason, pausedAtPhase)
	return args.Error(0)
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

func (m *mockKeywordRepository) GetPapersForKeywordAndSource(ctx context.Context, keywordID uuid.UUID, source domain.SourceType, limit, offset int) ([]*domain.Paper, int64, error) {
	args := m.Called(ctx, keywordID, source, limit, offset)
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

func (m *mockPaperRepository) UpdateIngestionResult(ctx context.Context, paperID uuid.UUID, fileID uuid.UUID, ingestionRunID string) error {
	args := m.Called(ctx, paperID, fileID, ingestionRunID)
	return args.Error(0)
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

func TestUpdatePaperIngestionResults_Success(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	paperID := uuid.New()
	fileID := uuid.New()

	paperRepo.On("UpdateIngestionResult", mock.Anything, paperID, fileID, "run-123").
		Return(nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdatePaperIngestionResults)

	input := UpdatePaperIngestionResultsInput{
		Results: []PaperIngestionResult{
			{
				PaperID:        paperID,
				FileID:         fileID.String(),
				IngestionRunID: "run-123",
				Status:         "RUN_STATUS_PENDING",
			},
		},
	}

	result, err := env.ExecuteActivity(act.UpdatePaperIngestionResults, input)
	require.NoError(t, err)

	var output UpdatePaperIngestionResultsOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, 1, output.Updated)
	assert.Equal(t, 0, output.Skipped)
	assert.Equal(t, 0, output.Failed)

	paperRepo.AssertExpectations(t)
}

func TestUpdatePaperIngestionResults_SkipsWithoutFileID(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdatePaperIngestionResults)

	input := UpdatePaperIngestionResultsInput{
		Results: []PaperIngestionResult{
			{
				PaperID:        uuid.New(),
				FileID:         "",
				IngestionRunID: "run-123",
			},
		},
	}

	result, err := env.ExecuteActivity(act.UpdatePaperIngestionResults, input)
	require.NoError(t, err)

	var output UpdatePaperIngestionResultsOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, 0, output.Updated)
	assert.Equal(t, 1, output.Skipped)
	assert.Equal(t, 0, output.Failed)

	// Verify repo was not called
	paperRepo.AssertNotCalled(t, "UpdateIngestionResult", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestUpdatePaperIngestionResults_SkipsWithoutIngestionRunID(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdatePaperIngestionResults)

	input := UpdatePaperIngestionResultsInput{
		Results: []PaperIngestionResult{
			{
				PaperID:        uuid.New(),
				FileID:         uuid.New().String(),
				IngestionRunID: "",
			},
		},
	}

	result, err := env.ExecuteActivity(act.UpdatePaperIngestionResults, input)
	require.NoError(t, err)

	var output UpdatePaperIngestionResultsOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, 0, output.Updated)
	assert.Equal(t, 1, output.Skipped)
	assert.Equal(t, 0, output.Failed)

	paperRepo.AssertNotCalled(t, "UpdateIngestionResult", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestUpdatePaperIngestionResults_InvalidFileID(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdatePaperIngestionResults)

	input := UpdatePaperIngestionResultsInput{
		Results: []PaperIngestionResult{
			{
				PaperID:        uuid.New(),
				FileID:         "not-a-uuid",
				IngestionRunID: "run-123",
			},
		},
	}

	result, err := env.ExecuteActivity(act.UpdatePaperIngestionResults, input)
	require.NoError(t, err)

	var output UpdatePaperIngestionResultsOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, 0, output.Updated)
	assert.Equal(t, 0, output.Skipped)
	assert.Equal(t, 1, output.Failed)

	paperRepo.AssertNotCalled(t, "UpdateIngestionResult", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestUpdatePaperIngestionResults_RepoError(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	paperID := uuid.New()
	fileID := uuid.New()

	paperRepo.On("UpdateIngestionResult", mock.Anything, paperID, fileID, "run-123").
		Return(assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdatePaperIngestionResults)

	input := UpdatePaperIngestionResultsInput{
		Results: []PaperIngestionResult{
			{
				PaperID:        paperID,
				FileID:         fileID.String(),
				IngestionRunID: "run-123",
			},
		},
	}

	result, err := env.ExecuteActivity(act.UpdatePaperIngestionResults, input)
	require.NoError(t, err) // Activity should not fail, just count as failed

	var output UpdatePaperIngestionResultsOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, 0, output.Updated)
	assert.Equal(t, 0, output.Skipped)
	assert.Equal(t, 1, output.Failed)

	paperRepo.AssertExpectations(t)
}

func TestUpdatePaperIngestionResults_MixedResults(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	paper1ID := uuid.New()
	paper2ID := uuid.New()
	paper3ID := uuid.New()
	file1ID := uuid.New()
	file3ID := uuid.New()

	// Paper 1: success
	paperRepo.On("UpdateIngestionResult", mock.Anything, paper1ID, file1ID, "run-1").
		Return(nil)
	// Paper 3: repo error
	paperRepo.On("UpdateIngestionResult", mock.Anything, paper3ID, file3ID, "run-3").
		Return(assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdatePaperIngestionResults)

	input := UpdatePaperIngestionResultsInput{
		Results: []PaperIngestionResult{
			{
				PaperID:        paper1ID,
				FileID:         file1ID.String(),
				IngestionRunID: "run-1",
			},
			{
				// Paper 2: skip (no file_id)
				PaperID:        paper2ID,
				FileID:         "",
				IngestionRunID: "run-2",
			},
			{
				PaperID:        paper3ID,
				FileID:         file3ID.String(),
				IngestionRunID: "run-3",
			},
		},
	}

	result, err := env.ExecuteActivity(act.UpdatePaperIngestionResults, input)
	require.NoError(t, err)

	var output UpdatePaperIngestionResultsOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, 1, output.Updated)
	assert.Equal(t, 1, output.Skipped)
	assert.Equal(t, 1, output.Failed)

	paperRepo.AssertExpectations(t)
}

func TestUpdatePauseState_Success(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()

	reviewRepo.On("UpdatePauseState", mock.Anything, "org-1", "proj-1", requestID,
		domain.ReviewStatusPaused, domain.PauseReasonBudgetExhausted, "searching").
		Return(nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdatePauseState)

	input := UpdatePauseStateInput{
		OrgID:         "org-1",
		ProjectID:     "proj-1",
		RequestID:     requestID,
		Status:        domain.ReviewStatusPaused,
		PauseReason:   domain.PauseReasonBudgetExhausted,
		PausedAtPhase: "searching",
	}

	_, err := env.ExecuteActivity(act.UpdatePauseState, input)
	require.NoError(t, err)

	reviewRepo.AssertExpectations(t)
}

func TestUpdatePauseState_UserPause(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()

	reviewRepo.On("UpdatePauseState", mock.Anything, "org-1", "proj-1", requestID,
		domain.ReviewStatusPaused, domain.PauseReasonUser, "ingesting").
		Return(nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdatePauseState)

	input := UpdatePauseStateInput{
		OrgID:         "org-1",
		ProjectID:     "proj-1",
		RequestID:     requestID,
		Status:        domain.ReviewStatusPaused,
		PauseReason:   domain.PauseReasonUser,
		PausedAtPhase: "ingesting",
	}

	_, err := env.ExecuteActivity(act.UpdatePauseState, input)
	require.NoError(t, err)

	reviewRepo.AssertExpectations(t)
}

func TestUpdatePauseState_Error(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	requestID := uuid.New()

	reviewRepo.On("UpdatePauseState", mock.Anything, "org-1", "proj-1", requestID,
		domain.ReviewStatusPaused, domain.PauseReasonBudgetExhausted, "expanding").
		Return(assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.UpdatePauseState)

	input := UpdatePauseStateInput{
		OrgID:         "org-1",
		ProjectID:     "proj-1",
		RequestID:     requestID,
		Status:        domain.ReviewStatusPaused,
		PauseReason:   domain.PauseReasonBudgetExhausted,
		PausedAtPhase: "expanding",
	}

	_, err := env.ExecuteActivity(act.UpdatePauseState, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update pause state")

	reviewRepo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// Tests: CheckSearchCompleted
// ---------------------------------------------------------------------------

func TestCheckSearchCompleted_CacheHit(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	keywordID := uuid.New()
	searchedAt := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	keywordRepo.On("GetLastSearch", mock.Anything, keywordID, domain.SourceTypeSemanticScholar).
		Return(&domain.KeywordSearch{
			ID:          uuid.New(),
			KeywordID:   keywordID,
			SourceAPI:   domain.SourceTypeSemanticScholar,
			SearchedAt:  searchedAt,
			PapersFound: 5,
			Status:      domain.SearchStatusCompleted,
		}, nil)

	paper1ID := uuid.New()
	paper2ID := uuid.New()
	paper3ID := uuid.New()
	cachedPapers := []*domain.Paper{
		{ID: paper1ID, CanonicalID: "doi:10.1234/p1", Title: "Paper 1"},
		{ID: paper2ID, CanonicalID: "doi:10.1234/p2", Title: "Paper 2"},
		{ID: paper3ID, CanonicalID: "doi:10.1234/p3", Title: "Paper 3"},
	}

	keywordRepo.On("GetPapersForKeywordAndSource", mock.Anything, keywordID, domain.SourceTypeSemanticScholar, 200, 0).
		Return(cachedPapers, int64(3), nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.CheckSearchCompleted)

	input := CheckSearchCompletedInput{
		KeywordID: keywordID,
		Keyword:   "CRISPR",
		Source:    domain.SourceTypeSemanticScholar,
	}

	result, err := env.ExecuteActivity(act.CheckSearchCompleted, input)
	require.NoError(t, err)

	var output CheckSearchCompletedOutput
	require.NoError(t, result.Get(&output))

	assert.True(t, output.AlreadyCompleted)
	assert.Equal(t, 5, output.PapersFoundCount)
	assert.Len(t, output.PreviouslyFoundPapers, 3)
	assert.Equal(t, "2026-02-01T12:00:00Z", output.SearchedAt)

	keywordRepo.AssertExpectations(t)
}

func TestCheckSearchCompleted_NeverSearched(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	keywordID := uuid.New()

	keywordRepo.On("GetLastSearch", mock.Anything, keywordID, domain.SourceTypeOpenAlex).
		Return(nil, domain.NewNotFoundError("search", keywordID.String()))

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.CheckSearchCompleted)

	input := CheckSearchCompletedInput{
		KeywordID: keywordID,
		Keyword:   "gene therapy",
		Source:    domain.SourceTypeOpenAlex,
	}

	result, err := env.ExecuteActivity(act.CheckSearchCompleted, input)
	require.NoError(t, err)

	var output CheckSearchCompletedOutput
	require.NoError(t, result.Get(&output))

	assert.False(t, output.AlreadyCompleted)
	assert.Empty(t, output.PreviouslyFoundPapers)

	keywordRepo.AssertExpectations(t)
}

func TestCheckSearchCompleted_PreviousSearchFailed(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	keywordID := uuid.New()

	keywordRepo.On("GetLastSearch", mock.Anything, keywordID, domain.SourceTypePubMed).
		Return(&domain.KeywordSearch{
			ID:        uuid.New(),
			KeywordID: keywordID,
			SourceAPI: domain.SourceTypePubMed,
			Status:    domain.SearchStatusFailed,
		}, nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.CheckSearchCompleted)

	input := CheckSearchCompletedInput{
		KeywordID: keywordID,
		Keyword:   "protein folding",
		Source:    domain.SourceTypePubMed,
	}

	result, err := env.ExecuteActivity(act.CheckSearchCompleted, input)
	require.NoError(t, err)

	var output CheckSearchCompletedOutput
	require.NoError(t, result.Get(&output))

	assert.False(t, output.AlreadyCompleted)

	keywordRepo.AssertExpectations(t)
	// GetPapersForKeywordAndSource should NOT be called since the search was not completed.
	keywordRepo.AssertNotCalled(t, "GetPapersForKeywordAndSource", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestCheckSearchCompleted_PaperFetchError(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	keywordID := uuid.New()

	keywordRepo.On("GetLastSearch", mock.Anything, keywordID, domain.SourceTypeSemanticScholar).
		Return(&domain.KeywordSearch{
			ID:          uuid.New(),
			KeywordID:   keywordID,
			SourceAPI:   domain.SourceTypeSemanticScholar,
			SearchedAt:  time.Now(),
			PapersFound: 10,
			Status:      domain.SearchStatusCompleted,
		}, nil)

	keywordRepo.On("GetPapersForKeywordAndSource", mock.Anything, keywordID, domain.SourceTypeSemanticScholar, 200, 0).
		Return(nil, int64(0), assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.CheckSearchCompleted)

	input := CheckSearchCompletedInput{
		KeywordID: keywordID,
		Keyword:   "RNA interference",
		Source:    domain.SourceTypeSemanticScholar,
	}

	result, err := env.ExecuteActivity(act.CheckSearchCompleted, input)
	require.NoError(t, err)

	var output CheckSearchCompletedOutput
	require.NoError(t, result.Get(&output))

	// Falls back to re-search when paper fetch fails.
	assert.False(t, output.AlreadyCompleted)

	keywordRepo.AssertExpectations(t)
}

func TestCheckSearchCompleted_UsesSourceFilter(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	keywordID := uuid.New()

	keywordRepo.On("GetLastSearch", mock.Anything, keywordID, domain.SourceTypeSemanticScholar).
		Return(&domain.KeywordSearch{
			ID:          uuid.New(),
			KeywordID:   keywordID,
			SourceAPI:   domain.SourceTypeSemanticScholar,
			SearchedAt:  time.Now(),
			PapersFound: 3,
			Status:      domain.SearchStatusCompleted,
		}, nil)

	// Verify the source filter is passed correctly using mock.MatchedBy.
	keywordRepo.On("GetPapersForKeywordAndSource",
		mock.Anything,
		keywordID,
		mock.MatchedBy(func(src domain.SourceType) bool {
			return src == domain.SourceTypeSemanticScholar
		}),
		200,
		0,
	).Return([]*domain.Paper{
		{ID: uuid.New(), Title: "Filtered Paper"},
	}, int64(1), nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.CheckSearchCompleted)

	input := CheckSearchCompletedInput{
		KeywordID: keywordID,
		Keyword:   "CRISPR-Cas9",
		Source:    domain.SourceTypeSemanticScholar,
	}

	result, err := env.ExecuteActivity(act.CheckSearchCompleted, input)
	require.NoError(t, err)

	var output CheckSearchCompletedOutput
	require.NoError(t, result.Get(&output))

	assert.True(t, output.AlreadyCompleted)
	assert.Len(t, output.PreviouslyFoundPapers, 1)

	keywordRepo.AssertExpectations(t)
	// Verify GetPapersForKeyword (without source filter) was NOT called.
	keywordRepo.AssertNotCalled(t, "GetPapersForKeyword", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// ---------------------------------------------------------------------------
// Tests: RecordSearchResult
// ---------------------------------------------------------------------------

func TestRecordSearchResult_Success(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	keywordID := uuid.New()
	paperID1 := uuid.New()
	paperID2 := uuid.New()

	keywordRepo.On("RecordSearch", mock.Anything, mock.MatchedBy(func(s *domain.KeywordSearch) bool {
		return s.KeywordID == keywordID &&
			s.SourceAPI == domain.SourceTypeOpenAlex &&
			s.PapersFound == 2 &&
			s.Status == domain.SearchStatusCompleted &&
			s.SearchWindowHash != ""
	})).Return(nil)

	keywordRepo.On("BulkAddPaperMappings", mock.Anything, mock.MatchedBy(func(mappings []*domain.KeywordPaperMapping) bool {
		if len(mappings) != 2 {
			return false
		}
		for _, m := range mappings {
			if m.KeywordID != keywordID {
				return false
			}
			if m.MappingType != domain.MappingTypeQueryMatch {
				return false
			}
			if m.SourceType != domain.SourceTypeOpenAlex {
				return false
			}
		}
		return mappings[0].PaperID == paperID1 && mappings[1].PaperID == paperID2
	})).Return(nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.RecordSearchResult)

	input := RecordSearchResultInput{
		KeywordID:   keywordID,
		Source:      domain.SourceTypeOpenAlex,
		PapersFound: 2,
		PaperIDs:    []uuid.UUID{paperID1, paperID2},
		Status:      domain.SearchStatusCompleted,
	}

	_, err := env.ExecuteActivity(act.RecordSearchResult, input)
	require.NoError(t, err)

	keywordRepo.AssertExpectations(t)
}

func TestRecordSearchResult_WithDateFrom_RFC3339(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	keywordID := uuid.New()
	dateFrom := "2026-02-01T10:00:00Z"

	expectedHash := domain.ComputeSearchWindowHash(keywordID, domain.SourceTypeSemanticScholar, &dateFrom, nil)

	keywordRepo.On("RecordSearch", mock.Anything, mock.MatchedBy(func(s *domain.KeywordSearch) bool {
		return s.KeywordID == keywordID &&
			s.SourceAPI == domain.SourceTypeSemanticScholar &&
			s.SearchWindowHash == expectedHash &&
			s.DateFrom != nil &&
			s.DateFrom.Year() == 2026 &&
			s.DateFrom.Month() == time.February &&
			s.DateFrom.Day() == 1
	})).Return(nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.RecordSearchResult)

	input := RecordSearchResultInput{
		KeywordID:   keywordID,
		Source:      domain.SourceTypeSemanticScholar,
		DateFrom:    &dateFrom,
		PapersFound: 0,
		PaperIDs:    nil,
		Status:      domain.SearchStatusCompleted,
	}

	_, err := env.ExecuteActivity(act.RecordSearchResult, input)
	require.NoError(t, err)

	keywordRepo.AssertExpectations(t)
	// BulkAddPaperMappings should NOT be called when PaperIDs is empty.
	keywordRepo.AssertNotCalled(t, "BulkAddPaperMappings", mock.Anything, mock.Anything)
}

func TestRecordSearchResult_FailedSearch(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	keywordID := uuid.New()

	keywordRepo.On("RecordSearch", mock.Anything, mock.MatchedBy(func(s *domain.KeywordSearch) bool {
		return s.KeywordID == keywordID &&
			s.Status == domain.SearchStatusFailed &&
			s.ErrorMessage == "timeout" &&
			s.PapersFound == 0
	})).Return(nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.RecordSearchResult)

	input := RecordSearchResultInput{
		KeywordID:    keywordID,
		Source:       domain.SourceTypePubMed,
		PapersFound:  0,
		PaperIDs:     nil,
		Status:       domain.SearchStatusFailed,
		ErrorMessage: "timeout",
	}

	_, err := env.ExecuteActivity(act.RecordSearchResult, input)
	require.NoError(t, err)

	keywordRepo.AssertExpectations(t)
	// BulkAddPaperMappings should NOT be called when there are no paper IDs.
	keywordRepo.AssertNotCalled(t, "BulkAddPaperMappings", mock.Anything, mock.Anything)
}

func TestRecordSearchResult_MappingFailureNonFatal(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	keywordID := uuid.New()
	paperID := uuid.New()

	keywordRepo.On("RecordSearch", mock.Anything, mock.MatchedBy(func(s *domain.KeywordSearch) bool {
		return s.KeywordID == keywordID && s.Status == domain.SearchStatusCompleted
	})).Return(nil)

	keywordRepo.On("BulkAddPaperMappings", mock.Anything, mock.Anything).
		Return(assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.RecordSearchResult)

	input := RecordSearchResultInput{
		KeywordID:   keywordID,
		Source:      domain.SourceTypeSemanticScholar,
		PapersFound: 1,
		PaperIDs:    []uuid.UUID{paperID},
		Status:      domain.SearchStatusCompleted,
	}

	// The activity should still succeed even when BulkAddPaperMappings fails.
	_, err := env.ExecuteActivity(act.RecordSearchResult, input)
	require.NoError(t, err)

	keywordRepo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// Tests: parseDate
// ---------------------------------------------------------------------------

func TestParseDate_Formats(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		year    int
		month   time.Month
		day     int
	}{
		{
			name:    "YYYY-MM-DD format",
			input:   "2026-01-15",
			wantErr: false,
			year:    2026,
			month:   time.January,
			day:     15,
		},
		{
			name:    "RFC3339 format",
			input:   "2026-02-01T10:00:00Z",
			wantErr: false,
			year:    2026,
			month:   time.February,
			day:     1,
		},
		{
			name:    "invalid date string",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "string exceeds max length",
			input:   "01234567890123456789012345678901234567890123456789",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDate(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.year, result.Year())
			assert.Equal(t, tt.month, result.Month())
			assert.Equal(t, tt.day, result.Day())
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: FetchPaperBatch
// ---------------------------------------------------------------------------

func TestFetchPaperBatch_Success(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	paper1ID := uuid.New()
	paper2ID := uuid.New()

	paperRepo.On("GetByIDs", mock.Anything, []uuid.UUID{paper1ID, paper2ID}).
		Return([]*domain.Paper{
			{
				ID:          paper1ID,
				CanonicalID: "doi:10.1234/alpha",
				Title:       "Alpha Paper",
				Abstract:    "Abstract of alpha paper.",
				PDFURL:      "https://example.com/alpha.pdf",
				Authors: []domain.Author{
					{Name: "Alice Smith"},
					{Name: "Bob Jones"},
				},
			},
			{
				ID:          paper2ID,
				CanonicalID: "doi:10.1234/beta",
				Title:       "Beta Paper",
				Abstract:    "Abstract of beta paper.",
				PDFURL:      "https://example.com/beta.pdf",
				Authors: []domain.Author{
					{Name: "Carol White"},
				},
			},
		}, nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.FetchPaperBatch)

	input := FetchPaperBatchInput{
		PaperIDs: []uuid.UUID{paper1ID, paper2ID},
	}

	result, err := env.ExecuteActivity(act.FetchPaperBatch, input)
	require.NoError(t, err)

	var output FetchPaperBatchOutput
	require.NoError(t, result.Get(&output))

	require.Len(t, output.Papers, 2)

	assert.Equal(t, paper1ID, output.Papers[0].PaperID)
	assert.Equal(t, "doi:10.1234/alpha", output.Papers[0].CanonicalID)
	assert.Equal(t, "Alpha Paper", output.Papers[0].Title)
	assert.Equal(t, "Abstract of alpha paper.", output.Papers[0].Abstract)
	assert.Equal(t, "https://example.com/alpha.pdf", output.Papers[0].PDFURL)
	assert.Equal(t, []string{"Alice Smith", "Bob Jones"}, output.Papers[0].Authors)

	assert.Equal(t, paper2ID, output.Papers[1].PaperID)
	assert.Equal(t, "doi:10.1234/beta", output.Papers[1].CanonicalID)
	assert.Equal(t, "Beta Paper", output.Papers[1].Title)
	assert.Equal(t, []string{"Carol White"}, output.Papers[1].Authors)

	paperRepo.AssertExpectations(t)
}

func TestFetchPaperBatch_EmptyInput(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.FetchPaperBatch)

	input := FetchPaperBatchInput{
		PaperIDs: []uuid.UUID{},
	}

	result, err := env.ExecuteActivity(act.FetchPaperBatch, input)
	require.NoError(t, err)

	var output FetchPaperBatchOutput
	require.NoError(t, result.Get(&output))

	assert.NotNil(t, output.Papers)
	assert.Empty(t, output.Papers)

	// Verify no repository methods were called.
	paperRepo.AssertNotCalled(t, "GetByIDs", mock.Anything, mock.Anything)
}

func TestFetchPaperBatch_RepoError(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	paperIDs := []uuid.UUID{uuid.New()}

	paperRepo.On("GetByIDs", mock.Anything, paperIDs).
		Return(nil, assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.FetchPaperBatch)

	input := FetchPaperBatchInput{
		PaperIDs: paperIDs,
	}

	_, err := env.ExecuteActivity(act.FetchPaperBatch, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch papers by IDs")

	paperRepo.AssertExpectations(t)
}

func TestFetchPaperBatch_NilPapersFiltered(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	paper1ID := uuid.New()
	paper2ID := uuid.New()
	paper3ID := uuid.New()

	// Repo returns a nil paper mixed with valid papers.
	paperRepo.On("GetByIDs", mock.Anything, []uuid.UUID{paper1ID, paper2ID, paper3ID}).
		Return([]*domain.Paper{
			{
				ID:          paper1ID,
				CanonicalID: "doi:10.1234/first",
				Title:       "First Paper",
				Authors:     []domain.Author{{Name: "Author One"}},
			},
			nil, // nil entry should be filtered out
			{
				ID:          paper3ID,
				CanonicalID: "doi:10.1234/third",
				Title:       "Third Paper",
				Authors:     []domain.Author{{Name: "Author Three"}},
			},
		}, nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.FetchPaperBatch)

	input := FetchPaperBatchInput{
		PaperIDs: []uuid.UUID{paper1ID, paper2ID, paper3ID},
	}

	result, err := env.ExecuteActivity(act.FetchPaperBatch, input)
	require.NoError(t, err)

	var output FetchPaperBatchOutput
	require.NoError(t, result.Get(&output))

	// Only the two non-nil papers should appear.
	require.Len(t, output.Papers, 2)
	assert.Equal(t, paper1ID, output.Papers[0].PaperID)
	assert.Equal(t, paper3ID, output.Papers[1].PaperID)

	paperRepo.AssertExpectations(t)
}

func TestFetchPaperBatch_EmptyAuthorNames(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	paperID := uuid.New()

	paperRepo.On("GetByIDs", mock.Anything, []uuid.UUID{paperID}).
		Return([]*domain.Paper{
			{
				ID:          paperID,
				CanonicalID: "doi:10.1234/mixed-authors",
				Title:       "Mixed Authors Paper",
				Authors: []domain.Author{
					{Name: "Valid Author"},
					{Name: ""},
					{Name: "Another Valid"},
					{Name: ""},
				},
			},
		}, nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.FetchPaperBatch)

	input := FetchPaperBatchInput{
		PaperIDs: []uuid.UUID{paperID},
	}

	result, err := env.ExecuteActivity(act.FetchPaperBatch, input)
	require.NoError(t, err)

	var output FetchPaperBatchOutput
	require.NoError(t, result.Get(&output))

	require.Len(t, output.Papers, 1)
	// Only non-empty author names should be included.
	assert.Equal(t, []string{"Valid Author", "Another Valid"}, output.Papers[0].Authors)

	paperRepo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// Tests: BulkCreateKeywordPaperMappings
// ---------------------------------------------------------------------------

func TestBulkCreateKeywordPaperMappings_Success(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	kw1ID := uuid.New()
	kw2ID := uuid.New()
	paper1ID := uuid.New()
	paper2ID := uuid.New()
	paper3ID := uuid.New()

	// All entries are flattened into a single BulkAddPaperMappings call.
	keywordRepo.On("BulkAddPaperMappings", mock.Anything, mock.MatchedBy(func(mappings []*domain.KeywordPaperMapping) bool {
		if len(mappings) != 3 {
			return false
		}
		// First two from kw1 + semantic_scholar
		if mappings[0].KeywordID != kw1ID || mappings[0].PaperID != paper1ID || mappings[0].SourceType != domain.SourceTypeSemanticScholar {
			return false
		}
		if mappings[1].KeywordID != kw1ID || mappings[1].PaperID != paper2ID || mappings[1].SourceType != domain.SourceTypeSemanticScholar {
			return false
		}
		// Third from kw2 + openalex
		if mappings[2].KeywordID != kw2ID || mappings[2].PaperID != paper3ID || mappings[2].SourceType != domain.SourceTypeOpenAlex {
			return false
		}
		// All should have MappingTypeQueryMatch
		for _, m := range mappings {
			if m.MappingType != domain.MappingTypeQueryMatch {
				return false
			}
		}
		return true
	})).Return(nil)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.BulkCreateKeywordPaperMappings)

	input := BulkCreateKeywordPaperMappingsInput{
		Entries: []KeywordPaperMappingEntry{
			{
				KeywordID: kw1ID,
				Source:    domain.SourceTypeSemanticScholar,
				PaperIDs:  []uuid.UUID{paper1ID, paper2ID},
			},
			{
				KeywordID: kw2ID,
				Source:    domain.SourceTypeOpenAlex,
				PaperIDs:  []uuid.UUID{paper3ID},
			},
		},
	}

	_, err := env.ExecuteActivity(act.BulkCreateKeywordPaperMappings, input)
	require.NoError(t, err)

	keywordRepo.AssertExpectations(t)
}

func TestBulkCreateKeywordPaperMappings_EmptyEntries(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.BulkCreateKeywordPaperMappings)

	input := BulkCreateKeywordPaperMappingsInput{
		Entries: []KeywordPaperMappingEntry{},
	}

	_, err := env.ExecuteActivity(act.BulkCreateKeywordPaperMappings, input)
	require.NoError(t, err)

	// Verify no repository methods were called.
	keywordRepo.AssertNotCalled(t, "BulkAddPaperMappings", mock.Anything, mock.Anything)
}

func TestBulkCreateKeywordPaperMappings_SkipsEmptyPaperIDs(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.BulkCreateKeywordPaperMappings)

	input := BulkCreateKeywordPaperMappingsInput{
		Entries: []KeywordPaperMappingEntry{
			{
				KeywordID: uuid.New(),
				Source:    domain.SourceTypePubMed,
				PaperIDs:  []uuid.UUID{}, // empty PaperIDs
			},
		},
	}

	_, err := env.ExecuteActivity(act.BulkCreateKeywordPaperMappings, input)
	require.NoError(t, err)

	// BulkAddPaperMappings should NOT be called for entries with empty PaperIDs.
	keywordRepo.AssertNotCalled(t, "BulkAddPaperMappings", mock.Anything, mock.Anything)
}

func TestBulkCreateKeywordPaperMappings_AllEntriesEmptyPaperIDs(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.BulkCreateKeywordPaperMappings)

	input := BulkCreateKeywordPaperMappingsInput{
		Entries: []KeywordPaperMappingEntry{
			{
				KeywordID: uuid.New(),
				Source:    domain.SourceTypeSemanticScholar,
				PaperIDs:  []uuid.UUID{},
			},
			{
				KeywordID: uuid.New(),
				Source:    domain.SourceTypeOpenAlex,
				PaperIDs:  nil,
			},
		},
	}

	// All entries have empty PaperIDs; totalMappings == 0, returns nil.
	_, err := env.ExecuteActivity(act.BulkCreateKeywordPaperMappings, input)
	require.NoError(t, err)

	keywordRepo.AssertNotCalled(t, "BulkAddPaperMappings", mock.Anything, mock.Anything)
}

func TestBulkCreateKeywordPaperMappings_DBError(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	reviewRepo := &mockReviewRepository{}
	keywordRepo := &mockKeywordRepository{}
	paperRepo := &mockPaperRepository{}

	paper1ID := uuid.New()
	paper2ID := uuid.New()

	// Single flattened call fails.
	keywordRepo.On("BulkAddPaperMappings", mock.Anything, mock.MatchedBy(func(mappings []*domain.KeywordPaperMapping) bool {
		return len(mappings) == 2
	})).Return(assert.AnError)

	act := NewStatusActivities(reviewRepo, keywordRepo, paperRepo, nil)
	env.RegisterActivity(act.BulkCreateKeywordPaperMappings)

	input := BulkCreateKeywordPaperMappingsInput{
		Entries: []KeywordPaperMappingEntry{
			{
				KeywordID: uuid.New(),
				Source:    domain.SourceTypePubMed,
				PaperIDs:  []uuid.UUID{paper1ID},
			},
			{
				KeywordID: uuid.New(),
				Source:    domain.SourceTypePubMed,
				PaperIDs:  []uuid.UUID{paper2ID},
			},
		},
	}

	_, err := env.ExecuteActivity(act.BulkCreateKeywordPaperMappings, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bulk add keyword-paper mappings")

	keywordRepo.AssertExpectations(t)
}
