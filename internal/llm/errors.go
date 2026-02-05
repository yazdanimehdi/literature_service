package llm

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError represents an error returned by an LLM provider API.
type APIError struct {
	// Provider is the name of the LLM provider (e.g., "openai", "anthropic").
	Provider string
	// StatusCode is the HTTP status code returned by the API.
	StatusCode int
	// Message is the error message from the API.
	Message string
	// Type is the error type classification from the API.
	Type string
	// Code is the provider-specific error code (if available).
	Code string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("%s: API error (status %d, type %s): %s", e.Provider, e.StatusCode, e.Type, e.Message)
	}
	return fmt.Sprintf("%s: API error (status %d): %s", e.Provider, e.StatusCode, e.Message)
}

// IsTransient returns true if the error is a transient error that may succeed
// on retry. This includes rate limiting (429), server errors (5xx), and network
// errors (StatusCode 0 indicates no HTTP response was received).
func (e *APIError) IsTransient() bool {
	return e.StatusCode == 0 ||
		e.StatusCode == http.StatusTooManyRequests ||
		e.StatusCode >= 500
}

// isTransientError checks whether an error is a transient API error that should be retried.
// Uses errors.As to correctly unwrap wrapped errors.
func isTransientError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsTransient()
	}
	return false
}
