package dedup

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/helixir/literature-review-service/internal/domain"
)

// TextEmbedder embeds a single text string into a vector representation.
type TextEmbedder interface {
	EmbedText(ctx context.Context, text string) ([]float32, error)
}

// SimilarPaper is a search result from the vector store, enriched with author data.
type SimilarPaper struct {
	PaperID uuid.UUID
	Score   float32
	Authors []domain.Author
}

// PaperEmbedding is a point to store in the vector store.
type PaperEmbedding struct {
	PaperID   uuid.UUID
	Embedding []float32
}

// VectorSearcher defines the vector store operations needed by the checker.
type VectorSearcher interface {
	Search(ctx context.Context, vector []float32, topK uint64) ([]SimilarPaper, error)
	Upsert(ctx context.Context, pe PaperEmbedding) error
}

// CheckerConfig holds the configuration for the duplicate checker.
type CheckerConfig struct {
	// SimilarityThreshold is the cosine similarity threshold above which
	// two papers are considered potential duplicates (e.g. 0.95).
	SimilarityThreshold float64

	// AuthorThreshold is the author overlap threshold above which
	// two papers with similar embeddings are considered duplicates (e.g. 0.5).
	AuthorThreshold float64

	// TopK is the number of nearest-neighbor candidates to retrieve from the
	// vector store for each check.
	TopK uint64
}

// CheckResult contains the result of a duplicate check for a single paper.
type CheckResult struct {
	// IsDuplicate indicates whether the paper is a duplicate of an existing paper.
	IsDuplicate bool

	// DuplicateOf is the ID of the existing paper if duplicate. Zero value if not.
	DuplicateOf uuid.UUID

	// Score is the cosine similarity score of the match. Zero if not a duplicate.
	Score float32
}

// Checker performs semantic deduplication of papers by comparing embeddings of
// title+abstract text and verifying author overlap for candidates above a
// similarity threshold.
type Checker struct {
	store    VectorSearcher
	embedder TextEmbedder
	cfg      CheckerConfig
}

// NewChecker creates a new Checker with the given dependencies and configuration.
func NewChecker(store VectorSearcher, embedder TextEmbedder, cfg CheckerConfig) *Checker {
	return &Checker{
		store:    store,
		embedder: embedder,
		cfg:      cfg,
	}
}

// Check determines whether the given paper is a duplicate of an existing paper
// in the vector store.
//
// The method:
//  1. Returns not-duplicate immediately if the paper has no abstract (nothing to embed).
//  2. Embeds the concatenation of title and abstract via the TextEmbedder.
//  3. Queries the vector store for the top-K most similar papers.
//  4. Always upserts the new embedding into the vector store.
//  5. For each candidate above the similarity threshold (skipping the paper itself),
//     computes author overlap. If overlap exceeds the author threshold, the paper
//     is declared a duplicate.
//  6. Returns not-duplicate if no candidate passes both thresholds.
func (c *Checker) Check(ctx context.Context, paper *domain.Paper) (*CheckResult, error) {
	// Papers without an abstract cannot be meaningfully embedded.
	if paper.Abstract == "" {
		return &CheckResult{}, nil
	}

	// Embed the combination of title and abstract.
	text := paper.Title + "\n" + paper.Abstract
	embedding, err := c.embedder.EmbedText(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("embedding text for paper %s: %w", paper.ID, err)
	}

	// Query vector store for similar papers.
	candidates, err := c.store.Search(ctx, embedding, c.cfg.TopK)
	if err != nil {
		return nil, fmt.Errorf("searching vector store for paper %s: %w", paper.ID, err)
	}

	// Always upsert the embedding into the vector store.
	if err := c.store.Upsert(ctx, PaperEmbedding{
		PaperID:   paper.ID,
		Embedding: embedding,
	}); err != nil {
		return nil, fmt.Errorf("upserting embedding for paper %s: %w", paper.ID, err)
	}

	// Evaluate candidates against thresholds.
	for _, candidate := range candidates {
		// Skip self-matches.
		if candidate.PaperID == paper.ID {
			continue
		}

		// Skip candidates below the similarity threshold.
		if float64(candidate.Score) < c.cfg.SimilarityThreshold {
			continue
		}

		// Check author overlap for candidates above the similarity threshold.
		overlap := AuthorOverlap(paper.Authors, candidate.Authors)
		if overlap >= c.cfg.AuthorThreshold {
			return &CheckResult{
				IsDuplicate: true,
				DuplicateOf: candidate.PaperID,
				Score:       candidate.Score,
			}, nil
		}
	}

	return &CheckResult{}, nil
}

// CheckWithEmbedding determines whether a paper with a pre-computed embedding
// is a duplicate of an existing paper in the vector store.
//
// This method is optimized for use in the concurrent pipeline where embeddings
// have already been generated. Unlike Check(), this method:
//   - Does NOT verify author overlap (not available with pre-computed embeddings).
//   - Uses only the similarity threshold to determine duplicates.
//   - Always upserts the embedding into the vector store.
//
// The method returns true if a duplicate is found (similarity above threshold).
func (c *Checker) CheckWithEmbedding(ctx context.Context, paperID uuid.UUID, embedding []float32) (bool, error) {
	if len(embedding) == 0 {
		return false, nil
	}

	// Query vector store for similar papers.
	candidates, err := c.store.Search(ctx, embedding, c.cfg.TopK)
	if err != nil {
		return false, fmt.Errorf("searching vector store for paper %s: %w", paperID, err)
	}

	// Always upsert the embedding into the vector store.
	if err := c.store.Upsert(ctx, PaperEmbedding{
		PaperID:   paperID,
		Embedding: embedding,
	}); err != nil {
		return false, fmt.Errorf("upserting embedding for paper %s: %w", paperID, err)
	}

	// Evaluate candidates against similarity threshold only (no author overlap check).
	for _, candidate := range candidates {
		// Skip self-matches.
		if candidate.PaperID == paperID {
			continue
		}

		// Check if candidate exceeds similarity threshold.
		if float64(candidate.Score) >= c.cfg.SimilarityThreshold {
			return true, nil
		}
	}

	return false, nil
}
