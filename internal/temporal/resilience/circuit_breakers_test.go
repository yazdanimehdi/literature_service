package resilience

import (
	"sync"
	"testing"
	"time"

	"github.com/helixir/llm"
)

func TestNewBreakerRegistry(t *testing.T) {
	reg := NewBreakerRegistry()
	if reg == nil {
		t.Fatal("NewBreakerRegistry returned nil")
	}
	if len(reg.configs) == 0 {
		t.Error("expected default configs, got empty map")
	}
}

func TestBreakerRegistry_GetReturnsSameInstance(t *testing.T) {
	reg := NewBreakerRegistry()
	cb1 := reg.Get("llm")
	cb2 := reg.Get("llm")
	if cb1 != cb2 {
		t.Error("Get() should return the same instance for the same name")
	}
}

func TestBreakerRegistry_GetDifferentNames(t *testing.T) {
	reg := NewBreakerRegistry()
	cbLLM := reg.Get("llm")
	cbSearch := reg.Get("semantic_scholar")
	if cbLLM == cbSearch {
		t.Error("Get() should return different instances for different names")
	}
}

func TestBreakerRegistry_GetUnknownName(t *testing.T) {
	reg := NewBreakerRegistry()
	cb := reg.Get("unknown_service")
	if cb == nil {
		t.Fatal("Get() should return a breaker even for unknown names")
	}
	// Should allow requests (closed state).
	if err := cb.Allow(); err != nil {
		t.Errorf("unknown breaker should be closed, got error: %v", err)
	}
}

func TestBreakerRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewBreakerRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb := reg.Get("llm")
			if cb == nil {
				t.Error("Get() returned nil")
			}
		}()
	}
	wg.Wait()
}

func TestBreakerRegistry_StateUnknownBreaker(t *testing.T) {
	reg := NewBreakerRegistry()
	state := reg.State("never_accessed")
	if state != llm.CircuitClosed {
		t.Errorf("State() for unknown breaker should be CircuitClosed, got %v", state)
	}
}

func TestBreakerRegistry_TripBreaker(t *testing.T) {
	// Use custom config with low threshold for testability.
	reg := NewBreakerRegistryWithConfigs(map[string]llm.CircuitBreakerConfig{
		"test_service": {
			ConsecutiveThreshold: 2,
			Cooldown:             1 * time.Second,
		},
	})

	cb := reg.Get("test_service")

	// Record enough failures to trip the breaker.
	cb.RecordFailure()
	cb.RecordFailure()

	// Breaker should now be open.
	if err := cb.Allow(); err == nil {
		t.Error("expected circuit breaker to be open after consecutive failures")
	}

	state := reg.State("test_service")
	if state != llm.CircuitOpen {
		t.Errorf("expected CircuitOpen, got %v", state)
	}
}

func TestNewBreakerRegistryWithConfigs_MergesDefaults(t *testing.T) {
	custom := map[string]llm.CircuitBreakerConfig{
		"llm": {
			ConsecutiveThreshold: 10,
			Cooldown:             5 * time.Second,
		},
	}
	reg := NewBreakerRegistryWithConfigs(custom)

	// Custom override should be applied.
	if cfg := reg.configs["llm"]; cfg.ConsecutiveThreshold != 10 {
		t.Errorf("expected custom threshold 10, got %d", cfg.ConsecutiveThreshold)
	}

	// Default configs should still be present.
	if _, ok := reg.configs["semantic_scholar"]; !ok {
		t.Error("expected default config for semantic_scholar to be preserved")
	}
}

func TestBreakerRegistry_RecordSuccess_ClosesBreaker(t *testing.T) {
	reg := NewBreakerRegistryWithConfigs(map[string]llm.CircuitBreakerConfig{
		"test": {
			ConsecutiveThreshold: 2,
			Cooldown:             100 * time.Millisecond,
		},
	})

	cb := reg.Get("test")

	// Trip the breaker.
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for cooldown to enter half-open.
	time.Sleep(150 * time.Millisecond)

	// In half-open, Allow() should succeed (probe request).
	if err := cb.Allow(); err != nil {
		t.Fatalf("expected half-open Allow() to succeed, got: %v", err)
	}

	// Record success to close the breaker.
	cb.RecordSuccess()

	// Breaker should be closed now.
	if err := cb.Allow(); err != nil {
		t.Errorf("expected closed breaker after success, got: %v", err)
	}
}
