package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Default values for the OpenAI provider.
const (
	defaultOpenAIBaseURL    = "https://api.openai.com/v1"
	defaultOpenAIModel      = "gpt-4-turbo"
	defaultOpenAIMaxTokens  = 1024
	defaultOpenAIRetryDelay = 2 * time.Second
)

// chatRequest represents the OpenAI Chat Completions API request body.
type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

// chatMessage represents a single message in the chat conversation.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// responseFormat specifies the output format for the API response.
type responseFormat struct {
	Type string `json:"type"`
}

// chatResponse represents the OpenAI Chat Completions API response body.
type chatResponse struct {
	ID      string       `json:"id"`
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
}

// chatChoice represents a single completion choice.
type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// chatUsage contains token usage information.
type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// openAIErrorResponse represents an error response from the OpenAI API.
type openAIErrorResponse struct {
	Error openAIErrorDetail `json:"error"`
}

// openAIErrorDetail contains error details from the OpenAI API.
type openAIErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// OpenAIProvider implements KeywordExtractor using the OpenAI Chat Completions API.
type OpenAIProvider struct {
	httpClient *http.Client
	apiKey     string
	model      string
	baseURL    string
	temperature float64
	maxRetries  int
	retryDelay  time.Duration
}

// OpenAIConfig holds the parameters needed to create an OpenAI provider.
// This is defined in the llm package to avoid importing the config package.
type OpenAIConfig struct {
	// APIKey is the OpenAI API key.
	APIKey string
	// Model is the model identifier (e.g., "gpt-4-turbo").
	Model string
	// BaseURL is the API base URL (empty means default).
	BaseURL string
}

// NewOpenAIProvider creates a new OpenAI keyword extraction provider.
//
// The provider uses the OpenAI Chat Completions API with JSON response format
// for structured keyword extraction. It handles retry logic for transient API errors.
func NewOpenAIProvider(cfg OpenAIConfig, temperature float64, timeout time.Duration, maxRetries int) *OpenAIProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}

	model := cfg.Model
	if model == "" {
		model = defaultOpenAIModel
	}

	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	if maxRetries < 0 {
		maxRetries = 0
	}

	return &OpenAIProvider{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		apiKey:      cfg.APIKey,
		model:       model,
		baseURL:     baseURL,
		temperature: temperature,
		maxRetries:  maxRetries,
		retryDelay:  defaultOpenAIRetryDelay,
	}
}

// ExtractKeywords extracts research keywords from the given text using the OpenAI API.
//
// It builds the extraction prompt, sends a request to the Chat Completions API with
// JSON response format, and parses the structured response. Transient errors (5xx and
// 429) are retried up to maxRetries times with exponential-like backoff.
func (p *OpenAIProvider) ExtractKeywords(ctx context.Context, req ExtractionRequest) (*ExtractionResult, error) {
	systemPrompt, userPrompt := BuildExtractionPrompt(req)

	chatReq := chatRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: p.temperature,
		MaxTokens:   defaultOpenAIMaxTokens,
		ResponseFormat: &responseFormat{
			Type: "json_object",
		},
	}

	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			delay := p.retryDelay * time.Duration(attempt)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("openai: context cancelled during retry wait: %w", ctx.Err())
			case <-time.After(delay):
			}
		}

		result, err := p.doRequest(ctx, chatReq)
		if err == nil {
			return result, nil
		}

		// Only retry on transient errors (5xx, 429).
		if !isTransientError(err) {
			return nil, err
		}
		lastErr = err
	}

	return nil, fmt.Errorf("openai: exhausted %d retries: %w", p.maxRetries, lastErr)
}

// Provider returns the name of the LLM provider.
func (p *OpenAIProvider) Provider() string {
	return "openai"
}

// Model returns the model identifier being used.
func (p *OpenAIProvider) Model() string {
	return p.model
}

// doRequest performs a single API request to the OpenAI Chat Completions endpoint.
func (p *OpenAIProvider) doRequest(ctx context.Context, chatReq chatRequest) (*ExtractionResult, error) {
	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to marshal request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("openai: failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseOpenAIAPIError(resp.StatusCode, respBody)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("openai: failed to unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: empty choices in response")
	}

	content := chatResp.Choices[0].Message.Content
	var llmResp llmResponse
	if err := json.Unmarshal([]byte(content), &llmResp); err != nil {
		return nil, fmt.Errorf("openai: failed to parse LLM JSON response: %w", err)
	}

	if len(llmResp.Keywords) == 0 {
		return nil, fmt.Errorf("openai: LLM returned no keywords")
	}

	return &ExtractionResult{
		Keywords:     llmResp.Keywords,
		Reasoning:    llmResp.Reasoning,
		Model:        p.model,
		InputTokens:  chatResp.Usage.PromptTokens,
		OutputTokens: chatResp.Usage.CompletionTokens,
	}, nil
}

// parseOpenAIAPIError parses an OpenAI API error from the response status code and body.
func parseOpenAIAPIError(statusCode int, body []byte) *APIError {
	apiErr := &APIError{
		Provider:   "openai",
		StatusCode: statusCode,
		Message:    string(body),
	}

	var errResp openAIErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		apiErr.Message = errResp.Error.Message
		apiErr.Type = errResp.Error.Type
		apiErr.Code = errResp.Error.Code
	}

	return apiErr
}
