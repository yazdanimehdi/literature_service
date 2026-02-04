package domain

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for common error conditions.
var (
	// ErrNotFound indicates that a requested entity was not found.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists indicates that an entity already exists.
	ErrAlreadyExists = errors.New("already exists")

	// ErrInvalidInput indicates that the input data is invalid.
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnauthorized indicates that the request lacks valid authentication.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates that the request is not allowed for the authenticated user.
	ErrForbidden = errors.New("forbidden")

	// ErrRateLimited indicates that the request was rate limited.
	ErrRateLimited = errors.New("rate limited")

	// ErrServiceUnavailable indicates that an external service is unavailable.
	ErrServiceUnavailable = errors.New("service unavailable")

	// ErrInternalError indicates an internal server error.
	ErrInternalError = errors.New("internal error")

	// ErrWorkflowFailed indicates that a Temporal workflow failed.
	ErrWorkflowFailed = errors.New("workflow failed")

	// ErrCancelled indicates that an operation was cancelled.
	ErrCancelled = errors.New("cancelled")

	// ErrNoIdentifier indicates that a paper has no valid identifier.
	ErrNoIdentifier = errors.New("no identifier")
)

// ValidationError represents a validation error for a specific field.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s: %s", e.Field, e.Message)
}

// NotFoundError provides details about a not found entity.
type NotFoundError struct {
	Entity string
	ID     string
}

// Error implements the error interface.
func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Entity, e.ID)
}

// Unwrap returns the underlying sentinel error for use with errors.Is.
func (e *NotFoundError) Unwrap() error {
	return ErrNotFound
}

// AlreadyExistsError provides details about a duplicate entity.
type AlreadyExistsError struct {
	Entity string
	ID     string
}

// Error implements the error interface.
func (e *AlreadyExistsError) Error() string {
	return fmt.Sprintf("%s already exists: %s", e.Entity, e.ID)
}

// Unwrap returns the underlying sentinel error for use with errors.Is.
func (e *AlreadyExistsError) Unwrap() error {
	return ErrAlreadyExists
}

// RateLimitError provides details about a rate limit error.
type RateLimitError struct {
	Source     string
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited by %s: retry after %s", e.Source, e.RetryAfter)
}

// Unwrap returns the underlying sentinel error for use with errors.Is.
func (e *RateLimitError) Unwrap() error {
	return ErrRateLimited
}

// ExternalAPIError provides details about an external API error.
type ExternalAPIError struct {
	Source     string
	StatusCode int
	Message    string
	Cause      error
}

// Error implements the error interface.
func (e *ExternalAPIError) Error() string {
	return fmt.Sprintf("%s API error (status %d): %s", e.Source, e.StatusCode, e.Message)
}

// Unwrap returns the underlying cause error.
func (e *ExternalAPIError) Unwrap() error {
	return e.Cause
}

// NewNotFoundError creates a new NotFoundError.
func NewNotFoundError(entity, id string) *NotFoundError {
	return &NotFoundError{
		Entity: entity,
		ID:     id,
	}
}

// NewAlreadyExistsError creates a new AlreadyExistsError.
func NewAlreadyExistsError(entity, id string) *AlreadyExistsError {
	return &AlreadyExistsError{
		Entity: entity,
		ID:     id,
	}
}

// NewValidationError creates a new ValidationError.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// NewRateLimitError creates a new RateLimitError.
func NewRateLimitError(source string, retryAfter time.Duration) *RateLimitError {
	return &RateLimitError{
		Source:     source,
		RetryAfter: retryAfter,
	}
}

// NewExternalAPIError creates a new ExternalAPIError.
func NewExternalAPIError(source string, statusCode int, message string, cause error) *ExternalAPIError {
	return &ExternalAPIError{
		Source:     source,
		StatusCode: statusCode,
		Message:    message,
		Cause:      cause,
	}
}
