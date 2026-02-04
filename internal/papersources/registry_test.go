package papersources

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/domain"
)

// mockPaperSource is a mock implementation of PaperSource for testing.
type mockPaperSource struct {
	sourceType domain.SourceType
	name       string
	enabled    bool

	// searchFunc allows customizing search behavior in tests
	searchFunc func(ctx context.Context, params SearchParams) (*SearchResult, error)

	// getByIDFunc allows customizing GetByID behavior in tests
	getByIDFunc func(ctx context.Context, id string) (*domain.Paper, error)

	// Track calls for verification
	searchCalls atomic.Int32
}

func newMockPaperSource(sourceType domain.SourceType, name string, enabled bool) *mockPaperSource {
	return &mockPaperSource{
		sourceType: sourceType,
		name:       name,
		enabled:    enabled,
	}
}

func (m *mockPaperSource) Search(ctx context.Context, params SearchParams) (*SearchResult, error) {
	m.searchCalls.Add(1)
	if m.searchFunc != nil {
		return m.searchFunc(ctx, params)
	}
	// Default behavior: return empty result
	return &SearchResult{
		Papers:       []*domain.Paper{},
		TotalResults: 0,
		HasMore:      false,
		Source:       m.sourceType,
	}, nil
}

func (m *mockPaperSource) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, domain.ErrNotFound
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

func (m *mockPaperSource) SearchCallCount() int {
	return int(m.searchCalls.Load())
}

func TestNewRegistry(t *testing.T) {
	t.Run("creates empty registry", func(t *testing.T) {
		registry := NewRegistry()

		require.NotNil(t, registry)
		require.NotNil(t, registry.sources)
		assert.Empty(t, registry.sources)
	})

	t.Run("registry is ready to use", func(t *testing.T) {
		registry := NewRegistry()

		// Should be able to get sources (returns nil for non-existent)
		source := registry.Get(domain.SourceTypeSemanticScholar)
		assert.Nil(t, source)

		// Should be able to list sources (returns empty)
		sources := registry.AllSources()
		assert.Empty(t, sources)
	})
}

func TestRegistry_Register(t *testing.T) {
	t.Run("registers single source", func(t *testing.T) {
		registry := NewRegistry()
		source := newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true)

		registry.Register(source)

		retrieved := registry.Get(domain.SourceTypeSemanticScholar)
		require.NotNil(t, retrieved)
		assert.Equal(t, source, retrieved)
	})

	t.Run("registers multiple sources", func(t *testing.T) {
		registry := NewRegistry()

		sources := []*mockPaperSource{
			newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true),
			newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", true),
			newMockPaperSource(domain.SourceTypePubMed, "PubMed", true),
		}

		for _, s := range sources {
			registry.Register(s)
		}

		assert.Len(t, registry.AllSources(), 3)
		for _, s := range sources {
			retrieved := registry.Get(s.SourceType())
			require.NotNil(t, retrieved)
			assert.Equal(t, s, retrieved)
		}
	})

	t.Run("replaces existing source with same type", func(t *testing.T) {
		registry := NewRegistry()

		original := newMockPaperSource(domain.SourceTypeSemanticScholar, "Original", true)
		replacement := newMockPaperSource(domain.SourceTypeSemanticScholar, "Replacement", true)

		registry.Register(original)
		registry.Register(replacement)

		retrieved := registry.Get(domain.SourceTypeSemanticScholar)
		require.NotNil(t, retrieved)
		assert.Equal(t, "Replacement", retrieved.Name())
		assert.Len(t, registry.AllSources(), 1)
	})

	t.Run("concurrent registration is safe", func(t *testing.T) {
		registry := NewRegistry()
		var wg sync.WaitGroup

		sourceTypes := []domain.SourceType{
			domain.SourceTypeSemanticScholar,
			domain.SourceTypeOpenAlex,
			domain.SourceTypePubMed,
			domain.SourceTypeBioRxiv,
			domain.SourceTypeArXiv,
			domain.SourceTypeScopus,
		}

		// Register sources concurrently
		for i := 0; i < 10; i++ {
			for _, st := range sourceTypes {
				wg.Add(1)
				go func(sourceType domain.SourceType, iteration int) {
					defer wg.Done()
					source := newMockPaperSource(sourceType, string(sourceType)+"_"+string(rune('0'+iteration)), true)
					registry.Register(source)
				}(st, i)
			}
		}

		wg.Wait()

		// Should have exactly 6 sources (one per type)
		assert.Len(t, registry.AllSources(), 6)
	})
}

func TestRegistry_Get(t *testing.T) {
	t.Run("returns source when found", func(t *testing.T) {
		registry := NewRegistry()
		source := newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true)
		registry.Register(source)

		retrieved := registry.Get(domain.SourceTypeSemanticScholar)

		require.NotNil(t, retrieved)
		assert.Equal(t, domain.SourceTypeSemanticScholar, retrieved.SourceType())
		assert.Equal(t, "Semantic Scholar", retrieved.Name())
	})

	t.Run("returns nil when not found", func(t *testing.T) {
		registry := NewRegistry()
		// Register a different source
		source := newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", true)
		registry.Register(source)

		retrieved := registry.Get(domain.SourceTypeSemanticScholar)

		assert.Nil(t, retrieved)
	})

	t.Run("returns nil for empty registry", func(t *testing.T) {
		registry := NewRegistry()

		retrieved := registry.Get(domain.SourceTypeSemanticScholar)

		assert.Nil(t, retrieved)
	})

	t.Run("concurrent get is safe", func(t *testing.T) {
		registry := NewRegistry()
		source := newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true)
		registry.Register(source)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				retrieved := registry.Get(domain.SourceTypeSemanticScholar)
				assert.NotNil(t, retrieved)
			}()
		}
		wg.Wait()
	})
}

func TestRegistry_AllSources(t *testing.T) {
	t.Run("returns empty slice for empty registry", func(t *testing.T) {
		registry := NewRegistry()

		sources := registry.AllSources()

		assert.NotNil(t, sources)
		assert.Empty(t, sources)
	})

	t.Run("returns all registered sources", func(t *testing.T) {
		registry := NewRegistry()

		mockSources := []*mockPaperSource{
			newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true),
			newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", false),
			newMockPaperSource(domain.SourceTypePubMed, "PubMed", true),
		}

		for _, s := range mockSources {
			registry.Register(s)
		}

		sources := registry.AllSources()

		assert.Len(t, sources, 3)

		// Verify all sources are present (order may vary)
		sourceTypes := make(map[domain.SourceType]bool)
		for _, s := range sources {
			sourceTypes[s.SourceType()] = true
		}
		assert.True(t, sourceTypes[domain.SourceTypeSemanticScholar])
		assert.True(t, sourceTypes[domain.SourceTypeOpenAlex])
		assert.True(t, sourceTypes[domain.SourceTypePubMed])
	})

	t.Run("returns snapshot independent of registry modifications", func(t *testing.T) {
		registry := NewRegistry()
		source := newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true)
		registry.Register(source)

		// Get snapshot
		sources := registry.AllSources()
		assert.Len(t, sources, 1)

		// Add another source
		registry.Register(newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", true))

		// Original snapshot should be unchanged
		assert.Len(t, sources, 1)

		// New call should show updated count
		assert.Len(t, registry.AllSources(), 2)
	})
}

func TestRegistry_EnabledSources(t *testing.T) {
	t.Run("returns empty slice for empty registry", func(t *testing.T) {
		registry := NewRegistry()

		sources := registry.EnabledSources()

		assert.NotNil(t, sources)
		assert.Empty(t, sources)
	})

	t.Run("returns only enabled sources", func(t *testing.T) {
		registry := NewRegistry()

		// Register mix of enabled and disabled sources
		registry.Register(newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true))
		registry.Register(newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", false))
		registry.Register(newMockPaperSource(domain.SourceTypePubMed, "PubMed", true))
		registry.Register(newMockPaperSource(domain.SourceTypeBioRxiv, "bioRxiv", false))
		registry.Register(newMockPaperSource(domain.SourceTypeArXiv, "arXiv", true))

		sources := registry.EnabledSources()

		assert.Len(t, sources, 3)

		// Verify only enabled sources are present
		for _, s := range sources {
			assert.True(t, s.IsEnabled(), "source %s should be enabled", s.Name())
		}

		// Verify specific enabled sources are present
		sourceTypes := make(map[domain.SourceType]bool)
		for _, s := range sources {
			sourceTypes[s.SourceType()] = true
		}
		assert.True(t, sourceTypes[domain.SourceTypeSemanticScholar])
		assert.True(t, sourceTypes[domain.SourceTypePubMed])
		assert.True(t, sourceTypes[domain.SourceTypeArXiv])
		assert.False(t, sourceTypes[domain.SourceTypeOpenAlex])
		assert.False(t, sourceTypes[domain.SourceTypeBioRxiv])
	})

	t.Run("returns empty when all sources disabled", func(t *testing.T) {
		registry := NewRegistry()

		registry.Register(newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", false))
		registry.Register(newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", false))

		sources := registry.EnabledSources()

		assert.Empty(t, sources)
	})

	t.Run("filters disabled sources correctly", func(t *testing.T) {
		registry := NewRegistry()

		// Only disabled sources
		registry.Register(newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", false))
		registry.Register(newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", false))

		enabled := registry.EnabledSources()
		all := registry.AllSources()

		assert.Empty(t, enabled)
		assert.Len(t, all, 2)
	})
}

func TestRegistry_SearchAll(t *testing.T) {
	t.Run("searches all enabled sources concurrently", func(t *testing.T) {
		registry := NewRegistry()

		// Create sources with tracking
		sources := []*mockPaperSource{
			newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true),
			newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", true),
			newMockPaperSource(domain.SourceTypePubMed, "PubMed", true),
		}

		for _, s := range sources {
			s.searchFunc = func(ctx context.Context, params SearchParams) (*SearchResult, error) {
				return &SearchResult{
					Papers:       []*domain.Paper{{Title: "Test Paper"}},
					TotalResults: 1,
					Source:       s.sourceType,
				}, nil
			}
			registry.Register(s)
		}

		results := registry.SearchAll(context.Background(), SearchParams{Query: "test"})

		assert.Len(t, results, 3)

		// Verify each enabled source was searched
		for _, s := range sources {
			assert.Equal(t, 1, s.SearchCallCount(), "source %s should be searched once", s.Name())
		}
	})

	t.Run("skips disabled sources", func(t *testing.T) {
		registry := NewRegistry()

		enabled := newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true)
		disabled := newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", false)

		registry.Register(enabled)
		registry.Register(disabled)

		results := registry.SearchAll(context.Background(), SearchParams{Query: "test"})

		assert.Len(t, results, 1)
		assert.Equal(t, 1, enabled.SearchCallCount())
		assert.Equal(t, 0, disabled.SearchCallCount())
	})

	t.Run("returns empty results for empty registry", func(t *testing.T) {
		registry := NewRegistry()

		results := registry.SearchAll(context.Background(), SearchParams{Query: "test"})

		assert.Nil(t, results)
	})

	t.Run("includes error results without filtering", func(t *testing.T) {
		registry := NewRegistry()

		successSource := newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true)
		successSource.searchFunc = func(ctx context.Context, params SearchParams) (*SearchResult, error) {
			return &SearchResult{
				Papers:       []*domain.Paper{{Title: "Success Paper"}},
				TotalResults: 1,
				Source:       domain.SourceTypeSemanticScholar,
			}, nil
		}

		errorSource := newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", true)
		errorSource.searchFunc = func(ctx context.Context, params SearchParams) (*SearchResult, error) {
			return nil, errors.New("API error")
		}

		registry.Register(successSource)
		registry.Register(errorSource)

		results := registry.SearchAll(context.Background(), SearchParams{Query: "test"})

		assert.Len(t, results, 2)

		// Find results by source type
		var successResult, errorResult *SourceResult
		for i := range results {
			if results[i].Source == domain.SourceTypeSemanticScholar {
				successResult = &results[i]
			} else if results[i].Source == domain.SourceTypeOpenAlex {
				errorResult = &results[i]
			}
		}

		require.NotNil(t, successResult)
		require.NotNil(t, errorResult)

		assert.NoError(t, successResult.Error)
		assert.NotNil(t, successResult.Result)

		assert.Error(t, errorResult.Error)
		assert.Nil(t, errorResult.Result)
	})

	t.Run("searches are concurrent", func(t *testing.T) {
		registry := NewRegistry()

		// Track when searches start and finish
		var startTimes sync.Map
		var endTimes sync.Map

		for _, st := range []domain.SourceType{
			domain.SourceTypeSemanticScholar,
			domain.SourceTypeOpenAlex,
			domain.SourceTypePubMed,
		} {
			sourceType := st // Capture for closure
			source := newMockPaperSource(sourceType, string(sourceType), true)
			source.searchFunc = func(ctx context.Context, params SearchParams) (*SearchResult, error) {
				startTimes.Store(sourceType, time.Now())
				time.Sleep(50 * time.Millisecond)
				endTimes.Store(sourceType, time.Now())
				return &SearchResult{Source: sourceType}, nil
			}
			registry.Register(source)
		}

		start := time.Now()
		results := registry.SearchAll(context.Background(), SearchParams{Query: "test"})
		elapsed := time.Since(start)

		assert.Len(t, results, 3)

		// If concurrent, total time should be close to 50ms (single search duration)
		// If sequential, would be ~150ms
		assert.Less(t, elapsed, 150*time.Millisecond,
			"searches should run concurrently, took %v", elapsed)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		registry := NewRegistry()

		source := newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true)
		source.searchFunc = func(ctx context.Context, params SearchParams) (*SearchResult, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return &SearchResult{}, nil
			}
		}
		registry.Register(source)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		results := registry.SearchAll(ctx, SearchParams{Query: "test"})
		elapsed := time.Since(start)

		assert.Len(t, results, 1)
		assert.Error(t, results[0].Error)
		assert.Less(t, elapsed, 1*time.Second, "should respect context cancellation")
	})
}

func TestRegistry_SearchSources(t *testing.T) {
	t.Run("searches specific sources only", func(t *testing.T) {
		registry := NewRegistry()

		sources := []*mockPaperSource{
			newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true),
			newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", true),
			newMockPaperSource(domain.SourceTypePubMed, "PubMed", true),
		}

		for _, s := range sources {
			registry.Register(s)
		}

		// Search only two specific sources
		results := registry.SearchSources(
			context.Background(),
			SearchParams{Query: "test"},
			[]domain.SourceType{domain.SourceTypeSemanticScholar, domain.SourceTypePubMed},
		)

		assert.Len(t, results, 2)

		// Verify only requested sources were searched
		assert.Equal(t, 1, sources[0].SearchCallCount()) // Semantic Scholar
		assert.Equal(t, 0, sources[1].SearchCallCount()) // OpenAlex - not requested
		assert.Equal(t, 1, sources[2].SearchCallCount()) // PubMed
	})

	t.Run("falls back to all enabled when sourceTypes is nil", func(t *testing.T) {
		registry := NewRegistry()

		enabled := newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true)
		disabled := newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", false)

		registry.Register(enabled)
		registry.Register(disabled)

		results := registry.SearchSources(context.Background(), SearchParams{Query: "test"}, nil)

		assert.Len(t, results, 1)
		assert.Equal(t, 1, enabled.SearchCallCount())
		assert.Equal(t, 0, disabled.SearchCallCount())
	})

	t.Run("falls back to all enabled when sourceTypes is empty", func(t *testing.T) {
		registry := NewRegistry()

		enabled := newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true)
		registry.Register(enabled)

		results := registry.SearchSources(context.Background(), SearchParams{Query: "test"}, []domain.SourceType{})

		assert.Len(t, results, 1)
		assert.Equal(t, 1, enabled.SearchCallCount())
	})

	t.Run("skips non-existent source types", func(t *testing.T) {
		registry := NewRegistry()

		source := newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true)
		registry.Register(source)

		results := registry.SearchSources(
			context.Background(),
			SearchParams{Query: "test"},
			[]domain.SourceType{domain.SourceTypeSemanticScholar, domain.SourceTypeOpenAlex},
		)

		// Only the registered source should be searched
		assert.Len(t, results, 1)
		assert.Equal(t, domain.SourceTypeSemanticScholar, results[0].Source)
	})

	t.Run("returns nil when no matching sources", func(t *testing.T) {
		registry := NewRegistry()

		source := newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true)
		registry.Register(source)

		results := registry.SearchSources(
			context.Background(),
			SearchParams{Query: "test"},
			[]domain.SourceType{domain.SourceTypeOpenAlex},
		)

		assert.Nil(t, results)
	})

	t.Run("searches disabled sources when explicitly requested", func(t *testing.T) {
		registry := NewRegistry()

		disabled := newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", false)
		registry.Register(disabled)

		// When explicitly requesting a disabled source, it should be searched
		results := registry.SearchSources(
			context.Background(),
			SearchParams{Query: "test"},
			[]domain.SourceType{domain.SourceTypeOpenAlex},
		)

		assert.Len(t, results, 1)
		assert.Equal(t, 1, disabled.SearchCallCount())
	})

	t.Run("handles concurrent requests safely", func(t *testing.T) {
		registry := NewRegistry()

		sources := []*mockPaperSource{
			newMockPaperSource(domain.SourceTypeSemanticScholar, "Semantic Scholar", true),
			newMockPaperSource(domain.SourceTypeOpenAlex, "OpenAlex", true),
			newMockPaperSource(domain.SourceTypePubMed, "PubMed", true),
		}

		for _, s := range sources {
			registry.Register(s)
		}

		var wg sync.WaitGroup
		resultsChan := make(chan []SourceResult, 100)

		// Make many concurrent search requests
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results := registry.SearchSources(
					context.Background(),
					SearchParams{Query: "test"},
					[]domain.SourceType{domain.SourceTypeSemanticScholar, domain.SourceTypeOpenAlex},
				)
				resultsChan <- results
			}()
		}

		wg.Wait()
		close(resultsChan)

		// Verify all requests completed successfully
		count := 0
		for results := range resultsChan {
			assert.Len(t, results, 2)
			count++
		}
		assert.Equal(t, 100, count)
	})
}
