package papersources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// HTTPClientConfig configures the HTTP client.
type HTTPClientConfig struct {
	// Timeout is the request timeout for HTTP operations.
	Timeout time.Duration

	// RateLimit is the maximum requests per second.
	RateLimit float64

	// BurstSize is the maximum burst of requests allowed.
	BurstSize int

	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int

	// RetryDelay is the base delay between retries.
	RetryDelay time.Duration

	// UserAgent is the User-Agent header sent with requests.
	UserAgent string

	// APIKey is an optional API key for authentication.
	APIKey string

	// APIKeyHeader is the header name for the API key (e.g., "X-API-Key", "Authorization").
	APIKeyHeader string
}

// HTTPClient wraps http.Client with rate limiting and retries.
// It is safe for concurrent use.
type HTTPClient struct {
	client      *http.Client
	rateLimiter *RateLimiter
	config      HTTPClientConfig
}

// NewHTTPClient creates a new HTTP client with rate limiting.
// The client applies rate limiting before each request and automatically
// retries on 429 (Too Many Requests) and 5xx server errors.
func NewHTTPClient(cfg HTTPClientConfig) *HTTPClient {
	// Apply defaults
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 10
	}
	if cfg.BurstSize == 0 {
		cfg.BurstSize = 10
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Helixir-LiteratureService/1.0"
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
// It waits for the rate limiter before each request attempt,
// sets the User-Agent and optional API key headers,
// and retries on 429 (Too Many Requests) with Retry-After support
// and on 5xx server errors.
//
// The request body is not preserved across retries; callers must provide
// requests with GetBody set if the body needs to be resent on retry.
func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Set default headers
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.config.UserAgent)
	}

	// Set API key if configured
	if c.config.APIKey != "" && c.config.APIKeyHeader != "" {
		req.Header.Set(c.config.APIKeyHeader, c.config.APIKey)
	}

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		// Wait for rate limiter
		if err := c.rateLimiter.Wait(req.Context()); err != nil {
			return nil, fmt.Errorf("rate limiter wait: %w", err)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			// Check for context cancellation
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			lastErr = fmt.Errorf("request failed: %w", err)
			// Continue to retry on network errors
			if attempt < c.config.MaxRetries {
				if err := c.waitForRetry(req.Context(), c.config.RetryDelay); err != nil {
					return nil, err
				}
				// Reset body if possible for retry
				if err := c.resetRequestBody(req); err != nil {
					return nil, fmt.Errorf("cannot retry request: %w", err)
				}
				continue
			}
			return nil, lastErr
		}

		// Check if we should retry based on status code
		if c.shouldRetry(resp.StatusCode) {
			retryDelay := c.getRetryDelay(resp)

			// Close the response body to free resources before retry
			if resp.Body != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}

			if attempt < c.config.MaxRetries {
				lastErr = fmt.Errorf("server returned status %d", resp.StatusCode)
				if err := c.waitForRetry(req.Context(), retryDelay); err != nil {
					return nil, err
				}
				// Reset body if possible for retry
				if err := c.resetRequestBody(req); err != nil {
					return nil, fmt.Errorf("cannot retry request: %w", err)
				}
				continue
			}

			// Max retries exhausted
			return nil, fmt.Errorf("max retries exhausted after %d attempts, last status: %d", c.config.MaxRetries+1, resp.StatusCode)
		}

		// Success or non-retryable error
		return resp, nil
	}

	// Should not reach here, but handle edge case
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("unexpected error: no response received")
}

// shouldRetry returns true if the status code indicates we should retry.
func (c *HTTPClient) shouldRetry(statusCode int) bool {
	// Retry on 429 Too Many Requests
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	// Retry on 5xx server errors
	return statusCode >= 500 && statusCode < 600
}

// getRetryDelay determines how long to wait before retrying.
// It respects the Retry-After header if present, otherwise uses the configured retry delay.
func (c *HTTPClient) getRetryDelay(resp *http.Response) time.Duration {
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return c.config.RetryDelay
	}

	// Try to parse as seconds
	if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil {
		if seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		return c.config.RetryDelay
	}

	// Try to parse as HTTP date
	if t, err := http.ParseTime(retryAfter); err == nil {
		delay := time.Until(t)
		if delay > 0 {
			return delay
		}
	}

	return c.config.RetryDelay
}

// waitForRetry waits for the specified duration, respecting context cancellation.
func (c *HTTPClient) waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// resetRequestBody resets the request body for retry if possible.
func (c *HTTPClient) resetRequestBody(req *http.Request) error {
	if req.Body == nil || req.GetBody == nil {
		return nil
	}

	body, err := req.GetBody()
	if err != nil {
		return fmt.Errorf("failed to get request body for retry: %w", err)
	}
	req.Body = body
	return nil
}
