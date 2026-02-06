package resilience

import (
	"errors"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// PhaseResult contains the outcome of a phase execution.
type PhaseResult struct {
	// Failed is true when a critical phase has exhausted retries.
	// The workflow should call handleFailure with Err.
	Failed bool

	// Degraded is true when an important phase has exhausted retries.
	// The workflow should mark the result as partial and continue.
	Degraded bool

	// Skipped is true when a non-critical phase has exhausted retries.
	// The workflow should silently continue to the next phase.
	Skipped bool

	// PausedForBudget is true when the phase encountered a budget error.
	// The caller should invoke the existing budget-pause logic.
	PausedForBudget bool

	// Err is the last error encountered. Non-nil when Failed, Degraded, or Skipped is true.
	Err error

	// Attempts is the total number of execution attempts (1 = succeeded on first try).
	Attempts int
}

// Progress tracks retry state for query visibility. The caller should embed
// a pointer to this struct in the workflowProgress so that the query handler
// can report retry information to external observers.
type Progress struct {
	// RetryAttempt is the current retry attempt number (0 = first execution).
	RetryAttempt int

	// RetryPhase is the name of the phase currently being retried.
	RetryPhase string

	// LastRetryError is the string representation of the last transient error.
	LastRetryError string
}

// ExecutePhase runs fn with phase-level retry logic using deterministic
// workflow.Sleep for backoff. It classifies errors and determines the
// outcome based on the phase's criticality and retry budget.
//
// The function uses workflow.Sleep for backoff (Temporal-safe, deterministic).
// Circuit-open errors from activities appear as transient errors and are retried.
//
// Budget errors are not retried here — the caller should check PausedForBudget
// and delegate to the existing checkPausePoint/executeWithBudgetPause logic.
func ExecutePhase(ctx workflow.Context, cfg PhaseConfig, progress *Progress, fn func() error) PhaseResult {
	logger := workflow.GetLogger(ctx)

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// Update progress for query visibility.
		if progress != nil {
			progress.RetryAttempt = attempt
			progress.RetryPhase = cfg.Name
		}

		err := fn()
		if err == nil {
			// Clear retry state on success.
			if progress != nil {
				progress.RetryAttempt = 0
				progress.RetryPhase = ""
				progress.LastRetryError = ""
			}
			return PhaseResult{Attempts: attempt + 1}
		}

		// Check for cancellation — never retry cancelled operations.
		var canceledErr *temporal.CanceledError
		if temporal.IsCanceledError(err) || errors.As(err, &canceledErr) {
			return PhaseResult{
				Failed:   true,
				Err:      fmt.Errorf("%s: %w", cfg.Name, err),
				Attempts: attempt + 1,
			}
		}

		// Check if the workflow context is done (cancelled/timed out).
		if ctx.Err() != nil {
			return PhaseResult{
				Failed:   true,
				Err:      fmt.Errorf("%s: context cancelled: %w", cfg.Name, err),
				Attempts: attempt + 1,
			}
		}

		category := Classify(err)

		// Record last error for observability.
		if progress != nil {
			progress.LastRetryError = err.Error()
		}

		logger.Info("phase execution failed",
			"phase", cfg.Name,
			"attempt", attempt+1,
			"maxAttempts", cfg.MaxRetries+1,
			"errorCategory", category.String(),
			"error", err,
		)

		switch category {
		case Budget:
			// Delegate budget handling to the caller.
			return PhaseResult{PausedForBudget: true, Err: err, Attempts: attempt + 1}

		case Permanent:
			// No point retrying permanent errors.
			return permanentResult(cfg, err, attempt+1)

		case Transient:
			if attempt < cfg.MaxRetries {
				backoff := cfg.backoffForAttempt(attempt)
				logger.Info("retrying phase after backoff",
					"phase", cfg.Name,
					"attempt", attempt+1,
					"backoff", backoff,
				)
				if sleepErr := workflow.Sleep(ctx, backoff); sleepErr != nil {
					// Context cancelled during sleep.
					return PhaseResult{Failed: true, Err: fmt.Errorf("%s: cancelled during retry backoff: %w", cfg.Name, sleepErr), Attempts: attempt + 1}
				}
				continue
			}
			// Retries exhausted.
			return exhaustedResult(cfg, err, attempt+1)
		}
	}

	// Should not be reached, but handle defensively.
	return PhaseResult{Failed: true, Err: fmt.Errorf("%s: unexpected retry loop exit", cfg.Name), Attempts: cfg.MaxRetries + 1}
}

// permanentResult returns the appropriate PhaseResult for a permanent error
// based on the phase's criticality.
func permanentResult(cfg PhaseConfig, err error, attempts int) PhaseResult {
	switch cfg.Criticality {
	case Critical:
		return PhaseResult{Failed: true, Err: fmt.Errorf("%s: permanent error: %w", cfg.Name, err), Attempts: attempts}
	case Important:
		return PhaseResult{Degraded: true, Err: fmt.Errorf("%s: degraded (permanent error): %w", cfg.Name, err), Attempts: attempts}
	default: // NonCritical
		return PhaseResult{Skipped: true, Err: fmt.Errorf("%s: skipped (permanent error): %w", cfg.Name, err), Attempts: attempts}
	}
}

// exhaustedResult returns the appropriate PhaseResult when retries are exhausted
// based on the phase's criticality.
func exhaustedResult(cfg PhaseConfig, err error, attempts int) PhaseResult {
	switch cfg.Criticality {
	case Critical:
		return PhaseResult{Failed: true, Err: fmt.Errorf("%s: retries exhausted: %w", cfg.Name, err), Attempts: attempts}
	case Important:
		return PhaseResult{Degraded: true, Err: fmt.Errorf("%s: degraded (retries exhausted): %w", cfg.Name, err), Attempts: attempts}
	default: // NonCritical
		return PhaseResult{Skipped: true, Err: fmt.Errorf("%s: skipped (retries exhausted): %w", cfg.Name, err), Attempts: attempts}
	}
}

// PhaseResultFromSleep creates a timeout for workflow sleep duration.
func PhaseResultFromSleep(phaseName string, dur time.Duration) PhaseResult {
	return PhaseResult{
		Failed: true,
		Err:    fmt.Errorf("%s: timed out after %v", phaseName, dur),
	}
}
