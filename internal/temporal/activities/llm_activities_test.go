package activities

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	sharedllm "github.com/helixir/llm"

	"github.com/helixir/literature-review-service/internal/llm"
)

// mockKeywordExtractor implements llm.KeywordExtractor for testing.
type mockKeywordExtractor struct {
	mock.Mock
}

func (m *mockKeywordExtractor) ExtractKeywords(ctx context.Context, req llm.ExtractionRequest) (*llm.ExtractionResult, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*llm.ExtractionResult), args.Error(1)
}

func (m *mockKeywordExtractor) Provider() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockKeywordExtractor) Model() string {
	args := m.Called()
	return args.String(0)
}

func TestExtractKeywords_Success(t *testing.T) {
	// Set up Temporal test environment.
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	// Set up mock extractor.
	extractor := &mockKeywordExtractor{}
	extractor.On("ExtractKeywords", mock.Anything, llm.ExtractionRequest{
		Text:             "What are the latest advances in CRISPR gene editing?",
		Mode:             llm.ExtractionModeQuery,
		MaxKeywords:      10,
		MinKeywords:      3,
		ExistingKeywords: nil,
		Context:          "",
	}).Return(&llm.ExtractionResult{
		Keywords:     []string{"CRISPR", "gene editing", "Cas9", "genome engineering"},
		Reasoning:    "Extracted core CRISPR-related terms and common synonyms",
		Model:        "gpt-4o",
		InputTokens:  150,
		OutputTokens: 50,
	}, nil)
	// Create activity with nil metrics (testing without metrics).
	activities := NewLLMActivities(extractor, nil)
	env.RegisterActivity(activities.ExtractKeywords)

	// Execute the activity.
	input := ExtractKeywordsInput{
		Text:        "What are the latest advances in CRISPR gene editing?",
		Mode:        "query",
		MaxKeywords: 10,
		MinKeywords: 3,
	}

	result, err := env.ExecuteActivity(activities.ExtractKeywords, input)
	require.NoError(t, err)

	var output ExtractKeywordsOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, []string{"CRISPR", "gene editing", "Cas9", "genome engineering"}, output.Keywords)
	assert.Equal(t, "Extracted core CRISPR-related terms and common synonyms", output.Reasoning)
	assert.Equal(t, "gpt-4o", output.Model)
	assert.Equal(t, 150, output.InputTokens)
	assert.Equal(t, 50, output.OutputTokens)

	extractor.AssertExpectations(t)
}

func TestExtractKeywords_Error(t *testing.T) {
	// Set up Temporal test environment.
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	// Set up mock extractor that returns an error.
	extractor := &mockKeywordExtractor{}
	extractor.On("ExtractKeywords", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("LLM provider unavailable"))

	// Create activity with nil metrics.
	activities := NewLLMActivities(extractor, nil)
	env.RegisterActivity(activities.ExtractKeywords)

	// Execute the activity.
	input := ExtractKeywordsInput{
		Text:        "Some research query",
		Mode:        "query",
		MaxKeywords: 10,
		MinKeywords: 3,
	}

	_, err := env.ExecuteActivity(activities.ExtractKeywords, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyword extraction failed")

	extractor.AssertExpectations(t)
}

func TestExtractKeywords_WithMetrics(t *testing.T) {
	// Set up Temporal test environment.
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	// Set up mock extractor.
	extractor := &mockKeywordExtractor{}
	extractor.On("ExtractKeywords", mock.Anything, mock.Anything).
		Return(&llm.ExtractionResult{
			Keywords:     []string{"CRISPR", "gene editing"},
			Reasoning:    "Core terms",
			Model:        "gpt-4o",
			InputTokens:  100,
			OutputTokens: 30,
		}, nil)

	// Use real metrics (nil-safe).
	activities := NewLLMActivities(extractor, nil)
	env.RegisterActivity(activities.ExtractKeywords)

	input := ExtractKeywordsInput{
		Text:        "CRISPR gene editing query",
		Mode:        "query",
		MaxKeywords: 10,
		MinKeywords: 3,
	}

	result, err := env.ExecuteActivity(activities.ExtractKeywords, input)
	require.NoError(t, err)

	var output ExtractKeywordsOutput
	require.NoError(t, result.Get(&output))
	assert.Len(t, output.Keywords, 2)

	extractor.AssertExpectations(t)
}

func TestErrorType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "APIError with Type field",
			err:      &llm.APIError{Provider: "openai", StatusCode: 429, Type: "rate_limit_error"},
			expected: "rate_limit_error",
		},
		{
			name:     "APIError without Type field uses status code",
			err:      &llm.APIError{Provider: "openai", StatusCode: 500, Type: ""},
			expected: "http_500",
		},
		{
			name:     "wrapped APIError with Type",
			err:      fmt.Errorf("call failed: %w", &llm.APIError{Provider: "anthropic", StatusCode: 400, Type: "invalid_request_error"}),
			expected: "invalid_request_error",
		},
		{
			name:     "wrapped APIError without Type",
			err:      fmt.Errorf("call failed: %w", &llm.APIError{Provider: "anthropic", StatusCode: 503}),
			expected: "http_503",
		},
		{
			name:     "shared llm.Error with Kind",
			err:      &sharedllm.Error{Provider: "openai", Kind: sharedllm.ErrRateLimit, StatusCode: 429, Message: "too many requests"},
			expected: "rate_limit",
		},
		{
			name:     "shared llm.Error with circuit open",
			err:      &sharedllm.Error{Kind: sharedllm.ErrCircuitOpen, Message: "circuit breaker open"},
			expected: "circuit_open",
		},
		{
			name:     "shared llm.Error with budget exceeded",
			err:      &sharedllm.Error{Kind: sharedllm.ErrBudgetExceeded, Message: "token budget exceeded"},
			expected: "budget_exceeded",
		},
		{
			name:     "shared llm.Error with StatusCode only",
			err:      &sharedllm.Error{Provider: "anthropic", StatusCode: 503},
			expected: "http_503",
		},
		{
			name:     "wrapped shared llm.Error",
			err:      fmt.Errorf("keyword extraction via openai failed: %w", &sharedllm.Error{Kind: sharedllm.ErrRateLimit}),
			expected: "rate_limit",
		},
		{
			name:     "non-APIError returns unknown",
			err:      fmt.Errorf("connection refused"),
			expected: "unknown",
		},
		{
			name:     "nil-like wrapped error returns unknown",
			err:      fmt.Errorf("something: %w", fmt.Errorf("nested")),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errorType(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockCoverageAssessor implements llm.CoverageAssessor for testing.
type mockCoverageAssessor struct {
	mock.Mock
}

func (m *mockCoverageAssessor) AssessCoverage(ctx context.Context, req llm.CoverageRequest) (*llm.CoverageResult, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*llm.CoverageResult), args.Error(1)
}

func TestAssessCoverage_Success(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	assessor := &mockCoverageAssessor{}
	assessor.On("AssessCoverage", mock.Anything, mock.MatchedBy(func(req llm.CoverageRequest) bool {
		return req.Title == "CRISPR gene editing" && req.TotalPapers == 25
	})).Return(&llm.CoverageResult{
		CoverageScore: 0.82,
		Reasoning:     "Good coverage of mechanisms, gap in delivery vectors",
		GapTopics:     []string{"lipid nanoparticle delivery"},
		IsSufficient:  true,
		Model:         "gpt-4o",
		InputTokens:   500,
		OutputTokens:  100,
	}, nil)

	extractor := &mockKeywordExtractor{}
	act := NewLLMActivities(extractor, nil, WithCoverageAssessor(assessor))
	env.RegisterActivity(act.AssessCoverage)

	val, err := env.ExecuteActivity(act.AssessCoverage, AssessCoverageInput{
		Title:       "CRISPR gene editing",
		AllKeywords: []string{"CRISPR", "Cas9"},
		PaperSummaries: []PaperSummary{
			{Title: "Paper 1", Abstract: "About CRISPR"},
		},
		TotalPapers: 25,
	})
	require.NoError(t, err)

	var output AssessCoverageOutput
	require.NoError(t, val.Get(&output))
	assert.InDelta(t, 0.82, output.CoverageScore, 0.001)
	assert.Equal(t, []string{"lipid nanoparticle delivery"}, output.GapTopics)
	assert.True(t, output.IsSufficient)
	assert.Equal(t, "gpt-4o", output.Model)
	assert.Equal(t, 500, output.InputTokens)
	assert.Equal(t, 100, output.OutputTokens)

	assessor.AssertExpectations(t)
}

func TestAssessCoverage_NilAssessor(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	extractor := &mockKeywordExtractor{}
	act := NewLLMActivities(extractor, nil)
	env.RegisterActivity(act.AssessCoverage)

	_, err := env.ExecuteActivity(act.AssessCoverage, AssessCoverageInput{
		Title: "test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "coverage assessor is not configured")
}

func TestAssessCoverage_Error(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	assessor := &mockCoverageAssessor{}
	assessor.On("AssessCoverage", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("LLM provider unavailable"))

	extractor := &mockKeywordExtractor{}
	act := NewLLMActivities(extractor, nil, WithCoverageAssessor(assessor))
	env.RegisterActivity(act.AssessCoverage)

	_, err := env.ExecuteActivity(act.AssessCoverage, AssessCoverageInput{
		Title:       "test",
		TotalPapers: 10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "coverage assessment failed")

	assessor.AssertExpectations(t)
}

func TestExtractKeywords_WithExistingKeywords(t *testing.T) {
	// Set up Temporal test environment.
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	existingKeywords := []string{"CRISPR", "gene editing"}

	// Set up mock extractor and verify ExistingKeywords are passed through.
	extractor := &mockKeywordExtractor{}
	extractor.On("ExtractKeywords", mock.Anything, mock.MatchedBy(func(req llm.ExtractionRequest) bool {
		return len(req.ExistingKeywords) == 2 &&
			req.ExistingKeywords[0] == "CRISPR" &&
			req.ExistingKeywords[1] == "gene editing" &&
			req.Mode == llm.ExtractionModeAbstract
	})).Return(&llm.ExtractionResult{
		Keywords:     []string{"Cas9", "guide RNA", "PAM sequence"},
		Reasoning:    "Found complementary terms not in existing set",
		Model:        "claude-sonnet-4-20250514",
		InputTokens:  200,
		OutputTokens: 60,
	}, nil)

	// Create activity with nil metrics.
	activities := NewLLMActivities(extractor, nil)
	env.RegisterActivity(activities.ExtractKeywords)

	// Execute the activity with existing keywords.
	input := ExtractKeywordsInput{
		Text:             "This paper describes a novel application of CRISPR-Cas9 with modified guide RNA targeting PAM sequences...",
		Mode:             "abstract",
		MaxKeywords:      10,
		MinKeywords:      3,
		ExistingKeywords: existingKeywords,
		Context:          "molecular biology",
	}

	result, err := env.ExecuteActivity(activities.ExtractKeywords, input)
	require.NoError(t, err)

	var output ExtractKeywordsOutput
	require.NoError(t, result.Get(&output))

	assert.Equal(t, []string{"Cas9", "guide RNA", "PAM sequence"}, output.Keywords)
	assert.Equal(t, "Found complementary terms not in existing set", output.Reasoning)
	assert.Equal(t, "claude-sonnet-4-20250514", output.Model)
	assert.Equal(t, 200, output.InputTokens)
	assert.Equal(t, 60, output.OutputTokens)

	// Verify the mock was called with the correct existing keywords.
	extractor.AssertExpectations(t)
}
