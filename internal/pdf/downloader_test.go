package pdf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// samplePDFContent simulates minimal PDF-like bytes for testing.
var samplePDFContent = []byte("%PDF-1.4 sample content for testing")

// writeContent is a test helper that writes content to the response writer.
func writeContent(w http.ResponseWriter, content []byte) {
	_, _ = w.Write(content)
}

func TestNewDownloader_Defaults(t *testing.T) {
	t.Run("applies default values", func(t *testing.T) {
		d := NewDownloader(Config{})

		require.NotNil(t, d)
		assert.Equal(t, int64(100*1024*1024), d.maxSize)
		assert.Equal(t, "Helixir-LitReview/1.0", d.userAgent)
		assert.Equal(t, 60*time.Second, d.client.Timeout)
	})

	t.Run("uses custom config values", func(t *testing.T) {
		cfg := Config{
			Timeout:   30 * time.Second,
			MaxSize:   50 * 1024 * 1024,
			UserAgent: "CustomAgent/2.0",
		}

		d := NewDownloader(cfg)

		require.NotNil(t, d)
		assert.Equal(t, int64(50*1024*1024), d.maxSize)
		assert.Equal(t, "CustomAgent/2.0", d.userAgent)
		assert.Equal(t, 30*time.Second, d.client.Timeout)
	})
}

func TestDownload_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		writeContent(w, samplePDFContent)
	}))
	defer server.Close()

	d := NewDownloader(Config{})

	result, err := d.Download(context.Background(), server.URL)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, samplePDFContent, result.Content)
	assert.Equal(t, int64(len(samplePDFContent)), result.SizeBytes)
	assert.Equal(t, "application/pdf", result.ContentType)
	assert.NotEmpty(t, result.ContentHash)
	assert.Len(t, result.ContentHash, 64) // SHA-256 produces 64 hex chars
}

func TestDownload_HashCorrectness(t *testing.T) {
	testContent := []byte("test PDF content for hash verification")
	expectedHash := sha256.Sum256(testContent)
	expectedHashHex := hex.EncodeToString(expectedHash[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		writeContent(w, testContent)
	}))
	defer server.Close()

	d := NewDownloader(Config{})

	result, err := d.Download(context.Background(), server.URL)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, expectedHashHex, result.ContentHash)
	// Verify lowercase hex
	assert.Equal(t, result.ContentHash, expectedHashHex)
}

func TestDownload_NonPDFContentType(t *testing.T) {
	testCases := []struct {
		name        string
		contentType string
	}{
		{"text/html", "text/html"},
		{"text/plain", "text/plain"},
		{"application/json", "application/json"},
		{"image/png", "image/png"},
		{"empty", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				}
				w.WriteHeader(http.StatusOK)
				writeContent(w, []byte("<html>Not a PDF</html>"))
			}))
			defer server.Close()

			d := NewDownloader(Config{})

			result, err := d.Download(context.Background(), server.URL)
			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, ErrNotPDF)
			assert.Contains(t, err.Error(), "Content-Type")
		})
	}
}

func TestDownload_ContentTypeWithCharset(t *testing.T) {
	testCases := []struct {
		name        string
		contentType string
	}{
		{"with charset utf-8", "application/pdf; charset=utf-8"},
		{"with charset ISO-8859-1", "application/pdf; charset=ISO-8859-1"},
		{"with boundary", "application/pdf; boundary=something"},
		{"uppercase", "Application/PDF"},
		{"mixed case", "Application/Pdf; Charset=UTF-8"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(http.StatusOK)
				writeContent(w, samplePDFContent)
			}))
			defer server.Close()

			d := NewDownloader(Config{})

			result, err := d.Download(context.Background(), server.URL)
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, samplePDFContent, result.Content)
			assert.Equal(t, tc.contentType, result.ContentType)
		})
	}
}

func TestDownload_TooLarge(t *testing.T) {
	// Create content larger than max size
	largeContent := make([]byte, 1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		writeContent(w, largeContent)
	}))
	defer server.Close()

	// Set max size smaller than content
	d := NewDownloader(Config{
		MaxSize: 512,
	})

	result, err := d.Download(context.Background(), server.URL)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrTooLarge)
	assert.Contains(t, err.Error(), "exceeded")
	assert.Contains(t, err.Error(), "512")
}

func TestDownload_ExactlyMaxSize(t *testing.T) {
	// Content exactly at max size should succeed
	content := make([]byte, 512)
	for i := range content {
		content[i] = byte(i % 256)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		writeContent(w, content)
	}))
	defer server.Close()

	d := NewDownloader(Config{
		MaxSize: 512,
	})

	result, err := d.Download(context.Background(), server.URL)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int64(512), result.SizeBytes)
}

func TestDownload_HTTP404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		writeContent(w, []byte("Not Found"))
	}))
	defer server.Close()

	d := NewDownloader(Config{})

	result, err := d.Download(context.Background(), server.URL)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrDownloadFailed)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestDownload_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		writeContent(w, []byte("Internal Server Error"))
	}))
	defer server.Close()

	d := NewDownloader(Config{})

	result, err := d.Download(context.Background(), server.URL)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrDownloadFailed)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestDownload_HTTPStatusCodes(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		wantError  bool
	}{
		{"200 OK", http.StatusOK, false},
		{"201 Created", http.StatusCreated, false},
		{"204 No Content", http.StatusNoContent, false},
		{"301 Moved Permanently", http.StatusMovedPermanently, true}, // Without redirect
		{"400 Bad Request", http.StatusBadRequest, true},
		{"401 Unauthorized", http.StatusUnauthorized, true},
		{"403 Forbidden", http.StatusForbidden, true},
		{"404 Not Found", http.StatusNotFound, true},
		{"429 Too Many Requests", http.StatusTooManyRequests, true},
		{"500 Internal Server Error", http.StatusInternalServerError, true},
		{"502 Bad Gateway", http.StatusBadGateway, true},
		{"503 Service Unavailable", http.StatusServiceUnavailable, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/pdf")
				w.WriteHeader(tc.statusCode)
				writeContent(w, samplePDFContent)
			}))
			defer server.Close()

			// Disable redirect following to test raw status codes
			d := &Downloader{
				client: &http.Client{
					Timeout: 10 * time.Second,
					CheckRedirect: func(req *http.Request, via []*http.Request) error {
						return http.ErrUseLastResponse
					},
				},
				maxSize:   100 * 1024 * 1024,
				userAgent: "Test/1.0",
			}

			result, err := d.Download(context.Background(), server.URL)
			if tc.wantError {
				require.Error(t, err)
				assert.Nil(t, result)
				assert.ErrorIs(t, err, ErrDownloadFailed)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
		})
	}
}

func TestDownload_Redirect(t *testing.T) {
	// Create final server that serves PDF
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		writeContent(w, samplePDFContent)
	}))
	defer finalServer.Close()

	// Create redirect server that redirects to final server
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL, http.StatusMovedPermanently)
	}))
	defer redirectServer.Close()

	d := NewDownloader(Config{})

	result, err := d.Download(context.Background(), redirectServer.URL)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, samplePDFContent, result.Content)
	assert.Equal(t, "application/pdf", result.ContentType)
}

func TestDownload_MultipleRedirects(t *testing.T) {
	// Create final server
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		writeContent(w, samplePDFContent)
	}))
	defer finalServer.Close()

	// Create intermediate redirect server
	intermediateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL, http.StatusFound)
	}))
	defer intermediateServer.Close()

	// Create first redirect server
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, intermediateServer.URL, http.StatusMovedPermanently)
	}))
	defer firstServer.Close()

	d := NewDownloader(Config{})

	result, err := d.Download(context.Background(), firstServer.URL)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, samplePDFContent, result.Content)
}

func TestDownload_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		writeContent(w, samplePDFContent)
	}))
	defer server.Close()

	d := NewDownloader(Config{})

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel context after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := d.Download(ctx, server.URL)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrDownloadFailed)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestDownload_ContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		writeContent(w, samplePDFContent)
	}))
	defer server.Close()

	d := NewDownloader(Config{})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := d.Download(ctx, server.URL)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrDownloadFailed)
}

func TestDownload_InvalidURL(t *testing.T) {
	d := NewDownloader(Config{})

	testCases := []struct {
		name string
		url  string
	}{
		{"empty URL", ""},
		{"invalid scheme", "not-a-url"},
		{"missing host", "http://"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := d.Download(context.Background(), tc.url)
			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, ErrDownloadFailed)
		})
	}
}

func TestDownload_UserAgent(t *testing.T) {
	var receivedUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		writeContent(w, samplePDFContent)
	}))
	defer server.Close()

	t.Run("default user agent", func(t *testing.T) {
		d := NewDownloader(Config{})

		_, err := d.Download(context.Background(), server.URL)
		require.NoError(t, err)

		assert.Equal(t, "Helixir-LitReview/1.0", receivedUserAgent)
	})

	t.Run("custom user agent", func(t *testing.T) {
		d := NewDownloader(Config{
			UserAgent: "CustomBot/3.0",
		})

		_, err := d.Download(context.Background(), server.URL)
		require.NoError(t, err)

		assert.Equal(t, "CustomBot/3.0", receivedUserAgent)
	})
}

func TestDownload_ConnectionRefused(t *testing.T) {
	d := NewDownloader(Config{
		Timeout: 1 * time.Second,
	})

	// Use a port that is unlikely to be in use
	result, err := d.Download(context.Background(), "http://127.0.0.1:59999/pdf")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrDownloadFailed)
}

func TestDownload_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		// Write nothing
	}))
	defer server.Close()

	d := NewDownloader(Config{})

	result, err := d.Download(context.Background(), server.URL)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Empty(t, result.Content)
	assert.Equal(t, int64(0), result.SizeBytes)
	// Empty content still has a hash
	assert.NotEmpty(t, result.ContentHash)
}

func TestDownloadResult_Fields(t *testing.T) {
	content := []byte("test content")
	hash := sha256.Sum256(content)
	hashHex := hex.EncodeToString(hash[:])

	result := &DownloadResult{
		Content:     content,
		ContentHash: hashHex,
		SizeBytes:   int64(len(content)),
		ContentType: "application/pdf",
	}

	assert.Equal(t, content, result.Content)
	assert.Equal(t, hashHex, result.ContentHash)
	assert.Equal(t, int64(12), result.SizeBytes)
	assert.Equal(t, "application/pdf", result.ContentType)
}
