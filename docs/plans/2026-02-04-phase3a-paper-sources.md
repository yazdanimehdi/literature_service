# Phase 3A: Paper Source Clients Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement paper source API clients for searching academic databases (Semantic Scholar, OpenAlex, PubMed, bioRxiv, arXiv) with rate limiting and a unified interface.

**Architecture:** A common `PaperSource` interface defines the search contract. Each source has its own client implementation that handles API-specific details and maps responses to domain models. A token bucket rate limiter enforces per-source rate limits. A `SourceRegistry` aggregates all enabled sources for concurrent searching.

**Tech Stack:** Go 1.25, net/http, golang.org/x/time/rate, testify, httptest

---

## Phase 3A Deliverables Summary

| ID | Deliverable | Description |
|----|-------------|-------------|
| D3A.1 | PaperSource Interface | Common interface for all paper sources |
| D3A.2 | Rate Limiter | Token bucket rate limiter with per-source limits |
| D3A.3 | Semantic Scholar Client | Semantic Scholar API client |
| D3A.4 | OpenAlex Client | OpenAlex API client |
| D3A.5 | PubMed Client | PubMed E-utilities API client |
| D3A.6 | arXiv Client | arXiv API client |
| D3A.7 | bioRxiv Client | bioRxiv API client |
| D3A.8 | Source Registry | Aggregates enabled sources |

---

## Task 1: Create PaperSource Interface and Search Types

**Files:**
- Create: `internal/papersources/source.go`
- Test: `internal/papersources/source_test.go`

**Step 1: Write the interface and types**

Create `internal/papersources/source.go`:

```go
// Package papersources provides clients for searching academic paper databases.
package papersources

import (
	"context"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
)

// SearchParams defines parameters for paper searches.
type SearchParams struct {
	// Query is the search query (keyword or phrase).
	Query string

	// DateFrom filters papers published on or after this date.
	DateFrom *time.Time

	// DateTo filters papers published on or before this date.
	DateTo *time.Time

	// MaxResults limits the number of results returned.
	MaxResults int

	// Offset for pagination (0-based).
	Offset int

	// IncludePreprints includes preprint papers.
	IncludePreprints bool

	// OpenAccessOnly returns only open access papers.
	OpenAccessOnly bool

	// MinCitations filters by minimum citation count.
	MinCitations int
}

// SearchResult holds the result of a paper search.
type SearchResult struct {
	// Papers contains the papers found.
	Papers []*domain.Paper

	// TotalResults is the total number of matching papers (for pagination).
	TotalResults int

	// HasMore indicates if more results are available.
	HasMore bool

	// NextOffset is the offset for the next page of results.
	NextOffset int

	// Source identifies which source produced these results.
	Source domain.SourceType

	// SearchDuration is how long the search took.
	SearchDuration time.Duration
}

// PaperSource defines the interface for paper search providers.
type PaperSource interface {
	// Search searches for papers matching the given parameters.
	Search(ctx context.Context, params SearchParams) (*SearchResult, error)

	// GetByID retrieves a paper by its source-specific identifier.
	GetByID(ctx context.Context, id string) (*domain.Paper, error)

	// SourceType returns the type of this source.
	SourceType() domain.SourceType

	// Name returns a human-readable name for this source.
	Name() string

	// IsEnabled returns whether this source is currently enabled.
	IsEnabled() bool
}
```

**Step 2: Run test to verify compilation**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go build ./internal/papersources/...`
Expected: Build succeeds

**Step 3: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/papersources/source.go
git commit -m "feat(papersources): add PaperSource interface and search types"
```

---

## Task 2: Implement Rate Limiter

**Files:**
- Create: `internal/papersources/ratelimit.go`
- Create: `internal/papersources/ratelimit_test.go`

**Step 1: Write the failing test**

Create `internal/papersources/ratelimit_test.go`:

```go
package papersources

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10.0, 5)
	require.NotNil(t, rl)
}

func TestRateLimiter_Wait(t *testing.T) {
	// Create limiter with 100 requests/second, burst of 10
	rl := NewRateLimiter(100.0, 10)

	ctx := context.Background()

	// First 10 requests should be instant (burst)
	start := time.Now()
	for i := 0; i < 10; i++ {
		err := rl.Wait(ctx)
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	// Burst should complete quickly (< 100ms)
	assert.Less(t, elapsed, 100*time.Millisecond, "burst should be instant")
}

func TestRateLimiter_WaitContextCanceled(t *testing.T) {
	// Create limiter with very low rate
	rl := NewRateLimiter(0.1, 1)

	// Exhaust burst
	ctx := context.Background()
	_ = rl.Wait(ctx)

	// Cancel context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should return context error
	err := rl.Wait(ctx)
	assert.Error(t, err)
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(10.0, 5)

	// First 5 should be allowed (burst)
	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow(), "request %d should be allowed", i)
	}

	// Next request might not be allowed immediately
	// (depends on timing, so we just check it doesn't panic)
	_ = rl.Allow()
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/... -run TestRateLimiter`
Expected: FAIL (NewRateLimiter undefined)

**Step 3: Write implementation**

Create `internal/papersources/ratelimit.go`:

```go
package papersources

import (
	"context"

	"golang.org/x/time/rate"
)

// RateLimiter wraps a token bucket rate limiter.
type RateLimiter struct {
	limiter *rate.Limiter
}

// NewRateLimiter creates a new rate limiter.
// ratePerSecond is the sustained rate of requests per second.
// burst is the maximum burst size (number of requests that can be made instantly).
func NewRateLimiter(ratePerSecond float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(ratePerSecond), burst),
	}
}

// Wait blocks until a request is allowed or the context is canceled.
func (r *RateLimiter) Wait(ctx context.Context) error {
	return r.limiter.Wait(ctx)
}

// Allow returns true if a request is allowed without waiting.
func (r *RateLimiter) Allow() bool {
	return r.limiter.Allow()
}

// SetRate updates the rate limit.
func (r *RateLimiter) SetRate(ratePerSecond float64) {
	r.limiter.SetLimit(rate.Limit(ratePerSecond))
}

// SetBurst updates the burst size.
func (r *RateLimiter) SetBurst(burst int) {
	r.limiter.SetBurst(burst)
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/... -run TestRateLimiter`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/papersources/ratelimit.go internal/papersources/ratelimit_test.go
git commit -m "feat(papersources): add token bucket rate limiter"
```

---

## Task 3: Create Base HTTP Client

**Files:**
- Create: `internal/papersources/httpclient.go`
- Create: `internal/papersources/httpclient_test.go`

**Step 1: Write the failing test**

Create `internal/papersources/httpclient_test.go`:

```go
package papersources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPClient(t *testing.T) {
	client := NewHTTPClient(HTTPClientConfig{
		Timeout:     30 * time.Second,
		RateLimit:   10.0,
		BurstSize:   5,
		MaxRetries:  3,
		RetryDelay:  time.Second,
		UserAgent:   "test-agent",
	})
	require.NotNil(t, client)
}

func TestHTTPClient_Do(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-agent", r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(HTTPClientConfig{
		Timeout:    30 * time.Second,
		RateLimit:  100.0,
		BurstSize:  10,
		MaxRetries: 3,
		UserAgent:  "test-agent",
	})

	req, err := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHTTPClient_DoWithRateLimit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient(HTTPClientConfig{
		Timeout:   30 * time.Second,
		RateLimit: 100.0,
		BurstSize: 5,
	})

	// Make 5 requests (within burst)
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
	}

	assert.Equal(t, 5, callCount)
}

func TestHTTPClient_DoRetryOn429(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient(HTTPClientConfig{
		Timeout:    30 * time.Second,
		RateLimit:  100.0,
		BurstSize:  10,
		MaxRetries: 3,
		RetryDelay: 10 * time.Millisecond,
	})

	req, _ := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 3, callCount)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/... -run TestHTTPClient`
Expected: FAIL (NewHTTPClient undefined)

**Step 3: Write implementation**

Create `internal/papersources/httpclient.go`:

```go
package papersources

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// HTTPClientConfig configures the HTTP client.
type HTTPClientConfig struct {
	// Timeout is the request timeout.
	Timeout time.Duration

	// RateLimit is requests per second.
	RateLimit float64

	// BurstSize is the maximum burst.
	BurstSize int

	// MaxRetries is the maximum retry attempts.
	MaxRetries int

	// RetryDelay is the base delay between retries.
	RetryDelay time.Duration

	// UserAgent is the User-Agent header value.
	UserAgent string

	// APIKey is an optional API key for authentication.
	APIKey string

	// APIKeyHeader is the header name for the API key.
	APIKeyHeader string
}

// HTTPClient wraps http.Client with rate limiting and retries.
type HTTPClient struct {
	client      *http.Client
	rateLimiter *RateLimiter
	config      HTTPClientConfig
}

// NewHTTPClient creates a new HTTP client with rate limiting.
func NewHTTPClient(cfg HTTPClientConfig) *HTTPClient {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 10.0
	}
	if cfg.BurstSize == 0 {
		cfg.BurstSize = 5
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Helixir-LiteratureReview/1.0"
	}

	return &HTTPClient{
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		rateLimiter: NewRateLimiter(cfg.RateLimit, cfg.BurstSize),
		config:      cfg,
	}
}

// Do executes an HTTP request with rate limiting and retries.
func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Wait for rate limiter
	if err := c.rateLimiter.Wait(req.Context()); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	// Set headers
	if c.config.UserAgent != "" {
		req.Header.Set("User-Agent", c.config.UserAgent)
	}
	if c.config.APIKey != "" && c.config.APIKeyHeader != "" {
		req.Header.Set(c.config.APIKeyHeader, c.config.APIKey)
	}

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			if !c.shouldRetry(req.Context(), attempt) {
				break
			}
			c.sleep(req.Context(), c.config.RetryDelay)
			continue
		}

		// Check for rate limiting response
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			lastErr = fmt.Errorf("rate limited (429)")
			if !c.shouldRetry(req.Context(), attempt) {
				break
			}
			retryAfter := c.parseRetryAfter(resp)
			c.sleep(req.Context(), retryAfter)
			continue
		}

		// Check for server errors (5xx)
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			if !c.shouldRetry(req.Context(), attempt) {
				break
			}
			c.sleep(req.Context(), c.config.RetryDelay)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", c.config.MaxRetries, lastErr)
}

func (c *HTTPClient) shouldRetry(ctx context.Context, attempt int) bool {
	if attempt >= c.config.MaxRetries {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	default:
		return true
	}
}

func (c *HTTPClient) parseRetryAfter(resp *http.Response) time.Duration {
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return c.config.RetryDelay
	}

	// Try parsing as seconds
	if seconds, err := strconv.Atoi(retryAfter); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date
	if t, err := http.ParseTime(retryAfter); err == nil {
		return time.Until(t)
	}

	return c.config.RetryDelay
}

func (c *HTTPClient) sleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/... -run TestHTTPClient`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/papersources/httpclient.go internal/papersources/httpclient_test.go
git commit -m "feat(papersources): add HTTP client with rate limiting and retries"
```

---

## Task 4: Implement Semantic Scholar Client

**Files:**
- Create: `internal/papersources/semanticscholar/client.go`
- Create: `internal/papersources/semanticscholar/client_test.go`
- Create: `internal/papersources/semanticscholar/types.go`

**Step 1: Write API response types**

Create `internal/papersources/semanticscholar/types.go`:

```go
// Package semanticscholar provides a client for the Semantic Scholar API.
package semanticscholar

// SearchResponse represents the Semantic Scholar search API response.
type SearchResponse struct {
	Total   int            `json:"total"`
	Offset  int            `json:"offset"`
	Next    int            `json:"next,omitempty"`
	Data    []PaperResult  `json:"data"`
}

// PaperResult represents a paper in search results.
type PaperResult struct {
	PaperID         string          `json:"paperId"`
	ExternalIDs     *ExternalIDs    `json:"externalIds,omitempty"`
	Title           string          `json:"title"`
	Abstract        string          `json:"abstract,omitempty"`
	Year            int             `json:"year,omitempty"`
	PublicationDate string          `json:"publicationDate,omitempty"`
	Venue           string          `json:"venue,omitempty"`
	Journal         *Journal        `json:"journal,omitempty"`
	Authors         []Author        `json:"authors,omitempty"`
	CitationCount   int             `json:"citationCount"`
	ReferenceCount  int             `json:"referenceCount"`
	IsOpenAccess    bool            `json:"isOpenAccess"`
	OpenAccessPDF   *OpenAccessPDF  `json:"openAccessPdf,omitempty"`
}

// ExternalIDs contains external identifiers for a paper.
type ExternalIDs struct {
	DOI      string `json:"DOI,omitempty"`
	ArXiv    string `json:"ArXiv,omitempty"`
	PubMed   string `json:"PubMed,omitempty"`
	PubMedCentral string `json:"PubMedCentral,omitempty"`
}

// Journal contains journal information.
type Journal struct {
	Name   string `json:"name,omitempty"`
	Volume string `json:"volume,omitempty"`
	Pages  string `json:"pages,omitempty"`
}

// Author represents a paper author.
type Author struct {
	AuthorID string `json:"authorId,omitempty"`
	Name     string `json:"name"`
}

// OpenAccessPDF contains open access PDF information.
type OpenAccessPDF struct {
	URL    string `json:"url,omitempty"`
	Status string `json:"status,omitempty"`
}
```

**Step 2: Write the failing test**

Create `internal/papersources/semanticscholar/client_test.go`:

```go
package semanticscholar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := NewClient(Config{
		BaseURL:   "https://api.semanticscholar.org",
		Timeout:   30 * time.Second,
		RateLimit: 10.0,
	})
	require.NotNil(t, client)
	assert.Equal(t, domain.SourceTypeSemanticScholar, client.SourceType())
	assert.Equal(t, "Semantic Scholar", client.Name())
	assert.True(t, client.IsEnabled())
}

func TestClient_Search(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/paper/search", r.URL.Path)
		assert.Equal(t, "CRISPR", r.URL.Query().Get("query"))

		resp := SearchResponse{
			Total:  100,
			Offset: 0,
			Next:   10,
			Data: []PaperResult{
				{
					PaperID: "abc123",
					Title:   "CRISPR-Cas9 Gene Editing",
					Abstract: "A comprehensive review of CRISPR technology.",
					Year:    2023,
					ExternalIDs: &ExternalIDs{
						DOI:    "10.1234/example",
						PubMed: "12345678",
					},
					Authors: []Author{
						{Name: "John Doe"},
						{Name: "Jane Smith"},
					},
					CitationCount:  50,
					ReferenceCount: 30,
					IsOpenAccess:   true,
					OpenAccessPDF: &OpenAccessPDF{
						URL: "https://example.com/paper.pdf",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:   server.URL,
		Timeout:   30 * time.Second,
		RateLimit: 100.0,
		Enabled:   true,
	})

	result, err := client.Search(context.Background(), papersources.SearchParams{
		Query:      "CRISPR",
		MaxResults: 10,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 100, result.TotalResults)
	assert.True(t, result.HasMore)
	assert.Len(t, result.Papers, 1)

	paper := result.Papers[0]
	assert.Equal(t, "CRISPR-Cas9 Gene Editing", paper.Title)
	assert.Equal(t, "doi:10.1234/example", paper.CanonicalID)
	assert.Equal(t, 50, paper.CitationCount)
	assert.True(t, paper.OpenAccess)
	assert.Len(t, paper.Authors, 2)
}

func TestClient_GetByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/paper/abc123", r.URL.Path)

		resp := PaperResult{
			PaperID:  "abc123",
			Title:    "Test Paper",
			Abstract: "Test abstract",
			Year:     2023,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:   server.URL,
		Timeout:   30 * time.Second,
		RateLimit: 100.0,
		Enabled:   true,
	})

	paper, err := client.GetByID(context.Background(), "abc123")
	require.NoError(t, err)
	assert.Equal(t, "Test Paper", paper.Title)
}
```

**Step 3: Run test to verify it fails**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/semanticscholar/... -run TestClient`
Expected: FAIL (NewClient undefined)

**Step 4: Write implementation**

Create `internal/papersources/semanticscholar/client.go`:

```go
package semanticscholar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
)

const (
	defaultBaseURL = "https://api.semanticscholar.org/graph/v1"
	defaultFields  = "paperId,externalIds,title,abstract,year,publicationDate,venue,journal,authors,citationCount,referenceCount,isOpenAccess,openAccessPdf"
)

// Config holds configuration for the Semantic Scholar client.
type Config struct {
	BaseURL    string
	APIKey     string
	Timeout    time.Duration
	RateLimit  float64
	BurstSize  int
	MaxResults int
	Enabled    bool
}

// Client implements PaperSource for Semantic Scholar.
type Client struct {
	httpClient *papersources.HTTPClient
	baseURL    string
	maxResults int
	enabled    bool
}

// NewClient creates a new Semantic Scholar client.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 10.0
	}
	if cfg.BurstSize == 0 {
		cfg.BurstSize = 5
	}
	if cfg.MaxResults == 0 {
		cfg.MaxResults = 100
	}

	httpCfg := papersources.HTTPClientConfig{
		Timeout:      cfg.Timeout,
		RateLimit:    cfg.RateLimit,
		BurstSize:    cfg.BurstSize,
		MaxRetries:   3,
		RetryDelay:   time.Second,
		UserAgent:    "Helixir-LiteratureReview/1.0",
		APIKey:       cfg.APIKey,
		APIKeyHeader: "x-api-key",
	}

	return &Client{
		httpClient: papersources.NewHTTPClient(httpCfg),
		baseURL:    cfg.BaseURL,
		maxResults: cfg.MaxResults,
		enabled:    cfg.Enabled,
	}
}

// Search searches for papers matching the given parameters.
func (c *Client) Search(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
	start := time.Now()

	// Build URL
	u, err := url.Parse(c.baseURL + "/paper/search")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	q := u.Query()
	q.Set("query", params.Query)
	q.Set("fields", defaultFields)

	limit := params.MaxResults
	if limit == 0 || limit > c.maxResults {
		limit = c.maxResults
	}
	q.Set("limit", strconv.Itoa(limit))

	if params.Offset > 0 {
		q.Set("offset", strconv.Itoa(params.Offset))
	}

	if params.DateFrom != nil {
		q.Set("year", fmt.Sprintf("%d-", params.DateFrom.Year()))
	}

	if params.OpenAccessOnly {
		q.Set("openAccessPdf", "")
	}

	u.RawQuery = q.Encode()

	// Make request
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Convert to domain papers
	papers := make([]*domain.Paper, 0, len(searchResp.Data))
	for _, p := range searchResp.Data {
		paper := c.toDomainPaper(&p)
		papers = append(papers, paper)
	}

	return &papersources.SearchResult{
		Papers:         papers,
		TotalResults:   searchResp.Total,
		HasMore:        searchResp.Next > 0,
		NextOffset:     searchResp.Next,
		Source:         domain.SourceTypeSemanticScholar,
		SearchDuration: time.Since(start),
	}, nil
}

// GetByID retrieves a paper by its Semantic Scholar ID.
func (c *Client) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	u := fmt.Sprintf("%s/paper/%s?fields=%s", c.baseURL, url.PathEscape(id), defaultFields)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.ErrNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var paperResp PaperResult
	if err := json.NewDecoder(resp.Body).Decode(&paperResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return c.toDomainPaper(&paperResp), nil
}

// SourceType returns the source type.
func (c *Client) SourceType() domain.SourceType {
	return domain.SourceTypeSemanticScholar
}

// Name returns the human-readable name.
func (c *Client) Name() string {
	return "Semantic Scholar"
}

// IsEnabled returns whether this source is enabled.
func (c *Client) IsEnabled() bool {
	return c.enabled
}

func (c *Client) toDomainPaper(p *PaperResult) *domain.Paper {
	paper := &domain.Paper{
		Title:          p.Title,
		Abstract:       p.Abstract,
		PublicationYear: p.Year,
		CitationCount:  p.CitationCount,
		ReferenceCount: p.ReferenceCount,
		OpenAccess:     p.IsOpenAccess,
		RawMetadata: map[string]interface{}{
			"semantic_scholar_id": p.PaperID,
		},
	}

	// Set venue/journal
	if p.Journal != nil {
		paper.Journal = p.Journal.Name
		paper.Volume = p.Journal.Volume
		paper.Pages = p.Journal.Pages
	}
	if p.Venue != "" {
		paper.Venue = p.Venue
	}

	// Convert authors
	authors := make([]domain.Author, 0, len(p.Authors))
	for _, a := range p.Authors {
		authors = append(authors, domain.Author{Name: a.Name})
	}
	paper.Authors = authors

	// Set PDF URL
	if p.OpenAccessPDF != nil && p.OpenAccessPDF.URL != "" {
		paper.PDFURL = p.OpenAccessPDF.URL
	}

	// Generate canonical ID from external IDs
	if p.ExternalIDs != nil {
		ids := domain.PaperIdentifiers{
			DOI:               p.ExternalIDs.DOI,
			ArXivID:           p.ExternalIDs.ArXiv,
			PubMedID:          p.ExternalIDs.PubMed,
			SemanticScholarID: p.PaperID,
		}
		paper.CanonicalID = domain.GenerateCanonicalID(ids)
	} else {
		paper.CanonicalID = domain.GenerateCanonicalID(domain.PaperIdentifiers{
			SemanticScholarID: p.PaperID,
		})
	}

	// Parse publication date
	if p.PublicationDate != "" {
		if t, err := time.Parse("2006-01-02", p.PublicationDate); err == nil {
			paper.PublicationDate = &t
		}
	}

	return paper
}

// Ensure Client implements PaperSource
var _ papersources.PaperSource = (*Client)(nil)
```

**Step 5: Run test to verify it passes**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/semanticscholar/...`
Expected: PASS

**Step 6: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/papersources/semanticscholar/
git commit -m "feat(papersources): add Semantic Scholar client"
```

---

## Task 5: Implement OpenAlex Client

**Files:**
- Create: `internal/papersources/openalex/client.go`
- Create: `internal/papersources/openalex/client_test.go`
- Create: `internal/papersources/openalex/types.go`

**Step 1: Write API response types**

Create `internal/papersources/openalex/types.go`:

```go
// Package openalex provides a client for the OpenAlex API.
package openalex

// SearchResponse represents the OpenAlex works search response.
type SearchResponse struct {
	Meta    Meta     `json:"meta"`
	Results []Work   `json:"results"`
}

// Meta contains pagination information.
type Meta struct {
	Count    int    `json:"count"`
	DBTime   int    `json:"db_response_time_ms"`
	Page     int    `json:"page"`
	PerPage  int    `json:"per_page"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// Work represents an OpenAlex work (paper).
type Work struct {
	ID                string        `json:"id"`
	DOI               string        `json:"doi,omitempty"`
	Title             string        `json:"title"`
	DisplayName       string        `json:"display_name"`
	PublicationYear   int           `json:"publication_year"`
	PublicationDate   string        `json:"publication_date,omitempty"`
	Type              string        `json:"type"`
	CitedByCount      int           `json:"cited_by_count"`
	IsOpenAccess      bool          `json:"is_oa"`
	OpenAccess        *OpenAccess   `json:"open_access,omitempty"`
	Authorships       []Authorship  `json:"authorships,omitempty"`
	PrimaryLocation   *Location     `json:"primary_location,omitempty"`
	Abstract          string        `json:"abstract_inverted_index,omitempty"`
	IDs               IDs           `json:"ids,omitempty"`
	ReferencedWorks   []string      `json:"referenced_works,omitempty"`
}

// OpenAccess contains open access information.
type OpenAccess struct {
	IsOA     bool   `json:"is_oa"`
	OAURL    string `json:"oa_url,omitempty"`
	OAStatus string `json:"oa_status,omitempty"`
}

// Authorship represents author information.
type Authorship struct {
	AuthorPosition string      `json:"author_position"`
	Author         AuthorInfo  `json:"author"`
	Institutions   []Institution `json:"institutions,omitempty"`
}

// AuthorInfo contains author details.
type AuthorInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Orcid       string `json:"orcid,omitempty"`
}

// Institution represents an institution.
type Institution struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// Location represents where a work is published.
type Location struct {
	Source  *Source `json:"source,omitempty"`
	PDFURL  string  `json:"pdf_url,omitempty"`
	Version string  `json:"version,omitempty"`
}

// Source represents a publication source.
type Source struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

// IDs contains various external identifiers.
type IDs struct {
	OpenAlex string `json:"openalex,omitempty"`
	DOI      string `json:"doi,omitempty"`
	MAG      string `json:"mag,omitempty"`
	PMID     string `json:"pmid,omitempty"`
	PMCID    string `json:"pmcid,omitempty"`
}
```

**Step 2: Write the failing test**

Create `internal/papersources/openalex/client_test.go`:

```go
package openalex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := NewClient(Config{
		BaseURL:   "https://api.openalex.org",
		Timeout:   30 * time.Second,
		RateLimit: 10.0,
		Enabled:   true,
	})
	require.NotNil(t, client)
	assert.Equal(t, domain.SourceTypeOpenAlex, client.SourceType())
	assert.Equal(t, "OpenAlex", client.Name())
}

func TestClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/works", r.URL.Path)
		assert.Contains(t, r.URL.Query().Get("search"), "CRISPR")

		resp := SearchResponse{
			Meta: Meta{
				Count:   500,
				Page:    1,
				PerPage: 25,
			},
			Results: []Work{
				{
					ID:              "W123456",
					DOI:             "https://doi.org/10.1234/example",
					Title:           "CRISPR Gene Editing Review",
					DisplayName:     "CRISPR Gene Editing Review",
					PublicationYear: 2023,
					CitedByCount:    100,
					IsOpenAccess:    true,
					Authorships: []Authorship{
						{
							Author: AuthorInfo{
								DisplayName: "John Doe",
								Orcid:       "0000-0001-2345-6789",
							},
						},
					},
					OpenAccess: &OpenAccess{
						IsOA:  true,
						OAURL: "https://example.com/paper.pdf",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:   server.URL,
		Timeout:   30 * time.Second,
		RateLimit: 100.0,
		Enabled:   true,
	})

	result, err := client.Search(context.Background(), papersources.SearchParams{
		Query:      "CRISPR",
		MaxResults: 25,
	})

	require.NoError(t, err)
	assert.Equal(t, 500, result.TotalResults)
	assert.Len(t, result.Papers, 1)
	assert.Equal(t, "CRISPR Gene Editing Review", result.Papers[0].Title)
}
```

**Step 3: Run test to verify it fails**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/openalex/... -run TestClient`
Expected: FAIL (NewClient undefined)

**Step 4: Write implementation**

Create `internal/papersources/openalex/client.go`:

```go
package openalex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
)

const (
	defaultBaseURL = "https://api.openalex.org"
)

// Config holds configuration for the OpenAlex client.
type Config struct {
	BaseURL    string
	Email      string // Polite pool - provide email for higher rate limits
	Timeout    time.Duration
	RateLimit  float64
	BurstSize  int
	MaxResults int
	Enabled    bool
}

// Client implements PaperSource for OpenAlex.
type Client struct {
	httpClient *papersources.HTTPClient
	baseURL    string
	email      string
	maxResults int
	enabled    bool
}

// NewClient creates a new OpenAlex client.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 10.0
	}
	if cfg.BurstSize == 0 {
		cfg.BurstSize = 5
	}
	if cfg.MaxResults == 0 {
		cfg.MaxResults = 200
	}

	httpCfg := papersources.HTTPClientConfig{
		Timeout:    cfg.Timeout,
		RateLimit:  cfg.RateLimit,
		BurstSize:  cfg.BurstSize,
		MaxRetries: 3,
		RetryDelay: time.Second,
		UserAgent:  "Helixir-LiteratureReview/1.0 (mailto:" + cfg.Email + ")",
	}

	return &Client{
		httpClient: papersources.NewHTTPClient(httpCfg),
		baseURL:    cfg.BaseURL,
		email:      cfg.Email,
		maxResults: cfg.MaxResults,
		enabled:    cfg.Enabled,
	}
}

// Search searches for papers matching the given parameters.
func (c *Client) Search(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
	start := time.Now()

	u, err := url.Parse(c.baseURL + "/works")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	q := u.Query()
	q.Set("search", params.Query)

	limit := params.MaxResults
	if limit == 0 || limit > c.maxResults {
		limit = c.maxResults
	}
	q.Set("per_page", strconv.Itoa(limit))

	if params.Offset > 0 {
		page := (params.Offset / limit) + 1
		q.Set("page", strconv.Itoa(page))
	}

	// Build filter
	var filters []string
	if params.DateFrom != nil {
		filters = append(filters, fmt.Sprintf("from_publication_date:%s", params.DateFrom.Format("2006-01-02")))
	}
	if params.DateTo != nil {
		filters = append(filters, fmt.Sprintf("to_publication_date:%s", params.DateTo.Format("2006-01-02")))
	}
	if params.OpenAccessOnly {
		filters = append(filters, "is_oa:true")
	}
	if params.MinCitations > 0 {
		filters = append(filters, fmt.Sprintf("cited_by_count:>%d", params.MinCitations))
	}
	if len(filters) > 0 {
		q.Set("filter", strings.Join(filters, ","))
	}

	if c.email != "" {
		q.Set("mailto", c.email)
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	papers := make([]*domain.Paper, 0, len(searchResp.Results))
	for _, w := range searchResp.Results {
		papers = append(papers, c.toDomainPaper(&w))
	}

	hasMore := searchResp.Meta.Count > (searchResp.Meta.Page * searchResp.Meta.PerPage)

	return &papersources.SearchResult{
		Papers:         papers,
		TotalResults:   searchResp.Meta.Count,
		HasMore:        hasMore,
		NextOffset:     searchResp.Meta.Page * searchResp.Meta.PerPage,
		Source:         domain.SourceTypeOpenAlex,
		SearchDuration: time.Since(start),
	}, nil
}

// GetByID retrieves a paper by its OpenAlex ID.
func (c *Client) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	u := fmt.Sprintf("%s/works/%s", c.baseURL, url.PathEscape(id))
	if c.email != "" {
		u += "?mailto=" + url.QueryEscape(c.email)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var work Work
	if err := json.NewDecoder(resp.Body).Decode(&work); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return c.toDomainPaper(&work), nil
}

// SourceType returns the source type.
func (c *Client) SourceType() domain.SourceType {
	return domain.SourceTypeOpenAlex
}

// Name returns the human-readable name.
func (c *Client) Name() string {
	return "OpenAlex"
}

// IsEnabled returns whether this source is enabled.
func (c *Client) IsEnabled() bool {
	return c.enabled
}

func (c *Client) toDomainPaper(w *Work) *domain.Paper {
	paper := &domain.Paper{
		Title:           w.Title,
		PublicationYear: w.PublicationYear,
		CitationCount:   w.CitedByCount,
		OpenAccess:      w.IsOpenAccess,
		ReferenceCount:  len(w.ReferencedWorks),
		RawMetadata: map[string]interface{}{
			"openalex_id": w.ID,
			"type":        w.Type,
		},
	}

	// Extract DOI
	doi := w.DOI
	if doi != "" {
		doi = strings.TrimPrefix(doi, "https://doi.org/")
	}

	// Generate canonical ID
	ids := domain.PaperIdentifiers{
		DOI:        doi,
		OpenAlexID: strings.TrimPrefix(w.ID, "https://openalex.org/"),
	}
	if w.IDs.PMID != "" {
		ids.PubMedID = w.IDs.PMID
	}
	paper.CanonicalID = domain.GenerateCanonicalID(ids)

	// Authors
	authors := make([]domain.Author, 0, len(w.Authorships))
	for _, a := range w.Authorships {
		author := domain.Author{
			Name:  a.Author.DisplayName,
			ORCID: a.Author.Orcid,
		}
		if len(a.Institutions) > 0 {
			author.Affiliation = a.Institutions[0].DisplayName
		}
		authors = append(authors, author)
	}
	paper.Authors = authors

	// Location/Journal
	if w.PrimaryLocation != nil {
		if w.PrimaryLocation.Source != nil {
			paper.Journal = w.PrimaryLocation.Source.DisplayName
			paper.Venue = w.PrimaryLocation.Source.DisplayName
		}
		if w.PrimaryLocation.PDFURL != "" {
			paper.PDFURL = w.PrimaryLocation.PDFURL
		}
	}

	// Open access PDF
	if paper.PDFURL == "" && w.OpenAccess != nil && w.OpenAccess.OAURL != "" {
		paper.PDFURL = w.OpenAccess.OAURL
	}

	// Publication date
	if w.PublicationDate != "" {
		if t, err := time.Parse("2006-01-02", w.PublicationDate); err == nil {
			paper.PublicationDate = &t
		}
	}

	return paper
}

var _ papersources.PaperSource = (*Client)(nil)
```

**Step 5: Run test to verify it passes**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/openalex/...`
Expected: PASS

**Step 6: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/papersources/openalex/
git commit -m "feat(papersources): add OpenAlex client"
```

---

## Task 6: Implement PubMed Client

**Files:**
- Create: `internal/papersources/pubmed/client.go`
- Create: `internal/papersources/pubmed/client_test.go`
- Create: `internal/papersources/pubmed/types.go`

**Step 1: Write API response types**

Create `internal/papersources/pubmed/types.go`:

```go
// Package pubmed provides a client for the PubMed E-utilities API.
package pubmed

import "encoding/xml"

// ESearchResult represents the esearch response.
type ESearchResult struct {
	XMLName    xml.Name `xml:"eSearchResult"`
	Count      int      `xml:"Count"`
	RetMax     int      `xml:"RetMax"`
	RetStart   int      `xml:"RetStart"`
	IDList     IDList   `xml:"IdList"`
	QueryKey   string   `xml:"QueryKey,omitempty"`
	WebEnv     string   `xml:"WebEnv,omitempty"`
}

// IDList contains PubMed IDs.
type IDList struct {
	IDs []string `xml:"Id"`
}

// PubmedArticleSet represents the efetch response.
type PubmedArticleSet struct {
	XMLName  xml.Name        `xml:"PubmedArticleSet"`
	Articles []PubmedArticle `xml:"PubmedArticle"`
}

// PubmedArticle represents a single article.
type PubmedArticle struct {
	MedlineCitation MedlineCitation `xml:"MedlineCitation"`
	PubmedData      PubmedData      `xml:"PubmedData"`
}

// MedlineCitation contains the main article data.
type MedlineCitation struct {
	PMID           PMID           `xml:"PMID"`
	Article        Article        `xml:"Article"`
	MeshHeadingList *MeshHeadingList `xml:"MeshHeadingList,omitempty"`
	KeywordList    *KeywordList   `xml:"KeywordList,omitempty"`
}

// PMID represents the PubMed ID.
type PMID struct {
	Value   string `xml:",chardata"`
	Version string `xml:"Version,attr,omitempty"`
}

// Article contains article details.
type Article struct {
	Journal         Journal         `xml:"Journal"`
	ArticleTitle    string          `xml:"ArticleTitle"`
	Abstract        *Abstract       `xml:"Abstract,omitempty"`
	AuthorList      *AuthorList     `xml:"AuthorList,omitempty"`
	ArticleDate     []ArticleDate   `xml:"ArticleDate,omitempty"`
	ELocationID     []ELocationID   `xml:"ELocationID,omitempty"`
}

// Journal contains journal information.
type Journal struct {
	Title           string          `xml:"Title"`
	ISOAbbreviation string          `xml:"ISOAbbreviation"`
	JournalIssue    JournalIssue    `xml:"JournalIssue"`
}

// JournalIssue contains issue details.
type JournalIssue struct {
	Volume  string   `xml:"Volume"`
	Issue   string   `xml:"Issue"`
	PubDate PubDate  `xml:"PubDate"`
}

// PubDate contains publication date.
type PubDate struct {
	Year  string `xml:"Year"`
	Month string `xml:"Month,omitempty"`
	Day   string `xml:"Day,omitempty"`
}

// Abstract contains the abstract text.
type Abstract struct {
	AbstractTexts []AbstractText `xml:"AbstractText"`
}

// AbstractText contains a section of the abstract.
type AbstractText struct {
	Label string `xml:"Label,attr,omitempty"`
	Text  string `xml:",chardata"`
}

// AuthorList contains authors.
type AuthorList struct {
	Authors []Author `xml:"Author"`
}

// Author represents an author.
type Author struct {
	LastName    string       `xml:"LastName"`
	ForeName    string       `xml:"ForeName"`
	Initials    string       `xml:"Initials"`
	Identifier  []Identifier `xml:"Identifier,omitempty"`
	Affiliation string       `xml:"AffiliationInfo>Affiliation,omitempty"`
}

// Identifier represents an author identifier like ORCID.
type Identifier struct {
	Source string `xml:"Source,attr"`
	Value  string `xml:",chardata"`
}

// ArticleDate contains article dates.
type ArticleDate struct {
	DateType string `xml:"DateType,attr"`
	Year     string `xml:"Year"`
	Month    string `xml:"Month"`
	Day      string `xml:"Day"`
}

// ELocationID contains electronic location identifiers.
type ELocationID struct {
	EIdType string `xml:"EIdType,attr"`
	Value   string `xml:",chardata"`
}

// PubmedData contains additional PubMed data.
type PubmedData struct {
	ArticleIdList ArticleIdList `xml:"ArticleIdList"`
}

// ArticleIdList contains article IDs.
type ArticleIdList struct {
	ArticleIds []ArticleId `xml:"ArticleId"`
}

// ArticleId represents an article identifier.
type ArticleId struct {
	IdType string `xml:"IdType,attr"`
	Value  string `xml:",chardata"`
}

// MeshHeadingList contains MeSH terms.
type MeshHeadingList struct {
	MeshHeadings []MeshHeading `xml:"MeshHeading"`
}

// MeshHeading represents a MeSH term.
type MeshHeading struct {
	DescriptorName DescriptorName `xml:"DescriptorName"`
}

// DescriptorName contains the MeSH term name.
type DescriptorName struct {
	UI   string `xml:"UI,attr"`
	Name string `xml:",chardata"`
}

// KeywordList contains keywords.
type KeywordList struct {
	Keywords []Keyword `xml:"Keyword"`
}

// Keyword represents a keyword.
type Keyword struct {
	Value string `xml:",chardata"`
}
```

**Step 2: Write the failing test**

Create `internal/papersources/pubmed/client_test.go`:

```go
package pubmed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := NewClient(Config{
		BaseURL:   "https://eutils.ncbi.nlm.nih.gov/entrez/eutils",
		Timeout:   30 * time.Second,
		RateLimit: 3.0,
		Enabled:   true,
	})
	require.NotNil(t, client)
	assert.Equal(t, domain.SourceTypePubMed, client.SourceType())
	assert.Equal(t, "PubMed", client.Name())
}

func TestClient_Search(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// ESearch response
			assert.Contains(t, r.URL.Path, "esearch.fcgi")
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<eSearchResult>
  <Count>100</Count>
  <RetMax>10</RetMax>
  <RetStart>0</RetStart>
  <IdList>
    <Id>12345678</Id>
  </IdList>
</eSearchResult>`))
		} else {
			// EFetch response
			assert.Contains(t, r.URL.Path, "efetch.fcgi")
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<PubmedArticleSet>
  <PubmedArticle>
    <MedlineCitation>
      <PMID>12345678</PMID>
      <Article>
        <Journal>
          <Title>Nature</Title>
          <JournalIssue>
            <Volume>600</Volume>
            <PubDate><Year>2023</Year></PubDate>
          </JournalIssue>
        </Journal>
        <ArticleTitle>CRISPR Gene Editing Advances</ArticleTitle>
        <Abstract>
          <AbstractText>This paper discusses CRISPR advances.</AbstractText>
        </Abstract>
        <AuthorList>
          <Author><LastName>Smith</LastName><ForeName>John</ForeName></Author>
        </AuthorList>
        <ELocationID EIdType="doi">10.1038/example</ELocationID>
      </Article>
    </MedlineCitation>
    <PubmedData>
      <ArticleIdList>
        <ArticleId IdType="pubmed">12345678</ArticleId>
        <ArticleId IdType="doi">10.1038/example</ArticleId>
      </ArticleIdList>
    </PubmedData>
  </PubmedArticle>
</PubmedArticleSet>`))
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:   server.URL,
		Timeout:   30 * time.Second,
		RateLimit: 100.0,
		Enabled:   true,
	})

	result, err := client.Search(context.Background(), papersources.SearchParams{
		Query:      "CRISPR",
		MaxResults: 10,
	})

	require.NoError(t, err)
	assert.Equal(t, 100, result.TotalResults)
	require.Len(t, result.Papers, 1)
	assert.Equal(t, "CRISPR Gene Editing Advances", result.Papers[0].Title)
	assert.Contains(t, result.Papers[0].CanonicalID, "doi:")
}
```

**Step 3: Run test to verify it fails**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/pubmed/... -run TestClient`
Expected: FAIL (NewClient undefined)

**Step 4: Write implementation**

Create `internal/papersources/pubmed/client.go`:

```go
package pubmed

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/papersources"
)

const (
	defaultBaseURL = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"
)

// Config holds configuration for the PubMed client.
type Config struct {
	BaseURL    string
	APIKey     string
	Timeout    time.Duration
	RateLimit  float64
	BurstSize  int
	MaxResults int
	Enabled    bool
}

// Client implements PaperSource for PubMed.
type Client struct {
	httpClient *papersources.HTTPClient
	baseURL    string
	apiKey     string
	maxResults int
	enabled    bool
}

// NewClient creates a new PubMed client.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 3.0 // NCBI recommends max 3 req/sec without API key
	}
	if cfg.BurstSize == 0 {
		cfg.BurstSize = 3
	}
	if cfg.MaxResults == 0 {
		cfg.MaxResults = 100
	}

	httpCfg := papersources.HTTPClientConfig{
		Timeout:    cfg.Timeout,
		RateLimit:  cfg.RateLimit,
		BurstSize:  cfg.BurstSize,
		MaxRetries: 3,
		RetryDelay: time.Second,
		UserAgent:  "Helixir-LiteratureReview/1.0",
	}

	return &Client{
		httpClient: papersources.NewHTTPClient(httpCfg),
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		maxResults: cfg.MaxResults,
		enabled:    cfg.Enabled,
	}
}

// Search searches PubMed using esearch + efetch.
func (c *Client) Search(ctx context.Context, params papersources.SearchParams) (*papersources.SearchResult, error) {
	start := time.Now()

	// Step 1: ESearch to get PMIDs
	ids, total, err := c.esearch(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("esearch: %w", err)
	}

	if len(ids) == 0 {
		return &papersources.SearchResult{
			Papers:         []*domain.Paper{},
			TotalResults:   total,
			HasMore:        false,
			Source:         domain.SourceTypePubMed,
			SearchDuration: time.Since(start),
		}, nil
	}

	// Step 2: EFetch to get full article data
	papers, err := c.efetch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("efetch: %w", err)
	}

	limit := params.MaxResults
	if limit == 0 || limit > c.maxResults {
		limit = c.maxResults
	}

	return &papersources.SearchResult{
		Papers:         papers,
		TotalResults:   total,
		HasMore:        total > params.Offset+len(papers),
		NextOffset:     params.Offset + len(papers),
		Source:         domain.SourceTypePubMed,
		SearchDuration: time.Since(start),
	}, nil
}

func (c *Client) esearch(ctx context.Context, params papersources.SearchParams) ([]string, int, error) {
	u, _ := url.Parse(c.baseURL + "/esearch.fcgi")
	q := u.Query()
	q.Set("db", "pubmed")
	q.Set("term", params.Query)
	q.Set("retmode", "xml")

	limit := params.MaxResults
	if limit == 0 || limit > c.maxResults {
		limit = c.maxResults
	}
	q.Set("retmax", strconv.Itoa(limit))

	if params.Offset > 0 {
		q.Set("retstart", strconv.Itoa(params.Offset))
	}

	// Date filters
	if params.DateFrom != nil {
		q.Set("mindate", params.DateFrom.Format("2006/01/02"))
	}
	if params.DateTo != nil {
		q.Set("maxdate", params.DateTo.Format("2006/01/02"))
	}
	if params.DateFrom != nil || params.DateTo != nil {
		q.Set("datetype", "pdat")
	}

	if c.apiKey != "" {
		q.Set("api_key", c.apiKey)
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	var result ESearchResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, err
	}

	return result.IDList.IDs, result.Count, nil
}

func (c *Client) efetch(ctx context.Context, ids []string) ([]*domain.Paper, error) {
	u, _ := url.Parse(c.baseURL + "/efetch.fcgi")
	q := u.Query()
	q.Set("db", "pubmed")
	q.Set("id", strings.Join(ids, ","))
	q.Set("retmode", "xml")
	q.Set("rettype", "abstract")

	if c.apiKey != "" {
		q.Set("api_key", c.apiKey)
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var articleSet PubmedArticleSet
	if err := xml.NewDecoder(resp.Body).Decode(&articleSet); err != nil {
		return nil, err
	}

	papers := make([]*domain.Paper, 0, len(articleSet.Articles))
	for _, article := range articleSet.Articles {
		papers = append(papers, c.toDomainPaper(&article))
	}

	return papers, nil
}

// GetByID retrieves a paper by its PubMed ID.
func (c *Client) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	papers, err := c.efetch(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	if len(papers) == 0 {
		return nil, domain.ErrNotFound
	}
	return papers[0], nil
}

// SourceType returns the source type.
func (c *Client) SourceType() domain.SourceType {
	return domain.SourceTypePubMed
}

// Name returns the human-readable name.
func (c *Client) Name() string {
	return "PubMed"
}

// IsEnabled returns whether this source is enabled.
func (c *Client) IsEnabled() bool {
	return c.enabled
}

func (c *Client) toDomainPaper(article *PubmedArticle) *domain.Paper {
	mc := &article.MedlineCitation
	a := &mc.Article

	paper := &domain.Paper{
		Title:   a.ArticleTitle,
		Journal: a.Journal.Title,
		Volume:  a.Journal.JournalIssue.Volume,
		RawMetadata: map[string]interface{}{
			"pubmed_id": mc.PMID.Value,
		},
	}

	// Abstract
	if a.Abstract != nil {
		var parts []string
		for _, at := range a.Abstract.AbstractTexts {
			if at.Label != "" {
				parts = append(parts, at.Label+": "+at.Text)
			} else {
				parts = append(parts, at.Text)
			}
		}
		paper.Abstract = strings.Join(parts, " ")
	}

	// Authors
	if a.AuthorList != nil {
		authors := make([]domain.Author, 0, len(a.AuthorList.Authors))
		for _, auth := range a.AuthorList.Authors {
			author := domain.Author{
				Name:        auth.ForeName + " " + auth.LastName,
				Affiliation: auth.Affiliation,
			}
			for _, id := range auth.Identifier {
				if id.Source == "ORCID" {
					author.ORCID = id.Value
				}
			}
			authors = append(authors, author)
		}
		paper.Authors = authors
	}

	// Publication year
	if year, err := strconv.Atoi(a.Journal.JournalIssue.PubDate.Year); err == nil {
		paper.PublicationYear = year
	}

	// External IDs
	ids := domain.PaperIdentifiers{
		PubMedID: mc.PMID.Value,
	}

	// DOI from ELocationID
	for _, eloc := range a.ELocationID {
		if eloc.EIdType == "doi" {
			ids.DOI = eloc.Value
		}
	}

	// DOI from ArticleIdList
	for _, aid := range article.PubmedData.ArticleIdList.ArticleIds {
		if aid.IdType == "doi" && ids.DOI == "" {
			ids.DOI = aid.Value
		}
		if aid.IdType == "pmc" {
			ids.PMCID = aid.Value
		}
	}

	paper.CanonicalID = domain.GenerateCanonicalID(ids)

	return paper
}

var _ papersources.PaperSource = (*Client)(nil)
```

**Step 5: Run test to verify it passes**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/pubmed/...`
Expected: PASS

**Step 6: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/papersources/pubmed/
git commit -m "feat(papersources): add PubMed E-utilities client"
```

---

## Task 7: Implement Source Registry

**Files:**
- Create: `internal/papersources/registry.go`
- Create: `internal/papersources/registry_test.go`

**Step 1: Write the failing test**

Create `internal/papersources/registry_test.go`:

```go
package papersources

import (
	"context"
	"testing"
	"time"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSource struct {
	sourceType domain.SourceType
	name       string
	enabled    bool
	searchFn   func(ctx context.Context, params SearchParams) (*SearchResult, error)
}

func (m *mockSource) Search(ctx context.Context, params SearchParams) (*SearchResult, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, params)
	}
	return &SearchResult{
		Papers:       []*domain.Paper{{Title: "Test from " + m.name}},
		TotalResults: 1,
		Source:       m.sourceType,
	}, nil
}

func (m *mockSource) GetByID(ctx context.Context, id string) (*domain.Paper, error) {
	return &domain.Paper{Title: "Test"}, nil
}

func (m *mockSource) SourceType() domain.SourceType { return m.sourceType }
func (m *mockSource) Name() string                  { return m.name }
func (m *mockSource) IsEnabled() bool               { return m.enabled }

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	require.NotNil(t, reg)
}

func TestRegistry_Register(t *testing.T) {
	reg := NewRegistry()

	source := &mockSource{
		sourceType: domain.SourceTypeSemanticScholar,
		name:       "Test Source",
		enabled:    true,
	}

	reg.Register(source)

	sources := reg.EnabledSources()
	assert.Len(t, sources, 1)
	assert.Equal(t, domain.SourceTypeSemanticScholar, sources[0].SourceType())
}

func TestRegistry_Get(t *testing.T) {
	reg := NewRegistry()

	source := &mockSource{
		sourceType: domain.SourceTypeOpenAlex,
		name:       "OpenAlex",
		enabled:    true,
	}
	reg.Register(source)

	got := reg.Get(domain.SourceTypeOpenAlex)
	require.NotNil(t, got)
	assert.Equal(t, "OpenAlex", got.Name())

	notFound := reg.Get(domain.SourceTypePubMed)
	assert.Nil(t, notFound)
}

func TestRegistry_SearchAll(t *testing.T) {
	reg := NewRegistry()

	reg.Register(&mockSource{
		sourceType: domain.SourceTypeSemanticScholar,
		name:       "S2",
		enabled:    true,
	})
	reg.Register(&mockSource{
		sourceType: domain.SourceTypeOpenAlex,
		name:       "OA",
		enabled:    true,
	})
	reg.Register(&mockSource{
		sourceType: domain.SourceTypePubMed,
		name:       "PM",
		enabled:    false, // Disabled
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := reg.SearchAll(ctx, SearchParams{Query: "test"})

	// Should have results from 2 enabled sources
	assert.Len(t, results, 2)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/... -run TestRegistry`
Expected: FAIL (NewRegistry undefined)

**Step 3: Write implementation**

Create `internal/papersources/registry.go`:

```go
package papersources

import (
	"context"
	"sync"

	"github.com/helixir/literature-review-service/internal/domain"
)

// SourceResult holds the result of a search from one source.
type SourceResult struct {
	Source domain.SourceType
	Result *SearchResult
	Error  error
}

// Registry manages paper sources.
type Registry struct {
	mu      sync.RWMutex
	sources map[domain.SourceType]PaperSource
}

// NewRegistry creates a new source registry.
func NewRegistry() *Registry {
	return &Registry{
		sources: make(map[domain.SourceType]PaperSource),
	}
}

// Register adds a source to the registry.
func (r *Registry) Register(source PaperSource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources[source.SourceType()] = source
}

// Get returns a source by type.
func (r *Registry) Get(sourceType domain.SourceType) PaperSource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sources[sourceType]
}

// AllSources returns all registered sources.
func (r *Registry) AllSources() []PaperSource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sources := make([]PaperSource, 0, len(r.sources))
	for _, s := range r.sources {
		sources = append(sources, s)
	}
	return sources
}

// EnabledSources returns only enabled sources.
func (r *Registry) EnabledSources() []PaperSource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sources := make([]PaperSource, 0, len(r.sources))
	for _, s := range r.sources {
		if s.IsEnabled() {
			sources = append(sources, s)
		}
	}
	return sources
}

// SearchAll searches all enabled sources concurrently.
func (r *Registry) SearchAll(ctx context.Context, params SearchParams) []SourceResult {
	sources := r.EnabledSources()
	if len(sources) == 0 {
		return nil
	}

	results := make([]SourceResult, len(sources))
	var wg sync.WaitGroup

	for i, source := range sources {
		wg.Add(1)
		go func(idx int, src PaperSource) {
			defer wg.Done()

			result, err := src.Search(ctx, params)
			results[idx] = SourceResult{
				Source: src.SourceType(),
				Result: result,
				Error:  err,
			}
		}(i, source)
	}

	wg.Wait()
	return results
}

// SearchSources searches specific sources concurrently.
func (r *Registry) SearchSources(ctx context.Context, params SearchParams, sourceTypes []domain.SourceType) []SourceResult {
	if len(sourceTypes) == 0 {
		return r.SearchAll(ctx, params)
	}

	results := make([]SourceResult, 0, len(sourceTypes))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, st := range sourceTypes {
		source := r.Get(st)
		if source == nil || !source.IsEnabled() {
			continue
		}

		wg.Add(1)
		go func(src PaperSource) {
			defer wg.Done()

			result, err := src.Search(ctx, params)
			mu.Lock()
			results = append(results, SourceResult{
				Source: src.SourceType(),
				Result: result,
				Error:  err,
			})
			mu.Unlock()
		}(source)
	}

	wg.Wait()
	return results
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && go test -v ./internal/papersources/... -run TestRegistry`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service
git add internal/papersources/registry.go internal/papersources/registry_test.go
git commit -m "feat(papersources): add source registry for concurrent searches"
```

---

## Task 8: Run Full Test Suite

**Step 1: Run all tests**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && make test`
Expected: All tests pass

**Step 2: Run linter**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && make lint`
Expected: No errors

**Step 3: Verify build**

Run: `cd /Users/mehdiyazdani/dev/VsCodeProjects/Helixir/literature_service && make build`
Expected: Build succeeds

---

## Summary

Phase 3A implements the paper source clients layer:

1. **PaperSource Interface** - Common contract for all sources
2. **Rate Limiter** - Token bucket rate limiting
3. **HTTP Client** - Shared HTTP client with retries
4. **Semantic Scholar** - Full-featured client
5. **OpenAlex** - Full-featured client
6. **PubMed** - E-utilities client (esearch + efetch)
7. **Source Registry** - Concurrent search across sources

**Not included (future tasks):**
- arXiv client
- bioRxiv client
- Scopus client (requires API key)
