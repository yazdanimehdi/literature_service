package papersources

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPClient(t *testing.T) {
	t.Run("creates client with custom config", func(t *testing.T) {
		cfg := HTTPClientConfig{
			Timeout:      15 * time.Second,
			RateLimit:    5,
			BurstSize:    3,
			MaxRetries:   2,
			RetryDelay:   500 * time.Millisecond,
			UserAgent:    "TestAgent/1.0",
			APIKey:       "test-key",
			APIKeyHeader: "X-API-Key",
		}

		client := NewHTTPClient(cfg)

		require.NotNil(t, client)
		require.NotNil(t, client.client)
		require.NotNil(t, client.rateLimiter)
		assert.Equal(t, 15*time.Second, client.client.Timeout)
		assert.Equal(t, cfg.UserAgent, client.config.UserAgent)
		assert.Equal(t, cfg.APIKey, client.config.APIKey)
		assert.Equal(t, cfg.APIKeyHeader, client.config.APIKeyHeader)
		assert.Equal(t, cfg.MaxRetries, client.config.MaxRetries)
	})

	t.Run("applies default values", func(t *testing.T) {
		client := NewHTTPClient(HTTPClientConfig{})

		require.NotNil(t, client)
		assert.Equal(t, 30*time.Second, client.client.Timeout)
		assert.Equal(t, "Helixir-LiteratureService/1.0", client.config.UserAgent)
		assert.Equal(t, 3, client.config.MaxRetries)
		assert.Equal(t, time.Second, client.config.RetryDelay)
		assert.Equal(t, float64(10), client.config.RateLimit)
		assert.Equal(t, 10, client.config.BurstSize)
	})
}

func TestHTTPClient_Do(t *testing.T) {
	t.Run("successful request with User-Agent", func(t *testing.T) {
		var receivedUserAgent string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedUserAgent = r.Header.Get("User-Agent")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			UserAgent: "TestAgent/2.0",
			RateLimit: 100, // High rate to avoid test delays
			BurstSize: 10,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "TestAgent/2.0", receivedUserAgent)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"status":"ok"}`, string(body))
	})

	t.Run("sets API key header when configured", func(t *testing.T) {
		var receivedAPIKey string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAPIKey = r.Header.Get("X-API-Key")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:    100,
			BurstSize:    10,
			APIKey:       "secret-key-123",
			APIKeyHeader: "X-API-Key",
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, "secret-key-123", receivedAPIKey)
	})

	t.Run("does not set API key when not configured", func(t *testing.T) {
		var receivedAPIKey string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAPIKey = r.Header.Get("X-API-Key")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit: 100,
			BurstSize: 10,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Empty(t, receivedAPIKey)
	})

	t.Run("preserves existing User-Agent header", func(t *testing.T) {
		var receivedUserAgent string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedUserAgent = r.Header.Get("User-Agent")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			UserAgent: "DefaultAgent/1.0",
			RateLimit: 100,
			BurstSize: 10,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "CustomAgent/3.0")

		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, "CustomAgent/3.0", receivedUserAgent)
	})
}

func TestHTTPClient_DoWithRateLimit(t *testing.T) {
	t.Run("respects rate limit", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Create client with 10 requests per second, burst of 2
		client := NewHTTPClient(HTTPClientConfig{
			RateLimit: 10,
			BurstSize: 2,
		})

		ctx := context.Background()
		start := time.Now()

		// Make 4 requests - first 2 should be instant (burst), next 2 should be rate limited
		for i := 0; i < 4; i++ {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()
		}

		elapsed := time.Since(start)
		// With 10 req/sec and burst of 2, requests 3 and 4 need to wait
		// At least 100ms for request 3 and 200ms total for both
		assert.GreaterOrEqual(t, elapsed, 150*time.Millisecond, "should have been rate limited")
		assert.Equal(t, int32(4), requestCount.Load())
	})
}

func TestHTTPClient_DoRetryOn429(t *testing.T) {
	t.Run("retries on 429 and succeeds", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			if count < 3 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, int32(3), requestCount.Load())
	})

	t.Run("respects Retry-After header as seconds", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			if count == 1 {
				w.Header().Set("Retry-After", "1") // 1 second
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond, // Default is shorter
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start)

		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		// Should have waited at least 1 second due to Retry-After
		assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond)
	})

	t.Run("respects Retry-After header as HTTP date", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			if count == 1 {
				// Set Retry-After as HTTP date 3 seconds in the future
				// Note: HTTP date format only has second-level precision,
				// so we use 3s and expect at least 1.5s delay to be robust
				retryTime := time.Now().Add(3 * time.Second)
				w.Header().Set("Retry-After", retryTime.UTC().Format(http.TimeFormat))
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start)

		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		// Should have waited at least 1.5 seconds.
		// Using lower bound since HTTP date format has second-level precision
		// and there's network latency in the test setup.
		assert.GreaterOrEqual(t, elapsed, 1500*time.Millisecond)
		// Also verify it didn't take too long (should be less than 5s)
		assert.Less(t, elapsed, 5*time.Second)
	})

	t.Run("fails after max retries on 429", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 2,
			RetryDelay: 10 * time.Millisecond,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "max retries exhausted")
		assert.Contains(t, err.Error(), "429")
		// Initial attempt + MaxRetries
		assert.Equal(t, int32(3), requestCount.Load())
	})
}

func TestHTTPClient_DoRetryOn5xx(t *testing.T) {
	t.Run("retries on 500 and succeeds", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			if count < 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, int32(2), requestCount.Load())
	})

	t.Run("retries on 502 Bad Gateway", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			if count < 2 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("retries on 503 Service Unavailable", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			if count < 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("retries on 504 Gateway Timeout", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			if count < 2 {
				w.WriteHeader(http.StatusGatewayTimeout)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("fails after max retries on 5xx", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 2,
			RetryDelay: 10 * time.Millisecond,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "max retries exhausted")
		assert.Contains(t, err.Error(), "500")
		assert.Equal(t, int32(3), requestCount.Load())
	})

	t.Run("does not retry on 4xx client errors", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Equal(t, int32(1), requestCount.Load())
	})

	t.Run("does not retry on 404 Not Found", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond,
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.Equal(t, int32(1), requestCount.Load())
	})
}

func TestHTTPClient_DoContextCanceled(t *testing.T) {
	t.Run("returns error when context is canceled before request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit: 100,
			BurstSize: 10,
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("returns error when context times out during rate limit wait", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Create client with very low rate to force waiting
		client := NewHTTPClient(HTTPClientConfig{
			RateLimit: 1,
			BurstSize: 1,
		})

		ctx := context.Background()

		// Exhaust the burst
		req1, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		resp, err := client.Do(req1)
		require.NoError(t, err)
		resp.Body.Close()

		// Now create a request with a short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		req2, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		resp, err = client.Do(req2)
		require.Error(t, err)
		assert.Nil(t, resp)
		// The error should indicate deadline/timeout
		assert.Contains(t, err.Error(), "deadline")
	})

	t.Run("returns error when context canceled during retry wait", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 5,
			RetryDelay: 1 * time.Second, // Long retry delay
		})

		ctx, cancel := context.WithCancel(context.Background())

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		require.NoError(t, err)

		// Cancel context after a short delay (after first request, during retry wait)
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		resp, err := client.Do(req)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, context.Canceled)
		// Should have made at least 1 request before being canceled
		assert.GreaterOrEqual(t, requestCount.Load(), int32(1))
	})
}

func TestHTTPClient_DoWithRequestBody(t *testing.T) {
	t.Run("retries POST request with body", func(t *testing.T) {
		var requestCount atomic.Int32
		var lastBody string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			body, _ := io.ReadAll(r.Body)
			lastBody = string(body)
			if count < 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{
			RateLimit:  100,
			BurstSize:  10,
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond,
		})

		bodyContent := `{"data":"test"}`
		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			server.URL,
			strings.NewReader(bodyContent),
		)
		require.NoError(t, err)

		// Set GetBody to enable body reset on retry
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(bodyContent)), nil
		}

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, bodyContent, lastBody)
		assert.Equal(t, int32(2), requestCount.Load())
	})
}

func TestHTTPClient_getRetryDelay(t *testing.T) {
	client := NewHTTPClient(HTTPClientConfig{
		RetryDelay: 500 * time.Millisecond,
	})

	t.Run("uses default when Retry-After is empty", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{},
		}
		delay := client.getRetryDelay(resp)
		assert.Equal(t, 500*time.Millisecond, delay)
	})

	t.Run("parses Retry-After as seconds", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{
				"Retry-After": []string{"5"},
			},
		}
		delay := client.getRetryDelay(resp)
		assert.Equal(t, 5*time.Second, delay)
	})

	t.Run("parses Retry-After as HTTP date", func(t *testing.T) {
		futureTime := time.Now().Add(10 * time.Second)
		resp := &http.Response{
			Header: http.Header{
				"Retry-After": []string{futureTime.UTC().Format(http.TimeFormat)},
			},
		}
		delay := client.getRetryDelay(resp)
		// Should be approximately 10 seconds
		assert.Greater(t, delay, 9*time.Second)
		assert.Less(t, delay, 11*time.Second)
	})

	t.Run("uses default for invalid Retry-After", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{
				"Retry-After": []string{"invalid"},
			},
		}
		delay := client.getRetryDelay(resp)
		assert.Equal(t, 500*time.Millisecond, delay)
	})

	t.Run("uses default for zero seconds", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{
				"Retry-After": []string{"0"},
			},
		}
		delay := client.getRetryDelay(resp)
		assert.Equal(t, 500*time.Millisecond, delay)
	})

	t.Run("uses default for negative seconds", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{
				"Retry-After": []string{"-5"},
			},
		}
		delay := client.getRetryDelay(resp)
		assert.Equal(t, 500*time.Millisecond, delay)
	})

	t.Run("uses default for past HTTP date", func(t *testing.T) {
		pastTime := time.Now().Add(-10 * time.Second)
		resp := &http.Response{
			Header: http.Header{
				"Retry-After": []string{pastTime.UTC().Format(http.TimeFormat)},
			},
		}
		delay := client.getRetryDelay(resp)
		assert.Equal(t, 500*time.Millisecond, delay)
	})
}

func TestHTTPClient_shouldRetry(t *testing.T) {
	client := NewHTTPClient(HTTPClientConfig{})

	testCases := []struct {
		statusCode  int
		shouldRetry bool
	}{
		{http.StatusOK, false},
		{http.StatusCreated, false},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},
		{599, true}, // Edge case: highest 5xx
	}

	for _, tc := range testCases {
		t.Run(http.StatusText(tc.statusCode), func(t *testing.T) {
			result := client.shouldRetry(tc.statusCode)
			assert.Equal(t, tc.shouldRetry, result, "status %d", tc.statusCode)
		})
	}
}
