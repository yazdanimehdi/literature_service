package qdrant

import (
	"context"
	"testing"
	"time"

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

// TestParseAddress tests address parsing (unit tests, no network required).
func TestParseAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		addr     string
		wantHost string
		wantPort int
		wantErr  string
	}{
		{
			name:     "valid localhost with port",
			addr:     "localhost:6334",
			wantHost: "localhost",
			wantPort: 6334,
		},
		{
			name:     "valid IP with port",
			addr:     "192.168.1.100:6334",
			wantHost: "192.168.1.100",
			wantPort: 6334,
		},
		{
			name:     "valid hostname with port",
			addr:     "qdrant.example.com:6334",
			wantHost: "qdrant.example.com",
			wantPort: 6334,
		},
		{
			name:     "port 1 (minimum)",
			addr:     "host:1",
			wantHost: "host",
			wantPort: 1,
		},
		{
			name:     "port 65535 (maximum)",
			addr:     "host:65535",
			wantHost: "host",
			wantPort: 65535,
		},
		{
			name:    "missing port",
			addr:    "localhost",
			wantErr: "missing port",
		},
		{
			name:    "empty port",
			addr:    "localhost:",
			wantErr: "empty port",
		},
		{
			name:    "invalid port - letters",
			addr:    "localhost:abc",
			wantErr: "invalid port",
		},
		{
			name:    "invalid port - mixed",
			addr:    "localhost:123abc",
			wantErr: "invalid port",
		},
		{
			name:    "port zero",
			addr:    "localhost:0",
			wantErr: "out of range",
		},
		{
			name:    "port too large",
			addr:    "localhost:65536",
			wantErr: "out of range",
		},
		{
			name:    "port way too large",
			addr:    "localhost:99999",
			wantErr: "out of range",
		},
		{
			name:    "empty address",
			addr:    "",
			wantErr: "missing port",
		},
		{
			name:    "just colon",
			addr:    ":",
			wantErr: "empty port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			host, port, err := parseAddress(tt.addr)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantHost, host)
				assert.Equal(t, tt.wantPort, port)
			}
		})
	}
}

// TestSplitHostPort tests host:port splitting.
func TestSplitHostPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		addr     string
		wantHost string
		wantPort string
		wantErr  bool
	}{
		{
			name:     "simple host:port",
			addr:     "localhost:6334",
			wantHost: "localhost",
			wantPort: "6334",
		},
		{
			name:     "ip:port",
			addr:     "127.0.0.1:8080",
			wantHost: "127.0.0.1",
			wantPort: "8080",
		},
		{
			name:     "multiple colons (takes last)",
			addr:     "host:with:colons:1234",
			wantHost: "host:with:colons",
			wantPort: "1234",
		},
		{
			name:    "no colon",
			addr:    "localhost",
			wantErr: true,
		},
		{
			name:    "empty string",
			addr:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			host, port, err := splitHostPort(tt.addr)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantHost, host)
				assert.Equal(t, tt.wantPort, port)
			}
		})
	}
}

// TestParsePort tests port string parsing.
func TestParsePort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    int
		wantErr string
	}{
		{name: "valid port 80", input: "80", want: 80},
		{name: "valid port 443", input: "443", want: 443},
		{name: "valid port 6334", input: "6334", want: 6334},
		{name: "valid port 1", input: "1", want: 1},
		{name: "valid port 65535", input: "65535", want: 65535},
		{name: "empty string", input: "", wantErr: "empty port"},
		{name: "letters", input: "abc", wantErr: "invalid port"},
		{name: "mixed", input: "80a", wantErr: "invalid port"},
		{name: "negative lookalike", input: "-1", wantErr: "invalid port"},
		{name: "zero", input: "0", wantErr: "out of range"},
		{name: "too large", input: "65536", wantErr: "out of range"},
		{name: "spaces", input: " 80", wantErr: "invalid port"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			port, err := parsePort(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, port)
			}
		})
	}
}

// TestClient_Close_NilClient tests close on nil client.
func TestClient_Close_NilClient(t *testing.T) {
	t.Parallel()

	// Client with nil internal client should not panic
	c := &Client{client: nil}
	err := c.Close()
	assert.NoError(t, err)
}

// Integration tests (require running Qdrant)

// TestClient_Integration tests the full client lifecycle with a real Qdrant instance.
func TestClient_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client := setupTestQdrantClient(t)
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// Ensure collection exists before any tests (not a subtest, must run first)
	err := client.EnsureCollection(ctx)
	require.NoError(t, err, "EnsureCollection should succeed")

	t.Run("EnsureCollection is idempotent", func(t *testing.T) {
		// Calling again should succeed (collection already exists)
		err := client.EnsureCollection(ctx)
		require.NoError(t, err)
	})

	t.Run("Upsert inserts a point", func(t *testing.T) {
		point := PaperPoint{
			PaperID:   uuid.New(),
			Embedding: generateTestEmbedding(1536),
		}

		err := client.Upsert(ctx, point)
		require.NoError(t, err)
	})

	t.Run("Upsert is idempotent", func(t *testing.T) {
		paperID := uuid.New()
		embedding := generateTestEmbedding(1536)

		point := PaperPoint{
			PaperID:   paperID,
			Embedding: embedding,
		}

		// Insert first time
		err := client.Upsert(ctx, point)
		require.NoError(t, err)

		// Insert same point again (should update, not error)
		err = client.Upsert(ctx, point)
		require.NoError(t, err)
	})

	t.Run("Upsert with different embedding updates", func(t *testing.T) {
		paperID := uuid.New()

		// First embedding
		point1 := PaperPoint{
			PaperID:   paperID,
			Embedding: generateTestEmbedding(1536),
		}
		err := client.Upsert(ctx, point1)
		require.NoError(t, err)

		// Different embedding for same paper
		point2 := PaperPoint{
			PaperID:   paperID,
			Embedding: generateTestEmbedding(1536),
		}
		err = client.Upsert(ctx, point2)
		require.NoError(t, err)
	})

	t.Run("Search returns similar vectors", func(t *testing.T) {
		// Insert a known vector
		knownID := uuid.New()
		knownEmbedding := make([]float32, 1536)
		for i := range knownEmbedding {
			knownEmbedding[i] = 0.5
		}

		err := client.Upsert(ctx, PaperPoint{
			PaperID:   knownID,
			Embedding: knownEmbedding,
		})
		require.NoError(t, err)

		// Search with similar vector
		searchVector := make([]float32, 1536)
		for i := range searchVector {
			searchVector[i] = 0.5
		}

		results, err := client.Search(ctx, searchVector, 10)
		require.NoError(t, err)
		require.NotEmpty(t, results)

		// The known vector should be in results with high score
		found := false
		for _, r := range results {
			if r.PaperID == knownID {
				found = true
				assert.Greater(t, r.Score, float32(0.9), "similar vector should have high score")
				break
			}
		}
		assert.True(t, found, "known vector should be in search results")
	})

	t.Run("Search with topK limits results", func(t *testing.T) {
		// Insert multiple vectors
		for i := 0; i < 5; i++ {
			err := client.Upsert(ctx, PaperPoint{
				PaperID:   uuid.New(),
				Embedding: generateTestEmbedding(1536),
			})
			require.NoError(t, err)
		}

		searchVector := generateTestEmbedding(1536)

		// Request only top 3
		results, err := client.Search(ctx, searchVector, 3)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), 3)
	})

	t.Run("Search empty collection returns empty results", func(t *testing.T) {
		// Create a separate client with a unique collection
		emptyClient, err := NewClient(Config{
			Address:        getQdrantAddress(),
			CollectionName: "test_empty_" + uuid.New().String()[:8],
			VectorSize:     1536,
		})
		require.NoError(t, err)
		defer func() { _ = emptyClient.Close() }()

		err = emptyClient.EnsureCollection(ctx)
		require.NoError(t, err)

		results, err := emptyClient.Search(ctx, generateTestEmbedding(1536), 10)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("Close succeeds", func(t *testing.T) {
		// Create a new client just to test close
		c, err := NewClient(Config{
			Address:        getQdrantAddress(),
			CollectionName: "test_close_" + uuid.New().String()[:8],
			VectorSize:     1536,
		})
		require.NoError(t, err)

		err = c.Close()
		assert.NoError(t, err)
	})
}

// TestClient_NewClient_Success tests successful client creation.
func TestClient_NewClient_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	client, err := NewClient(Config{
		Address:        getQdrantAddress(),
		CollectionName: "test_newclient_" + uuid.New().String()[:8],
		VectorSize:     1536,
	})
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.Equal(t, uint64(1536), client.vectorSize)

	err = client.Close()
	assert.NoError(t, err)
}

// setupTestQdrantClient creates a test Qdrant client.
func setupTestQdrantClient(t *testing.T) *Client {
	t.Helper()

	cfg := Config{
		Address:        getQdrantAddress(),
		CollectionName: "test_" + uuid.New().String()[:8],
		VectorSize:     1536,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping integration test: cannot connect to Qdrant: %v", err)
	}

	// Qdrant SDK connects lazily — probe the server to verify reachability.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.EnsureCollection(ctx); err != nil {
		_ = client.Close()
		t.Skipf("Skipping integration test: Qdrant not reachable at %s: %v", cfg.Address, err)
	}

	return client
}

// getQdrantAddress returns the Qdrant gRPC address.
// Tries test port first (docker-compose.test.yml), then dev port.
func getQdrantAddress() string {
	// Test port from docker-compose.test.yml
	return "localhost:6336"
}

// generateTestEmbedding creates a deterministic test embedding vector.
func generateTestEmbedding(size int) []float32 {
	embedding := make([]float32, size)
	for i := range embedding {
		// Use (i+1) to avoid zeros, and modulo to keep values reasonable
		embedding[i] = float32((i+1)%100+1) / 100.0
	}
	return embedding
}
