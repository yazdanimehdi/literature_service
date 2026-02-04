package ingestion

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashFromURL(t *testing.T) {
	hash := hashFromURL("https://example.com/paper.pdf")
	assert.Len(t, hash, 64, "hash should be 64 characters")
}

func TestHashFromURL_DifferentURLs(t *testing.T) {
	hash1 := hashFromURL("https://example.com/paper1.pdf")
	hash2 := hashFromURL("https://example.com/paper2.pdf")
	assert.NotEqual(t, hash1, hash2, "different URLs should produce different hashes")
}

func TestNewClient_EmptyAddress(t *testing.T) {
	_, err := NewClient(Config{Address: ""})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "address is required")
}
