package dedup

import (
	"context"
	"fmt"

	llm "github.com/helixir/llm"
)

// Compile-time check that EmbedderAdapter implements TextEmbedder.
var _ TextEmbedder = (*EmbedderAdapter)(nil)

// EmbedderAdapter adapts llm.Embedder to the TextEmbedder interface used by the
// dedup checker. It converts a single text string into an embedding by
// delegating to the underlying llm.Embedder with a single-item input slice.
type EmbedderAdapter struct {
	embedder llm.Embedder
}

// NewEmbedderAdapter creates a new EmbedderAdapter wrapping the given llm.Embedder.
func NewEmbedderAdapter(e llm.Embedder) *EmbedderAdapter {
	return &EmbedderAdapter{embedder: e}
}

// EmbedText embeds a single text string by calling the underlying Embedder with
// a single-element input slice and returning the first (and only) embedding vector.
func (a *EmbedderAdapter) EmbedText(ctx context.Context, text string) ([]float32, error) {
	resp, err := a.embedder.Embed(ctx, llm.EmbedRequest{
		Input: []string{text},
	})
	if err != nil {
		return nil, fmt.Errorf("embedder adapter: %w", err)
	}

	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("embedder adapter: no embeddings returned for input")
	}

	return resp.Embeddings[0], nil
}
