package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time check that OpenAIProvider implements KeywordExtractor.
var _ KeywordExtractor = (*OpenAIProvider)(nil)

// newOpenAITestServer creates an httptest server that responds with the given handler.
func newOpenAITestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// newOpenAITestProvider creates an OpenAIProvider configured to use the test server.
func newOpenAITestProvider(t *testing.T, serverURL string) *OpenAIProvider {
	t.Helper()
	cfg := OpenAIConfig{
		APIKey:  "test-api-key",
		Model:   "gpt-4-turbo",
		BaseURL: serverURL,
	}
	provider := NewOpenAIProvider(cfg, 0.3, 10*time.Second, 0)
	return provider
}

func TestOpenAIProvider_ExtractKeywords(t *testing.T) {
	t.Run("successful extraction returns keywords and metadata", func(t *testing.T) {
		var receivedReq chatRequest
		var receivedAuthHeader string
		var receivedContentType string

		server := newOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
			receivedAuthHeader = r.Header.Get("Authorization")
			receivedContentType = r.Header.Get("Content-Type")

			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			defer r.Body.Close()

			err = json.Unmarshal(body, &receivedReq)
			require.NoError(t, err)

			resp := chatResponse{
				ID: "chatcmpl-abc123",
				Choices: []chatChoice{
					{
						Index: 0,
						Message: chatMessage{
							Role:    "assistant",
							Content: `{"keywords": ["CRISPR", "gene editing", "Cas9", "genome engineering", "guide RNA"], "reasoning": "Extracted core CRISPR-related terms for academic database search."}`,
						},
						FinishReason: "stop",
					},
				},
				Usage: chatUsage{
					PromptTokens:     150,
					CompletionTokens: 45,
					TotalTokens:      195,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		})

		provider := newOpenAITestProvider(t, server.URL)
		req := ExtractionRequest{
			Text:        "What are the latest advances in CRISPR gene editing for therapeutic applications?",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 10,
			MinKeywords: 3,
		}

		result, err := provider.ExtractKeywords(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify extracted keywords.
		assert.Equal(t, []string{"CRISPR", "gene editing", "Cas9", "genome engineering", "guide RNA"}, result.Keywords)
		assert.Equal(t, "Extracted core CRISPR-related terms for academic database search.", result.Reasoning)
		assert.Equal(t, "gpt-4-turbo", result.Model)
		assert.Equal(t, 150, result.InputTokens)
		assert.Equal(t, 45, result.OutputTokens)

		// Verify request was correctly formed.
		assert.Equal(t, "Bearer test-api-key", receivedAuthHeader)
		assert.Equal(t, "application/json", receivedContentType)
		assert.Equal(t, "gpt-4-turbo", receivedReq.Model)
		assert.Equal(t, float64(0.3), receivedReq.Temperature)
		require.NotNil(t, receivedReq.ResponseFormat)
		assert.Equal(t, "json_object", receivedReq.ResponseFormat.Type)

		// Verify messages contain system and user prompts.
		require.Len(t, receivedReq.Messages, 2)
		assert.Equal(t, "system", receivedReq.Messages[0].Role)
		assert.Equal(t, "user", receivedReq.Messages[1].Role)
		assert.Contains(t, receivedReq.Messages[0].Content, "keyword extraction specialist")
		assert.Contains(t, receivedReq.Messages[1].Content, "CRISPR gene editing")
	})

	t.Run("context cancellation stops request", func(t *testing.T) {
		server := newOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Simulate a slow server that never responds in time.
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusOK)
		})

		provider := newOpenAITestProvider(t, server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		req := ExtractionRequest{
			Text:        "test query",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 2,
		}

		_, err := provider.ExtractKeywords(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "openai:")
	})
}

func TestOpenAIProvider_ExtractKeywords_WithExistingKeywords(t *testing.T) {
	t.Run("existing keywords are included in prompt and not duplicated in result", func(t *testing.T) {
		var receivedReq chatRequest

		server := newOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			defer r.Body.Close()

			err = json.Unmarshal(body, &receivedReq)
			require.NoError(t, err)

			resp := chatResponse{
				ID: "chatcmpl-existing-kw",
				Choices: []chatChoice{
					{
						Index: 0,
						Message: chatMessage{
							Role:    "assistant",
							Content: `{"keywords": ["genome engineering", "nuclease", "homologous recombination"], "reasoning": "Complementary terms avoiding existing keywords."}`,
						},
						FinishReason: "stop",
					},
				},
				Usage: chatUsage{
					PromptTokens:     200,
					CompletionTokens: 30,
					TotalTokens:      230,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		})

		provider := newOpenAITestProvider(t, server.URL)
		req := ExtractionRequest{
			Text:             "What are the latest advances in CRISPR gene editing?",
			Mode:             ExtractionModeQuery,
			MaxKeywords:      10,
			MinKeywords:      3,
			ExistingKeywords: []string{"CRISPR", "gene editing", "Cas9"},
		}

		result, err := provider.ExtractKeywords(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify new keywords were returned.
		assert.Equal(t, []string{"genome engineering", "nuclease", "homologous recombination"}, result.Keywords)
		assert.Equal(t, "Complementary terms avoiding existing keywords.", result.Reasoning)

		// Verify the system prompt mentions existing keywords.
		require.Len(t, receivedReq.Messages, 2)
		systemPrompt := receivedReq.Messages[0].Content
		assert.Contains(t, systemPrompt, "CRISPR")
		assert.Contains(t, systemPrompt, "gene editing")
		assert.Contains(t, systemPrompt, "Cas9")
		assert.Contains(t, systemPrompt, "already been extracted")
	})
}

func TestOpenAIProvider_ExtractKeywords_APIError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		wantErrContain string
	}{
		{
			name:       "401 unauthorized with structured error",
			statusCode: http.StatusUnauthorized,
			responseBody: `{
				"error": {
					"message": "Incorrect API key provided: test-a...key.",
					"type": "invalid_request_error",
					"code": "invalid_api_key"
				}
			}`,
			wantErrContain: "Incorrect API key provided",
		},
		{
			name:       "400 bad request",
			statusCode: http.StatusBadRequest,
			responseBody: `{
				"error": {
					"message": "Invalid model specified.",
					"type": "invalid_request_error",
					"code": "model_not_found"
				}
			}`,
			wantErrContain: "Invalid model specified",
		},
		{
			name:           "429 rate limit with retry exhaustion",
			statusCode:     http.StatusTooManyRequests,
			responseBody:   `{"error": {"message": "Rate limit exceeded", "type": "rate_limit_error", "code": "rate_limit_exceeded"}}`,
			wantErrContain: "exhausted",
		},
		{
			name:           "500 internal server error with retry exhaustion",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `{"error": {"message": "Internal server error", "type": "server_error", "code": "server_error"}}`,
			wantErrContain: "exhausted",
		},
		{
			name:           "503 service unavailable",
			statusCode:     http.StatusServiceUnavailable,
			responseBody:   `{"error": {"message": "Service temporarily unavailable", "type": "server_error", "code": "service_unavailable"}}`,
			wantErrContain: "exhausted",
		},
		{
			name:           "non-JSON error body",
			statusCode:     http.StatusForbidden,
			responseBody:   "Forbidden: access denied",
			wantErrContain: "Forbidden: access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestCount := 0
			server := newOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			})

			cfg := OpenAIConfig{
				APIKey:  "test-api-key",
				Model:   "gpt-4-turbo",
				BaseURL: server.URL,
			}
			// Use 1 retry for transient errors, 0 retries for non-transient.
			retries := 1
			provider := NewOpenAIProvider(cfg, 0.3, 10*time.Second, retries)
			// Reduce retry delay for fast test execution.
			provider.retryDelay = 10 * time.Millisecond

			req := ExtractionRequest{
				Text:        "test query",
				Mode:        ExtractionModeQuery,
				MaxKeywords: 5,
				MinKeywords: 2,
			}

			_, err := provider.ExtractKeywords(context.Background(), req)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrContain)

			// Transient errors should be retried.
			isTransient := tt.statusCode == http.StatusTooManyRequests || tt.statusCode >= 500
			if isTransient {
				assert.Equal(t, retries+1, requestCount, "transient error should trigger retries")
			} else {
				assert.Equal(t, 1, requestCount, "non-transient error should not be retried")
			}
		})
	}
}

func TestOpenAIProvider_ExtractKeywords_InvalidJSON(t *testing.T) {
	t.Run("LLM returns non-JSON content", func(t *testing.T) {
		server := newOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
			resp := chatResponse{
				ID: "chatcmpl-badjson",
				Choices: []chatChoice{
					{
						Index: 0,
						Message: chatMessage{
							Role:    "assistant",
							Content: "Here are some keywords: CRISPR, gene editing, Cas9",
						},
						FinishReason: "stop",
					},
				},
				Usage: chatUsage{
					PromptTokens:     100,
					CompletionTokens: 20,
					TotalTokens:      120,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		})

		provider := newOpenAITestProvider(t, server.URL)
		req := ExtractionRequest{
			Text:        "CRISPR gene editing",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 2,
		}

		_, err := provider.ExtractKeywords(context.Background(), req)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "openai: failed to parse LLM JSON response")
	})

	t.Run("LLM returns valid JSON but empty keywords", func(t *testing.T) {
		server := newOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
			resp := chatResponse{
				ID: "chatcmpl-emptykw",
				Choices: []chatChoice{
					{
						Index: 0,
						Message: chatMessage{
							Role:    "assistant",
							Content: `{"keywords": [], "reasoning": "No keywords found."}`,
						},
						FinishReason: "stop",
					},
				},
				Usage: chatUsage{
					PromptTokens:     100,
					CompletionTokens: 15,
					TotalTokens:      115,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		})

		provider := newOpenAITestProvider(t, server.URL)
		req := ExtractionRequest{
			Text:        "random non-scientific text",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 2,
		}

		_, err := provider.ExtractKeywords(context.Background(), req)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "openai: LLM returned no keywords")
	})

	t.Run("LLM returns malformed JSON in chat response wrapper", func(t *testing.T) {
		server := newOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Return a response body that is not valid JSON.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`not valid json at all`))
		})

		provider := newOpenAITestProvider(t, server.URL)
		req := ExtractionRequest{
			Text:        "test query",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 2,
		}

		_, err := provider.ExtractKeywords(context.Background(), req)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "openai: failed to unmarshal response")
	})

	t.Run("API returns empty choices array", func(t *testing.T) {
		server := newOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
			resp := chatResponse{
				ID:      "chatcmpl-nochoices",
				Choices: []chatChoice{},
				Usage: chatUsage{
					PromptTokens:     100,
					CompletionTokens: 0,
					TotalTokens:      100,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		})

		provider := newOpenAITestProvider(t, server.URL)
		req := ExtractionRequest{
			Text:        "test query",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 2,
		}

		_, err := provider.ExtractKeywords(context.Background(), req)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "openai: empty choices in response")
	})
}

func TestOpenAIProvider_Provider(t *testing.T) {
	provider := NewOpenAIProvider(OpenAIConfig{}, 0.5, 30*time.Second, 3)
	assert.Equal(t, "openai", provider.Provider())
}

func TestOpenAIProvider_Model(t *testing.T) {
	t.Run("returns configured model", func(t *testing.T) {
		cfg := OpenAIConfig{
			Model: "gpt-4o",
		}
		provider := NewOpenAIProvider(cfg, 0.5, 30*time.Second, 3)
		assert.Equal(t, "gpt-4o", provider.Model())
	})

	t.Run("returns default model when not configured", func(t *testing.T) {
		provider := NewOpenAIProvider(OpenAIConfig{}, 0.5, 30*time.Second, 3)
		assert.Equal(t, defaultOpenAIModel, provider.Model())
	})
}

func TestNewOpenAIProvider(t *testing.T) {
	t.Run("applies default values for empty config", func(t *testing.T) {
		provider := NewOpenAIProvider(OpenAIConfig{}, 0.7, 0, -1)

		assert.Equal(t, defaultOpenAIBaseURL, provider.baseURL)
		assert.Equal(t, defaultOpenAIModel, provider.model)
		assert.Equal(t, 0.7, provider.temperature)
		assert.Equal(t, 0, provider.maxRetries)
		assert.NotNil(t, provider.httpClient)
	})

	t.Run("uses provided config values", func(t *testing.T) {
		cfg := OpenAIConfig{
			APIKey:  "sk-test-key",
			Model:   "gpt-4o-mini",
			BaseURL: "https://custom-api.example.com/v1",
		}
		provider := NewOpenAIProvider(cfg, 0.2, 45*time.Second, 5)

		assert.Equal(t, "https://custom-api.example.com/v1", provider.baseURL)
		assert.Equal(t, "gpt-4o-mini", provider.model)
		assert.Equal(t, "sk-test-key", provider.apiKey)
		assert.Equal(t, 0.2, provider.temperature)
		assert.Equal(t, 5, provider.maxRetries)
	})
}

func TestAPIError(t *testing.T) {
	t.Run("Error() formats message correctly with type", func(t *testing.T) {
		err := &APIError{
			Provider:   "openai",
			StatusCode: 401,
			Message:    "Invalid API key",
			Type:       "invalid_request_error",
			Code:       "invalid_api_key",
		}
		assert.Equal(t, "openai: API error (status 401, type invalid_request_error): Invalid API key", err.Error())
	})

	t.Run("Error() formats message correctly without type", func(t *testing.T) {
		err := &APIError{
			Provider:   "openai",
			StatusCode: 401,
			Message:    "Invalid API key",
		}
		assert.Equal(t, "openai: API error (status 401): Invalid API key", err.Error())
	})

	t.Run("IsTransient returns true for status 0 (network error)", func(t *testing.T) {
		err := &APIError{StatusCode: 0}
		assert.True(t, err.IsTransient())
	})

	t.Run("IsTransient returns true for 429", func(t *testing.T) {
		err := &APIError{StatusCode: http.StatusTooManyRequests}
		assert.True(t, err.IsTransient())
	})

	t.Run("IsTransient returns true for 500", func(t *testing.T) {
		err := &APIError{StatusCode: http.StatusInternalServerError}
		assert.True(t, err.IsTransient())
	})

	t.Run("IsTransient returns true for 503", func(t *testing.T) {
		err := &APIError{StatusCode: http.StatusServiceUnavailable}
		assert.True(t, err.IsTransient())
	})

	t.Run("IsTransient returns false for 400", func(t *testing.T) {
		err := &APIError{StatusCode: http.StatusBadRequest}
		assert.False(t, err.IsTransient())
	})

	t.Run("IsTransient returns false for 401", func(t *testing.T) {
		err := &APIError{StatusCode: http.StatusUnauthorized}
		assert.False(t, err.IsTransient())
	})

	t.Run("IsTransient returns false for 403", func(t *testing.T) {
		err := &APIError{StatusCode: http.StatusForbidden}
		assert.False(t, err.IsTransient())
	})
}

func TestIsTransientError(t *testing.T) {
	t.Run("returns true for transient APIError", func(t *testing.T) {
		err := &APIError{StatusCode: http.StatusTooManyRequests}
		assert.True(t, isTransientError(err))
	})

	t.Run("returns false for non-transient APIError", func(t *testing.T) {
		err := &APIError{StatusCode: http.StatusBadRequest}
		assert.False(t, isTransientError(err))
	})

	t.Run("returns false for non-APIError", func(t *testing.T) {
		err := context.DeadlineExceeded
		assert.False(t, isTransientError(err))
	})
}
