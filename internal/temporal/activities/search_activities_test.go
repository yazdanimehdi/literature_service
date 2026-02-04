package activities

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
)

// mockPaperSource implements papersources.PaperSource for testing.
type mockPaperSource struct {
	sourceType domain.SourceType
	name       string
	enabled    bool
	searchFn   func(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error)
}

func (m *mockPaperSource) Search(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, params)
	}
	return &papersources.SearchResult{}, nil
}

func (m *mockPaperSource) GetByID(_ context.Context, _ string) (*domain.Paper, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockPaperSource) SourceType() domain.SourceType {
	return m.sourceType
}

func (m *mockPaperSource) Name() string {
	return m.name
}

func (m *mockPaperSource) IsEnabled() bool {
	return m.enabled
}

// newTestPapers creates a slice of test papers for use in test cases.
func newTestPapers(count int) []*domain.Paper {
	papers := make([]*domain.Paper, count)
	for i := range count {
		papers[i] = &domain.Paper{
			ID:              uuid.New(),
			CanonicalID:     fmt.Sprintf("doi:10.1234/test-%d", i),
			Title:           fmt.Sprintf("Test Paper %d", i+1),
			Abstract:        fmt.Sprintf("Abstract for test paper %d", i+1),
			PublicationYear: 2024,
			CitationCount:   i * 10,
		}
	}
	return papers
}

func TestSearchPapers_Success(t *testing.T) {
	// Set up Temporal test environment.
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	// Create mock source that returns 2 papers.
	testPapers := newTestPapers(2)
	registry := papersources.NewRegistry()
	registry.Register(&mockPaperSource{
		sourceType: domain.SourceTypeSemanticScholar,
		name:       "Semantic Scholar",
		enabled:    true,
		searchFn: func(_ context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
			assert.Equal(t, "CRISPR gene editing", params.Query)
			assert.Equal(t, 50, params.MaxResults)
			assert.True(t, params.IncludePreprints)
			return &papersources.SearchResult{
				Papers:         testPapers,
				TotalResults:   2,
				Source:         domain.SourceTypeSemanticScholar,
				SearchDuration: 150 * time.Millisecond,
			}, nil
		},
	})

	// Create activity with nil metrics.
	activities := NewSearchActivities(registry, nil)
	env.RegisterActivity(activities.SearchPapers)

	// Execute the activity.
	input := SearchPapersInput{
		Query:            "CRISPR gene editing",
		Sources:          []domain.SourceType{domain.SourceTypeSemanticScholar},
		MaxResults:       50,
		IncludePreprints: true,
	}

	result, err := env.ExecuteActivity(activities.SearchPapers, input)
	require.NoError(t, err)

	var output SearchPapersOutput
	require.NoError(t, result.Get(&output))

	assert.Len(t, output.Papers, 2)
	assert.Equal(t, 2, output.TotalFound)
	assert.Equal(t, 2, output.BySource[domain.SourceTypeSemanticScholar])
	assert.Empty(t, output.Errors)

	// Verify paper contents.
	assert.Equal(t, testPapers[0].Title, output.Papers[0].Title)
	assert.Equal(t, testPapers[1].Title, output.Papers[1].Title)
}

func TestSearchPapers_PartialFailure(t *testing.T) {
	// Set up Temporal test environment.
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	// Create two sources: one succeeds, one fails.
	testPapers := newTestPapers(3)
	registry := papersources.NewRegistry()

	registry.Register(&mockPaperSource{
		sourceType: domain.SourceTypeSemanticScholar,
		name:       "Semantic Scholar",
		enabled:    true,
		searchFn: func(_ context.Context, _ papersources.SearchParams) (*papersources.SearchResult, error) {
			return &papersources.SearchResult{
				Papers:         testPapers,
				TotalResults:   3,
				Source:         domain.SourceTypeSemanticScholar,
				SearchDuration: 200 * time.Millisecond,
			}, nil
		},
	})

	registry.Register(&mockPaperSource{
		sourceType: domain.SourceTypeOpenAlex,
		name:       "OpenAlex",
		enabled:    true,
		searchFn: func(_ context.Context, _ papersources.SearchParams) (*papersources.SearchResult, error) {
			return nil, fmt.Errorf("connection timeout")
		},
	})

	// Create activity with nil metrics.
	activities := NewSearchActivities(registry, nil)
	env.RegisterActivity(activities.SearchPapers)

	// Execute the activity requesting both sources.
	input := SearchPapersInput{
		Query:      "machine learning protein folding",
		Sources:    []domain.SourceType{domain.SourceTypeSemanticScholar, domain.SourceTypeOpenAlex},
		MaxResults: 100,
	}

	result, err := env.ExecuteActivity(activities.SearchPapers, input)
	require.NoError(t, err, "should succeed with partial results when at least one source returns papers")

	var output SearchPapersOutput
	require.NoError(t, result.Get(&output))

	// Should have papers from the successful source.
	assert.Len(t, output.Papers, 3)
	assert.Equal(t, 3, output.TotalFound)
	assert.Equal(t, 3, output.BySource[domain.SourceTypeSemanticScholar])

	// OpenAlex should not be in BySource since it failed.
	_, hasOpenAlex := output.BySource[domain.SourceTypeOpenAlex]
	assert.False(t, hasOpenAlex)

	// Should have one error from OpenAlex.
	require.Len(t, output.Errors, 1)
	assert.Equal(t, domain.SourceTypeOpenAlex, output.Errors[0].Source)
	assert.Contains(t, output.Errors[0].Error, "connection timeout")
}

func TestSearchPapers_AllFail(t *testing.T) {
	// Set up Temporal test environment.
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	// Create a single source that fails.
	registry := papersources.NewRegistry()
	registry.Register(&mockPaperSource{
		sourceType: domain.SourceTypePubMed,
		name:       "PubMed",
		enabled:    true,
		searchFn: func(_ context.Context, _ papersources.SearchParams) (*papersources.SearchResult, error) {
			return nil, fmt.Errorf("rate limit exceeded")
		},
	})

	// Create activity with nil metrics.
	activities := NewSearchActivities(registry, nil)
	env.RegisterActivity(activities.SearchPapers)

	// Execute the activity.
	input := SearchPapersInput{
		Query:      "neurodegenerative diseases",
		Sources:    []domain.SourceType{domain.SourceTypePubMed},
		MaxResults: 25,
	}

	_, err := env.ExecuteActivity(activities.SearchPapers, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all paper sources failed")
	assert.Contains(t, err.Error(), "rate limit exceeded")
}

func TestSearchPapers_EmptyResults(t *testing.T) {
	// Set up Temporal test environment.
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	// Create a source that succeeds but returns no papers.
	registry := papersources.NewRegistry()
	registry.Register(&mockPaperSource{
		sourceType: domain.SourceTypeOpenAlex,
		name:       "OpenAlex",
		enabled:    true,
		searchFn: func(_ context.Context, _ papersources.SearchParams) (*papersources.SearchResult, error) {
			return &papersources.SearchResult{
				Papers:         nil,
				TotalResults:   0,
				Source:         domain.SourceTypeOpenAlex,
				SearchDuration: 50 * time.Millisecond,
			}, nil
		},
	})

	// Create activity with nil metrics.
	activities := NewSearchActivities(registry, nil)
	env.RegisterActivity(activities.SearchPapers)

	// Execute the activity.
	input := SearchPapersInput{
		Query:        "extremely specific nonexistent topic xyz123",
		Sources:      []domain.SourceType{domain.SourceTypeOpenAlex},
		MaxResults:   50,
		MinCitations: 100,
	}

	result, err := env.ExecuteActivity(activities.SearchPapers, input)
	require.NoError(t, err, "should succeed even with 0 papers when source did not error")

	var output SearchPapersOutput
	require.NoError(t, result.Get(&output))

	assert.Empty(t, output.Papers)
	assert.Equal(t, 0, output.TotalFound)
	assert.Equal(t, 0, output.BySource[domain.SourceTypeOpenAlex])
	assert.Empty(t, output.Errors)
}
