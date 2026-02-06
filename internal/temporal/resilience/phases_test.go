package resilience

import (
	"testing"
	"time"
)

func TestDefaultPhaseConfigs(t *testing.T) {
	configs := DefaultPhaseConfigs()

	expectedPhases := []string{
		"extracting_keywords",
		"searching",
		"processing",
		"expanding",
		"reviewing",
	}

	for _, phase := range expectedPhases {
		if _, ok := configs[phase]; !ok {
			t.Errorf("missing default config for phase %q", phase)
		}
	}
}

func TestDefaultPhaseConfigs_Criticality(t *testing.T) {
	configs := DefaultPhaseConfigs()

	tests := []struct {
		phase    string
		expected PhaseCriticality
	}{
		{"extracting_keywords", Critical},
		{"searching", Important},
		{"processing", Important},
		{"expanding", NonCritical},
		{"reviewing", NonCritical},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			cfg := configs[tt.phase]
			if cfg.Criticality != tt.expected {
				t.Errorf("phase %q criticality = %v, want %v", tt.phase, cfg.Criticality, tt.expected)
			}
		})
	}
}

func TestDefaultPhaseConfigs_RetryLimits(t *testing.T) {
	configs := DefaultPhaseConfigs()

	// Critical phases get more retries.
	if configs["extracting_keywords"].MaxRetries != 3 {
		t.Errorf("extracting_keywords MaxRetries = %d, want 3", configs["extracting_keywords"].MaxRetries)
	}

	// Important phases get moderate retries.
	if configs["searching"].MaxRetries != 2 {
		t.Errorf("searching MaxRetries = %d, want 2", configs["searching"].MaxRetries)
	}

	// Non-critical phases get minimal retries.
	if configs["expanding"].MaxRetries != 1 {
		t.Errorf("expanding MaxRetries = %d, want 1", configs["expanding"].MaxRetries)
	}
}

func TestPhaseConfig_BackoffForAttempt(t *testing.T) {
	cfg := PhaseConfig{
		InitialBackoff:    5 * time.Second,
		BackoffMultiplier: 2.0,
		MaxBackoff:        30 * time.Second,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 5 * time.Second},
		{1, 10 * time.Second},
		{2, 20 * time.Second},
		{3, 30 * time.Second}, // capped at MaxBackoff
		{4, 30 * time.Second}, // still capped
	}

	for _, tt := range tests {
		got := cfg.backoffForAttempt(tt.attempt)
		if got != tt.expected {
			t.Errorf("backoffForAttempt(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestPhaseConfig_BackoffCapping(t *testing.T) {
	cfg := PhaseConfig{
		InitialBackoff:    1 * time.Second,
		BackoffMultiplier: 10.0,
		MaxBackoff:        5 * time.Second,
	}

	// Attempt 0: 1s, Attempt 1: 10s -> capped at 5s
	got := cfg.backoffForAttempt(1)
	if got != 5*time.Second {
		t.Errorf("backoffForAttempt(1) = %v, want %v", got, 5*time.Second)
	}
}

func TestPhaseCriticality_String(t *testing.T) {
	tests := []struct {
		c    PhaseCriticality
		want string
	}{
		{Critical, "critical"},
		{Important, "important"},
		{NonCritical, "non-critical"},
		{PhaseCriticality(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("PhaseCriticality(%d).String() = %q, want %q", tt.c, got, tt.want)
		}
	}
}
