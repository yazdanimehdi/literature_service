package resilience

import (
	"errors"
	"fmt"
	"testing"

	"go.temporal.io/sdk/temporal"

	sharedllm "github.com/helixir/llm"

	"github.com/helixir/literature-review-service/internal/domain"
)

func TestClassify_LLMErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCategory
	}{
		{
			name:     "budget exceeded",
			err:      &sharedllm.Error{Kind: sharedllm.ErrBudgetExceeded, Message: "over budget"},
			expected: Budget,
		},
		{
			name:     "rate limit",
			err:      &sharedllm.Error{Kind: sharedllm.ErrRateLimit, Message: "too fast"},
			expected: Transient,
		},
		{
			name:     "timeout",
			err:      &sharedllm.Error{Kind: sharedllm.ErrTimeout, Message: "timed out"},
			expected: Transient,
		},
		{
			name:     "network",
			err:      &sharedllm.Error{Kind: sharedllm.ErrNetwork, Message: "connection refused"},
			expected: Transient,
		},
		{
			name:     "server error",
			err:      &sharedllm.Error{Kind: sharedllm.ErrServerError, Message: "500"},
			expected: Transient,
		},
		{
			name:     "circuit open",
			err:      &sharedllm.Error{Kind: sharedllm.ErrCircuitOpen, Message: "breaker tripped"},
			expected: Transient,
		},
		{
			name:     "client rate limited",
			err:      &sharedllm.Error{Kind: sharedllm.ErrClientRateLimited, Message: "client throttled"},
			expected: Transient,
		},
		{
			name:     "quota exceeded",
			err:      &sharedllm.Error{Kind: sharedllm.ErrQuotaExceeded, Message: "quota hit"},
			expected: Transient,
		},
		{
			name:     "auth",
			err:      &sharedllm.Error{Kind: sharedllm.ErrAuth, Message: "bad key"},
			expected: Permanent,
		},
		{
			name:     "bad request",
			err:      &sharedllm.Error{Kind: sharedllm.ErrBadRequest, Message: "invalid param"},
			expected: Permanent,
		},
		{
			name:     "content filter",
			err:      &sharedllm.Error{Kind: sharedllm.ErrContentFilter, Message: "filtered"},
			expected: Permanent,
		},
		{
			name:     "internal (unknown llm kind)",
			err:      &sharedllm.Error{Kind: sharedllm.ErrInternal, Message: "unexpected"},
			expected: Transient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.err)
			if got != tt.expected {
				t.Errorf("Classify(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestClassify_WrappedLLMErrors(t *testing.T) {
	wrapped := fmt.Errorf("extraction failed: %w", &sharedllm.Error{Kind: sharedllm.ErrBudgetExceeded})
	if got := Classify(wrapped); got != Budget {
		t.Errorf("Classify(wrapped budget) = %v, want Budget", got)
	}
}

func TestClassify_DomainSentinelErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCategory
	}{
		{"rate limited", domain.ErrRateLimited, Transient},
		{"service unavailable", domain.ErrServiceUnavailable, Transient},
		{"wrapped rate limited", fmt.Errorf("search: %w", domain.ErrRateLimited), Transient},
		{"invalid input", domain.ErrInvalidInput, Permanent},
		{"not found", domain.ErrNotFound, Permanent},
		{"unauthorized", domain.ErrUnauthorized, Permanent},
		{"forbidden", domain.ErrForbidden, Permanent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.err)
			if got != tt.expected {
				t.Errorf("Classify(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestClassify_MessageSubstrings(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		expected ErrorCategory
	}{
		{"timeout message", "request timeout after 30s", Transient},
		{"connection refused", "dial tcp: connection refused", Transient},
		{"deadline exceeded", "context deadline exceeded", Transient},
		{"budget exceeded in message", "budget exceeded for org", Budget},
		{"bad request message", "bad_request: invalid JSON", Permanent},
		{"not found message", "paper not found", Permanent},
		{"validation message", "validation failed: title required", Permanent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(errors.New(tt.msg))
			if got != tt.expected {
				t.Errorf("Classify(%q) = %v, want %v", tt.msg, got, tt.expected)
			}
		})
	}
}

func TestClassify_NilError(t *testing.T) {
	got := Classify(nil)
	if got != Permanent {
		t.Errorf("Classify(nil) = %v, want Permanent", got)
	}
}

func TestClassify_TemporalApplicationError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCategory
	}{
		{
			name:     "circuit_open type",
			err:      temporal.NewNonRetryableApplicationError("circuit_open: llm", "circuit_open", nil),
			expected: Transient,
		},
		{
			name:     "budget_exceeded type",
			err:      temporal.NewApplicationError("budget exhausted", "budget_exceeded", nil),
			expected: Budget,
		},
		{
			name:     "non-retryable without known type",
			err:      temporal.NewNonRetryableApplicationError("something failed", "UNKNOWN_TYPE", nil),
			expected: Permanent,
		},
		{
			name:     "wrapped circuit_open",
			err:      fmt.Errorf("activity failed: %w", temporal.NewNonRetryableApplicationError("circuit_open: search", "circuit_open", nil)),
			expected: Transient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.err)
			if got != tt.expected {
				t.Errorf("Classify(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestClassify_NarrowedSubstrings(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		expected ErrorCategory
	}{
		// "auth" alone should NOT match permanent (could be "author").
		{"author is not permanent", "paper author not available", Transient},
		// But "unauthorized" and "authentication failed" should.
		{"unauthorized is permanent", "unauthorized: invalid token", Permanent},
		{"authentication failed is permanent", "authentication failed for user", Permanent},
		{"authorization failed is permanent", "authorization failed: insufficient scope", Permanent},
		// "invalid" alone should NOT match permanent (could be "invalidated cache").
		{"invalidated is not permanent", "cache invalidated during refresh", Transient},
		// But "invalid_input" and "invalid request" should.
		{"invalid_input is permanent", "invalid_input: missing field", Permanent},
		{"invalid request is permanent", "invalid request body", Permanent},
		{"invalid parameter is permanent", "invalid parameter: max_results", Permanent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(errors.New(tt.msg))
			if got != tt.expected {
				t.Errorf("Classify(%q) = %v, want %v", tt.msg, got, tt.expected)
			}
		})
	}
}

func TestClassify_TransientBeforePermanent(t *testing.T) {
	// An error containing both transient and permanent substrings should be
	// classified as transient (fail-safe bias).
	err := errors.New("service unavailable: not found but temporary")
	got := Classify(err)
	if got != Transient {
		t.Errorf("Classify(%q) = %v, want Transient (fail-safe bias)", err.Error(), got)
	}
}

func TestClassify_UnknownError(t *testing.T) {
	got := Classify(errors.New("something completely unexpected"))
	if got != Transient {
		t.Errorf("Classify(unknown) = %v, want Transient (default)", got)
	}
}

func TestErrorCategory_String(t *testing.T) {
	if Transient.String() != "transient" {
		t.Errorf("Transient.String() = %q", Transient.String())
	}
	if Budget.String() != "budget" {
		t.Errorf("Budget.String() = %q", Budget.String())
	}
	if Permanent.String() != "permanent" {
		t.Errorf("Permanent.String() = %q", Permanent.String())
	}
}
