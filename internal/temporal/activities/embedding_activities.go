package activities

import (
	"context"
	"fmt"
	"math"

	"go.temporal.io/sdk/activity"
)

// Embedder defines the interface for generating embeddings.
type Embedder interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// EmbeddingActivities provides Temporal activities for paper embedding operations.
// Methods on this struct are registered as Temporal activities via the worker.
type EmbeddingActivities struct {
	embedder Embedder
}

// NewEmbeddingActivities creates a new EmbeddingActivities instance.
func NewEmbeddingActivities(embedder Embedder) *EmbeddingActivities {
	return &EmbeddingActivities{
		embedder: embedder,
	}
}

// EmbedPapers generates embeddings for a batch of paper abstracts.
// Papers without abstracts are skipped.
func (a *EmbeddingActivities) EmbedPapers(ctx context.Context, input EmbedPapersInput) (*EmbedPapersOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("starting batch embedding", "paperCount", len(input.Papers))

	if a.embedder == nil {
		return nil, fmt.Errorf("embedder is not configured")
	}

	output := &EmbedPapersOutput{
		Embeddings: make(map[string][]float32),
	}

	// Collect papers with abstracts.
	var textsToEmbed []string
	var canonicalIDs []string
	for i, paper := range input.Papers {
		if i > 0 && i%10 == 0 {
			activity.RecordHeartbeat(ctx, fmt.Sprintf("collecting %d/%d", i, len(input.Papers)))
		}
		if paper.Abstract == "" {
			output.Skipped++
			continue
		}
		textsToEmbed = append(textsToEmbed, paper.Abstract)
		canonicalIDs = append(canonicalIDs, paper.CanonicalID)
	}

	if len(textsToEmbed) == 0 {
		logger.Info("no papers with abstracts to embed")
		return output, nil
	}

	// Record heartbeat before potentially long embedding call.
	activity.RecordHeartbeat(ctx, fmt.Sprintf("embedding %d papers", len(textsToEmbed)))

	// Call embedder with batch.
	embeddings, err := a.embedder.EmbedBatch(ctx, textsToEmbed)
	if err != nil {
		logger.Error("batch embedding failed", "error", err)
		return nil, fmt.Errorf("embed batch: %w", err)
	}

	// Validate embedding count matches.
	if len(embeddings) != len(textsToEmbed) {
		logger.Error("embedder returned mismatched count",
			"expected", len(textsToEmbed),
			"got", len(embeddings),
		)
		return nil, fmt.Errorf("embedder returned %d embeddings for %d texts", len(embeddings), len(textsToEmbed))
	}

	// Map embeddings to canonical IDs.
	for i, embedding := range embeddings {
		output.Embeddings[canonicalIDs[i]] = embedding
	}

	logger.Info("batch embedding completed",
		"embedded", len(output.Embeddings),
		"skipped", output.Skipped,
	)

	return output, nil
}

// EmbedText generates an embedding for a single text string.
func (a *EmbeddingActivities) EmbedText(ctx context.Context, input EmbedTextInput) (*EmbedTextOutput, error) {
	if a.embedder == nil {
		return nil, fmt.Errorf("embedder is not configured")
	}
	if input.Text == "" {
		return nil, fmt.Errorf("text is required")
	}

	embeddings, err := a.embedder.EmbedBatch(ctx, []string{input.Text})
	if err != nil {
		return nil, fmt.Errorf("embed text: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("embedder returned no embeddings")
	}

	return &EmbedTextOutput{Embedding: embeddings[0]}, nil
}

// ScoreKeywordRelevance scores keywords by cosine similarity against a query embedding
// and partitions them into accepted (>= threshold) and rejected (< threshold).
func (a *EmbeddingActivities) ScoreKeywordRelevance(ctx context.Context, input ScoreKeywordRelevanceInput) (*ScoreKeywordRelevanceOutput, error) {
	output := &ScoreKeywordRelevanceOutput{}
	if len(input.Keywords) == 0 {
		return output, nil
	}
	if a.embedder == nil {
		return nil, fmt.Errorf("embedder is not configured")
	}

	embeddings, err := a.embedder.EmbedBatch(ctx, input.Keywords)
	if err != nil {
		return nil, fmt.Errorf("embed keywords: %w", err)
	}
	if len(embeddings) != len(input.Keywords) {
		return nil, fmt.Errorf("embedder returned %d embeddings for %d keywords", len(embeddings), len(input.Keywords))
	}

	for i, kw := range input.Keywords {
		score := cosineSimilarity(input.QueryEmbedding, embeddings[i])
		sk := ScoredKeyword{Keyword: kw, Score: score}
		if score >= input.Threshold {
			output.Accepted = append(output.Accepted, sk)
		} else {
			output.Rejected = append(output.Rejected, sk)
		}
	}
	return output, nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
// Returns 0 if either vector has zero magnitude.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	// Accumulate in float32 to avoid per-element float32→float64 conversions.
	// Only convert to float64 for the final division and sqrt.
	var dot, magA, magB float32
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return float64(dot) / math.Sqrt(float64(magA)*float64(magB))
}
