// Package qdrant provides a vector store client for Qdrant, used to store and
// search paper embeddings for semantic deduplication and similarity search.
package qdrant

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	pb "github.com/qdrant/go-client/qdrant"
)

// Config holds the configuration for connecting to a Qdrant instance.
type Config struct {
	// Address is the host:port of the Qdrant gRPC endpoint (e.g. "localhost:6334").
	Address string
	// CollectionName is the Qdrant collection to use (e.g. "paper_embeddings").
	CollectionName string
	// VectorSize is the dimensionality of the embedding vectors (e.g. 1536 for text-embedding-3-small).
	VectorSize uint64
}

// Validate checks that all required Config fields are set.
func (c Config) Validate() error {
	if c.Address == "" {
		return fmt.Errorf("qdrant config: address is required")
	}
	if c.CollectionName == "" {
		return fmt.Errorf("qdrant config: collection name is required")
	}
	if c.VectorSize == 0 {
		return fmt.Errorf("qdrant config: vector size must be > 0")
	}
	return nil
}

// PaperPoint represents a paper's embedding vector to be stored in Qdrant.
type PaperPoint struct {
	// PaperID is the unique identifier of the paper, used as the Qdrant point ID.
	PaperID uuid.UUID
	// Embedding is the dense vector representation of the paper.
	Embedding []float32
}

// SearchResult represents a single result from a vector similarity search.
type SearchResult struct {
	// PaperID is the unique identifier of the matched paper.
	PaperID uuid.UUID
	// Score is the cosine similarity score (higher is more similar).
	Score float32
}

// VectorStore defines the interface for vector storage and retrieval operations.
type VectorStore interface {
	// EnsureCollection creates the collection if it does not already exist.
	EnsureCollection(ctx context.Context) error
	// Upsert inserts or updates a single paper embedding in the collection.
	Upsert(ctx context.Context, point PaperPoint) error
	// Search finds the topK most similar vectors and returns their paper IDs and scores.
	Search(ctx context.Context, vector []float32, topK uint64) ([]SearchResult, error)
	// Close releases the underlying gRPC connection.
	Close() error
}

// Compile-time check that Client implements VectorStore.
var _ VectorStore = (*Client)(nil)

// Client is a Qdrant vector store client that implements VectorStore via gRPC.
type Client struct {
	client         *pb.Client
	collectionName string
	vectorSize     uint64
}

// NewClient creates a new Qdrant client by dialing the configured gRPC address.
// The connection uses insecure credentials, suitable for internal network deployments.
func NewClient(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Parse host and port from the address.
	host, port, err := parseAddress(cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("qdrant: invalid address %q: %w", cfg.Address, err)
	}

	qdrantClient, err := pb.NewClient(&pb.Config{
		Host: host,
		Port: port,
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to create client: %w", err)
	}

	return &Client{
		client:         qdrantClient,
		collectionName: cfg.CollectionName,
		vectorSize:     cfg.VectorSize,
	}, nil
}

// EnsureCollection checks whether the configured collection exists and creates it
// with cosine distance if it does not.
func (c *Client) EnsureCollection(ctx context.Context) error {
	exists, err := c.client.CollectionExists(ctx, c.collectionName)
	if err != nil {
		return fmt.Errorf("qdrant: failed to check collection existence: %w", err)
	}
	if exists {
		return nil
	}

	err = c.client.CreateCollection(ctx, &pb.CreateCollection{
		CollectionName: c.collectionName,
		VectorsConfig: pb.NewVectorsConfig(&pb.VectorParams{
			Size:     c.vectorSize,
			Distance: pb.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("qdrant: failed to create collection %q: %w", c.collectionName, err)
	}

	return nil
}

// Upsert inserts or updates a single paper embedding point. The paper's UUID is
// used as the Qdrant point ID, enabling idempotent upserts.
func (c *Client) Upsert(ctx context.Context, point PaperPoint) error {
	wait := true
	_, err := c.client.Upsert(ctx, &pb.UpsertPoints{
		CollectionName: c.collectionName,
		Wait:           &wait,
		Points: []*pb.PointStruct{
			{
				Id:      pb.NewIDUUID(point.PaperID.String()),
				Vectors: pb.NewVectors(point.Embedding...),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("qdrant: failed to upsert point %s: %w", point.PaperID, err)
	}

	return nil
}

// Search performs a nearest-neighbor vector search returning up to topK results
// ordered by cosine similarity (descending).
func (c *Client) Search(ctx context.Context, vector []float32, topK uint64) ([]SearchResult, error) {
	scored, err := c.client.Query(ctx, &pb.QueryPoints{
		CollectionName: c.collectionName,
		Query:          pb.NewQueryDense(vector),
		Limit:          &topK,
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: search failed: %w", err)
	}

	results := make([]SearchResult, 0, len(scored))
	for _, sp := range scored {
		if sp.Id == nil {
			continue
		}
		uuidStr := sp.Id.GetUuid()
		if uuidStr == "" {
			continue
		}
		paperID, err := uuid.Parse(uuidStr)
		if err != nil {
			return nil, fmt.Errorf("qdrant: invalid UUID in search result %q: %w", uuidStr, err)
		}
		results = append(results, SearchResult{
			PaperID: paperID,
			Score:   sp.Score,
		})
	}

	return results, nil
}

// Close releases the gRPC connection to Qdrant.
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// parseAddress splits an address string of the form "host:port" into its components.
func parseAddress(addr string) (string, int, error) {
	// Use net to parse host:port.
	host, portStr, err := splitHostPort(addr)
	if err != nil {
		return "", 0, err
	}

	port, err := parsePort(portStr)
	if err != nil {
		return "", 0, err
	}

	return host, port, nil
}

// splitHostPort splits an address into host and port strings.
// It handles the common case of "host:port" without importing net
// to avoid unnecessary dependencies for a simple split.
func splitHostPort(addr string) (string, string, error) {
	// Find last colon (handles IPv6 addresses in brackets).
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("missing port in address %q", addr)
}

// parsePort converts a port string to an integer.
func parsePort(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty port")
	}
	var port int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid port %q", s)
		}
		port = port*10 + int(c-'0')
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port %d out of range", port)
	}
	return port, nil
}
