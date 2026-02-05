// Package papersources provides clients for searching academic paper databases.
package papersources

import (
	"context"

	"golang.org/x/time/rate"
)

// RateLimiter wraps a token bucket rate limiter for controlling request rates
// to external APIs. It is safe for concurrent use because the underlying
// rate.Limiter is goroutine-safe for all operations.
type RateLimiter struct {
	limiter *rate.Limiter
}

// NewRateLimiter creates a new rate limiter.
// ratePerSecond is the sustained rate of requests per second.
// burst is the maximum burst size (number of tokens that can be consumed at once).
//
// Example configurations:
//   - PubMed: NewRateLimiter(3, 3) for 3 requests per second
//   - Semantic Scholar: NewRateLimiter(10, 10) for 10 requests per second
func NewRateLimiter(ratePerSecond float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(ratePerSecond), burst),
	}
}

// Wait blocks until a request is allowed or the context is canceled.
// It returns an error if the context is canceled or the deadline is exceeded.
func (r *RateLimiter) Wait(ctx context.Context) error {
	return r.limiter.Wait(ctx)
}

// Allow returns true if a request is allowed without waiting.
// It consumes one token if allowed, and returns false if no tokens are available.
func (r *RateLimiter) Allow() bool {
	return r.limiter.Allow()
}

// SetRate updates the rate limit while preserving the current burst size.
// This can be used to dynamically adjust the rate based on API responses
// (e.g., when receiving rate limit headers from the API).
func (r *RateLimiter) SetRate(ratePerSecond float64) {
	r.limiter.SetLimit(rate.Limit(ratePerSecond))
}

// SetBurst updates the burst size.
// The burst size determines the maximum number of requests that can be made
// at once when tokens are available.
func (r *RateLimiter) SetBurst(burst int) {
	r.limiter.SetBurst(burst)
}

// Reserve returns a Reservation that indicates how long the caller must wait
// before n events happen. This is useful for more advanced rate limiting scenarios
// where you need to know the delay before consuming the token.
func (r *RateLimiter) Reserve() *rate.Reservation {
	return r.limiter.Reserve()
}

// Tokens returns the current number of available tokens.
// This can be useful for monitoring and debugging.
func (r *RateLimiter) Tokens() float64 {
	return r.limiter.Tokens()
}
