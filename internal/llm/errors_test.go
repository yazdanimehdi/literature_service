package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// TestAPIError_Error
// ---------------------------------------------------------------------------

func TestAPIError_Error(t *testing.T) {
	t.Parallel()

	t.Run("with type field", func(t *testing.T) {
		t.Parallel()
		err := &APIError{
			Provider:   "openai",
			StatusCode: 429,
			Message:    "rate limit exceeded",
			Type:       "rate_limit_error",
		}
		got := err.Error()
		assert.Contains(t, got, "openai")
		assert.Contains(t, got, "429")
		assert.Contains(t, got, "rate_limit_error")
		assert.Contains(t, got, "rate limit exceeded")
		assert.Equal(t, "openai: API error (status 429, type rate_limit_error): rate limit exceeded", got)
	})

	t.Run("without type field", func(t *testing.T) {
		t.Parallel()
		err := &APIError{
			Provider:   "anthropic",
			StatusCode: 500,
			Message:    "internal server error",
		}
		got := err.Error()
		assert.Contains(t, got, "anthropic")
		assert.Contains(t, got, "500")
		assert.Contains(t, got, "internal server error")
		assert.NotContains(t, got, "type")
		assert.Equal(t, "anthropic: API error (status 500): internal server error", got)
	})

	t.Run("with code field does not affect output", func(t *testing.T) {
		t.Parallel()
		err := &APIError{
			Provider:   "openai",
			StatusCode: 401,
			Message:    "invalid api key",
			Code:       "invalid_api_key",
		}
		got := err.Error()
		// Code is stored but not included in Error() string.
		assert.Equal(t, "openai: API error (status 401): invalid api key", got)
		assert.Equal(t, "invalid_api_key", err.Code) // Verify code is stored.
	})
}

// ---------------------------------------------------------------------------
// TestAPIError_IsTransient
// ---------------------------------------------------------------------------

func TestAPIError_IsTransient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		// Transient errors.
		{name: "429 Too Many Requests is transient", statusCode: 429, want: true},
		{name: "500 Internal Server Error is transient", statusCode: 500, want: true},
		{name: "502 Bad Gateway is transient", statusCode: 502, want: true},
		{name: "503 Service Unavailable is transient", statusCode: 503, want: true},
		{name: "504 Gateway Timeout is transient", statusCode: 504, want: true},
		{name: "0 (no HTTP response / network error) is transient", statusCode: 0, want: true},
		{name: "599 unknown 5xx is transient", statusCode: 599, want: true},

		// Non-transient errors.
		{name: "400 Bad Request is not transient", statusCode: 400, want: false},
		{name: "401 Unauthorized is not transient", statusCode: 401, want: false},
		{name: "403 Forbidden is not transient", statusCode: 403, want: false},
		{name: "404 Not Found is not transient", statusCode: 404, want: false},
		{name: "422 Unprocessable Entity is not transient", statusCode: 422, want: false},
		{name: "200 OK is not transient", statusCode: 200, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := &APIError{
				Provider:   "test-provider",
				StatusCode: tc.statusCode,
				Message:    "test message",
			}
			assert.Equal(t, tc.want, err.IsTransient())
		})
	}
}

// ---------------------------------------------------------------------------
// TestAPIError_ImplementsError
// ---------------------------------------------------------------------------

func TestAPIError_ImplementsError(t *testing.T) {
	t.Parallel()

	var err error = &APIError{
		Provider:   "openai",
		StatusCode: 500,
		Message:    "server error",
	}
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "openai")
}
