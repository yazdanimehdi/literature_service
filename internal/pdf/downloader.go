// Package pdf provides utilities for downloading and handling PDF files.
package pdf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
	// ErrSSRF is returned when the URL resolves to a private/internal network address.
	ErrSSRF = errors.New("pdf: request to private network denied")
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
	// AllowPrivateNetworks disables SSRF private-IP checks. This MUST only be
	// set to true in test environments. Production code must never set this.
	AllowPrivateNetworks bool
}

// Downloader downloads PDFs from URLs.
type Downloader struct {
	client               *http.Client
	maxSize              int64
	userAgent            string
	allowPrivateNetworks bool // For testing only; never enable in production.
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

	d := &Downloader{
		maxSize:              cfg.MaxSize,
		userAgent:            cfg.UserAgent,
		allowPrivateNetworks: cfg.AllowPrivateNetworks,
	}

	d.client = &http.Client{
		Timeout: cfg.Timeout,
		// Validate each redirect URL against private IP checks to prevent
		// SSRF via open redirects that land on internal network addresses.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("%w: too many redirects", ErrSSRF)
			}
			if !d.allowPrivateNetworks {
				if err := validateURLNotPrivate(req.URL.String()); err != nil {
					return err
				}
			}
			return nil
		},
	}

	return d
}

// isPrivateIP returns true if the IP address is in a private, loopback, or
// otherwise non-routable range. Covers both IPv4 and IPv6 private ranges.
func isPrivateIP(ip net.IP) bool {
	// IPv4 private ranges.
	privateRanges := []struct{ start, end net.IP }{
		{net.ParseIP("10.0.0.0"), net.ParseIP("10.255.255.255")},
		{net.ParseIP("172.16.0.0"), net.ParseIP("172.31.255.255")},
		{net.ParseIP("192.168.0.0"), net.ParseIP("192.168.255.255")},
		{net.ParseIP("169.254.0.0"), net.ParseIP("169.254.255.255")},
		// IPv6 Unique Local Addresses (fc00::/7).
		{net.ParseIP("fc00::"), net.ParseIP("fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")},
		// IPv6 link-local (fe80::/10).
		{net.ParseIP("fe80::"), net.ParseIP("febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff")},
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// IPv6 loopback (::1) is already covered by ip.IsLoopback() above.
	for _, r := range privateRanges {
		if bytesInRange(ip.To16(), r.start.To16(), r.end.To16()) {
			return true
		}
	}
	return false
}

func bytesInRange(ip, lo, hi []byte) bool {
	for i := range ip {
		if ip[i] < lo[i] {
			return false
		}
		if ip[i] > hi[i] {
			return false
		}
	}
	return true
}

// validateURLNotPrivate resolves the hostname and rejects private IPs.
func validateURLNotPrivate(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSSRF, err)
	}

	// Reject non-HTTP(S) schemes to prevent file://, gopher://, etc.
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		// allowed
	default:
		return fmt.Errorf("%w: scheme %q is not allowed", ErrSSRF, parsed.Scheme)
	}

	host := parsed.Hostname()
	ips, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("%w: DNS lookup failed for %s: %w", ErrDownloadFailed, host, err)
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip != nil && isPrivateIP(ip) {
			return fmt.Errorf("%w: %s resolves to private address %s", ErrSSRF, host, ipStr)
		}
	}
	return nil
}

// Download fetches a PDF from the given URL.
// Returns ErrNotPDF if Content-Type is not application/pdf.
// Returns ErrTooLarge if the response exceeds MaxSize.
// Returns ErrSSRF if the URL resolves to a private network address.
// Returns ErrDownloadFailed wrapped with HTTP status for non-2xx responses.
func (d *Downloader) Download(ctx context.Context, url string) (*DownloadResult, error) {
	if !d.allowPrivateNetworks {
		if err := validateURLNotPrivate(url); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid URL: %w", ErrDownloadFailed, err)
	}
	req.Header.Set("User-Agent", d.userAgent)
	req.Header.Set("Accept", "application/pdf, */*;q=0.8")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDownloadFailed, err)
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
		return nil, fmt.Errorf("%w: read body: %w", ErrDownloadFailed, err)
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
