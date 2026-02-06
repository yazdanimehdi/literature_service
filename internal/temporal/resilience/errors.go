// Package resilience provides workflow-level error classification, phase
// execution with retry logic, and circuit breaker management for the
// literature review Temporal workflows.
package resilience

import (
	"errors"
	"strings"

	"go.temporal.io/sdk/temporal"

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
// Substrings are chosen to avoid false positives: "unauthorized" instead of
// "auth" (which would match "author"), "invalid_input"/"invalid request"/
// "invalid parameter" instead of bare "invalid" (which would match
// "invalidated cache" or similar transient contexts).
var permanentSubstrings = []string{
	"unauthorized",
	"authentication failed",
	"authorization failed",
	"forbidden",
	"bad_request",
	"bad request",
	"not_found",
	"not found",
	"invalid_input",
	"invalid request",
	"invalid parameter",
	"validation",
	"content_filter",
}

// Classify inspects err and returns its ErrorCategory.
//
// Classification priority:
//  1. Nil errors — Permanent (no-op; callers should not retry nil)
//  2. Structured LLM errors (llm.Error) — uses ErrorKind
//  3. Temporal ApplicationError — uses Type() field
//  4. Domain sentinel errors — ErrRateLimited, ErrServiceUnavailable, etc.
//  5. Error message substring matching (transient checked first for fail-safe bias)
//  6. Default: Transient (safer to retry than to fail)
func Classify(err error) ErrorCategory {
	if err == nil {
		return Permanent
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

	// 2. Check Temporal ApplicationError Type() field.
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		switch appErr.Type() {
		case "circuit_open":
			return Transient
		case "budget_exceeded":
			return Budget
		}
		// If NonRetryable is set, treat as permanent.
		if appErr.NonRetryable() {
			return Permanent
		}
	}

	// 3. Check domain sentinel errors.
	if errors.Is(err, domain.ErrRateLimited) || errors.Is(err, domain.ErrServiceUnavailable) {
		return Transient
	}
	if errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrNotFound) ||
		errors.Is(err, domain.ErrUnauthorized) || errors.Is(err, domain.ErrForbidden) {
		return Permanent
	}

	// 4. Fall back to message substring matching.
	msg := strings.ToLower(err.Error())

	// Budget substring check (most specific, check first).
	if strings.Contains(msg, "budget") && strings.Contains(msg, "exceeded") {
		return Budget
	}

	// Transient substrings checked before permanent for fail-safe bias:
	// if in doubt, retry is safer than giving up.
	for _, sub := range transientSubstrings {
		if strings.Contains(msg, sub) {
			return Transient
		}
	}

	for _, sub := range permanentSubstrings {
		if strings.Contains(msg, sub) {
			return Permanent
		}
	}

	// 5. Default: treat unknown errors as transient (safer to retry).
	return Transient
}
