// Package resilience provides workflow-level error classification, phase
// execution with retry logic, and circuit breaker management for the
// literature review Temporal workflows.
package resilience

import (
	"errors"
	"strings"

	sharedllm "github.com/helixir/llm"

	"github.com/helixir/literature-review-service/internal/domain"
)

// ErrorCategory classifies errors into workflow-level categories that
// determine the retry and degradation behaviour of each phase.
type ErrorCategory int

const (
	// Transient errors are temporary failures that should be retried with
	// exponential backoff (e.g. network timeouts, rate limits, circuit open).
	Transient ErrorCategory = iota

	// Budget errors indicate budget/quota exhaustion. The workflow should
	// pause and wait for a budget refill signal before retrying.
	Budget

	// Permanent errors are non-recoverable. The workflow should either fail
	// (for critical phases) or degrade/skip (for non-critical phases).
	Permanent
)

// String returns a human-readable name for the category.
func (c ErrorCategory) String() string {
	switch c {
	case Transient:
		return "transient"
	case Budget:
		return "budget"
	case Permanent:
		return "permanent"
	default:
		return "unknown"
	}
}

// transientSubstrings are error message substrings that indicate a transient failure
// when the error is not already classified by a structured error type.
var transientSubstrings = []string{
	"timeout",
	"network",
	"connection refused",
	"connection reset",
	"circuit_open",
	"circuit breaker",
	"rate limit",
	"rate_limit",
	"server_error",
	"service unavailable",
	"temporary",
	"deadline exceeded",
	"i/o timeout",
}

// permanentSubstrings indicate a permanent failure.
var permanentSubstrings = []string{
	"auth",
	"unauthorized",
	"forbidden",
	"bad_request",
	"bad request",
	"not_found",
	"not found",
	"invalid",
	"validation",
	"content_filter",
}

// Classify inspects err and returns its ErrorCategory.
//
// Classification priority:
//  1. Structured LLM errors (llm.Error) — uses ErrorKind
//  2. Domain sentinel errors — ErrRateLimited, ErrServiceUnavailable, etc.
//  3. Error message substring matching
//  4. Default: Transient (safer to retry than to fail)
func Classify(err error) ErrorCategory {
	if err == nil {
		return Transient
	}

	// 1. Check shared llm.Error (covers both direct and wrapped errors).
	kind := sharedllm.ErrorKindOf(err)
	if kind != "" {
		switch kind {
		case sharedllm.ErrBudgetExceeded:
			return Budget
		case sharedllm.ErrRateLimit, sharedllm.ErrClientRateLimited,
			sharedllm.ErrTimeout, sharedllm.ErrNetwork,
			sharedllm.ErrServerError, sharedllm.ErrCircuitOpen,
			sharedllm.ErrQuotaExceeded:
			return Transient
		case sharedllm.ErrAuth, sharedllm.ErrBadRequest,
			sharedllm.ErrContentFilter:
			return Permanent
		default:
			return Transient
		}
	}

	// 2. Check domain sentinel errors.
	if errors.Is(err, domain.ErrRateLimited) || errors.Is(err, domain.ErrServiceUnavailable) {
		return Transient
	}
	if errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrNotFound) ||
		errors.Is(err, domain.ErrUnauthorized) || errors.Is(err, domain.ErrForbidden) {
		return Permanent
	}

	// 3. Fall back to message substring matching.
	msg := strings.ToLower(err.Error())

	// Budget substring check (before transient, since "budget" is more specific).
	if strings.Contains(msg, "budget") && strings.Contains(msg, "exceeded") {
		return Budget
	}

	for _, sub := range permanentSubstrings {
		if strings.Contains(msg, sub) {
			return Permanent
		}
	}

	for _, sub := range transientSubstrings {
		if strings.Contains(msg, sub) {
			return Transient
		}
	}

	// 4. Default: treat unknown errors as transient (safer to retry).
	return Transient
}
