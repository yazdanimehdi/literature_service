package resilience

import (
	"sync"
	"time"

	"github.com/helixir/llm"
)

// Default circuit breaker configurations for external dependencies.
var defaultBreakerConfigs = map[string]llm.CircuitBreakerConfig{
	"semantic_scholar": {
		ConsecutiveThreshold: 5,
		Cooldown:             60 * time.Second,
	},
	"openalex": {
		ConsecutiveThreshold: 5,
		Cooldown:             60 * time.Second,
	},
	"pubmed": {
		ConsecutiveThreshold: 5,
		Cooldown:             60 * time.Second,
	},
	"scopus": {
		ConsecutiveThreshold: 5,
		Cooldown:             60 * time.Second,
	},
	"biorxiv": {
		ConsecutiveThreshold: 5,
		Cooldown:             60 * time.Second,
	},
	"arxiv": {
		ConsecutiveThreshold: 5,
		Cooldown:             60 * time.Second,
	},
	"llm": {
		ConsecutiveThreshold: 3,
		Cooldown:             30 * time.Second,
	},
	"ingestion": {
		ConsecutiveThreshold: 5,
		Cooldown:             45 * time.Second,
	},
}

// BreakerRegistry provides named circuit breakers for external dependencies.
// It is safe for concurrent use and lazily creates breakers on first access.
//
// Circuit breakers live in activities (not workflows) because they use
// sync.Mutex and time.Now() which are non-deterministic and would violate
// Temporal's workflow determinism requirements. The workflow sees circuit-open
// errors as transient errors and retries after backoff.
type BreakerRegistry struct {
	mu       sync.Mutex
	breakers map[string]*llm.CircuitBreaker
	configs  map[string]llm.CircuitBreakerConfig
}

// NewBreakerRegistry creates a BreakerRegistry with default configurations
// for all known external dependencies.
func NewBreakerRegistry() *BreakerRegistry {
	return &BreakerRegistry{
		breakers: make(map[string]*llm.CircuitBreaker),
		configs:  defaultBreakerConfigs,
	}
}

// NewBreakerRegistryWithConfigs creates a BreakerRegistry with custom configurations.
// Any name not in the provided map falls back to a sensible default.
func NewBreakerRegistryWithConfigs(configs map[string]llm.CircuitBreakerConfig) *BreakerRegistry {
	merged := make(map[string]llm.CircuitBreakerConfig, len(defaultBreakerConfigs))
	for k, v := range defaultBreakerConfigs {
		merged[k] = v
	}
	for k, v := range configs {
		merged[k] = v
	}
	return &BreakerRegistry{
		breakers: make(map[string]*llm.CircuitBreaker),
		configs:  merged,
	}
}

// Get returns the circuit breaker for the given dependency name.
// If no breaker exists yet, one is created using the configured (or default)
// parameters. This method is safe for concurrent use.
func (r *BreakerRegistry) Get(name string) *llm.CircuitBreaker {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cb, ok := r.breakers[name]; ok {
		return cb
	}

	cfg, ok := r.configs[name]
	if !ok {
		// Sensible fallback for unknown dependencies.
		cfg = llm.CircuitBreakerConfig{
			ConsecutiveThreshold: 5,
			Cooldown:             60 * time.Second,
		}
	}

	cb := llm.NewCircuitBreaker(cfg)
	r.breakers[name] = cb
	return cb
}

// State returns the current state of the named breaker, or CircuitClosed
// if the breaker has not been created yet.
func (r *BreakerRegistry) State(name string) llm.CircuitState {
	r.mu.Lock()
	cb, ok := r.breakers[name]
	r.mu.Unlock()

	if !ok {
		return llm.CircuitClosed
	}
	return cb.State()
}
