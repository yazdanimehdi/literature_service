package dedup

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/qdrant"
)

// Compile-time check that QdrantAdapter implements VectorSearcher.
var _ VectorSearcher = (*QdrantAdapter)(nil)

// PaperLookup fetches paper author data for dedup candidate enrichment.
type PaperLookup interface {
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.Paper, error)
}

// QdrantAdapter adapts the Qdrant VectorStore and a PaperLookup to the
// VectorSearcher interface used by the dedup checker. It bridges the gap
// between the low-level vector search results (which only contain IDs and
// scores) and the enriched SimilarPaper results that include author data.
type QdrantAdapter struct {
	store  qdrant.VectorStore
	lookup PaperLookup
}

// NewQdrantAdapter creates a new QdrantAdapter with the given vector store and
// paper lookup.
func NewQdrantAdapter(store qdrant.VectorStore, lookup PaperLookup) *QdrantAdapter {
	return &QdrantAdapter{
		store:  store,
		lookup: lookup,
	}
}

// Search queries the vector store for the topK most similar vectors, then
// enriches each result with author data from the paper lookup.
func (a *QdrantAdapter) Search(ctx context.Context, vector []float32, topK uint64) ([]SimilarPaper, error) {
	// Step 1: Query the vector store.
	results, err := a.store.Search(ctx, vector, topK)
	if err != nil {
		return nil, fmt.Errorf("qdrant adapter search: %w", err)
	}

	if len(results) == 0 {
		return []SimilarPaper{}, nil
	}

	// Step 2: Collect paper IDs from the search results.
	ids := make([]uuid.UUID, len(results))
	for i, r := range results {
		ids[i] = r.PaperID
	}

	// Step 3: Fetch papers with author data.
	papers, err := a.lookup.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("qdrant adapter lookup: %w", err)
	}

	// Step 4: Build a lookup map from paper ID to paper.
	paperMap := make(map[uuid.UUID]*domain.Paper, len(papers))
	for _, p := range papers {
		paperMap[p.ID] = p
	}

	// Step 5: Combine search results with author data.
	similar := make([]SimilarPaper, 0, len(results))
	for _, r := range results {
		sp := SimilarPaper{
			PaperID: r.PaperID,
			Score:   r.Score,
		}
		if paper, ok := paperMap[r.PaperID]; ok {
			sp.Authors = paper.Authors
		}
		similar = append(similar, sp)
	}

	return similar, nil
}

// Upsert stores a paper embedding in the vector store.
func (a *QdrantAdapter) Upsert(ctx context.Context, pe PaperEmbedding) error {
	point := qdrant.PaperPoint{
		PaperID:   pe.PaperID,
		Embedding: pe.Embedding,
	}
	if err := a.store.Upsert(ctx, point); err != nil {
		return fmt.Errorf("qdrant adapter upsert: %w", err)
	}
	return nil
}
