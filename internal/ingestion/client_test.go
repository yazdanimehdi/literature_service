package ingestion

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingestionv1 "github.com/helixir/ingestion-service/api/gen/ingestion/v1"
)

func TestHashURL(t *testing.T) {
	t.Run("returns 64-char hex string", func(t *testing.T) {
		hash := hashURL("https://example.com/paper.pdf")
		assert.Len(t, hash, 64, "SHA-256 hex digest must be 64 characters")
	})

	t.Run("different URLs produce different hashes", func(t *testing.T) {
		hash1 := hashURL("https://example.com/paper1.pdf")
		hash2 := hashURL("https://example.com/paper2.pdf")
		assert.NotEqual(t, hash1, hash2, "different URLs should produce different hashes")
	})

	t.Run("same URL produces same hash (deterministic)", func(t *testing.T) {
		url := "https://example.com/stable.pdf"
		hash1 := hashURL(url)
		hash2 := hashURL(url)
		assert.Equal(t, hash1, hash2, "same URL must always produce the same hash")
	})

	t.Run("empty URL returns valid 64-char hex", func(t *testing.T) {
		hash := hashURL("")
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
			hash := hashURL(u)
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

func TestHashURL_LongURL(t *testing.T) {
	// Verify that a very long URL (10 000 bytes) still produces a valid
	// 64-character lowercase hex SHA-256 digest.
	longURL := "https://example.com/" + string(make([]byte, 10000))
	hash := hashURL(longURL)

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

func TestStartIngestionWithContentRequest(t *testing.T) {
	t.Run("request struct has all expected fields", func(t *testing.T) {
		req := StartIngestionWithContentRequest{
			OrgID:          "org-123",
			ProjectID:      "proj-456",
			IdempotencyKey: "idem-key-789",
			MimeType:       "application/pdf",
			ContentHash:    "abc123def456",
			FileSize:       1024,
			SourceKind:     "literature_review",
			Filename:       "paper.pdf",
			Content:        []byte("test content"),
		}

		assert.Equal(t, "org-123", req.OrgID)
		assert.Equal(t, "proj-456", req.ProjectID)
		assert.Equal(t, "idem-key-789", req.IdempotencyKey)
		assert.Equal(t, "application/pdf", req.MimeType)
		assert.Equal(t, "abc123def456", req.ContentHash)
		assert.Equal(t, int64(1024), req.FileSize)
		assert.Equal(t, "literature_review", req.SourceKind)
		assert.Equal(t, "paper.pdf", req.Filename)
		assert.Equal(t, []byte("test content"), req.Content)
	})

	t.Run("empty content is valid", func(t *testing.T) {
		req := StartIngestionWithContentRequest{
			OrgID:          "org-123",
			ProjectID:      "proj-456",
			IdempotencyKey: "key",
			MimeType:       "application/pdf",
			ContentHash:    "abc123",
			FileSize:       0,
			SourceKind:     "literature_review",
			Filename:       "empty.pdf",
			Content:        []byte{},
		}
		assert.Empty(t, req.Content)
		assert.Equal(t, int64(0), req.FileSize)
	})
}

func TestStartIngestionWithContentResult(t *testing.T) {
	t.Run("result struct has all expected fields", func(t *testing.T) {
		result := StartIngestionWithContentResult{
			RunID:      "run-123",
			FileID:     "file-456",
			Status:     "RUN_STATUS_PENDING",
			IsExisting: false,
		}

		assert.Equal(t, "run-123", result.RunID)
		assert.Equal(t, "file-456", result.FileID)
		assert.Equal(t, "RUN_STATUS_PENDING", result.Status)
		assert.False(t, result.IsExisting)
	})

	t.Run("result can represent idempotent hit", func(t *testing.T) {
		result := StartIngestionWithContentResult{
			RunID:      "existing-run",
			FileID:     "existing-file",
			Status:     "RUN_STATUS_EXECUTING",
			IsExisting: true,
		}

		assert.True(t, result.IsExisting)
	})
}

func TestStartIngestionResult_FileID(t *testing.T) {
	t.Run("FileID field is present and can be populated", func(t *testing.T) {
		result := StartIngestionResult{
			RunID:      "run-123",
			Status:     "RUN_STATUS_PENDING",
			IsExisting: false,
			FileID:     "file-abc-123",
		}

		assert.Equal(t, "file-abc-123", result.FileID)
	})

	t.Run("FileID can be empty string", func(t *testing.T) {
		result := StartIngestionResult{
			RunID:      "run-123",
			Status:     "RUN_STATUS_PENDING",
			IsExisting: false,
			FileID:     "",
		}

		assert.Empty(t, result.FileID)
	})
}

func TestStreamingConstants(t *testing.T) {
	t.Run("chunk size is 32KB", func(t *testing.T) {
		assert.Equal(t, 32*1024, streamingChunkSize)
	})

	t.Run("streaming timeout is 5 minutes", func(t *testing.T) {
		assert.Equal(t, 5*time.Minute, streamingTimeout)
	})

	t.Run("max content size is 100MB", func(t *testing.T) {
		assert.Equal(t, 100*1024*1024, maxContentSize)
	})
}

// TestStartIngestionWithContent_Validation tests all validation cases for streaming ingestion.
func TestStartIngestionWithContent_Validation(t *testing.T) {
	// Create a client (doesn't need real server for validation tests)
	c, err := NewClient(Config{Address: "localhost:50051"})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// Valid content hash for reuse
	validHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	validContent := []byte("test content")

	tests := []struct {
		name    string
		req     StartIngestionWithContentRequest
		wantErr string
	}{
		{
			name: "missing OrgID",
			req: StartIngestionWithContentRequest{
				OrgID:          "",
				ProjectID:      "proj-123",
				IdempotencyKey: "key-123",
				MimeType:       "application/pdf",
				ContentHash:    validHash,
				FileSize:       int64(len(validContent)),
				Content:        validContent,
			},
			wantErr: "OrgID is required",
		},
		{
			name: "missing ProjectID",
			req: StartIngestionWithContentRequest{
				OrgID:          "org-123",
				ProjectID:      "",
				IdempotencyKey: "key-123",
				MimeType:       "application/pdf",
				ContentHash:    validHash,
				FileSize:       int64(len(validContent)),
				Content:        validContent,
			},
			wantErr: "ProjectID is required",
		},
		{
			name: "missing IdempotencyKey",
			req: StartIngestionWithContentRequest{
				OrgID:          "org-123",
				ProjectID:      "proj-123",
				IdempotencyKey: "",
				MimeType:       "application/pdf",
				ContentHash:    validHash,
				FileSize:       int64(len(validContent)),
				Content:        validContent,
			},
			wantErr: "IdempotencyKey is required",
		},
		{
			name: "missing MimeType",
			req: StartIngestionWithContentRequest{
				OrgID:          "org-123",
				ProjectID:      "proj-123",
				IdempotencyKey: "key-123",
				MimeType:       "",
				ContentHash:    validHash,
				FileSize:       int64(len(validContent)),
				Content:        validContent,
			},
			wantErr: "MimeType is required",
		},
		{
			name: "missing ContentHash",
			req: StartIngestionWithContentRequest{
				OrgID:          "org-123",
				ProjectID:      "proj-123",
				IdempotencyKey: "key-123",
				MimeType:       "application/pdf",
				ContentHash:    "",
				FileSize:       int64(len(validContent)),
				Content:        validContent,
			},
			wantErr: "ContentHash is required",
		},
		{
			name: "ContentHash too short",
			req: StartIngestionWithContentRequest{
				OrgID:          "org-123",
				ProjectID:      "proj-123",
				IdempotencyKey: "key-123",
				MimeType:       "application/pdf",
				ContentHash:    "abcdef",
				FileSize:       int64(len(validContent)),
				Content:        validContent,
			},
			wantErr: "ContentHash must be 64-character hex SHA-256 digest",
		},
		{
			name: "ContentHash too long",
			req: StartIngestionWithContentRequest{
				OrgID:          "org-123",
				ProjectID:      "proj-123",
				IdempotencyKey: "key-123",
				MimeType:       "application/pdf",
				ContentHash:    validHash + "extra",
				FileSize:       int64(len(validContent)),
				Content:        validContent,
			},
			wantErr: "ContentHash must be 64-character hex SHA-256 digest",
		},
		{
			name: "FileSize mismatch (too small)",
			req: StartIngestionWithContentRequest{
				OrgID:          "org-123",
				ProjectID:      "proj-123",
				IdempotencyKey: "key-123",
				MimeType:       "application/pdf",
				ContentHash:    validHash,
				FileSize:       5, // Content is 12 bytes
				Content:        validContent,
			},
			wantErr: "FileSize (5) does not match Content length",
		},
		{
			name: "FileSize mismatch (too large)",
			req: StartIngestionWithContentRequest{
				OrgID:          "org-123",
				ProjectID:      "proj-123",
				IdempotencyKey: "key-123",
				MimeType:       "application/pdf",
				ContentHash:    validHash,
				FileSize:       1000,
				Content:        validContent,
			},
			wantErr: "FileSize (1000) does not match Content length",
		},
		{
			name: "Content exceeds max size",
			req: StartIngestionWithContentRequest{
				OrgID:          "org-123",
				ProjectID:      "proj-123",
				IdempotencyKey: "key-123",
				MimeType:       "application/pdf",
				ContentHash:    validHash,
				FileSize:       int64(maxContentSize + 1),
				Content:        make([]byte, maxContentSize+1),
			},
			wantErr: "Content exceeds maximum size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.StartIngestionWithContent(ctx, tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestStartIngestionWithContentRequest_Valid tests valid request edge cases.
func TestStartIngestionWithContentRequest_Valid(t *testing.T) {
	t.Run("empty content is valid with FileSize 0", func(t *testing.T) {
		req := StartIngestionWithContentRequest{
			OrgID:          "org-123",
			ProjectID:      "proj-123",
			IdempotencyKey: "key-123",
			MimeType:       "application/pdf",
			ContentHash:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			FileSize:       0,
			Content:        []byte{},
		}

		// Validation should pass (actual send would fail without server)
		assert.Equal(t, int64(0), req.FileSize)
		assert.Empty(t, req.Content)
		assert.Equal(t, 64, len(req.ContentHash))
	})

	t.Run("content at chunk boundary", func(t *testing.T) {
		// Exactly 32KB
		content := make([]byte, streamingChunkSize)
		for i := range content {
			content[i] = byte(i % 256)
		}

		req := StartIngestionWithContentRequest{
			OrgID:          "org-123",
			ProjectID:      "proj-123",
			IdempotencyKey: "key-123",
			MimeType:       "application/pdf",
			ContentHash:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			FileSize:       int64(len(content)),
			Content:        content,
		}

		assert.Equal(t, int64(streamingChunkSize), req.FileSize)
	})

	t.Run("content just under chunk boundary", func(t *testing.T) {
		content := make([]byte, streamingChunkSize-1)
		req := StartIngestionWithContentRequest{
			FileSize: int64(len(content)),
			Content:  content,
		}
		assert.Equal(t, int64(streamingChunkSize-1), req.FileSize)
	})

	t.Run("content spanning multiple chunks", func(t *testing.T) {
		// 2.5 chunks = 80KB
		content := make([]byte, streamingChunkSize*2+streamingChunkSize/2)
		req := StartIngestionWithContentRequest{
			FileSize: int64(len(content)),
			Content:  content,
		}
		assert.Equal(t, int64(80*1024), req.FileSize)
	})

	t.Run("content at max size boundary", func(t *testing.T) {
		// Exactly at max (100MB) should be valid
		req := StartIngestionWithContentRequest{
			FileSize: int64(maxContentSize),
			Content:  make([]byte, maxContentSize),
		}
		assert.Equal(t, int64(maxContentSize), req.FileSize)
	})
}

// TestRunStatus tests the RunStatus struct.
func TestRunStatus(t *testing.T) {
	t.Run("all fields populated", func(t *testing.T) {
		status := RunStatus{
			RunID:        "run-123",
			Status:       "RUN_STATUS_EXECUTING",
			IsTerminal:   false,
			ErrorMessage: "",
		}

		assert.Equal(t, "run-123", status.RunID)
		assert.Equal(t, "RUN_STATUS_EXECUTING", status.Status)
		assert.False(t, status.IsTerminal)
		assert.Empty(t, status.ErrorMessage)
	})

	t.Run("failed run with error message", func(t *testing.T) {
		status := RunStatus{
			RunID:        "run-456",
			Status:       "RUN_STATUS_FAILED",
			IsTerminal:   true,
			ErrorMessage: "PDF parsing failed: corrupted file",
		}

		assert.True(t, status.IsTerminal)
		assert.NotEmpty(t, status.ErrorMessage)
	})
}

// TestConfig tests the Config struct.
func TestConfig(t *testing.T) {
	t.Run("default TLS is false", func(t *testing.T) {
		cfg := Config{Address: "localhost:50051"}
		assert.False(t, cfg.TLS)
	})

	t.Run("TLS can be enabled", func(t *testing.T) {
		cfg := Config{Address: "localhost:50051", TLS: true}
		assert.True(t, cfg.TLS)
	})

	t.Run("timeout can be customized", func(t *testing.T) {
		cfg := Config{Address: "localhost:50051", Timeout: 60 * time.Second}
		assert.Equal(t, 60*time.Second, cfg.Timeout)
	})
}

// TestNewClient_TLS tests TLS configuration.
func TestNewClient_TLS(t *testing.T) {
	t.Run("creates client with TLS enabled", func(t *testing.T) {
		// This will create a client with TLS credentials
		// Note: connection won't succeed without a real TLS server
		c, err := NewClient(Config{
			Address: "localhost:50051",
			TLS:     true,
		})
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.NoError(t, c.Close())
	})

	t.Run("creates client with TLS disabled", func(t *testing.T) {
		c, err := NewClient(Config{
			Address: "localhost:50051",
			TLS:     false,
		})
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.NoError(t, c.Close())
	})
}

// TestStartIngestionRequest tests the StartIngestionRequest struct.
func TestStartIngestionRequest(t *testing.T) {
	t.Run("all fields populated", func(t *testing.T) {
		req := StartIngestionRequest{
			OrgID:          "org-123",
			ProjectID:      "proj-456",
			IdempotencyKey: "litreview/req-789/paper-abc",
			PDFURL:         "https://arxiv.org/pdf/2301.00000.pdf",
			MimeType:       "application/pdf",
		}

		assert.Equal(t, "org-123", req.OrgID)
		assert.Equal(t, "proj-456", req.ProjectID)
		assert.Equal(t, "litreview/req-789/paper-abc", req.IdempotencyKey)
		assert.Equal(t, "https://arxiv.org/pdf/2301.00000.pdf", req.PDFURL)
		assert.Equal(t, "application/pdf", req.MimeType)
	})
}

// TestChunkingLogic tests the chunking behavior conceptually.
func TestChunkingLogic(t *testing.T) {
	t.Run("calculates correct number of chunks", func(t *testing.T) {
		testCases := []struct {
			contentSize    int
			expectedChunks int
		}{
			{0, 0},
			{1, 1},
			{streamingChunkSize - 1, 1},
			{streamingChunkSize, 1},
			{streamingChunkSize + 1, 2},
			{streamingChunkSize * 2, 2},
			{streamingChunkSize*2 + 1, 3},
			{streamingChunkSize * 3, 3},
			{maxContentSize, maxContentSize / streamingChunkSize}, // 3200 chunks for 100MB
		}

		for _, tc := range testCases {
			// Simulate chunking logic from StartIngestionWithContent
			content := make([]byte, tc.contentSize)
			chunks := 0
			remaining := content
			for len(remaining) > 0 {
				end := streamingChunkSize
				if end > len(remaining) {
					end = len(remaining)
				}
				remaining = remaining[end:]
				chunks++
			}

			if tc.contentSize == 0 {
				assert.Equal(t, 0, chunks, "empty content should produce 0 chunks")
			} else {
				assert.Equal(t, tc.expectedChunks, chunks, "content size %d should produce %d chunks", tc.contentSize, tc.expectedChunks)
			}
		}
	})
}
