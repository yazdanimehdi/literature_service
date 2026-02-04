package papersources

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRateLimiter(t *testing.T) {
	t.Run("creates limiter with specified rate and burst", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		require.NotNil(t, rl)
		require.NotNil(t, rl.limiter)

		// Verify burst by allowing multiple requests
		for i := 0; i < 5; i++ {
			assert.True(t, rl.Allow(), "should allow request %d within burst", i+1)
		}
	})

	t.Run("creates limiter with PubMed rate (3 req/sec)", func(t *testing.T) {
		rl := NewRateLimiter(3, 3)

		require.NotNil(t, rl)
		// Should allow burst of 3
		for i := 0; i < 3; i++ {
			assert.True(t, rl.Allow())
		}
		// 4th request should be denied immediately
		assert.False(t, rl.Allow())
	})

	t.Run("creates limiter with Semantic Scholar rate (10 req/sec)", func(t *testing.T) {
		rl := NewRateLimiter(10, 10)

		require.NotNil(t, rl)
		// Should allow burst of 10
		for i := 0; i < 10; i++ {
			assert.True(t, rl.Allow())
		}
		// 11th request should be denied immediately
		assert.False(t, rl.Allow())
	})

	t.Run("creates limiter with fractional rate", func(t *testing.T) {
		// 0.5 requests per second (1 request every 2 seconds)
		rl := NewRateLimiter(0.5, 1)

		require.NotNil(t, rl)
		assert.True(t, rl.Allow())
		assert.False(t, rl.Allow())
	})
}

func TestRateLimiter_Wait(t *testing.T) {
	t.Run("burst allows instant requests", func(t *testing.T) {
		rl := NewRateLimiter(100, 5)

		ctx := context.Background()
		start := time.Now()

		// All 5 burst requests should be nearly instant
		for i := 0; i < 5; i++ {
			err := rl.Wait(ctx)
			require.NoError(t, err)
		}

		elapsed := time.Since(start)
		// Should complete in under 50ms (generous margin for test stability)
		assert.Less(t, elapsed, 50*time.Millisecond,
			"burst requests should be nearly instant, took %v", elapsed)
	})

	t.Run("waits for token after burst exhausted", func(t *testing.T) {
		// 10 requests per second = 100ms between requests
		rl := NewRateLimiter(10, 1)

		ctx := context.Background()

		// First request consumes the burst
		err := rl.Wait(ctx)
		require.NoError(t, err)

		start := time.Now()
		// Second request must wait for token replenishment
		err = rl.Wait(ctx)
		require.NoError(t, err)
		elapsed := time.Since(start)

		// Should wait approximately 100ms (10 req/sec)
		assert.GreaterOrEqual(t, elapsed, 90*time.Millisecond,
			"should wait for token, waited only %v", elapsed)
	})

	t.Run("respects context deadline", func(t *testing.T) {
		rl := NewRateLimiter(1, 1)

		// Exhaust the burst
		assert.True(t, rl.Allow())

		// Create context with short deadline
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		// This should fail because we need to wait ~1 second but deadline is 10ms
		// Note: rate.Limiter.Wait returns "rate: Wait(n=1) would exceed context deadline"
		// when it detects the deadline would be exceeded, not context.DeadlineExceeded directly
		err := rl.Wait(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "deadline")
	})

	t.Run("returns immediately with canceled context", func(t *testing.T) {
		rl := NewRateLimiter(1, 1)

		// Exhaust the burst
		assert.True(t, rl.Allow())

		// Create pre-canceled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := rl.Wait(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestRateLimiter_WaitContextCanceled(t *testing.T) {
	t.Run("returns error when context canceled during wait", func(t *testing.T) {
		rl := NewRateLimiter(1, 1)

		// Exhaust the burst
		assert.True(t, rl.Allow())

		ctx, cancel := context.WithCancel(context.Background())

		// Cancel context after a short delay in a separate goroutine
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		err := rl.Wait(ctx)
		elapsed := time.Since(start)

		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		// Should have been canceled quickly, not waiting full second
		assert.Less(t, elapsed, 500*time.Millisecond)
	})

	t.Run("returns error when context deadline exceeded during wait", func(t *testing.T) {
		rl := NewRateLimiter(1, 1)

		// Exhaust the burst
		assert.True(t, rl.Allow())

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		err := rl.Wait(ctx)
		elapsed := time.Since(start)

		require.Error(t, err)
		// Note: rate.Limiter.Wait returns "rate: Wait(n=1) would exceed context deadline"
		// when it detects the deadline would be exceeded, not context.DeadlineExceeded directly
		assert.Contains(t, err.Error(), "deadline")
		// Should return quickly since limiter detects deadline cannot be met
		assert.Less(t, elapsed, 200*time.Millisecond)
	})
}

func TestRateLimiter_Allow(t *testing.T) {
	t.Run("allows requests within burst", func(t *testing.T) {
		rl := NewRateLimiter(10, 3)

		assert.True(t, rl.Allow(), "first request should be allowed")
		assert.True(t, rl.Allow(), "second request should be allowed")
		assert.True(t, rl.Allow(), "third request should be allowed")
	})

	t.Run("denies requests beyond burst", func(t *testing.T) {
		rl := NewRateLimiter(10, 2)

		assert.True(t, rl.Allow())
		assert.True(t, rl.Allow())
		assert.False(t, rl.Allow(), "third request should be denied")
		assert.False(t, rl.Allow(), "fourth request should also be denied")
	})

	t.Run("allows requests after token replenishment", func(t *testing.T) {
		// 100 requests per second = 10ms per token
		rl := NewRateLimiter(100, 1)

		assert.True(t, rl.Allow())
		assert.False(t, rl.Allow())

		// Wait for token to replenish
		time.Sleep(15 * time.Millisecond)

		assert.True(t, rl.Allow(), "should allow after token replenished")
	})

	t.Run("never allows with zero burst", func(t *testing.T) {
		rl := NewRateLimiter(10, 0)

		// With zero burst, Allow should always return false
		assert.False(t, rl.Allow())
		assert.False(t, rl.Allow())
	})
}

func TestRateLimiter_SetRate(t *testing.T) {
	t.Run("updates rate limit", func(t *testing.T) {
		rl := NewRateLimiter(1, 1)

		// Exhaust burst
		assert.True(t, rl.Allow())
		assert.False(t, rl.Allow())

		// Increase rate to 1000/sec (1ms per token)
		rl.SetRate(1000)

		// Wait a short time for new tokens at higher rate
		time.Sleep(5 * time.Millisecond)

		// Should now have tokens available
		assert.True(t, rl.Allow())
	})

	t.Run("can decrease rate", func(t *testing.T) {
		rl := NewRateLimiter(1000, 1)

		// Set very low rate
		rl.SetRate(0.1) // 1 request per 10 seconds

		// Exhaust burst
		assert.True(t, rl.Allow())

		// Short wait shouldn't produce new tokens at this low rate
		time.Sleep(50 * time.Millisecond)
		assert.False(t, rl.Allow())
	})
}

func TestRateLimiter_SetBurst(t *testing.T) {
	t.Run("updates burst size", func(t *testing.T) {
		rl := NewRateLimiter(1000, 2)

		// Initial burst allows 2
		assert.True(t, rl.Allow())
		assert.True(t, rl.Allow())
		assert.False(t, rl.Allow())

		// Wait for tokens to replenish
		time.Sleep(10 * time.Millisecond)

		// Increase burst to 5
		rl.SetBurst(5)

		// Wait for full replenishment with new burst
		time.Sleep(10 * time.Millisecond)

		// Should now allow up to 5 burst requests
		allowCount := 0
		for i := 0; i < 10; i++ {
			if rl.Allow() {
				allowCount++
			}
		}
		assert.Equal(t, 5, allowCount, "should allow exactly 5 requests with new burst")
	})

	t.Run("can decrease burst size", func(t *testing.T) {
		rl := NewRateLimiter(1000, 10)

		// Set lower burst
		rl.SetBurst(2)

		// Should only allow 2 now
		assert.True(t, rl.Allow())
		assert.True(t, rl.Allow())
		assert.False(t, rl.Allow())
	})
}

func TestRateLimiter_Reserve(t *testing.T) {
	t.Run("returns valid reservation", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		reservation := rl.Reserve()
		require.NotNil(t, reservation)
		assert.True(t, reservation.OK(), "reservation should be OK")
	})

	t.Run("reservation indicates delay when burst exhausted", func(t *testing.T) {
		// 1 request per second
		rl := NewRateLimiter(1, 1)

		// First reservation uses the burst token
		r1 := rl.Reserve()
		assert.True(t, r1.OK())
		assert.Zero(t, r1.Delay(), "first reservation should have no delay")

		// Second reservation must wait
		r2 := rl.Reserve()
		assert.True(t, r2.OK())
		delay := r2.Delay()
		assert.Greater(t, delay, time.Duration(0), "second reservation should have delay")

		// Cancel the second reservation to release the token
		r2.Cancel()
	})
}

func TestRateLimiter_Tokens(t *testing.T) {
	t.Run("returns current token count", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		// Should start with full burst
		tokens := rl.Tokens()
		assert.InDelta(t, 5.0, tokens, 0.1, "should have approximately 5 tokens")

		// Consume some tokens
		rl.Allow()
		rl.Allow()

		tokens = rl.Tokens()
		assert.InDelta(t, 3.0, tokens, 0.1, "should have approximately 3 tokens left")
	})

	t.Run("tokens replenish over time", func(t *testing.T) {
		// 100 tokens per second
		rl := NewRateLimiter(100, 5)

		// Exhaust all tokens
		for rl.Allow() {
		}

		initialTokens := rl.Tokens()
		assert.Less(t, initialTokens, 1.0)

		// Wait for tokens to replenish
		time.Sleep(30 * time.Millisecond)

		newTokens := rl.Tokens()
		assert.Greater(t, newTokens, initialTokens, "tokens should have increased")
	})
}

func TestRateLimiter_Concurrency(t *testing.T) {
	t.Run("is safe for concurrent use", func(t *testing.T) {
		rl := NewRateLimiter(1000, 100)
		ctx := context.Background()

		var wg sync.WaitGroup
		errChan := make(chan error, 100)

		// Spawn multiple goroutines making concurrent requests
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					if err := rl.Wait(ctx); err != nil {
						errChan <- err
						return
					}
				}
			}()
		}

		// Also test concurrent Allow calls
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					rl.Allow()
				}
			}()
		}

		// Also test concurrent rate updates
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(rate float64) {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					rl.SetRate(rate)
					rl.SetBurst(int(rate))
				}
			}(float64(100 + i*10))
		}

		wg.Wait()
		close(errChan)

		for err := range errChan {
			t.Errorf("unexpected error during concurrent access: %v", err)
		}
	})

	t.Run("concurrent Wait with context cancellation", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)
		ctx, cancel := context.WithCancel(context.Background())

		var wg sync.WaitGroup

		// Start several waiters
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Some will succeed, some will be canceled
				_ = rl.Wait(ctx)
			}()
		}

		// Give some time for requests to start, then cancel
		time.Sleep(10 * time.Millisecond)
		cancel()

		// Wait for all goroutines to complete
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for goroutines to complete")
		}
	})
}
