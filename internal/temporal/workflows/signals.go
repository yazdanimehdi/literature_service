// Package workflows defines Temporal workflow implementations for the
// literature review service pipeline.
package workflows

import "github.com/helixir/literature-review-service/internal/domain"

// PauseSignal carries the reason for pausing a workflow.
type PauseSignal struct {
	// Reason indicates why the workflow is being paused.
	Reason domain.PauseReason `json:"reason"`
	// Message provides optional context about the pause.
	Message string `json:"message"`
}

// ResumeSignal carries information about workflow resumption.
type ResumeSignal struct {
	// ResumedBy indicates who/what triggered the resume (e.g., "user", "budget_refill").
	ResumedBy string `json:"resumed_by"`
}

// StopSignal requests graceful workflow termination with partial results.
type StopSignal struct {
	// Reason provides optional context about why the stop was requested.
	Reason string `json:"reason"`
}
