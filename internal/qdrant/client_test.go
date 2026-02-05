package qdrant

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaperPoint(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	embedding := []float32{0.1, 0.2, 0.3}

	point := PaperPoint{
		PaperID:   id,
		Embedding: embedding,
	}

	assert.Equal(t, id, point.PaperID)
	assert.Equal(t, embedding, point.Embedding)
}

func TestSearchResult(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	result := SearchResult{
		PaperID: id,
		Score:   0.95,
	}

	assert.Equal(t, id, result.PaperID)
	assert.InDelta(t, float32(0.95), result.Score, 0.001)
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "valid config",
			cfg: Config{
				Address:        "localhost:6334",
				CollectionName: "paper_embeddings",
				VectorSize:     1536,
			},
			wantErr: "",
		},
		{
			name: "empty address",
			cfg: Config{
				Address:        "",
				CollectionName: "paper_embeddings",
				VectorSize:     1536,
			},
			wantErr: "address is required",
		},
		{
			name: "empty collection name",
			cfg: Config{
				Address:        "localhost:6334",
				CollectionName: "",
				VectorSize:     1536,
			},
			wantErr: "collection name is required",
		},
		{
			name: "zero vector size",
			cfg: Config{
				Address:        "localhost:6334",
				CollectionName: "paper_embeddings",
				VectorSize:     0,
			},
			wantErr: "vector size must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNewClient_EmptyAddress(t *testing.T) {
	t.Parallel()

	_, err := NewClient(Config{
		Address:        "",
		CollectionName: "paper_embeddings",
		VectorSize:     1536,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "address is required")
}

func TestNewClient_InvalidAddress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test that attempts network dial in short mode")
	}
	t.Parallel()

	// This will attempt to connect to a non-existent server.
	// The Qdrant client may or may not fail at dial time depending on the library version,
	// but we verify it does not panic.
	_, err := NewClient(Config{
		Address:        "localhost:19999",
		CollectionName: "test",
		VectorSize:     1536,
	})
	// The client may succeed at creation (lazy connect) or fail.
	// We just verify no panic occurred.
	if err != nil {
		assert.Error(t, err)
	}
}

func TestVectorStoreInterface(t *testing.T) {
	t.Parallel()

	// Compile-time check is in client.go; this test verifies
	// the interface is importable and usable as a type.
	var _ VectorStore = (*Client)(nil)
}
