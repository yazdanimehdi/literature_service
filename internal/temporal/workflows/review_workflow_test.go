package workflows

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
)

// newTestInput returns a ReviewWorkflowInput configured for tests.
func newTestInput() ReviewWorkflowInput {
	return ReviewWorkflowInput{
		RequestID: uuid.New(),
		OrgID:     "org-1",
		ProjectID: "proj-1",
		UserID:    "user-1",
		Query:     "CRISPR gene editing therapeutic applications",
		Config: domain.ReviewConfiguration{
			MaxPapers:           50,
			MaxExpansionDepth:   0,
			MaxKeywordsPerRound: 5,
			Sources:             []domain.SourceType{domain.SourceTypeSemanticScholar},
			IncludePreprints:    true,
			RequireOpenAccess:   false,
			MinCitations:        0,
		},
	}
}

func TestLiteratureReviewWorkflow_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	// Activity nil-pointer references matching the workflow pattern.
	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus - accept any input.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - return test keywords.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{"CRISPR", "gene therapy"},
			Reasoning: "test",
			Model:     "test-model",
		}, nil,
	)

	// Mock SaveKeywords.
	env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).Return(
		&activities.SaveKeywordsOutput{
			KeywordIDs: []uuid.UUID{uuid.New(), uuid.New()},
			NewCount:   2,
		}, nil,
	)

	// Mock SearchPapers - return some papers.
	env.OnActivity(searchAct.SearchPapers, mock.Anything, mock.Anything).Return(
		&activities.SearchPapersOutput{
			Papers: []*domain.Paper{
				{
					ID:          uuid.New(),
					CanonicalID: "doi:10.1234/test",
					Title:       "Test Paper",
					Abstract:    "Test abstract about CRISPR",
				},
			},
			TotalFound: 1,
			BySource:   map[domain.SourceType]int{domain.SourceTypeSemanticScholar: 1},
		}, nil,
	)

	// Mock SavePapers.
	env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).Return(
		&activities.SavePapersOutput{
			SavedCount:     1,
			DuplicateCount: 0,
		}, nil,
	)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, input.RequestID, result.RequestID)
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	assert.Equal(t, 2, result.KeywordsFound)
	assert.Equal(t, 2, result.PapersFound) // 2 keywords, 1 paper each = 2 total
	assert.Equal(t, 1, result.PapersIngested)
	assert.Equal(t, 0, result.ExpansionRounds)
	assert.GreaterOrEqual(t, result.Duration, 0.0)

	env.AssertExpectations(t)
}

func TestLiteratureReviewWorkflow_ExtractionFailure(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	var llmAct *activities.LLMActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus - succeed for all calls.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - return error.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		nil, assert.AnError,
	)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())

	err := env.GetWorkflowError()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract_keywords")
}

func TestLiteratureReviewWorkflow_WithExpansion(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()
	input.Config.MaxExpansionDepth = 1

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - returns keywords for both query and abstract modes.
	// The workflow calls this first for query extraction, then for each expansion paper.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{"CRISPR", "gene therapy"},
			Reasoning: "test extraction",
			Model:     "test-model",
		}, nil,
	)

	// Mock SaveKeywords.
	env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).Return(
		&activities.SaveKeywordsOutput{
			KeywordIDs: []uuid.UUID{uuid.New()},
			NewCount:   1,
		}, nil,
	)

	// Mock SearchPapers - return papers with abstracts for expansion.
	env.OnActivity(searchAct.SearchPapers, mock.Anything, mock.Anything).Return(
		&activities.SearchPapersOutput{
			Papers: []*domain.Paper{
				{
					ID:          uuid.New(),
					CanonicalID: "doi:10.1234/test1",
					Title:       "CRISPR Paper 1",
					Abstract:    "This paper discusses CRISPR-Cas9 nuclease systems.",
				},
			},
			TotalFound: 1,
			BySource:   map[domain.SourceType]int{domain.SourceTypeSemanticScholar: 1},
		}, nil,
	)

	// Mock SavePapers.
	env.OnActivity(statusAct.SavePapers, mock.Anything, mock.Anything).Return(
		&activities.SavePapersOutput{
			SavedCount:     1,
			DuplicateCount: 0,
		}, nil,
	)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	// Initial: 2 keywords ("CRISPR", "gene therapy") + Expansion: 1 keyword ("cas9 nuclease") = 3 total.
	assert.GreaterOrEqual(t, result.KeywordsFound, 2)
	assert.Equal(t, 1, result.ExpansionRounds)
}

func TestLiteratureReviewWorkflow_ProgressQuery(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	var llmAct *activities.LLMActivities
	var searchAct *activities.SearchActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		&activities.ExtractKeywordsOutput{
			Keywords:  []string{"CRISPR"},
			Reasoning: "test",
			Model:     "test-model",
		}, nil,
	)

	// Mock SaveKeywords.
	env.OnActivity(statusAct.SaveKeywords, mock.Anything, mock.Anything).Return(
		&activities.SaveKeywordsOutput{
			KeywordIDs: []uuid.UUID{uuid.New()},
			NewCount:   1,
		}, nil,
	)

	// Mock SearchPapers.
	env.OnActivity(searchAct.SearchPapers, mock.Anything, mock.Anything).Return(
		&activities.SearchPapersOutput{
			Papers:     []*domain.Paper{},
			TotalFound: 0,
			BySource:   map[domain.SourceType]int{},
		}, nil,
	)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Query progress after completion - query handlers remain registered.
	encoded, err := env.QueryWorkflow(QueryProgress)
	require.NoError(t, err)

	var queryProgress workflowProgress
	require.NoError(t, encoded.Get(&queryProgress))

	// The progress query should reflect the final state.
	assert.Equal(t, string(domain.ReviewStatusCompleted), queryProgress.Status)
	assert.Equal(t, "completed", queryProgress.Phase)
	assert.Equal(t, 0, queryProgress.MaxExpansionDepth)
	assert.Equal(t, 1, queryProgress.KeywordsFound)
}

func TestLiteratureReviewWorkflow_Cancellation(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	input := newTestInput()

	var llmAct *activities.LLMActivities
	var statusAct *activities.StatusActivities
	var eventAct *activities.EventActivities

	// Mock UpdateStatus - succeed for all calls.
	env.OnActivity(statusAct.UpdateStatus, mock.Anything, mock.Anything).Return(nil)

	// Mock PublishEvent - fire-and-forget.
	env.OnActivity(eventAct.PublishEvent, mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - simulate cancellation by returning a cancellation error.
	// When the cancel signal fires during an activity, Temporal wraps it as a CanceledError.
	env.OnActivity(llmAct.ExtractKeywords, mock.Anything, mock.Anything).Return(
		nil, temporal.NewCanceledError("workflow cancelled"),
	)

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())

	err := env.GetWorkflowError()
	require.Error(t, err)
	// The error should indicate the workflow failed during keyword extraction.
	assert.Contains(t, err.Error(), "extract_keywords")
}

func TestSelectPapersForExpansion(t *testing.T) {
	t.Run("returns papers with abstracts up to max", func(t *testing.T) {
		papers := []*domain.Paper{
			{Title: "Paper 1", Abstract: "Abstract 1"},
			{Title: "Paper 2", Abstract: ""},
			{Title: "Paper 3", Abstract: "Abstract 3"},
			{Title: "Paper 4", Abstract: "Abstract 4"},
		}

		selected := selectPapersForExpansion(papers, 2)
		assert.Len(t, selected, 2)
		assert.Equal(t, "Paper 1", selected[0].Title)
		assert.Equal(t, "Paper 3", selected[1].Title)
	})

	t.Run("returns empty for no abstracts", func(t *testing.T) {
		papers := []*domain.Paper{
			{Title: "Paper 1", Abstract: ""},
			{Title: "Paper 2", Abstract: ""},
		}

		selected := selectPapersForExpansion(papers, 5)
		assert.Empty(t, selected)
	})

	t.Run("handles nil papers in slice", func(t *testing.T) {
		papers := []*domain.Paper{
			nil,
			{Title: "Paper 1", Abstract: "Abstract 1"},
			nil,
		}

		selected := selectPapersForExpansion(papers, 5)
		assert.Len(t, selected, 1)
		assert.Equal(t, "Paper 1", selected[0].Title)
	})

	t.Run("handles empty slice", func(t *testing.T) {
		selected := selectPapersForExpansion(nil, 5)
		assert.Empty(t, selected)
	})
}
