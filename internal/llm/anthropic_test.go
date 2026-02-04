package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixir/literature-review-service/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check.
var _ KeywordExtractor = (*AnthropicProvider)(nil)

// newAnthropicTestServer creates an httptest server that responds with the given handler.
func newAnthropicTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// newAnthropicTestProvider creates an AnthropicProvider pointing at the given test server URL.
func newAnthropicTestProvider(baseURL string) *AnthropicProvider {
	cfg := config.AnthropicConfig{
		APIKey:  "test-api-key",
		Model:   "claude-3-sonnet-20240229",
		BaseURL: baseURL,
	}
	p := NewAnthropicProvider(cfg, 0.7, 10*time.Second, 2)
	p.retryDelay = 10 * time.Millisecond // Fast retries for tests.
	return p
}

func TestAnthropicProvider_ExtractKeywords(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path.
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/messages", r.URL.Path)

		// Verify headers.
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))

		// Verify request body structure.
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		defer r.Body.Close()

		var reqBody messagesRequest
		err = json.Unmarshal(body, &reqBody)
		require.NoError(t, err)

		assert.Equal(t, "claude-3-sonnet-20240229", reqBody.Model)
		assert.Equal(t, defaultAnthropicMaxTokens, reqBody.MaxTokens)
		assert.NotEmpty(t, reqBody.System, "system prompt should be set")
		assert.Len(t, reqBody.Messages, 1)
		assert.Equal(t, "user", reqBody.Messages[0].Role)
		assert.NotEmpty(t, reqBody.Messages[0].Content)
		assert.InDelta(t, 0.7, reqBody.Temperature, 0.001)

		// Return a valid response.
		resp := messagesResponse{
			ID:   "msg_test123",
			Type: "message",
			Role: "assistant",
			Content: []contentBlock{
				{
					Type: "text",
					Text: `{"keywords": ["CRISPR", "gene editing", "Cas9", "genome engineering"], "reasoning": "Core gene editing terms extracted from query."}`,
				},
			},
			Model:      "claude-3-sonnet-20240229",
			StopReason: "end_turn",
			Usage: anthropicUsage{
				InputTokens:  150,
				OutputTokens: 45,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}

	srv := newAnthropicTestServer(t, handler)
	provider := newAnthropicTestProvider(srv.URL)

	req := ExtractionRequest{
		Text:        "What are the latest advances in CRISPR gene editing?",
		Mode:        ExtractionModeQuery,
		MaxKeywords: 10,
		MinKeywords: 3,
	}

	result, err := provider.ExtractKeywords(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, []string{"CRISPR", "gene editing", "Cas9", "genome engineering"}, result.Keywords)
	assert.Equal(t, "Core gene editing terms extracted from query.", result.Reasoning)
	assert.Equal(t, "claude-3-sonnet-20240229", result.Model)
	assert.Equal(t, 150, result.InputTokens)
	assert.Equal(t, 45, result.OutputTokens)
}

func TestAnthropicProvider_ExtractKeywords_APIError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		errorType  string
		message    string
		wantRetry  bool
	}{
		{
			name:       "authentication error (401)",
			statusCode: http.StatusUnauthorized,
			errorType:  "authentication_error",
			message:    "invalid x-api-key",
			wantRetry:  false,
		},
		{
			name:       "invalid request error (400)",
			statusCode: http.StatusBadRequest,
			errorType:  "invalid_request_error",
			message:    "max_tokens must be positive",
			wantRetry:  false,
		},
		{
			name:       "rate limit error (429)",
			statusCode: http.StatusTooManyRequests,
			errorType:  "rate_limit_error",
			message:    "rate limit exceeded",
			wantRetry:  true,
		},
		{
			name:       "overloaded error (529)",
			statusCode: 529,
			errorType:  "overloaded_error",
			message:    "API is overloaded",
			wantRetry:  true,
		},
		{
			name:       "internal server error (500)",
			statusCode: http.StatusInternalServerError,
			errorType:  "api_error",
			message:    "internal server error",
			wantRetry:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var requestCount atomic.Int32

			handler := func(w http.ResponseWriter, r *http.Request) {
				requestCount.Add(1)

				errResp := anthropicErrorResponse{
					Type: "error",
					Error: anthropicAPIErrorDetail{
						Type:    tt.errorType,
						Message: tt.message,
					},
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(errResp)
			}

			srv := newAnthropicTestServer(t, handler)
			provider := newAnthropicTestProvider(srv.URL)

			req := ExtractionRequest{
				Text:        "test query",
				Mode:        ExtractionModeQuery,
				MaxKeywords: 5,
				MinKeywords: 2,
			}

			result, err := provider.ExtractKeywords(context.Background(), req)
			assert.Nil(t, result)
			require.Error(t, err)

			assert.Contains(t, err.Error(), tt.errorType)
			assert.Contains(t, err.Error(), tt.message)

			if tt.wantRetry {
				// 1 initial + maxRetries (2) = 3 total attempts.
				assert.Equal(t, int32(3), requestCount.Load(),
					"transient errors should be retried")
			} else {
				assert.Equal(t, int32(1), requestCount.Load(),
					"non-transient errors should not be retried")
			}
		})
	}
}

func TestAnthropicProvider_ExtractKeywords_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		responseText string
		wantErrMsg   string
	}{
		{
			name:         "malformed JSON in content",
			responseText: `not valid json at all`,
			wantErrMsg:   "failed to parse LLM response as JSON",
		},
		{
			name:         "valid JSON but empty keywords",
			responseText: `{"keywords": [], "reasoning": "no keywords found"}`,
			wantErrMsg:   "LLM response contains no keywords",
		},
		{
			name:         "valid JSON but missing keywords field",
			responseText: `{"reasoning": "something"}`,
			wantErrMsg:   "LLM response contains no keywords",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := func(w http.ResponseWriter, r *http.Request) {
				resp := messagesResponse{
					ID:   "msg_test_invalid",
					Type: "message",
					Role: "assistant",
					Content: []contentBlock{
						{
							Type: "text",
							Text: tt.responseText,
						},
					},
					Model:      "claude-3-sonnet-20240229",
					StopReason: "end_turn",
					Usage: anthropicUsage{
						InputTokens:  100,
						OutputTokens: 20,
					},
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(resp)
			}

			srv := newAnthropicTestServer(t, handler)
			provider := newAnthropicTestProvider(srv.URL)

			req := ExtractionRequest{
				Text:        "test query",
				Mode:        ExtractionModeQuery,
				MaxKeywords: 5,
				MinKeywords: 2,
			}

			result, err := provider.ExtractKeywords(context.Background(), req)
			assert.Nil(t, result)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrMsg)
		})
	}
}

func TestAnthropicProvider_ExtractKeywords_EmptyContentBlocks(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		resp := messagesResponse{
			ID:      "msg_empty",
			Type:    "message",
			Role:    "assistant",
			Content: []contentBlock{},
			Model:   "claude-3-sonnet-20240229",
			Usage:   anthropicUsage{InputTokens: 50, OutputTokens: 0},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}

	srv := newAnthropicTestServer(t, handler)
	provider := newAnthropicTestProvider(srv.URL)

	req := ExtractionRequest{
		Text:        "test",
		Mode:        ExtractionModeQuery,
		MaxKeywords: 5,
		MinKeywords: 2,
	}

	result, err := provider.ExtractKeywords(context.Background(), req)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no content blocks")
}

func TestAnthropicProvider_ExtractKeywords_ContextCancelled(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		// Return a transient error to trigger a retry.
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(anthropicErrorResponse{
			Type: "error",
			Error: anthropicAPIErrorDetail{
				Type:    "rate_limit_error",
				Message: "rate limited",
			},
		})
	}

	srv := newAnthropicTestServer(t, handler)
	provider := newAnthropicTestProvider(srv.URL)
	provider.retryDelay = 500 * time.Millisecond // Long enough to cancel during wait.

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay to trigger during retry backoff.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	req := ExtractionRequest{
		Text:        "test",
		Mode:        ExtractionModeQuery,
		MaxKeywords: 5,
		MinKeywords: 2,
	}

	result, err := provider.ExtractKeywords(ctx, req)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context cancelled")
}

func TestAnthropicProvider_ExtractKeywords_RetryThenSuccess(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32

	handler := func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)

		if count < 3 {
			// First two requests return 500.
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(anthropicErrorResponse{
				Type: "error",
				Error: anthropicAPIErrorDetail{
					Type:    "api_error",
					Message: "internal error",
				},
			})
			return
		}

		// Third request succeeds.
		resp := messagesResponse{
			ID:   "msg_retry_success",
			Type: "message",
			Role: "assistant",
			Content: []contentBlock{
				{
					Type: "text",
					Text: `{"keywords": ["genomics"], "reasoning": "retry success"}`,
				},
			},
			Model:      "claude-3-sonnet-20240229",
			StopReason: "end_turn",
			Usage:      anthropicUsage{InputTokens: 80, OutputTokens: 15},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}

	srv := newAnthropicTestServer(t, handler)
	provider := newAnthropicTestProvider(srv.URL)

	req := ExtractionRequest{
		Text:        "genomics research",
		Mode:        ExtractionModeQuery,
		MaxKeywords: 5,
		MinKeywords: 1,
	}

	result, err := provider.ExtractKeywords(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, []string{"genomics"}, result.Keywords)
	assert.Equal(t, int32(3), requestCount.Load())
}

func TestAnthropicProvider_Provider(t *testing.T) {
	t.Parallel()

	provider := NewAnthropicProvider(config.AnthropicConfig{
		APIKey:  "key",
		Model:   "claude-3-sonnet-20240229",
		BaseURL: "https://api.anthropic.com",
	}, 0.7, 30*time.Second, 3)

	assert.Equal(t, "anthropic", provider.Provider())
}

func TestAnthropicProvider_Model(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model string
	}{
		{name: "sonnet", model: "claude-3-sonnet-20240229"},
		{name: "haiku", model: "claude-3-haiku-20240307"},
		{name: "opus", model: "claude-3-opus-20240229"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := NewAnthropicProvider(config.AnthropicConfig{
				APIKey:  "key",
				Model:   tt.model,
				BaseURL: "https://api.anthropic.com",
			}, 0.7, 30*time.Second, 3)

			assert.Equal(t, tt.model, provider.Model())
		})
	}
}
