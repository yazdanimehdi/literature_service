# Phase 3C: Temporal Workflows & Activities Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement Temporal workflow activities and the main LiteratureReviewWorkflow that orchestrates keyword extraction, paper search, recursive expansion, and status tracking.

**Architecture:** Activities are grouped by responsibility (LLM, Search, Status) as struct-based activity implementations with dependency injection. The `LiteratureReviewWorkflow` orchestrates the pipeline: extract keywords → search sources → expand (recursive) → return results. Activities use the `llm.KeywordExtractor`, `papersources.Registry`, and repository interfaces built in previous phases. Temporal's `testsuite.WorkflowTestSuite` is used for all workflow/activity tests.

**Tech Stack:** Go 1.25, Temporal SDK v1.39.0, testify, Temporal testsuite

---

## Task 1: Create Activity Input/Output Types and LLM Activities

**Files:**
- Create: `internal/temporal/activities/types.go`
- Create: `internal/temporal/activities/llm_activities.go`
- Create: `internal/temporal/activities/llm_activities_test.go`

**Context:**
- `internal/llm/extractor.go` defines `KeywordExtractor` interface, `ExtractionRequest`, `ExtractionResult`, `ExtractionMode`
- `internal/observability/metrics.go` defines `Metrics` with `RecordLLMRequest`, `RecordLLMRequestFailed`, `RecordKeywordsExtracted`
- Activity structs hold dependencies; methods become Temporal activities
- Use `activity.GetLogger(ctx)` for logging inside activities

**Step 1: Create `internal/temporal/activities/types.go`**

All activity input/output types as serializable structs. Keep them separate from domain types for Temporal serialization boundary.

```go
package activities

import (
	"github.com/google/uuid"
	"github.com/helixir/literature-review-service/internal/domain"
)

// --- LLM Activity Types ---

// ExtractKeywordsInput is the input for the ExtractKeywords activity.
type ExtractKeywordsInput struct {
	Text             string   `json:"text"`
	Mode             string   `json:"mode"` // "query" or "abstract"
	MaxKeywords      int      `json:"max_keywords"`
	MinKeywords      int      `json:"min_keywords"`
	ExistingKeywords []string `json:"existing_keywords,omitempty"`
	Context          string   `json:"context,omitempty"`
}

// ExtractKeywordsOutput is the output from the ExtractKeywords activity.
type ExtractKeywordsOutput struct {
	Keywords     []string `json:"keywords"`
	Reasoning    string   `json:"reasoning,omitempty"`
	Model        string   `json:"model"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
}

// --- Search Activity Types ---

// SearchPapersInput is the input for the SearchPapers activity.
type SearchPapersInput struct {
	Query          string              `json:"query"`
	Sources        []domain.SourceType `json:"sources,omitempty"`
	MaxResults     int                 `json:"max_results"`
	IncludePreprints bool             `json:"include_preprints"`
	OpenAccessOnly bool               `json:"open_access_only"`
	MinCitations   int                `json:"min_citations"`
}

// SearchPapersOutput is the output from the SearchPapers activity.
type SearchPapersOutput struct {
	Papers      []*domain.Paper          `json:"papers"`
	TotalFound  int                      `json:"total_found"`
	BySource    map[domain.SourceType]int `json:"by_source"`
	Errors      []SourceError            `json:"errors,omitempty"`
}

// SourceError captures an error from a specific source.
type SourceError struct {
	Source  domain.SourceType `json:"source"`
	Error  string            `json:"error"`
}

// --- Status Activity Types ---

// UpdateStatusInput is the input for the UpdateStatus activity.
type UpdateStatusInput struct {
	OrgID     string              `json:"org_id"`
	ProjectID string              `json:"project_id"`
	RequestID uuid.UUID           `json:"request_id"`
	Status    domain.ReviewStatus `json:"status"`
	ErrorMsg  string              `json:"error_msg,omitempty"`
}

// SaveKeywordsInput is the input for the SaveKeywords activity.
type SaveKeywordsInput struct {
	RequestID       uuid.UUID  `json:"request_id"`
	Keywords        []string   `json:"keywords"`
	ExtractionRound int        `json:"extraction_round"`
	SourcePaperID   *uuid.UUID `json:"source_paper_id,omitempty"`
	SourceType      string     `json:"source_type"` // "query", "llm_extraction"
}

// SaveKeywordsOutput is the output from the SaveKeywords activity.
type SaveKeywordsOutput struct {
	KeywordIDs []uuid.UUID `json:"keyword_ids"`
	NewCount   int         `json:"new_count"`
}

// SavePapersInput is the input for the SavePapers activity.
type SavePapersInput struct {
	RequestID          uuid.UUID       `json:"request_id"`
	OrgID              string          `json:"org_id"`
	ProjectID          string          `json:"project_id"`
	Papers             []*domain.Paper `json:"papers"`
	DiscoveredViaKeywordID *uuid.UUID  `json:"discovered_via_keyword_id,omitempty"`
	DiscoveredViaSource domain.SourceType `json:"discovered_via_source"`
	ExpansionDepth     int             `json:"expansion_depth"`
}

// SavePapersOutput is the output from the SavePapers activity.
type SavePapersOutput struct {
	SavedCount    int `json:"saved_count"`
	DuplicateCount int `json:"duplicate_count"`
}

// IncrementCountersInput is the input for the IncrementCounters activity.
type IncrementCountersInput struct {
	OrgID          string    `json:"org_id"`
	ProjectID      string    `json:"project_id"`
	RequestID      uuid.UUID `json:"request_id"`
	PapersFound    int       `json:"papers_found"`
	PapersIngested int       `json:"papers_ingested"`
}
```

**Step 2: Create `internal/temporal/activities/llm_activities.go`**

```go
package activities

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"

	"github.com/helixir/literature-review-service/internal/llm"
	"github.com/helixir/literature-review-service/internal/observability"
)

// LLMActivities provides Temporal activities for LLM-based operations.
type LLMActivities struct {
	extractor llm.KeywordExtractor
	metrics   *observability.Metrics
}

// NewLLMActivities creates a new LLMActivities instance.
func NewLLMActivities(extractor llm.KeywordExtractor, metrics *observability.Metrics) *LLMActivities {
	return &LLMActivities{
		extractor: extractor,
		metrics:   metrics,
	}
}

// ExtractKeywords extracts research keywords from text using the configured LLM provider.
func (a *LLMActivities) ExtractKeywords(ctx context.Context, input ExtractKeywordsInput) (*ExtractKeywordsOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Extracting keywords", "mode", input.Mode, "text_length", len(input.Text))

	start := time.Now()

	req := llm.ExtractionRequest{
		Text:             input.Text,
		Mode:             llm.ExtractionMode(input.Mode),
		MaxKeywords:      input.MaxKeywords,
		MinKeywords:      input.MinKeywords,
		ExistingKeywords: input.ExistingKeywords,
		Context:          input.Context,
	}

	result, err := a.extractor.ExtractKeywords(ctx, req)
	if err != nil {
		duration := time.Since(start).Seconds()
		if a.metrics != nil {
			a.metrics.RecordLLMRequestFailed("keyword_extraction", a.extractor.Model(), "extraction_error")
		}
		logger.Error("Keyword extraction failed", "error", err, "duration_s", duration)
		return nil, fmt.Errorf("extract keywords: %w", err)
	}

	duration := time.Since(start).Seconds()
	if a.metrics != nil {
		a.metrics.RecordLLMRequest("keyword_extraction", result.Model, duration, result.InputTokens, result.OutputTokens)
		a.metrics.RecordKeywordsExtracted(input.Mode, len(result.Keywords))
	}

	logger.Info("Keywords extracted",
		"count", len(result.Keywords),
		"model", result.Model,
		"input_tokens", result.InputTokens,
		"output_tokens", result.OutputTokens,
		"duration_s", duration,
	)

	return &ExtractKeywordsOutput{
		Keywords:     result.Keywords,
		Reasoning:    result.Reasoning,
		Model:        result.Model,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	}, nil
}
```

**Step 3: Create `internal/temporal/activities/llm_activities_test.go`**

```go
package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/llm"
)

// mockKeywordExtractor implements llm.KeywordExtractor for tests.
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
	return "mock"
}

func (m *mockKeywordExtractor) Model() string {
	return "mock-model"
}

func TestExtractKeywords_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	extractor := new(mockKeywordExtractor)
	activities := NewLLMActivities(extractor, nil)
	env.RegisterActivity(activities.ExtractKeywords)

	extractor.On("ExtractKeywords", mock.Anything, mock.MatchedBy(func(req llm.ExtractionRequest) bool {
		return req.Text == "CRISPR gene editing" && req.Mode == llm.ExtractionModeQuery
	})).Return(&llm.ExtractionResult{
		Keywords:     []string{"CRISPR", "gene editing", "Cas9"},
		Reasoning:    "Core CRISPR terms",
		Model:        "gpt-4",
		InputTokens:  100,
		OutputTokens: 50,
	}, nil)

	input := ExtractKeywordsInput{
		Text:        "CRISPR gene editing",
		Mode:        "query",
		MaxKeywords: 10,
		MinKeywords: 3,
	}

	result, err := env.ExecuteActivity(activities.ExtractKeywords, input)
	require.NoError(t, err)

	var output ExtractKeywordsOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, []string{"CRISPR", "gene editing", "Cas9"}, output.Keywords)
	assert.Equal(t, "gpt-4", output.Model)
	assert.Equal(t, 100, output.InputTokens)
	assert.Equal(t, 50, output.OutputTokens)

	extractor.AssertExpectations(t)
}

func TestExtractKeywords_Error(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	extractor := new(mockKeywordExtractor)
	activities := NewLLMActivities(extractor, nil)
	env.RegisterActivity(activities.ExtractKeywords)

	extractor.On("ExtractKeywords", mock.Anything, mock.Anything).
		Return(nil, errors.New("API rate limited"))

	input := ExtractKeywordsInput{
		Text:        "test query",
		Mode:        "query",
		MaxKeywords: 10,
		MinKeywords: 3,
	}

	_, err := env.ExecuteActivity(activities.ExtractKeywords, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract keywords")

	extractor.AssertExpectations(t)
}

func TestExtractKeywords_WithExistingKeywords(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	extractor := new(mockKeywordExtractor)
	activities := NewLLMActivities(extractor, nil)
	env.RegisterActivity(activities.ExtractKeywords)

	extractor.On("ExtractKeywords", mock.Anything, mock.MatchedBy(func(req llm.ExtractionRequest) bool {
		return len(req.ExistingKeywords) == 2 && req.Mode == llm.ExtractionModeAbstract
	})).Return(&llm.ExtractionResult{
		Keywords:     []string{"new-keyword1", "new-keyword2"},
		Model:        "gpt-4",
		InputTokens:  200,
		OutputTokens: 60,
	}, nil)

	input := ExtractKeywordsInput{
		Text:             "Paper abstract about gene therapy...",
		Mode:             "abstract",
		MaxKeywords:      10,
		MinKeywords:      3,
		ExistingKeywords: []string{"CRISPR", "Cas9"},
	}

	result, err := env.ExecuteActivity(activities.ExtractKeywords, input)
	require.NoError(t, err)

	var output ExtractKeywordsOutput
	require.NoError(t, result.Get(&output))
	assert.Len(t, output.Keywords, 2)

	extractor.AssertExpectations(t)
}
```

**Step 4: Run tests**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test ./internal/temporal/activities/... -v -race -count=1`
Expected: All 3 tests PASS

**Step 5: Commit**

```bash
git add internal/temporal/activities/
git commit -m "feat(temporal): add activity types and LLM activities with tests"
```

---

## Task 2: Implement Search Activities

**Files:**
- Create: `internal/temporal/activities/search_activities.go`
- Create: `internal/temporal/activities/search_activities_test.go`

**Context:**
- `internal/papersources/registry.go` defines `Registry` with `SearchAll`, `SearchSources` methods
- `internal/papersources/source.go` defines `SearchParams`, `SearchResult`, `PaperSource` interface
- `internal/observability/metrics.go` has `RecordSearchStarted`, `RecordSearchCompleted`, `RecordSearchFailed`, `RecordPapersDiscovered`
- The activity wraps `Registry.SearchSources()` converting `SourceResult` list into `SearchPapersOutput`

**Step 1: Create `internal/temporal/activities/search_activities.go`**

```go
package activities

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/observability"
	"github.com/helixir/literature-review-service/internal/papersources"
)

// SearchActivities provides Temporal activities for paper source searches.
type SearchActivities struct {
	registry *papersources.Registry
	metrics  *observability.Metrics
}

// NewSearchActivities creates a new SearchActivities instance.
func NewSearchActivities(registry *papersources.Registry, metrics *observability.Metrics) *SearchActivities {
	return &SearchActivities{
		registry: registry,
		metrics:  metrics,
	}
}

// SearchPapers searches for papers across configured academic sources.
func (a *SearchActivities) SearchPapers(ctx context.Context, input SearchPapersInput) (*SearchPapersOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Searching papers", "query", input.Query, "sources", input.Sources, "max_results", input.MaxResults)

	start := time.Now()

	params := papersources.SearchParams{
		Query:            input.Query,
		MaxResults:       input.MaxResults,
		IncludePreprints: input.IncludePreprints,
		OpenAccessOnly:   input.OpenAccessOnly,
		MinCitations:     input.MinCitations,
	}

	// Record search started for each source
	if a.metrics != nil {
		for _, s := range input.Sources {
			a.metrics.RecordSearchStarted(string(s))
		}
	}

	results := a.registry.SearchSources(ctx, params, input.Sources)

	output := &SearchPapersOutput{
		Papers:   make([]*domain.Paper, 0),
		BySource: make(map[domain.SourceType]int),
	}

	for _, sr := range results {
		if sr.Error != nil {
			output.Errors = append(output.Errors, SourceError{
				Source: sr.Source,
				Error:  sr.Error.Error(),
			})
			if a.metrics != nil {
				duration := time.Since(start).Seconds()
				a.metrics.RecordSearchFailed(string(sr.Source), duration)
			}
			logger.Warn("Source search failed", "source", sr.Source, "error", sr.Error)
			continue
		}

		if sr.Result != nil {
			output.Papers = append(output.Papers, sr.Result.Papers...)
			output.TotalFound += sr.Result.TotalResults
			output.BySource[sr.Source] = len(sr.Result.Papers)

			if a.metrics != nil {
				duration := sr.Result.SearchDuration.Seconds()
				a.metrics.RecordSearchCompleted(string(sr.Source), len(sr.Result.Papers), duration)
				a.metrics.RecordPapersDiscovered(string(sr.Source), len(sr.Result.Papers))
			}
		}
	}

	duration := time.Since(start).Seconds()
	logger.Info("Paper search completed",
		"total_papers", len(output.Papers),
		"total_found", output.TotalFound,
		"errors", len(output.Errors),
		"duration_s", duration,
	)

	// If all sources failed, return an error
	if len(output.Papers) == 0 && len(output.Errors) > 0 {
		return nil, fmt.Errorf("all source searches failed: %d errors", len(output.Errors))
	}

	return output, nil
}
```

**Step 2: Create `internal/temporal/activities/search_activities_test.go`**

Test using mock PaperSource implementations registered in a Registry. Tests:
- Successful search with multiple sources returning papers
- Partial failure (one source fails, others succeed)
- All sources fail → returns error
- Empty results (no papers found)

```go
package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
)

// mockPaperSource implements papersources.PaperSource for tests.
type mockPaperSource struct {
	sourceType domain.SourceType
	name       string
	enabled    bool
	searchFn   func(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error)
}

func (m *mockPaperSource) Search(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
	return m.searchFn(ctx, params)
}

func (m *mockPaperSource) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	return nil, domain.ErrNotFound
}

func (m *mockPaperSource) SourceType() domain.SourceType { return m.sourceType }
func (m *mockPaperSource) Name() string                  { return m.name }
func (m *mockPaperSource) IsEnabled() bool               { return m.enabled }

func TestSearchPapers_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	registry := papersources.NewRegistry()
	registry.Register(&mockPaperSource{
		sourceType: domain.SourceTypeSemanticScholar,
		name:       "Semantic Scholar",
		enabled:    true,
		searchFn: func(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
			return &papersources.SearchResult{
				Papers: []*domain.Paper{
					{Title: "Paper 1", CanonicalID: "doi:10.1234/p1"},
					{Title: "Paper 2", CanonicalID: "doi:10.1234/p2"},
				},
				TotalResults:   2,
				Source:         domain.SourceTypeSemanticScholar,
				SearchDuration: 500 * time.Millisecond,
			}, nil
		},
	})

	act := NewSearchActivities(registry, nil)
	env.RegisterActivity(act.SearchPapers)

	input := SearchPapersInput{
		Query:      "CRISPR",
		Sources:    []domain.SourceType{domain.SourceTypeSemanticScholar},
		MaxResults: 100,
	}

	result, err := env.ExecuteActivity(act.SearchPapers, input)
	require.NoError(t, err)

	var output SearchPapersOutput
	require.NoError(t, result.Get(&output))
	assert.Len(t, output.Papers, 2)
	assert.Equal(t, 2, output.TotalFound)
	assert.Equal(t, 2, output.BySource[domain.SourceTypeSemanticScholar])
	assert.Empty(t, output.Errors)
}

func TestSearchPapers_PartialFailure(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	registry := papersources.NewRegistry()
	registry.Register(&mockPaperSource{
		sourceType: domain.SourceTypeSemanticScholar,
		name:       "Semantic Scholar",
		enabled:    true,
		searchFn: func(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
			return &papersources.SearchResult{
				Papers:         []*domain.Paper{{Title: "Paper 1", CanonicalID: "doi:1"}},
				TotalResults:   1,
				Source:         domain.SourceTypeSemanticScholar,
				SearchDuration: 100 * time.Millisecond,
			}, nil
		},
	})
	registry.Register(&mockPaperSource{
		sourceType: domain.SourceTypeOpenAlex,
		name:       "OpenAlex",
		enabled:    true,
		searchFn: func(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
			return nil, errors.New("connection timeout")
		},
	})

	act := NewSearchActivities(registry, nil)
	env.RegisterActivity(act.SearchPapers)

	input := SearchPapersInput{
		Query:      "CRISPR",
		Sources:    []domain.SourceType{domain.SourceTypeSemanticScholar, domain.SourceTypeOpenAlex},
		MaxResults: 100,
	}

	result, err := env.ExecuteActivity(act.SearchPapers, input)
	require.NoError(t, err) // partial failure is not a fatal error

	var output SearchPapersOutput
	require.NoError(t, result.Get(&output))
	assert.Len(t, output.Papers, 1)
	assert.Len(t, output.Errors, 1)
	assert.Equal(t, domain.SourceTypeOpenAlex, output.Errors[0].Source)
}

func TestSearchPapers_AllFail(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	registry := papersources.NewRegistry()
	registry.Register(&mockPaperSource{
		sourceType: domain.SourceTypePubMed,
		name:       "PubMed",
		enabled:    true,
		searchFn: func(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
			return nil, errors.New("service unavailable")
		},
	})

	act := NewSearchActivities(registry, nil)
	env.RegisterActivity(act.SearchPapers)

	input := SearchPapersInput{
		Query:      "CRISPR",
		Sources:    []domain.SourceType{domain.SourceTypePubMed},
		MaxResults: 100,
	}

	_, err := env.ExecuteActivity(act.SearchPapers, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all source searches failed")
}
```

**Step 3: Run tests**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test ./internal/temporal/activities/... -v -race -count=1`
Expected: All tests PASS (LLM + Search activities)

**Step 4: Commit**

```bash
git add internal/temporal/activities/search_activities.go internal/temporal/activities/search_activities_test.go
git commit -m "feat(temporal): add search activities with tests"
```

---

## Task 3: Implement Status Activities

**Files:**
- Create: `internal/temporal/activities/status_activities.go`
- Create: `internal/temporal/activities/status_activities_test.go`

**Context:**
- `internal/repository/review_repository.go` defines `ReviewRepository` interface with `UpdateStatus`, `IncrementCounters`
- `internal/repository/keyword_repository.go` defines `KeywordRepository` with `BulkGetOrCreate`
- `internal/repository/paper_repository.go` defines `PaperRepository` with `BulkUpsert`
- `internal/domain/review.go` defines `RequestKeywordMapping`, `RequestPaperMapping`
- These activities handle persistence side-effects (DB writes) from the workflow

**Step 1: Create `internal/temporal/activities/status_activities.go`**

```go
package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/observability"
	"github.com/helixir/literature-review-service/internal/repository"
)

// StatusActivities provides Temporal activities for review status and persistence operations.
type StatusActivities struct {
	reviewRepo  repository.ReviewRepository
	keywordRepo repository.KeywordRepository
	paperRepo   repository.PaperRepository
	metrics     *observability.Metrics
}

// NewStatusActivities creates a new StatusActivities instance.
func NewStatusActivities(
	reviewRepo repository.ReviewRepository,
	keywordRepo repository.KeywordRepository,
	paperRepo repository.PaperRepository,
	metrics *observability.Metrics,
) *StatusActivities {
	return &StatusActivities{
		reviewRepo:  reviewRepo,
		keywordRepo: keywordRepo,
		paperRepo:   paperRepo,
		metrics:     metrics,
	}
}

// UpdateStatus updates the status of a literature review request.
func (a *StatusActivities) UpdateStatus(ctx context.Context, input UpdateStatusInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Updating review status", "request_id", input.RequestID, "status", input.Status)

	err := a.reviewRepo.UpdateStatus(ctx, input.OrgID, input.ProjectID, input.RequestID, input.Status, input.ErrorMsg)
	if err != nil {
		return fmt.Errorf("update review status: %w", err)
	}

	// Record metrics for terminal states
	if a.metrics != nil {
		switch input.Status {
		case domain.ReviewStatusFailed:
			a.metrics.RecordReviewFailed(0)
		case domain.ReviewStatusCancelled:
			a.metrics.RecordReviewCancelled()
		}
	}

	return nil
}

// SaveKeywords persists extracted keywords and creates request-keyword mappings.
func (a *StatusActivities) SaveKeywords(ctx context.Context, input SaveKeywordsInput) (*SaveKeywordsOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Saving keywords", "request_id", input.RequestID, "count", len(input.Keywords))

	// Bulk get-or-create keywords (handles normalization and deduplication)
	keywords, err := a.keywordRepo.BulkGetOrCreate(ctx, input.Keywords)
	if err != nil {
		return nil, fmt.Errorf("bulk get or create keywords: %w", err)
	}

	// Create request-keyword mappings
	keywordIDs := make([]uuid.UUID, 0, len(keywords))
	for _, kw := range keywords {
		keywordIDs = append(keywordIDs, kw.ID)

		mapping := &domain.RequestKeywordMapping{
			ID:              uuid.New(),
			RequestID:       input.RequestID,
			KeywordID:       kw.ID,
			ExtractionRound: input.ExtractionRound,
			SourcePaperID:   input.SourcePaperID,
			SourceType:      input.SourceType,
			CreatedAt:       time.Now(),
		}
		// Note: RequestKeywordMapping persistence would typically be done via ReviewRepository
		// For now we just track keyword IDs; the mapping will be saved when the repository supports it
		_ = mapping
	}

	logger.Info("Keywords saved", "saved_count", len(keywords))

	return &SaveKeywordsOutput{
		KeywordIDs: keywordIDs,
		NewCount:   len(keywords),
	}, nil
}

// SavePapers persists discovered papers and updates review counters.
func (a *StatusActivities) SavePapers(ctx context.Context, input SavePapersInput) (*SavePapersOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Saving papers", "request_id", input.RequestID, "count", len(input.Papers))

	if len(input.Papers) == 0 {
		return &SavePapersOutput{}, nil
	}

	// Bulk upsert papers (handles deduplication by canonical_id)
	savedPapers, err := a.paperRepo.BulkUpsert(ctx, input.Papers)
	if err != nil {
		return nil, fmt.Errorf("bulk upsert papers: %w", err)
	}

	savedCount := len(savedPapers)
	duplicateCount := len(input.Papers) - savedCount

	// Increment review counters
	if err := a.reviewRepo.IncrementCounters(ctx, input.OrgID, input.ProjectID, input.RequestID, savedCount, 0); err != nil {
		logger.Warn("Failed to increment review counters", "error", err)
	}

	// Record metrics
	if a.metrics != nil {
		a.metrics.RecordPapersDiscovered(string(input.DiscoveredViaSource), savedCount)
		for i := 0; i < duplicateCount; i++ {
			a.metrics.RecordPaperDuplicate()
		}
	}

	logger.Info("Papers saved", "saved", savedCount, "duplicates", duplicateCount)

	return &SavePapersOutput{
		SavedCount:     savedCount,
		DuplicateCount: duplicateCount,
	}, nil
}

// IncrementCounters atomically increments review progress counters.
func (a *StatusActivities) IncrementCounters(ctx context.Context, input IncrementCountersInput) error {
	return a.reviewRepo.IncrementCounters(ctx, input.OrgID, input.ProjectID, input.RequestID, input.PapersFound, input.PapersIngested)
}
```

**Step 2: Create `internal/temporal/activities/status_activities_test.go`**

Tests using mock repositories. Tests:
- UpdateStatus success
- SaveKeywords with BulkGetOrCreate
- SavePapers with BulkUpsert and counter increment
- SavePapers with empty input (no-op)
- IncrementCounters

```go
package activities

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
)

// --- Mock Repositories ---

type mockReviewRepo struct {
	mock.Mock
}

func (m *mockReviewRepo) Create(ctx context.Context, review *domain.LiteratureReviewRequest) error {
	return m.Called(ctx, review).Error(0)
}

func (m *mockReviewRepo) Get(ctx context.Context, orgID, projectID string, id uuid.UUID) (*domain.LiteratureReviewRequest, error) {
	args := m.Called(ctx, orgID, projectID, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.LiteratureReviewRequest), args.Error(1)
}

func (m *mockReviewRepo) Update(ctx context.Context, orgID, projectID string, id uuid.UUID, fn func(*domain.LiteratureReviewRequest) error) error {
	return m.Called(ctx, orgID, projectID, id, fn).Error(0)
}

func (m *mockReviewRepo) UpdateStatus(ctx context.Context, orgID, projectID string, id uuid.UUID, status domain.ReviewStatus, errorMsg string) error {
	return m.Called(ctx, orgID, projectID, id, status, errorMsg).Error(0)
}

func (m *mockReviewRepo) List(ctx context.Context, filter repository.ReviewFilter) ([]*domain.LiteratureReviewRequest, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*domain.LiteratureReviewRequest), args.Get(1).(int64), args.Error(2)
}

func (m *mockReviewRepo) IncrementCounters(ctx context.Context, orgID, projectID string, id uuid.UUID, papersFound, papersIngested int) error {
	return m.Called(ctx, orgID, projectID, id, papersFound, papersIngested).Error(0)
}

func (m *mockReviewRepo) GetByWorkflowID(ctx context.Context, workflowID string) (*domain.LiteratureReviewRequest, error) {
	args := m.Called(ctx, workflowID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.LiteratureReviewRequest), args.Error(1)
}

type mockKeywordRepo struct {
	mock.Mock
}

// Implement all KeywordRepository methods (only BulkGetOrCreate used in tests)
func (m *mockKeywordRepo) GetOrCreate(ctx context.Context, keyword string) (*domain.Keyword, error) {
	args := m.Called(ctx, keyword)
	return args.Get(0).(*domain.Keyword), args.Error(1)
}

func (m *mockKeywordRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Keyword, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*domain.Keyword), args.Error(1)
}

func (m *mockKeywordRepo) GetByNormalized(ctx context.Context, normalized string) (*domain.Keyword, error) {
	args := m.Called(ctx, normalized)
	return args.Get(0).(*domain.Keyword), args.Error(1)
}

func (m *mockKeywordRepo) BulkGetOrCreate(ctx context.Context, keywords []string) ([]*domain.Keyword, error) {
	args := m.Called(ctx, keywords)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Keyword), args.Error(1)
}

func (m *mockKeywordRepo) RecordSearch(ctx context.Context, search *domain.KeywordSearch) error {
	return m.Called(ctx, search).Error(0)
}

func (m *mockKeywordRepo) GetLastSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI domain.SourceType) (*domain.KeywordSearch, error) {
	args := m.Called(ctx, keywordID, sourceAPI)
	return args.Get(0).(*domain.KeywordSearch), args.Error(1)
}

func (m *mockKeywordRepo) NeedsSearch(ctx context.Context, keywordID uuid.UUID, sourceAPI domain.SourceType, maxAge interface{}) (bool, error) {
	args := m.Called(ctx, keywordID, sourceAPI, maxAge)
	return args.Bool(0), args.Error(1)
}

func (m *mockKeywordRepo) ListSearches(ctx context.Context, filter repository.SearchFilter) ([]*domain.KeywordSearch, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*domain.KeywordSearch), args.Get(1).(int64), args.Error(2)
}

func (m *mockKeywordRepo) AddPaperMapping(ctx context.Context, mapping *domain.KeywordPaperMapping) error {
	return m.Called(ctx, mapping).Error(0)
}

func (m *mockKeywordRepo) BulkAddPaperMappings(ctx context.Context, mappings []*domain.KeywordPaperMapping) error {
	return m.Called(ctx, mappings).Error(0)
}

func (m *mockKeywordRepo) GetPapersForKeyword(ctx context.Context, keywordID uuid.UUID, limit, offset int) ([]*domain.Paper, int64, error) {
	args := m.Called(ctx, keywordID, limit, offset)
	return args.Get(0).([]*domain.Paper), args.Get(1).(int64), args.Error(2)
}

func (m *mockKeywordRepo) List(ctx context.Context, filter repository.KeywordFilter) ([]*domain.Keyword, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*domain.Keyword), args.Get(1).(int64), args.Error(2)
}

type mockPaperRepo struct {
	mock.Mock
}

func (m *mockPaperRepo) Create(ctx context.Context, paper *domain.Paper) (*domain.Paper, error) {
	args := m.Called(ctx, paper)
	return args.Get(0).(*domain.Paper), args.Error(1)
}

func (m *mockPaperRepo) GetByCanonicalID(ctx context.Context, canonicalID string) (*domain.Paper, error) {
	args := m.Called(ctx, canonicalID)
	return args.Get(0).(*domain.Paper), args.Error(1)
}

func (m *mockPaperRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Paper, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*domain.Paper), args.Error(1)
}

func (m *mockPaperRepo) FindByIdentifier(ctx context.Context, idType domain.IdentifierType, value string) (*domain.Paper, error) {
	args := m.Called(ctx, idType, value)
	return args.Get(0).(*domain.Paper), args.Error(1)
}

func (m *mockPaperRepo) UpsertIdentifier(ctx context.Context, paperID uuid.UUID, idType domain.IdentifierType, value string, sourceAPI domain.SourceType) error {
	return m.Called(ctx, paperID, idType, value, sourceAPI).Error(0)
}

func (m *mockPaperRepo) AddSource(ctx context.Context, paperID uuid.UUID, sourceAPI domain.SourceType, metadata map[string]interface{}) error {
	return m.Called(ctx, paperID, sourceAPI, metadata).Error(0)
}

func (m *mockPaperRepo) List(ctx context.Context, filter repository.PaperFilter) ([]*domain.Paper, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*domain.Paper), args.Get(1).(int64), args.Error(2)
}

func (m *mockPaperRepo) MarkKeywordsExtracted(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *mockPaperRepo) BulkUpsert(ctx context.Context, papers []*domain.Paper) ([]*domain.Paper, error) {
	args := m.Called(ctx, papers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Paper), args.Error(1)
}

// --- Tests ---

func TestUpdateStatus_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	reviewRepo := new(mockReviewRepo)
	act := NewStatusActivities(reviewRepo, nil, nil, nil)
	env.RegisterActivity(act.UpdateStatus)

	reqID := uuid.New()
	reviewRepo.On("UpdateStatus", mock.Anything, "org1", "proj1", reqID, domain.ReviewStatusSearching, "").Return(nil)

	input := UpdateStatusInput{
		OrgID:     "org1",
		ProjectID: "proj1",
		RequestID: reqID,
		Status:    domain.ReviewStatusSearching,
	}

	_, err := env.ExecuteActivity(act.UpdateStatus, input)
	require.NoError(t, err)
	reviewRepo.AssertExpectations(t)
}

func TestSaveKeywords_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	keywordRepo := new(mockKeywordRepo)
	act := NewStatusActivities(nil, keywordRepo, nil, nil)
	env.RegisterActivity(act.SaveKeywords)

	kw1ID := uuid.New()
	kw2ID := uuid.New()
	keywordRepo.On("BulkGetOrCreate", mock.Anything, []string{"CRISPR", "gene editing"}).
		Return([]*domain.Keyword{
			{ID: kw1ID, Normalized: "crispr"},
			{ID: kw2ID, Normalized: "gene editing"},
		}, nil)

	input := SaveKeywordsInput{
		RequestID:       uuid.New(),
		Keywords:        []string{"CRISPR", "gene editing"},
		ExtractionRound: 0,
		SourceType:      "query",
	}

	result, err := env.ExecuteActivity(act.SaveKeywords, input)
	require.NoError(t, err)

	var output SaveKeywordsOutput
	require.NoError(t, result.Get(&output))
	assert.Len(t, output.KeywordIDs, 2)
	assert.Equal(t, 2, output.NewCount)
	keywordRepo.AssertExpectations(t)
}

func TestSavePapers_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	reviewRepo := new(mockReviewRepo)
	paperRepo := new(mockPaperRepo)
	act := NewStatusActivities(reviewRepo, nil, paperRepo, nil)
	env.RegisterActivity(act.SavePapers)

	reqID := uuid.New()
	papers := []*domain.Paper{
		{Title: "Paper 1", CanonicalID: "doi:1"},
		{Title: "Paper 2", CanonicalID: "doi:2"},
	}

	paperRepo.On("BulkUpsert", mock.Anything, papers).Return(papers, nil)
	reviewRepo.On("IncrementCounters", mock.Anything, "org1", "proj1", reqID, 2, 0).Return(nil)

	input := SavePapersInput{
		RequestID:           reqID,
		OrgID:               "org1",
		ProjectID:           "proj1",
		Papers:              papers,
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
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	act := NewStatusActivities(nil, nil, nil, nil)
	env.RegisterActivity(act.SavePapers)

	input := SavePapersInput{
		RequestID: uuid.New(),
		OrgID:     "org1",
		ProjectID: "proj1",
		Papers:    []*domain.Paper{},
	}

	result, err := env.ExecuteActivity(act.SavePapers, input)
	require.NoError(t, err)

	var output SavePapersOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, 0, output.SavedCount)
}
```

**Step 3: Run tests**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test ./internal/temporal/activities/... -v -race -count=1`
Expected: All tests PASS

**Step 4: Commit**

```bash
git add internal/temporal/activities/status_activities.go internal/temporal/activities/status_activities_test.go
git commit -m "feat(temporal): add status activities for persistence operations"
```

---

## Task 4: Implement LiteratureReviewWorkflow

**Files:**
- Create: `internal/temporal/workflows/review_workflow.go`
- Create: `internal/temporal/workflows/review_workflow_test.go`
- Modify: `internal/temporal/worker.go` (update ActivityDependencies to use typed activity structs)

**Context:**
- `internal/temporal/activities/` contains all activity types and implementations
- `internal/temporal/client.go` defines `ReviewWorkflowRequest`
- `internal/domain/models.go` defines `ReviewStatus` enum with all status values
- `internal/domain/review.go` defines `ReviewConfiguration` with `MaxPapers`, `MaxExpansionDepth`, `MaxKeywordsPerRound`, `Sources`

The workflow orchestrates:
1. Update status to `extracting_keywords`
2. Extract keywords from query (LLM activity)
3. Save keywords
4. Update status to `searching`
5. Search papers across sources
6. Save discovered papers
7. For each expansion round (up to MaxExpansionDepth):
   - Update status to `expanding`
   - Extract keywords from paper abstracts (select top papers)
   - Save new keywords
   - Search for new papers with new keywords
   - Save new papers
8. Update status to `completed`
9. Return workflow result

Uses `workflow.SetQueryHandler` for progress query and `workflow.GetSignalChannel` for cancellation.

**Step 1: Create `internal/temporal/workflows/review_workflow.go`**

```go
package workflows

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
)

const (
	// SignalCancel is the signal name for cancelling a review workflow.
	SignalCancel = "cancel"

	// QueryProgress is the query name for retrieving workflow progress.
	QueryProgress = "progress"

	// Default activity timeout for LLM operations.
	llmActivityTimeout = 2 * time.Minute

	// Default activity timeout for search operations.
	searchActivityTimeout = 5 * time.Minute

	// Default activity timeout for status/persistence operations.
	statusActivityTimeout = 30 * time.Second

	// Maximum papers to extract keywords from per expansion round.
	maxPapersForExpansion = 5
)

// ReviewWorkflowInput is the input for the LiteratureReviewWorkflow.
type ReviewWorkflowInput struct {
	RequestID   uuid.UUID              `json:"request_id"`
	OrgID       string                 `json:"org_id"`
	ProjectID   string                 `json:"project_id"`
	UserID      string                 `json:"user_id"`
	Query       string                 `json:"query"`
	Config      domain.ReviewConfiguration `json:"config"`
}

// ReviewWorkflowResult is the output from the LiteratureReviewWorkflow.
type ReviewWorkflowResult struct {
	RequestID       uuid.UUID `json:"request_id"`
	Status          string    `json:"status"`
	KeywordsFound   int       `json:"keywords_found"`
	PapersFound     int       `json:"papers_found"`
	PapersIngested  int       `json:"papers_ingested"`
	ExpansionRounds int       `json:"expansion_rounds"`
	Duration        float64   `json:"duration_seconds"`
}

// workflowProgress tracks the progress of the review workflow.
type workflowProgress struct {
	Status           string `json:"status"`
	Phase            string `json:"phase"`
	KeywordsFound    int    `json:"keywords_found"`
	PapersFound      int    `json:"papers_found"`
	ExpansionRound   int    `json:"expansion_round"`
	MaxExpansionDepth int   `json:"max_expansion_depth"`
}

// LiteratureReviewWorkflow orchestrates the literature review pipeline.
// It extracts keywords, searches papers, and recursively expands the search.
func LiteratureReviewWorkflow(ctx workflow.Context, input ReviewWorkflowInput) (*ReviewWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting literature review workflow",
		"request_id", input.RequestID,
		"query", input.Query,
		"max_papers", input.Config.MaxPapers,
		"max_depth", input.Config.MaxExpansionDepth,
	)

	startTime := workflow.Now(ctx)

	// Initialize progress tracking
	progress := &workflowProgress{
		Status:            string(domain.ReviewStatusPending),
		Phase:             "initializing",
		MaxExpansionDepth: input.Config.MaxExpansionDepth,
	}

	// Register progress query handler
	err := workflow.SetQueryHandler(ctx, QueryProgress, func() (*workflowProgress, error) {
		return progress, nil
	})
	if err != nil {
		return nil, fmt.Errorf("register query handler: %w", err)
	}

	// Set up cancellation signal channel
	cancelCh := workflow.GetSignalChannel(ctx, SignalCancel)

	// Create cancellable context
	ctx, cancelFunc := workflow.WithCancel(ctx)
	defer cancelFunc()

	// Monitor cancellation signal in a goroutine
	workflow.Go(ctx, func(gCtx workflow.Context) {
		cancelCh.Receive(gCtx, nil)
		logger.Info("Cancellation signal received")
		cancelFunc()
	})

	// Activity options
	llmOpts := workflow.ActivityOptions{
		StartToCloseTimeout: llmActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	}

	searchOpts := workflow.ActivityOptions{
		StartToCloseTimeout: searchActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}

	statusOpts := workflow.ActivityOptions{
		StartToCloseTimeout: statusActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    5,
		},
	}

	llmCtx := workflow.WithActivityOptions(ctx, llmOpts)
	searchCtx := workflow.WithActivityOptions(ctx, searchOpts)
	statusCtx := workflow.WithActivityOptions(ctx, statusOpts)

	// Track all keywords to avoid duplicates across expansion rounds
	allKeywords := make([]string, 0)
	totalPapersFound := 0

	// Helper to update status
	updateStatus := func(status domain.ReviewStatus) error {
		progress.Status = string(status)
		return workflow.ExecuteActivity(statusCtx, "StatusActivities.UpdateStatus", activities.UpdateStatusInput{
			OrgID:     input.OrgID,
			ProjectID: input.ProjectID,
			RequestID: input.RequestID,
			Status:    status,
		}).Get(ctx, nil)
	}

	// Helper to handle failure
	handleFailure := func(phase string, err error) (*ReviewWorkflowResult, error) {
		logger.Error("Workflow failed", "phase", phase, "error", err)
		progress.Status = string(domain.ReviewStatusFailed)
		progress.Phase = phase

		_ = workflow.ExecuteActivity(statusCtx, "StatusActivities.UpdateStatus", activities.UpdateStatusInput{
			OrgID:     input.OrgID,
			ProjectID: input.ProjectID,
			RequestID: input.RequestID,
			Status:    domain.ReviewStatusFailed,
			ErrorMsg:  fmt.Sprintf("%s: %v", phase, err),
		}).Get(ctx, nil)

		return nil, fmt.Errorf("%s: %w", phase, err)
	}

	// --- Phase 1: Extract keywords from query ---
	progress.Phase = "extracting_keywords"
	if err := updateStatus(domain.ReviewStatusExtractingKeywords); err != nil {
		return handleFailure("update_status_extracting", err)
	}

	maxKeywords := input.Config.MaxKeywordsPerRound
	if maxKeywords == 0 {
		maxKeywords = 10
	}

	var extractOutput activities.ExtractKeywordsOutput
	err = workflow.ExecuteActivity(llmCtx, "LLMActivities.ExtractKeywords", activities.ExtractKeywordsInput{
		Text:        input.Query,
		Mode:        string(domain.ReviewStatusExtractingKeywords),
		MaxKeywords: maxKeywords,
		MinKeywords: 3,
	}).Get(ctx, &extractOutput)
	if err != nil {
		return handleFailure("extract_keywords", err)
	}

	allKeywords = append(allKeywords, extractOutput.Keywords...)
	progress.KeywordsFound = len(allKeywords)

	// Save keywords
	var saveKwOutput activities.SaveKeywordsOutput
	err = workflow.ExecuteActivity(statusCtx, "StatusActivities.SaveKeywords", activities.SaveKeywordsInput{
		RequestID:       input.RequestID,
		Keywords:        extractOutput.Keywords,
		ExtractionRound: 0,
		SourceType:      "query",
	}).Get(ctx, &saveKwOutput)
	if err != nil {
		return handleFailure("save_keywords", err)
	}

	// --- Phase 2: Search papers ---
	progress.Phase = "searching"
	if err := updateStatus(domain.ReviewStatusSearching); err != nil {
		return handleFailure("update_status_searching", err)
	}

	// Build source list from config
	sources := make([]domain.SourceType, 0, len(input.Config.Sources))
	sources = append(sources, input.Config.Sources...)

	// Search for each keyword
	var allPapers []*domain.Paper
	for _, keyword := range extractOutput.Keywords {
		var searchOutput activities.SearchPapersOutput
		err = workflow.ExecuteActivity(searchCtx, "SearchActivities.SearchPapers", activities.SearchPapersInput{
			Query:            keyword,
			Sources:          sources,
			MaxResults:       input.Config.MaxPapers / len(extractOutput.Keywords),
			IncludePreprints: input.Config.IncludePreprints,
			OpenAccessOnly:   input.Config.RequireOpenAccess,
			MinCitations:     input.Config.MinCitations,
		}).Get(ctx, &searchOutput)
		if err != nil {
			logger.Warn("Search failed for keyword", "keyword", keyword, "error", err)
			continue // Non-fatal: skip this keyword
		}

		allPapers = append(allPapers, searchOutput.Papers...)
		totalPapersFound += len(searchOutput.Papers)
		progress.PapersFound = totalPapersFound

		// Check if we've reached the paper limit
		if totalPapersFound >= input.Config.MaxPapers {
			break
		}
	}

	// Save discovered papers
	if len(allPapers) > 0 {
		err = workflow.ExecuteActivity(statusCtx, "StatusActivities.SavePapers", activities.SavePapersInput{
			RequestID:           input.RequestID,
			OrgID:               input.OrgID,
			ProjectID:           input.ProjectID,
			Papers:              allPapers,
			DiscoveredViaSource: domain.SourceTypeSemanticScholar,
			ExpansionDepth:      0,
		}).Get(ctx, nil)
		if err != nil {
			return handleFailure("save_papers", err)
		}
	}

	// --- Phase 3: Recursive expansion ---
	for round := 1; round <= input.Config.MaxExpansionDepth; round++ {
		if totalPapersFound >= input.Config.MaxPapers {
			logger.Info("Paper limit reached, stopping expansion", "total", totalPapersFound)
			break
		}

		progress.Phase = "expanding"
		progress.ExpansionRound = round
		if err := updateStatus(domain.ReviewStatusExpanding); err != nil {
			return handleFailure("update_status_expanding", err)
		}

		// Select top papers for keyword extraction (those with abstracts)
		papersForExpansion := selectPapersForExpansion(allPapers, maxPapersForExpansion)
		if len(papersForExpansion) == 0 {
			logger.Info("No papers with abstracts for expansion, stopping")
			break
		}

		// Extract keywords from each selected paper's abstract
		var roundKeywords []string
		for _, paper := range papersForExpansion {
			if paper.Abstract == "" {
				continue
			}

			var expandOutput activities.ExtractKeywordsOutput
			err = workflow.ExecuteActivity(llmCtx, "LLMActivities.ExtractKeywords", activities.ExtractKeywordsInput{
				Text:             paper.Abstract,
				Mode:             "abstract",
				MaxKeywords:      maxKeywords,
				MinKeywords:      2,
				ExistingKeywords: allKeywords,
			}).Get(ctx, &expandOutput)
			if err != nil {
				logger.Warn("Expansion keyword extraction failed", "paper", paper.Title, "error", err)
				continue // Non-fatal
			}

			roundKeywords = append(roundKeywords, expandOutput.Keywords...)
		}

		if len(roundKeywords) == 0 {
			logger.Info("No new keywords from expansion, stopping")
			break
		}

		allKeywords = append(allKeywords, roundKeywords...)
		progress.KeywordsFound = len(allKeywords)

		// Save expansion keywords
		err = workflow.ExecuteActivity(statusCtx, "StatusActivities.SaveKeywords", activities.SaveKeywordsInput{
			RequestID:       input.RequestID,
			Keywords:        roundKeywords,
			ExtractionRound: round,
			SourceType:      "llm_extraction",
		}).Get(ctx, nil)
		if err != nil {
			logger.Warn("Failed to save expansion keywords", "error", err)
		}

		// Search with new keywords
		var roundPapers []*domain.Paper
		for _, keyword := range roundKeywords {
			if totalPapersFound >= input.Config.MaxPapers {
				break
			}

			var searchOutput activities.SearchPapersOutput
			err = workflow.ExecuteActivity(searchCtx, "SearchActivities.SearchPapers", activities.SearchPapersInput{
				Query:            keyword,
				Sources:          sources,
				MaxResults:       input.Config.MaxPapers / len(roundKeywords),
				IncludePreprints: input.Config.IncludePreprints,
				OpenAccessOnly:   input.Config.RequireOpenAccess,
				MinCitations:     input.Config.MinCitations,
			}).Get(ctx, &searchOutput)
			if err != nil {
				logger.Warn("Expansion search failed", "keyword", keyword, "error", err)
				continue
			}

			roundPapers = append(roundPapers, searchOutput.Papers...)
			totalPapersFound += len(searchOutput.Papers)
			progress.PapersFound = totalPapersFound
		}

		// Save expansion papers
		if len(roundPapers) > 0 {
			err = workflow.ExecuteActivity(statusCtx, "StatusActivities.SavePapers", activities.SavePapersInput{
				RequestID:           input.RequestID,
				OrgID:               input.OrgID,
				ProjectID:           input.ProjectID,
				Papers:              roundPapers,
				DiscoveredViaSource: domain.SourceTypeSemanticScholar,
				ExpansionDepth:      round,
			}).Get(ctx, nil)
			if err != nil {
				logger.Warn("Failed to save expansion papers", "error", err)
			}
		}

		allPapers = append(allPapers, roundPapers...)
	}

	// --- Phase 4: Complete ---
	progress.Phase = "completed"
	if err := updateStatus(domain.ReviewStatusCompleted); err != nil {
		return handleFailure("update_status_completed", err)
	}

	duration := workflow.Now(ctx).Sub(startTime)

	result := &ReviewWorkflowResult{
		RequestID:       input.RequestID,
		Status:          string(domain.ReviewStatusCompleted),
		KeywordsFound:   len(allKeywords),
		PapersFound:     totalPapersFound,
		ExpansionRounds: progress.ExpansionRound,
		Duration:        duration.Seconds(),
	}

	logger.Info("Literature review workflow completed",
		"keywords_found", result.KeywordsFound,
		"papers_found", result.PapersFound,
		"expansion_rounds", result.ExpansionRounds,
		"duration_s", result.Duration,
	)

	return result, nil
}

// selectPapersForExpansion selects papers with abstracts for keyword expansion.
// Prioritizes papers with longer abstracts as they tend to yield better keywords.
func selectPapersForExpansion(papers []*domain.Paper, max int) []*domain.Paper {
	var withAbstract []*domain.Paper
	for _, p := range papers {
		if p.Abstract != "" {
			withAbstract = append(withAbstract, p)
		}
	}

	if len(withAbstract) <= max {
		return withAbstract
	}

	return withAbstract[:max]
}
```

**Step 2: Create `internal/temporal/workflows/review_workflow_test.go`**

Tests using Temporal's workflow test suite with mocked activities:
- Full workflow success (extract → search → save → complete)
- Workflow with expansion rounds
- Workflow cancellation via signal
- Workflow failure during keyword extraction
- Progress query returns current state

```go
package workflows

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
)

func TestLiteratureReviewWorkflow_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Mock UpdateStatus activity (called multiple times)
	env.OnActivity("StatusActivities.UpdateStatus", mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords activity
	env.OnActivity("LLMActivities.ExtractKeywords", mock.Anything, mock.MatchedBy(func(input activities.ExtractKeywordsInput) bool {
		return input.Mode == string(domain.ReviewStatusExtractingKeywords)
	})).Return(&activities.ExtractKeywordsOutput{
		Keywords:  []string{"CRISPR", "gene editing"},
		Model:     "gpt-4",
		InputTokens: 100,
		OutputTokens: 50,
	}, nil)

	// Mock SaveKeywords activity
	env.OnActivity("StatusActivities.SaveKeywords", mock.Anything, mock.Anything).Return(&activities.SaveKeywordsOutput{
		KeywordIDs: []uuid.UUID{uuid.New(), uuid.New()},
		NewCount:   2,
	}, nil)

	// Mock SearchPapers activity
	env.OnActivity("SearchActivities.SearchPapers", mock.Anything, mock.Anything).Return(&activities.SearchPapersOutput{
		Papers: []*domain.Paper{
			{Title: "Paper 1", CanonicalID: "doi:1", Abstract: "Abstract 1"},
			{Title: "Paper 2", CanonicalID: "doi:2", Abstract: "Abstract 2"},
		},
		TotalFound: 2,
		BySource:   map[domain.SourceType]int{domain.SourceTypeSemanticScholar: 2},
	}, nil)

	// Mock SavePapers activity
	env.OnActivity("StatusActivities.SavePapers", mock.Anything, mock.Anything).Return(&activities.SavePapersOutput{
		SavedCount: 2,
	}, nil)

	input := ReviewWorkflowInput{
		RequestID: uuid.New(),
		OrgID:     "org1",
		ProjectID: "proj1",
		UserID:    "user1",
		Query:     "CRISPR gene editing applications",
		Config: domain.ReviewConfiguration{
			MaxPapers:           100,
			MaxExpansionDepth:   0, // No expansion for this test
			MaxKeywordsPerRound: 10,
			Sources:             []domain.SourceType{domain.SourceTypeSemanticScholar},
		},
	}

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	assert.Equal(t, 2, result.KeywordsFound)
	assert.True(t, result.PapersFound > 0)
}

func TestLiteratureReviewWorkflow_ExtractionFailure(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Mock UpdateStatus
	env.OnActivity("StatusActivities.UpdateStatus", mock.Anything, mock.Anything).Return(nil)

	// Mock ExtractKeywords - fail
	env.OnActivity("LLMActivities.ExtractKeywords", mock.Anything, mock.Anything).
		Return(nil, assert.AnError)

	input := ReviewWorkflowInput{
		RequestID: uuid.New(),
		OrgID:     "org1",
		ProjectID: "proj1",
		Query:     "test query",
		Config:    domain.DefaultReviewConfiguration(),
	}

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)
	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	assert.Contains(t, env.GetWorkflowError().Error(), "extract_keywords")
}

func TestLiteratureReviewWorkflow_WithExpansion(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.OnActivity("StatusActivities.UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	env.OnActivity("StatusActivities.SaveKeywords", mock.Anything, mock.Anything).Return(&activities.SaveKeywordsOutput{
		KeywordIDs: []uuid.UUID{uuid.New()},
		NewCount:   1,
	}, nil)
	env.OnActivity("StatusActivities.SavePapers", mock.Anything, mock.Anything).Return(&activities.SavePapersOutput{
		SavedCount: 1,
	}, nil)

	// Initial keyword extraction
	env.OnActivity("LLMActivities.ExtractKeywords", mock.Anything, mock.MatchedBy(func(input activities.ExtractKeywordsInput) bool {
		return input.Mode == string(domain.ReviewStatusExtractingKeywords)
	})).Return(&activities.ExtractKeywordsOutput{
		Keywords: []string{"CRISPR"},
		Model:    "gpt-4",
	}, nil).Once()

	// Expansion keyword extraction (from abstracts)
	env.OnActivity("LLMActivities.ExtractKeywords", mock.Anything, mock.MatchedBy(func(input activities.ExtractKeywordsInput) bool {
		return input.Mode == "abstract"
	})).Return(&activities.ExtractKeywordsOutput{
		Keywords: []string{"Cas9 protein"},
		Model:    "gpt-4",
	}, nil)

	// Search returns papers with abstracts
	env.OnActivity("SearchActivities.SearchPapers", mock.Anything, mock.Anything).Return(&activities.SearchPapersOutput{
		Papers: []*domain.Paper{
			{Title: "P1", CanonicalID: "doi:1", Abstract: "About CRISPR Cas9 protein..."},
		},
		TotalFound: 1,
	}, nil)

	input := ReviewWorkflowInput{
		RequestID: uuid.New(),
		OrgID:     "org1",
		ProjectID: "proj1",
		Query:     "CRISPR",
		Config: domain.ReviewConfiguration{
			MaxPapers:           100,
			MaxExpansionDepth:   1,
			MaxKeywordsPerRound: 5,
			Sources:             []domain.SourceType{domain.SourceTypeSemanticScholar},
		},
	}

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result ReviewWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, string(domain.ReviewStatusCompleted), result.Status)
	assert.True(t, result.KeywordsFound >= 2) // Initial + expansion
}

func TestLiteratureReviewWorkflow_ProgressQuery(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.OnActivity("StatusActivities.UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	env.OnActivity("LLMActivities.ExtractKeywords", mock.Anything, mock.Anything).Return(&activities.ExtractKeywordsOutput{
		Keywords: []string{"test"},
		Model:    "gpt-4",
	}, nil)
	env.OnActivity("StatusActivities.SaveKeywords", mock.Anything, mock.Anything).Return(&activities.SaveKeywordsOutput{
		KeywordIDs: []uuid.UUID{uuid.New()},
		NewCount:   1,
	}, nil)
	env.OnActivity("SearchActivities.SearchPapers", mock.Anything, mock.Anything).Return(&activities.SearchPapersOutput{
		Papers: []*domain.Paper{{Title: "P1", CanonicalID: "doi:1"}},
		TotalFound: 1,
	}, nil)
	env.OnActivity("StatusActivities.SavePapers", mock.Anything, mock.Anything).Return(&activities.SavePapersOutput{
		SavedCount: 1,
	}, nil)

	// Register query handler check - Temporal test env supports this via env.QueryWorkflow
	env.RegisterDelayedCallback(func() {
		result, err := env.QueryWorkflow(QueryProgress)
		require.NoError(t, err)
		var progress workflowProgress
		require.NoError(t, result.Get(&progress))
		// Progress should be tracked
		assert.NotEmpty(t, progress.Status)
	}, 0) // Fires immediately when workflow yields

	input := ReviewWorkflowInput{
		RequestID: uuid.New(),
		OrgID:     "org1",
		ProjectID: "proj1",
		Query:     "test",
		Config: domain.ReviewConfiguration{
			MaxPapers:         10,
			MaxExpansionDepth: 0,
			Sources:           []domain.SourceType{domain.SourceTypeSemanticScholar},
		},
	}

	env.ExecuteWorkflow(LiteratureReviewWorkflow, input)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}
```

**Step 3: Update `internal/temporal/worker.go` - update ActivityDependencies**

Update `ActivityDependencies` to use typed activity structs:

Replace:
```go
type ActivityDependencies struct {
	LLMActivities interface{}
	SearchActivities interface{}
	StatusActivities interface{}
	IngestionActivities interface{}
}
```

With:
```go
type ActivityDependencies struct {
	LLMActivities       *activities.LLMActivities
	SearchActivities     *activities.SearchActivities
	StatusActivities     *activities.StatusActivities
	IngestionActivities  interface{} // Placeholder until Phase 3E
}
```

And add the import for `activities` package.

**Step 4: Run tests**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test ./internal/temporal/... -v -race -count=1`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add internal/temporal/workflows/ internal/temporal/worker.go
git commit -m "feat(temporal): add LiteratureReviewWorkflow with expansion, signals, and queries"
```

---

## Task 5: Run Full Test Suite and Verify

**Files:**
- All files in `internal/temporal/activities/` and `internal/temporal/workflows/`

**Step 1: Run full test suite with race detector**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test ./... -race -count=1 2>&1 | tail -30`
Expected: All packages PASS

**Step 2: Run vet and verify no issues**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go vet ./...`
Expected: No issues

**Step 3: Verify build**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go build ./...`
Expected: Build succeeds

**Step 4: Commit (if any fixes)**

```bash
git add -A
git commit -m "fix(temporal): address test/vet issues"
```
