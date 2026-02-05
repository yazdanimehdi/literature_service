package llm

import (
	"context"
	"errors"
	"strings"
	"testing"

	sharedllm "github.com/helixir/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockClient implements sharedllm.Client for testing.
type mockClient struct {
	completeFunc func(ctx context.Context, req sharedllm.Request) (*sharedllm.Response, error)
	provider     string
	model        string
}

func (m *mockClient) Complete(ctx context.Context, req sharedllm.Request) (*sharedllm.Response, error) {
	return m.completeFunc(ctx, req)
}

func (m *mockClient) Provider() string { return m.provider }
func (m *mockClient) Model() string    { return m.model }

// ---------------------------------------------------------------------------
// TestBuildExtractionPrompt
// ---------------------------------------------------------------------------

func TestBuildExtractionPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		req                    ExtractionRequest
		wantSystemContains     []string
		wantSystemNotContains  []string
		wantUserContains       []string
	}{
		{
			name: "query mode with no existing keywords",
			req: ExtractionRequest{
				Text:        "What is the role of p53 in cancer?",
				Mode:        ExtractionModeQuery,
				MaxKeywords: 10,
				MinKeywords: 3,
			},
			wantSystemContains: []string{
				"keyword extraction specialist",
				"JSON",
				`"keywords"`,
			},
			wantSystemNotContains: []string{
				"Already extracted",
			},
			wantUserContains: []string{
				"user query",
				"between 3 and 10 keywords",
				"What is the role of p53 in cancer?",
			},
		},
		{
			name: "abstract mode with existing keywords",
			req: ExtractionRequest{
				Text:             "We studied CRISPR-Cas9 for genome editing in zebrafish.",
				Mode:             ExtractionModeAbstract,
				MaxKeywords:      8,
				MinKeywords:      2,
				ExistingKeywords: []string{"CRISPR", "gene editing"},
			},
			wantSystemContains: []string{
				"keyword extraction specialist",
				"JSON",
				"Already extracted",
				"CRISPR",
				"gene editing",
			},
			wantUserContains: []string{
				"paper abstract",
				"between 2 and 8 keywords",
				"CRISPR-Cas9",
			},
		},
		{
			name: "different MaxKeywords values",
			req: ExtractionRequest{
				Text:        "Machine learning for drug discovery",
				Mode:        ExtractionModeQuery,
				MaxKeywords: 20,
				MinKeywords: 5,
			},
			wantUserContains: []string{
				"between 5 and 20 keywords",
			},
		},
		{
			name: "unknown mode falls back to default prompt",
			req: ExtractionRequest{
				Text:        "Some arbitrary text",
				Mode:        ExtractionMode("custom"),
				MaxKeywords: 5,
				MinKeywords: 1,
			},
			wantUserContains: []string{
				"Extract research keywords from the following text.",
				"between 1 and 5 keywords",
			},
		},
		{
			name: "with context field",
			req: ExtractionRequest{
				Text:        "Cancer immunotherapy",
				Mode:        ExtractionModeQuery,
				MaxKeywords: 10,
				MinKeywords: 3,
				Context:     "oncology",
			},
			wantUserContains: []string{
				"Research domain context: oncology",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			systemPrompt, userPrompt := BuildExtractionPrompt(tc.req)

			for _, want := range tc.wantSystemContains {
				assert.Contains(t, systemPrompt, want, "system prompt should contain %q", want)
			}
			for _, notWant := range tc.wantSystemNotContains {
				assert.NotContains(t, systemPrompt, notWant, "system prompt should not contain %q", notWant)
			}
			for _, want := range tc.wantUserContains {
				assert.Contains(t, userPrompt, want, "user prompt should contain %q", want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestBuildSystemPrompt
// ---------------------------------------------------------------------------

func TestBuildSystemPrompt(t *testing.T) {
	t.Parallel()

	t.Run("contains keyword extraction instruction", func(t *testing.T) {
		t.Parallel()
		prompt := buildSystemPrompt(ExtractionRequest{})
		assert.Contains(t, prompt, "keyword extraction specialist")
		assert.Contains(t, prompt, "extract")
	})

	t.Run("contains JSON format instruction", func(t *testing.T) {
		t.Parallel()
		prompt := buildSystemPrompt(ExtractionRequest{})
		assert.Contains(t, prompt, "JSON")
		assert.Contains(t, prompt, `"keywords"`)
		assert.Contains(t, prompt, `"reasoning"`)
	})

	t.Run("includes existing keywords section when provided", func(t *testing.T) {
		t.Parallel()
		prompt := buildSystemPrompt(ExtractionRequest{
			ExistingKeywords: []string{"alpha", "beta"},
		})
		assert.Contains(t, prompt, "Already extracted")
		assert.Contains(t, prompt, "alpha, beta")
		assert.Contains(t, prompt, "Do NOT repeat them")
	})

	t.Run("excludes existing keywords section when empty", func(t *testing.T) {
		t.Parallel()
		prompt := buildSystemPrompt(ExtractionRequest{})
		assert.NotContains(t, prompt, "Already extracted")
	})
}

// ---------------------------------------------------------------------------
// TestBuildUserPrompt
// ---------------------------------------------------------------------------

func TestBuildUserPrompt(t *testing.T) {
	t.Parallel()

	t.Run("query mode includes research question language", func(t *testing.T) {
		t.Parallel()
		prompt := buildUserPrompt(ExtractionRequest{
			Text:        "some query text",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 10,
			MinKeywords: 3,
		})
		assert.Contains(t, prompt, "user query")
		assert.Contains(t, prompt, "core research topics")
		assert.Contains(t, prompt, "some query text")
	})

	t.Run("abstract mode includes abstract language", func(t *testing.T) {
		t.Parallel()
		prompt := buildUserPrompt(ExtractionRequest{
			Text:        "some abstract text",
			Mode:        ExtractionModeAbstract,
			MaxKeywords: 8,
			MinKeywords: 2,
		})
		assert.Contains(t, prompt, "paper abstract")
		assert.Contains(t, prompt, "key findings")
		assert.Contains(t, prompt, "some abstract text")
	})

	t.Run("default mode for unknown extraction type", func(t *testing.T) {
		t.Parallel()
		prompt := buildUserPrompt(ExtractionRequest{
			Text:        "text",
			Mode:        ExtractionMode("other"),
			MaxKeywords: 5,
			MinKeywords: 1,
		})
		assert.Contains(t, prompt, "Extract research keywords from the following text.")
		// Should NOT contain mode-specific language.
		assert.NotContains(t, prompt, "user query")
		assert.NotContains(t, prompt, "paper abstract")
	})

	t.Run("includes keyword count constraints", func(t *testing.T) {
		t.Parallel()
		prompt := buildUserPrompt(ExtractionRequest{
			Text:        "text",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 15,
			MinKeywords: 4,
		})
		assert.Contains(t, prompt, "between 4 and 15 keywords")
	})

	t.Run("wraps text in delimiters", func(t *testing.T) {
		t.Parallel()
		prompt := buildUserPrompt(ExtractionRequest{
			Text:        "the input text",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 1,
		})
		assert.Contains(t, prompt, "---\nthe input text\n---")
	})

	t.Run("includes context when provided", func(t *testing.T) {
		t.Parallel()
		prompt := buildUserPrompt(ExtractionRequest{
			Text:        "text",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 1,
			Context:     "neuroscience",
		})
		assert.Contains(t, prompt, "Research domain context: neuroscience")
	})

	t.Run("excludes context when empty", func(t *testing.T) {
		t.Parallel()
		prompt := buildUserPrompt(ExtractionRequest{
			Text:        "text",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 1,
		})
		assert.NotContains(t, prompt, "Research domain context")
	})
}

// ---------------------------------------------------------------------------
// TestNewKeywordExtractorFromClient
// ---------------------------------------------------------------------------

func TestNewKeywordExtractorFromClient(t *testing.T) {
	t.Parallel()

	mock := &mockClient{provider: "test-provider", model: "test-model-v1"}
	extractor := NewKeywordExtractorFromClient(mock)

	require.NotNil(t, extractor)
	assert.Equal(t, "test-provider", extractor.Provider())
	assert.Equal(t, "test-model-v1", extractor.Model())
}

// ---------------------------------------------------------------------------
// TestClientAdapter_ExtractKeywords
// ---------------------------------------------------------------------------

func TestClientAdapter_ExtractKeywords(t *testing.T) {
	t.Parallel()

	t.Run("happy path returns parsed keywords", func(t *testing.T) {
		t.Parallel()

		mock := &mockClient{
			provider: "openai",
			model:    "gpt-4o",
			completeFunc: func(_ context.Context, req sharedllm.Request) (*sharedllm.Response, error) {
				// Verify the request structure.
				assert.Equal(t, "json", req.ResponseFormat)
				assert.Len(t, req.Messages, 2)
				assert.Equal(t, sharedllm.RoleSystem, req.Messages[0].Role)
				assert.Equal(t, sharedllm.RoleUser, req.Messages[1].Role)

				return &sharedllm.Response{
					Content: `{"keywords":["CRISPR","gene editing","Cas9"],"reasoning":"Core genome editing terms"}`,
					Model:   "gpt-4o",
					Usage: sharedllm.Usage{
						InputTokens:  150,
						OutputTokens: 30,
						TotalTokens:  180,
					},
				}, nil
			},
		}

		extractor := NewKeywordExtractorFromClient(mock)
		result, err := extractor.ExtractKeywords(context.Background(), ExtractionRequest{
			Text:        "What are the latest advances in CRISPR gene editing?",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 10,
			MinKeywords: 3,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, []string{"CRISPR", "gene editing", "Cas9"}, result.Keywords)
		assert.Equal(t, "Core genome editing terms", result.Reasoning)
		assert.Equal(t, "gpt-4o", result.Model)
		assert.Equal(t, 150, result.InputTokens)
		assert.Equal(t, 30, result.OutputTokens)
	})

	t.Run("error from client is wrapped with provider", func(t *testing.T) {
		t.Parallel()

		mock := &mockClient{
			provider: "anthropic",
			model:    "claude-3-sonnet",
			completeFunc: func(_ context.Context, _ sharedllm.Request) (*sharedllm.Response, error) {
				return nil, errors.New("connection refused")
			},
		}

		extractor := NewKeywordExtractorFromClient(mock)
		result, err := extractor.ExtractKeywords(context.Background(), ExtractionRequest{
			Text:        "Some research query",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 1,
		})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "anthropic")
		assert.Contains(t, err.Error(), "keyword extraction")
		assert.Contains(t, err.Error(), "connection refused")
	})

	t.Run("invalid JSON response returns error", func(t *testing.T) {
		t.Parallel()

		mock := &mockClient{
			provider: "openai",
			model:    "gpt-4o",
			completeFunc: func(_ context.Context, _ sharedllm.Request) (*sharedllm.Response, error) {
				return &sharedllm.Response{
					Content: `this is not valid JSON at all`,
					Model:   "gpt-4o",
				}, nil
			},
		}

		extractor := NewKeywordExtractorFromClient(mock)
		result, err := extractor.ExtractKeywords(context.Background(), ExtractionRequest{
			Text:        "Some text",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 1,
		})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to parse LLM response as JSON")
	})

	t.Run("empty keywords array in response returns error", func(t *testing.T) {
		t.Parallel()

		mock := &mockClient{
			provider: "openai",
			model:    "gpt-4o",
			completeFunc: func(_ context.Context, _ sharedllm.Request) (*sharedllm.Response, error) {
				return &sharedllm.Response{
					Content: `{"keywords":[],"reasoning":"No keywords found"}`,
					Model:   "gpt-4o",
				}, nil
			},
		}

		extractor := NewKeywordExtractorFromClient(mock)
		result, err := extractor.ExtractKeywords(context.Background(), ExtractionRequest{
			Text:        "Some text",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 1,
		})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no keywords")
	})

	t.Run("empty text input returns validation error", func(t *testing.T) {
		t.Parallel()

		mock := &mockClient{
			provider: "openai",
			model:    "gpt-4o",
			completeFunc: func(_ context.Context, _ sharedllm.Request) (*sharedllm.Response, error) {
				t.Fatal("Complete should not be called for empty text")
				return nil, nil
			},
		}

		extractor := NewKeywordExtractorFromClient(mock)
		result, err := extractor.ExtractKeywords(context.Background(), ExtractionRequest{
			Text:        "",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 1,
		})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "extraction text is required")
	})

	t.Run("whitespace-only text input returns validation error", func(t *testing.T) {
		t.Parallel()

		mock := &mockClient{
			provider: "openai",
			model:    "gpt-4o",
			completeFunc: func(_ context.Context, _ sharedllm.Request) (*sharedllm.Response, error) {
				t.Fatal("Complete should not be called for whitespace-only text")
				return nil, nil
			},
		}

		extractor := NewKeywordExtractorFromClient(mock)
		result, err := extractor.ExtractKeywords(context.Background(), ExtractionRequest{
			Text:        "   \t\n  ",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 1,
		})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "extraction text is required")
	})

	t.Run("context cancellation is propagated", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately.

		mock := &mockClient{
			provider: "openai",
			model:    "gpt-4o",
			completeFunc: func(ctx context.Context, _ sharedllm.Request) (*sharedllm.Response, error) {
				return nil, ctx.Err()
			},
		}

		extractor := NewKeywordExtractorFromClient(mock)
		result, err := extractor.ExtractKeywords(ctx, ExtractionRequest{
			Text:        "Some text",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 1,
		})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.True(t, errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled"),
			"error should indicate context cancellation, got: %v", err)
	})

	t.Run("JSON with missing keywords field returns error", func(t *testing.T) {
		t.Parallel()

		mock := &mockClient{
			provider: "openai",
			model:    "gpt-4o",
			completeFunc: func(_ context.Context, _ sharedllm.Request) (*sharedllm.Response, error) {
				return &sharedllm.Response{
					Content: `{"reasoning":"I found no terms"}`,
					Model:   "gpt-4o",
				}, nil
			},
		}

		extractor := NewKeywordExtractorFromClient(mock)
		result, err := extractor.ExtractKeywords(context.Background(), ExtractionRequest{
			Text:        "Some text",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 1,
		})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no keywords")
	})

	t.Run("response with no reasoning is accepted", func(t *testing.T) {
		t.Parallel()

		mock := &mockClient{
			provider: "openai",
			model:    "gpt-4o",
			completeFunc: func(_ context.Context, _ sharedllm.Request) (*sharedllm.Response, error) {
				return &sharedllm.Response{
					Content: `{"keywords":["p53","tumor suppressor"]}`,
					Model:   "gpt-4o",
					Usage:   sharedllm.Usage{InputTokens: 100, OutputTokens: 20},
				}, nil
			},
		}

		extractor := NewKeywordExtractorFromClient(mock)
		result, err := extractor.ExtractKeywords(context.Background(), ExtractionRequest{
			Text:        "Role of p53 in cancer",
			Mode:        ExtractionModeQuery,
			MaxKeywords: 5,
			MinKeywords: 1,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, []string{"p53", "tumor suppressor"}, result.Keywords)
		assert.Empty(t, result.Reasoning)
	})
}
