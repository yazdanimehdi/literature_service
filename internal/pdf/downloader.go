// Package pdf provides utilities for downloading and handling PDF files.
package pdf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Sentinel errors for PDF download operations.
var (
	// ErrNotPDF is returned when the response Content-Type is not application/pdf.
	ErrNotPDF = errors.New("pdf: response is not a PDF")
	// ErrTooLarge is returned when the file exceeds the maximum allowed size.
	ErrTooLarge = errors.New("pdf: file exceeds maximum size")
	// ErrDownloadFailed is returned when the download fails due to network or HTTP errors.
	ErrDownloadFailed = errors.New("pdf: download failed")
)

// DownloadResult holds the result of downloading a PDF.
type DownloadResult struct {
	// Content is the PDF bytes.
	Content []byte
	// ContentHash is the SHA-256 hex digest of the content.
	ContentHash string
	// SizeBytes is the size of the content in bytes.
	SizeBytes int64
	// ContentType is the actual Content-Type header from the response.
	ContentType string
}

// Config holds downloader configuration.
type Config struct {
	// Timeout is the HTTP request timeout. Default: 60 seconds.
	Timeout time.Duration
	// MaxSize is the maximum file size in bytes. Default: 100MB.
	MaxSize int64
	// UserAgent is the User-Agent header. Default: "Helixir-LitReview/1.0".
	UserAgent string
}

// Downloader downloads PDFs from URLs.
type Downloader struct {
	client    *http.Client
	maxSize   int64
	userAgent string
}

// NewDownloader creates a new Downloader with the given configuration.
func NewDownloader(cfg Config) *Downloader {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 100 * 1024 * 1024 // 100MB
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Mozilla/5.0 (compatible; Helixir-LitReview/1.0; +https://helixir.io/bot)"
	}

	return &Downloader{
		client: &http.Client{
			Timeout: cfg.Timeout,
			// Follow redirects automatically (default behavior)
		},
		maxSize:   cfg.MaxSize,
		userAgent: cfg.UserAgent,
	}
}

// Download fetches a PDF from the given URL.
// Returns ErrNotPDF if Content-Type is not application/pdf.
// Returns ErrTooLarge if the response exceeds MaxSize.
// Returns ErrDownloadFailed wrapped with HTTP status for non-2xx responses.
func (d *Downloader) Download(ctx context.Context, url string) (*DownloadResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid URL: %v", ErrDownloadFailed, err)
	}
	req.Header.Set("User-Agent", d.userAgent)
	req.Header.Set("Accept", "application/pdf, */*;q=0.8")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDownloadFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check HTTP status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrDownloadFailed, resp.StatusCode)
	}

	// Validate Content-Type (allow "application/pdf" with optional charset etc.)
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "application/pdf") {
		return nil, fmt.Errorf("%w: Content-Type is %q", ErrNotPDF, contentType)
	}

	// Read body with size limit.
	// Read one extra byte to detect if file is too large.
	limitReader := io.LimitReader(resp.Body, d.maxSize+1)
	content, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrDownloadFailed, err)
	}

	// Check if we read more than maxSize
	if int64(len(content)) > d.maxSize {
		return nil, fmt.Errorf("%w: exceeded %d bytes", ErrTooLarge, d.maxSize)
	}

	// Compute SHA-256 hash
	hash := sha256.Sum256(content)

	return &DownloadResult{
		Content:     content,
		ContentHash: hex.EncodeToString(hash[:]),
		SizeBytes:   int64(len(content)),
		ContentType: contentType,
	}, nil
}
