package ingestion

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingestionv1 "github.com/helixir/ingestion-service/api/gen/ingestion/v1"
)

func TestHashFromURL(t *testing.T) {
	t.Run("returns 64-char hex string", func(t *testing.T) {
		hash := hashFromURL("https://example.com/paper.pdf")
		assert.Len(t, hash, 64, "SHA-256 hex digest must be 64 characters")
	})

	t.Run("different URLs produce different hashes", func(t *testing.T) {
		hash1 := hashFromURL("https://example.com/paper1.pdf")
		hash2 := hashFromURL("https://example.com/paper2.pdf")
		assert.NotEqual(t, hash1, hash2, "different URLs should produce different hashes")
	})

	t.Run("same URL produces same hash (deterministic)", func(t *testing.T) {
		url := "https://example.com/stable.pdf"
		hash1 := hashFromURL(url)
		hash2 := hashFromURL(url)
		assert.Equal(t, hash1, hash2, "same URL must always produce the same hash")
	})

	t.Run("empty URL returns valid 64-char hex", func(t *testing.T) {
		hash := hashFromURL("")
		assert.Len(t, hash, 64, "even an empty URL should produce a 64-char hash")
		assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), hash, "hash must be lowercase hex")
	})

	t.Run("result is always lowercase hex", func(t *testing.T) {
		urls := []string{
			"https://example.com/a",
			"https://example.com/b",
			"ftp://files.org/doc.pdf",
			"",
			"   ",
		}
		hexPattern := regexp.MustCompile(`^[0-9a-f]{64}$`)
		for _, u := range urls {
			hash := hashFromURL(u)
			assert.Regexp(t, hexPattern, hash, "hash for %q must be 64 lowercase hex chars", u)
		}
	})
}

func TestIsTerminalStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   ingestionv1.RunStatus
		terminal bool
	}{
		// Terminal statuses
		{name: "COMPLETED is terminal", status: ingestionv1.RunStatus_RUN_STATUS_COMPLETED, terminal: true},
		{name: "PARTIAL is terminal", status: ingestionv1.RunStatus_RUN_STATUS_PARTIAL, terminal: true},
		{name: "FAILED is terminal", status: ingestionv1.RunStatus_RUN_STATUS_FAILED, terminal: true},
		{name: "CANCELLED is terminal", status: ingestionv1.RunStatus_RUN_STATUS_CANCELLED, terminal: true},
		{name: "TIMEOUT is terminal", status: ingestionv1.RunStatus_RUN_STATUS_TIMEOUT, terminal: true},
		// Non-terminal statuses
		{name: "UNSPECIFIED is not terminal", status: ingestionv1.RunStatus_RUN_STATUS_UNSPECIFIED, terminal: false},
		{name: "PENDING is not terminal", status: ingestionv1.RunStatus_RUN_STATUS_PENDING, terminal: false},
		{name: "PLANNING is not terminal", status: ingestionv1.RunStatus_RUN_STATUS_PLANNING, terminal: false},
		{name: "EXECUTING is not terminal", status: ingestionv1.RunStatus_RUN_STATUS_EXECUTING, terminal: false},
		{name: "VALIDATING is not terminal", status: ingestionv1.RunStatus_RUN_STATUS_VALIDATING, terminal: false},
		{name: "PERSISTING is not terminal", status: ingestionv1.RunStatus_RUN_STATUS_PERSISTING, terminal: false},
		// Unknown / out-of-range value
		{name: "unknown numeric value is not terminal", status: ingestionv1.RunStatus(999), terminal: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTerminalStatus(tt.status)
			assert.Equal(t, tt.terminal, got)
		})
	}
}

func TestNewClient(t *testing.T) {
	t.Run("empty address returns error", func(t *testing.T) {
		_, err := NewClient(Config{Address: ""})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "address is required")
	})

	t.Run("valid address creates client", func(t *testing.T) {
		// grpc.NewClient with passthrough resolver does not actually dial,
		// so any address string is accepted without a real server.
		c, err := NewClient(Config{Address: "localhost:50051"})
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.NotNil(t, c.conn)
		assert.NotNil(t, c.client)
		assert.NoError(t, c.Close())
	})

	t.Run("default timeout is 30s when zero", func(t *testing.T) {
		c, err := NewClient(Config{Address: "localhost:50051", Timeout: 0})
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.Equal(t, 30*time.Second, c.timeout, "zero timeout should default to 30s")
		assert.NoError(t, c.Close())
	})

	t.Run("custom timeout is preserved", func(t *testing.T) {
		c, err := NewClient(Config{Address: "localhost:50051", Timeout: 10 * time.Second})
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.Equal(t, 10*time.Second, c.timeout, "custom timeout should be preserved")
		assert.NoError(t, c.Close())
	})
}

func TestClient_Close(t *testing.T) {
	t.Run("nil conn returns nil error", func(t *testing.T) {
		c := &Client{conn: nil}
		err := c.Close()
		assert.NoError(t, err)
	})

	t.Run("close on valid conn succeeds", func(t *testing.T) {
		c, err := NewClient(Config{Address: "localhost:50051"})
		require.NoError(t, err)
		require.NotNil(t, c)

		err = c.Close()
		assert.NoError(t, err)
	})
}

func TestHashFromURL_LongURL(t *testing.T) {
	// Verify that a very long URL (10 000 bytes) still produces a valid
	// 64-character lowercase hex SHA-256 digest.
	longURL := "https://example.com/" + string(make([]byte, 10000))
	hash := hashFromURL(longURL)

	assert.Len(t, hash, 64, "SHA-256 hex digest must be 64 characters even for very long URLs")
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), hash, "hash must be lowercase hex")
}

func TestClient_Close_Idempotent(t *testing.T) {
	// Calling Close() twice on the same client must not panic.
	c, err := NewClient(Config{Address: "localhost:50051"})
	require.NoError(t, err)
	require.NotNil(t, c)

	err = c.Close()
	assert.NoError(t, err, "first close should succeed")

	// Second close — gRPC conn may return an error, but it must not panic.
	assert.NotPanics(t, func() {
		_ = c.Close()
	}, "second close must not panic")
}
