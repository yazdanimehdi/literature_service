package papersources

import (
	"context"
	"sync"

	"github.com/helixir/literature-review-service/internal/domain"
)

// SourceResult holds the result of a search from one source.
type SourceResult struct {
	// Source identifies which paper source provided the result.
	Source domain.SourceType

	// Result contains the search results if the search succeeded.
	// Will be nil if Error is non-nil.
	Result *SearchResult

	// Error contains the error if the search failed.
	// Will be nil if Result is non-nil.
	Error error
}

// Registry manages paper sources and coordinates concurrent searches.
// It provides thread-safe registration and retrieval of paper sources,
// as well as concurrent search operations across multiple sources.
type Registry struct {
	mu      sync.RWMutex
	sources map[domain.SourceType]PaperSource
}

// NewRegistry creates a new source registry with an empty source map.
func NewRegistry() *Registry {
	return &Registry{
		sources: make(map[domain.SourceType]PaperSource),
	}
}

// Register adds a source to the registry.
// If a source with the same type already exists, it will be replaced.
// This method is thread-safe.
func (r *Registry) Register(source PaperSource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources[source.SourceType()] = source
}

// Get returns a source by type, or nil if not found.
// This method is thread-safe.
func (r *Registry) Get(sourceType domain.SourceType) PaperSource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sources[sourceType]
}

// AllSources returns all registered sources.
// The returned slice is a snapshot and is safe to iterate even if
// sources are added or removed concurrently.
// This method is thread-safe.
func (r *Registry) AllSources() []PaperSource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sources := make([]PaperSource, 0, len(r.sources))
	for _, source := range r.sources {
		sources = append(sources, source)
	}
	return sources
}

// EnabledSources returns only enabled sources.
// Sources are considered enabled if their IsEnabled() method returns true.
// The returned slice is a snapshot and is safe to iterate even if
// sources are added or removed concurrently.
// This method is thread-safe.
func (r *Registry) EnabledSources() []PaperSource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sources := make([]PaperSource, 0, len(r.sources))
	for _, source := range r.sources {
		if source.IsEnabled() {
			sources = append(sources, source)
		}
	}
	return sources
}

// SearchAll searches all enabled sources concurrently.
// Returns results for each source (including errors). Errors are not filtered;
// the caller is responsible for handling them appropriately.
// The search respects context cancellation - if the context is canceled,
// ongoing searches will be interrupted and their errors returned.
// This method is thread-safe.
func (r *Registry) SearchAll(ctx context.Context, params SearchParams) []SourceResult {
	return r.SearchSources(ctx, params, nil)
}

// SearchSources searches specific sources concurrently.
// If sourceTypes is nil or empty, searches all enabled sources.
// Returns results for each source (including errors). Errors are not filtered;
// the caller is responsible for handling them appropriately.
// If a requested source type is not found in the registry, it will be skipped.
// The search respects context cancellation - if the context is canceled,
// ongoing searches will be interrupted and their errors returned.
// This method is thread-safe.
func (r *Registry) SearchSources(ctx context.Context, params SearchParams, sourceTypes []domain.SourceType) []SourceResult {
	var sources []PaperSource

	if len(sourceTypes) == 0 {
		// Search all enabled sources
		sources = r.EnabledSources()
	} else {
		// Search specific sources
		r.mu.RLock()
		sources = make([]PaperSource, 0, len(sourceTypes))
		for _, st := range sourceTypes {
			if source, ok := r.sources[st]; ok {
				sources = append(sources, source)
			}
		}
		r.mu.RUnlock()
	}

	if len(sources) == 0 {
		return nil
	}

	// Create result channel and wait group
	resultChan := make(chan SourceResult, len(sources))
	var wg sync.WaitGroup

	// Launch goroutines for concurrent searches
	for _, source := range sources {
		wg.Add(1)
		go func(s PaperSource) {
			defer wg.Done()

			result, err := s.Search(ctx, params)
			resultChan <- SourceResult{
				Source: s.SourceType(),
				Result: result,
				Error:  err,
			}
		}(source)
	}

	// Wait for all searches to complete in a separate goroutine
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	results := make([]SourceResult, 0, len(sources))
	for result := range resultChan {
		results = append(results, result)
	}

	return results
}
