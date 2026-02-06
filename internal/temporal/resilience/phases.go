package resilience

import "time"

// PhaseCriticality determines how the workflow handles exhausted retries
// for a given phase.
type PhaseCriticality int

const (
	// Critical phases cause the entire workflow to fail when retries are exhausted.
	Critical PhaseCriticality = iota

	// Important phases degrade to partial results when retries are exhausted.
	// The workflow continues to the next phase.
	Important

	// NonCritical phases are silently skipped when retries are exhausted.
	// The workflow continues to the next phase without marking as degraded.
	NonCritical
)

// String returns a human-readable name for the criticality level.
func (c PhaseCriticality) String() string {
	switch c {
	case Critical:
		return "critical"
	case Important:
		return "important"
	case NonCritical:
		return "non-critical"
	default:
		return "unknown"
	}
}

// PhaseConfig holds the retry and criticality configuration for a single
// workflow phase.
type PhaseConfig struct {
	// Name is the phase identifier (e.g. "extracting_keywords", "searching").
	Name string

	// Criticality determines behaviour when retries are exhausted.
	Criticality PhaseCriticality

	// MaxRetries is the maximum number of retry attempts for transient errors.
	MaxRetries int

	// InitialBackoff is the delay before the first retry.
	InitialBackoff time.Duration

	// BackoffMultiplier controls exponential growth of the backoff interval.
	BackoffMultiplier float64

	// MaxBackoff caps the maximum backoff interval.
	MaxBackoff time.Duration
}

// backoffForAttempt computes the backoff duration for the given attempt (0-indexed).
func (p PhaseConfig) backoffForAttempt(attempt int) time.Duration {
	backoff := p.InitialBackoff
	for i := 0; i < attempt; i++ {
		backoff = time.Duration(float64(backoff) * p.BackoffMultiplier)
		if backoff > p.MaxBackoff {
			backoff = p.MaxBackoff
			break
		}
	}
	return backoff
}

// DefaultPhaseConfigs returns the standard phase configurations for the
// literature review workflow.
func DefaultPhaseConfigs() map[string]PhaseConfig {
	return map[string]PhaseConfig{
		"extracting_keywords": {
			Name:              "extracting_keywords",
			Criticality:       Critical,
			MaxRetries:        3,
			InitialBackoff:    5 * time.Second,
			BackoffMultiplier: 2.0,
			MaxBackoff:        60 * time.Second,
		},
		"searching": {
			Name:              "searching",
			Criticality:       Important,
			MaxRetries:        2,
			InitialBackoff:    10 * time.Second,
			BackoffMultiplier: 2.0,
			MaxBackoff:        60 * time.Second,
		},
		"processing": {
			Name:              "processing",
			Criticality:       Important,
			MaxRetries:        2,
			InitialBackoff:    10 * time.Second,
			BackoffMultiplier: 2.0,
			MaxBackoff:        60 * time.Second,
		},
		"expanding": {
			Name:              "expanding",
			Criticality:       NonCritical,
			MaxRetries:        1,
			InitialBackoff:    5 * time.Second,
			BackoffMultiplier: 2.0,
			MaxBackoff:        30 * time.Second,
		},
		"reviewing": {
			Name:              "reviewing",
			Criticality:       NonCritical,
			MaxRetries:        1,
			InitialBackoff:    5 * time.Second,
			BackoffMultiplier: 2.0,
			MaxBackoff:        30 * time.Second,
		},
	}
}
